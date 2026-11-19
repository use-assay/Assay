package clockskew

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRecordDetectsDateHeaderSkewFromFakeServer(t *testing.T) {
	serverTime := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Date", serverTime.Format(http.TimeFormat))
	}))
	defer server.Close()

	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatalf("fake upstream request: %v", err)
	}
	defer resp.Body.Close()

	observedAt := serverTime.Add(6 * time.Minute)
	recorder := NewRecorder()
	Record(WithRecorder(context.Background(), recorder), "fake-upstream", resp.Request.URL.String(),
		resp.Header.Get("Date"), observedAt)

	observations := recorder.Observations()
	if len(observations) != 1 {
		t.Fatalf("observations = %d, want one", len(observations))
	}
	got := observations[0]
	if got.Source != "fake-upstream" ||
		got.ServerTime != canonicalTime(serverTime) || got.ObservedAt != canonicalTime(observedAt) {
		t.Fatalf("skew observation = %+v", got)
	}
	if got.SkewSeconds != 360 || got.ThresholdSeconds != 300 {
		t.Fatalf("skew/threshold = %d/%d, want 360/300", got.SkewSeconds, got.ThresholdSeconds)
	}
}

func TestRecordIgnoresWithinThresholdAndInvalidDates(t *testing.T) {
	recorder := NewRecorder()
	ctx := WithRecorder(context.Background(), recorder)
	now := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	Record(ctx, "upstream", "https://example.test", now.Add(Threshold).Format(http.TimeFormat), now)
	Record(ctx, "upstream", "https://example.test", "not a date", now)
	if got := len(recorder.Observations()); got != 0 {
		t.Fatalf("observations = %d, want none", got)
	}
}
