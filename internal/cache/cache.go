// Package cache is a small, concurrency-safe TTL cache for consumed signals.
//
// It exists for one reason. Assay consumes free, third-party-curated data sets
// (StellarExpert's directory and blocklist) on every scan, and re-fetching them
// on every lookup is both slow and discourteous to a service that can
// rate-limit — a rate-limit would degrade scans exactly when the tool is most
// used. Caching them is the easy part. The part worth being careful about is
// time, which is why this cache stores each value together with the instant it
// was fetched from upstream and hands that instant back on every hit.
//
// Evidence.RetrievedAt is a claim about when data was fetched. A cache that
// stamped the hit time would make Assay assert a freshness it does not have, in
// reports and in evidence_hash preimages. A hit here never re-stamps: the
// caller gets the original fetch time and decides what to say about it.
//
// Single-flight: GetOrFetch deduplicates concurrent fetches for the same key.
// The first caller runs the fetch; waiters receive the same Entry (value and
// FetchedAt) or the same error. Errors are not cached — a failed fetch is
// returned to all waiters but the cache remains empty for that key, so the
// next lookup retries.
package cache

import (
	"sync"
	"time"
)

// Entry is one cached value together with the instant the value was fetched
// from its upstream source.
//
// The fetch time is stored rather than recomputed because it is the only honest
// answer to "how old is this claim?" once a value has been served from cache,
// and because expiry must be measured from when the data was produced, not from
// when this process happened to read it.
type Entry[T any] struct {
	Value     T
	FetchedAt time.Time
}

// Option configures a Cache.
type Option func(*config)

type config struct {
	now func() time.Time
}

// WithClock replaces the cache's time source. It exists so expiry can be
// exercised deterministically, without sleeping. Production uses time.Now.
func WithClock(now func() time.Time) Option {
	return func(c *config) { c.now = now }
}

// Cache is a TTL cache keyed by string.
//
// It is safe for concurrent use: every read and write takes the same mutex, so
// a Scanner serving requests in parallel cannot observe a half-written entry.
//
// A Cache with a non-positive TTL is disarmed: Get always reports a miss and
// Put discards. That is how "caching is off" is expressed everywhere in Assay,
// so the uncached path is the same code path as the cached one and cannot drift
// from it.
type Cache[T any] struct {
	ttl time.Duration

	mu      sync.Mutex
	entries map[string]Entry[T]
	flights map[string]*flight[T]
	now     func() time.Time
}

type flight[T any] struct {
	waiters []chan flightResult[T]
	done    bool
	value   Entry[T]
	err     error
}

type flightResult[T any] struct {
	value Entry[T]
	err   error
}

// New returns a Cache that serves an entry for ttl after the entry's own fetch
// time. A ttl <= 0 returns a disarmed cache: every lookup misses.
func New[T any](ttl time.Duration, opts ...Option) *Cache[T] {
	cfg := config{now: time.Now}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Cache[T]{
		ttl:     ttl,
		entries: make(map[string]Entry[T]),
		flights: make(map[string]*flight[T]),
		now:     cfg.now,
	}
}

// TTL returns the lifetime this cache serves entries for. It is zero or
// negative when the cache is disarmed.
func (c *Cache[T]) TTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.ttl
}

// Get returns the entry stored under key if one is present and still within its
// TTL.
//
// An expired entry is deleted and reported as a miss. That rule is the reason
// this type exists: expiry always means re-fetch, never "assume the previous
// answer still holds". It matters most for a negative answer, where serving an
// expired "not blocked" would turn a stale observation into a safety
// assumption — and a stale negative is exactly the shape of a false clean bill
// of health.
func (c *Cache[T]) Get(key string) (Entry[T], bool) {
	var zero Entry[T]
	if c == nil || c.ttl <= 0 {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return zero, false
	}
	// Expiry is measured from the fetch time rather than from insertion, so a
	// value that finished fetching slowly still expires when the data does.
	if !c.now().Before(e.FetchedAt.Add(c.ttl)) {
		delete(c.entries, key)
		return zero, false
	}
	return e, true
}

