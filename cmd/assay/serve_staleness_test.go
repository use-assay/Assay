package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/api"
	"github.com/use-assay/assay/internal/attester"
	"github.com/use-assay/assay/internal/registry"
	"github.com/use-assay/assay/internal/sac"
)

// stubSafetyReader answers GetSafety from a map keyed by SAC address, so the
// serve wiring is exercised end to end without a Soroban RPC.
type stubSafetyReader struct {
	rpcURL, contractID string
	records            map[string]*registry.Safety
}

func (s *stubSafetyReader) GetSafety(_ context.Context, sacAddr string) (*registry.Safety, error) {
	if r, ok := s.records[sacAddr]; ok {
		return r, nil
	}
	return nil, registry.ErrNotAttested
}

const (
	stalenessContract = "CBK4FBIHMDTXCUPE4E3ZDVSFJSCY5FJETTKNIQPN4LFJIKKIBLKIXQ73"
	stalenessFresh    = "USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
	stalenessStale    = "AQUA-GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA"
	stalenessMissing  = "DOGE-GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P"
)

func writeAttested(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "attested.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func attestedJSON(cadence string) string {
	c := ""
	if cadence != "" {
		c = `"cadence": "` + cadence + `",`
	}
	return `{"network": "testnet", "contract_id": "` + stalenessContract + `", ` + c + `
	  "assets": [{"asset": "` + stalenessFresh + `"}, {"asset": "` + stalenessStale + `"}, {"asset": "` + stalenessMissing + `"}]}`
}

