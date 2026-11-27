package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSourceLatencyIsRecordedForUpstreamFetches pins that a fetch made through
// the scanner's Horizon client lands in the per-source latency histogram, so
// the metric is not permanently zero.
func TestSourceLatencyIsRecordedForUpstreamFetches(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	s := NewServer(nil)
	s.Handler() // instruments the scanner's transports

	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.Scanner.Horizon.HTTP.Do(req)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	_ = resp.Body.Close()

	counts := s.Metrics.SourceLatencyBucketCounts()["horizon"]
	var total uint64
	for _, c := range counts {
		total += c
	}
	if total != 1 {
		t.Fatalf("horizon latency observations = %d, want 1 (counts %v)", total, counts)
	}
	if _, ok := s.Metrics.SourceLatencyBucketCounts()["stellar.expert"]; ok {
		t.Error("no stellar.expert fetch was made, so it must have no latency series")
	}
}

// TestSourceLatencySnapshotIsACopy pins that a returned snapshot does not alias
// the live buckets, so later observations cannot change it under the reader.
func TestSourceLatencySnapshotIsACopy(t *testing.T) {
	m := NewMetrics()
	m.IncSourceLatency("horizon", 0)
	snap := m.SourceLatencyBucketCounts()["horizon"]
	m.IncSourceLatency("horizon", 0)
	if snap[0] != 1 {
		t.Fatalf("snapshot changed after a later observation: %v", snap)
	}
}
