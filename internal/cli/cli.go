// Package cli implements the gamel10n commands without touching os.Exit.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/marvelousu/game-l10n-updater/internal/catalog"
	"github.com/marvelousu/game-l10n-updater/internal/check"
	"github.com/marvelousu/game-l10n-updater/internal/config"
	"github.com/marvelousu/game-l10n-updater/internal/display"
	"github.com/marvelousu/game-l10n-updater/internal/exporter"
	"github.com/marvelousu/game-l10n-updater/internal/fsutil"
	"github.com/marvelousu/game-l10n-updater/internal/godotcsv"
	"github.com/marvelousu/game-l10n-updater/internal/hashx"
	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
	"github.com/marvelousu/game-l10n-updater/internal/lockfile"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/pathx"
	"github.com/marvelousu/game-l10n-updater/internal/plan"
	"github.com/marvelousu/game-l10n-updater/internal/project"
	"github.com/marvelousu/game-l10n-updater/internal/state"
	"github.com/marvelousu/game-l10n-updater/internal/syncer"
	"github.com/marvelousu/game-l10n-updater/internal/target"
	"github.com/marvelousu/game-l10n-updater/internal/translate"
)

// Version is the CLI release string.
const Version = "0.2.1"

const (
	exitOK         = 0
	exitError      = 1
	exitUsage      = 2
	exitStructural = 3
	exitDisplay    = 4
)

// Run executes one command and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	command, rest := args[0], args[1:]
	switch command {
	case "init":
		return runInit(rest, stdout, stderr)
	case "plan":
		return runPlan(rest, stdout, stderr)
	case "sync":
		return runSync(rest, stdout, stderr)
	case "check":
		return runCheck(rest, stdout, stderr)
	case "export":
		return runExport(rest, stdout, stderr)
	case "import-csv":
		return runImportCSV(rest, stdout, stderr)
	case "approve":
		return runApprove(rest, stdout, stderr)
	case "unlock":
		return runUnlock(rest, stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "gamel10n %s\n", Version)
		return exitOK
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", command)
		usage(stderr)
		return exitUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `gamel10n tracks game localization changes and preserves human translations.

Usage:
  gamel10n <command> [flags]

Commands:
  init        Create gamel10n.json, an empty source catalog, targets and state.
  plan        Show new/changed/unchanged/deleted/needs_review actions. No API calls.
  sync        Translate new/changed entries and commit target+state transactionally.
  check       Check placeholders, max_length and merge a Godot display report.
  export      Write stable JSON or Godot CSV.
  import-csv  Import a Godot CSV into source and target files.
  approve     Mark a target manual+approved (and optionally locked).
  unlock      Remove the locked flag from a target state entry.
  version     Print the version.

Reports are JSON with schema_version 1. Human summaries never replace JSON reports.
Exit codes: 0 ok, 1 operational error, 2 usage, 3 placeholder/structure errors,
4 imported display errors only.
`)
}

