package sacadmin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two payloads below were read live from Soroban RPC testnet on 2026-11-23
// with getLedgerEntries. They are the reproduction the issue asks for:
//
//   - issuer-admin: asset ASSAYPRB-GAIFQB42…, SAC
//     CAQRN7WMPPS2VBQQVIUQXGRVYIBXPLT65KIQAI52FJCFFZQ6SG4PP7R4, deployed by its
//     issuer in transaction
//     ce28eb7dbad21005faff17f39288479c6602d4c3e88f5f385ed1facfb7cbc5e1.
//     `stellar contract invoke --id CAQRN… -- admin` returned the issuer.
//   - moved-admin: asset zUSDC-GAA3S6XLOKFX3SGDQ3VGLLXMCFMVB7E6WYNGHRGIRD62AEJ73ASPQ4KX,
//     SAC CAIUOHVZ77RSCDBNWR3BCZPTWHPUXQRTQXSW4VE3HGC2M5PRPJNSFBRU. Its Admin is a
//     contract, CDFQ5K2BPKB7BWNW2SJPGIIK5OOFQIR434MOX5YYBDKAN3M5CFVJKHR2, not
//     the issuer.
const (
	issuerAdminSAC = "CAQRN7WMPPS2VBQQVIUQXGRVYIBXPLT65KIQAI52FJCFFZQ6SG4PP7R4"
	movedAdminSAC  = "CAIUOHVZ77RSCDBNWR3BCZPTWHPUXQRTQXSW4VE3HGC2M5PRPJNSFBRU"

	issuerAdminIssuer = "GAIFQB42YWW6IRWMT7WFFZUB22HJ2SMBZ6SDMVEXALGE6XK5GGB3PPHJ"
	movedAdminAdmin   = "CDFQ5K2BPKB7BWNW2SJPGIIK5OOFQIR434MOX5YYBDKAN3M5CFVJKHR2"

	issuerAdminKey = "AAAABgAAAAEhFv7Me+WqhhCqKQuaNcIDd65+6pEAI7oqRFLmHpG49wAAABQAAAAB"
	movedAdminKey  = "AAAABgAAAAERRx65/+MhDC20dhFl87HfS8IzheVuVJs5haZ18XpbIgAAABQAAAAB"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return raw
}

