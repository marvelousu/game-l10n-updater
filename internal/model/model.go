// Package model contains the on-disk data types shared by the CLI.
package model

// SchemaVersion is the only schema version accepted by this release.
const SchemaVersion = 1

// Config is the JSON project configuration (gamel10n.json).
type Config struct {
	SchemaVersion int            `json:"schema_version"`
	Source        SourceConfig   `json:"source"`
	Targets       TargetsConfig  `json:"targets"`
	Languages     []string       `json:"languages"`
	API           APIConfig      `json:"api"`
	Glossary      []GlossaryTerm `json:"glossary,omitempty"`
}

// SourceConfig points at the structured source catalog.
type SourceConfig struct {
	Language string `json:"language"`
	Path     string `json:"path"`
}

// TargetsConfig names the flat target directory and the state file.
type TargetsConfig struct {
	Directory string `json:"directory"`
	State     string `json:"state"`
}

// APIConfig configures the local translation service.
type APIConfig struct {
	URL            string `json:"url"`
	Style          string `json:"style,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	MaxTokens      int    `json:"max_tokens,omitempty"`
}

// GlossaryTerm is a source-to-target terminology pair. An empty Locale
// applies the term to every target language.
type GlossaryTerm struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Locale string `json:"locale,omitempty"`
}

// SourceCatalog is the structured source catalog.
type SourceCatalog struct {
	SchemaVersion int           `json:"schema_version"`
	Entries       []SourceEntry `json:"entries"`
}

// SourceEntry is one translatable string.
type SourceEntry struct {
	Key       string `json:"key"`
	Text      string `json:"text"`
	Context   string `json:"context,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
}

// State records the last generated hashes and human ownership flags.
type State struct {
	SchemaVersion int                     `json:"schema_version"`
	Files         map[string]*LocaleState `json:"files"`
}

// LocaleState is the state of one target locale.
type LocaleState struct {
	Entries map[string]*EntryState `json:"entries"`
}

// EntryState is the tracked state of one key/locale pair.
type EntryState struct {
	SourceHash  string `json:"source_hash"`
	TargetHash  string `json:"target_hash"`
	Manual      bool   `json:"manual,omitempty"`
	Approved    bool   `json:"approved,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
	NeedsReview bool   `json:"needs_review,omitempty"`
}

// Protected reports whether the entry must never be overwritten by sync.
func (e *EntryState) Protected() bool {
	if e == nil {
		return false
	}
	return e.Manual || e.Approved || e.Locked
}

// NewState returns an empty state with the supported schema version.
func NewState() *State {
	return &State{SchemaVersion: SchemaVersion, Files: map[string]*LocaleState{}}
}

// Locale returns the locale state, creating it when needed.
func (s *State) Locale(locale string) *LocaleState {
	if s.Files == nil {
		s.Files = map[string]*LocaleState{}
	}
	ls := s.Files[locale]
	if ls == nil {
		ls = &LocaleState{Entries: map[string]*EntryState{}}
		s.Files[locale] = ls
	}
	if ls.Entries == nil {
		ls.Entries = map[string]*EntryState{}
	}
	return ls
}

// Entry returns one state entry or nil.
func (s *State) Entry(locale, key string) *EntryState {
	if s == nil || s.Files == nil {
		return nil
	}
	ls := s.Files[locale]
	if ls == nil {
		return nil
	}
	return ls.Entries[key]
}

// Issue is a validation or engine report issue.
type Issue struct {
	Key           string   `json:"key"`
	Locale        string   `json:"locale"`
	Kind          string   `json:"kind"`
	Severity      string   `json:"severity"`
	Message       string   `json:"message"`
	Category      string   `json:"category,omitempty"`
	Source        string   `json:"source,omitempty"`
	NodePath      string   `json:"node_path,omitempty"`
	Missing       []string `json:"missing,omitempty"`
	Extra         []string `json:"extra,omitempty"`
	UnknownKey    bool     `json:"unknown_key,omitempty"`
	UnknownLocale bool     `json:"unknown_locale,omitempty"`
}