func newFlagSet(name string, stderr io.Writer, usageLine string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s\n", usageLine)
		fmt.Fprintf(stderr, "\nflags:\n")
		fs.PrintDefaults()
	}
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string, stderr io.Writer) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK, false
		}
		fs.Usage()
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		fs.Usage()
		return exitUsage, false
	}
	return exitOK, true
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func runInit(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("init", stderr, "gamel10n init [-dir DIR] [-source-lang ja] [-languages en,zh-hans] [-api-url URL] [-force]")
	dir := fs.String("dir", ".", "project directory to initialize")
	sourceLang := fs.String("source-lang", "ja", "source language code")
	languages := fs.String("languages", "en", "comma-separated target language codes")
	apiURL := fs.String("api-url", "http://127.0.0.1:8787", "loopback translation API base URL")
	force := fs.Bool("force", false, "overwrite an existing gamel10n.json (translations are never overwritten)")
	if code, ok := parseFlags(fs, args, stderr); !ok {
		return code
	}
	langs := splitList(*languages)
	if len(langs) == 0 {
		fmt.Fprintln(stderr, "error: at least one target language is required")
		return exitUsage
	}
	if err := pathx.ValidateLocale(*sourceLang); err != nil {
		fmt.Fprintf(stderr, "error: -source-lang: %v\n", err)
		return exitUsage
	}
	seen := map[string]bool{}
	for _, locale := range langs {
		if err := pathx.ValidateLocale(locale); err != nil {
			fmt.Fprintf(stderr, "error: -languages: %v\n", err)
			return exitUsage
		}
		if locale == *sourceLang {
			fmt.Fprintf(stderr, "error: target language %q equals the source language\n", locale)
			return exitUsage
		}
		if seen[locale] {
			fmt.Fprintf(stderr, "error: duplicate target language %q\n", locale)
			return exitUsage
		}
		seen[locale] = true
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	cfg := config.Default(*sourceLang, langs)
	cfg.API.URL = *apiURL
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if _, err := pathx.EnsureDir(absDir); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	configPath := filepath.Join(absDir, "gamel10n.json")
	if _, exists, err := fsutil.ReadOptional(configPath); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	} else if exists && !*force {
		fmt.Fprintf(stderr, "error: %s already exists (use -force to rewrite it; source/targets/state are preserved)\n", configPath)
		return exitError
	}
	configData, err := config.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if err := fsutil.WriteAtomic(configPath, configData, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}

	sourcePath, err := pathx.SafeJoin(absDir, cfg.Source.Path)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if _, exists, err := fsutil.ReadOptional(sourcePath); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	} else if !exists {
		data, err := catalog.Marshal(&model.SourceCatalog{SchemaVersion: model.SchemaVersion, Entries: []model.SourceEntry{}})
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		if err := fsutil.WriteAtomic(sourcePath, data, 0o644); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
	}

	for _, locale := range langs {
		targetPath, err := pathx.SafeTargetFile(absDir, cfg.Targets.Directory, locale)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		if _, exists, err := fsutil.ReadOptional(targetPath); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		} else if !exists {
			data, err := target.Marshal(map[string]string{})
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return exitError
			}
			if err := fsutil.WriteAtomic(targetPath, data, 0o644); err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return exitError
			}
		}
	}

	statePath, err := pathx.SafeJoin(absDir, cfg.Targets.State)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if _, exists, err := fsutil.ReadOptional(statePath); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	} else if !exists {
		data, err := state.Marshal(model.NewState())
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		if err := fsutil.WriteAtomic(statePath, data, 0o644); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
	}

	fmt.Fprintf(stdout, "initialized %s\n", configPath)
	fmt.Fprintf(stdout, "  source: %s (%s)\n", sourcePath, cfg.Source.Language)
	fmt.Fprintf(stdout, "  targets: %s\n", filepath.Join(absDir, cfg.Targets.Directory))
	fmt.Fprintf(stdout, "  state: %s\n", statePath)
	fmt.Fprintln(stdout, "Existing source, target and state files were not overwritten.")
	return exitOK
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("plan", stderr, "gamel10n plan [-config PATH] [-report PATH]")
	configPath := fs.String("config", "gamel10n.json", "project config path")
	reportPath := fs.String("report", "", "write the JSON plan report to this path (use - for stdout)")
	if code, ok := parseFlags(fs, args, stderr); !ok {
		return code
	}
	p, err := project.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	result := plan.Build(p.Source, p.Targets, p.State, p.Config.Languages)
	report := plan.NewReport(p.Config.Source.Language, p.Locales(), result)
	toStdout, err := writeJSON(*reportPath, report, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if toStdout {
		printPlanSummary(stderr, report)
	} else {
		printPlanSummary(stdout, report)
	}
	return exitOK
}

func writeJSON(path string, value any, stdout io.Writer) (bool, error) {
	if path == "" {
		return false, nil
	}
	data, err := jsonx.MarshalStable(value)
	if err != nil {
		return false, err
	}
	if path == "-" {
		_, err := stdout.Write(data)
		return true, err
	}
	if err := fsutil.WriteAtomic(path, data, 0o644); err != nil {
		return false, err
	}
	return false, nil
}