// TestParseInstanceAdminFromLiveTestnet is the protocol check: the hand-written
// XDR reader must find the same admin the Stellar CLI's `admin` invocation
// returned for the issuer-admin case, and the contract address for the
// moved-admin case.
func TestParseInstanceAdminFromLiveTestnet(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		want    string
	}{
		{"issuer-admin", "issuer-admin.b64", issuerAdminIssuer},
		{"moved-admin", "moved-admin.b64", movedAdminAdmin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseInstanceAdmin(readFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("ParseInstanceAdmin: %v", err)
			}
			if got != tc.want {
				t.Errorf("admin = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLedgerKeyForSAC(t *testing.T) {
	cases := []struct{ sac, want string }{
		{issuerAdminSAC, issuerAdminKey},
		{movedAdminSAC, movedAdminKey},
	}
	for _, tc := range cases {
		got, err := LedgerKeyForSAC(tc.sac)
		if err != nil {
			t.Fatalf("LedgerKeyForSAC(%s): %v", tc.sac, err)
		}
		if got != tc.want {
			t.Errorf("key for %s = %q, want %q", tc.sac, got, tc.want)
		}
	}
}

func TestLedgerKeyForSACRejectsMalformedAddress(t *testing.T) {
	cases := []string{
		"",
		"G" + strings.Repeat("A", 55), // account, not contract
		"CAQRN7WMPPS2VBQQVIUQXGRVYIBXPLT65KIQAI52FJCFFZQ6SG4PP7R0",      // bad checksum
		"CAQRN7WMPPS2VBQQVIUQXGRVYIBXPLT65KIQAI52FJCFFZQ6SG4PP7R4EXTRA", // wrong length
	}
	for _, sac := range cases {
		if _, err := LedgerKeyForSAC(sac); !errors.Is(err, ErrBadContractID) {
			t.Errorf("LedgerKeyForSAC(%q) error = %v, want ErrBadContractID", sac, err)
		}
	}
}

func TestParseInstanceAdminRefusesUnreadablePayloads(t *testing.T) {
	full := readFixture(t, "issuer-admin.b64")

	t.Run("not contract data", func(t *testing.T) {
		_, err := ParseInstanceAdmin([]byte{0, 0, 0, 0})
		if !errors.Is(err, ErrNotContractData) {
			t.Errorf("error = %v, want ErrNotContractData", err)
		}
	})

	t.Run("truncated", func(t *testing.T) {
		_, err := ParseInstanceAdmin(full[:20])
		if !errors.Is(err, ErrTruncated) {
			t.Errorf("error = %v, want ErrTruncated", err)
		}
	})

	t.Run("no admin entry", func(t *testing.T) {
		// A storage map with zero entries is readable and legitimately has no
		// admin. The map count sits at offset 64 in this layout:
		//   0 data type, 4 ext, 8 address type, 12..44 contract id,
		//   44 instance key, 48 durability, 52 SCV_CONTRACT_INSTANCE,
		//   56 executable type, 60 storage-pointer present, 64 map count.
		const mapCountOffset = 64
		b := append([]byte(nil), full[:mapCountOffset+4]...)
		copy(b[mapCountOffset:mapCountOffset+4], []byte{0, 0, 0, 0})
		if _, err := ParseInstanceAdmin(b); !errors.Is(err, ErrNoAdmin) {
			t.Errorf("error = %v, want ErrNoAdmin", err)
		}
	})

	t.Run("unknown SCVal tag", func(t *testing.T) {
		// Corrupt the instance value tag at offset 52.
		b := append([]byte(nil), full...)
		copy(b[52:56], []byte{0, 0, 0, 99})
		_, err := ParseInstanceAdmin(b)
		if !errors.Is(err, ErrUnknownXDR) {
			t.Errorf("error = %v, want ErrUnknownXDR", err)
		}
	})
}

func TestStrkeyRoundTrip(t *testing.T) {
	for _, sac := range []string{issuerAdminSAC, movedAdminSAC} {
		raw, err := decodeContractStrKey(sac)
		if err != nil {
			t.Fatalf("decode %s: %v", sac, err)
		}
		if got := encodeStrKey(strKeyVersionContract, raw); got != sac {
			t.Errorf("round trip = %q, want %q", got, sac)
		}
	}
}

// newRPCServer serves a getLedgerEntries response for wantKey with the given
// entry XDR, or a JSON-RPC error when fail is true.
func newRPCServer(t *testing.T, wantKey, entryXDR string, fail bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Keys []string `json:"keys"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Params.Keys) != 1 || req.Params.Keys[0] != wantKey {
			t.Errorf("requested keys %v, want [%s]", req.Params.Keys, wantKey)
		}
		w.Header().Set("Content-Type", "application/json")
		if fail {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"entries": []map[string]string{{"key": wantKey, "xdr": entryXDR}},
			},
		})
	}))
}

func TestClientAdmin(t *testing.T) {
	entry := strings.TrimSpace(string(mustRead(t, "testdata/issuer-admin.b64")))
	srv := newRPCServer(t, issuerAdminKey, entry, false)
	defer srv.Close()

	got, err := New(srv.URL).Admin(context.Background(), issuerAdminSAC)
	if err != nil {
		t.Fatalf("Admin: %v", err)
	}
	if got != issuerAdminIssuer {
		t.Errorf("admin = %q, want %q", got, issuerAdminIssuer)
	}
}

func TestClientAdminReportsRPCErrorAndAbsence(t *testing.T) {
	t.Run("rpc error", func(t *testing.T) {
		srv := newRPCServer(t, issuerAdminKey, "", true)
		defer srv.Close()
		if _, err := New(srv.URL).Admin(context.Background(), issuerAdminSAC); !errors.Is(err, ErrRPC) {
			t.Errorf("error = %v, want ErrRPC", err)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"entries":[]}}`))
		}))
		defer srv.Close()
		if _, err := New(srv.URL).Admin(context.Background(), issuerAdminSAC); !errors.Is(err, ErrEntryAbsent) {
			t.Errorf("error = %v, want ErrEntryAbsent", err)
		}
	})
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
