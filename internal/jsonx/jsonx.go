// Package jsonx contains the strict JSON helpers used for project files.
package jsonx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// DecodeStrict decodes exactly one JSON value and rejects unknown fields and
// trailing content.
func DecodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return fmt.Errorf("unexpected trailing data: %w", err)
	}
	return nil
}

// MarshalStable encodes indented UTF-8 JSON with a trailing newline, HTML
// escaping disabled, and deterministic map key ordering.
func MarshalStable(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
