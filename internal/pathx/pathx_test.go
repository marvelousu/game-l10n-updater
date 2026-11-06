package pathx

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateLocale(t *testing.T) {
	valid := []string{"en", "ja", "zh-hans", "zh_Hant", "pt-BR", "a1"}
	for _, locale := range valid {
		if err := ValidateLocale(locale); err != nil {
			t.Errorf("ValidateLocale(%q) = %v", locale, err)
		}
	}
	invalid := []string{"", "../en", "en/us", `en\us`, ".", "en US", "en.json", "-en"}
	for _, locale := range invalid {
		if err := ValidateLocale(locale); err == nil {
			t.Errorf("ValidateLocale(%q) unexpectedly succeeded", locale)
		}
	}
}

func TestSafeJoinRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	good, err := SafeJoin(root, "targets/en.json")
	if err != nil {
		t.Fatal(err)
	}
	if !Inside(root, good) {
		t.Fatalf("%s is not inside %s", good, root)
	}
	bad := []string{
		"../escape.json",
		"targets/../../escape.json",
		`targets\..\..\escape.json`,
		filepath.Join(root, "absolute.json"),
		"",
	}
	for _, rel := range bad {
		if _, err := SafeJoin(root, rel); err == nil {
			t.Errorf("SafeJoin(%q) unexpectedly succeeded", rel)
		}
	}
}

func TestSafeTargetFile(t *testing.T) {
	root := t.TempDir()
	path, err := SafeTargetFile(root, "targets", "zh-hans")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, filepath.Join("targets", "zh-hans.json")) {
		t.Fatalf("unexpected path: %s", path)
	}
	if _, err := SafeTargetFile(root, "targets", "../en"); err == nil {
		t.Fatal("traversal locale accepted")
	}
}
