package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marvelousu/game-l10n-updater/internal/catalog"
	"github.com/marvelousu/game-l10n-updater/internal/config"
	"github.com/marvelousu/game-l10n-updater/internal/hashx"
	"github.com/marvelousu/game-l10n-updater/internal/jsonx"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/state"
	"github.com/marvelousu/game-l10n-updater/internal/target"
	"github.com/marvelousu/game-l10n-updater/internal/translate"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeLanguages(t *testing.T, w http.ResponseWriter, codes ...string) {
	t.Helper()
	items := make([]map[string]string, 0, len(codes))
	for _, code := range codes {
		items = append(items, map[string]string{"code": code})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"languages": items})
}

func writeCLIFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

type cliProject struct {
	dir     string
	config  string
	source  []model.SourceEntry
	targets map[string]string
	state   *model.State
}

func newCLIProject(t *testing.T, apiURL string, source []model.SourceEntry, targets map[string]string, st *model.State) cliProject {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default("ja", []string{"en"})
	if apiURL != "" {
		cfg.API.URL = apiURL
	}
	cfgData, err := config.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, filepath.Join(dir, "gamel10n.json"), cfgData)
	catData, err := catalog.Marshal(&model.SourceCatalog{SchemaVersion: 1, Entries: source})
	if err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, filepath.Join(dir, "source", "catalog.json"), catData)
	targetData, err := target.Marshal(targets)
	if err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, filepath.Join(dir, "targets", "en.json"), targetData)
	if st != nil {
		stateData, err := state.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		writeCLIFile(t, filepath.Join(dir, "state", "gamel10n-state.json"), stateData)
	}
	return cliProject{dir: dir, config: filepath.Join(dir, "gamel10n.json"), source: source, targets: targets, state: st}
}

