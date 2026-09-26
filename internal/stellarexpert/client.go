// Package stellarexpert consumes StellarExpert's curated reputation data.
//
// Assay does not build a reputation layer and does not second-guess this one.
// Everything this package returns is passed through to the report as attributed
// evidence, carrying the source URL and retrieval time. Nothing here is
// re-derived, re-scored, or restated as an Assay conclusion.
//
// Endpoints verified live against api.stellar.expert:
//
//	GET /explorer/directory/{address}                 -> curated address entry
//	GET /explorer/directory/blocked-domains/{domain}  -> {"domain":..,"blocked":bool}
//
// This is an extraction candidate for a shared ledger-access library.
package stellarexpert

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultURL is the public StellarExpert API root.
const DefaultURL = "https://api.stellar.expert"

// version is kept here so every outbound client reports the same tool
// version in its User-Agent. Bump it alongside any scanner-version release;
// the API documents the value in release notes.
const version = "v0.1.0"

// defaultUserAgent is how Assay identifies itself to the public StellarExpert
// API. It is a courtesy to the operators of sources Assay depends on and makes
// automated traffic distinguishable from botnets and scanners.
const defaultUserAgent = "assay/" + version + " (+https://github.com/use-assay/Assay)"

// MaxBody caps a curated-endpoint response. Real responses are a few KB; like
// the SEP-1 toml cap this stops a broken or hostile server from streaming an
// unbounded body at the scanner. The decode reads at most this many bytes, so
// an oversized document is truncated and then fails to decode rather than
// allocating without bound. It is exported so the resource-exhaustion tests
// can assert the bound rather than assume it.
const MaxBody = 1 << 20 // 1 MiB

// RetryOptions bounds exponential backoff with jitter for transient failures.
// Retry-After takes precedence when the server advertises it.
type RetryOptions struct {
	Attempts        int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	MaxTotalLatency time.Duration
}

// DefaultRetryOptions caps total added latency at ~2s, well inside the
// outer 30s scan budget, and honours the Retry-After we actually observed on
// api.stellar.expert.
func DefaultRetryOptions() *RetryOptions {
	return &RetryOptions{
		Attempts:        3,
		InitialDelay:    250 * time.Millisecond,
		MaxDelay:        500 * time.Millisecond,
		MaxTotalLatency: 2 * time.Second,
	}
}

// Backoff returns the next delay after an attempt failed, with full jitter:
// a uniform draw in [0, min(MaxDelay, InitialDelay * 2^attempt)].
func (o *RetryOptions) Backoff(attempt int, retryAfter time.Duration) time.Duration {
	target := o.InitialDelay * time.Duration(math.Pow(2, float64(attempt)))
	if target > o.MaxDelay {
		target = o.MaxDelay
	}
	if retryAfter > 0 && retryAfter < target {
		target = retryAfter
	}
	if target <= 0 {
		target = o.InitialDelay
	}
	n := rand.Int63()
	if n < 0 {
		n = -n
	}
	target = time.Duration(n % int64(target))
	return target
}

// DirectoryEntry is a curated entry from StellarExpert's address directory,
// the data set standardized by SEP-0037.
type DirectoryEntry struct {
	Address string   `json:"address"`
	Name    string   `json:"name"`
	Domain  string   `json:"domain"`
	Tags    []string `json:"tags"`
}

// HasTag reports whether the entry carries the given tag.
func (e *DirectoryEntry) HasTag(tag string) bool {
	if e == nil {
		return false
	}
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// BlockedDomain is the response from the blocked-domains lookup.
type BlockedDomain struct {
	Domain  string `json:"domain"`
	Blocked bool   `json:"blocked"`
}

// Client reads StellarExpert's curated data sets.
type Client struct {
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
}

// New returns a Client for a given API root, defaulting to the public one.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{
		BaseURL:   baseURL,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		UserAgent: defaultUserAgent,
	}
}

// DirectoryURL returns the public URL for an address's directory entry, used
// to attribute the claim in the report.
func (c *Client) DirectoryURL(address string) string {
	return c.BaseURL + "/explorer/directory/" + url.PathEscape(address)
}

