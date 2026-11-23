package mechanics_test

import (
	"testing"
	"time"

	"github.com/use-assay/assay/internal/mechanics"
)

func TestFreshnessRequiresObservationTime(t *testing.T) {
	asOf := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		scanned time.Time
		window  time.Duration
		check   func(*mechanics.Report) bool
	}{
		{"missing", time.Time{}, time.Hour, func(r *mechanics.Report) bool { return r.State == mechanics.StateUnknown && !r.Stale }},
		{"missing with disabled policy", time.Time{}, 0, func(r *mechanics.Report) bool { return r.State == mechanics.StateUnknown && !r.Stale }},
		{"past window", asOf.Add(-2 * time.Hour), time.Hour, func(r *mechanics.Report) bool { return r.State == mechanics.StateStale && r.Stale }},
		{"within window", asOf.Add(-time.Minute), time.Hour, func(r *mechanics.Report) bool { return r.State == mechanics.StateValid && !r.Stale }},
		{"future", asOf.Add(time.Minute), time.Hour, func(r *mechanics.Report) bool { return r.State == mechanics.StateValid && !r.Stale }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep := &mechanics.Report{ScannedAt: mechanics.CanonicalTime(tt.scanned), Base: mechanics.Clear, Severity: mechanics.Clear}
			if result := mechanics.EvaluateFreshness(rep, asOf, tt.window); !tt.check(result) {
				t.Fatalf("unexpected freshness state %v (stale=%v)", result.State, result.Stale)
			}
		})
	}
}
