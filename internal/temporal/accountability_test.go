package temporal_test

import (
	"testing"
	"time"

	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/temporal"
)

func observation(t *testing.T, when string, acct mechanics.Accountability, undetermined bool) temporal.Observation {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, when)
	if err != nil {
		t.Fatalf("parse %s: %v", when, err)
	}
	return temporal.Observation{
		Asset:          mechanics.Asset{Code: "AQUA", Issuer: "GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA"},
		At:             ts.UTC(),
		Accountability: acct,
		Undetermined:   undetermined,
	}
}

func TestAccountabilityTransitionReportsChange(t *testing.T) {
	m := temporal.AccountabilityTransition(
		observation(t, "2026-09-01T00:00:00Z", mechanics.AccountabilityVerified, false),
		observation(t, "2026-09-08T00:00:00Z", mechanics.AccountabilityUnverified, false),
	)
	if m.State != temporal.Valid {
		t.Fatalf("State = %s, want valid", m.State)
	}
	if !m.Changed {
		t.Fatal("Changed = false, want true")
	}
	if m.From != mechanics.AccountabilityVerified || m.To != mechanics.AccountabilityUnverified {
		t.Fatalf("movement = %s -> %s, want verified -> unverified", m.From, m.To)
	}
}

func TestAccountabilityTransitionReportsUnchanged(t *testing.T) {
	m := temporal.AccountabilityTransition(
		observation(t, "2026-09-01T00:00:00Z", mechanics.AccountabilityUnverified, false),
		observation(t, "2026-09-08T00:00:00Z", mechanics.AccountabilityUnverified, false),
	)
	if m.State != temporal.Valid || m.Changed {
		t.Fatalf("movement = %+v, want valid and unchanged", m)
	}
}

func TestAccountabilityTransitionIsUnknownWhenEitherSideIsPartial(t *testing.T) {
	m := temporal.AccountabilityTransition(
		observation(t, "2026-09-01T00:00:00Z", mechanics.AccountabilityVerified, true),
		observation(t, "2026-09-08T00:00:00Z", mechanics.AccountabilityUnverified, false),
	)
	if m.State != temporal.Unknown {
		t.Fatalf("State = %s, want unknown", m.State)
	}
	if m.Changed {
		t.Fatal("Changed = true on an unknown comparison; a partial side must not produce a movement")
	}
	if m.Reason == "" {
		t.Fatal("Reason is empty; unknown must say what was partial")
	}
}

// The transition carries the same movement as the standalone detector, so a
// consumer of /api/v1/history and a caller of the detector cannot disagree.
func TestTransitionCarriesAccountability(t *testing.T) {
	from := observation(t, "2026-09-01T00:00:00Z", mechanics.AccountabilityUnknown, false)
	to := observation(t, "2026-09-08T00:00:00Z", mechanics.AccountabilityVerified, false)

	tr := temporal.Between(from, to)
	if tr.Accountability != temporal.AccountabilityTransition(from, to) {
		t.Fatalf("transition accountability = %+v, want %+v",
			tr.Accountability, temporal.AccountabilityTransition(from, to))
	}
	if tr.Accountability.From != mechanics.AccountabilityUnknown ||
		tr.Accountability.To != mechanics.AccountabilityVerified || !tr.Accountability.Changed {
		t.Fatalf("from/to values wrong: %+v", tr.Accountability)
	}
}

func TestAccountabilityTransitionIsMissingWithoutAPair(t *testing.T) {
	m := temporal.AccountabilityTransition(
		observation(t, "2026-09-01T00:00:00Z", mechanics.AccountabilityVerified, false),
		temporal.Observation{},
	)
	if m.State != temporal.Missing || m.Changed || m.Reason == "" {
		t.Fatalf("movement = %+v, want missing with no change and a reason", m)
	}
}
