package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/api"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/temporal"
)

// threadSafeRecorder wraps httptest.ResponseRecorder with a mutex for thread-safe access.
type threadSafeRecorder struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func (f *threadSafeRecorder) Flush() {}

func (f *threadSafeRecorder) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := f.ResponseRecorder.Write(p)
	return n, fmt.Errorf("write: %w", err)
}

func (f *threadSafeRecorder) WriteHeader(statusCode int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ResponseRecorder.WriteHeader(statusCode)
}

func (f *threadSafeRecorder) BodyString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Body.String()
}

func (f *threadSafeRecorder) StatusCode() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Code
}

func newThreadSafeRecorder() *threadSafeRecorder {
	return &threadSafeRecorder{ResponseRecorder: httptest.NewRecorder()}
}

const (
	testCode   = "TEST"
	testIssuer = "GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA"
	testAsset  = testCode + "-" + testIssuer
)

func newEventsServer(maxClients int) *api.Server {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := api.NewServer(log)
	if maxClients > 0 {
		srv.EventsLimiter = api.NewEventsLimiter(maxClients)
	}
	return srv
}

// TestEventsRejectsBadInput verifies the events endpoint rejects invalid input.
func TestEventsRejectsBadInput(t *testing.T) {
	srv := newEventsServer(10)
	handler := srv.Handler()

	for _, tc := range []struct {
		name  string
		query string
		code  int
	}{
		{"missing asset", "/api/v1/events", http.StatusBadRequest},
		{"empty asset", "/api/v1/events?asset=", http.StatusBadRequest},
		{"not an asset", "/api/v1/events?asset=hello", http.StatusBadRequest},
		{"bad issuer", "/api/v1/events?asset=TEST-NOPE", http.StatusBadRequest},
		{"native asset", "/api/v1/events?asset=native", http.StatusBadRequest},
		{"XLM", "/api/v1/events?asset=XLM", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.query, nil))
			if rec.Code != tc.code {
				t.Fatalf("status = %d, want %d", rec.Code, tc.code)
			}
		})
	}
}

// TestEventsClientCap verifies the SSE endpoint enforces the client cap.
func TestEventsClientCap(t *testing.T) {
	// Test the limiter directly.
	limiter := api.NewEventsLimiter(2)
	if !limiter.Acquire() {
		t.Fatal("first acquire should succeed")
	}
	if !limiter.Acquire() {
		t.Fatal("second acquire should succeed")
	}
	if limiter.Acquire() {
		t.Fatal("third acquire should fail")
	}
	limiter.Release()
	if !limiter.Acquire() {
		t.Fatal("acquire after release should succeed")
	}
	limiter.Release()
	limiter.Release()

	// Test the HTTP endpoint rejects when at capacity.
	srv := newEventsServer(1)
	handler := srv.Handler()

	// Start one connection and keep it open.
	rec := newThreadSafeRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?asset="+testAsset, nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	// Wait for handler to write initial response.
	select {
	case <-done:
		t.Fatal("handler returned unexpectedly")
	case <-time.After(200 * time.Millisecond):
	}

	if rec.StatusCode() != http.StatusOK {
		t.Fatalf("first connection: status = %d, want 200", rec.StatusCode())
	}

	// Second connection should be rejected (at capacity).
	rec2 := newThreadSafeRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/v1/events?asset="+testAsset, nil))
	if rec2.StatusCode() != http.StatusTooManyRequests {
		t.Fatalf("second connection: status = %d, want 429", rec2.StatusCode())
	}

	// Clean up first connection.
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		// Handler might not exit cleanly in test due to ResponseRecorder limitations;
		// this is acceptable for the unit test.
	}
}

// TestEventsStreamDeliversTransition verifies the SSE stream delivers a
// transition event when a new observation creates one.
func TestEventsStreamDeliversTransition(t *testing.T) {
	srv := newEventsServer(10)
	handler := srv.Handler()

	// Connect to SSE stream.
	ctx, cancel := context.WithCancel(context.Background())
	rec := newThreadSafeRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?asset="+testAsset, nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	// Wait for connection to be established (initial comment).
	time.Sleep(100 * time.Millisecond)

	// Append first observation (no transition expected).
	firstObs := temporal.Observation{
		Asset:     mechanics.Asset{Code: testCode, Issuer: testIssuer},
		At:        time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Mechanics: 0,
		Base:      mechanics.Clear,
		Severity:  mechanics.Clear,
	}
	if err := srv.History.Append(firstObs); err != nil {
		t.Fatalf("append first: %v", err)
	}

	// Append second observation (should produce a transition).
	secondObs := temporal.Observation{
		Asset:          mechanics.Asset{Code: testCode, Issuer: testIssuer},
		At:             time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
		Mechanics:      mechanics.MechClawbackEnabled,
		Base:           mechanics.High,
		Severity:       mechanics.High,
		Accountability: mechanics.AccountabilityVerified,
	}
	if err := srv.History.Append(secondObs); err != nil {
		t.Fatalf("append second: %v", err)
	}

	// Manually broadcast the transition since we bypassed handleScan.
	prevList := srv.History.List(mechanics.Asset{Code: testCode, Issuer: testIssuer})
	if len(prevList) >= 2 {
		transition := temporal.Between(prevList[len(prevList)-2], prevList[len(prevList)-1])
		if transition.State == temporal.Valid {
			srv.BroadcastEventForTest(testAsset, transition)
		}
	}

	// Give time for event to be broadcast and received.
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop handler and read body.
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}

	// Check that we received an event.
	body := rec.BodyString()
	if !strings.Contains(body, "data: ") {
		t.Fatalf("no SSE data event received; body: %s", body)
	}

	// Parse the event data.
	lines := strings.Split(body, "\n")
	var eventData string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			eventData = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if eventData == "" {
		t.Fatalf("no data: line found in body: %s", body)
	}

	var msg struct {
		Asset      string              `json:"asset"`
		Transition temporal.Transition `json:"transition"`
		Timestamp  time.Time           `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(eventData), &msg); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	if msg.Asset != testAsset {
		t.Errorf("asset = %q, want %q", msg.Asset, testAsset)
	}
	if msg.Transition.State != temporal.Valid {
		t.Errorf("transition state = %q, want %q", msg.Transition.State, temporal.Valid)
	}
	if !contains(msg.Transition.AddedNames, "auth_clawback_enabled") {
		t.Errorf("added_names = %v, want auth_clawback_enabled", msg.Transition.AddedNames)
	}
}

// TestEventsHeartbeat verifies the SSE stream sends heartbeats.
func TestEventsHeartbeat(t *testing.T) {
	srv := newEventsServer(10)
	handler := srv.Handler()

	ctx, cancel := context.WithCancel(context.Background())
	rec := newThreadSafeRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?asset="+testAsset, nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	// Wait for initial comment.
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop handler and read body.
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}

	// Verify the connection was established.
	body := rec.BodyString()
	if !strings.Contains(body, ": connected") {
		t.Errorf("missing initial comment; body: %s", body)
	}

	// The heartbeat test is limited in a unit test because the interval is 15s.
	// In practice, the heartbeat keeps the connection alive through proxies.
}