func runSync(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("sync", stderr, "gamel10n sync [-config PATH] [-report PATH] [-prune-deleted]")
	configPath := fs.String("config", "gamel10n.json", "project config path")
	reportPath := fs.String("report", "", "write the JSON sync report to this path (use - for stdout)")
	prune := fs.Bool("prune-deleted", false, "remove target/state entries whose source key was deleted")
	if code, ok := parseFlags(fs, args, stderr); !ok {
		return code
	}
	loaded, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	client, err := translate.New(loaded.Config.API.URL, time.Duration(loaded.Config.API.TimeoutSeconds)*time.Second)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	opts := syncer.Options{PruneDeleted: *prune}
	stdoutReport := *reportPath == "-"
	if !stdoutReport {
		opts.ReportPath = *reportPath
	}
	report, runErr := syncer.Run(context.Background(), *configPath, client, opts)
	if runErr != nil {
		failurePath := *reportPath
		if failurePath == "" {
			failurePath = filepath.Join(loaded.Root, "reports", "sync-failure.json")
		}
		if failurePath == "-" {
			if report == nil {
				report = genericFailureReport(runErr)
			}
			if _, err := writeJSON("-", report, stdout); err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
			}
			printSyncSummary(stderr, report)
		} else if report == nil {
			if err := writeFailureReport(failurePath, nil, runErr); err != nil {
				fmt.Fprintf(stderr, "error: write failure report: %v\n", err)
			} else {
				fmt.Fprintf(stderr, "failure report: %s\n", failurePath)
			}
		} else if *reportPath == "" {
			if err := writeFailureReport(failurePath, report, runErr); err != nil {
				fmt.Fprintf(stderr, "error: write failure report: %v\n", err)
			} else {
				fmt.Fprintf(stderr, "failure report: %s\n", failurePath)
			}
		}
		fmt.Fprintf(stderr, "error: %v\n", runErr)
		return exitError
	}
	if stdoutReport {
		if _, err := writeJSON("-", report, stdout); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		printSyncSummary(stderr, report)
	} else {
		if *reportPath != "" {
			fmt.Fprintf(stdout, "report: %s\n", *reportPath)
		}
		printSyncSummary(stdout, report)
	}
	return exitOK
}

func genericFailureReport(err error) *syncer.Report {
	return &syncer.Report{
		SchemaVersion: model.SchemaVersion,
		Kind:          "sync",
		Status:        "failed",
		Actions:       []syncer.ActionLine{},
		Counts:        map[string]int{},
		Translations:  []syncer.Translation{},
		Error:         err.Error(),
	}
}

func writeFailureReport(path string, report *syncer.Report, err error) error {
	if report == nil {
		report = genericFailureReport(err)
	}
	if report.Error == "" {
		report.Error = err.Error()
	}
	data, marshalErr := jsonx.MarshalStable(report)
	if marshalErr != nil {
		return marshalErr
	}
	return fsutil.WriteAtomic(path, data, 0o644)
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("check", stderr, "gamel10n check [-config PATH] [-report PATH] [-display-report PATH] [-locale en,fr]")
	configPath := fs.String("config", "gamel10n.json", "project config path")
	reportPath := fs.String("report", "", "write the JSON check report to this path (default: stdout; use - for stdout)")
	displayPath := fs.String("display-report", "", "Godot display report JSON to merge")
	localeFlag := fs.String("locale", "", "comma-separated subset of configured locales")
	if code, ok := parseFlags(fs, args, stderr); !ok {
		return code
	}
	p, err := project.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	var displayReport *display.Report
	if *displayPath != "" {
		displayReport, err = display.Load(*displayPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
	}
	report, err := check.Build(p, displayReport, *displayPath, splitList(*localeFlag))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if *reportPath == "" || *reportPath == "-" {
		if _, err := writeJSON("-", report, stdout); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		printCheckSummary(stderr, report)
	} else {
		if err := writeJSONToFile(*reportPath, report); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "report: %s\n", *reportPath)
		printCheckSummary(stdout, report)
	}
	return report.ExitCode()
}

func writeJSONToFile(path string, value any) error {
	data, err := jsonx.MarshalStable(value)
	if err != nil {
		return err
	}
	return fsutil.WriteAtomic(path, data, 0o644)
}

