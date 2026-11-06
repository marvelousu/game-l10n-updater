package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCSVImportPreservesAbsentAndEmptyTargets(t *testing.T) {
	dir := t.TempDir()
	targets := filepath.Join(dir, "targets")
	os.MkdirAll(targets, 0755)
	path := filepath.Join(targets, "en.json")
	os.WriteFile(path, []byte(`{"keep":"human reviewed","blank":"keep this","update":"old"}`), 0600)
	csv := filepath.Join(dir, "input.csv")
	os.WriteFile(csv, []byte("keys,ja,en\nupdate,更新,new\nblank,空欄,\n"), 0600)
	var output bytes.Buffer
	code := Run([]string{"import-csv", "-csv", csv, "-out", filepath.Join(dir, "source.json"), "-targets-dir", targets, "-force"}, &output, &output)
	if code != 0 {
		t.Fatalf("import failed: %s", output.String())
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte(`"keep": "human reviewed"`)) || !bytes.Contains(data, []byte(`"blank": "keep this"`)) || !bytes.Contains(data, []byte(`"update": "new"`)) {
		t.Fatalf("lost existing translations: %s", data)
	}
}

func TestCSVImportHonorsExistingProjectLock(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "input.csv")
	os.WriteFile(csv, []byte("keys,ja,en\na,元,translated\n"), 0600)
	os.WriteFile(filepath.Join(dir, ".gamel10n.lock"), []byte("other sync"), 0600)
	var output bytes.Buffer
	if code := Run([]string{"import-csv", "-csv", csv, "-out", filepath.Join(dir, "source.json")}, &output, &output); code == 0 {
		t.Fatal("import bypassed project lock")
	}
	if _, err := os.Stat(filepath.Join(dir, "source.json")); !os.IsNotExist(err) {
		t.Fatal("locked import wrote source")
	}
}
