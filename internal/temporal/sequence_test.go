package temporal_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/temporal"
)

// The add-then-remove fixture is the case issue #599 exists for: the last pair
// reads as a removal, and only the whole sequence shows the window in which
// clawback was held.
func TestAllSeesAddThenRemove(t *testing.T) {
	s := loadSequence(t, "seq8-add-then-remove")

	all := temporal.All(s.obs)
	if len(all) != 2 {
		t.Fatalf("All returned %d transitions over %d observations, want 2", len(all), len(s.obs))
	}
	for i, tr := range all {
		if tr.State != temporal.Valid {
			t.Fatalf("transition %d state = %q, want valid: %s", i, tr.State, tr.Reason)
		}
	}
	if all[0].Added != mechanics.MechClawbackEnabled || all[0].Removed != 0 {
		t.Errorf("first transition added=%v removed=%v, want clawback added", all[0].AddedNames, all[0].RemovedNames)
	}
	if all[1].Added != 0 || all[1].Removed != mechanics.MechClawbackEnabled {
		t.Errorf("second transition added=%v removed=%v, want clawback removed", all[1].AddedNames, all[1].RemovedNames)
	}

	// The last element of All is exactly what Of reports, so the two surfaces
	// never disagree about the most recent change.
	of := temporal.Of(s.obs)
	if !of.From.At.Equal(all[1].From.At) || !of.To.At.Equal(all[1].To.At) ||
		of.Added != all[1].Added || of.Removed != all[1].Removed {
		t.Errorf("Of disagrees with the last transition of All: %+v vs %+v", of, all[1])
	}

	sum := temporal.Summarize(s.obs)
	if sum.State != temporal.Valid || sum.Reason != "" {
		t.Fatalf("summary state = %q (%q), want valid with no reason", sum.State, sum.Reason)
	}
	if sum.Count != 3 {
		t.Errorf("summary count = %d, want 3", sum.Count)
	}
	if sum.Held != mechanics.MechClawbackEnabled {
		t.Errorf("held = %v, want auth_clawback_enabled", sum.HeldNames)
	}
	if sum.Latest != 0 || len(sum.LatestNames) != 0 {
		t.Errorf("latest = %v, want nothing", sum.LatestNames)
	}
	if sum.Withdrawn != mechanics.MechClawbackEnabled {
		t.Errorf("withdrawn = %v, want auth_clawback_enabled", sum.WithdrawnNames)
	}
	if !slices.Equal(sum.HeldNames, mechanics.MechClawbackEnabled.Names()) {
		t.Errorf("held names = %v, want %v", sum.HeldNames, mechanics.MechClawbackEnabled.Names())
	}
}

// Every fixture with n observations yields n-1 transitions, each between
// time-adjacent observations, and Summarize holds every capability bit any of
// them carries.
func TestAllCoversEveryAdjacentPairInEveryFixture(t *testing.T) {
	for _, s := range sequences(t) {
		t.Run(s.dir, func(t *testing.T) {
			all := temporal.All(s.obs)
			if len(all) != len(s.obs)-1 {
				t.Fatalf("All returned %d transitions over %d observations", len(all), len(s.obs))
			}
			for i := 1; i < len(all); i++ {
				if !all[i].From.At.Equal(all[i-1].To.At) {
					t.Errorf("transition %d does not start where %d ended: %v vs %v",
						i, i-1, all[i].From.At, all[i-1].To.At)
				}
			}
			var held mechanics.Mechanic
			for _, o := range s.obs {
				held |= o.Mechanics & temporal.CapabilityMask
			}
			if got := temporal.Summarize(s.obs).Held; got != held {
				t.Errorf("summary held = %v, want %v", got.Names(), held.Names())
			}
		})
	}
}

func TestAllOrdersByTime(t *testing.T) {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	obs := []temporal.Observation{
		{At: base.Add(48 * time.Hour), Mechanics: 0},
		{At: base, Mechanics: 0},
		{At: base.Add(24 * time.Hour), Mechanics: mechanics.MechAuthRevocable},
	}
	in := slices.Clone(obs)

	all := temporal.All(obs)
	if len(all) != 2 {
		t.Fatalf("got %d transitions, want 2", len(all))
	}
	if all[0].Added != mechanics.MechAuthRevocable || all[1].Removed != mechanics.MechAuthRevocable {
		t.Errorf("transitions out of time order: %+v", all)
	}
	for i := range obs {
		if !obs[i].At.Equal(in[i].At) {
			t.Fatal("All reordered the caller's slice")
		}
	}
}

func TestAllFewerThanTwo(t *testing.T) {
	for _, obs := range [][]temporal.Observation{nil, {{At: time.Now()}}} {
		got := temporal.All(obs)
		if got == nil || len(got) != 0 {
			t.Errorf("All(%d observations) = %#v, want an empty non-nil slice", len(obs), got)
		}
		b, _ := json.Marshal(got)
		if string(b) != "[]" {
			t.Errorf("All(%d observations) encodes as %s, want []", len(obs), b)
		}
	}
}

// An undetermined observation in the middle makes both pairs around it
// Unknown, never "no change", and the summary Unknown with the check named —
// while still reporting a bit that partial observation did read.
func TestAllAndSummaryUndetermined(t *testing.T) {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	obs := []temporal.Observation{
		{At: base},
		{At: base.Add(time.Hour), Mechanics: mechanics.MechClawbackEnabled,
			Undetermined: true, UndeterminedChecks: []string{"reputation"}},
		{At: base.Add(2 * time.Hour)},
	}
	all := temporal.All(obs)
	for i, tr := range all {
		if tr.State != temporal.Unknown {
			t.Errorf("transition %d state = %q, want unknown", i, tr.State)
		}
	}
	sum := temporal.Summarize(obs)
	if sum.State != temporal.Unknown {
		t.Fatalf("summary state = %q, want unknown", sum.State)
	}
	if !strings.Contains(sum.Reason, "reputation") {
		t.Errorf("summary reason %q does not name the incomplete check", sum.Reason)
	}
	if sum.Held != mechanics.MechClawbackEnabled {
		t.Errorf("held = %v: a bit read by a partial observation must still be reported", sum.HeldNames)
	}
}

func TestSummarizeEmptyIsMissing(t *testing.T) {
	sum := temporal.Summarize(nil)
	if sum.State != temporal.Missing || sum.Reason == "" {
		t.Fatalf("summary of nothing = %q (%q), want missing with a reason", sum.State, sum.Reason)
	}
	b, err := json.Marshal(sum)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("empty summary encodes a null name list: %s", b)
	}
}

// Only capability bits are summarised: a reported-only signal held at some
// point is not a power over a holder's balance.
func TestSummarizeIgnoresNonCapabilityBits(t *testing.T) {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	obs := []temporal.Observation{
		{At: base, Mechanics: mechanics.MechBlocklisted | mechanics.MechFlagsLocked},
		{At: base.Add(time.Hour)},
	}
	if sum := temporal.Summarize(obs); sum.Held != 0 || sum.Withdrawn != 0 {
		t.Errorf("summary held %v, withdrawn %v; want no capability bits", sum.HeldNames, sum.WithdrawnNames)
	}
}