func sacFor(t *testing.T, asset string) string {
	t.Helper()
	code, issuer, _ := strings.Cut(asset, "-")
	addr, err := sac.Address(code, issuer, sac.Passphrase("testnet"))
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

// stubReaders swaps the production registry client for a stub for the
// duration of the test and returns the stub so its wiring can be inspected.
func stubReaders(t *testing.T, records map[string]*registry.Safety) *stubSafetyReader {
	t.Helper()
	stub := &stubSafetyReader{records: records}
	prev := newStalenessReader
	newStalenessReader = func(rpcURL, contractID string) api.RegistrySafetyReader {
		stub.rpcURL, stub.contractID = rpcURL, contractID
		return stub
	}
	t.Cleanup(func() { newStalenessReader = prev })
	return stub
}

type readyzRegistry struct {
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	WindowSecs      int64  `json:"window_secs"`
	AssetCount      int    `json:"asset_count"`
	AttestedCount   int    `json:"attested_count"`
	UnattestedCount int    `json:"unattested_count"`
	StaleCount      int    `json:"stale_count"`
}

// readyzThroughServe builds the server as runServe does — the prober set
// before Handler() — and reads the registry block from GET /readyz.
func readyzThroughServe(t *testing.T, p *api.RegistryStalenessProber) readyzRegistry {
	t.Helper()
	srv := api.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.RegistryStaleness = p
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var body struct {
		Registry readyzRegistry `json:"registry_staleness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /readyz: %v\n%s", err, rec.Body)
	}
	return body.Registry
}

func TestServeRegistryStalenessReportsRealCounts(t *testing.T) {
	now := time.Now().UTC()
	stub := stubReaders(t, nil)
	stub.records = map[string]*registry.Safety{
		sacFor(t, stalenessFresh): {AttestedAt: uint64(now.Add(-time.Hour).Unix())},
		sacFor(t, stalenessStale): {AttestedAt: uint64(now.Add(-72 * time.Hour).Unix())},
	}

	p := registryStalenessProber(writeAttested(t, attestedJSON("24h")), "https://rpc.example", 0)
	if p == nil || p.ConfigurationError != "" {
		t.Fatalf("prober = %+v, want a configured prober", p)
	}
	if stub.rpcURL != "https://rpc.example" || stub.contractID != stalenessContract {
		t.Errorf("reader built for %q / %q, want the flag's RPC and the file's contract", stub.rpcURL, stub.contractID)
	}

	got := readyzThroughServe(t, p)
	want := readyzRegistry{Status: "stale", WindowSecs: 24 * 3600, AssetCount: 3,
		AttestedCount: 2, UnattestedCount: 1, StaleCount: 1}
	if got != want {
		t.Errorf("/readyz registry_staleness = %+v, want %+v", got, want)
	}
}

func TestServeRegistryStalenessWindowFlagOverridesCadence(t *testing.T) {
	now := time.Now().UTC()
	stubReaders(t, map[string]*registry.Safety{
		sacFor(t, stalenessFresh):   {AttestedAt: uint64(now.Add(-time.Hour).Unix())},
		sacFor(t, stalenessStale):   {AttestedAt: uint64(now.Add(-72 * time.Hour).Unix())},
		sacFor(t, stalenessMissing): {AttestedAt: uint64(now.Add(-72 * time.Hour).Unix())},
	})
	p := registryStalenessProber(writeAttested(t, attestedJSON("24h")), "https://rpc.example", 7*24*time.Hour)
	got := readyzThroughServe(t, p)
	if got.Status != "current" || got.WindowSecs != 7*24*3600 || got.StaleCount != 0 || got.AttestedCount != 3 {
		t.Errorf("/readyz registry_staleness = %+v, want current with a 7d window", got)
	}
}

func TestServeRegistryStalenessUnconfiguredIsUnchanged(t *testing.T) {
	if p := registryStalenessProber("", "https://rpc.example", time.Hour); p != nil {
		t.Fatalf("no -attested gave a prober: %+v", p)
	}
	got := readyzThroughServe(t, nil)
	if got.Status != "unknown" || got.Reason != "registry staleness is not configured" {
		t.Errorf("/readyz registry_staleness = %+v, want the unconfigured answer", got)
	}
}

func TestServeRegistryStalenessBadConfigIsUnknownWithReason(t *testing.T) {
	stubReaders(t, nil)
	cases := []struct {
		name, path string
		window     time.Duration
		reason     string
	}{
		{"missing file", filepath.Join(t.TempDir(), "absent.json"), 0, "read"},
		{"malformed json", writeAttested(t, `{"network": "testnet",`), 0, "unexpected EOF"},
		{"invalid contract", writeAttested(t, `{"network": "testnet", "contract_id": "nope", "assets": [{"asset": "`+stalenessFresh+`"}]}`), 0, "contract_id"},
		{"no cadence and no window", writeAttested(t, attestedJSON("")), 0, "no cadence"},
		{"negative window", writeAttested(t, attestedJSON("24h")), -time.Hour, "negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := registryStalenessProber(tc.path, "https://rpc.example", tc.window)
			if p == nil || p.ConfigurationError == "" {
				t.Fatalf("bad config gave %+v, want a ConfigurationError", p)
			}
			got := readyzThroughServe(t, p)
			if got.Status != "unknown" {
				t.Errorf("status = %q, want unknown", got.Status)
			}
			if !strings.Contains(got.Reason, tc.reason) || got.Reason != p.ConfigurationError {
				t.Errorf("reason = %q, want the parse error (containing %q)", got.Reason, tc.reason)
			}
		})
	}
}

// The repository's own attested.json must wire without a configuration error,
// so `assay serve -attested attested.json` works as documented.
func TestServeRegistryStalenessAcceptsRepoAttestedJSON(t *testing.T) {
	stubReaders(t, nil)
	path := filepath.Join("..", "..", "attested.json")
	cfg, err := attester.LoadConfig(path)
	if err != nil {
		t.Fatalf("load repo attested.json: %v", err)
	}
	p := registryStalenessProber(path, registry.DefaultRPCURL, 0)
	if p == nil || p.ConfigurationError != "" {
		t.Fatalf("repo attested.json gave %+v", p)
	}
	if len(p.Assets) != len(cfg.Assets) || p.Window != 24*time.Hour {
		t.Errorf("prober has %d assets and window %s, want %d and 24h", len(p.Assets), p.Window, len(cfg.Assets))
	}
}
