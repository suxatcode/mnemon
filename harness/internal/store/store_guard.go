package store

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// fsKind classifies the filesystem hosting a database path for the anti-NFS guard. networked is true for
// any filesystem on which SQLite's WAL is unsafe (NFS/SMB/CIFS/FUSE/webdav). name/magic are diagnostics
// (the GOOS-specific defaultStatFS fills whichever its platform exposes).
type fsKind struct {
	name      string
	magic     int64
	networked bool
}

// statFSFunc classifies the filesystem under a path. It is injected into openGuard so a unit test can
// simulate a network mount without one (review blocker #9); the GOOS-tagged defaultStatFS is the real impl.
type statFSFunc func(path string) (fsKind, error)

// openGuard enforces S11 for a file-backed store: (1) the path must not live on a networked filesystem
// (a WAL DB on NFS silently corrupts — the one FATAL), and (2) only one writer may hold the file at a time
// (an exclusive PID lockfile next to it, with a liveness reap of a dead owner's stale lock). Both checks are
// skipped for :memory: and return a no-op release. The returned release MUST be called on Close.
func openGuard(path string, statFS statFSFunc) (func() error, error) {
	if path == ":memory:" {
		return func() error { return nil }, nil
	}
	kind, err := statFS(path)
	if err != nil {
		return nil, err
	}
	if kind.networked {
		return nil, fmt.Errorf("refusing to open %q on networked filesystem %q: WAL requires local disk (S11)", path, kind.name)
	}
	return acquireWriterLock(path + ".writer.lock")
}

// acquireWriterLock creates an exclusive lockfile holding this process's PID. If the lock already exists it
// reaps it iff the recorded owner is dead (liveness reap), then retries once; otherwise it reports the file
// as held by a live writer.
func acquireWriterLock(lock string) (func() error, error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d", os.Getpid())
			f.Close()
			return func() error { return os.Remove(lock) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if !reapStaleLock(lock) {
			return nil, fmt.Errorf("database %q is locked by another live writer (%s)", strings.TrimSuffix(lock, ".writer.lock"), lock)
		}
	}
	return nil, fmt.Errorf("database lock %q could not be acquired", lock)
}

// reapStaleLock removes a lockfile whose recorded owner PID is dead (or whose content is unreadable/garbage,
// which can only be a leftover from a crash). It returns true iff it removed the lock so the caller may retry.
func reapStaleLock(lock string) bool {
	b, err := os.ReadFile(lock)
	if err != nil {
		return false
	}
	pid, perr := strconv.Atoi(strings.TrimSpace(string(b)))
	if perr != nil || pid <= 0 {
		return os.Remove(lock) == nil // garbage content -> crash leftover -> reap
	}
	if processAlive(pid) {
		return false
	}
	return os.Remove(lock) == nil
}

// processAlive reports whether a process with the given PID exists (signal 0 probes liveness without
// delivering a signal). Cross-unix (darwin + linux), which are the only build targets.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
