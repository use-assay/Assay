// Package history stores per-asset observations so a consumer can ask what
// changed, not only what is true now.
//
// internal/temporal computes a transition between two observations and
// deliberately stores nothing — a transition is a pure function of its pair. A
// caller, though, has to get the pair from somewhere, and until this package
// existed Assay persisted nothing at all. This is that somewhere: an
// append-only sequence of observations per asset, which the API exposes as a
// history view and internal/temporal reduces to transitions on demand.
//
// Three design decisions are recorded here rather than left implicit, because
// this is the first component in Assay that retains state:
//
//   - Observations are stored; transitions are not. A stored transition would
//     be a second source of truth beside the pair it was derived from, and the
//     state of that pair can change (an observation can be replaced by a
//     complete one), so a frozen transition can disagree with its own inputs.
//     Recomputing is a handful of bitwise operations. See docs/transitions.md.
//
//   - The storage is an append-only JSON Lines log written with the standard
//     library. A registry whose whole threat model is "no third-party trust we
//     did not decide to take on" does not acquire a database dependency to keep
//     a few hundred small records per asset; encoding/json and os are already
//     part of the toolchain. The decision is recorded in docs/history.md.
//
//   - Retention is bounded per asset and drops the oldest observation first.
//     An unbounded log is a denial-of-service surface, and the oldest
//     observation is the one a consumer is least likely to need. The cap is
//     reported in every API response, so a history that has reached it is
//     visible to a consumer rather than passing as complete.
//
//   - The store is bounded globally as well as per asset. A per-asset cap
//     alone bounds nothing in aggregate: every distinct asset scanned through
//     the public API adds up to DefaultRetention records and the whole file is
//     rewritten on each append, so an instance could be driven to an
//     arbitrarily large file and O(n) write cost by scanning many assets. The
//     number of assets kept is capped (DefaultMaxAssets) and the asset whose
//     observations are least recent is evicted whole, which keeps every
//     retained asset's per-asset history intact. Both bounds are reported in
//     every API response. See docs/history.md.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/temporal"
)

// DefaultRetention is the greatest number of observations kept for one asset.
//
// It is deliberately generous relative to the twenty-odd assets Assay has ever
// scanned: the cost of an observation is a few hundred bytes, so the bound
// exists to make an unbounded log impossible rather than to save space. When
// more than this many observations exist for one asset, the oldest are dropped.
const DefaultRetention = 256

// DefaultMaxAssets is the greatest number of assets the store keeps
// observations for.
//
// It exists so the log file has a global bound, not only a per-asset one: with
// DefaultRetention observations per asset, the log can therefore never exceed
// DefaultMaxAssets × DefaultRetention records. Eviction is whole-asset and
// oldest-observed-first — the asset whose most recent observation is oldest is
// dropped, so an asset being scanned now is never the one evicted by a burst of
// new assets. It is deliberately generous relative to the ~20 assets Assay has
// ever scanned, and, like DefaultRetention, it is a bound against unbounded
// growth rather than a space-saving choice.
//
// An evicted asset's history reads as `missing`, exactly like an asset that was
// never scanned. That is why the bound is reported in every response: a
// consumer of a deployment that has reached the cap must read `missing` as
// "not retained" rather than "never observed". docs/history.md records the
// consequences.
const DefaultMaxAssets = 512

var (
	// ErrLocked reports that another process already holds the history's
	// single-writer lock. Callers can match it with errors.Is to turn startup
	// into a clear refusal rather than a generic I/O failure.
	ErrLocked = errors.New("history: already in use by another process")
	// ErrClosed reports use of a store after Close released its writer lock.
	ErrClosed = errors.New("history: store is closed")
)

// Store is an append-only, per-asset sequence of observations, ordered by time.
//
// The zero value is not usable; use New or Open. All methods are safe for
// concurrent use, which the HTTP server requires because scans and history
// reads arrive on separate connections.
type Store struct {
	mu        sync.Mutex
	byAsset   map[string][]temporal.Observation
	path      string
	retention int
	maxAssets int
	lock      *os.File
	closed    bool
}

// New returns an in-memory Store that retains DefaultRetention observations per
// asset. Nothing survives a restart, which is the right default for a process
// that has not been told where to write: losing history is honest, whereas
// writing to an unconfigured path would be a surprise.
func New() *Store { return NewWithRetention(DefaultRetention) }

// NewWithRetention returns an in-memory Store bounded to n observations per
// asset. A non-positive n is replaced by DefaultRetention rather than treated as
// "keep nothing", because silently retaining nothing would present as a
// permanently empty history.
func NewWithRetention(n int) *Store {
	return NewWithLimits(n, DefaultMaxAssets)
}

