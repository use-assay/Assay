package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type captureManifest struct {
	Subjects []struct {
		Fixture     string `json:"fixture"`
		CaptureDate string `json:"capture_date"`
	} `json:"subjects"`
}

// CaptureDates returns each subject's capture date from the dataset manifest.
func CaptureDates(fixturesDir string) (map[string]time.Time, error) {
	path := filepath.Join(fixturesDir, "manifest.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture manifest %s: %w", path, err)
	}
	var manifest captureManifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil, fmt.Errorf("decode fixture manifest %s: %w", path, err)
	}
	dates := make(map[string]time.Time, len(manifest.Subjects))
	for _, subject := range manifest.Subjects {
		if subject.Fixture == "" {
			return nil, fmt.Errorf("fixture manifest %s: subject has no fixture name", path)
		}
		if _, exists := dates[subject.Fixture]; exists {
			return nil, fmt.Errorf("fixture manifest %s: duplicate subject %q", path, subject.Fixture)
		}
		date, err := time.Parse("2006-01-02", subject.CaptureDate)
		if err != nil {
			return nil, fmt.Errorf("fixture manifest %s: subject %s has invalid capture_date %q: %w",
				path, subject.Fixture, subject.CaptureDate, err)
		}
		dates[subject.Fixture] = date
	}
	if len(dates) == 0 {
		return nil, fmt.Errorf("fixture manifest %s: no subject capture dates", path)
	}
	return dates, nil
}
