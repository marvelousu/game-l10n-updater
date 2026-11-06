// Package validate compares placeholder multisets and rune lengths.
package validate

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/marvelousu/game-l10n-updater/internal/model"
)

// Mismatch is a placeholder/tag category difference.
type Mismatch struct {
	Category string
	Missing  []string
	Extra    []string
}

// Tokens is the counted placeholder/markup token set of one string.
type Tokens struct {
	Brace   map[string]int
	Percent map[string]int
	HTML    map[string]int
	BBCode  map[string]int
}

func newTokens() Tokens {
	return Tokens{
		Brace:   map[string]int{},
		Percent: map[string]int{},
		HTML:    map[string]int{},
		BBCode:  map[string]int{},
	}
}

// Extract tokenizes a string. Escaped braces ({{ and }}) and percent literal
// (%% ) are skipped, never counted as placeholders.
func Extract(s string) Tokens {
	tokens := newTokens()
	extractBraces(s, tokens.Brace)
	extractPercents(s, tokens.Percent)
	extractHTML(s, tokens.HTML)
	extractBBCode(s, tokens.BBCode)
	return tokens
}

func extractBraces(s string, out map[string]int) {
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '{' {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && isBraceChar(s[j]) {
			j++
		}
		if j > i+1 && j < len(s) && s[j] == '}' {
			out[s[i:j+1]]++
			i = j
		}
	}
}

func isBraceChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
		b == '_' || b == '.' || b == '-'
}

func extractPercents(s string, out map[string]int) {
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		// Positional prefix, e.g. %1$s.
		if j < len(s) && isDigit(s[j]) {
			k := j
			for k < len(s) && isDigit(s[k]) {
				k++
			}
			if k < len(s) && s[k] == '$' {
				j = k + 1
			}
		}
		for j < len(s) && strings.ContainsRune("-+0#", rune(s[j])) {
			j++
		}
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		if j < len(s) && s[j] == '.' {
			j++
			for j < len(s) && isDigit(s[j]) {
				j++
			}
		}
		for j < len(s) && strings.ContainsRune("hlLqjzt", rune(s[j])) {
			j++
		}
		if j < len(s) && strings.ContainsRune("sdifuxXocGgEeFfavA", rune(s[j])) {
			out[s[i:j+1]]++
			i = j
		}
	}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func extractHTML(s string, out map[string]int) {
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		end := strings.IndexByte(s[i+1:], '>')
		if end < 0 {
			return
		}
		content := s[i+1 : i+1+end]
		if token, ok := canonicalHTMLTag(content); ok {
			out[token]++
			i += end + 1
		}
	}
}

func canonicalHTMLTag(content string) (string, bool) {
	content = strings.TrimSpace(content)
	if content == "" || strings.HasPrefix(content, "!") || strings.HasPrefix(content, "?") {
		return "", false
	}
	closing := strings.HasPrefix(content, "/")
	name := strings.TrimPrefix(content, "/")
	if idx := strings.IndexAny(name, " \t\r\n"); idx >= 0 {
		name = name[:idx]
	}
	name = strings.TrimSuffix(name, "/")
	name = strings.ToLower(name)
	if !validTagName(name) || !knownHTMLTag(name) {
		return "", false
	}
	if closing {
		return "</" + name + ">", true
	}
	return "<" + name + ">", true
}

// Only a basic, documented HTML subset is treated as markup. Game text such as
// "<Player>" stays literal instead of becoming a false placeholder.
func knownHTMLTag(name string) bool {
	switch name {
	case "a", "abbr", "b", "blockquote", "br", "code", "del", "div", "em", "font",
		"h1", "h2", "h3", "h4", "h5", "h6", "hr", "i", "img", "ins", "li", "ol",
		"p", "pre", "s", "small", "span", "strike", "strong", "sub", "sup", "u", "ul":
		return true
	}
	return false
}

func extractBBCode(s string, out map[string]int) {
	for i := 0; i < len(s); i++ {
		if s[i] != '[' {
			continue
		}
		end := strings.IndexByte(s[i+1:], ']')
		if end < 0 {
			return
		}
		content := s[i+1 : i+1+end]
		if token, ok := canonicalBBCodeTag(content); ok {
			out[token]++
			i += end + 1
		}
	}
}