func TestInitCreatesProjectAndPreservesExistingSource(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runCLI(t, "init", "-dir", dir, "-languages", "en,zh-hans")
	if code != exitOK {
		t.Fatalf("init exit = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "initialized") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	for _, rel := range []string{"gamel10n.json", "source/catalog.json", "targets/en.json", "targets/zh-hans.json", "state/gamel10n-state.json"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	sourcePath := filepath.Join(dir, "source", "catalog.json")
	writeCLIFile(t, sourcePath, []byte(`{"schema_version":1,"entries":[{"key":"k","text":"custom"}]}`))
	code, _, _ = runCLI(t, "init", "-dir", dir)
	if code != exitError {
		t.Fatalf("second init exit = %d, want %d", code, exitError)
	}
	code, _, stderr = runCLI(t, "init", "-dir", dir, "-force")
	if code != exitOK {
		t.Fatalf("forced init exit = %d, stderr=%s", code, stderr)
	}
	data, _ := os.ReadFile(sourcePath)
	if !strings.Contains(string(data), "custom") {
		t.Fatal("forced init overwrote existing source catalog")
	}
}

func TestPlanCheckAndExports(t *testing.T) {
	p := newCLIProject(t, "", []model.SourceEntry{
		{Key: "greet", Text: "Hello {name}"},
		{Key: "short", Text: "Short", MaxLength: 3},
	}, map[string]string{
		"greet": "こんにちは {name}",
		"short": "very long",
	}, nil)

	reportPath := filepath.Join(p.dir, "plan.json")
	code, stdout, stderr := runCLI(t, "plan", "-config", p.config, "-report", reportPath)
	if code != exitOK {
		t.Fatalf("plan exit = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "unchanged=2") {
		t.Fatalf("unexpected plan summary: %s", stdout)
	}
	var planReport map[string]any
	data, _ := os.ReadFile(reportPath)
	if err := json.Unmarshal(data, &planReport); err != nil {
		t.Fatal(err)
	}
	if planReport["kind"] != "plan" {
		t.Fatalf("unexpected plan report: %#v", planReport)
	}

	// check writes JSON to stdout and a human summary to stderr by default.
	code, stdout, stderr = runCLI(t, "check", "-config", p.config)
	if code != exitOK {
		t.Fatalf("check exit = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "warnings=1") || !strings.Contains(stderr, "max_length") {
		t.Fatalf("unexpected check summary: %s", stderr)
	}
	var checkReport map[string]any
	if err := json.Unmarshal([]byte(stdout), &checkReport); err != nil {
		t.Fatalf("check stdout is not JSON: %v\n%s", err, stdout)
	}
	if checkReport["kind"] != "check" {
		t.Fatalf("unexpected check report: %#v", checkReport)
	}

	csvPath := filepath.Join(p.dir, "export.csv")
	code, _, stderr = runCLI(t, "export", "-config", p.config, "-format", "csv", "-out", csvPath)
	if code != exitOK {
		t.Fatalf("export csv exit = %d, stderr=%s", code, stderr)
	}
	csvData, _ := os.ReadFile(csvPath)
	if !strings.HasPrefix(string(csvData), "keys,ja,en\n") || !strings.Contains(string(csvData), "こんにちは") {
		t.Fatalf("unexpected CSV:\n%s", csvData)
	}

	jsonPath := filepath.Join(p.dir, "export.json")
	code, _, stderr = runCLI(t, "export", "-config", p.config, "-format", "json", "-out", jsonPath)
	if code != exitOK {
		t.Fatalf("export json exit = %d, stderr=%s", code, stderr)
	}
	var exported map[string]any
	jsonData, _ := os.ReadFile(jsonPath)
	if err := json.Unmarshal(jsonData, &exported); err != nil {
		t.Fatal(err)
	}
	if exported["source_language"] != "ja" {
		t.Fatalf("unexpected JSON export: %#v", exported)
	}
}

func TestCheckStructuralAndDisplayExitCodes(t *testing.T) {
	p := newCLIProject(t, "", []model.SourceEntry{{Key: "greet", Text: "Hello {name}"}},
		map[string]string{"greet": "こんにちは"}, nil)
	code, stdout, stderr := runCLI(t, "check", "-config", p.config)
	if code != exitStructural {
		t.Fatalf("check exit = %d, want %d (stdout=%s stderr=%s)", code, exitStructural, stdout, stderr)
	}
	if !strings.Contains(stderr, "placeholder_mismatch") {
		t.Fatalf("structural issue missing: %s", stderr)
	}

	// Fix the target and merge a display overflow instead.
	writeCLIFile(t, filepath.Join(p.dir, "targets", "en.json"), []byte("{\"greet\":\"こんにちは {name}\"}\n"))
	displayPath := filepath.Join(p.dir, "display.json")
	writeCLIFile(t, displayPath, []byte(`{"schema_version":1,"checked":1,"issues":[{"key":"greet","locale":"en","node_path":"Label","kind":"overflow","message":"clipped"}]}`))
	code, _, stderr = runCLI(t, "check", "-config", p.config, "-display-report", displayPath)
	if code != exitDisplay {
		t.Fatalf("display check exit = %d, want %d (stderr=%s)", code, exitDisplay, stderr)
	}
}

func TestSyncThroughCLIWithFakeAPIAndNoRepeatCalls(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/languages" {
			writeLanguages(t, w, "ja", "en")
			return
		}
		mu.Lock()
		calls++
		mu.Unlock()
		var req translate.Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		body, _ := json.Marshal(map[string]string{"translated_text": "EN:" + req.Text})
		_, _ = w.Write(body)
	}))
	defer server.Close()

	st := model.NewState()
	st.Locale("en").Entries["b"] = &model.EntryState{SourceHash: hashx.Text("B old"), TargetHash: hashx.Text("B old")}
	p := newCLIProject(t, server.URL, []model.SourceEntry{
		{Key: "a", Text: "A"},
		{Key: "b", Text: "B new"},
	}, map[string]string{"b": "B old"}, st)
	reportPath := filepath.Join(p.dir, "sync.json")
	code, stdout, stderr := runCLI(t, "sync", "-config", p.config, "-report", reportPath)
	if code != exitOK {
		t.Fatalf("sync exit = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "api_calls=2") || !strings.Contains(stdout, "wrote_files=true") {
		t.Fatalf("unexpected sync summary: %s", stdout)
	}
	mu.Lock()
	firstCalls := calls
	mu.Unlock()
	if firstCalls != 2 {
		t.Fatalf("API calls = %d, want 2", firstCalls)
	}
	targetData, _ := os.ReadFile(filepath.Join(p.dir, "targets", "en.json"))
	if !strings.Contains(string(targetData), "EN:A") || !strings.Contains(string(targetData), "EN:B new") {
		t.Fatalf("unexpected target: %s", targetData)
	}
	var syncReport map[string]any
	reportData, _ := os.ReadFile(reportPath)
	if err := json.Unmarshal(reportData, &syncReport); err != nil {
		t.Fatal(err)
	}
	if syncReport["kind"] != "sync" || syncReport["status"] != "success" {
		t.Fatalf("unexpected sync report: %#v", syncReport)
	}

	code, _, stderr = runCLI(t, "sync", "-config", p.config)
	if code != exitOK {
		t.Fatalf("repeat sync exit = %d, stderr=%s", code, stderr)
	}
	mu.Lock()
	secondCalls := calls
	mu.Unlock()
	if secondCalls != firstCalls {
		t.Fatalf("repeat sync called API: %d -> %d", firstCalls, secondCalls)
	}
}

func TestSyncFailureWritesSeparateReportAndPreservesFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/languages" {
			writeLanguages(t, w, "ja", "en")
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"backend","message":"boom"}}`))
	}))
	defer server.Close()
	p := newCLIProject(t, server.URL, []model.SourceEntry{{Key: "a", Text: "A"}},
		map[string]string{}, model.NewState())
	targetPath := filepath.Join(p.dir, "targets", "en.json")
	statePath := filepath.Join(p.dir, "state", "gamel10n-state.json")
	targetBefore, _ := os.ReadFile(targetPath)
	stateBefore, _ := os.ReadFile(statePath)
	code, _, stderr := runCLI(t, "sync", "-config", p.config)
	if code != exitError {
		t.Fatalf("sync exit = %d, stderr=%s", code, stderr)
	}
	if after, _ := os.ReadFile(targetPath); string(after) != string(targetBefore) {
		t.Fatal("target changed after failure")
	}
	if after, _ := os.ReadFile(statePath); string(after) != string(stateBefore) {
		t.Fatal("state changed after failure")
	}
	reportPath := filepath.Join(p.dir, "reports", "sync-failure.json")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failure report missing: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatal(err)
	}
	if report["status"] != "failed" || !strings.Contains(report["error"].(string), "boom") {
		t.Fatalf("unexpected failure report: %#v", report)
	}
}

func TestImportCSVThenPlanPreservesImportedTargets(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "godot.csv")
	writeCLIFile(t, csvPath, []byte("keys,ja,en\nui.start,はじめから,Start\nui.quote,\"カンマ, と引用\",\"Comma, and quote\"\n"))
	sourceOut := filepath.Join(dir, "source", "catalog.json")
	targetsDir := filepath.Join(dir, "targets")
	code, stdout, stderr := runCLI(t, "import-csv", "-csv", csvPath, "-out", sourceOut, "-targets-dir", targetsDir)
	if code != exitOK {
		t.Fatalf("import exit = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "imported 2 source entries") {
		t.Fatalf("unexpected import summary: %s", stdout)
	}
	var cat model.SourceCatalog
	sourceData, _ := os.ReadFile(sourceOut)
	if err := json.Unmarshal(sourceData, &cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.Entries) != 2 || cat.Entries[0].Key != "ui.quote" {
		t.Fatalf("unexpected source catalog: %#v", cat)
	}
	targetData, _ := os.ReadFile(filepath.Join(targetsDir, "en.json"))
	if !strings.Contains(string(targetData), "Comma, and quote") {
		t.Fatalf("target not imported: %s", targetData)
	}
	// Build a config pointing at the imported files and verify manual preservation.
	cfg := config.Default("ja", []string{"en"})
	cfgData, _ := jsonx.MarshalStable(cfg)
	writeCLIFile(t, filepath.Join(dir, "gamel10n.json"), cfgData)
	code, stdout, stderr = runCLI(t, "plan", "-config", filepath.Join(dir, "gamel10n.json"))
	if code != exitOK {
		t.Fatalf("plan exit = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "unchanged=2") {
		t.Fatalf("imported targets not preserved:\n%s", stdout)
	}

	code, _, _ = runCLI(t, "import-csv", "-csv", csvPath, "-out", sourceOut, "-targets-dir", targetsDir)
	if code != exitError {
		t.Fatalf("re-import without -force exit = %d", code)
	}
	code, _, stderr = runCLI(t, "import-csv", "-csv", csvPath, "-out", sourceOut, "-targets-dir", targetsDir, "-force")
	if code != exitOK {
		t.Fatalf("forced import exit = %d, stderr=%s", code, stderr)
	}
}

func TestApproveAndLockPreserveTarget(t *testing.T) {
	p := newCLIProject(t, "", []model.SourceEntry{{Key: "k", Text: "Hello"}},
		map[string]string{"k": "Hello human"}, model.NewState())
	code, _, stderr := runCLI(t, "approve", "-config", p.config, "-locale", "en", "-key", "k", "-lock")
	if code != exitOK {
		t.Fatalf("approve exit = %d, stderr=%s", code, stderr)
	}
	stateData, _ := os.ReadFile(filepath.Join(p.dir, "state", "gamel10n-state.json"))
	var st model.State
	if err := json.Unmarshal(stateData, &st); err != nil {
		t.Fatal(err)
	}
	entry := st.Files["en"].Entries["k"]
	if entry == nil || !entry.Manual || !entry.Approved || !entry.Locked || entry.NeedsReview {
		t.Fatalf("unexpected approved state: %#v", entry)
	}
	code, _, stderr = runCLI(t, "unlock", "-config", p.config, "-locale", "en", "-key", "k")
	if code != exitOK {
		t.Fatalf("unlock exit = %d, stderr=%s", code, stderr)
	}
	stateData, _ = os.ReadFile(filepath.Join(p.dir, "state", "gamel10n-state.json"))
	_ = json.Unmarshal(stateData, &st)
	if st.Files["en"].Entries["k"].Locked {
		t.Fatal("unlock did not clear locked")
	}
}

func writeCustomCLIProject(t *testing.T, cfg model.Config, source []model.SourceEntry, targets map[string]map[string]string, st *model.State) (string, string) {
	t.Helper()
	dir := t.TempDir()
	cfgData, err := config.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, filepath.Join(dir, "gamel10n.json"), cfgData)
	catData, err := catalog.Marshal(&model.SourceCatalog{SchemaVersion: 1, Entries: source})
	if err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, filepath.Join(dir, "source", "catalog.json"), catData)
	for _, locale := range cfg.Languages {
		values := targets[locale]
		if values == nil {
			values = map[string]string{}
		}
		targetData, err := target.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		writeCLIFile(t, filepath.Join(dir, "targets", locale+".json"), targetData)
	}
	if st != nil {
		stateData, err := state.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		writeCLIFile(t, filepath.Join(dir, "state", "gamel10n-state.json"), stateData)
	}
	return dir, filepath.Join(dir, "gamel10n.json")
}

func TestSyncRejectsUnsupportedLanguageBeforeTranslate(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/languages" {
			writeLanguages(t, w, "ja", "en")
			return
		}
		calls.Add(1)
		_, _ = w.Write([]byte(`{"translated_text":"x"}`))
	}))
	defer server.Close()
	cfg := config.Default("ja", []string{"en", "pt-BR"})
	cfg.API.URL = server.URL
	dir, configPath := writeCustomCLIProject(t, cfg, []model.SourceEntry{{Key: "a", Text: "A"}},
		map[string]map[string]string{"en": {}, "pt-BR": {}}, model.NewState())
	statePath := filepath.Join(dir, "state", "gamel10n-state.json")
	beforeState, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "sync", "-config", configPath)
	if code != exitError {
		t.Fatalf("sync exit = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, `"pt-BR"`) || !strings.Contains(stderr, `"pt-br"`) {
		t.Fatalf("error lacks configured and canonical locale: %s", stderr)
	}
	if calls.Load() != 0 {
		t.Fatalf("translation called %d time(s) before language check", calls.Load())
	}
	if after, err := os.ReadFile(statePath); err != nil || !bytes.Equal(after, beforeState) {
		t.Fatalf("state changed after language rejection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gamel10n.lock")); !os.IsNotExist(err) {
		t.Fatal("lock was not released")
	}
}

func TestSyncKeepsLocaleFileNameAndSendsCanonicalCode(t *testing.T) {
	var got translate.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/languages" {
			writeLanguages(t, w, "ja", "zh-hant")
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"translated_text":"ZH:A"}`))
	}))
	defer server.Close()
	cfg := config.Default("ja", []string{"zh_TW"})
	cfg.API.URL = server.URL
	cfg.API.Style = "polite"
	dir, configPath := writeCustomCLIProject(t, cfg, []model.SourceEntry{{Key: "a", Text: "A"}},
		map[string]map[string]string{"zh_TW": {}}, model.NewState())
	code, _, stderr := runCLI(t, "sync", "-config", configPath)
	if code != exitOK {
		t.Fatalf("sync exit = %d, stderr=%s", code, stderr)
	}
	if got.Target != "zh-hant" || got.Style != "" {
		t.Fatalf("unexpected wire request: %#v", got)
	}
	targetPath := filepath.Join(dir, "targets", "zh_TW.json")
	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("locale file name was not preserved: %v", err)
	}
	if !strings.Contains(string(targetData), "ZH:A") {
		t.Fatalf("translation missing: %s", targetData)
	}
	stored := readCLIState(t, filepath.Join(dir, "state", "gamel10n-state.json"))
	if stored.Files["zh_TW"] == nil {
		t.Fatalf("state locale key was not preserved: %#v", stored.Files)
	}
}

