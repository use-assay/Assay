package scan_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/scan"
)

// TestSubjectSkipsSACAdminWithoutRPC pins the pubnet default: no RPC endpoint
// is configured, so the SAC admin is not fetched at all and the subject records
// that it was skipped. "Skipped" must stay distinct from "failed" and from "the
// admin is the issuer".
func TestSubjectSkipsSACAdminWithoutRPC(t *testing.T) {
	fs := newFakeSources(t)
	sc := scan.New()
	sc.Horizon.BaseURL = fs.horizon.URL
	sc.Expert.BaseURL = fs.expert.URL
	sc.Toml.HTTP.Transport = &singleHostTransport{host: strings.TrimPrefix(fs.toml.URL, "http://")}
	sc.SACAdminRPC = ""

	sub, err := sc.Subject(context.Background(), mustParse(t, "USDC-"+scanIssuer))
	if err != nil {
		t.Fatalf("Subject: %v", err)
	}
	if sub.SACAdmin != "" {
		t.Errorf("SAC admin was read without an RPC endpoint: %q", sub.SACAdmin)
	}
	if !strings.Contains(sub.SACAdminSkipped, "no Soroban RPC endpoint") {
		t.Errorf("SACAdminSkipped = %q, want an explanation of the skip", sub.SACAdminSkipped)
	}
	if sub.SACAdminErr != "" {
		t.Errorf("a skipped lookup must not be recorded as a failure: %q", sub.SACAdminErr)
	}
	if sub.SACAddress == "" {
		t.Error("the SAC address should be derivable even when the admin is not read")
	}
}

// TestSubjectRecordsSACAdminFailure: an RPC failure is best-effort. The scan
// must still succeed, the failure must be recorded verbatim, and the admin must
// stay unset rather than defaulting to the issuer.
func TestSubjectRecordsSACAdminFailure(t *testing.T) {
	fs := newFakeSources(t)
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}`))
	}))
	defer rpc.Close()

	sc := scan.New()
	sc.Horizon.BaseURL = fs.horizon.URL
	sc.Expert.BaseURL = fs.expert.URL
	sc.Toml.HTTP.Transport = &singleHostTransport{host: strings.TrimPrefix(fs.toml.URL, "http://")}
	sc.SACAdminRPC = rpc.URL

	sub, err := sc.Subject(context.Background(), mustParse(t, "USDC-"+scanIssuer))
	if err != nil {
		t.Fatalf("an RPC failure must not fail the scan: %v", err)
	}
	if sub.SACAdmin != "" {
		t.Errorf("SAC admin set despite a failed read: %q", sub.SACAdmin)
	}
	if !strings.Contains(sub.SACAdminErr, "boom") {
		t.Errorf("SACAdminErr = %q, want the RPC failure", sub.SACAdminErr)
	}
	if sub.SACAdminSkipped != "" {
		t.Errorf("a failed fetch must not be recorded as skipped: %q", sub.SACAdminSkipped)
	}
	if sub.SACAdminAttemptedAt.IsZero() {
		t.Error("a failed fetch still has an attempt time")
	}
}

// TestSubjectReadsSACAdmin: with an RPC endpoint that answers, the scanner puts
// the admin on the subject and the capability finding cites it when it differs
// from the issuer. The entry here is the live testnet issuer-admin payload; its
// admin is an account, which differs from this fixture's USDC issuer, so the
// moved-admin path is exercised end to end.
func TestSubjectReadsSACAdmin(t *testing.T) {
	fs := newFakeSources(t)
	entry := strings.TrimSpace(string(mustReadFile(t, filepath.Join("..", "sacadmin", "testdata", "issuer-admin.b64"))))

	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Keys []string `json:"keys"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"entries": []map[string]string{{"key": req.Params.Keys[0], "xdr": entry}},
			},
		})
	}))
	defer rpc.Close()

	sc := scan.New()
	sc.Horizon.BaseURL = fs.horizon.URL
	sc.Expert.BaseURL = fs.expert.URL
	sc.Toml.HTTP.Transport = &singleHostTransport{host: strings.TrimPrefix(fs.toml.URL, "http://")}
	sc.SACAdminRPC = rpc.URL

	rep, sub, err := sc.ScanWithSubject(context.Background(), mustParse(t, "USDC-"+scanIssuer))
	if err != nil {
		t.Fatalf("ScanWithSubject: %v", err)
	}
	if sub.SACAdmin != "GAIFQB42YWW6IRWMT7WFFZUB22HJ2SMBZ6SDMVEXALGE6XK5GGB3PPHJ" {
		t.Fatalf("SACAdmin = %q, want the fixture's admin", sub.SACAdmin)
	}
	if sub.SACAddress == "" || sub.SACAdminURL != rpc.URL {
		t.Errorf("admin read is not attributed: sac=%q url=%q", sub.SACAddress, sub.SACAdminURL)
	}

	var found bool
	for _, f := range rep.Findings {
		if f.Check != "capability" {
			continue
		}
		if !strings.Contains(f.Reasoning, "administered by") {
			t.Errorf("capability reasoning does not report the moved admin: %s", f.Reasoning)
		}
		for _, e := range f.Evidence {
			if e.Source == "soroban-rpc/sac-admin" {
				found = true
			}
		}
	}
	if !found {
		t.Error("capability finding carries no SAC admin evidence")
	}
	if rep.Base != rep.Severity {
		t.Errorf("SAC admin changed severity: base=%v final=%v", rep.Base, rep.Severity)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
