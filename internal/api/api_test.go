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
	"github.com/use-assay/assay/internal/mechanics"
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

// TestMetaReportsSchemaVersion is the server half of the UI/API agreement:
// the page checks this value before it renders anything.
func TestMetaReportsSchemaVersion(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		SchemaVersion int    `json:"schema_version"`
		RouteVersion  string `json:"route_version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SchemaVersion != mechanics.ReportSchemaVersion {
		t.Errorf("schema_version = %d, want %d", body.SchemaVersion, mechanics.ReportSchemaVersion)
	}
	if body.RouteVersion != "v1" {
		t.Errorf("route_version = %q, want v1", body.RouteVersion)
	}
}

// TestDynamicResponsesAreNotStored pins the caching policy on every surface
// that can carry a verdict or operational state. The UI may be stored but must
// be revalidated, so an old page cannot silently pair with a newer API.
func TestDynamicResponsesAreNotStored(t *testing.T) {
	h := newTestServer()
	for _, tc := range []struct{ path, want string }{
		{"/api/v1/meta", "no-store"},
		{"/api/v1/scan", "no-store"},
		{"/api/v1/history?asset=USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "no-store"},
		{"/metrics", "no-store"},
		{"/healthz", "no-store"},
		{"/", "no-cache"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if got := rec.Header().Get("Cache-Control"); got != tc.want {
				t.Errorf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
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

func TestCORSHeadersOnAPIRoutes(t *testing.T) {
	h := newTestServer()

	for _, tc := range []struct {
		name   string
		path   string
		method string
		want   int
	}{
		{"GET scan CORS", "/api/v1/scan", http.MethodGet, http.StatusBadRequest},
		{"OPTIONS preflight scan", "/api/v1/scan", http.MethodOptions, http.StatusNoContent},
		{"OPTIONS preflight history", "/api/v1/history", http.MethodOptions, http.StatusNoContent},
		{"OPTIONS preflight exposure", "/api/v1/exposure", http.MethodOptions, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
			}
		})
	}
}

func TestCORSNotSetOnNonAPIRoutes(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin set on UI route: %q", got)
	}
}

func TestErrorBodyHasCode(t *testing.T) {
	h := newTestServer()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/scan", nil))
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
	if body.Code == "" {
		t.Error("error body missing code field; API contract requires a stable code")
	}
}
