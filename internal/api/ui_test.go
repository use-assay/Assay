package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/api"
	"github.com/use-assay/assay/internal/mechanics"
)

var expectedSchemaVersionRE = regexp.MustCompile(`const EXPECTED_SCHEMA_VERSION = ([0-9]+);`)

func readUISource(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatalf("read ui/index.html: %v", err)
	}
	return string(src)
}

func expectedSchemaVersion(t *testing.T, html string) int {
	t.Helper()
	m := expectedSchemaVersionRE.FindStringSubmatch(html)
	if m == nil {
		t.Fatal("ui/index.html does not declare EXPECTED_SCHEMA_VERSION")
	}
	got, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("parse EXPECTED_SCHEMA_VERSION: %v", err)
	}
	return got
}

// TestUIIndexBuildsDOMWithoutInnerHTML enforces the invariant issue #418
// established: issuer-controlled strings (directory name, domain, toml claims)
// reach the page only through textContent, never through an HTML-parsing sink.
// A regression here is a stored XSS on any page that renders a scan, so it
// fails the build rather than waiting for a manual review.
//
// This is a static check over the single-file UI rather than a headless render
// because the page is the whole surface: with no sink present, hostile input
// can only arrive as text.
func TestUIIndexBuildsDOMWithoutInnerHTML(t *testing.T) {
	src, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatalf("read ui/index.html: %v", err)
	}
	html := string(src)

	for _, sink := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
		if strings.Contains(html, sink) {
			t.Errorf("ui/index.html uses %s; build nodes with textContent so issuer-controlled strings cannot be parsed as markup", sink)
		}
	}

	// The guard is only meaningful if the page renders dynamic text at all;
	// assert the safe sink is the one actually in use.
	if !strings.Contains(html, "textContent") {
		t.Error("ui/index.html has no textContent assignment: the DOM-construction path this test guards is missing")
	}
}

// TestUIIndexRendersUndeterminedAsFloor pins the rendering contract for a
// degraded scan: it must be visually distinct from a clean one, name the check
// that could not complete, and state that the severity is a floor rather than
// an answer. A clean-looking result for an unchecked asset is the failure this
// test exists to prevent.
//
// The page has no browser harness, so — like the DOM-sink test above — this is
// a static assertion over the shipped file. It checks the pieces the render
// path is built from; removing any one of them breaks the contract. The
// end-to-end rendering is verified manually against a degraded scan, with the
// procedure recorded in the pull request.
func TestUIIndexRendersUndeterminedAsFloor(t *testing.T) {
	src, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatalf("read ui/index.html: %v", err)
	}
	html := string(src)

	for _, want := range []struct{ name, needle string }{
		{"decides the state from the report and the backward-compatible header",
			"const degraded = isUndetermined(rep, undeterminedHeader);"},
		{"reads the completeness flag", "rep.undetermined === true"},
		{"reads the checks that did not complete", "rep.undetermined_checks"},
		{"handles report shapes without the field", "Array.isArray(raw)"},
		{"renders the undetermined state in its own panel", "el('section', 'degraded')"},
		{"names each undetermined check", "checkLabel(id)"},
		{"falls back when no check is named", "did not name the check that failed"},
		{"states that severity is a floor", "The severity shown is a floor, not an answer"},
		{"marks the severity card as a floor", "sevCard.classList.add('floor')"},
	} {
		if !strings.Contains(html, want.needle) {
			t.Errorf("ui/index.html no longer %s (missing %q); a degraded scan must not render like a complete one",
				want.name, want.needle)
		}
	}

	// The banner used to describe the transport header instead of the meaning of
	// the result. That wording let "we could not check" read as "nothing found".
	for _, gone := range []string{
		"Findings may be incomplete.",
		"This is signaled via the X-Assay-Undetermined header.",
	} {
		if strings.Contains(html, gone) {
			t.Errorf("ui/index.html still carries the old undetermined wording %q; it must name the check and call the severity a floor", gone)
		}
	}

}

func TestUIIndexRendersNetworkAndClockSkew(t *testing.T) {
	src, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatalf("read ui/index.html: %v", err)
	}
	html := string(src)
	for _, needle := range []string{
		"networkPanel(rep.network)",
		"Network read",
		"clockSkewPanel(rep.clock_skews)",
		"Upstream clock skew detected.",
		"Observation timestamps were not changed.",
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("ui/index.html does not render %q", needle)
		}
	}
}

