package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/use-assay/assay/internal/mechanics"
)

// A scan answer is a claim about one instant. An intermediary that caches it
// without the instant would serve an old verdict as if it were current, and the
// caller would have no way to notice: freshness is the caller's policy, but
// only if the age reaches the caller. So dynamic answers are never stored, and
// a scan response carries the age of the oldest signal that produced it.
//
// The age travels in X-Assay-Oldest-Evidence-Age rather than the standard Age
// header. Age (RFC 9111) describes how long a response has been in an HTTP
// cache; this value describes how old the ledger and curated-source data inside
// the response is. Using Age for the latter would make a cache hop and a stale
// source indistinguishable. The header is an integer number of seconds.
const oldestEvidenceAgeHeader = "X-Assay-Oldest-Evidence-Age"

// withCacheControl marks the dynamic surface as non-cacheable. The embedded UI
// is allowed to be stored but must be revalidated on every use, so a browser
// holding an old page cannot silently pair it with a newer API.
func (s *Server) withCacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(r.URL.Path, "/api/"),
			r.URL.Path == "/metrics",
			r.URL.Path == "/healthz",
			r.URL.Path == "/readyz":
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// oldestEvidenceAge returns the age, in whole seconds, of the oldest timestamp
// that contributed to a report. Evidence completion times are preferred;
// ScannedAt is the fallback for a report that carries no evidence timestamps,
// because the scan instant still bounds how old the answer can be. A future
// timestamp (a skewed source clock) is clamped to zero rather than reported as
// negative age.
func oldestEvidenceAge(rep *mechanics.Report, now time.Time) (int64, bool) {
	if rep == nil {
		return 0, false
	}
	var oldest time.Time
	for _, ev := range rep.Evidence {
		t := ev.RetrievedAt.Time()
		if t.IsZero() {
			continue
		}
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
	}
	if oldest.IsZero() {
		oldest = rep.ScannedAt.Time()
	}
	if oldest.IsZero() {
		return 0, false
	}
	age := now.Sub(oldest)
	if age < 0 {
		age = 0
	}
	return int64(age / time.Second), true
}

// setScanFreshnessHeaders attaches the non-cacheable and evidence-age headers
// to a successful scan response.
func (s *Server) setScanFreshnessHeaders(w http.ResponseWriter, rep *mechanics.Report) {
	w.Header().Set("Cache-Control", "no-store")
	if age, ok := oldestEvidenceAge(rep, s.now()); ok {
		w.Header().Set(oldestEvidenceAgeHeader, strconv.FormatInt(age, 10))
	}
}

// now reads the scanner clock so a test that pins it also pins the age header.
// Production leaves Scanner.Now nil, which means the wall clock.
func (s *Server) now() time.Time {
	if s.Scanner != nil && s.Scanner.Now != nil {
		return s.Scanner.Now().UTC()
	}
	return time.Now().UTC()
}
