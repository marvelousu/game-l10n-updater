// Package check validates targets and merges Godot display reports.
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/marvelousu/game-l10n-updater/internal/display"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/pathx"
	"github.com/marvelousu/game-l10n-updater/internal/project"
	"github.com/marvelousu/game-l10n-updater/internal/validate"
)

// Counts summarizes a check report.
type Counts struct {
	Errors     int `json:"errors"`
	Warnings   int `json:"warnings"`
	Structural int `json:"structural"`
	Display    int `json:"display"`
}

// DisplayInfo records the merged engine report.
type DisplayInfo struct {
	Path           string   `json:"path"`
	Checked        int      `json:"checked"`
	IssueCount     int      `json:"issues"`
	UnknownKeys    []string `json:"unknown_keys,omitempty"`
	UnknownLocales []string `json:"unknown_locales,omitempty"`
}

// Report is the JSON written by gamel10n check.
type Report struct {
	SchemaVersion  int           `json:"schema_version"`
	Kind           string        `json:"kind"`
	SourceLanguage string        `json:"source_language"`
	Locales        []string      `json:"locales"`
	Issues         []model.Issue `json:"issues"`
	Counts         Counts        `json:"counts"`
	Checked        int           `json:"checked"`
	Display        *DisplayInfo  `json:"display_report,omitempty"`
}

// Build validates every selected locale and merges an optional display report.
func Build(p *project.Project, displayReport *display.Report, displayPath string, selected []string) (*Report, error) {
	locales, err := selectLocales(p, selected)
	if err != nil {
		return nil, err
	}
	report := &Report{
		SchemaVersion:  model.SchemaVersion,
		Kind:           "check",
		SourceLanguage: p.Config.Source.Language,
		Locales:        locales,
		Issues:         []model.Issue{},
	}
	for _, locale := range locales {
		targetMap := p.Targets[locale]
		for _, key := range p.SourceKeys {
			entry := p.SourceMap[key]
			text, ok := targetMap[key]
			if !ok {
				report.Issues = append(report.Issues, model.Issue{
					Key: key, Locale: locale, Kind: "missing_target", Severity: "warning",
					Source: "catalog", Message: "target key is missing",
				})
				continue
			}
			report.Issues = append(report.Issues, validate.Issues(entry.Text, text, key, locale)...)
			if issue := validate.MaxLengthIssue(entry, text, locale); issue != nil {
				report.Issues = append(report.Issues, *issue)
			}
		}
	}
	report.Checked = len(locales) * len(p.SourceKeys)

	if displayReport != nil {
		info := &DisplayInfo{Path: displayPath, Checked: displayReport.Checked, IssueCount: len(displayReport.Issues)}
		info.UnknownKeys = []string{}
		info.UnknownLocales = []string{}
		selectedSet := map[string]bool{}
		for _, locale := range locales {
			selectedSet[locale] = true
		}
		configuredSet := map[string]bool{}
		configuredSet[p.Config.Source.Language] = true
		for _, locale := range p.Config.Languages {
			configuredSet[locale] = true
		}
		filterActive := len(selected) > 0
		for _, issue := range displayReport.Issues {
			if filterActive && !selectedSet[issue.Locale] {
				continue
			}
			_, knownKey := p.SourceMap[issue.Key]
			unknownLocale := !configuredSet[issue.Locale]
			report.Issues = append(report.Issues, model.Issue{
				Key: issue.Key, Locale: issue.Locale, Kind: issue.Kind, Severity: displaySeverity(issue.Kind),
				Message: issue.Message, Source: "display", NodePath: issue.NodePath,
				UnknownKey: !knownKey, UnknownLocale: unknownLocale,
			})
			if !knownKey {
				info.UnknownKeys = append(info.UnknownKeys, issue.Key)
			}
			if unknownLocale {
				info.UnknownLocales = append(info.UnknownLocales, issue.Locale)
			}
		}
		sort.Strings(info.UnknownKeys)
		info.UnknownKeys = unique(info.UnknownKeys)
		sort.Strings(info.UnknownLocales)
		info.UnknownLocales = unique(info.UnknownLocales)
		report.Display = info
	}

	sort.SliceStable(report.Issues, func(i, j int) bool {
		a, b := report.Issues[i], report.Issues[j]
		if a.Locale != b.Locale {
			return a.Locale < b.Locale
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Category < b.Category
	})
	for _, issue := range report.Issues {
		if issue.Severity == "warning" {
			report.Counts.Warnings++
			continue
		}
		report.Counts.Errors++
		if issue.Source == "display" {
			report.Counts.Display++
		} else {
			report.Counts.Structural++
		}
	}
	return report, nil
}

// ExitCode maps structural and display errors to distinct nonzero codes.
func (r *Report) ExitCode() int {
	if r.Counts.Structural > 0 {
		return 3
	}
	if r.Counts.Display > 0 {
		return 4
	}
	return 0
}

func selectLocales(p *project.Project, selected []string) ([]string, error) {
	if len(selected) == 0 {
		locales := p.Locales()
		sort.Strings(locales)
		return locales, nil
	}
	configured := map[string]bool{}
	for _, locale := range p.Config.Languages {
		configured[locale] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, locale := range selected {
		if err := pathx.ValidateLocale(locale); err != nil {
			return nil, err
		}
		if !configured[locale] {
			return nil, fmt.Errorf("locale %q is not configured", locale)
		}
		if !seen[locale] {
			seen[locale] = true
			out = append(out, locale)
		}
	}
	sort.Strings(out)
	return out, nil
}

func unique(items []string) []string {
	if len(items) == 0 {
		return items
	}
	out := items[:1]
	for _, item := range items[1:] {
		if item != out[len(out)-1] {
			out = append(out, item)
		}
	}
	return out
}

// displaySeverity treats confirmed rendered clipping/overflow as errors.
// Missing glyphs, missing catalog keys, and unsupported controls/modes are
// advisory: the Godot report itself documents them as inconclusive or already
// covered by catalog checks.
func displaySeverity(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "clipped", "overflow":
		return "error"
	default:
		return "warning"
	}
}
