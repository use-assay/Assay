package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/use-assay/assay/internal/history"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/scan"
	"github.com/use-assay/assay/internal/temporal"
)

// History pagination bounds. A history page is bounded because the retained
// history is the response size the server hits first: capped at
// DefaultRetention observations per asset, but still returned whole on every
// request until pagination existed. The default is small enough to be a
// comfortable response and the maximum is the retention cap, so one page can
// never be larger than the history that exists. A limit outside the range is
// refused rather than clamped, unlike the exposure page: a caller walking
// history with a cursor must know exactly how its request was interpreted, and
// a silently clamped page would make the cursor it computed wrong.
const (
	defaultHistoryLimit = 100
	maxHistoryLimit     = history.DefaultRetention
)

// History states. They are the same three words docs/transitions.md gives a
// comparison, applied to the history as a whole, and they carry the same rule:
// none of them is an error, and an empty or partial history must never render
// as an API failure or as silence.
const (
	// historyValid means at least one observation exists and every observation
	// returned is complete.
	historyValid = "valid"
	// historyUnknown means at least one returned observation is undetermined,
	// so it is marked and returned rather than dropped. Nothing may be read
	// from what an undetermined observation omits.
	historyUnknown = "unknown"
	// historyMissing means no observations have been recorded for the asset.
	// That is an explicit empty result, not a 404.
	historyMissing = "missing"
)

// historyResponse is the body of GET /api/v1/history.
//
// Observations are in time order, oldest first, and transitions are the derived
// difference between each consecutive pair — the view a consumer needs to ask
// "what changed", where /api/v1/scan answers "what is true now".
type historyResponse struct {
	Asset  mechanics.Asset `json:"asset"`
	State  string          `json:"state"`
	Reason string          `json:"reason,omitempty"`
	Count  int             `json:"count"`

	Observations []temporal.Observation `json:"observations"`
	Transitions  []temporal.Transition  `json:"transitions"`

	// Limit is the page size actually applied, so a caller can page without
	// guessing how its request was interpreted.
	Limit int `json:"limit"`
	// NextBefore is the cursor for the next page: the earliest returned
	// observation's time, to be sent back as ?before=. It is absent on the
	// page that reaches the oldest retained observation, and an empty page is
	// a valid end-of-history result rather than an error. A page can be larger
	// than Limit when observations share its oldest timestamp: the page never
	// splits observations taken at the same instant, because a time cursor
	// cannot resume between them.
	NextBefore *time.Time `json:"next_before,omitempty"`

	// LatestAt and LatestAgeSecs carry the freshness of the most recent
	// observation so a consumer can apply its own staleness policy. Assay does
	// not declare a history stale: the tolerance belongs to the consumer, the
	// same way max_age_secs does on-chain.
	LatestAt      *time.Time `json:"latest_at,omitempty"`
	LatestAgeSecs *int64     `json:"latest_age_secs,omitempty"`

	// Retention is the greatest number of observations kept per asset. A
	// consumer that sees exactly this many should read the history as possibly
	// truncated at the oldest end rather than as complete.
	Retention int `json:"retention"`

	// MaxAssets is the global cap on assets this store retains. A consumer of a
	// deployment that has reached it must read state "missing" on some assets
	// as "not retained" rather than "never observed" — eviction is whole-asset
	// and least-recently-observed-first. docs/history.md documents it.
	MaxAssets int `json:"max_assets"`
}

