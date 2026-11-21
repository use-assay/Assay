package mechanics

import (
	"fmt"
	"net/url"

	"github.com/use-assay/assay/internal/horizon"
)

// horizonAssetURL returns the Horizon URL backing an asset claim, so the
// reader can re-fetch exactly what Assay read.
//
// If baseURL is empty, horizon.DefaultURL is used.
func horizonAssetURL(a Asset, baseURL string) string {
	q := url.Values{}
	q.Set("asset_code", a.Code)
	q.Set("asset_issuer", a.Issuer)
	if baseURL == "" {
		baseURL = horizon.DefaultURL
	}
	return baseURL + "/assets?" + q.Encode()
}

// horizonAccountURL returns the Horizon URL for the issuer account, so a reader
// can re-fetch the second copy of the authorization flags themselves.
//
// If baseURL is empty, horizon.DefaultURL is used.
func horizonAccountURL(issuer string, baseURL string) string {
	if baseURL == "" {
		baseURL = horizon.DefaultURL
	}
	return baseURL + "/accounts/" + url.PathEscape(issuer)
}

// flagSummary renders the issuer flag set in Horizon's own vocabulary.
func flagSummary(f horizon.Flags) string {
	return fmt.Sprintf(
		"auth_required=%t auth_revocable=%t auth_immutable=%t auth_clawback_enabled=%t",
		f.AuthRequired, f.AuthRevocable, f.AuthImmutable, f.AuthClawbackEnabled,
	)
}
