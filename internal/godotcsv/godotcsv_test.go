package godotcsv

import (
	"strings"
	"testing"
)

func TestImportAndExportRoundTrip(t *testing.T) {
	input := "keys,ja,en,zh-hans\n" +
		"ui.start,はじめから,Start,开始\n" +
		"ui.quote,\"カンマ, と \"\"引用\"\"\",\"Comma, and \"\"quote\"\"\",\n" +
		"ui.multi,\"行1\n行2\",\"line1\nline2\",\n"
	cat, targets, locales, err := Import([]byte(input), "ja")
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Entries) != 3 {
		t.Fatalf("entries = %d", len(cat.Entries))
	}
	if cat.Entries[0].Key != "ui.multi" {
		t.Fatalf("entries not sorted: %#v", cat.Entries)
	}
	if targets["en"]["ui.quote"] != "Comma, and \"quote\"" {
		t.Fatalf("quote not preserved: %q", targets["en"]["ui.quote"])
	}
	if targets["zh-hans"]["ui.start"] != "开始" {
		t.Fatalf("Unicode not preserved: %q", targets["zh-hans"]["ui.start"])
	}
	if len(locales) != 2 || locales[0] != "en" || locales[1] != "zh-hans" {
		t.Fatalf("locales = %#v", locales)
	}
	if _, ok := targets["zh-hans"]["ui.multi"]; ok {
		t.Fatal("empty target cell should be omitted")
	}

	source := map[string]string{"ui.start": "はじめから", "ui.multi": "行1\n行2"}
	exported, err := Export("ja", []string{"en", "zh-hans"}, source, targets)
	if err != nil {
		t.Fatal(err)
	}
	text := string(exported)
	if !strings.HasPrefix(text, "keys,ja,en,zh-hans\n") {
		t.Fatalf("unexpected header:\n%s", text)
	}
	if !strings.Contains(text, "開始") && !strings.Contains(text, "はじめから") {
		t.Fatalf("source Unicode missing:\n%s", text)
	}
	if !strings.Contains(text, "\"Comma, and \"\"quote\"\"\"") {
		t.Fatalf("CSV quoting missing:\n%s", text)
	}
	// Re-import the export.
	if _, _, _, err := Import(exported, "ja"); err != nil {
		t.Fatalf("exported CSV does not re-import: %v", err)
	}
}

func TestImportRejectsBadHeadersAndRows(t *testing.T) {
	cases := []string{
		"",
		"keys,ja,ja\na,x,y\n",
		"keys,ja,en\na,x,y\nb,only-one\n",
		"keys,ja,en\n,x,y\n",
		"keys,ja,en\na,x,y\na,z,w\n",
	}
	for _, input := range cases {
		if _, _, _, err := Import([]byte(input), "ja"); err == nil {
			t.Errorf("Import(%q) unexpectedly succeeded", input)
		}
	}
}

func TestImportRequiresSourceColumn(t *testing.T) {
	if _, _, _, err := Import([]byte("keys,en\na,x\n"), "ja"); err == nil {
		t.Fatal("missing source column accepted")
	}
}

func TestImportAcceptsKeyHeader(t *testing.T) {
	cat, targets, locales, err := Import([]byte("key,ja,en\na,あ,A\n"), "ja")
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Entries) != 1 || targets["en"]["a"] != "A" || len(locales) != 1 {
		t.Fatalf("unexpected import: %#v %#v %#v", cat, targets, locales)
	}
}

func TestImportRejectsTraversalLocale(t *testing.T) {
	if _, _, _, err := Import([]byte("keys,ja,../en\na,x,y\n"), "ja"); err == nil {
		t.Fatal("traversal locale accepted")
	}
}

func TestExportIncludesDeletedTargetKeys(t *testing.T) {
	data, err := Export("ja", []string{"en"}, map[string]string{"a": "A"}, map[string]map[string]string{
		"en": {"a": "A en", "deleted": "kept en"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "deleted,,") && !strings.Contains(string(data), "deleted") {
		t.Fatalf("deleted target key missing:\n%s", data)
	}
}
