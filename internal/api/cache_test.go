package api

import (
	"testing"
	"time"

	"github.com/use-assay/assay/internal/mechanics"
)

func TestOldestEvidenceAge(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	t.Run("oldest evidence wins", func(t *testing.T) {
		rep := &mechanics.Report{
			ScannedAt: mechanics.NewCanonicalTime(now),
			Evidence: []mechanics.Evidence{
				{RetrievedAt: mechanics.NewCanonicalTime(now.Add(-90 * time.Second))},
				{RetrievedAt: mechanics.NewCanonicalTime(now.Add(-10 * time.Second))},
			},
		}
		got, ok := oldestEvidenceAge(rep, now)
		if !ok || got != 90 {
			t.Fatalf("oldestEvidenceAge = (%d, %v), want (90, true)", got, ok)
		}
	})

	t.Run("falls back to the scan time", func(t *testing.T) {
		rep := &mechanics.Report{ScannedAt: mechanics.NewCanonicalTime(now.Add(-45 * time.Second))}
		got, ok := oldestEvidenceAge(rep, now)
		if !ok || got != 45 {
			t.Fatalf("oldestEvidenceAge = (%d, %v), want (45, true)", got, ok)
		}
	})

	t.Run("future source clock is clamped", func(t *testing.T) {
		rep := &mechanics.Report{Evidence: []mechanics.Evidence{
			{RetrievedAt: mechanics.NewCanonicalTime(now.Add(30 * time.Second))},
		}}
		got, ok := oldestEvidenceAge(rep, now)
		if !ok || got != 0 {
			t.Fatalf("oldestEvidenceAge = (%d, %v), want (0, true)", got, ok)
		}
	})

	t.Run("no timestamp is unknown, not zero age", func(t *testing.T) {
		if got, ok := oldestEvidenceAge(&mechanics.Report{}, now); ok {
			t.Fatalf("oldestEvidenceAge = (%d, true), want absent", got)
		}
		if got, ok := oldestEvidenceAge(nil, now); ok {
			t.Fatalf("oldestEvidenceAge(nil) = (%d, true), want absent", got)
		}
	})
}
