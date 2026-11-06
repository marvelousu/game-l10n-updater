// Package display parses the external Godot display-check report.
package display

import (
	"fmt"
	"os"

	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
	"github.com/marvelousu/game-l10n-updater/internal/pathx"
)

// Issue is one rendered display problem reported by the Godot worker.
type Issue struct {
	Key      string `json:"key"`
	Locale   string `json:"locale"`
	NodePath string `json:"node_path"`
	Kind     string `json:"kind"`
	Message  string `json:"message"`
}

// Report is the schema_version 1 report produced by the Godot adapter.
type Report struct {
	SchemaVersion int     `json:"schema_version"`
	Issues        []Issue `json:"issues"`
	Checked       int     `json:"checked"`
}

// Parse strictly validates a display report.
func Parse(data []byte) (*Report, error) {
	var report Report
	if err := jsonx.DecodeStrict(data, &report); err != nil {
		return nil, fmt.Errorf("parse display report: %w", err)
	}
	if report.SchemaVersion != 1 {
		return nil, fmt.Errorf("display report schema_version must be 1, got %d", report.SchemaVersion)
	}
	if report.Checked < 0 {
		return nil, fmt.Errorf("display report checked must not be negative")
	}
	for i, issue := range report.Issues {
		if issue.Key == "" {
			return nil, fmt.Errorf("display issue %d has an empty key", i)
		}
		if issue.Locale == "" {
			return nil, fmt.Errorf("display issue %d has an empty locale", i)
		}
		if err := pathx.ValidateLocale(issue.Locale); err != nil {
			return nil, fmt.Errorf("display issue %d locale: %w", i, err)
		}
		if issue.Kind == "" {
			return nil, fmt.Errorf("display issue %d has an empty kind", i)
		}
	}
	return &report, nil
}

// Load reads and validates path.
func Load(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read display report %s: %w", path, err)
	}
	report, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("display report %s: %w", path, err)
	}
	return report, nil
}
