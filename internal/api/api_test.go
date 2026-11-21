package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/api"
)

func newTestServer() http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.NewServer(log).Handler()
}

// TestScanRejectsBadInput covers the paths that fail before any network call,
// so the test stays hermetic.
func TestScanRejectsBadInput(t *testing.T) {
	h := newTestServer()

	for _, tc := range []struct {
		name  string
		query string
		code  string
	}{
		{"missing asset", "/api/v1/scan", "missing_param"},
		{"empty asset", "/api/v1/scan?asset=", "missing_param"},
		{"not an asset", "/api/v1/scan?asset=hello", "invalid_asset"},
		{"bad issuer", "/api/v1/scan?asset=USDC-NOPE", "invalid_asset"},
		{"invalid max_age_secs", "/api/v1/scan?asset=USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&max_age_secs=notanumber", "invalid_param"},
		{"negative max_age_secs", "/api/v1/scan?asset=USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&max_age_secs=-10", "invalid_param"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.query, nil))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			var body struct {
				Error string `json:"error"`
				Code  string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error == "" {
				t.Error("error body was empty; a caller must be able to tell why it failed")
			}
			if body.Code != tc.code {
				t.Errorf("code = %q, want %q", body.Code, tc.code)
			}
		})
	}
}

// TestScanNativeAssetIsExplicit400 pins the documented answer for the
// network's native asset: an understood input, refused with 400 and a message
// naming the ledger fact — no issuer, no authorization flags — rather than a
// parse error. docs/api.md documents both the status and the wording.
func TestScanNativeAssetIsExplicit400(t *testing.T) {
	h := newTestServer()

	for _, query := range []string{
		"/api/v1/scan?asset=native",
		"/api/v1/scan?asset=XLM",
		"/api/v1/scan?asset=xlm",
	} {
		t.Run(query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, query, nil))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			for _, want := range []string{
				"native asset has no issuer and no authorization flags",
				"cannot be scanned or attested",
			} {
				if !strings.Contains(body.Error, want) {
					t.Errorf("error = %q, want it to say %q", body.Error, want)
				}
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestUIServed(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// The UI must present severity and accountability as separate things. If
	// someone collapses them into one score, this fails loudly.
	for _, want := range []string{"Severity — issuer capability", "Accountability — who stands behind it"} {
		if !strings.Contains(body, want) {
			t.Errorf("UI missing %q", want)
		}
	}
}

func TestUnknownPathIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestErrorBodiesHaveCode verifies that every error response includes a stable
// machine-readable code field. The test stays hermetic: every path exercised
// fails before any network call.
func TestErrorBodiesHaveCode(t *testing.T) {
	h := newTestServer()

	type errorResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}

	for _, tc := range []struct {
		name   string
		method string
		query  string
		want   string
	}{
		{"scan: missing asset", http.MethodGet, "/api/v1/scan", "missing_asset"},
		{"scan: invalid asset", http.MethodGet, "/api/v1/scan?asset=bad", "invalid_asset"},
		{"scan: invalid holder", http.MethodGet, "/api/v1/scan?asset=USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&holder=notanaccount", "invalid_holder"},
		{"scan: invalid max_age_secs", http.MethodGet, "/api/v1/scan?asset=USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&max_age_secs=bad", "invalid_parameter"},
		{"history: missing asset", http.MethodGet, "/api/v1/history", "missing_asset"},
		{"history: invalid asset", http.MethodGet, "/api/v1/history?asset=bad", "invalid_asset"},
		{"exposure: missing account", http.MethodGet, "/api/v1/exposure", "missing_account"},
		{"exposure: invalid account", http.MethodGet, "/api/v1/exposure?account=notanaccount", "invalid_account"},
		{"exposure: invalid limit", http.MethodGet, "/api/v1/exposure?account=GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H&limit=bad", "invalid_parameter"},
		{"exposure: invalid cursor", http.MethodGet, "/api/v1/exposure?account=GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H&cursor=-1", "invalid_parameter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.query, nil))

			var body errorResp
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v (body: %s)", err, rec.Body.String())
			}
			if body.Code != tc.want {
				t.Errorf("code = %q, want %q (error: %q)", body.Code, tc.want, body.Error)
			}
			if body.Error == "" {
				t.Error("error message was empty")
			}
		})
	}
}

func TestScanUndeterminedHeaderContract(t *testing.T) {
	// A scan on a nonexistent or unreachable asset returns undetermined or fails;
	// we can verify the header contract is set properly on responses.
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/scan?asset=UNKNOWN-GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA", nil))
	if rec.Code == http.StatusOK {
		if got := rec.Header().Get("X-Assay-Undetermined"); got != "true" && got != "false" {
			t.Errorf("X-Assay-Undetermined header missing or invalid: %q", got)
		}
	}
}

// TestCORSHeaderOnGET verifies that every GET response includes the
// Access-Control-Allow-Origin: * header required by issue #428.
func TestCORSHeaderOnGET(t *testing.T) {
	h := newTestServer()

	for _, path := range []string{
		"/api/v1/scan?asset=USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		"/api/v1/history?asset=USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		"/api/v1/scan",          // 400 response also carries the header
		"/healthz",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want \"*\"", got)
			}
		})
	}
}

// TestCORSPreflight verifies that an OPTIONS preflight returns 204 with the
// required CORS headers, without hitting the scan or any upstream.
func TestCORSPreflight(t *testing.T) {
	h := newTestServer()

	for _, path := range []string{
		"/api/v1/scan",
		"/api/v1/history",
		"/api/v1/exposure",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, path, nil)
			req.Header.Set("Origin", "https://example.com")
			req.Header.Set("Access-Control-Request-Method", "GET")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Errorf("preflight status = %d, want 204", rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want \"*\"", got)
			}
			if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
				t.Error("Access-Control-Allow-Methods header missing")
			}
		})
	}
}