// TestUIIndexPinsReportSchemaVersion pins the two halves of the UI/API version
// agreement: the page states the report schema it can render, and it refuses
// to render a body or talk to a server that speaks a different one. The
// constant is compared to the Go constant so a schema bump cannot leave the
// page silently behind.
func TestUIIndexPinsReportSchemaVersion(t *testing.T) {
	html := readUISource(t)
	got := expectedSchemaVersion(t, html)
	if got != mechanics.ReportSchemaVersion {
		t.Errorf("UI EXPECTED_SCHEMA_VERSION = %d, want mechanics.ReportSchemaVersion = %d",
			got, mechanics.ReportSchemaVersion)
	}

	for _, want := range []struct{ name, needle string }{
		{"asks the server which schema it speaks before rendering", "/api/v1/meta"},
		{"checks the metadata response", "meta.schema_version !== EXPECTED_SCHEMA_VERSION"},
		{"checks every scan body before rendering it", "body.schema_version !== EXPECTED_SCHEMA_VERSION"},
		{"stops the form on a mismatch", "input.disabled = true"},
		{"offers a reload", "window.location.reload()"},
		{"says the page is out of date", "This page is out of date."},
	} {
		if !strings.Contains(html, want.needle) {
			t.Errorf("ui/index.html no longer %s (missing %q)", want.name, want.needle)
		}
	}
}

// setExpectedSchemaVersion returns a copy of the page whose pinned schema
// version is replaced. It is how the mismatch tests below serve a stale page
// without checking out an older build: the only difference is the value the
// page says it can render.
func setExpectedSchemaVersion(t *testing.T, html string, version int) string {
	t.Helper()
	current := expectedSchemaVersion(t, html)
	if current == version {
		return html
	}
	old := fmt.Sprintf("const EXPECTED_SCHEMA_VERSION = %d;", current)
	new := fmt.Sprintf("const EXPECTED_SCHEMA_VERSION = %d;", version)
	out := strings.Replace(html, old, new, 1)
	if out == html {
		t.Fatalf("could not replace %q", old)
	}
	return out
}

// TestUISchemaMismatchServed exercises the acceptance criterion "tested by
// serving a modified version" without adding a browser to the test suite. Each
// subtest serves a page and API responses with deliberately different schema
// versions, fetches them over HTTP, and checks both sides of the contract:
// the versions really disagree, and the page served contains the branch that
// reports the mismatch and offers a reload.
func TestUISchemaMismatchServed(t *testing.T) {
	src := readUISource(t)
	shipped := expectedSchemaVersion(t, src)
	other := shipped + 1

	for _, tc := range []struct {
		name        string
		pageVersion int
		apiVersion  int
	}{
		{"stale page against current API", other, shipped},
		{"current page against future API", shipped, other},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := setExpectedSchemaVersion(t, src, tc.pageVersion)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = io.WriteString(w, page)
				case "/api/v1/meta":
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					_ = json.NewEncoder(w).Encode(api.Meta{SchemaVersion: tc.apiVersion, RouteVersion: "v1"})
				case "/api/v1/scan":
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					_ = json.NewEncoder(w).Encode(struct {
						SchemaVersion int `json:"schema_version"`
					}{SchemaVersion: tc.apiVersion})
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			get := func(path string) []byte {
				t.Helper()
				resp, err := http.Get(srv.URL + path)
				if err != nil {
					t.Fatalf("GET %s: %v", path, err)
				}
				defer func() { _ = resp.Body.Close() }()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				return body
			}

			servedPage := string(get("/"))
			if got := expectedSchemaVersion(t, servedPage); got != tc.pageVersion {
				t.Fatalf("served page EXPECTED_SCHEMA_VERSION = %d, want %d", got, tc.pageVersion)
			}

			var meta api.Meta
			if err := json.Unmarshal(get("/api/v1/meta"), &meta); err != nil {
				t.Fatalf("decode meta: %v", err)
			}
			if meta.SchemaVersion != tc.apiVersion {
				t.Fatalf("served meta schema_version = %d, want %d", meta.SchemaVersion, tc.apiVersion)
			}

			var scan struct {
				SchemaVersion int `json:"schema_version"`
			}
			if err := json.Unmarshal(get("/api/v1/scan"), &scan); err != nil {
				t.Fatalf("decode scan: %v", err)
			}
			if scan.SchemaVersion != tc.apiVersion {
				t.Fatalf("served scan schema_version = %d, want %d", scan.SchemaVersion, tc.apiVersion)
			}

			if meta.SchemaVersion == tc.pageVersion || scan.SchemaVersion == tc.pageVersion {
				t.Fatal("test setup did not create a schema mismatch")
			}
			for _, needle := range []string{
				"meta.schema_version !== EXPECTED_SCHEMA_VERSION",
				"body.schema_version !== EXPECTED_SCHEMA_VERSION",
				"This page is out of date.",
				"window.location.reload()",
				"input.disabled = true",
			} {
				if !strings.Contains(servedPage, needle) {
					t.Errorf("served page does not handle the mismatch (missing %q)", needle)
				}
			}
		})
	}
}