// handleHistory serves an asset's recorded observations and the transitions
// derived between them.
//
// It reads only what has been recorded; it never scans. An asset with no
// recorded history is a valid response with an empty list and state "missing",
// so a consumer can distinguish "we have never looked" from "the request
// failed" without inspecting an HTTP status.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("asset")
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, apiErr(codeMissingParam, "missing ?asset=CODE-ISSUER"))
		return
	}

	asset, err := scan.ParseAsset(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiErr(codeInvalidAsset, err.Error()))
		return
	}

	limit := defaultHistoryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxHistoryLimit {
			writeJSON(w, http.StatusBadRequest, apiErr(codeInvalidParam,
				"invalid limit: must be an integer between 1 and "+strconv.Itoa(maxHistoryLimit)))
			return
		}
		limit = n
	}

	// before is an exclusive time cursor: the page is the retained observations
	// strictly older than it, newest last. It is a timestamp rather than an
	// index because an index shifts under a concurrent append, and a paginated
	// read that skipped or repeated an observation would be a silent wrong
	// answer. An unparseable cursor is refused, never ignored.
	var before *time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiErr(codeInvalidParam,
				"invalid before: must be an RFC 3339 timestamp, e.g. 2026-09-01T00:00:00Z"))
			return
		}
		before = &t
	}

	all := []temporal.Observation{}
	retention, maxAssets := 0, 0
	if s.History != nil {
		all = s.History.List(asset)
		retention = s.History.RetentionLimit()
		maxAssets = s.History.MaxAssets()
	}

	resp := historyResponse{
		Asset:        asset,
		State:        historyValid,
		Observations: []temporal.Observation{},
		Transitions:  []temporal.Transition{},
		Retention:    retention,
		MaxAssets:    maxAssets,
		Limit:        limit,
	}

	if len(all) == 0 {
		resp.State = historyMissing
		resp.Reason = "no observations have been recorded for this asset. " +
			"GET /api/v1/scan records one per successful scan, so an asset that " +
			"has never been scanned has an empty history rather than an unknown " +
			"or failing one."
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Select the page: the end is the first observation not older than the
	// cursor, and the page is the limit-sized window ending there. A page that
	// reaches past the oldest retained observation is shorter rather than an
	// error, and an empty page is an explicit end-of-history result.
	end := len(all)
	if before != nil {
		end = sort.Search(len(all), func(i int) bool { return !all[i].At.Before(*before) })
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	// Observations sharing the page's oldest timestamp travel together. A time
	// cursor cannot resume between two observations taken at the same instant,
	// so splitting them would drop one from the walk; a page may therefore be
	// larger than limit, bounded by the per-asset retention cap.
	for start > 0 && all[start-1].At.Equal(all[start].At) {
		start--
	}
	page := all[start:end]

	// The transition *into* the page is computed against one predecessor from
	// outside it, so a change exactly at the page boundary is still reported.
	// The predecessor is not returned as an observation: count describes this
	// page, and an entry the caller did not ask for would be one it cannot
	// page over. It is included when judging completeness, because a change
	// derived from it is part of this response.
	compared := page
	if start > 0 {
		compared = append([]temporal.Observation{all[start-1]}, page...)
	}

	// Transitions are derived on demand between consecutive observations and
	// never stored; docs/transitions.md records why.
	for i := 1; i < len(compared); i++ {
		resp.Transitions = append(resp.Transitions,
			temporal.Between(compared[i-1], compared[i]))
	}

	resp.Count = len(page)
	resp.Observations = page
	if start > 0 && len(page) > 0 {
		next := page[0].At
		resp.NextBefore = &next
	}

	if undetermined, named := undeterminedChecks(compared); undetermined {
		resp.State = historyUnknown
		resp.Reason = "one or more observations are undetermined because a source " +
			"did not answer (" + named + "), so what they omit must not be read " +
			"as unchanged. They are returned marked rather than omitted."
	}

	// LatestAt is the most recent observation overall, not of this page: it
	// reports how fresh the history is, and walking backwards through pages
	// must not make the asset look progressively staler than it is.
	resp.LatestAt, resp.LatestAgeSecs = latestAge(all, time.Now().UTC())
	writeJSON(w, http.StatusOK, resp)
}

// undeterminedChecks reports whether any observation is partial and, if so,
// which checks did not complete across the whole history, deduplicated and in
// first-seen order. An undetermined observation with no named check still
// counts, so state cannot depend on how loudly a scan failed.
func undeterminedChecks(observations []temporal.Observation) (bool, string) {
	var checks []string
	seen := map[string]bool{}
	anyUndetermined := false
	for _, o := range observations {
		if !o.Undetermined {
			continue
		}
		anyUndetermined = true
		for _, c := range o.UndeterminedChecks {
			if !seen[c] {
				seen[c] = true
				checks = append(checks, c)
			}
		}
	}
	if !anyUndetermined {
		return false, ""
	}
	if len(checks) == 0 {
		return true, "an unnamed check"
	}
	return true, strings.Join(checks, ", ")
}

// latestAge returns the most recent observation's time and its age, so a
// consumer can judge staleness itself. A zero timestamp is reported as absent
// rather than as an age measured from the zero instant.
func latestAge(observations []temporal.Observation, now time.Time) (*time.Time, *int64) {
	latest := observations[len(observations)-1].At
	if latest.IsZero() {
		return nil, nil
	}
	age := int64(now.Sub(latest).Seconds())
	if age < 0 {
		// A clock skew between the writer and the reader must not print a
		// negative age, which would read as "in the future" and confuse any
		// downstream comparison.
		age = 0
	}
	return &latest, &age
}
