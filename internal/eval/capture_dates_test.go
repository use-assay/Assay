package eval

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureDatesReadsManifestDates(t *testing.T) {
	root := t.TempDir()
	manifest := `{"subjects":[{"fixture":"alpha","capture_date":"2026-08-10"},{"fixture":"beta","capture_date":"2026-09-25"}]}`
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dates, err := CaptureDates(root)
	if err != nil {
		t.Fatalf("CaptureDates: %v", err)
	}
	want := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	if !dates["alpha"].Equal(want) || dates["beta"].Format("2006-01-02") != "2026-09-25" {
		t.Fatalf("capture dates = %v", dates)
	}
}

func TestCaptureDatesRejectsDuplicateSubjects(t *testing.T) {
	root := t.TempDir()
	manifest := `{"subjects":[{"fixture":"alpha","capture_date":"2026-08-10"},{"fixture":"alpha","capture_date":"2026-09-25"}]}`
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := CaptureDates(root); err == nil {
		t.Fatal("duplicate manifest subject was accepted")
	}
}
