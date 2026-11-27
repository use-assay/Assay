package api_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/mechanics"
)

func TestCheckLabelMatchesEngineChecks(t *testing.T) {
	src, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatalf("read ui/index.html: %v", err)
	}

	object := regexp.MustCompile(`(?s)const CHECK_LABEL = \{(.*?)\n\};`).FindSubmatch(src)
	if len(object) != 2 {
		t.Fatal("ui/index.html does not contain a recognizable CHECK_LABEL object")
	}
	keyPattern := regexp.MustCompile(`(?m)^\s*(?:'([^']+)'|([A-Za-z_$][A-Za-z0-9_$]*))\s*:`)
	labels := make(map[string]struct{})
	for _, match := range keyPattern.FindAllSubmatch(object[1], -1) {
		key := match[1]
		if len(key) == 0 {
			key = match[2]
		}
		labels[string(key)] = struct{}{}
	}

	// TrustlineCheck runs only for holder scans and is intentionally absent from
	// mechanics.NewEngine's issuer-only default check set.
	holderOnlyChecks := []string{"trustline"}
	expected := append(mechanics.NewEngine().CheckIDs(), holderOnlyChecks...)
	checks := make(map[string]struct{}, len(expected))
	for _, id := range expected {
		checks[id] = struct{}{}
	}

	var missingLabels, staleLabels []string
	for id := range checks {
		if _, ok := labels[id]; !ok {
			missingLabels = append(missingLabels, id)
		}
	}
	for id := range labels {
		if _, ok := checks[id]; !ok {
			staleLabels = append(staleLabels, id)
		}
	}
	sort.Strings(missingLabels)
	sort.Strings(staleLabels)
	if len(missingLabels) > 0 {
		t.Errorf("CHECK_LABEL is missing engine check IDs: %s", strings.Join(missingLabels, ", "))
	}
	if len(staleLabels) > 0 {
		t.Errorf("CHECK_LABEL contains IDs with no engine check: %s", strings.Join(staleLabels, ", "))
	}
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
