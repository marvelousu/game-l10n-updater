// Package godotcsv reads and writes Godot translation CSV files.
package godotcsv

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/pathx"
)

// Table is a parsed CSV with a header and per-row column values.
type Table struct {
	Header []string
	Rows   []map[string]string
}

// Parse reads a Godot-style CSV. The first column must be keys.
func Parse(data []byte) (*Table, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = 0
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("CSV is empty")
	}
	header := append([]string(nil), records[0]...)
	if len(header) == 0 {
		return nil, fmt.Errorf("CSV header is empty")
	}
	header[0] = strings.TrimPrefix(header[0], "\ufeff")
	if !strings.EqualFold(header[0], "keys") && !strings.EqualFold(header[0], "key") {
		return nil, fmt.Errorf("first CSV column must be %q, got %q", "keys", header[0])
	}
	header[0] = "keys"
	seen := map[string]bool{}
	for _, column := range header {
		if column == "" {
			return nil, fmt.Errorf("CSV header contains an empty column")
		}
		if seen[column] {
			return nil, fmt.Errorf("duplicate CSV column %q", column)
		}
		seen[column] = true
	}
	table := &Table{Header: header}
	for rowIndex, record := range records[1:] {
		if len(record) != len(header) {
			return nil, fmt.Errorf("row %d has %d fields, want %d", rowIndex+2, len(record), len(header))
		}
		row := map[string]string{}
		for i, column := range header {
			row[column] = record[i]
		}
		if row["keys"] == "" {
			return nil, fmt.Errorf("row %d has an empty key", rowIndex+2)
		}
		table.Rows = append(table.Rows, row)
	}
	return table, nil
}

// Import converts a table into a source catalog and flat target maps. Empty
// target cells are treated as missing (not as manual empty translations).
func Import(data []byte, sourceLanguage string) (*model.SourceCatalog, map[string]map[string]string, []string, error) {
	if err := pathx.ValidateLocale(sourceLanguage); err != nil {
		return nil, nil, nil, err
	}
	table, err := Parse(data)
	if err != nil {
		return nil, nil, nil, err
	}
	haveSource := false
	for _, column := range table.Header[1:] {
		if column == sourceLanguage {
			haveSource = true
		}
	}
	if !haveSource {
		return nil, nil, nil, fmt.Errorf("CSV has no source language column %q", sourceLanguage)
	}
	var locales []string
	for _, column := range table.Header[1:] {
		if column == sourceLanguage {
			continue
		}
		if err := pathx.ValidateLocale(column); err != nil {
			return nil, nil, nil, fmt.Errorf("CSV column %q: %w", column, err)
		}
		locales = append(locales, column)
	}
	seen := map[string]bool{}
	entries := make([]model.SourceEntry, 0, len(table.Rows))
	targets := map[string]map[string]string{}
	for _, locale := range locales {
		targets[locale] = map[string]string{}
	}
	for _, row := range table.Rows {
		key := row["keys"]
		if seen[key] {
			return nil, nil, nil, fmt.Errorf("duplicate CSV key %q", key)
		}
		seen[key] = true
		entries = append(entries, model.SourceEntry{Key: key, Text: row[sourceLanguage]})
		for _, locale := range locales {
			if value := row[locale]; value != "" {
				targets[locale][key] = value
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return &model.SourceCatalog{SchemaVersion: model.SchemaVersion, Entries: entries}, targets, locales, nil
}

// Export writes keys,source,locale... with sorted keys and standard CSV
// quoting. Source keys and target extras are both included.
func Export(sourceLanguage string, locales []string, source map[string]string, targets map[string]map[string]string) ([]byte, error) {
	columns := []string{"keys", sourceLanguage}
	seen := map[string]bool{sourceLanguage: true}
	for _, locale := range locales {
		if seen[locale] {
			continue
		}
		seen[locale] = true
		columns = append(columns, locale)
	}
	keySet := map[string]bool{}
	for key := range source {
		keySet[key] = true
	}
	for _, targetMap := range targets {
		for key := range targetMap {
			keySet[key] = true
		}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(columns); err != nil {
		return nil, err
	}
	for _, key := range keys {
		row := make([]string, len(columns))
		row[0] = key
		row[1] = source[key]
		for i := 2; i < len(columns); i++ {
			if m := targets[columns[i]]; m != nil {
				row[i] = m[key]
			}
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ReadAll is a convenience wrapper for tests and tools.
func ReadAll(r io.Reader) (*Table, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}
