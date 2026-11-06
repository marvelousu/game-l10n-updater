// Package syncer plans and applies translations transactionally.
package syncer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/marvelousu/game-l10n-updater/internal/config"
	"github.com/marvelousu/game-l10n-updater/internal/fsutil"
	"github.com/marvelousu/game-l10n-updater/internal/hashx"
	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
	"github.com/marvelousu/game-l10n-updater/internal/lockfile"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/plan"
	"github.com/marvelousu/game-l10n-updater/internal/project"
	"github.com/marvelousu/game-l10n-updater/internal/state"
	"github.com/marvelousu/game-l10n-updater/internal/target"
	"github.com/marvelousu/game-l10n-updater/internal/translate"
	"github.com/marvelousu/game-l10n-updater/internal/validate"
)

// Translator is the API surface used by sync.
type Translator interface {
	Translate(ctx context.Context, req translate.Request) (string, error)
}

// LanguageChecker is an optional capability implemented by the concrete API
// client. Sync checks the locales needing translation in this run before any
// inference. Injected test translators do not need to implement it.
type LanguageChecker interface {
	SupportedLanguages(ctx context.Context) ([]string, error)
}

// Options controls one sync.
type Options struct {
	ReportPath   string
	PruneDeleted bool
	// Hook is test instrumentation passed to the transactional commit.
	Hook func(event string) error
}

// ActionLine is one planned or applied key/locale action in a report.
type ActionLine struct {
	Key         string `json:"key"`
	Locale      string `json:"locale"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	SourceText  string `json:"source_text,omitempty"`
	TargetText  string `json:"target_text,omitempty"`
	SourceHash  string `json:"source_hash,omitempty"`
	TargetHash  string `json:"target_hash,omitempty"`
	Manual      bool   `json:"manual,omitempty"`
	Approved    bool   `json:"approved,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
	NeedsReview bool   `json:"needs_review,omitempty"`
}

// Translation records a successful API result.
type Translation struct {
	Key     string `json:"key"`
	Locale  string `json:"locale"`
	Text    string `json:"translated_text"`
	Applied bool   `json:"applied"`
}

// Rejection records a translation that failed placeholder validation.
type Rejection struct {
	Key            string        `json:"key"`
	Locale         string        `json:"locale"`
	TranslatedText string        `json:"translated_text"`
	Issues         []model.Issue `json:"issues"`
}

// Report is the separate JSON report written by sync.
type Report struct {
	SchemaVersion  int            `json:"schema_version"`
	Kind           string         `json:"kind"`
	Status         string         `json:"status"`
	SourceLanguage string         `json:"source_language"`
	Locales        []string       `json:"locales"`
	Actions        []ActionLine   `json:"actions"`
	Counts         map[string]int `json:"counts"`
	APICalls       int            `json:"api_calls"`
	Translations   []Translation  `json:"translations"`
	Rejected       []Rejection    `json:"rejected,omitempty"`
	Warnings       []model.Issue  `json:"warnings,omitempty"`
	WroteFiles     bool           `json:"wrote_files"`
	Pruned         int            `json:"pruned"`
	Error          string         `json:"error,omitempty"`
}

// Run locks the project, loads it, translates only necessary entries, and
// commits target plus state through a rollback-capable transaction.
func Run(ctx context.Context, configPath string, api Translator, opts Options) (report *Report, runErr error) {
	loaded, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	lock, err := lockfile.Acquire(filepath.Join(loaded.Root, ".gamel10n.lock"))
	if err != nil {
		return nil, err
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			runErr = errors.Join(runErr, releaseErr)
			report.Error = runErr.Error()
			report, runErr = finish(report, opts, runErr)
		}
	}()

	report = &Report{
		SchemaVersion:  model.SchemaVersion,
		Kind:           "sync",
		Status:         "success",
		SourceLanguage: loaded.Config.Source.Language,
		Locales:        sortedCopy(loaded.Config.Languages),
		Actions:        []ActionLine{},
		Counts:         map[string]int{},
		Translations:   []Translation{},
	}
	p, err := project.LoadFrom(loaded)
	if err != nil {
		report, runErr = finish(report, opts, err)
		return
	}
	report, runErr = runOnProject(ctx, p, api, opts, report)
	return
}

