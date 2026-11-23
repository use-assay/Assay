package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/api"
	"github.com/use-assay/assay/internal/history"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/temporal"
)

const (
	aquaCode   = "AQUA"
	aquaIssuer = "GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA"
	aquaID     = aquaCode + "-" + aquaIssuer
)

// historyBody is the subset of the response these tests read. It is declared
// here rather than reusing the server's unexported type on purpose: the test
// should fail if the wire shape changes, not track it automatically.
type historyBody struct {
	Asset struct {
		Code   string `json:"code"`
		Issuer string `json:"issuer"`
	} `json:"asset"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`

	Observations []struct {
		At                 time.Time `json:"at"`
		Mechanics          int       `json:"mechanics"`
		BaseSeverity       string    `json:"base_severity"`
		Severity           string    `json:"severity"`
		Undetermined       bool      `json:"undetermined"`
		UndeterminedChecks []string  `json:"undetermined_checks"`
	} `json:"observations"`

	Transitions []struct {
		From struct {
			At time.Time `json:"at"`
		} `json:"from"`
		To struct {
			At time.Time `json:"at"`
		} `json:"to"`
		State          string   `json:"state"`
		Reason         string   `json:"reason"`
		AddedNames     []string `json:"added_names"`
		RemovedNames   []string `json:"removed_names"`
		Attribution    string   `json:"attribution"`
		SeverityBefore string   `json:"severity_before"`
		SeverityAfter  string   `json:"severity_after"`
		Accountability struct {
			From    string `json:"from"`
			To      string `json:"to"`
			Changed bool   `json:"changed"`
			State   string `json:"state"`
		} `json:"accountability"`
	} `json:"transitions"`

	Limit      int        `json:"limit"`
	NextBefore *time.Time `json:"next_before"`

	LatestAt      *time.Time `json:"latest_at"`
	LatestAgeSecs *int64     `json:"latest_age_secs"`
	Retention     int        `json:"retention"`
	MaxAssets     int        `json:"max_assets"`
}

func newHistoryServer() *api.Server {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.NewServer(log)
}

func historyAt(day int) time.Time { return time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC) }

// historyObs builds a complete observation for AQUA at a given day.
func historyObs(day int, bits mechanics.Mechanic, base, sev mechanics.Severity) temporal.Observation {
	return temporal.Observation{
		Asset:     mechanics.Asset{Code: aquaCode, Issuer: aquaIssuer},
		At:        historyAt(day),
		Mechanics: bits,
		Base:      base,
		Severity:  sev,
	}
}

func getHistory(t *testing.T, srv *api.Server) (*httptest.ResponseRecorder, historyBody) {
	t.Helper()
	return getHistoryQuery(t, srv, "asset="+aquaID)
}

func getHistoryQuery(t *testing.T, srv *api.Server, query string) (*httptest.ResponseRecorder, historyBody) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?"+query, nil)
	srv.Handler().ServeHTTP(rec, req)

	var body historyBody
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode history: %v\nbody: %s", err, rec.Body.String())
		}
	}
	return rec, body
}

func mustAppend(t *testing.T, srv *api.Server, o temporal.Observation) {
	t.Helper()
	if err := srv.History.Append(o); err != nil {
		t.Fatalf("Append: %v", err)
	}
}

// No observations is an explicit empty result: HTTP 200, state "missing", a
// reason, and empty (not null) arrays. It must never be a 404 or a bare 200
// with nothing in it, because a consumer has to tell "we have never looked"
// apart from "the request failed".
func TestHistoryEmpty(t *testing.T) {
	srv := newHistoryServer()
	rec, body := getHistory(t, srv)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; an empty history is a result, not an error", rec.Code)
	}
	if body.State != "missing" {
		t.Errorf("state = %q, want %q", body.State, "missing")
	}
	if body.Reason == "" {
		t.Error("empty history carried no reason; it is then indistinguishable from a silent failure")
	}
	if body.Count != 0 {
		t.Errorf("count = %d, want 0", body.Count)
	}
	if body.Observations == nil {
		t.Error("observations decoded to nil, so the body rendered null instead of []")
	}
	if len(body.Observations) != 0 {
		t.Errorf("observations = %d entries, want 0", len(body.Observations))
	}
	if body.Transitions == nil {
		t.Error("transitions decoded to nil, so the body rendered null instead of []")
	}
	if body.Asset.Code != aquaCode || body.Asset.Issuer != aquaIssuer {
		t.Errorf("asset not echoed: %+v", body.Asset)
	}
}

