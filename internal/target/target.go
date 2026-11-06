// Package target reads and writes flat JSON target catalogs.
package target

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
)

// Parse decodes a flat JSON object of key to string. Duplicate keys and
// non-string values are rejected.
func Parse(data []byte) (map[string]string, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("target catalog is empty")
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("target catalog must be a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("target catalog must be a JSON object")
	}
	out := map[string]string{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("read target key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("target key is not a string")
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate target key %q", key)
		}
		var value string
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("target key %q must have a string value: %w", key, err)
		}
		if strings.ContainsRune(key, 0) {
			return nil, fmt.Errorf("target key contains NUL")
		}
		out[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("read target object end: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON value in target catalog")
		}
		return nil, fmt.Errorf("unexpected trailing data in target catalog: %w", err)
	}
	return out, nil
}

// Load reads path. A missing file is an empty catalog.
func Load(path string) (map[string]string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, false, nil
		}
		return nil, false, err
	}
	m, err := Parse(data)
	if err != nil {
		return nil, true, fmt.Errorf("target catalog %s: %w", path, err)
	}
	return m, true, nil
}

// Marshal returns stable, sorted, UTF-8 JSON with markup preserved.
func Marshal(m map[string]string) ([]byte, error) {
	if m == nil {
		m = map[string]string{}
	}
	return jsonx.MarshalStable(m)
}

// Keys returns sorted target keys.
func Keys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
