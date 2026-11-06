package check

import (
	"testing"

	"github.com/marvelousu/game-l10n-updater/internal/display"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/project"
)

func fixtureProject() *project.Project {
	cat := &model.SourceCatalog{SchemaVersion: 1, Entries: []model.SourceEntry{
		{Key: "greet", Text: "Hello {name}"},
		{Key: "count", Text: "%d items"},
		{Key: "html", Text: "<b>bold</b>"},
		{Key: "long", Text: "Short", MaxLength: 3},
		{Key: "missing", Text: "Missing"},
	}}
	p := &project.Project{
		Config: model.Config{Source: model.SourceConfig{Language: "ja"}, Languages: []string{"en"}},
		Source: cat,
		Targets: map[string]map[string]string{
			"en": {
				"greet": "こんにちは", // missing {name}
				"count": "%d 個",
				"html":  "<b>太字</b>",
				"long":  "too long",
				// missing intentionally absent
			},
		},
	}
	p.SourceMap, p.SourceKeys = map[string]model.SourceEntry{}, []string{}
	for _, entry := range cat.Entries {
		p.SourceMap[entry.Key] = entry
		p.SourceKeys = append(p.SourceKeys, entry.Key)
	}
	return p
}

func issueByKey(report *Report, key string) *model.Issue {
	for i := range report.Issues {
		if report.Issues[i].Key == key {
			return &report.Issues[i]
		}
	}
	return nil
}