// NewWithLimits returns an in-memory Store bounded to n observations per asset
// and m assets overall. A non-positive n or m is replaced by its default rather
// than treated as "keep nothing", for the same reason NewWithRetention gives.
func NewWithLimits(n, m int) *Store {
	if n <= 0 {
		n = DefaultRetention
	}
	if m <= 0 {
		m = DefaultMaxAssets
	}
	return &Store{
		byAsset:   map[string][]temporal.Observation{},
		retention: n,
		maxAssets: m,
	}
}

// Open returns a Store persisted to path as a JSON Lines log, loading whatever
// is already there.
//
// An empty path is equivalent to New: an in-memory store. A path that does not
// exist yet is not an error — the log is created on the first append — because
// "no history yet" and "the history file is broken" are different facts and the
// first must not fail startup.
//
// A file-backed store takes an exclusive advisory lock on a sidecar ".lock"
// file at open time and refuses a second writer with ErrLocked. Call Close to
// release it; on platforms without flock the lock is a documented no-op.
func Open(path string) (*Store, error) {
	return OpenWithRetention(path, DefaultRetention)
}

// OpenWithRetention is Open with an explicit per-asset retention bound, for a
// deployment that wants a different one than DefaultRetention.
func OpenWithRetention(path string, n int) (*Store, error) {
	return OpenWithLimits(path, n, DefaultMaxAssets)
}

// OpenWithLimits is Open with explicit per-asset and global bounds, for a
// deployment that wants different ones than the defaults. The global bound is
// applied while loading, so a log written under a larger bound is rewritten to
// the smaller retained set on the next append.
func OpenWithLimits(path string, n, m int) (_ *Store, err error) {
	s := NewWithLimits(n, m)
	s.path = path
	if path == "" {
		return s, nil
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("history: create %s: %w", dir, err)
		}
	}
	lock, err := acquireLock(path)
	if err != nil {
		return nil, err
	}
	s.lock = lock
	defer func() {
		if err != nil {
			_ = releaseLock(lock)
		}
	}()

	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("history: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	// Observations carry evidence-derived check names and can legitimately be
	// longer than the default 64 KiB line limit.
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec logRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("history: decode %s: %w", path, err)
		}
		if rec.Version != logVersion {
			return nil, fmt.Errorf("history: decode %s: unsupported log record version %d, want %d",
				path, rec.Version, logVersion)
		}
		s.insertLocked(rec.observation())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("history: read %s: %w", path, err)
	}
	return s, nil
}

// Append records one observation, evicting the oldest for that asset when the
// retention bound is exceeded, and persists the result when the store is
// file-backed.
//
// The returned error is about storage, never about the observation itself: a
// caller recording a scan is not told its scan was rejected because the log
// could not be written.
func (s *Store) Append(o temporal.Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrClosed
	}
	s.insertLocked(o)
	if s.path == "" {
		return nil
	}
	return s.persistLocked()
}

// Close releases the single-writer lock held by a file-backed store and leaves
// the store unusable for appends. A process that exits without calling Close
// still releases the lock through the OS; Close exists so a test or a graceful
// shutdown can hand ownership to another writer immediately. It is safe to call
// more than once and on an in-memory store.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	if s.lock == nil {
		return nil
	}
	lock := s.lock
	s.lock = nil
	return releaseLock(lock)
}

// List returns the observations for one asset, oldest first.
//
// The result is a copy. A caller that mutated the returned slice would
// otherwise be editing the store, and a stored observation must not change when
// a later report is produced.
//
// The slice is non-nil even when empty, so an empty history serializes as `[]`
// and not `null`. "There are no observations" is a result and has to render as
// one.
func (s *Store) List(a mechanics.Asset) []temporal.Observation {
	s.mu.Lock()
	defer s.mu.Unlock()

	kept := s.byAsset[a.String()]
	out := make([]temporal.Observation, len(kept))
	copy(out, kept)
	return out
}

// Len reports how many observations are retained for one asset.
func (s *Store) Len(a mechanics.Asset) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byAsset[a.String()])
}

// RetentionLimit reports the per-asset bound this store keeps, so an API
// response can state the cap a consumer is reading against.
func (s *Store) RetentionLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retention
}

// MaxAssets reports the global cap on assets this store keeps, so an API
// response can state the bound the whole retained history is read against.
func (s *Store) MaxAssets() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxAssets
}

// AssetCount reports how many assets the store currently retains. It is the
// observable side of the global bound, used by tests and by an operator
// checking that a file has not grown past its cap.
func (s *Store) AssetCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byAsset)
}

// insertLocked adds one observation and re-applies the retention bound. The
// caller holds s.mu.
func (s *Store) insertLocked(o temporal.Observation) {
	key := o.Asset.String()
	list := append(s.byAsset[key], o)
	// Observations arrive in scan order, which is usually but not always time
	// order, and two observations may share a timestamp. Sorting here keeps the
	// sequence ordered by time regardless of arrival order; the stable sort
	// leaves same-instant observations in the order they arrived.
	sort.SliceStable(list, func(i, j int) bool { return list[i].At.Before(list[j].At) })
	if len(list) > s.retention {
		list = list[len(list)-s.retention:]
	}
	s.byAsset[key] = list
	s.evictAssetsLocked()
}

