package fsutil

import (
	"github.com/marvelousu/game-l10n-updater/internal/hashx"
	"os"
	"path/filepath"
	"testing"
)

func TestBackedUpConcurrentEditIsRestored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.json")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	err := Commit([]FileUpdate{{Path: path, Data: []byte("new"), Existed: true, ExpectedHash: hashx.Text("old")}}, nil, CommitOptions{Hook: func(event string) error {
		if event == "backed-up" {
			backups, _ := filepath.Glob(path + ".gamel10n-bak-*")
			if len(backups) != 1 {
				t.Fatal("missing backup")
			}
			return os.WriteFile(backups[0], []byte("late edit"), 0600)
		}
		return nil
	}})
	if err == nil {
		t.Fatal("concurrent edit accepted")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "late edit" {
		t.Fatalf("late edit lost: %q %v", got, readErr)
	}
}
