// Package clockskew records upstream Date headers that disagree with the
// scanner host's wall clock.
package clockskew

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Threshold is the maximum tolerated absolute difference between an upstream
// Date header and the scanner's local clock.
const Threshold = 5 * time.Minute

type recorderKey struct{}

// Observation describes one upstream Date header beyond the skew threshold.
// Times use Assay's canonical UTC, whole-second representation.
type Observation struct {
	Source           string `json:"source"`
	URL              string `json:"url"`
	ServerTime       string `json:"server_time"`
	ObservedAt       string `json:"observed_at"`
	SkewSeconds      int64  `json:"skew_seconds"`
	ThresholdSeconds int64  `json:"threshold_seconds"`
}

// Recorder collects clock-skew observations made during one scan.
type Recorder struct {
	mu           sync.Mutex
	observations []Observation
}

// NewRecorder creates an empty scan-scoped recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// WithRecorder adds a recorder to ctx for upstream fetchers to use.
func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, recorder)
}

// Record adds a diagnostic when dateHeader is farther than Threshold from
// observedAt. Invalid or absent Date headers are ignored because they do not
// provide a trustworthy comparison. It never changes observedAt or any source
// timestamp.
func Record(ctx context.Context, source, url, dateHeader string, observedAt time.Time) {
	if dateHeader == "" {
		return
	}
	serverTime, err := http.ParseTime(dateHeader)
	if err != nil {
		return
	}
	delta := observedAt.Sub(serverTime)
	if delta <= Threshold && delta >= -Threshold {
		return
	}
	recorder, ok := ctx.Value(recorderKey{}).(*Recorder)
	if !ok || recorder == nil {
		return
	}
	recorder.mu.Lock()
	recorder.observations = append(recorder.observations, Observation{
		Source:           source,
		URL:              url,
		ServerTime:       canonicalTime(serverTime),
		ObservedAt:       canonicalTime(observedAt),
		SkewSeconds:      int64(delta.Seconds()),
		ThresholdSeconds: int64(Threshold.Seconds()),
	})
	recorder.mu.Unlock()
}

// Observations returns a snapshot of the skew diagnostics collected so far.
func (r *Recorder) Observations() []Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	observations := append([]Observation(nil), r.observations...)
	sort.Slice(observations, func(i, j int) bool {
		if observations[i].Source != observations[j].Source {
			return observations[i].Source < observations[j].Source
		}
		if observations[i].URL != observations[j].URL {
			return observations[i].URL < observations[j].URL
		}
		if observations[i].ServerTime != observations[j].ServerTime {
			return observations[i].ServerTime < observations[j].ServerTime
		}
		return observations[i].ObservedAt < observations[j].ObservedAt
	})
	return observations
}

func canonicalTime(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}