func runOnProject(ctx context.Context, p *project.Project, api Translator, opts Options, report *Report) (*Report, error) {
	if err := cancelError(ctx); err != nil {
		return failSync(report, opts, err)
	}
	planned := plan.Build(p.Source, p.Targets, p.State, p.Config.Languages)
	report.Actions = actionLines(planned.Decisions)
	report.Counts = countsMap(planned.Counts)

	if planned.APIRequests > 0 {
		if checker, ok := api.(LanguageChecker); ok {
			needed := map[string]bool{}
			for _, decision := range planned.Decisions {
				if decision.Action == plan.ActionNew || decision.Action == plan.ActionChanged {
					needed[decision.Locale] = true
				}
			}
			locales := make([]string, 0, len(needed))
			for locale := range needed {
				locales = append(locales, locale)
			}
			sort.Strings(locales)
			if err := checkLanguages(ctx, checker, p.Config.Source.Language, locales); err != nil {
				return failSync(report, opts, err)
			}
		}
	}

	newTargets := cloneTargets(p.Targets)
	newState := state.Clone(p.State)
	warnings := []model.Issue{}

	for _, decision := range planned.Decisions {
		switch decision.Action {
		case plan.ActionNew, plan.ActionChanged:
			if err := cancelError(ctx); err != nil {
				return failSync(report, opts, err)
			}
			report.APICalls++
			translated, err := api.Translate(ctx, translate.Request{
				Text:      decision.Entry.Text,
				Source:    p.Config.Source.Language,
				Target:    decision.Locale,
				Style:     styleFor(p.Config.API.Style, decision.Locale),
				Glossary:  glossaryFor(p.Config.Glossary, decision.Locale),
				Context:   decision.Entry.Context,
				MaxTokens: p.Config.API.MaxTokens,
			})
			if err != nil {
				if ctxErr := cancelError(ctx); ctxErr != nil {
					return failSync(report, opts, ctxErr)
				}
				report.Status = "failed"
				report.Error = fmt.Sprintf("translate %s/%s: %v", decision.Locale, decision.Key, err)
				return finish(report, opts, fmt.Errorf("%s", report.Error))
			}
			issues := validate.Issues(decision.Entry.Text, translated, decision.Key, decision.Locale)
			if len(issues) > 0 {
				report.Rejected = append(report.Rejected, Rejection{
					Key: decision.Key, Locale: decision.Locale, TranslatedText: translated, Issues: issues,
				})
				report.Status = "failed"
				report.Error = fmt.Sprintf("translation rejected for %s/%s: placeholder or markup mismatch; not applied", decision.Locale, decision.Key)
				return finish(report, opts, fmt.Errorf("%s", report.Error))
			}
			if issue := validate.MaxLengthIssue(decision.Entry, translated, decision.Locale); issue != nil {
				warnings = append(warnings, *issue)
				report.Warnings = warnings
			}
			newTargets[decision.Locale][decision.Key] = translated
			es := newState.Locale(decision.Locale).Entries[decision.Key]
			if es == nil {
				es = &model.EntryState{}
				newState.Locale(decision.Locale).Entries[decision.Key] = es
			}
			es.SourceHash = decision.SourceHash
			es.TargetHash = hashx.Text(translated)
			es.Manual = false
			es.Approved = false
			es.Locked = false
			es.NeedsReview = false
			report.Translations = append(report.Translations, Translation{
				Key: decision.Key, Locale: decision.Locale, Text: translated,
			})

		case plan.ActionUnchanged:
			ls := newState.Locale(decision.Locale)
			es := ls.Entries[decision.Key]
			if es == nil {
				es = &model.EntryState{}
				ls.Entries[decision.Key] = es
				es.Manual = true
			}
			if decision.Edited {
				es.Manual = true
			}
			es.SourceHash = decision.SourceHash
			es.TargetHash = decision.TargetHash
			es.NeedsReview = false

		case plan.ActionNeedsReview:
			ls := newState.Locale(decision.Locale)
			es := ls.Entries[decision.Key]
			if es == nil {
				es = &model.EntryState{Manual: true}
				ls.Entries[decision.Key] = es
			}
			if decision.Edited {
				es.Manual = true
			}
			if decision.HasTarget {
				es.TargetHash = decision.TargetHash
			} else {
				es.TargetHash = ""
			}
			es.NeedsReview = true

		case plan.ActionDeleted:
			if opts.PruneDeleted {
				delete(newTargets[decision.Locale], decision.Key)
				if ls := newState.Files[decision.Locale]; ls != nil {
					delete(ls.Entries, decision.Key)
				}
				report.Pruned++
			}
		}
	}
	report.Warnings = warnings

	// Cancellation aborts before the transaction starts, including runs that
	// only need to record state. Commit itself is not interrupted midflight.
	if err := cancelError(ctx); err != nil {
		return failSync(report, opts, err)
	}

	// Validate output structure before any write.
	for _, locale := range p.Config.Languages {
		for key := range newTargets[locale] {
			if key == "" {
				report.Status = "failed"
				report.Error = fmt.Sprintf("target %s contains an empty key", locale)
				return finish(report, opts, fmt.Errorf("%s", report.Error))
			}
			if bytes.IndexByte([]byte(key), 0) >= 0 {
				report.Status = "failed"
				report.Error = fmt.Sprintf("target %s contains a NUL key", locale)
				return finish(report, opts, fmt.Errorf("%s", report.Error))
			}
		}
	}

	var updates []fsutil.FileUpdate
	for _, locale := range p.Config.Languages {
		data, err := target.Marshal(newTargets[locale])
		if err != nil {
			report.Status = "failed"
			report.Error = fmt.Sprintf("encode target %s: %v", locale, err)
			return finish(report, opts, fmt.Errorf("%s", report.Error))
		}
		if !p.TargetExists[locale] || !bytes.Equal(data, p.TargetRaw[locale]) {
			updates = append(updates, fsutil.FileUpdate{
				Path:         p.TargetPaths[locale],
				Data:         data,
				Existed:      p.TargetExists[locale],
				ExpectedHash: hashx.Bytes(p.TargetRaw[locale]),
			})
		}
	}
	stateData, err := state.Marshal(newState)
	if err != nil {
		report.Status = "failed"
		report.Error = fmt.Sprintf("encode state: %v", err)
		return finish(report, opts, fmt.Errorf("%s", report.Error))
	}
	if !p.StateExists || !bytes.Equal(stateData, p.StateRaw) {
		updates = append(updates, fsutil.FileUpdate{
			Path:         p.StatePath,
			Data:         stateData,
			Existed:      p.StateExists,
			ExpectedHash: hashx.Bytes(p.StateRaw),
		})
	}

	if len(updates) == 0 {
		report.Status = "no_changes"
		for i := range report.Translations {
			report.Translations[i].Applied = false
		}
		return finish(report, opts, nil)
	}

	guards := []fsutil.Guard{
		{Path: p.SourcePath, Existed: true, Hash: hashx.Bytes(p.SourceRaw)},
		{Path: p.ConfigPath, Existed: true, Hash: hashx.Bytes(p.ConfigRaw)},
		{Path: p.StatePath, Existed: p.StateExists, Hash: hashx.Bytes(p.StateRaw)},
	}
	for _, locale := range p.Config.Languages {
		guards = append(guards, fsutil.Guard{
			Path: p.TargetPaths[locale], Existed: p.TargetExists[locale], Hash: hashx.Bytes(p.TargetRaw[locale]),
		})
	}
	if err := fsutil.Commit(updates, guards, fsutil.CommitOptions{Hook: opts.Hook}); err != nil {
		report.Status = "failed"
		report.Error = fmt.Sprintf("commit failed: %v", err)
		for i := range report.Translations {
			report.Translations[i].Applied = false
		}
		return finish(report, opts, fmt.Errorf("%s", report.Error))
	}
	report.Status = "success"
	report.WroteFiles = true
	for i := range report.Translations {
		report.Translations[i].Applied = true
	}
	return finish(report, opts, nil)
}

