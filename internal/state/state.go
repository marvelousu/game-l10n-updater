// Package state loads and saves the translation state file.
package state

import (
	"fmt"
	"os"

	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/pathx"
)

// Load reads path. A missing state file yields an empty state.
func Load(path string) (*model.State, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return model.NewState(), false, nil
		}
		return nil, false, err
	}
	st, err := Parse(data)
	if err != nil {
		return nil, true, fmt.Errorf("state %s: %w", path, err)
	}
	return st, true, nil
}

// Parse strictly decodes a state document.
func Parse(data []byte) (*model.State, error) {
	var st model.State
	if err := jsonx.DecodeStrict(data, &st); err != nil {
		return nil, err
	}
	if err := Validate(&st); err != nil {
		return nil, err
	}
	Normalize(&st)
	return &st, nil
}

// Validate checks versions, locales. Structural consistency is kept lenient so
// a human can add approval flags by hand.
func Validate(st *model.State) error {
	if st == nil {
		return fmt.Errorf("state is nil")
	}
	if st.SchemaVersion != model.SchemaVersion {
		return fmt.Errorf("schema_version must be %d, got %d", model.SchemaVersion, st.SchemaVersion)
	}
	for locale, ls := range st.Files {
		if err := pathx.ValidateLocale(locale); err != nil {
			return fmt.Errorf("files locale: %w", err)
		}
		if ls == nil {
			return fmt.Errorf("locale %q has null state", locale)
		}
		for key, entry := range ls.Entries {
			if key == "" {
				return fmt.Errorf("locale %q has an empty key", locale)
			}
			if entry == nil {
				return fmt.Errorf("locale %q key %q has null state entry", locale, key)
			}
			if !validHash(entry.SourceHash) || !validHash(entry.TargetHash) {
				return fmt.Errorf("locale %q key %q has an invalid hash", locale, key)
			}
		}
	}
	return nil
}

func validHash(hash string) bool {
	if hash == "" {
		return true
	}
	if len(hash) != 64 {
		return false
	}
	for _, r := range hash {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// Normalize initializes nil maps so callers can mutate safely.
func Normalize(st *model.State) {
	if st.Files == nil {
		st.Files = map[string]*model.LocaleState{}
	}
	for _, ls := range st.Files {
		if ls.Entries == nil {
			ls.Entries = map[string]*model.EntryState{}
		}
	}
}

// Marshal returns stable state bytes.
func Marshal(st *model.State) ([]byte, error) {
	if st == nil {
		st = model.NewState()
	}
	cp := Clone(st)
	cp.SchemaVersion = model.SchemaVersion
	return jsonx.MarshalStable(cp)
}

// Clone deep-copies a state so sync can build output in memory.
func Clone(st *model.State) *model.State {
	if st == nil {
		return model.NewState()
	}
	cp := &model.State{SchemaVersion: st.SchemaVersion, Files: map[string]*model.LocaleState{}}
	for locale, ls := range st.Files {
		if ls == nil {
			continue
		}
		nls := &model.LocaleState{Entries: map[string]*model.EntryState{}}
		for key, entry := range ls.Entries {
			if entry == nil {
				continue
			}
			copied := *entry
			nls.Entries[key] = &copied
		}
		cp.Files[locale] = nls
	}
	Normalize(cp)
	return cp
}
