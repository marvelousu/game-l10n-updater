package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireReleaseAndContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gamel10n.lock")
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	if _, err := Acquire(path); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("second acquire should fail, got %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file not removed: %v", err)
	}
	again, err := Acquire(path)
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	if err := again.Release(); err != nil {
		t.Fatal(err)
	}
	// Release is idempotent.
	if err := again.Release(); err != nil {
		t.Fatal(err)
	}
}
