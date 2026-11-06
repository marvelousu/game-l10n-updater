package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marvelousu/game-l10n-updater/internal/model"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gamel10n.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultIsValid(t *testing.T) {
	cfg := Default("ja", []string{"en", "zh-hans"})
	if err := Validate(cfg); err != nil {
		t.Fatalf("Default() invalid: %v", err)
	}
}

func TestLoadRejectsRemoteAPI(t *testing.T) {
	cfg := Default("ja", []string{"en"})
	cfg.API.URL = "http://example.com:8787"
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := writeConfig(t, string(data))
	_, err = Load(path)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("want loopback error, got %v", err)
	}
}

func TestLoadRejectsUnsafePaths(t *testing.T) {
	cfg := Default("ja", []string{"en"})
	cfg.Targets.State = "../state.json"
	data, _ := Marshal(cfg)
	path := writeConfig(t, string(data))
	if _, err := Load(path); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestLoadRejectsDuplicateLanguage(t *testing.T) {
	cfg := Default("ja", []string{"en", "en"})
	data, _ := Marshal(cfg)
	path := writeConfig(t, string(data))
	if _, err := Load(path); err == nil {
		t.Fatal("duplicate language accepted")
	}
}

func TestLoadRejectsUnknownFieldAndSchema(t *testing.T) {
	path := writeConfig(t, `{"schema_version":1,"source":{"language":"ja","path":"source/catalog.json"},"targets":{"directory":"targets","state":"state/s.json"},"languages":["en"],"api":{"url":"http://127.0.0.1:8787"},"extra":true}`)
	if _, err := Load(path); err == nil {
		t.Fatal("unknown field accepted")
	}
	path = writeConfig(t, `{"schema_version":2,"source":{"language":"ja","path":"source/catalog.json"},"targets":{"directory":"targets","state":"state/s.json"},"languages":["en"],"api":{"url":"http://127.0.0.1:8787"}}`)
	if _, err := Load(path); err == nil {
		t.Fatal("schema version 2 accepted")
	}
}

func TestLoadFillsDefaults(t *testing.T) {
	path := writeConfig(t, `{"schema_version":1,"source":{"language":"ja","path":"source/catalog.json"},"targets":{"directory":"targets","state":"state/s.json"},"languages":["en"],"api":{"url":"http://127.0.0.1:8787"}}`)
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Config.API.TimeoutSeconds != 120 || loaded.Config.API.MaxTokens != 512 {
		t.Fatalf("defaults not applied: %#v", loaded.Config.API)
	}
	if loaded.Config.Source.Language != "ja" {
		t.Fatalf("unexpected config: %#v", loaded.Config)
	}
}

func TestValidateGlossaryLocale(t *testing.T) {
	cfg := model.Config{}
	_ = cfg
	bad := Default("ja", []string{"en"})
	bad.Glossary = []model.GlossaryTerm{{Source: "a", Target: "b", Locale: "../en"}}
	if err := Validate(bad); err == nil {
		t.Fatal("invalid glossary locale accepted")
	}
}
