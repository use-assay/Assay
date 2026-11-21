package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// A caller-supplied X-Request-Id is echoed and logged only when it is a short
// safe token. Anything else is replaced by a generated identifier, so a long
// or hostile header cannot bloat every log line of the request.
func TestRequestIDAcceptsOnlySafeValues(t *testing.T) {
	generated := regexp.MustCompile(`^[0-9a-f]{32}$`)

	for _, tc := range []struct {
		name      string
		header    string
		setHeader bool
		wantEcho  bool
	}{
		{name: "absent", setHeader: false},
		{name: "empty", header: "", setHeader: true},
		{name: "valid", header: "req-123_AB.c:d", setHeader: true, wantEcho: true},
		{name: "maximum length", header: strings.Repeat("a", maxRequestIDLen), setHeader: true, wantEcho: true},
		{name: "too long", header: strings.Repeat("a", maxRequestIDLen+1), setHeader: true},
		{name: "newline", header: "abc\ndef", setHeader: true},
		{name: "space", header: "abc def", setHeader: true},
		{name: "control", header: "abc\x00def", setHeader: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			srv := NewServer(slog.New(slog.NewJSONHandler(&logs, nil)))

			var contextID string
			handler := srv.withObservability(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contextID = RequestID(r.Context())
				srv.loggerFrom(r.Context()).Info("handled")
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.setHeader {
				req.Header.Set(requestIDHeader, tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			got := rec.Header().Get(requestIDHeader)
			if got == "" {
				t.Fatal("no request identifier was echoed")
			}
			if tc.wantEcho {
				if got != tc.header {
					t.Errorf("echoed identifier = %q, want the caller's %q", got, tc.header)
				}
			} else {
				if got == tc.header {
					t.Errorf("unsafe identifier %q was accepted", tc.header)
				}
				if !generated.MatchString(got) {
					t.Errorf("generated identifier = %q, want 32 lowercase hex characters", got)
				}
			}
			if contextID != got {
				t.Errorf("context identifier = %q, want the echoed %q", contextID, got)
			}

			line := strings.TrimSpace(logs.String())
			if line == "" {
				t.Fatal("handler produced no log line")
			}
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("log line is not one JSON record: %q: %v", line, err)
			}
			if record[requestIDLogKey] != got {
				t.Errorf("logged %s = %v, want %q", requestIDLogKey, record[requestIDLogKey], got)
			}
		})
	}
}
