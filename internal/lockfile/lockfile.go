// Package lockfile prevents concurrent sync runs in one project.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Lock is an exclusive project sync lock.
type Lock struct {
	path     string
	released bool
}

// Acquire creates path exclusively. A stale lock is not stolen automatically:
// removing the file is an explicit human decision.
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			existing, _ := os.ReadFile(path)
			detail := strings.TrimSpace(string(existing))
			if detail != "" {
				detail = ": " + detail
			}
			return nil, fmt.Errorf("project is locked by another sync (%s)%s; remove the lock file only after confirming no sync is running", path, detail)
		}
		return nil, fmt.Errorf("create lock %s: %w", path, err)
	}
	content := fmt.Sprintf("pid=%d\nacquired=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write lock %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close lock %s: %w", path, err)
	}
	return &Lock{path: path}, nil
}

// Release removes the lock. It is safe to call more than once.
func (l *Lock) Release() error {
	if l == nil || l.released {
		return nil
	}
	l.released = true
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove lock %s: %w", l.path, err)
	}
	return nil
}

// Path returns the lock file path.
func (l *Lock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}
