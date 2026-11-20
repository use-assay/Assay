package history_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/history"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/temporal"
)

const (
	aquaIssuer = "GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA"
	usdcIssuer = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
)

func at(day int) time.Time { return time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC) }

func obs(code, issuer string, t time.Time, bits mechanics.Mechanic) temporal.Observation {
	return temporal.Observation{
		Asset:     mechanics.Asset{Code: code, Issuer: issuer},
		At:        t,
		Mechanics: bits,
		Severity:  mechanics.Clear,
	}
}

// An empty store returns a non-nil, empty slice so the caller cannot mistake
// "no observations" for a nil-marshalled null.
func TestStoreEmpty(t *testing.T) {
	s := history.New()
	got := s.List(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer})
	if got == nil {
		t.Fatal("List returned nil; an empty history must be an empty slice, not null")
	}
	if len(got) != 0 {
		t.Fatalf("List returned %d observations on a fresh store", len(got))
	}
}

func TestStoreAppendAndListAreOrderedByTime(t *testing.T) {
	s := history.New()
	// Appended newest-first on purpose: the store must order by time, not by
	// arrival, because a history read that inherited arrival order would invert
	// a transition direction.
	mustAppend(t, s, obs("AQUA", aquaIssuer, at(3), mechanics.MechClawbackEnabled))
	mustAppend(t, s, obs("AQUA", aquaIssuer, at(1), 0))
	mustAppend(t, s, obs("AQUA", aquaIssuer, at(2), mechanics.MechAuthRevocable))

	got := s.List(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer})
	if len(got) != 3 {
		t.Fatalf("List returned %d observations, want 3", len(got))
	}
	for i, want := range []int{1, 2, 3} {
		if got[i].At != at(want) {
			t.Fatalf("observation %d at %v, want day %d; history is not in time order", i, got[i].At, want)
		}
	}
}

func TestStoreKeepsAssetsSeparate(t *testing.T) {
	s := history.New()
	mustAppend(t, s, obs("AQUA", aquaIssuer, at(1), 0))
	mustAppend(t, s, obs("USDC", usdcIssuer, at(1), mechanics.MechAuthRevocable))

	if n := s.Len(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer}); n != 1 {
		t.Fatalf("AQUA has %d observations, want 1", n)
	}
	got := s.List(mechanics.Asset{Code: "USDC", Issuer: usdcIssuer})
	if len(got) != 1 || got[0].Asset.Code != "USDC" {
		t.Fatalf("USDC history = %+v, want a single USDC observation", got)
	}
}

// The returned slice is a copy: mutating it must not edit the store.
func TestStoreListReturnsCopy(t *testing.T) {
	s := history.New()
	mustAppend(t, s, obs("AQUA", aquaIssuer, at(1), 0))

	got := s.List(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer})
	got[0].Mechanics = mechanics.MechClawbackEnabled

	again := s.List(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer})
	if again[0].Mechanics != 0 {
		t.Fatal("mutating the returned slice edited the stored observation")
	}
}

// Retention drops the oldest observation first and never grows past the bound.
func TestStoreRetentionDropsOldest(t *testing.T) {
	s := history.NewWithRetention(2)
	mustAppend(t, s, obs("AQUA", aquaIssuer, at(1), 0))
	mustAppend(t, s, obs("AQUA", aquaIssuer, at(2), 0))
	mustAppend(t, s, obs("AQUA", aquaIssuer, at(3), 0))

	got := s.List(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer})
	if len(got) != 2 {
		t.Fatalf("retention kept %d observations, want 2", len(got))
	}
	if got[0].At != at(2) || got[1].At != at(3) {
		t.Fatalf("retention kept %v..%v, want the two newest observations", got[0].At, got[1].At)
	}
}

