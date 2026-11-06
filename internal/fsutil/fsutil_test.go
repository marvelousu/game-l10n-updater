package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marvelousu/game-l10n-updater/internal/hashx"
)

func TestWriteAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "out.json")
	if err := WriteAtomic(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "hello" {
		t.Fatalf("unexpected content: %q %v", data, err)
	}
	if err := WriteAtomic(path, []byte("replaced"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "replaced" {
		t.Fatalf("not replaced: %q", data)
	}
}

func TestCommitRollsBackBothFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte("old-a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("old-b"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed := 0
	err := Commit([]FileUpdate{
		{Path: a, Data: []byte("new-a"), Existed: true, ExpectedHash: hashx.Text("old-a")},
		{Path: b, Data: []byte("new-b"), Existed: true, ExpectedHash: hashx.Text("old-b")},
	}, nil, CommitOptions{Hook: func(event string) error {
		if event == "installed" {
			installed++
			if installed == 2 {
				return errors.New("boom")
			}
		}
		return nil
	}})
	if err == nil {
		t.Fatal("expected commit failure")
	}
	for path, want := range map[string]string{a: "old-a", b: "old-b"} {
		data, readErr := os.ReadFile(path)
		if readErr != nil || string(data) != want {
			t.Fatalf("%s = %q, %v; want %q", path, data, readErr, want)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".gamel10n-") {
			t.Fatalf("temporary or backup file left behind: %s", entry.Name())
		}
	}
}

func TestCommitRollsBackNewFile(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte("old-a"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed := 0
	err := Commit([]FileUpdate{
		{Path: a, Data: []byte("new-a"), Existed: true, ExpectedHash: hashx.Text("old-a")},
		{Path: b, Data: []byte("new-b"), Existed: false},
	}, nil, CommitOptions{Hook: func(event string) error {
		if event == "installed" {
			installed++
			if installed == 2 {
				return errors.New("boom")
			}
		}
		return nil
	}})
	if err == nil {
		t.Fatal("expected commit failure")
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Fatalf("new file was not removed on rollback: %v", err)
	}
	data, _ := os.ReadFile(a)
	if string(data) != "old-a" {
		t.Fatalf("original not restored: %q", data)
	}
}

func TestCommitDetectsConcurrentChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Commit([]FileUpdate{{Path: path, Data: []byte("new"), Existed: true, ExpectedHash: hashx.Text("old")}}, nil, CommitOptions{})
	if err == nil || !strings.Contains(err.Error(), "changed during sync") {
		t.Fatalf("want concurrent-change error, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "external edit" {
		t.Fatalf("external edit overwritten: %q", data)
	}
}

func TestCommitDetectsAppearedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	if err := os.WriteFile(path, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Commit([]FileUpdate{{Path: path, Data: []byte("new"), Existed: false}}, nil, CommitOptions{})
	if err == nil || !strings.Contains(err.Error(), "appeared during sync") {
		t.Fatalf("want appeared error, got %v", err)
	}
}

func TestCommitRechecksAfterStaging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Commit([]FileUpdate{{Path: path, Data: []byte("new"), Existed: true, ExpectedHash: hashx.Text("old")}}, nil, CommitOptions{
		Hook: func(event string) error {
			if event == "staged" {
				return os.WriteFile(path, []byte("external during staging"), 0o644)
			}
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "changed during sync") {
		t.Fatalf("want staged concurrent-change error, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "external during staging" {
		t.Fatalf("external edit overwritten: %q", data)
	}
}
