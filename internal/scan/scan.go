// Package scan assembles a mechanics.Subject from live sources and runs the
// engine over it.
//
// This is the only place in Assay that performs network I/O for a scan. It
// exists so that the checks never do: every fetch happens here, once, and the
// result is handed to pure classifiers. That split is what makes the checks
// testable without a network and keeps each fetcher independently extractable.
package scan

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/use-assay/assay/internal/horizon"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/sep1"
	"github.com/use-assay/assay/internal/stellarexpert"
)

// issuerRE matches a Stellar ed25519 public key.
var issuerRE = regexp.MustCompile(`^G[A-Z2-7]{55}$`)

// codeRE matches a valid classic asset code (1-12 alphanumeric).
var codeRE = regexp.MustCompile(`^[A-Za-z0-9]{1,12}$`)

// ErrBadAsset reports an unparseable asset identifier.
var ErrBadAsset = errors.New("scan: invalid asset")

// ParseAsset accepts the canonical CODE-ISSUER form and validates both halves.
func ParseAsset(s string) (mechanics.Asset, error) {
	s = strings.TrimSpace(s)
	code, issuer, ok := strings.Cut(s, "-")
	if !ok {
		return mechanics.Asset{}, fmt.Errorf("%w: expected CODE-ISSUER, got %q", ErrBadAsset, s)
	}
	// Trailing "-1"/"-2" suffixes appear in some explorer asset identifiers.
	if i := strings.Index(issuer, "-"); i >= 0 {
		issuer = issuer[:i]
	}
	if !codeRE.MatchString(code) {
		return mechanics.Asset{}, fmt.Errorf("%w: bad asset code %q", ErrBadAsset, code)
	}
	if !issuerRE.MatchString(issuer) {
		return mechanics.Asset{}, fmt.Errorf("%w: bad issuer %q", ErrBadAsset, issuer)
	}
	return mechanics.Asset{Code: code, Issuer: issuer}, nil
}

// Scanner fetches subject state and classifies it.
type Scanner struct {
	Horizon *horizon.Client
	Toml    *sep1.Fetcher
	Expert  *stellarexpert.Client
	Engine  *mechanics.Engine
}

// New returns a Scanner wired to the public production sources.
func New() *Scanner {
	return &Scanner{
		Horizon: horizon.New(""),
		Toml:    sep1.NewFetcher(),
		Expert:  stellarexpert.New(""),
		Engine:  mechanics.NewEngine(),
	}
}

// Subject fetches everything the checks need for one asset.
//
// Only the ledger lookups are fatal: without issuer flags there is no
// classification to make. Every consumed signal is best-effort, because a
// third-party outage must not be able to turn a dangerous asset into an error
// page. When a source is unreachable the failure is recorded verbatim and
// surfaced, never smoothed into a false negative.
func (s *Scanner) Subject(ctx context.Context, a mechanics.Asset) (*mechanics.Subject, error) {
	sub := &mechanics.Subject{Asset: a, FetchedAt: time.Now().UTC()}

	stat, err := s.Horizon.Asset(ctx, a.Code, a.Issuer)
	if err != nil {
		return nil, err
	}
	sub.Stat = stat

	issuer, err := s.Horizon.Account(ctx, a.Issuer)
	if err != nil {
		return nil, err
	}
	sub.Issuer = issuer

	if domain := issuer.HomeDomain; domain != "" {
		sub.TomlURL = sep1.URLFor(domain)
		doc, err := s.Toml.Fetch(ctx, domain)
		if err != nil {
			sub.TomlErr = err.Error()
		} else {
			sub.Toml = doc
		}

		sub.BlockedURL = s.Expert.BlockedDomainURL(domain)
		if blocked, err := s.Expert.BlockedDomain(ctx, domain); err == nil {
			sub.Blocked = blocked
		}
	}

	sub.DirectoryURL = s.Expert.DirectoryURL(a.Issuer)
	if entry, err := s.Expert.Directory(ctx, a.Issuer); err == nil {
		sub.Directory = entry
	}

	return sub, nil
}

// Scan fetches and classifies an asset.
func (s *Scanner) Scan(ctx context.Context, a mechanics.Asset) (*mechanics.Report, error) {
	sub, err := s.Subject(ctx, a)
	if err != nil {
		return nil, err
	}
	return s.Engine.Run(ctx, sub)
}
