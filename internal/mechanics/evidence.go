package mechanics

import (
	"fmt"
	"net/url"

	"github.com/use-assay/assay/internal/horizon"
)

// horizonAssetURL returns the public Horizon URL backing an asset claim, so the
// reader can re-fetch exactly what Assay read.
func horizonAssetURL(a Asset) string {
	q := url.Values{}
	q.Set("asset_code", a.Code)
	q.Set("asset_issuer", a.Issuer)
	return horizon.DefaultURL + "/assets?" + q.Encode()
}

// flagSummary renders the issuer flag set in Horizon's own vocabulary.
func flagSummary(f horizon.Flags) string {
	return fmt.Sprintf(
		"auth_required=%t auth_revocable=%t auth_immutable=%t auth_clawback_enabled=%t",
		f.AuthRequired, f.AuthRevocable, f.AuthImmutable, f.AuthClawbackEnabled,
	)
}
