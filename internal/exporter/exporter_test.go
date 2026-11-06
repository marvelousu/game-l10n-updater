package exporter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/project"
)

func fixtureProject() *project.Project {
	return &project.Project{
		Config: model.Config{
			Source:    model.SourceConfig{Language: "ja"},
			Languages: []string{"en", "zh-hans"},
		},
		Source: &model.SourceCatalog{SchemaVersion: 1, Entries: []model.SourceEntry{
			{Key: "b", Text: "こんにちは <b>{name}</b>"},
			{Key: "a", Text: "A"},
		}},
		Targets: map[string]map[string]string{
			"en":      {"a": "A en", "b": "Hello <b>{name}</b>", "deleted": "still here"},
			"zh-hans": {"a": "A zh"},
		},
	}
}

func TestBuildJSONStableUnicode(t *testing.T) {
	p := fixtureProject()
	data, err := BuildJSON(p, []string{"en", "zh-hans"})
	if err != nil {
		t.Fatal(err)
	}
	var doc JSONDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SourceLanguage != "ja" || doc.Languages["ja"]["b"] != "こんにちは <b>{name}</b>" {
		t.Fatalf("unexpected doc: %#v", doc)
	}
	if doc.Languages["en"]["deleted"] != "still here" {
		t.Fatal("deleted target key not exported")
	}
	text := string(data)
	if !strings.Contains(text, "こんにちは") || !strings.Contains(text, "<b>") {
		t.Fatalf("Unicode/markup escaped:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("missing trailing newline")
	}
}

func TestBuildCSVColumnsAndOrder(t *testing.T) {
	p := fixtureProject()
	data, err := BuildCSV(p, []string{"en", "zh-hans"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "keys,ja,en,zh-hans\n") {
		t.Fatalf("unexpected header:\n%s", text)
	}
	if !strings.Contains(text, "deleted,still here") && !strings.Contains(text, "deleted,,still here") {
		// deleted is a target-only key; source cell is empty.
		t.Fatalf("deleted key row missing:\n%s", text)
	}
}

func TestSelectLocales(t *testing.T) {
	p := fixtureProject()
	out, err := SelectLocales(p, nil)
	if err != nil || len(out) != 2 || out[0] != "en" {
		t.Fatalf("unexpected locales: %#v %v", out, err)
	}
	out, err = SelectLocales(p, []string{"zh-hans", "zh-hans"})
	if err != nil || len(out) != 1 || out[0] != "zh-hans" {
		t.Fatalf("unexpected locales: %#v %v", out, err)
	}
	if _, err := SelectLocales(p, []string{"fr"}); err == nil {
		t.Fatal("unconfigured locale accepted")
	}
}