func runExport(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("export", stderr, "gamel10n export [-config PATH] [-format json|csv] [-out PATH] [-locale en,fr]")
	configPath := fs.String("config", "gamel10n.json", "project config path")
	format := fs.String("format", "json", "export format: json or csv")
	out := fs.String("out", "-", "output path, or - for stdout")
	localeFlag := fs.String("locale", "", "comma-separated subset of configured locales")
	if code, ok := parseFlags(fs, args, stderr); !ok {
		return code
	}
	p, err := project.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	requested := splitList(*localeFlag)
	for _, locale := range requested {
		if err := pathx.ValidateLocale(locale); err != nil {
			fmt.Fprintf(stderr, "error: -locale: %v\n", err)
			return exitUsage
		}
	}
	locales, err := exporter.SelectLocales(p, requested)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	var data []byte
	switch strings.ToLower(*format) {
	case "json":
		data, err = exporter.BuildJSON(p, locales)
	case "csv":
		data, err = exporter.BuildCSV(p, locales)
	default:
		fmt.Fprintf(stderr, "error: unknown -format %q (want json or csv)\n", *format)
		return exitUsage
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if *out == "-" {
		if _, err := stdout.Write(data); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		return exitOK
	}
	if err := fsutil.WriteAtomic(*out, data, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stderr, "exported %d locale(s) to %s\n", len(locales), *out)
	return exitOK
}

func runImportCSV(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("import-csv", stderr, "gamel10n import-csv -csv FILE [-out source/catalog.json] -source-language ja [-targets-dir targets] [-force]")
	csvPath := fs.String("csv", "", "Godot CSV file to import (required)")
	out := fs.String("out", "source/catalog.json", "source catalog output path")
	sourceLang := fs.String("source-language", "ja", "column to use as source text")
	targetsDir := fs.String("targets-dir", "", "optional directory for flat target JSON files")
	force := fs.Bool("force", false, "overwrite existing output files")
	if code, ok := parseFlags(fs, args, stderr); !ok {
		return code
	}
	if *csvPath == "" {
		fmt.Fprintln(stderr, "error: -csv is required")
		fs.Usage()
		return exitUsage
	}
	data, err := os.ReadFile(*csvPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	cat, targets, locales, err := godotcsv.Import(data, *sourceLang)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	sourceData, err := catalog.Marshal(cat)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	outPath, err := filepath.Abs(*out)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	importRoot := filepath.Dir(outPath)
	for candidate := importRoot; ; candidate = filepath.Dir(candidate) {
		if _, err := os.Stat(filepath.Join(candidate, "gamel10n.json")); err == nil {
			importRoot = candidate
			break
		}
		if filepath.Dir(candidate) == candidate {
			break
		}
	}
	importLock, err := lockfile.Acquire(filepath.Join(importRoot, ".gamel10n.lock"))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	defer importLock.Release()
	csvAbsolute, _ := filepath.Abs(*csvPath)
	guards := []fsutil.Guard{{Path: csvAbsolute, Existed: true, Hash: hashx.Bytes(data)}}
	if loaded, err := config.Load(filepath.Join(importRoot, "gamel10n.json")); err == nil {
		for _, path := range []string{loaded.Path, filepath.Join(importRoot, loaded.Config.Targets.State)} {
			raw, exists, err := fsutil.ReadOptional(path)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return exitError
			}
			guards = append(guards, fsutil.Guard{Path: path, Existed: exists, Hash: hashx.Bytes(raw)})
		}
	}
	var updates []fsutil.FileUpdate
	appendUpdate := func(path string, content []byte) error {
		raw, exists, err := fsutil.ReadOptional(path)
		if err != nil {
			return err
		}
		if exists && !*force {
			return fmt.Errorf("%s already exists (use -force to overwrite)", path)
		}
		updates = append(updates, fsutil.FileUpdate{
			Path: path, Data: content, Existed: exists, ExpectedHash: hashx.Bytes(raw),
		})
		return nil
	}
	if err := appendUpdate(outPath, sourceData); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if *targetsDir != "" {
		absTargets, err := filepath.Abs(*targetsDir)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		for _, locale := range locales {
			path := filepath.Join(absTargets, locale+".json")
			raw, exists, err := fsutil.ReadOptional(path)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return exitError
			}
			guards = append(guards, fsutil.Guard{Path: path, Existed: exists, Hash: hashx.Bytes(raw)})
			merged := map[string]string{}
			if exists {
				if err := jsonx.DecodeStrict(raw, &merged); err != nil {
					fmt.Fprintf(stderr, "error: existing target %s: %v\n", path, err)
					return exitError
				}
			}
			if merged == nil {
				merged = map[string]string{}
			}
			for key, value := range targets[locale] {
				merged[key] = value
			}
			content, err := target.Marshal(merged)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return exitError
			}
			if err := appendUpdate(path, content); err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return exitError
			}
		}
	}
	if err := fsutil.Commit(updates, guards, fsutil.CommitOptions{}); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "imported %d source entries to %s\n", len(cat.Entries), outPath)
	if *targetsDir != "" {
		fmt.Fprintf(stdout, "wrote targets for %s to %s\n", strings.Join(locales, ","), *targetsDir)
	}
	fmt.Fprintln(stdout, "Imported targets are treated as manual on the next sync unless state already exists.")
	return exitOK
}