func cancelError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sync canceled: %w", err)
	}
	return nil
}

func failSync(report *Report, opts Options, err error) (*Report, error) {
	report.Status = "failed"
	report.Error = err.Error()
	return finish(report, opts, err)
}

func checkLanguages(ctx context.Context, checker LanguageChecker, sourceLanguage string, targets []string) error {
	supported, err := checker.SupportedLanguages(ctx)
	if err != nil {
		if ctxErr := cancelError(ctx); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("check translation API languages: %w", err)
	}
	set := map[string]bool{}
	for _, code := range supported {
		set[translate.CanonicalLanguage(code)] = true
	}
	source := translate.CanonicalLanguage(sourceLanguage)
	if !set[source] {
		return fmt.Errorf("configured source language %q is not supported by the translation API (canonical %q)", sourceLanguage, source)
	}
	for _, locale := range targets {
		canonical := translate.CanonicalLanguage(locale)
		if !set[canonical] {
			return fmt.Errorf("configured target language %q is not supported by the translation API (canonical %q)", locale, canonical)
		}
	}
	return nil
}

// styleFor sends a configured style only when the API target is Japanese.
// Other targets omit it from the wire request.
func styleFor(style, locale string) string {
	if style == "" || translate.CanonicalLanguage(locale) != "ja" {
		return ""
	}
	return style
}