func readCLIState(t *testing.T, path string) *model.State {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestRunContextCancellationLeavesTargetStateAndLock(t *testing.T) {
	var stalled atomic.Bool
	stalled.Store(true)
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/languages" {
			writeLanguages(t, w, "ja", "en")
			return
		}
		if stalled.Load() {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case started <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			return
		}
		var req translate.Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		body, _ := json.Marshal(map[string]string{"translated_text": "EN:" + req.Text})
		_, _ = w.Write(body)
	}))
	defer server.Close()

	cfg := config.Default("ja", []string{"en"})
	cfg.API.URL = server.URL
	dir, configPath := writeCustomCLIProject(t, cfg, []model.SourceEntry{{Key: "a", Text: "A"}},
		map[string]map[string]string{"en": {}}, model.NewState())
	targetPath := filepath.Join(dir, "targets", "en.json")
	statePath := filepath.Join(dir, "state", "gamel10n-state.json")
	targetBefore, _ := os.ReadFile(targetPath)
	stateBefore, _ := os.ReadFile(statePath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	var stdout, stderr bytes.Buffer
	go func() {
		done <- RunContext(ctx, []string{"sync", "-config", configPath}, &stdout, &stderr)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("translation did not start")
	}
	cancel()
	select {
	case code := <-done:
		if code != exitError {
			t.Fatalf("canceled sync exit = %d, stderr=%s", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled sync did not return")
	}
	if after, _ := os.ReadFile(targetPath); string(after) != string(targetBefore) {
		t.Fatal("target changed after cancellation")
	}
	if after, _ := os.ReadFile(statePath); string(after) != string(stateBefore) {
		t.Fatal("state changed after cancellation")
	}
	if _, err := os.Stat(filepath.Join(dir, ".gamel10n.lock")); !os.IsNotExist(err) {
		t.Fatal("lock was not released")
	}

	stalled.Store(false)
	code, _, stderrText := runCLI(t, "sync", "-config", configPath)
	if code != exitOK {
		t.Fatalf("second sync exit = %d, stderr=%s", code, stderrText)
	}
	targetData, _ := os.ReadFile(targetPath)
	if !strings.Contains(string(targetData), "EN:A") {
		t.Fatalf("second sync did not translate: %s", targetData)
	}
}

func TestLockFileBlocksCLISync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("API must not be called while locked")
		_, _ = w.Write([]byte(`{"translated_text":"x"}`))
	}))
	defer server.Close()
	p := newCLIProject(t, server.URL, []model.SourceEntry{{Key: "a", Text: "A"}},
		map[string]string{}, model.NewState())
	writeCLIFile(t, filepath.Join(p.dir, ".gamel10n.lock"), []byte("pid=999\n"))
	code, _, stderr := runCLI(t, "sync", "-config", p.config)
	if code != exitError || !strings.Contains(stderr, "locked") {
		t.Fatalf("sync exit = %d, stderr=%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(p.dir, "reports", "sync-failure.json")); err != nil {
		t.Fatalf("generic failure report missing: %v", err)
	}
}
