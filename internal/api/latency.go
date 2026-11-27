package api

import (
	"fmt"
	"net/http"
	"time"
)

// latencyTransport records how long each upstream request took against the
// per-source latency histogram served at GET /metrics. It measures the round
// trip to response headers, which is the time a scan waits on that source; a
// transport error is recorded too, because a source that times out is the
// slowest answer it can give.
type latencyTransport struct {
	next    http.RoundTripper
	metrics *Metrics
	source  string
}

func (t *latencyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	t.metrics.IncSourceLatency(t.source, time.Since(start))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.source, err)
	}
	return resp, nil
}

// instrumentClient wraps c's transport so every request it makes is timed under
// source. A nil client is left alone.
func instrumentClient(c *http.Client, m *Metrics, source string) {
	if c == nil || m == nil {
		return
	}
	next := c.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	c.Transport = &latencyTransport{next: next, metrics: m, source: source}
}

// instrumentSources times the scanner's upstream fetchers. It runs once, when
// the handler is built, so a scanner swapped in after NewServer (as the serve
// command does) is still covered.
func (s *Server) instrumentSources() {
	sc := s.Scanner
	if sc == nil || s.Metrics == nil {
		return
	}
	if sc.Horizon != nil {
		instrumentClient(sc.Horizon.HTTP, s.Metrics, "horizon")
	}
	if sc.Expert != nil {
		instrumentClient(sc.Expert.HTTP, s.Metrics, "stellar.expert")
	}
	if sc.Toml != nil {
		instrumentClient(sc.Toml.HTTP, s.Metrics, "stellar.toml")
	}
	if sc.Lists != nil {
		instrumentClient(sc.Lists.HTTP, s.Metrics, "asset-list")
	}
}
