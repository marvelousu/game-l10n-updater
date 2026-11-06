package catalog

import (
	"strings"
	"testing"

	"github.com/marvelousu/game-l10n-updater/internal/model"
)

func TestParseArrayAndObject(t *testing.T) {
	array := []byte(`[{"key":"b","text":"B"},{"key":"a","text":"A","context":"button","max_length":4}]`)
	cat, err := Parse(array)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Entries) != 2 || cat.SchemaVersion != 1 {
		t.Fatalf("unexpected catalog: %#v", cat)
	}
	obj := []byte(`{"schema_version":1,"entries":[{"key":"a","text":"A","context":"button","max_length":4}]}`)
	cat, err = Parse(obj)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Entries) != 1 {
		t.Fatalf("unexpected catalog: %#v", cat)
	}
}

func TestParseRejectsStructuralProblems(t *testing.T) {
	cases := []string{
		`{"schema_version":2,"entries":[]}`,
		`{"schema_version":1,"entries":[{"key":"","text":"x"}]}`,
		`{"schema_version":1,"entries":[{"key":"a","text":"x"},{"key":"a","text":"y"}]}`,
		`{"schema_version":1,"entries":[{"key":"a","text":"x","max_length":-1}]}`,
		`{"schema_version":1,"entries":[{"key":"a","text":"x","unknown":1}]}`,
		`{"schema_version":1,"entries":[]} trailing`,
	}
	for _, input := range cases {
		if _, err := Parse([]byte(input)); err == nil {
			t.Errorf("Parse(%s) unexpectedly succeeded", input)
		}
	}
}

func TestMarshalStableAndUnicode(t *testing.T) {
	cat := &model.SourceCatalog{SchemaVersion: 1, Entries: []model.SourceEntry{
		{Key: "b", Text: "こんにちは <b>{name}</b>"},
		{Key: "a", Text: "A"},
	}}
	data, err := Marshal(cat)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Index(text, `"a"`) > strings.Index(text, `"b"`) {
		t.Fatalf("entries not sorted:\n%s", text)
	}
	if !strings.Contains(text, "こんにちは") || !strings.Contains(text, "<b>") {
		t.Fatalf("Unicode/markup was escaped:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("missing trailing newline")
	}
}