func runApprove(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("approve", stderr, "gamel10n approve -locale en -key ui.start,ui.quit [-config PATH] [-lock]")
	configPath := fs.String("config", "gamel10n.json", "project config path")
	locale := fs.String("locale", "", "target locale (required)")
	keys := fs.String("key", "", "comma-separated source keys (required)")
	lock := fs.Bool("lock", false, "also set the locked flag")
	if code, ok := parseFlags(fs, args, stderr); !ok {
		return code
	}
	return setApproval(*configPath, *locale, splitList(*keys), true, *lock, stdout, stderr)
}

func runUnlock(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("unlock", stderr, "gamel10n unlock -locale en -key ui.start [-config PATH]")
	configPath := fs.String("config", "gamel10n.json", "project config path")
	locale := fs.String("locale", "", "target locale (required)")
	keys := fs.String("key", "", "comma-separated source keys (required)")
	if code, ok := parseFlags(fs, args, stderr); !ok {
		return code
	}
	return setApproval(*configPath, *locale, splitList(*keys), false, false, stdout, stderr)
}

func setApproval(configPath, locale string, keys []string, approve, lock bool, stdout, stderr io.Writer) int {
	if locale == "" || len(keys) == 0 {
		fmt.Fprintln(stderr, "error: -locale and -key are required")
		return exitUsage
	}
	if err := pathx.ValidateLocale(locale); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	lockHandle, err := lockfile.Acquire(filepath.Join(loaded.Root, ".gamel10n.lock"))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	defer func() { _ = lockHandle.Release() }()
	p, err := project.LoadFrom(loaded)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	configured := false
	for _, item := range p.Config.Languages {
		if item == locale {
			configured = true
		}
	}
	if !configured {
		fmt.Fprintf(stderr, "error: locale %q is not configured\n", locale)
		return exitError
	}
	newState := state.Clone(p.State)
	sort.Strings(keys)
	for _, key := range keys {
		entry, ok := p.SourceMap[key]
		if !ok {
			fmt.Fprintf(stderr, "error: source key %q does not exist\n", key)
			return exitError
		}
		targetText, ok := p.Targets[locale][key]
		if !ok {
			fmt.Fprintf(stderr, "error: target %s/%s does not exist\n", locale, key)
			return exitError
		}
		es := newState.Locale(locale).Entries[key]
		if es == nil {
			es = &model.EntryState{}
			newState.Locale(locale).Entries[key] = es
		}
		if approve {
			es.Manual = true
			es.Approved = true
			es.Locked = es.Locked || lock
			es.NeedsReview = false
			es.SourceHash = hashx.Text(entry.Text)
			es.TargetHash = hashx.Text(targetText)
		} else {
			es.Locked = false
		}
	}
	stateData, err := state.Marshal(newState)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	updates := []fsutil.FileUpdate{{
		Path: p.StatePath, Data: stateData, Existed: p.StateExists, ExpectedHash: hashx.Bytes(p.StateRaw),
	}}
	guards := []fsutil.Guard{
		{Path: p.SourcePath, Existed: true, Hash: hashx.Bytes(p.SourceRaw)},
		{Path: p.ConfigPath, Existed: true, Hash: hashx.Bytes(p.ConfigRaw)},
	}
	for _, item := range p.Config.Languages {
		guards = append(guards, fsutil.Guard{
			Path: p.TargetPaths[item], Existed: p.TargetExists[item], Hash: hashx.Bytes(p.TargetRaw[item]),
		})
	}
	if err := fsutil.Commit(updates, guards, fsutil.CommitOptions{}); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	action := "approved"
	if !approve {
		action = "unlocked"
	}
	fmt.Fprintf(stdout, "%s %d key(s) for %s\n", action, len(keys), locale)
	return exitOK
}

