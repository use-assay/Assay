package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/scan"
)

// TestServeHistoryRetentionWiring checks that -history-retention reaches both
// store forms and is what /api/v1/history reports back as "retention".
func TestServeHistoryRetentionWiring(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tc := range []struct {
		name        string
		historyPath string
	}{
		{name: "in-memory"},
		{name: "file-backed", historyPath: "history.jsonl"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.historyPath
			if path != "" {
				path = filepath.Join(t.TempDir(), path)
			}
			srv, closeHistory, err := newServeServer(log, nil, scan.DefaultOptions(), path, 7)
			if err != nil {
				t.Fatalf("newServeServer: %v", err)
			}
			t.Cleanup(func() { _ = closeHistory() })

			if got := srv.History.RetentionLimit(); got != 7 {
				t.Errorf("store retention = %d, want 7", got)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/history?asset=USDC-"+cliIssuer, nil)
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET /api/v1/history = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Retention int `json:"retention"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode history response: %v", err)
			}
			if body.Retention != 7 {
				t.Errorf("response retention = %d, want 7", body.Retention)
			}
		})
	}
}

// A retention below one has no meaning: the store would evict every
// observation it wrote. It is a usage error, not a silent clamp.
func TestServeRejectsInvalidHistoryRetention(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, value := range []string{"0", "-1"} {
		err := runServe([]string{"-history-retention", value}, log)
		if err == nil {
			t.Fatalf("-history-retention %s was accepted", value)
		}
		if got := exitCodeForError(err); got != exitUsage {
			t.Errorf("-history-retention %s exit = %d, want %d", value, got, exitUsage)
		}
		if !strings.Contains(err.Error(), "history-retention") {
			t.Errorf("-history-retention %s error = %v, want it to name the flag", value, err)
		}
	}
}