// evictAssetsLocked enforces the global asset bound. The caller holds s.mu.
//
// The asset whose newest observation is oldest is dropped whole, which is the
// global analogue of dropping a per-asset observation: the least recently
// observed asset is the one a consumer is least likely to ask about, and the
// assets still being scanned keep their full per-asset history. Ties are broken
// by asset key so eviction cannot depend on Go's map iteration order. An asset
// is never left half-evicted, because a partial history would read as complete.
func (s *Store) evictAssetsLocked() {
	for len(s.byAsset) > s.maxAssets {
		oldestKey := ""
		var oldestAt time.Time
		for key, list := range s.byAsset {
			// Lists are never empty: insertLocked appends before calling here.
			latest := list[len(list)-1].At
			if oldestKey == "" || latest.Before(oldestAt) ||
				(latest.Equal(oldestAt) && key < oldestKey) {
				oldestKey, oldestAt = key, latest
			}
		}
		delete(s.byAsset, oldestKey)
	}
}

// persistLocked rewrites the log from the retained set. The caller holds s.mu.
//
// The log is small and the store is written to at human rates, so a full
// rewrite is simpler and more obviously correct than an append with periodic
// compaction: the file is exactly the retained set at all times, which means a
// restart cannot resurrect observations that retention had already evicted. The
// write is atomic — a temporary file renamed over the target — so a crash
// mid-write cannot leave a half-observation in the log. The directory entry
// created by the rename is synced as well, because on some filesystems the
// rename itself may not survive a power loss otherwise; docs/history.md records
// the guarantee and its platform fallback.
func (s *Store) persistLocked() error {
	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("history: create %s: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return fmt.Errorf("history: create temp log: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	w := bufio.NewWriter(tmp)
	for _, key := range sortedKeys(s.byAsset) {
		for _, o := range s.byAsset[key] {
			b, err := json.Marshal(record(o))
			if err != nil {
				_ = tmp.Close()
				return fmt.Errorf("history: encode observation: %w", err)
			}
			if _, err := w.Write(append(b, '\n')); err != nil {
				_ = tmp.Close()
				return fmt.Errorf("history: write temp log: %w", err)
			}
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("history: flush temp log: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("history: close temp log: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("history: replace %s: %w", s.path, err)
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	return nil
}

// logVersion is written into every persisted record, so a future change to the
// stored shape cannot be read as if it were this one. It is the same reasoning
// as the evidence preimage's version line: a format a verifier reads has to
// carry the format it is.
const logVersion = 1

// logRecord is the persisted shape of one observation.
//
// It is deliberately not temporal.Observation. That type is the JSON shape the
// API renders, where severity is its level name for a human reader; this is the
// on-disk shape, where severity is stored as the number the model is actually
// built from. Keeping the two apart means a change to how severity is
// displayed cannot silently change what the log contains, and it keeps the log
// independent of mechanics.Severity having a symmetric JSON encoding.
// Accountability is optional in old logs: a record written before the field
// existed decodes with an empty value rather than failing to load, which keeps
// existing histories readable without a version bump.
type logRecord struct {
	Version            int                      `json:"v"`
	Asset              mechanics.Asset          `json:"asset"`
	At                 time.Time                `json:"at"`
	Mechanics          mechanics.Mechanic       `json:"mechanics"`
	BaseSeverity       uint32                   `json:"base_severity"`
	Severity           uint32                   `json:"severity"`
	Accountability     mechanics.Accountability `json:"accountability,omitempty"`
	Undetermined       bool                     `json:"undetermined"`
	UndeterminedChecks []string                 `json:"undetermined_checks,omitempty"`
}

func record(o temporal.Observation) logRecord {
	return logRecord{
		Version:            logVersion,
		Asset:              o.Asset,
		At:                 o.At,
		Mechanics:          o.Mechanics,
		BaseSeverity:       uint32(o.Base),
		Severity:           uint32(o.Severity),
		Accountability:     o.Accountability,
		Undetermined:       o.Undetermined,
		UndeterminedChecks: append([]string{}, o.UndeterminedChecks...),
	}
}

func (r logRecord) observation() temporal.Observation {
	return temporal.Observation{
		Asset:              r.Asset,
		At:                 r.At,
		Mechanics:          r.Mechanics,
		Base:               mechanics.Severity(r.BaseSeverity),
		Severity:           mechanics.Severity(r.Severity),
		Accountability:     r.Accountability,
		Undetermined:       r.Undetermined,
		UndeterminedChecks: append([]string{}, r.UndeterminedChecks...),
	}
}

func sortedKeys(m map[string][]temporal.Observation) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
