// Package project loads a validated project without writing anything.
package project

import (
	"fmt"
	"path/filepath"

	"github.com/marvelousu/game-l10n-updater/internal/catalog"
	"github.com/marvelousu/game-l10n-updater/internal/config"
	"github.com/marvelousu/game-l10n-updater/internal/fsutil"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/pathx"
	"github.com/marvelousu/game-l10n-updater/internal/state"
	"github.com/marvelousu/game-l10n-updater/internal/target"
)

// Project is the complete in-memory view of one config and its files.
type Project struct {
	Root       string
	ConfigPath string
	ConfigRaw  []byte
	Config     model.Config

	SourcePath string
	SourceRaw  []byte
	Source     *model.SourceCatalog
	SourceMap  map[string]model.SourceEntry
	SourceKeys []string

	TargetDir    string
	TargetPaths  map[string]string
	TargetRaw    map[string][]byte
	TargetExists map[string]bool
	Targets      map[string]map[string]string

	StatePath   string
	StateRaw    []byte
	StateExists bool
	State       *model.State
}

// Load reads a config and all referenced files.
func Load(configPath string) (*Project, error) {
	loaded, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	return LoadFrom(loaded)
}

// LoadFrom reads files using an already validated config.
func LoadFrom(loaded *config.Loaded) (*Project, error) {
	p := &Project{
		Root:         loaded.Root,
		ConfigPath:   loaded.Path,
		ConfigRaw:    loaded.Raw,
		Config:       loaded.Config,
		TargetPaths:  map[string]string{},
		TargetRaw:    map[string][]byte{},
		TargetExists: map[string]bool{},
		Targets:      map[string]map[string]string{},
	}

	sourcePath, err := pathx.SafeJoin(p.Root, p.Config.Source.Path)
	if err != nil {
		return nil, fmt.Errorf("source.path: %w", err)
	}
	p.SourcePath = sourcePath
	sourceRaw, exists, err := fsutil.ReadOptional(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source catalog: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("source catalog %s does not exist", sourcePath)
	}
	cat, err := catalog.Parse(sourceRaw)
	if err != nil {
		return nil, fmt.Errorf("source catalog %s: %w", sourcePath, err)
	}
	p.SourceRaw = sourceRaw
	p.Source = cat
	p.SourceMap, p.SourceKeys = catalog.Index(cat)

	p.TargetDir, err = pathx.SafeJoin(p.Root, p.Config.Targets.Directory)
	if err != nil {
		return nil, fmt.Errorf("targets.directory: %w", err)
	}
	for _, locale := range p.Config.Languages {
		path, err := pathx.SafeTargetFile(p.Root, p.Config.Targets.Directory, locale)
		if err != nil {
			return nil, fmt.Errorf("target locale %q: %w", locale, err)
		}
		p.TargetPaths[locale] = path
		raw, exists, err := fsutil.ReadOptional(path)
		if err != nil {
			return nil, fmt.Errorf("read target %s: %w", path, err)
		}
		p.TargetRaw[locale] = raw
		p.TargetExists[locale] = exists
		if !exists {
			p.Targets[locale] = map[string]string{}
			continue
		}
		m, err := target.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("target catalog %s: %w", path, err)
		}
		p.Targets[locale] = m
	}

	statePath, err := pathx.SafeJoin(p.Root, p.Config.Targets.State)
	if err != nil {
		return nil, fmt.Errorf("targets.state: %w", err)
	}
	p.StatePath = statePath
	stateRaw, stateExists, err := fsutil.ReadOptional(statePath)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	p.StateRaw = stateRaw
	p.StateExists = stateExists
	if !stateExists {
		p.State = model.NewState()
	} else {
		st, err := state.Parse(stateRaw)
		if err != nil {
			return nil, fmt.Errorf("state %s: %w", statePath, err)
		}
		p.State = st
	}
	return p, nil
}

// Locales returns the configured target locales in order.
func (p *Project) Locales() []string {
	return append([]string(nil), p.Config.Languages...)
}

// LocalePath returns the target path for locale.
func (p *Project) LocalePath(locale string) string {
	return filepath.Clean(p.TargetPaths[locale])
}
