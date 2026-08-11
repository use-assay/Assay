package sep1_test

import (
	"testing"

	"github.com/use-assay/assay/internal/sep1"
)

const issuer = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"

func TestClaims(t *testing.T) {
	doc, err := sep1.Parse([]byte(`
[[CURRENCIES]]
code = "USDC"
issuer = "` + issuer + `"

[[CURRENCIES]]
code = "EURC"
issuer = "GDHU6WRG4IEQXM5NZ4BMPKOXHW76MZM4Y2IEMFDVXBSDP6SJY4ITNPP2"
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !doc.Claims("USDC", issuer) {
		t.Error("Claims(USDC, issuer) = false, want true")
	}
	if !doc.Claims("usdc", issuer) {
		t.Error("code matching should be case-insensitive")
	}

	// A toml claiming the right code under the wrong issuer is exactly the
	// impersonation this check exists to catch, so code alone must never match.
	if doc.Claims("USDC", "GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA") {
		t.Error("Claims matched on code alone with a different issuer")
	}
	if doc.Claims("XLM", issuer) {
		t.Error("Claims matched an unlisted code")
	}

	var nilDoc *sep1.Doc
	if nilDoc.Claims("USDC", issuer) {
		t.Error("nil doc must not claim anything")
	}
}

func TestURLFor(t *testing.T) {
	want := "https://circle.com/.well-known/stellar.toml"
	for _, in := range []string{"circle.com", "circle.com/"} {
		if got := sep1.URLFor(in); got != want {
			t.Errorf("URLFor(%q) = %q, want %q", in, got, want)
		}
	}
}
