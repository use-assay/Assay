// Package horizon fetches asset and issuer-account state from Horizon.
//
// This package is deliberately small and dependency-free: it is one of the
// fetchers earmarked for extraction into a shared ledger-access library once
// a second project needs it. Keep the signatures clean and the types local.
package horizon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// DefaultURL is the public Horizon instance for pubnet.
const DefaultURL = "https://horizon.stellar.org"

// ErrNotFound reports that Horizon has no record of the asset or account.
var ErrNotFound = errors.New("horizon: not found")

// Flags mirrors the issuer authorization flags exactly as Horizon names them.
//
// The field names are the wire names, verified against live Horizon responses
// for both /assets and /accounts. Do not rename them to something tidier: the
// whole point of this scanner is that it reports the ledger's own vocabulary.
type Flags struct {
	AuthRequired        bool `json:"auth_required"`
	AuthRevocable       bool `json:"auth_revocable"`
	AuthImmutable       bool `json:"auth_immutable"`
	AuthClawbackEnabled bool `json:"auth_clawback_enabled"`
}

// AssetStat is the subset of Horizon's /assets record that Assay reasons about.
type AssetStat struct {
	AssetType   string `json:"asset_type"`
	AssetCode   string `json:"asset_code"`
	AssetIssuer string `json:"asset_issuer"`
	ContractID  string `json:"contract_id"`
	Flags       Flags  `json:"flags"`
	Accounts    struct {
		Authorized   int `json:"authorized"`
		Unauthorized int `json:"unauthorized"`
	} `json:"accounts"`
	Links struct {
		Toml struct {
			Href string `json:"href"`
		} `json:"toml"`
	} `json:"_links"`
}

// Account is the subset of Horizon's /accounts record that Assay reasons about.
type Account struct {
	AccountID  string `json:"account_id"`
	HomeDomain string `json:"home_domain"`
	Flags      Flags  `json:"flags"`
}

// Client reads from a Horizon instance.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a Client for the given Horizon base URL, defaulting to pubnet.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Asset returns the asset statistics record for code/issuer.
func (c *Client) Asset(ctx context.Context, code, issuer string) (*AssetStat, error) {
	q := url.Values{}
	q.Set("asset_code", code)
	q.Set("asset_issuer", issuer)

	var page struct {
		Embedded struct {
			Records []AssetStat `json:"records"`
		} `json:"_embedded"`
	}
	if err := c.get(ctx, "/assets?"+q.Encode(), &page); err != nil {
		return nil, err
	}
	if len(page.Embedded.Records) == 0 {
		return nil, fmt.Errorf("%w: asset %s-%s", ErrNotFound, code, issuer)
	}
	return &page.Embedded.Records[0], nil
}

// Account returns the account record for the given account ID.
func (c *Client) Account(ctx context.Context, id string) (*Account, error) {
	var a Account
	if err := c.get(ctx, "/accounts/"+url.PathEscape(id), &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("horizon: get %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("horizon: get %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("horizon: decode %s: %w", path, err)
	}
	return nil
}