func canonicalBBCodeTag(content string) (string, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", false
	}
	closing := strings.HasPrefix(content, "/")
	name := strings.TrimPrefix(content, "/")
	if idx := strings.IndexAny(name, "= \t\r\n"); idx >= 0 {
		name = name[:idx]
	}
	name = strings.ToLower(name)
	if !validTagName(name) || !knownBBCodeTag(name) {
		return "", false
	}
	if closing {
		return "[/" + name + "]", true
	}
	return "[" + name + "]", true
}

// Basic Godot RichTextLabel BBCode tags. Unknown bracket labels such as
// "[Rare]" in item text remain literal instead of becoming false positives.
func knownBBCodeTag(name string) bool {
	switch name {
	case "b", "i", "u", "s", "code", "indent", "url", "img", "color", "bgcolor",
		"fgcolor", "font", "font_size", "center", "left", "right", "fill", "list",
		"ul", "ol", "table", "cell", "dropcap", "p", "wave", "tornado", "shake",
		"fade", "rainbow", "pulse", "opentype", "outline_size", "outline_color",
		"paragraph", "tooltip", "hint":
		return true
	}
	return false
}

func validTagName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		ok := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_'
		if !ok {
			return false
		}
	}
	return true
}

// Mismatches compares placeholder multisets in a stable category order.
func Mismatches(source, target string) []Mismatch {
	src := Extract(source)
	dst := Extract(target)
	type pair struct {
		name string
		a    map[string]int
		b    map[string]int
	}
	pairs := []pair{
		{"brace", src.Brace, dst.Brace},
		{"percent", src.Percent, dst.Percent},
		{"html", src.HTML, dst.HTML},
		{"bbcode", src.BBCode, dst.BBCode},
	}
	var out []Mismatch
	for _, p := range pairs {
		missing := difference(p.a, p.b)
		extra := difference(p.b, p.a)
		if len(missing) > 0 || len(extra) > 0 {
			out = append(out, Mismatch{Category: p.name, Missing: missing, Extra: extra})
		}
	}
	return out
}

func difference(a, b map[string]int) []string {
	var out []string
	for token, count := range a {
		if count > b[token] {
			out = append(out, token)
		}
	}
	sort.Strings(out)
	return out
}

// Issues converts mismatches into report issues for one key/locale.
func Issues(source, target, key, locale string) []model.Issue {
	mismatches := Mismatches(source, target)
	issues := make([]model.Issue, 0, len(mismatches))
	for _, mismatch := range mismatches {
		msg := mismatchMessage(mismatch)
		issues = append(issues, model.Issue{
			Key:      key,
			Locale:   locale,
			Kind:     "placeholder_mismatch",
			Severity: "error",
			Category: mismatch.Category,
			Message:  msg,
			Source:   "catalog",
			Missing:  mismatch.Missing,
			Extra:    mismatch.Extra,
		})
	}
	return issues
}

func mismatchMessage(m Mismatch) string {
	var parts []string
	if len(m.Missing) > 0 {
		parts = append(parts, "missing "+strings.Join(m.Missing, " "))
	}
	if len(m.Extra) > 0 {
		parts = append(parts, "extra "+strings.Join(m.Extra, " "))
	}
	return fmt.Sprintf("%s placeholder multiset differs: %s", m.Category, strings.Join(parts, "; "))
}

// MaxLengthIssue returns a rune-length warning, or nil. It is deliberately a
// warning: rune count is not proof of rendered pixel fit.
func MaxLengthIssue(entry model.SourceEntry, target, locale string) *model.Issue {
	if entry.MaxLength <= 0 {
		return nil
	}
	count := utf8.RuneCountInString(target)
	if count <= entry.MaxLength {
		return nil
	}
	return &model.Issue{
		Key:      entry.Key,
		Locale:   locale,
		Kind:     "max_length",
		Severity: "warning",
		Source:   "catalog",
		Message:  fmt.Sprintf("target is %d runes; max_length is %d (rune warning only, not rendered pixel fit)", count, entry.MaxLength),
	}
}