// Bad input fails before any store read, like the scan endpoint.
func TestHistoryRejectsBadInput(t *testing.T) {
	for _, tc := range []struct{ name, query string }{
		{"missing asset", "/api/v1/history"},
		{"empty asset", "/api/v1/history?asset="},
		{"not an asset", "/api/v1/history?asset=hello"},
		{"bad issuer", "/api/v1/history?asset=AQUA-NOPE"},
		// The native asset is refused with the same explicit explanation the
		// scan endpoint gives; docs/api.md documents both.
		{"native asset", "/api/v1/history?asset=native"},
		{"XLM", "/api/v1/history?asset=XLM"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newHistoryServer().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.query, nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

// A single observation is a history of one. It is "valid" — nothing about it is
// missing or undetermined — but it yields no transition, because a transition
// is the difference between two points and there is only one.
func TestHistorySingleObservation(t *testing.T) {
	srv := newHistoryServer()
	mustAppend(t, srv, historyObs(1, 0, mechanics.Clear, mechanics.Clear))

	rec, body := getHistory(t, srv)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body.State != "valid" {
		t.Errorf("state = %q, want %q", body.State, "valid")
	}
	if body.Count != 1 || len(body.Observations) != 1 {
		t.Fatalf("got %d observations, want 1", len(body.Observations))
	}
	if len(body.Transitions) != 0 {
		t.Errorf("a single observation produced %d transitions, want 0", len(body.Transitions))
	}
	if body.LatestAt == nil || !body.LatestAt.Equal(historyAt(1)) {
		t.Errorf("latest_at = %v, want %v", body.LatestAt, historyAt(1))
	}
	if body.LatestAgeSecs == nil {
		t.Error("latest_age_secs missing; a consumer cannot judge staleness without it")
	}
	if body.Retention != history.DefaultRetention {
		t.Errorf("retention = %d, want %d", body.Retention, history.DefaultRetention)
	}
}

// Two observations produce one derived transition, in time order, attributed to
// the axis that moved.
func TestHistoryMultipleWithTransition(t *testing.T) {
	srv := newHistoryServer()
	first := historyObs(1, 0, mechanics.Clear, mechanics.Clear)
	first.Accountability = mechanics.AccountabilityVerified
	second := historyObs(2, mechanics.MechClawbackEnabled, mechanics.High, mechanics.High)
	second.Accountability = mechanics.AccountabilityUnverified
	mustAppend(t, srv, first)
	mustAppend(t, srv, second)

	rec, body := getHistory(t, srv)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body.State != "valid" {
		t.Errorf("state = %q, want %q", body.State, "valid")
	}
	if len(body.Observations) != 2 {
		t.Fatalf("got %d observations, want 2", len(body.Observations))
	}
	if !body.Observations[0].At.Equal(historyAt(1)) || !body.Observations[1].At.Equal(historyAt(2)) {
		t.Errorf("observations are not in time order: %v", body.Observations)
	}
	if len(body.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(body.Transitions))
	}

	tr := body.Transitions[0]
	if tr.State != "valid" {
		t.Errorf("transition state = %q, want %q (%s)", tr.State, "valid", tr.Reason)
	}
	if !contains(tr.AddedNames, "auth_clawback_enabled") {
		t.Errorf("added_names = %v, want the clawback capability", tr.AddedNames)
	}
	if len(tr.RemovedNames) != 0 {
		t.Errorf("removed_names = %v, want none", tr.RemovedNames)
	}
	if tr.Attribution != "capability" {
		t.Errorf("attribution = %q, want %q", tr.Attribution, "capability")
	}
	if tr.SeverityBefore != "clear" || tr.SeverityAfter != "high" {
		t.Errorf("severity moved %q -> %q, want clear -> high", tr.SeverityBefore, tr.SeverityAfter)
	}
	if tr.Accountability.State != "valid" || tr.Accountability.From != "verified" ||
		tr.Accountability.To != "unverified" || !tr.Accountability.Changed {
		t.Errorf("accountability = %+v, want valid verified -> unverified", tr.Accountability)
	}
}

// An undetermined observation is returned marked, not dropped, and it makes the
// whole history "unknown": a source that did not answer could have hidden
// exactly the change being asked about, so no transition may be derived from it.
func TestHistoryUndeterminedObservation(t *testing.T) {
	srv := newHistoryServer()
	mustAppend(t, srv, historyObs(1, 0, mechanics.Clear, mechanics.Clear))

	partial := historyObs(2, 0, mechanics.Clear, mechanics.Clear)
	partial.Undetermined = true
	partial.UndeterminedChecks = []string{"reputation"}
	mustAppend(t, srv, partial)

	rec, body := getHistory(t, srv)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body.State != "unknown" {
		t.Errorf("state = %q, want %q", body.State, "unknown")
	}
	if body.Reason == "" {
		t.Error("unknown history carried no reason")
	}
	if body.Count != 2 {
		t.Fatalf("count = %d, want the undetermined observation included", body.Count)
	}
	if !body.Observations[1].Undetermined {
		t.Error("undetermined observation was not marked")
	}
	if !contains(body.Observations[1].UndeterminedChecks, "reputation") {
		t.Errorf("undetermined_checks = %v, want reputation", body.Observations[1].UndeterminedChecks)
	}
	if len(body.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(body.Transitions))
	}
	if body.Transitions[0].State != "unknown" {
		t.Errorf("transition state = %q, want %q; a partial observation cannot yield a derived change",
			body.Transitions[0].State, "unknown")
	}
	if body.Transitions[0].Accountability.State != "unknown" ||
		body.Transitions[0].Accountability.Changed {
		t.Errorf("accountability = %+v, want unknown with no change on a partial pair",
			body.Transitions[0].Accountability)
	}
}

// Arrival order must not leak into the response: history is ordered by time.
func TestHistoryOrdersObservationsByTime(t *testing.T) {
	srv := newHistoryServer()
	mustAppend(t, srv, historyObs(3, 0, mechanics.Clear, mechanics.Clear))
	mustAppend(t, srv, historyObs(1, 0, mechanics.Clear, mechanics.Clear))
	mustAppend(t, srv, historyObs(2, 0, mechanics.Clear, mechanics.Clear))

	_, body := getHistory(t, srv)
	if len(body.Observations) != 3 {
		t.Fatalf("got %d observations, want 3", len(body.Observations))
	}
	for i, want := range []int{1, 2, 3} {
		if !body.Observations[i].At.Equal(historyAt(want)) {
			t.Fatalf("observation %d at %v, want day %d", i, body.Observations[i].At, want)
		}
	}
}

// Pagination is bounded and backwards: a page is the latest `limit`
// observations not newer than the `before` cursor, and the transition at a page
// boundary is derived against one predecessor from outside the page, so a
// change landing exactly on the boundary is still reported.
func TestHistoryPaginationWalksTheHistory(t *testing.T) {
	srv := newHistoryServer()
	for day := 1; day <= 5; day++ {
		mustAppend(t, srv, historyObs(day, 0, mechanics.Clear, mechanics.Clear))
	}

	// Page 1: the newest two observations, with both transitions — including
	// the one from the predecessor that is not itself returned.
	rec, first := getHistoryQuery(t, srv, "asset="+aquaID+"&limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if first.Limit != 2 {
		t.Errorf("limit = %d, want the applied page size 2", first.Limit)
	}
	if first.Count != 2 || len(first.Observations) != 2 {
		t.Fatalf("page returned %d observations, want 2", len(first.Observations))
	}
	if !first.Observations[0].At.Equal(historyAt(4)) || !first.Observations[1].At.Equal(historyAt(5)) {
		t.Errorf("page spans %v..%v, want the two newest observations",
			first.Observations[0].At, first.Observations[1].At)
	}
	if len(first.Transitions) != 2 {
		t.Fatalf("got %d transitions, want 2: one at the boundary plus one inside the page",
			len(first.Transitions))
	}
	if !first.Transitions[0].From.At.Equal(historyAt(3)) || !first.Transitions[0].To.At.Equal(historyAt(4)) {
		t.Errorf("boundary transition compares %v -> %v, want day 3 -> day 4",
			first.Transitions[0].From.At, first.Transitions[0].To.At)
	}
	if first.NextBefore == nil || !first.NextBefore.Equal(historyAt(4)) {
		t.Fatalf("next_before = %v, want the earliest returned observation %v", first.NextBefore, historyAt(4))
	}
	if first.MaxAssets != history.DefaultMaxAssets {
		t.Errorf("max_assets = %d, want %d", first.MaxAssets, history.DefaultMaxAssets)
	}

	// Page 2: the cursor is exclusive, so day 4 is not returned twice.
	rec, second := getHistoryQuery(t, srv,
		"asset="+aquaID+"&limit=2&before="+first.NextBefore.UTC().Format(time.RFC3339))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if second.Count != 2 {
		t.Fatalf("second page returned %d observations, want 2", second.Count)
	}
	if !second.Observations[0].At.Equal(historyAt(2)) || !second.Observations[1].At.Equal(historyAt(3)) {
		t.Errorf("second page spans %v..%v, want day 2..day 3",
			second.Observations[0].At, second.Observations[1].At)
	}
	if len(second.Transitions) != 2 {
		t.Fatalf("second page has %d transitions, want the boundary plus one", len(second.Transitions))
	}
	if second.Transitions[0].From.At.IsZero() {
		t.Error("second page dropped the predecessor needed for its boundary transition")
	}
	if second.NextBefore == nil || !second.NextBefore.Equal(historyAt(2)) {
		t.Fatalf("next_before = %v, want %v", second.NextBefore, historyAt(2))
	}

	// Page 3: the last page is short and carries no cursor.
	rec, third := getHistoryQuery(t, srv,
		"asset="+aquaID+"&limit=2&before="+second.NextBefore.UTC().Format(time.RFC3339))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if third.Count != 1 || len(third.Observations) != 1 {
		t.Fatalf("last page returned %d observations, want the one remaining", third.Count)
	}
	if third.NextBefore != nil {
		t.Errorf("next_before = %v on the last page, want it absent", third.NextBefore)
	}
	// Day 1 is the oldest retained observation, so there is no predecessor to
	// compare it against and no transition to report.
	if len(third.Transitions) != 0 {
		t.Errorf("last page has %d transitions, want 0: nothing precedes the oldest observation",
			len(third.Transitions))
	}
}

// A page never splits observations that share a timestamp: a time cursor cannot
// resume between them, so splitting would drop one from the walk. The page is
// allowed to exceed limit rather than lose an observation.
func TestHistoryPaginationDoesNotSplitSameInstant(t *testing.T) {
	srv := newHistoryServer()
	mustAppend(t, srv, historyObs(1, 0, mechanics.Clear, mechanics.Clear))
	mustAppend(t, srv, historyObs(2, 0, mechanics.Clear, mechanics.Clear))
	mustAppend(t, srv, historyObs(2, mechanics.MechAuthRevocable, mechanics.Medium, mechanics.Medium))
	mustAppend(t, srv, historyObs(3, 0, mechanics.Clear, mechanics.Clear))

	type observed struct {
		at   time.Time
		bits int
	}
	seen := map[observed]int{}
	cursor := ""
	pages := 0
	for {
		query := "asset=" + aquaID + "&limit=1"
		if cursor != "" {
			query += "&before=" + cursor
		}
		rec, body := getHistoryQuery(t, srv, query)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		pages++
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
		for _, o := range body.Observations {
			seen[observed{o.At, o.Mechanics}]++
		}
		if body.NextBefore == nil {
			break
		}
		cursor = body.NextBefore.UTC().Format(time.RFC3339Nano)
	}

	if len(seen) != 4 {
		t.Fatalf("walk returned %d distinct observations, want all 4: %v", len(seen), seen)
	}
	for o, n := range seen {
		if n != 1 {
			t.Errorf("observation at %v (bits %d) was returned %d times, want exactly once",
				o.at, o.bits, n)
		}
	}
}

// A cursor older than every retained observation is an empty page, not an
// error: the walk has simply reached the end of what is retained.
func TestHistoryPaginationPastTheOldestIsEmpty(t *testing.T) {
	srv := newHistoryServer()
	mustAppend(t, srv, historyObs(2, 0, mechanics.Clear, mechanics.Clear))
	mustAppend(t, srv, historyObs(3, 0, mechanics.Clear, mechanics.Clear))

	before := historyAt(1).UTC().Format(time.RFC3339)
	rec, body := getHistoryQuery(t, srv, "asset="+aquaID+"&before="+before)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body.Count != 0 || len(body.Observations) != 0 {
		t.Fatalf("got %d observations, want an empty page", body.Count)
	}
	if body.Observations == nil {
		t.Error("observations decoded to nil, so an empty page rendered null instead of []")
	}
	if body.State != "valid" {
		t.Errorf("state = %q, want valid: the history exists, this page is just empty", body.State)
	}
	if body.NextBefore != nil {
		t.Errorf("next_before = %v, want it absent on an empty page", body.NextBefore)
	}
}

// latest_at describes the history, not the page: walking backwards must not
// make an asset look staler than its most recent observation.
func TestHistoryPaginationReportsOverallFreshness(t *testing.T) {
	srv := newHistoryServer()
	for day := 1; day <= 3; day++ {
		mustAppend(t, srv, historyObs(day, 0, mechanics.Clear, mechanics.Clear))
	}

	before := historyAt(3).UTC().Format(time.RFC3339)
	_, body := getHistoryQuery(t, srv, "asset="+aquaID+"&limit=1&before="+before)
	if body.LatestAt == nil || !body.LatestAt.Equal(historyAt(3)) {
		t.Errorf("latest_at = %v, want the overall latest observation %v", body.LatestAt, historyAt(3))
	}
}

// The default page size is documented and applied when no limit is sent.
func TestHistoryDefaultLimit(t *testing.T) {
	srv := newHistoryServer()
	for day := 1; day <= 3; day++ {
		mustAppend(t, srv, historyObs(day, 0, mechanics.Clear, mechanics.Clear))
	}

	_, body := getHistory(t, srv)
	if body.Limit != 100 {
		t.Errorf("limit = %d, want the documented default 100", body.Limit)
	}
	if body.Count != 3 {
		t.Errorf("count = %d, want all 3 observations within the default page", body.Count)
	}
}

// Invalid pagination parameters are refused with 400 before any store read,
// not silently clamped or ignored.
func TestHistoryRejectsInvalidPaginationParameters(t *testing.T) {
	for _, tc := range []struct{ name, query string }{
		{"zero limit", "limit=0"},
		{"negative limit", "limit=-1"},
		{"non-numeric limit", "limit=all"},
		{"limit above the maximum", "limit=257"},
		{"unparseable before", "before=yesterday"},
		{"before without a zone", "before=2026-09-01T00:00:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, _ := getHistoryQuery(t, newHistoryServer(), "asset="+aquaID+"&"+tc.query)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 for %s", rec.Code, tc.query)
			}
		})
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
