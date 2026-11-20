//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package history

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// syncDir flushes the directory entry created by a rename. A file's contents
// and its name in the directory are different durable facts: without this the
// rename may vanish after a power loss even though the data survived.
//
// A few filesystems do not support syncing a directory. That is the one case
// where the error is ignored — an unsupported operation is not a failed one —
// and it is recognised by errno rather than by guessing from the path.
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("history: open directory %s: %w", dir, err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Sync(); err != nil {
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
			return nil
		}
		return fmt.Errorf("history: sync directory %s: %w", dir, err)
	}
	return nil
}

// acquireLock takes an exclusive, non-blocking advisory lock on a sidecar file.
// The sidecar is used rather than the log itself because every append replaces
// the log by rename; a lock on the old inode would not protect the next write.
func acquireLock(path string) (*os.File, error) {
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("history: open lock %s: %w", lockPath, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("%w: %s is already being written by another assay process (lock %s)",
				ErrLocked, path, lockPath)
		}
		return nil, fmt.Errorf("history: lock %s: %w", lockPath, err)
	}
	return f, nil
}

func releaseLock(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("history: unlock %s: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("history: close lock %s: %w", f.Name(), err)
	}
	return nil
}
