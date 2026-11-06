// Package pathx validates locale codes and keeps project-relative paths
// inside the project root.
package pathx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateLocale accepts a conservative locale code usable as a file name,
// e.g. en, ja, zh-hans, zh_Hant, pt-BR. Path separators, dots and whitespace
// are rejected.
func ValidateLocale(locale string) error {
	if locale == "" {
		return fmt.Errorf("locale must not be empty")
	}
	if len(locale) > 64 {
		return fmt.Errorf("locale %q is too long", locale)
	}
	for i, r := range locale {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("locale %q contains invalid character %q", locale, r)
		}
		if i == 0 && !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return fmt.Errorf("locale %q must start with a letter or digit", locale)
		}
	}
	return nil
}

// SafeJoin resolves rel under root and fails if rel is absolute or escapes
// root. Both slash styles are treated as separators so a config written on
// Windows is still checked when read on Unix.
func SafeJoin(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("path must not contain NUL")
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("path %q must be relative", rel)
	}

	normalized := strings.ReplaceAll(rel, "\\", "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("path %q must not traverse upward", rel)
		}
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	target := filepath.Join(rootAbs, filepath.FromSlash(normalized))
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", rel, err)
	}
	relToRoot, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("check path %q: %w", rel, err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes project root", rel)
	}
	return targetAbs, nil
}

// SafeTargetFile returns the flat JSON target path for locale under dir.
func SafeTargetFile(root, dir, locale string) (string, error) {
	if err := ValidateLocale(locale); err != nil {
		return "", err
	}
	return SafeJoin(root, filepath.ToSlash(filepath.Join(dir, locale+".json")))
}

// RootOf returns the absolute directory containing path.
func RootOf(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Dir(abs), nil
}

// Inside reports whether path is root itself or below root.
func Inside(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// EnsureDir creates a directory if needed and reports whether it existed.
func EnsureDir(path string) (created bool, err error) {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("%s is not a directory", path)
		}
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return false, err
	}
	return true, nil
}