// A file-backed store survives a reopen with the retained set intact, order and
// all. Persistence is the point of the file backend; a round trip that lost
// order would be worse than no persistence at all.
func TestStoreFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "history.jsonl")

	first, err := history.Open(path)
	if err != nil {
		t.Fatalf("Open on a missing file: %v", err)
	}
	mustAppend(t, first, obs("AQUA", aquaIssuer, at(2), mechanics.MechAuthRevocable))
	mustAppend(t, first, obs("AQUA", aquaIssuer, at(1), 0))
	mustAppend(t, first, obs("USDC", usdcIssuer, at(1), 0))
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := history.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := second.List(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer})
	if len(got) != 2 {
		t.Fatalf("reopened store has %d AQUA observations, want 2", len(got))
	}
	if got[0].At != at(1) || got[1].At != at(2) {
		t.Fatalf("reopened store lost time order: %v, %v", got[0].At, got[1].At)
	}
	if n := second.Len(mechanics.Asset{Code: "USDC", Issuer: usdcIssuer}); n != 1 {
		t.Fatalf("reopened store has %d USDC observations, want 1", n)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// Retention must hold across a reopen: the log on disk is exactly the retained
// set, so a restart can never resurrect an observation the running process had
// already evicted.
func TestStoreFileRetentionPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	s, err := history.OpenWithRetention(path, 2)
	if err != nil {
		t.Fatalf("OpenWithRetention: %v", err)
	}
	for day := 1; day <= 3; day++ {
		mustAppend(t, s, obs("AQUA", aquaIssuer, at(day), 0))
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := history.OpenWithRetention(path, 2)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := reopened.List(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer})
	if len(got) != 2 {
		t.Fatalf("reopened with %d observations, want the 2 retained ones", len(got))
	}
	if got[0].At != at(2) || got[1].At != at(3) {
		t.Fatalf("reopen resurrected an evicted observation: %v, %v", got[0].At, got[1].At)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("log file was not written: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// Two writers on one path must not silently drop each other's observations:
// the second Open refuses with a clear, matchable error, and ownership returns
// to a later caller once the first store is closed.
func TestOpenRefusesSecondWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")

	first, err := history.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	second, err := history.Open(path)
	if !errors.Is(err, history.ErrLocked) {
		t.Fatalf("second Open error = %v, want ErrLocked", err)
	}
	if second != nil {
		t.Fatal("second Open returned a store despite the lock")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	third, err := history.Open(path)
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	if err := third.Append(obs("AQUA", aquaIssuer, at(1), 0)); err != nil {
		t.Fatalf("Append after acquiring lock: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := third.Append(obs("AQUA", aquaIssuer, at(2), 0)); !errors.Is(err, history.ErrClosed) {
		t.Fatalf("Append after Close = %v, want ErrClosed", err)
	}
}

// An in-memory store takes no OS lock, so two such stores can coexist.
func TestInMemoryStoresDoNotLock(t *testing.T) {
	first, second := history.New(), history.New()
	mustAppend(t, first, obs("AQUA", aquaIssuer, at(1), 0))
	mustAppend(t, second, obs("AQUA", aquaIssuer, at(2), 0))
	if err := first.Close(); err != nil {
		t.Fatalf("Close in-memory: %v", err)
	}
}

// Accountability is part of an observation and must survive a log round trip
// or the history API cannot derive an accountability movement from it.
func TestStoreFilePersistsAccountability(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	s, err := history.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	o := obs("AQUA", aquaIssuer, at(1), 0)
	o.Accountability = mechanics.AccountabilityVerified
	mustAppend(t, s, o)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := history.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	got := reopened.List(mechanics.Asset{Code: "AQUA", Issuer: aquaIssuer})
	if len(got) != 1 || got[0].Accountability != mechanics.AccountabilityVerified {
		t.Fatalf("accountability lost on reopen: %+v", got)
	}
}

func mustAppend(t *testing.T, s *history.Store, o temporal.Observation) {
	t.Helper()
	if err := s.Append(o); err != nil {
		t.Fatalf("Append: %v", err)
	}
}
