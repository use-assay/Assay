package attester

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/use-assay/assay/internal/horizon"
)

// watchStateVersion is the state file's format version. A file with any other
// version is refused rather than guessed at.
const watchStateVersion = 1

// watchState is the watcher's persisted position: per issuer, the cursor of
// the last operation applied and the flags that operation left. It is what a
// restart resumes from, so downtime is not a blind spot.
type watchState struct {
	Version int                         `json:"version"`
	Issuers map[string]issuerCheckpoint `json:"issuers"`
}

type issuerCheckpoint struct {
	Cursor string        `json:"cursor"`
	Flags  horizon.Flags `json:"flags"`
}

// loadWatchState reads the state file at path. A missing file is not an
// error: it returns nil, and the watcher starts from the current head. Any
// other failure, including a corrupt file, is an error: silently starting
// over would reopen the blind spot the file exists to close.
func loadWatchState(path string) (*watchState, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read watch state %s: %w", path, err)
	}
	var s watchState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("watch state %s: %w", path, err)
	}
	if s.Version != watchStateVersion {
		return nil, fmt.Errorf("watch state %s: version %d, want %d", path, s.Version, watchStateVersion)
	}
	return &s, nil
}

// saveWatchState writes s to path atomically: it writes and syncs a temporary
// file in the same directory, then renames it over path. A crash mid-write
// leaves the previous state intact, never a truncated file.
func saveWatchState(path string, s *watchState) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode watch state: %w", err)
	}
	b = append(b, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write watch state: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write watch state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync watch state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write watch state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace watch state: %w", err)
	}
	// Persist the rename itself. Not every platform can sync a directory;
	// the rename is still atomic there, only its durability is weaker.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
