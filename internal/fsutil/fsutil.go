// Package fsutil provides atomic file writes and a small multi-file
// transactional commit used by sync.
package fsutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/marvelousu/game-l10n-updater/internal/hashx"
)

// FileUpdate is one file to replace.
type FileUpdate struct {
	Path         string
	Data         []byte
	Existed      bool
	ExpectedHash string
}

// Guard is a read-only file whose content must not have changed.
type Guard struct {
	Path    string
	Existed bool
	Hash    string
}

// CommitOptions controls commit instrumentation. Hook is for tests and is
// called with "staged", "backed-up", "installed" and "committed".
type CommitOptions struct {
	Hook func(event string) error
	Mode os.FileMode
}

// ReadOptional reads path and reports whether it existed.
func ReadOptional(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

// WriteAtomic replaces path with data using a temporary file in the same
// directory. It is used for reports, exports and initialization files.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".gamel10n-write-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if mode == 0 {
		mode = 0o644
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("set mode on temporary file for %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

type stagedFile struct {
	update  FileUpdate
	tmpPath string
	backup  string
}

// Commit replaces all updates or leaves every original file untouched. It
// verifies guards and expected update hashes first, stages temporary files,
// moves originals to backups, installs the new files, and rolls back on any
// error.
func Commit(updates []FileUpdate, guards []Guard, opts CommitOptions) error {
	if len(updates) == 0 {
		return nil
	}
	mode := opts.Mode
	if mode == 0 {
		mode = 0o644
	}
	hook := opts.Hook
	callHook := func(event string) error {
		if hook == nil {
			return nil
		}
		if err := hook(event); err != nil {
			return fmt.Errorf("commit hook %s: %w", event, err)
		}
		return nil
	}

	updates = append([]FileUpdate(nil), updates...)
	sort.Slice(updates, func(i, j int) bool { return updates[i].Path < updates[j].Path })
	seen := map[string]bool{}
	for _, u := range updates {
		if seen[u.Path] {
			return fmt.Errorf("duplicate commit path %s", u.Path)
		}
		seen[u.Path] = true
	}

	verify := func(path string, existed bool, expected string) error {
		data, exists, err := ReadOptional(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if existed != exists {
			if existed {
				return fmt.Errorf("%s disappeared during sync", path)
			}
			return fmt.Errorf("%s appeared during sync", path)
		}
		if existed && hashx.Bytes(data) != expected {
			return fmt.Errorf("%s changed during sync", path)
		}
		return nil
	}

	for _, g := range guards {
		if err := verify(g.Path, g.Existed, g.Hash); err != nil {
			return err
		}
	}
	for _, u := range updates {
		if err := verify(u.Path, u.Existed, u.ExpectedHash); err != nil {
			return err
		}
	}

	stagedFiles := make([]stagedFile, 0, len(updates))
	removeTemps := func() {
		for _, s := range stagedFiles {
			if s.tmpPath != "" {
				_ = os.Remove(s.tmpPath)
			}
		}
	}

	// Stage every output before touching an original.
	for i := range updates {
		u := updates[i]
		if err := os.MkdirAll(filepath.Dir(u.Path), 0o755); err != nil {
			removeTemps()
			return fmt.Errorf("create directory for %s: %w", u.Path, err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(u.Path), ".gamel10n-stage-*.tmp")
		if err != nil {
			removeTemps()
			return fmt.Errorf("stage %s: %w", u.Path, err)
		}
		tmpName := tmp.Name()
		if err := tmp.Chmod(mode); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
			removeTemps()
			return fmt.Errorf("set mode for %s: %w", u.Path, err)
		}
		if _, err := tmp.Write(u.Data); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
			removeTemps()
			return fmt.Errorf("write stage for %s: %w", u.Path, err)
		}
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
			removeTemps()
			return fmt.Errorf("sync stage for %s: %w", u.Path, err)
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmpName)
			removeTemps()
			return fmt.Errorf("close stage for %s: %w", u.Path, err)
		}
		stagedFiles = append(stagedFiles, stagedFile{update: u, tmpPath: tmpName})
	}
	if err := callHook("staged"); err != nil {
		removeTemps()
		return err
	}
	// Re-check immediately before moving originals: staging can take time and
	// an external editor may have written a file in the meantime.
	for _, g := range guards {
		if err := verify(g.Path, g.Existed, g.Hash); err != nil {
			removeTemps()
			return err
		}
	}
	for _, u := range updates {
		if err := verify(u.Path, u.Existed, u.ExpectedHash); err != nil {
			removeTemps()
			return err
		}
	}

	for i := range stagedFiles {
		s := &stagedFiles[i]
		if !s.update.Existed {
			continue
		}
		backup := s.update.Path + ".gamel10n-bak-" + randomSuffix()
		if err := os.Rename(s.update.Path, backup); err != nil {
			removeTemps()
			if rbErr := restoreBackups(stagedFiles); rbErr != nil {
				return fmt.Errorf("backup %s: %w (rollback also failed: %v)", s.update.Path, err, rbErr)
			}
			return fmt.Errorf("backup %s: %w", s.update.Path, err)
		}
		s.backup = backup
	}
	if err := callHook("backed-up"); err != nil {
		removeTemps()
		if rbErr := restoreBackups(stagedFiles); rbErr != nil {
			return fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
		}
		return err
	}
	// Validate what was actually moved so an edit in the final verification /
	// rename window is restored rather than overwritten.
	for _, s := range stagedFiles {
		if s.backup != "" {
			if err := verify(s.backup, true, s.update.ExpectedHash); err != nil {
				removeTemps()
				if restoreErr := restoreBackups(stagedFiles); restoreErr != nil {
					return fmt.Errorf("%w (rollback also failed: %v)", err, restoreErr)
				}
				return err
			}
		}
	}

	for i := range stagedFiles {
		s := &stagedFiles[i]
		if err := os.Rename(s.tmpPath, s.update.Path); err != nil {
			removeTemps()
			rbErr := restoreBackups(stagedFiles)
			if rbErr != nil {
				return fmt.Errorf("install %s: %w (rollback also failed: %v)", s.update.Path, err, rbErr)
			}
			return fmt.Errorf("install %s: %w", s.update.Path, err)
		}
		s.tmpPath = ""
		if err := callHook("installed"); err != nil {
			removeTemps()
			rbErr := restoreBackups(stagedFiles)
			if rbErr != nil {
				return fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
			}
			return err
		}
	}

	for i := range stagedFiles {
		if stagedFiles[i].backup != "" {
			// The new files are already installed. A leftover backup is
			// harmless and must not turn a successful commit into a reported
			// failure that implies the target/state were rolled back.
			_ = os.Remove(stagedFiles[i].backup)
			stagedFiles[i].backup = ""
		}
	}
	if err := callHook("committed"); err != nil {
		return err
	}
	return nil
}

// restoreBackups removes files installed by this commit and moves every
// backup back into place. It is safe to call from any failure point.
func restoreBackups(files []stagedFile) error {
	var errs []string
	for i := len(files) - 1; i >= 0; i-- {
		s := files[i]
		if s.tmpPath == "" {
			// The staged file was installed at this path.
			if err := os.Remove(s.update.Path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("remove %s: %v", s.update.Path, err))
			}
		}
		if s.backup == "" {
			continue
		}
		if err := os.Rename(s.backup, s.update.Path); err != nil {
			errs = append(errs, fmt.Sprintf("restore %s: %v", s.update.Path, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}

func joinErrors(errs []string) string {
	out := ""
	for i, e := range errs {
		if i > 0 {
			out += "; "
		}
		out += e
	}
	return out
}

func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", os.Getpid())
	}
	return hex.EncodeToString(b[:])
}
