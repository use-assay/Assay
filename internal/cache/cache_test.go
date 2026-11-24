package cache_test

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/cache"
)

// newClock returns a clock the test drives by hand, so expiry is exercised by
// arithmetic rather than by sleeping.
func newClock(start time.Time) *clock {
	return &clock{now: start}
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var t0 = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func TestMissOnEmpty(t *testing.T) {
	c := cache.New[string](time.Minute, cache.WithClock(newClock(t0).Now))
	if _, ok := c.Get("absent"); ok {
		t.Fatal("an empty cache reported a hit")
	}
}

// The property the whole package exists for: a hit returns the fetch time of
// the value, not the time of the lookup. A cache that re-stamped it would make
// Evidence.RetrievedAt claim a freshness the data does not have.
func TestHitPreservesOriginalFetchTime(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[string](time.Hour, cache.WithClock(clk.Now))

	c.Put("k", cache.Entry[string]{
		Value:     "v",
		FetchedAt: t0.Add(-30 * time.Minute),
	})
	clk.Advance(10 * time.Minute) // the lookup happens later than the fetch

	e, ok := c.Get("k")
	if !ok {
		t.Fatal("a live entry was not served")
	}
	if e.Value != "v" {
		t.Fatalf("Value = %q, want %q", e.Value, "v")
	}
	want := t0.Add(-30 * time.Minute)
	if !e.FetchedAt.Equal(want) {
		t.Fatalf("FetchedAt = %s, want the original fetch time %s",
			e.FetchedAt.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// Expiry is measured from the fetch time, not insertion. A value fetched well
// before it was stored is already partly stale.
func TestExpiryIsMeasuredFromFetchTime(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[string](10*time.Minute, cache.WithClock(clk.Now))

	// Fetched 9 minutes before "now", so only a minute of TTL remains.
	c.Put("k", cache.Entry[string]{Value: "v", FetchedAt: t0.Add(-9 * time.Minute)})

	clk.Advance(30 * time.Second)
	if _, ok := c.Get("k"); !ok {
		t.Fatal("an unexpired entry was reported as a miss")
	}

	clk.Advance(61 * time.Second) // total age 10m1s from the fetch
	if _, ok := c.Get("k"); ok {
		t.Fatal("an entry past its TTL was served; expiry must be a miss")
	}
}

// An expired entry is gone, not merely hidden: a later Put of the same key has
// nothing to compare against, and a re-fetch is the only way to get an answer.
func TestExpiredEntryIsDiscarded(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[string](time.Minute, cache.WithClock(clk.Now))
	c.Put("k", cache.Entry[string]{Value: "old", FetchedAt: t0})

	clk.Advance(2 * time.Minute)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expired entry served")
	}
	if n := c.Len(); n != 0 {
		t.Fatalf("expired entry left in the map: Len = %d, want 0", n)
	}
}

// Zero and negative TTLs disarm the cache. Every lookup must miss, so a
// deployment that disables caching cannot accidentally serve a cached answer.
func TestNonPositiveTTLDisarmsTheCache(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Minute} {
		c := cache.New[string](ttl)
		c.Put("k", cache.Entry[string]{Value: "v", FetchedAt: t0})
		if _, ok := c.Get("k"); ok {
			t.Errorf("ttl %s: disarmed cache served an entry", ttl)
		}
		if n := c.Len(); n != 0 {
			t.Errorf("ttl %s: disarmed cache stored an entry: Len = %d", ttl, n)
		}
	}
}

// A cached "negative" answer is a real answer and must be cached like any
// other, then re-fetched once it expires — never assumed to still hold.
func TestNegativeAnswerExpiresAndIsNotAssumed(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[[]string](time.Minute, cache.WithClock(clk.Now))
	// An empty slice is the negative answer: "the source said nothing useful".
	c.Put("k", cache.Entry[[]string]{Value: nil, FetchedAt: t0})

	if _, ok := c.Get("k"); !ok {
		t.Fatal("a cached negative answer was not served within its TTL")
	}
	clk.Advance(2 * time.Minute)
	if _, ok := c.Get("k"); ok {
		t.Fatal("an expired negative answer was served; it must re-fetch")
	}
}

// An entry Put without a fetch time gets one, so it can never outlive every
// TTL. This is the belt to the expiry measurement's braces.
func TestPutStampsMissingFetchTime(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[string](time.Minute, cache.WithClock(clk.Now))
	c.Put("k", cache.Entry[string]{Value: "v"})

	clk.Advance(30 * time.Second)
	if _, ok := c.Get("k"); !ok {
		t.Fatal("entry stamped with the cache clock was reported expired too early")
	}
	clk.Advance(31 * time.Second)
	if _, ok := c.Get("k"); ok {
		t.Fatal("entry stamped with a zero fetch time never expired")
	}
}

// Concurrency: the scanner serving parallel requests shares one cache, and
// `make test` runs with -race. This exercises concurrent readers, writers and
// expiry on the same keys.
func TestConcurrentGetPut(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[int](time.Hour, cache.WithClock(clk.Now))

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := strconv.Itoa(i % 16)
				if e, ok := c.Get(key); ok {
					if e.Value < 0 {
						t.Errorf("impossible value %d", e.Value)
					}
					continue
				}
				c.Put(key, cache.Entry[int]{Value: i, FetchedAt: clk.Now()})
			}
		}(w)
	}
	wg.Wait()
}

