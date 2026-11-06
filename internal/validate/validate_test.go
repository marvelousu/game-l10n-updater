package validate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/marvelousu/game-l10n-updater/internal/model"
)

func TestExtractEscapesAndLiterals(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		tokens map[string]int
	}{
		{"escaped braces", "{{name}} {{0}}", map[string]int{}},
		{"single brace", "{name} {0}", map[string]int{"{name}": 1, "{0}": 1}},
		{"triple brace", "{{{name}}}", map[string]int{"{name}": 1}},
		{"percent literal", "100%% %s", map[string]int{"%s": 1}},
		{"single percent text", "100% success", map[string]int{}},
		{"percent positional", "%1$s %2$d", map[string]int{"%1$s": 1, "%2$d": 1}},
		{"percent width", "%02d %.2f", map[string]int{"%02d": 1, "%.2f": 1}},
		{"html", "<b>x</b><br/>", map[string]int{"<b>": 1, "</b>": 1, "<br>": 1}},
		{"html attributes", `<a href="x">x</a>`, map[string]int{"<a>": 1, "</a>": 1}},
		{"bbcode", "[b]x[/b][color=red]y[/color]", map[string]int{"[b]": 1, "[/b]": 1, "[color]": 1, "[/color]": 1}},
		{"unknown bracket label stays literal", "[Rare] <Player>", map[string]int{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Extract(tc.input)
			all := map[string]int{}
			for _, m := range []map[string]int{got.Brace, got.Percent, got.HTML, got.BBCode} {
				for token, count := range m {
					all[token] += count
				}
			}
			if !reflect.DeepEqual(all, tc.tokens) {
				t.Fatalf("Extract(%q) = %#v, want %#v", tc.input, all, tc.tokens)
			}
		})
	}
}

func TestMismatchesExactMultisets(t *testing.T) {
	cases := []struct {
		name   string
		source string
		target string
		want   []Mismatch
	}{
		{"order ignored", "{name} {0}", "{0} {name}", nil},
		{"missing brace", "{name}", "Name", []Mismatch{{Category: "brace", Missing: []string{"{name}"}}}},
		{"duplicate brace", "{0}", "{0}{0}", []Mismatch{{Category: "brace", Extra: []string{"{0}"}}}},
		{"different brace", "{name}", "{nom}", []Mismatch{{Category: "brace", Missing: []string{"{name}"}, Extra: []string{"{nom}"}}}},
		{"escaped braces are literals", "{{name}}", "{{nom}}", nil},
		{"escaped brace is not a placeholder", "{{name}}", "name", nil},
		{"percent missing", "Hi %s", "Hi", []Mismatch{{Category: "percent", Missing: []string{"%s"}}}},
		{"positional differs", "%1$s", "%2$s", []Mismatch{{Category: "percent", Missing: []string{"%1$s"}, Extra: []string{"%2$s"}}}},
		{"positional and plain differ", "%1$s", "%s", []Mismatch{{Category: "percent", Missing: []string{"%1$s"}, Extra: []string{"%s"}}}},
		{"percent literal", "100%%", "100%", nil},
		{"width differs", "%02d", "%d", []Mismatch{{Category: "percent", Missing: []string{"%02d"}, Extra: []string{"%d"}}}},
		{"percent count", "%s %s", "%s", []Mismatch{{Category: "percent", Missing: []string{"%s"}}}},
		{"html missing close", "<b>x</b>", "<b>x", []Mismatch{{Category: "html", Missing: []string{"</b>"}}}},
		{"html order ignored", "<b>x</b>", "</b>x<b>", nil},
		{"html case ignored", "<B>x</B>", "<b>x</b>", nil},
		{"html self close equals open", "<br/>", "<br>", nil},
		{"html attributes ignored", `<a href="x">x</a>`, `<a>x</a>`, nil},
		{"bbcode missing close", "[b]x[/b]", "[b]x", []Mismatch{{Category: "bbcode", Missing: []string{"[/b]"}}}},
		{"bbcode case ignored", "[B]x[/B]", "[b]x[/b]", nil},
		{"bbcode attributes ignored", "[color=red]x[/color]", "[color=blue]x[/color]", nil},
		{"unknown bracket label is literal", "[Rare] Sword", "[希少] Sword", nil},
		{"unknown angle label is literal", "<Player> joined", "<プレイヤー> joined", nil},
		{"mixed", "{name} %s <b>x</b> [i]y[/i]", "{name} <b>x</b> [i]y[/i]", []Mismatch{{Category: "percent", Missing: []string{"%s"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Mismatches(tc.source, tc.target)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Mismatches(%q, %q) = %#v, want %#v", tc.source, tc.target, got, tc.want)
			}
		})
	}
}

func TestIssuesCarryKeyLocaleAndCategory(t *testing.T) {
	issues := Issues("Hi {name}", "Hi", "greeting", "en")
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	issue := issues[0]
	if issue.Key != "greeting" || issue.Locale != "en" || issue.Kind != "placeholder_mismatch" || issue.Category != "brace" {
		t.Fatalf("unexpected issue: %#v", issue)
	}
	if !strings.Contains(issue.Message, "missing {name}") {
		t.Fatalf("message does not identify missing token: %s", issue.Message)
	}
}

func TestMaxLengthRuneWarning(t *testing.T) {
	entry := model.SourceEntry{Key: "k", Text: "x", MaxLength: 3}
	if issue := MaxLengthIssue(entry, "あいう", "en"); issue != nil {
		t.Fatalf("equal rune count should pass: %#v", issue)
	}
	issue := MaxLengthIssue(entry, "あいうえ", "ja")
	if issue == nil {
		t.Fatal("expected warning")
	}
	if issue.Kind != "max_length" || issue.Severity != "warning" {
		t.Fatalf("unexpected issue: %#v", issue)
	}
	if !strings.Contains(issue.Message, "rune warning only") {
		t.Fatalf("message must distinguish rune warning: %s", issue.Message)
	}
	if issue := MaxLengthIssue(model.SourceEntry{Key: "k"}, "very long", "en"); issue != nil {
		t.Fatalf("missing max_length should not warn: %#v", issue)
	}
}