func TestBuildFindsStructuralWarningsAndMissing(t *testing.T) {
	p := fixtureProject()
	report, err := Build(p, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Checked != 5 {
		t.Fatalf("checked = %d", report.Checked)
	}
	greet := issueByKey(report, "greet")
	if greet == nil || greet.Kind != "placeholder_mismatch" || greet.Severity != "error" || greet.Category != "brace" {
		t.Fatalf("unexpected greet issue: %#v", greet)
	}
	long := issueByKey(report, "long")
	if long == nil || long.Kind != "max_length" || long.Severity != "warning" {
		t.Fatalf("unexpected long issue: %#v", long)
	}
	missing := issueByKey(report, "missing")
	if missing == nil || missing.Kind != "missing_target" || missing.Severity != "warning" {
		t.Fatalf("unexpected missing issue: %#v", missing)
	}
	if report.Counts.Structural != 1 || report.Counts.Display != 0 {
		t.Fatalf("unexpected counts: %#v", report.Counts)
	}
	if report.ExitCode() != 3 {
		t.Fatalf("exit = %d, want 3", report.ExitCode())
	}
}

func TestBuildMergesDisplayReport(t *testing.T) {
	p := fixtureProject()
	displayReport := &display.Report{SchemaVersion: 1, Checked: 12, Issues: []display.Issue{
		{Key: "greet", Locale: "en", NodePath: "Main/Label", Kind: "overflow", Message: "text overflows"},
		{Key: "unknown", Locale: "en", NodePath: "Main/Other", Kind: "clipped", Message: "unknown key"},
		{Key: "greet", Locale: "ja", NodePath: "Main/Label", Kind: "overflow", Message: "filtered locale"},
	}}
	report, err := Build(p, displayReport, "reports/display.json", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Display == nil || report.Display.Checked != 12 || report.Display.IssueCount != 3 {
		t.Fatalf("unexpected display info: %#v", report.Display)
	}
	if len(report.Display.UnknownKeys) != 1 || report.Display.UnknownKeys[0] != "unknown" {
		t.Fatalf("unknown keys = %#v", report.Display.UnknownKeys)
	}
	displayIssues := 0
	for _, issue := range report.Issues {
		if issue.Source == "display" {
			displayIssues++
			if issue.Severity != "error" || issue.NodePath == "" {
				t.Fatalf("unexpected display issue: %#v", issue)
			}
		}
	}
	if displayIssues != 2 {
		t.Fatalf("display issues = %d, want 2", displayIssues)
	}
	if report.Counts.Display != 2 {
		t.Fatalf("display count = %d", report.Counts.Display)
	}
}

func TestWarningsOnlyExitZero(t *testing.T) {
	p := &project.Project{
		Config: model.Config{Source: model.SourceConfig{Language: "ja"}, Languages: []string{"en"}},
		Source: &model.SourceCatalog{SchemaVersion: 1, Entries: []model.SourceEntry{
			{Key: "k", Text: "Hello", MaxLength: 2},
		}},
		Targets: map[string]map[string]string{"en": {"k": "Hello"}},
		SourceMap: map[string]model.SourceEntry{
			"k": {Key: "k", Text: "Hello", MaxLength: 2},
		},
		SourceKeys: []string{"k"},
	}
	report, err := Build(p, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts.Structural != 0 || report.Counts.Warnings != 1 {
		t.Fatalf("unexpected counts: %#v", report.Counts)
	}
	if report.ExitCode() != 0 {
		t.Fatalf("warnings must exit 0, got %d", report.ExitCode())
	}
}

func TestDisplayOnlyExitFour(t *testing.T) {
	p := &project.Project{
		Config: model.Config{Source: model.SourceConfig{Language: "ja"}, Languages: []string{"en"}},
		Source: &model.SourceCatalog{SchemaVersion: 1, Entries: []model.SourceEntry{
			{Key: "k", Text: "Hello"},
		}},
		Targets:    map[string]map[string]string{"en": {"k": "Hello"}},
		SourceMap:  map[string]model.SourceEntry{"k": {Key: "k", Text: "Hello"}},
		SourceKeys: []string{"k"},
	}
	report, err := Build(p, &display.Report{SchemaVersion: 1, Checked: 1, Issues: []display.Issue{
		{Key: "k", Locale: "en", NodePath: "Label", Kind: "overflow", Message: "overflow"},
	}}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.ExitCode() != 4 {
		t.Fatalf("exit = %d, want 4", report.ExitCode())
	}
}

func TestSelectedLocaleMustBeConfigured(t *testing.T) {
	p := fixtureProject()
	if _, err := Build(p, nil, "", []string{"fr"}); err == nil {
		t.Fatal("unconfigured locale accepted")
	}
}

func TestDisplayAdvisoryKindsAreWarnings(t *testing.T) {
	p := &project.Project{
		Config: model.Config{Source: model.SourceConfig{Language: "ja"}, Languages: []string{"en"}},
		Source: &model.SourceCatalog{SchemaVersion: 1, Entries: []model.SourceEntry{
			{Key: "k", Text: "Hello"},
		}},
		Targets:    map[string]map[string]string{"en": {"k": "Hello"}},
		SourceMap:  map[string]model.SourceEntry{"k": {Key: "k", Text: "Hello"}},
		SourceKeys: []string{"k"},
	}
	report, err := Build(p, &display.Report{SchemaVersion: 1, Checked: 3, Issues: []display.Issue{
		{Key: "k", Locale: "en", NodePath: "Label", Kind: "missing_glyph", Message: "font chain"},
		{Key: "k", Locale: "en", NodePath: "Label", Kind: "unsupported_mode", Message: "scrollable"},
	}}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts.Display != 0 || report.Counts.Warnings != 2 {
		t.Fatalf("advisory display kinds should be warnings: %#v", report.Counts)
	}
	if report.ExitCode() != 0 {
		t.Fatalf("advisory display kinds must exit 0, got %d", report.ExitCode())
	}
}

func TestDisplayUnknownLocaleIsReportedWhenNotFiltered(t *testing.T) {
	p := &project.Project{
		Config: model.Config{Source: model.SourceConfig{Language: "ja"}, Languages: []string{"en"}},
		Source: &model.SourceCatalog{SchemaVersion: 1, Entries: []model.SourceEntry{
			{Key: "k", Text: "Hello"},
		}},
		Targets:    map[string]map[string]string{"en": {"k": "Hello"}},
		SourceMap:  map[string]model.SourceEntry{"k": {Key: "k", Text: "Hello"}},
		SourceKeys: []string{"k"},
	}
	report, err := Build(p, &display.Report{SchemaVersion: 1, Checked: 1, Issues: []display.Issue{
		{Key: "k", Locale: "fr", NodePath: "Label", Kind: "overflow", Message: "overflow"},
	}}, "display.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Display == nil || len(report.Display.UnknownLocales) != 1 || report.Display.UnknownLocales[0] != "fr" {
		t.Fatalf("unknown locale not reported: %#v", report.Display)
	}
	if report.Counts.Display != 1 || report.ExitCode() != 4 {
		t.Fatalf("unexpected display result: %#v", report.Counts)
	}

	filtered, err := Build(p, &display.Report{SchemaVersion: 1, Checked: 1, Issues: []display.Issue{
		{Key: "k", Locale: "fr", NodePath: "Label", Kind: "overflow", Message: "overflow"},
	}}, "display.json", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Counts.Display != 0 || filtered.ExitCode() != 0 {
		t.Fatalf("locale filter did not exclude display issue: %#v", filtered.Counts)
	}
}
