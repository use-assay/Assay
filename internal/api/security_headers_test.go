package api_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/api"
	"github.com/use-assay/assay/internal/history"
)

// TestUIContentSecurityPolicy verifies issue #584: the embedded UI is served
// with a strict Content-Security-Policy matching the exact SHA-256 hashes of
// its single inline script and style elements.
func TestUIContentSecurityPolicy(t *testing.T) {
	srv := api.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := srv.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returned status %d, want 200", rec.Code)
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("GET / response is missing Content-Security-Policy header")
	}

	src, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatalf("read ui/index.html: %v", err)
	}

	scriptHash, styleHash, err := api.ExtractInlineHashes(string(src))
	if err != nil {
		t.Fatalf("ExtractInlineHashes: %v", err)
	}

	wantScriptDirective := fmt.Sprintf("'sha256-%s'", scriptHash)
	if !strings.Contains(csp, wantScriptDirective) {
		t.Errorf("CSP missing script hash directive %s; got:\n%s", wantScriptDirective, csp)
	}

	wantStyleDirective := fmt.Sprintf("'sha256-%s'", styleHash)
	if !strings.Contains(csp, wantStyleDirective) {
		t.Errorf("CSP missing style hash directive %s; got:\n%s", wantStyleDirective, csp)
	}

	// Verify all required CSP directives per issue #584.
	for _, directive := range []string{
		"default-src 'none'",
		"connect-src 'self'",
		"img-src 'self'",
		"base-uri 'none'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP missing directive %q; got:\n%s", directive, csp)
		}
	}

	if csp != api.UICSP {
		t.Errorf("GET / CSP header does not match api.UICSP constant:\ngot:  %s\nwant: %s", csp, api.UICSP)
	}
}

// TestUIContentSecurityPolicyFailsOnScriptChange proves acceptance criterion:
// "Test fails if the inline script changes without the hash changing."
func TestUIContentSecurityPolicyFailsOnScriptChange(t *testing.T) {
	src, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatalf("read ui/index.html: %v", err)
	}

	// Simulate an edit to the inline script without updating the policy.
	modified := strings.Replace(string(src), "</script>", "\n// modified\n</script>", 1)
	newScriptHash, _, err := api.ExtractInlineHashes(modified)
	if err != nil {
		t.Fatalf("ExtractInlineHashes on modified html: %v", err)
	}

	if newScriptHash == api.UIScriptSHA256 {
		t.Fatal("modifying script content failed to change the computed hash")
	}

	if strings.Contains(api.UICSP, newScriptHash) {
		t.Errorf("api.UICSP unexpectedly contains the modified script hash %s", newScriptHash)
	}
}

// TestSecurityHeadersOnAllResponses verifies acceptance criteria:
// "All responses carry nosniff."
// "set X-Content-Type-Options: nosniff, Referrer-Policy: no-referrer and frame-ancestors 'none' on every response."
func TestSecurityHeadersOnAllResponses(t *testing.T) {
	srv := api.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.History = history.New()
	handler := srv.Handler()

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/"},
		{http.MethodGet, "/api/v1/scan?asset=native"},
		{http.MethodGet, "/api/v1/history?asset=native"},
		{http.MethodGet, "/api/v1/exposure"},
		{http.MethodGet, "/metrics"},
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/api/v1/unknown-endpoint"},
		{http.MethodGet, "/nonexistent-page"},
		{http.MethodOptions, "/api/v1/scan"},
	}

	for _, tc := range paths {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			handler.ServeHTTP(rec, req)

			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("%s %s missing X-Content-Type-Options: nosniff; got %q", tc.method, tc.path, got)
			}

			if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
				t.Errorf("%s %s missing Referrer-Policy: no-referrer; got %q", tc.method, tc.path, got)
			}

			csp := rec.Header().Get("Content-Security-Policy")
			if !strings.Contains(csp, "frame-ancestors 'none'") {
				t.Errorf("%s %s missing frame-ancestors 'none' in Content-Security-Policy; got %q", tc.method, tc.path, csp)
			}
		})
	}
}
