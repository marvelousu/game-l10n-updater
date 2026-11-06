package target

import (
	"strings"
	"testing"
)

func TestParseFlatJSON(t *testing.T) {
	m, err := Parse([]byte(`{"z":"こんにちは","a":"A <b>{name}</b>"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["z"] != "こんにちは" || m["a"] != "A <b>{name}</b>" {
		t.Fatalf("unexpected map: %#v", m)
	}
}

func TestParseRejectsStructuralProblems(t *testing.T) {
	cases := []string{
		`null`,
		`[]`,
		`{"a":"x","a":"y"}`,
		`{"a":1}`,
		`{"a":"x"} trailing`,
		``,
	}
	for _, input := range cases {
		if _, err := Parse([]byte(input)); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", input)
		}
	}
}

func TestMarshalStableUnicode(t *testing.T) {
	data, err := Marshal(map[string]string{
		"b": "こんにちは \"quoted\" <tag>",
		"a": "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Index(text, `"a"`) > strings.Index(text, `"b"`) {
		t.Fatalf("keys not sorted:\n%s", text)
	}
	if !strings.Contains(text, "こんにちは") || !strings.Contains(text, "<tag>") {
		t.Fatalf("Unicode/markup was escaped:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("missing trailing newline")
	}
}
