package scan_test

import (
	"errors"
	"testing"

	"github.com/use-assay/assay/internal/scan"
)

func TestParseAsset(t *testing.T) {
	const issuer = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"

	t.Run("valid", func(t *testing.T) {
		for _, in := range []string{
			"USDC-" + issuer,
			"  USDC-" + issuer + "  ",
			// StellarExpert renders assets with a trailing sequence suffix.
			"USDC-" + issuer + "-1",
		} {
			a, err := scan.ParseAsset(in)
			if err != nil {
				t.Fatalf("ParseAsset(%q): %v", in, err)
			}
			if a.Code != "USDC" || a.Issuer != issuer {
				t.Errorf("ParseAsset(%q) = %+v", in, a)
			}
		}
	})

	t.Run("rejected", func(t *testing.T) {
		for _, in := range []string{
			"",
			"USDC",
			"-" + issuer,
			"USDC-NOTAKEY",
			// Lowercase is outside the base32 alphabet Stellar keys use, so
			// accepting it would let a lookalike issuer through.
			"USDC-" + "ga5zsejyb37jrc5avcia5mop4rhtm335x2kgx3ihojapp5re34k4kzvn",
			"TOOLONGASSETCODE-" + issuer,
			"US DC-" + issuer,
		} {
			if _, err := scan.ParseAsset(in); !errors.Is(err, scan.ErrBadAsset) {
				t.Errorf("ParseAsset(%q) error = %v, want ErrBadAsset", in, err)
			}
		}
	})
}
