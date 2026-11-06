package jsonx

import (
	"strings"
	"testing"
)

func TestDecodeStrictRejectsUnknownAndTrailing(t *testing.T) {
	var out struct {
		A int `json:"a"`
	}
	if err := DecodeStrict([]byte(`{"a":1}`), &out); err != nil {
		t.Fatal(err)
	}
	if err := DecodeStrict([]byte(`{"a":1,"b":2}`), &out); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := DecodeStrict([]byte(`{"a":1} {}`), &out); err == nil {
		t.Fatal("trailing value accepted")
	}
}

func TestMarshalStable(t *testing.T) {
	data, err := MarshalStable(map[string]string{"b": "<tag>", "a": "こんにちは"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Index(text, `"a"`) > strings.Index(text, `"b"`) {
		t.Fatalf("keys not sorted:\n%s", text)
	}
	if !strings.Contains(text, "<tag>") || !strings.Contains(text, "こんにちは") {
		t.Fatalf("content escaped:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("missing trailing newline")
	}
}
