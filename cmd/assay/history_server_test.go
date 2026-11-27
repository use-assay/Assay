package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/api"
	historystore "github.com/use-assay/assay/internal/history"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/temporal"
)

// historyServer serves /api/v1/history from an in-memory store holding obs.
func historyServer(t *testing.T, obs ...temporal.Observation) *httptest.Server {
	t.Helper()
	srv := api.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	store := historystore.New()
	for _, o := range obs {
		if err := store.Append(o); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	srv.History = store
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// A capability added and then removed between three scans must show in
// `assay history -server` as two transitions and in the summary as held at
// some point — issue #599. The last pair alone reads as a plain removal.
func TestHistoryServerReportsEveryTransition(t *testing.T) {
	asset := mechanics.Asset{Code: "USDC", Issuer: cliIssuer}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ts := historyServer(t,
		temporal.Observation{Asset: asset, At: base, Base: mechanics.Clear, Severity: mechanics.Clear},
		temporal.Observation{Asset: asset, At: base.Add(24 * time.Hour), Mechanics: mechanics.MechClawbackEnabled,
			Base: mechanics.High, Severity: mechanics.High},
		temporal.Observation{Asset: asset, At: base.Add(48 * time.Hour), Base: mechanics.Clear, Severity: mechanics.Clear},
	)

	d, out, errOut, _ := deps(nil, nil)
	if err := runWith([]string{"history", "-server", ts.URL, asset.String()}, d); err != nil {
		t.Fatalf("history -server: %v", err)
	}

	var hist []historyEntry
	if err := json.Unmarshal(out.Bytes(), &hist); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if len(hist) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(hist), hist)
	}
	wantTransitions := []string{"first", "added auth_clawback_enabled", "removed auth_clawback_enabled"}
	for i, w := range wantTransitions {
		if hist[i].Transition != w {
			t.Errorf("entry %d transition = %q, want %q", i, hist[i].Transition, w)
		}
		if hist[i].Asset != asset.String() {
			t.Errorf("entry %d asset = %q, want %q", i, hist[i].Asset, asset.String())
		}
	}
	if hist[1].Severity != "high" {
		t.Errorf("entry 1 severity = %q, want high", hist[1].Severity)
	}

	summary := errOut.String()
	for _, want := range []string{
		"capabilities held at any point: auth_clawback_enabled",
		"held now:                       none",
		"held earlier, not now:          auth_clawback_enabled",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestHistoryServerUndeterminedIsNotNoChange(t *testing.T) {
	asset := mechanics.Asset{Code: "USDC", Issuer: cliIssuer}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ts := historyServer(t,
		temporal.Observation{Asset: asset, At: base},
		temporal.Observation{Asset: asset, At: base.Add(time.Hour), Undetermined: true,
			UndeterminedChecks: []string{"reputation"}},
	)

	d, out, errOut, _ := deps(nil, nil)
	if err := runWith([]string{"history", "-raw", "-server", ts.URL, asset.String()}, d); err != nil {
		t.Fatalf("history -server: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), out)
	}
	fields := strings.Split(lines[1], "\t")
	if len(fields) != 4 || fields[2] != "unknown" {
		t.Errorf("undetermined pair printed as %q, want transition unknown", lines[1])
	}
	if !strings.Contains(errOut.String(), "state unknown") {
		t.Errorf("summary does not report unknown state:\n%s", errOut)
	}
}

func TestHistoryServerEmpty(t *testing.T) {
	ts := historyServer(t)
	d, out, errOut, _ := deps(nil, nil)
	if err := runWith([]string{"history", "-guarantee", "-server", ts.URL, "USDC-" + cliIssuer}, d); err == nil {
		t.Error("history -guarantee on an empty server history returned nil error")
	}
	if !strings.Contains(out.String(), "no observations") {
		t.Errorf("output = %q", out)
	}
	if errOut.Len() != 0 {
		t.Errorf("empty history printed a summary: %q", errOut)
	}
}