// GetOrFetch returns the cached entry for key if present and fresh, otherwise
// it runs fetch exactly once for this key and returns its result to all
// concurrent callers.
//
// The boolean return value reports whether the answer came from cache (true) or
// from a fresh fetch (false). A single-flight waiter receives fromCache=true
// because it did not initiate the upstream request.
//
// If fetch returns an error, the error is returned to all waiters but the
// cache is not updated — a failed fetch does not populate the cache, so the
// next lookup will retry. On success, the returned Entry is stored in the
// cache (with its FetchedAt stamped if zero) and returned to all waiters.
//
// The fetch function is called without holding the cache lock, so it may
// perform I/O. The lock is held only to check the cache, register the flight,
// and store the result.
func (c *Cache[T]) GetOrFetch(key string, fetch func() (Entry[T], error)) (Entry[T], bool, error) {
	if c == nil || c.ttl <= 0 {
		val, err := fetch()
		return val, false, err
	}

	// Fast path: check cache under lock.
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		if c.now().Before(e.FetchedAt.Add(c.ttl)) {
			c.mu.Unlock()
			return e, true, nil
		}
		delete(c.entries, key)
	}

	// Check for an in-flight fetch for this key.
	if f, ok := c.flights[key]; ok {
		// If the flight is done and has no waiters, it's stale — remove it and
		// start a new fetch. This avoids a race where a fast fetch completes
		// before other callers can register as waiters.
		if f.done && len(f.waiters) == 0 {
			delete(c.flights, key)
		} else {
			// Create a channel for this waiter and register it.
			ch := make(chan flightResult[T], 1)
			f.waiters = append(f.waiters, ch)
			c.mu.Unlock()
			// Wait for the in-flight fetch to complete.
			result := <-ch
			c.mu.Lock()
			// Remove this waiter's channel from the list.
			for i, w := range f.waiters {
				if w == ch {
					f.waiters = append(f.waiters[:i], f.waiters[i+1:]...)
					break
				}
			}
			if len(f.waiters) == 0 && f.done {
				delete(c.flights, key)
			}
			c.mu.Unlock()
			return result.value, true, result.err
		}
	}

	// No cached entry and no in-flight fetch (or stale flight cleaned up): start one.
	f := &flight[T]{waiters: make([]chan flightResult[T], 0)}
	c.flights[key] = f
	c.mu.Unlock()

	// Run fetch without holding the lock.
	val, err := fetch()

	c.mu.Lock()
	f.done = true
	if err == nil {
		// Stamp FetchedAt if not set by caller.
		if val.FetchedAt.IsZero() {
			val.FetchedAt = c.now()
		}
		c.entries[key] = val
		f.value = val
	}
	f.err = err
	// Notify all waiters.
	result := flightResult[T]{value: f.value, err: err}
	for _, ch := range f.waiters {
		ch <- result
	}
	f.waiters = nil

	// Keep the flight in the map (with done=true, waiters=nil) so that callers
	// arriving during the fetch can still register as waiters. The next caller
	// to find a done flight with no waiters will clean it up (see above).
	c.mu.Unlock()

	return val, false, err
}

// Put stores an entry under key. A zero FetchedAt is stamped with the cache's
// clock so an entry can never silently outlive every TTL.
//
// Callers must not Put a value produced by a failed fetch: caching an outage
// would keep serving it after the source recovered, which is the same
// outage-rendered-as-an-answer failure the source error fields exist to
// prevent.
func (c *Cache[T]) Put(key string, e Entry[T]) {
	if c == nil || c.ttl <= 0 {
		return
	}
	if e.FetchedAt.IsZero() {
		e.FetchedAt = c.now()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = e
}

// Len reports how many entries the cache holds, including expired entries that
// have not been read since they expired. It is for tests and diagnostics.
func (c *Cache[T]) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