func printPlanSummary(w io.Writer, report *plan.Report) {
	fmt.Fprintf(w, "plan: new=%d changed=%d unchanged=%d deleted=%d needs_review=%d api_requests=%d\n",
		report.Counts["new"], report.Counts["changed"], report.Counts["unchanged"],
		report.Counts["deleted"], report.Counts["needs_review"], report.APIRequests)
	for _, action := range report.Actions {
		suffix := ""
		if action.Manual {
			suffix += " manual"
		}
		if action.Approved {
			suffix += " approved"
		}
		if action.Locked {
			suffix += " locked"
		}
		if action.NeedsReview {
			suffix += " needs_review"
		}
		fmt.Fprintf(w, "  %-12s %-8s %s%s (%s)\n", action.Action, action.Locale, action.Key, suffix, action.Reason)
	}
}

func printSyncSummary(w io.Writer, report *syncer.Report) {
	if report == nil {
		return
	}
	fmt.Fprintf(w, "sync %s: api_calls=%d wrote_files=%t new=%d changed=%d unchanged=%d needs_review=%d deleted=%d pruned=%d\n",
		report.Status, report.APICalls, report.WroteFiles,
		report.Counts["new"], report.Counts["changed"], report.Counts["unchanged"],
		report.Counts["needs_review"], report.Counts["deleted"], report.Pruned)
	for _, translation := range report.Translations {
		fmt.Fprintf(w, "  translated %-8s %s = %s\n", translation.Locale, translation.Key, shorten(translation.Text, 100))
	}
	for _, rejection := range report.Rejected {
		fmt.Fprintf(w, "  rejected %-8s %s: %s\n", rejection.Locale, rejection.Key, shorten(rejection.TranslatedText, 100))
		for _, issue := range rejection.Issues {
			fmt.Fprintf(w, "    %s: %s\n", issue.Kind, issue.Message)
		}
	}
	for _, action := range report.Actions {
		if action.Action == "needs_review" {
			fmt.Fprintf(w, "  needs review %-8s %s (%s)\n", action.Locale, action.Key, action.Reason)
		}
	}
	for _, action := range report.Actions {
		if action.Action == "deleted" {
			fmt.Fprintf(w, "  deleted %-8s %s (target preserved)\n", action.Locale, action.Key)
		}
	}
	for _, issue := range report.Warnings {
		fmt.Fprintf(w, "  warning %-8s %s: %s\n", issue.Locale, issue.Key, issue.Message)
	}
	if report.Error != "" {
		fmt.Fprintf(w, "  error: %s\n", report.Error)
	}
}

func printCheckSummary(w io.Writer, report *check.Report) {
	fmt.Fprintf(w, "check: checked=%d errors=%d warnings=%d structural=%d display=%d\n",
		report.Checked, report.Counts.Errors, report.Counts.Warnings, report.Counts.Structural, report.Counts.Display)
	if report.Display != nil {
		fmt.Fprintf(w, "  display report: checked=%d issues=%d", report.Display.Checked, report.Display.IssueCount)
		if len(report.Display.UnknownKeys) > 0 {
			fmt.Fprintf(w, " unknown_keys=%s", strings.Join(report.Display.UnknownKeys, ","))
		}
		if len(report.Display.UnknownLocales) > 0 {
			fmt.Fprintf(w, " unknown_locales=%s", strings.Join(report.Display.UnknownLocales, ","))
		}
		fmt.Fprintln(w)
	}
	for _, issue := range report.Issues {
		location := issue.Locale + "/" + issue.Key
		if issue.NodePath != "" {
			location += " " + issue.NodePath
		}
		fmt.Fprintf(w, "  %-8s %-20s %s: %s\n", issue.Severity, location, issue.Kind, issue.Message)
	}
}

func shorten(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}
