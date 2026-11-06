// Package exporter builds stable JSON and Godot CSV exports.
package exporter

import (
	"sort"

	"github.com/marvelousu/game-l10n-updater/internal/godotcsv"
	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/project"
)

// JSONDocument is the combined stable JSON export.
type JSONDocument struct {
	SchemaVersion  int                          `json:"schema_version"`
	SourceLanguage string                       `json:"source_language"`
	Languages      map[string]map[string]string `json:"languages"`
}

// BuildJSON exports the source locale and selected target locales.
func BuildJSON(p *project.Project, locales []string) ([]byte, error) {
	doc := JSONDocument{
		SchemaVersion:  model.SchemaVersion,
		SourceLanguage: p.Config.Source.Language,
		Languages:      map[string]map[string]string{},
	}
	source := make(map[string]string, len(p.Source.Entries))
	for _, entry := range p.Source.Entries {
		source[entry.Key] = entry.Text
	}
	doc.Languages[p.Config.Source.Language] = source
	for _, locale := range locales {
		m := p.Targets[locale]
		if m == nil {
			m = map[string]string{}
		}
		copied := make(map[string]string, len(m))
		for key, value := range m {
			copied[key] = value
		}
		doc.Languages[locale] = copied
	}
	return jsonx.MarshalStable(doc)
}

// BuildCSV exports keys,source,locale... preserving Unicode and quoting.
func BuildCSV(p *project.Project, locales []string) ([]byte, error) {
	source := make(map[string]string, len(p.Source.Entries))
	for _, entry := range p.Source.Entries {
		source[entry.Key] = entry.Text
	}
	selected := map[string]map[string]string{}
	for _, locale := range locales {
		selected[locale] = p.Targets[locale]
	}
	return godotcsv.Export(p.Config.Source.Language, locales, source, selected)
}

// SelectLocales validates a subset of configured locales, preserving config
// order for export columns.
func SelectLocales(p *project.Project, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return p.Locales(), nil
	}
	configured := map[string]bool{}
	for _, locale := range p.Config.Languages {
		configured[locale] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, locale := range requested {
		if !configured[locale] {
			return nil, &UnknownLocaleError{Locale: locale}
		}
		if !seen[locale] {
			seen[locale] = true
			out = append(out, locale)
		}
	}
	return out, nil
}

// UnknownLocaleError describes a locale absent from the config.
type UnknownLocaleError struct{ Locale string }

func (e *UnknownLocaleError) Error() string { return "locale " + e.Locale + " is not configured" }

// SortKeys returns sorted map keys.
func SortKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