// Single-flight: concurrent GetOrFetch for the same absent key runs fetch once.
func TestSingleFlightDeduplicatesConcurrentFetches(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[string](time.Hour, cache.WithClock(clk.Now))

	fetchCount := 0
	var fetchMu sync.Mutex

	fetch := func() (cache.Entry[string], error) {
		fetchMu.Lock()
		fetchCount++
		fetchMu.Unlock()
		// Simulate a slow fetch.
		time.Sleep(10 * time.Millisecond)
		return cache.Entry[string]{Value: "fetched", FetchedAt: clk.Now()}, nil
	}

	var wg sync.WaitGroup
	results := make(chan cache.Entry[string], 8)
	errors := make(chan error, 8)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry, fromCache, err := c.GetOrFetch("same-key", fetch)
			if err != nil {
				errors <- err
				return
			}
			results <- entry
			_ = fromCache // we don't assert on fromCache here, just that all succeed
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	// All 8 callers should have succeeded.
	for err := range errors {
		t.Fatalf("unexpected error: %v", err)
	}

	count := 0
	var fetchedAt time.Time
	for entry := range results {
		count++
		if entry.Value != "fetched" {
			t.Errorf("got value %q, want %q", entry.Value, "fetched")
		}
		if fetchedAt.IsZero() {
			fetchedAt = entry.FetchedAt
		} else if !entry.FetchedAt.Equal(fetchedAt) {
			t.Errorf("waiters received different FetchedAt: %v vs %v", entry.FetchedAt, fetchedAt)
		}
	}

	if count != 8 {
		t.Fatalf("expected 8 results, got %d", count)
	}

	fetchMu.Lock()
	if fetchCount != 1 {
		t.Fatalf("fetch ran %d times, want 1 (single-flight deduplication)", fetchCount)
	}
	fetchMu.Unlock()
}

// Single-flight: errors are shared but not cached.
func TestSingleFlightErrorNotCached(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[string](time.Hour, cache.WithClock(clk.Now))

	fetchCount := 0
	var fetchMu sync.Mutex

	fetch := func() (cache.Entry[string], error) {
		fetchMu.Lock()
		fetchCount++
		fetchMu.Unlock()
		time.Sleep(5 * time.Millisecond) // simulate network round-trip for error
		return cache.Entry[string]{}, fmt.Errorf("upstream error")
	}

	// First concurrent batch: all should get the same error.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := c.GetOrFetch("error-key", fetch)
			if err == nil {
				t.Error("expected error, got nil")
			}
		}()
	}
	wg.Wait()

	// Cache should still be empty for this key (error not cached).
	if _, ok := c.Get("error-key"); ok {
		t.Fatal("error was cached; cache should remain empty after failed fetch")
	}

	// Second batch: fetch should run again.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := c.GetOrFetch("error-key", fetch)
			if err == nil {
				t.Error("expected error, got nil")
			}
		}()
	}
	wg.Wait()

	fetchMu.Lock()
	if fetchCount != 2 {
		t.Fatalf("fetch ran %d times, want 2 (error not cached, so retry runs fetch again)", fetchCount)
	}
	fetchMu.Unlock()
}

// Single-flight: cache hit returns fromCache=true, waiters also get fromCache=true.
func TestSingleFlightFromCacheReporting(t *testing.T) {
	clk := newClock(t0)
	c := cache.New[string](time.Hour, cache.WithClock(clk.Now))

	// Pre-populate cache.
	c.Put("cached-key", cache.Entry[string]{Value: "cached", FetchedAt: t0})

	entry, fromCache, err := c.GetOrFetch("cached-key", func() (cache.Entry[string], error) {
		t.Fatal("fetch should not be called for cached key")
		return cache.Entry[string]{}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fromCache {
		t.Error("cache hit should report fromCache=true")
	}
	if entry.Value != "cached" {
		t.Errorf("got %q, want %q", entry.Value, "cached")
	}

	// Now test single-flight waiters.
	fetchCount := 0
	fetch := func() (cache.Entry[string], error) {
		fetchCount++
		time.Sleep(10 * time.Millisecond)
		return cache.Entry[string]{Value: "fetched", FetchedAt: clk.Now()}, nil
	}

	var wg sync.WaitGroup
	fromCacheResults := make(chan bool, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, fromCache, err := c.GetOrFetch("wait-key", fetch)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			fromCacheResults <- fromCache
		}()
	}
	wg.Wait()
	close(fromCacheResults)

	// The first caller (fetcher) gets fromCache=false, waiters get fromCache=true.
	fromCacheCount := 0
	for fc := range fromCacheResults {
		if fc {
			fromCacheCount++
		}
	}
	if fromCacheCount != 3 {
		t.Errorf("expected 3 waiters with fromCache=true, got %d", fromCacheCount)
	}
	if fetchCount != 1 {
		t.Fatalf("fetch ran %d times, want 1", fetchCount)
	}
}
