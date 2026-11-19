package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/eval"
)

func TestPrintFixtureAgesFlagsSubjectsOlderThan90Days(t *testing.T) {
	now := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	rec := &eval.Record{Subjects: []eval.SubjectResult{
		{Dir: "recent"},
		{Dir: "ninety-days"},
		{Dir: "stale"},
	}}
	dates := map[string]time.Time{
		"recent":      time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC),
		"ninety-days": time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
		"stale":       time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
	}
	var out bytes.Buffer
	if err := printFixtureAges(&out, rec, dates, now); err != nil {
		t.Fatalf("printFixtureAges: %v", err)
	}
	text := out.String()
	for _, want := range []string{
		"recent captured 2026-11-18 (1 day old)",
		"ninety-days captured 2026-08-21 (90 days old)",
		"stale captured 2026-08-20 (91 days old) - STALE: older than 90 days",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("age output does not contain %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "ninety-days captured 2026-08-21 (90 days old) - STALE") {
		t.Error("a fixture exactly 90 days old was flagged stale")
	}
}
