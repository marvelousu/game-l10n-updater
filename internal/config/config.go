// Package config loads and validates gamel10n.json.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/netx"
	"github.com/marvelousu/game-l10n-updater/internal/pathx"
)

// DefaultTimeoutSeconds is the default API timeout. The translation server's
// own default is 180 seconds, so the client waits a little longer.
const DefaultTimeoutSeconds = 210

// Loaded is a validated config plus the absolute project root.
type Loaded struct {
	Config model.Config
	Root   string
	Path   string
	Raw    []byte
}

// Default returns the config written by init.
func Default(sourceLanguage string, languages []string) model.Config {
	return model.Config{
		SchemaVersion: model.SchemaVersion,
		Source: model.SourceConfig{
			Language: sourceLanguage,
			Path:     "source/catalog.json",
		},
		Targets: model.TargetsConfig{
			Directory: "targets",
			State:     "state/gamel10n-state.json",
		},
		Languages: append([]string(nil), languages...),
		API: model.APIConfig{
			URL:            "http://127.0.0.1:8787",
			Style:          "",
			TimeoutSeconds: DefaultTimeoutSeconds,
			MaxTokens:      512,
		},
	}
}

// Load reads, strictly decodes and validates path.
func Load(path string) (*Loaded, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", abs, err)
	}
	var cfg model.Config
	if err := jsonx.DecodeStrict(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", abs, err)
	}
	if cfg.API.TimeoutSeconds == 0 {
		cfg.API.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if cfg.API.MaxTokens == 0 {
		cfg.API.MaxTokens = 512
	}
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", abs, err)
	}
	return &Loaded{Config: cfg, Root: filepath.Dir(abs), Path: abs, Raw: raw}, nil
}

// Validate checks schema, path safety and API constraints.
func Validate(cfg model.Config) error {
	if cfg.SchemaVersion != model.SchemaVersion {
		return fmt.Errorf("schema_version must be %d, got %d", model.SchemaVersion, cfg.SchemaVersion)
	}
	if err := pathx.ValidateLocale(cfg.Source.Language); err != nil {
		return fmt.Errorf("source.language: %w", err)
	}
	if cfg.Source.Language == "auto" {
		return fmt.Errorf("source.language must be a concrete language code, not auto")
	}
	if _, err := pathx.SafeJoin(".", cfg.Source.Path); err != nil {
		return fmt.Errorf("source.path: %w", err)
	}
	if _, err := pathx.SafeJoin(".", cfg.Targets.Directory); err != nil {
		return fmt.Errorf("targets.directory: %w", err)
	}
	if _, err := pathx.SafeJoin(".", cfg.Targets.State); err != nil {
		return fmt.Errorf("targets.state: %w", err)
	}
	seen := map[string]bool{cfg.Source.Language: true}
	for _, locale := range cfg.Languages {
		if err := pathx.ValidateLocale(locale); err != nil {
			return fmt.Errorf("languages: %w", err)
		}
		if seen[locale] {
			return fmt.Errorf("languages contains duplicate or source locale %q", locale)
		}
		seen[locale] = true
	}
	if err := netx.ValidateLoopback(cfg.API.URL); err != nil {
		return fmt.Errorf("api.url: %w", err)
	}
	if cfg.API.TimeoutSeconds < 1 || cfg.API.TimeoutSeconds > 3600 {
		return fmt.Errorf("api.timeout_seconds must be between 1 and 3600")
	}
	switch strings.TrimSpace(cfg.API.Style) {
	case "", "default", "polite", "plain":
	default:
		return fmt.Errorf("api.style must be empty, default, polite, or plain")
	}
	if cfg.API.MaxTokens < 0 {
		return fmt.Errorf("api.max_tokens must not be negative")
	}
	for i, term := range cfg.Glossary {
		if term.Source == "" || term.Target == "" {
			return fmt.Errorf("glossary[%d] needs non-empty source and target", i)
		}
		if term.Locale != "" {
			if err := pathx.ValidateLocale(term.Locale); err != nil {
				return fmt.Errorf("glossary[%d].locale: %w", i, err)
			}
		}
	}
	return nil
}

// Marshal returns the canonical config bytes.
func Marshal(cfg model.Config) ([]byte, error) {
	return jsonx.MarshalStable(cfg)
}