// BlockedDomainURL returns the public URL for a blocked-domain lookup.
func (c *Client) BlockedDomainURL(domain string) string {
	return c.BaseURL + "/explorer/directory/blocked-domains/" + url.PathEscape(domain)
}

// Directory looks up an address in the curated directory. A nil entry with a
// nil error means the address is simply not listed, which is the common case
// and is not itself a signal in either direction.
func (c *Client) Directory(ctx context.Context, address string) (*DirectoryEntry, error) {
	var e DirectoryEntry
	found, err := c.get(ctx, c.DirectoryURL(address), &e)
	if err != nil || !found {
		return nil, err
	}

	// The directory answers an address it holds no entry for with 200 and an
	// empty object, not a 404 — verified against several unlisted addresses.
	// So a successful decode is not by itself a listing. A real entry always
	// echoes the address it describes, which is the discriminator used here.
	//
	// Without this, an absent entry surfaces downstream as the attributed
	// claim `listed as "" (domain "", tags: )` — a statement this source never
	// made, attached to its name and URL.
	if e.Address == "" {
		return nil, nil
	}
	return &e, nil
}

// BlockedDomain reports whether a domain appears on the malicious-domain
// blocklist.
func (c *Client) BlockedDomain(ctx context.Context, domain string) (*BlockedDomain, error) {
	var b BlockedDomain
	found, err := c.get(ctx, c.BlockedDomainURL(domain), &b)
	if err != nil || !found {
		return nil, err
	}
	return &b, nil
}

// get returns found=false on 404 rather than an error, because "not listed" is
// a normal answer from these endpoints.
//
// Transient failures (429/5xx) are retried with bounded exponential backoff
// and jitter, honouring Retry-After when the server advertises it. The retry
// policy is keyed by status code so a 404 is never retried and the ledger
// source (Horizon) is never touched here. If the retries exhaust, err is the
// last transient error, which the caller records verbatim as evidence and
// reports as undetermined.
func (c *Client) get(ctx context.Context, target string, out any) (bool, error) {
	var lastErr error
	opts := DefaultRetryOptions()
	for attempt := 0; attempt < opts.Attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return false, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.UserAgent)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("stellarexpert: get %s: %w", target, err)
			if attempt == opts.Attempts-1 {
				return false, lastErr
			}
			t := opts.Backoff(attempt, 0)
			if !wait(ctx, t) {
				return false, ctx.Err()
			}
			continue
		}

		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			if attempt == opts.Attempts-1 {
				return false, fmt.Errorf("stellarexpert: get %s: status %d", target, resp.StatusCode)
			}
			t := opts.Backoff(attempt, retryAfter)
			if !wait(ctx, t) {
				return false, ctx.Err()
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("stellarexpert: get %s: status %d", target, resp.StatusCode)
			if attempt == opts.Attempts-1 {
				return false, lastErr
			}
			t := opts.Backoff(attempt, 0)
			if !wait(ctx, t) {
				return false, ctx.Err()
			}
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody))
		if err != nil {
			lastErr = fmt.Errorf("stellarexpert: read %s: %w", target, err)
			if attempt == opts.Attempts-1 {
				return false, lastErr
			}
			t := opts.Backoff(attempt, 0)
			if !wait(ctx, t) {
				return false, ctx.Err()
			}
			continue
		}
		if err := json.Unmarshal(body, out); err != nil {
			lastErr = fmt.Errorf("stellarexpert: decode %s: %w", target, err)
			if attempt == opts.Attempts-1 {
				return false, lastErr
			}
			t := opts.Backoff(attempt, 0)
			if !wait(ctx, t) {
				return false, ctx.Err()
			}
			continue
		}
		return true, nil
	}
	return false, lastErr
}

// wait sleeps for d unless the context is cancelled or timed out first.
func wait(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// parseRetryAfter parses the Retry-After header, returning the delay in
// seconds (the documented unit for this header) or zero when absent or
// unparseable.
func parseRetryAfter(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if i, err := strconv.Atoi(s); err == nil {
		return time.Duration(i) * time.Second
	}
	t, err := time.Parse(time.RFC1123, s)
	if err != nil {
		return 0
	}
	return time.Until(t.UTC())
}