func finish(report *Report, opts Options, runErr error) (*Report, error) {
	if runErr != nil {
		report.Status = "failed"
		if report.Error == "" {
			report.Error = runErr.Error()
		}
	}
	if opts.ReportPath == "" {
		return report, runErr
	}
	data, err := jsonx.MarshalStable(report)
	if err != nil {
		if runErr != nil {
			return report, fmt.Errorf("%w; encode sync report: %v", runErr, err)
		}
		return report, fmt.Errorf("encode sync report: %w", err)
	}
	if err := fsutil.WriteAtomic(opts.ReportPath, data, 0o644); err != nil {
		if runErr != nil {
			return report, fmt.Errorf("%w; write sync report: %v", runErr, err)
		}
		return report, fmt.Errorf("write sync report: %w", err)
	}
	return report, runErr
}

func actionLines(decisions []plan.Decision) []ActionLine {
	out := make([]ActionLine, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, ActionLine{
			Key: d.Key, Locale: d.Locale, Action: string(d.Action), Reason: d.Reason,
			SourceText: d.Entry.Text, TargetText: d.TargetText,
			SourceHash: d.SourceHash, TargetHash: d.TargetHash,
			Manual: d.Manual, Approved: d.Approved, Locked: d.Locked, NeedsReview: d.NeedsReview,
		})
	}
	return out
}

func countsMap(counts map[plan.Action]int) map[string]int {
	out := map[string]int{}
	for _, action := range plan.AllActions {
		out[string(action)] = counts[action]
	}
	return out
}

func cloneTargets(in map[string]map[string]string) map[string]map[string]string {
	out := make(map[string]map[string]string, len(in))
	for locale, m := range in {
		copied := make(map[string]string, len(m))
		for key, value := range m {
			copied[key] = value
		}
		out[locale] = copied
	}
	return out
}

func glossaryFor(terms []model.GlossaryTerm, locale string) []translate.GlossaryEntry {
	out := []translate.GlossaryEntry{}
	for _, term := range terms {
		if term.Locale == "" || term.Locale == locale {
			out = append(out, translate.GlossaryEntry{Source: term.Source, Target: term.Target})
		}
	}
	return out
}

func sortedCopy(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}
