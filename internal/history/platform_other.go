//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris)

package history

import "os"

// syncDir is a no-op on platforms that cannot open and sync a directory.
// Durability of the rename is best-effort there; docs/history.md records the
// fallback so a deployment does not assume a guarantee it does not have.
func syncDir(string) error { return nil }

// acquireLock reports no lock on platforms without the flock syscall. The
// single-writer guarantee is therefore unavailable on those platforms and
// operators must run one writer per history path; docs/history.md says so.
func acquireLock(string) (*os.File, error) { return nil, nil }

func releaseLock(*os.File) error { return nil }
