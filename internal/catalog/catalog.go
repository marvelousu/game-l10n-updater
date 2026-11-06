// Package catalog parses the structured source catalog.
package catalog

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
	"github.com/marvelousu/game-l10n-updater/internal/model"
)

type document struct {
	SchemaVersion int                 `json:"schema_version"`
	Entries       []model.SourceEntry `json:"entries"`
}

// Parse accepts either a top-level entry array or
// {"schema_version":1,"entries":[...]}.
func Parse(data []byte) (*model.SourceCatalog, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("source catalog is empty")
	}
	var cat model.SourceCatalog
	switch trimmed[0] {
	case '[':
		if err := jsonx.DecodeStrict(trimmed, &cat.Entries); err != nil {
			return nil, fmt.Errorf("parse source entries: %w", err)
		}
		cat.SchemaVersion = model.SchemaVersion
	case '{':
		var doc document
		if err := jsonx.DecodeStrict(trimmed, &doc); err != nil {
			return nil, fmt.Errorf("parse source catalog: %w", err)
		}
		if doc.SchemaVersion != model.SchemaVersion {
			return nil, fmt.Errorf("source catalog schema_version must be %d, got %d", model.SchemaVersion, doc.SchemaVersion)
		}
		cat = model.SourceCatalog{SchemaVersion: doc.SchemaVersion, Entries: doc.Entries}
	default:
		return nil, fmt.Errorf("source catalog must be a JSON object or array")
	}
	if err := Validate(&cat); err != nil {
		return nil, err
	}
	return &cat, nil
}

// Load reads and parses path.
func Load(path string) (*model.SourceCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source catalog %s: %w", path, err)
	}
	cat, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("source catalog %s: %w", path, err)
	}
	return cat, nil
}

// Validate checks keys, duplicates and max_length.
func Validate(cat *model.SourceCatalog) error {
	if cat == nil {
		return fmt.Errorf("source catalog is nil")
	}
	seen := map[string]bool{}
	for i, entry := range cat.Entries {
		if entry.Key == "" {
			return fmt.Errorf("source entry %d has an empty key", i)
		}
		if strings.ContainsRune(entry.Key, 0) {
			return fmt.Errorf("source key %q contains NUL", entry.Key)
		}
		if seen[entry.Key] {
			return fmt.Errorf("duplicate source key %q", entry.Key)
		}
		seen[entry.Key] = true
		if entry.MaxLength < 0 {
			return fmt.Errorf("source key %q has negative max_length", entry.Key)
		}
	}
	return nil
}

// Marshal returns the canonical catalog bytes with entries sorted by key.
func Marshal(cat *model.SourceCatalog) ([]byte, error) {
	sorted := *cat
	sorted.SchemaVersion = model.SchemaVersion
	sorted.Entries = append([]model.SourceEntry(nil), cat.Entries...)
	if sorted.Entries == nil {
		sorted.Entries = []model.SourceEntry{}
	}
	sort.Slice(sorted.Entries, func(i, j int) bool { return sorted.Entries[i].Key < sorted.Entries[j].Key })
	return jsonx.MarshalStable(sorted)
}

// Index returns entries by key and sorted keys.
func Index(cat *model.SourceCatalog) (map[string]model.SourceEntry, []string) {
	byKey := make(map[string]model.SourceEntry, len(cat.Entries))
	keys := make([]string, 0, len(cat.Entries))
	for _, entry := range cat.Entries {
		byKey[entry.Key] = entry
		keys = append(keys, entry.Key)
	}
	sort.Strings(keys)
	return byKey, keys
}
