package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marvelousu/game-l10n-updater/internal/catalog"
	"github.com/marvelousu/game-l10n-updater/internal/config"
	"github.com/marvelousu/game-l10n-updater/internal/hashx"
	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/state"
	"github.com/marvelousu/game-l10n-updater/internal/target"
	"github.com/marvelousu/game-l10n-updater/internal/translate"
)

type fakeAPI struct {
	server     *httptest.Server
	calls      []translate.Request
	handle     func(req translate.Request) (int, string)
	configPath string
	languages  []string
}

func newFakeAPI(t *testing.T, handle func(req translate.Request) (int, string)) *fakeAPI {
	t.Helper()
	fake := &fakeAPI{handle: handle}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/languages" {
			if r.Method != http.MethodGet {
				t.Errorf("languages method = %s", r.Method)
			}
			codes := fake.languages
			if len(codes) == 0 {
				codes = []string{"ja", "en", "fr", "zh-hans", "zh-hant", "ko"}
			}
			items := make([]map[string]string, 0, len(codes))
			for _, code := range codes {
				items = append(items, map[string]string{"code": code})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"languages": items})
			return
		}
		var req translate.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fake.calls = append(fake.calls, req)
		status, body := fake.handle(req)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func translateOK(req translate.Request) (int, string) {
	body, _ := json.Marshal(map[string]string{"translated_text": "EN:" + req.Text})
	return http.StatusOK, string(body)
}

type projectSetup struct {
	languages      []string
	sourceLanguage string
	source         []model.SourceEntry
	targets        map[string]map[string]string
	state          *model.State
	writeState     bool
	style          string
	maxTokens      int
	glossary       []model.GlossaryTerm
}

func writeProject(t *testing.T, apiURL string, setup projectSetup) string {
	t.Helper()
	dir := t.TempDir()
	sourceLanguage := setup.sourceLanguage
	if sourceLanguage == "" {
		sourceLanguage = "ja"
	}
	cfg := config.Default(sourceLanguage, setup.languages)
	cfg.API.URL = apiURL
	cfg.API.Style = setup.style
	if setup.maxTokens != 0 {
		cfg.API.MaxTokens = setup.maxTokens
	}
	cfg.Glossary = setup.glossary
	cfgData, err := config.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "gamel10n.json"), cfgData)
	catData, err := catalog.Marshal(&model.SourceCatalog{SchemaVersion: 1, Entries: setup.source})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "source", "catalog.json"), catData)
	for _, locale := range setup.languages {
		m := setup.targets[locale]
		if m == nil {
			m = map[string]string{}
		}
		data, err := target.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, "targets", locale+".json"), data)
	}
	if setup.writeState {
		st := setup.state
		if st == nil {
			st = model.NewState()
		}
		data, err := state.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, "state", "gamel10n-state.json"), data)
	}
	return filepath.Join(dir, "gamel10n.json")
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readTarget(t *testing.T, configPath, locale string) map[string]string {
	t.Helper()
	root := filepath.Dir(configPath)
	data := readFile(t, filepath.Join(root, "targets", locale+".json"))
	m, err := target.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func readState(t *testing.T, configPath string) *model.State {
	t.Helper()
	data := readFile(t, filepath.Join(filepath.Dir(configPath), "state", "gamel10n-state.json"))
	st, err := state.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestSyncTranslatesOnlyNewAndChangedAndPreservesProtected(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	st := model.NewState()
	st.Locale("en").Entries["b"] = &model.EntryState{SourceHash: hashx.Text("B old"), TargetHash: hashx.Text("B old")}
	st.Locale("en").Entries["c"] = &model.EntryState{SourceHash: hashx.Text("C"), TargetHash: hashx.Text("C")}
	st.Locale("en").Entries["d"] = &model.EntryState{SourceHash: hashx.Text("D"), TargetHash: hashx.Text("D human"), Manual: true}
	st.Locale("en").Entries["e"] = &model.EntryState{SourceHash: hashx.Text("E old"), TargetHash: hashx.Text("E human"), Manual: true, Approved: true}
	st.Locale("en").Entries["f"] = &model.EntryState{SourceHash: hashx.Text("F"), TargetHash: hashx.Text("F"), Locked: true}
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source: []model.SourceEntry{
			{Key: "a", Text: "A new"},
			{Key: "b", Text: "B new"},
			{Key: "c", Text: "C"},
			{Key: "d", Text: "D"},
			{Key: "e", Text: "E new"},
			{Key: "f", Text: "F"},
		},
		targets: map[string]map[string]string{"en": {
			"b": "B old", "c": "C", "d": "D human", "e": "E human", "f": "F", "x": "X keep",
		}},
		state: st, writeState: true,
	})
	reportPath := filepath.Join(filepath.Dir(configPath), "reports", "sync.json")
	client, err := translate.New(fake.server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), configPath, client, Options{ReportPath: reportPath})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "success" || !report.WroteFiles {
		t.Fatalf("unexpected report: %#v", report)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("API calls = %d, want 2: %#v", len(fake.calls), fake.calls)
	}
	if fake.calls[0].Text != "A new" || fake.calls[1].Text != "B new" {
		t.Fatalf("unexpected call order: %#v", fake.calls)
	}
	targets := readTarget(t, configPath, "en")
	want := map[string]string{
		"a": "EN:A new", "b": "EN:B new", "c": "C", "d": "D human", "e": "E human", "f": "F", "x": "X keep",
	}
	for key, value := range want {
		if targets[key] != value {
			t.Errorf("target %s = %q, want %q", key, targets[key], value)
		}
	}
	stored := readState(t, configPath)
	if !stored.Entry("en", "d").Manual {
		t.Error("manual flag lost")
	}
	if !stored.Entry("en", "e").NeedsReview || !stored.Entry("en", "e").Approved || !stored.Entry("en", "e").Manual {
		t.Errorf("approved changed entry not preserved for review: %#v", stored.Entry("en", "e"))
	}
	if !stored.Entry("en", "f").Locked {
		t.Error("lock flag lost")
	}
	if stored.Entry("en", "a").Manual || stored.Entry("en", "b").Manual {
		t.Error("machine entries marked manual")
	}
	if report.Counts["new"] != 1 || report.Counts["changed"] != 1 || report.Counts["needs_review"] != 1 || report.Counts["deleted"] != 1 {
		t.Fatalf("unexpected counts: %#v", report.Counts)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("sync report missing: %v", err)
	}

	// Second sync must not call the API or rewrite target/state.
	targetBefore := readFile(t, filepath.Join(filepath.Dir(configPath), "targets", "en.json"))
	stateBefore := readFile(t, filepath.Join(filepath.Dir(configPath), "state", "gamel10n-state.json"))
	report2, err := Run(context.Background(), configPath, client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("repeat sync called API: %d", len(fake.calls))
	}
	if report2.Status != "no_changes" {
		t.Fatalf("repeat sync status = %s", report2.Status)
	}
	if string(readFile(t, filepath.Join(filepath.Dir(configPath), "targets", "en.json"))) != string(targetBefore) {
		t.Fatal("repeat sync rewrote target")
	}
	if string(readFile(t, filepath.Join(filepath.Dir(configPath), "state", "gamel10n-state.json"))) != string(stateBefore) {
		t.Fatal("repeat sync rewrote state")
	}
}

func TestFirstRunExistingHumanTargetIsPreserved(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "k", Text: "Hello"}},
		targets:   map[string]map[string]string{"en": {"k": "Konnichiwa human"}},
	})
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	report, err := Run(context.Background(), configPath, client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("first-run human target triggered API: %#v", fake.calls)
	}
	if report.Counts["unchanged"] != 1 {
		t.Fatalf("unexpected counts: %#v", report.Counts)
	}
	if readTarget(t, configPath, "en")["k"] != "Konnichiwa human" {
		t.Fatal("human target overwritten")
	}
	st := readState(t, configPath)
	if st.Entry("en", "k") == nil || !st.Entry("en", "k").Manual {
		t.Fatalf("first-run target not recorded manual: %#v", st.Entry("en", "k"))
	}
}

func TestManualEditIsPreservedWithoutAPICall(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	st := model.NewState()
	st.Locale("en").Entries["k"] = &model.EntryState{SourceHash: hashx.Text("Hello"), TargetHash: hashx.Text("Hello")}
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "k", Text: "Hello"}},
		targets:   map[string]map[string]string{"en": {"k": "Hello human edit"}},
		state:     st, writeState: true,
	})
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	if _, err := Run(context.Background(), configPath, client, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatal("manual edit triggered API")
	}
	if readTarget(t, configPath, "en")["k"] != "Hello human edit" {
		t.Fatal("manual edit overwritten")
	}
	if !readState(t, configPath).Entry("en", "k").Manual {
		t.Fatal("manual edit not recorded")
	}
}

func TestAPIFailuresLeaveTargetAndStateUnchanged(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"malformed", 200, `{bad`},
		{"truncated", 200, `{"translated_text":"Hello`},
		{"http error", 500, `{"error":{"code":"backend","message":"boom"}}`},
		{"empty", 200, `{"translated_text":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeAPI(t, func(req translate.Request) (int, string) { return tc.status, tc.body })
			st := model.NewState()
			st.Locale("en").Entries["keep"] = &model.EntryState{SourceHash: hashx.Text("Keep"), TargetHash: hashx.Text("Keep")}
			configPath := writeProject(t, fake.server.URL, projectSetup{
				languages: []string{"en"},
				source:    []model.SourceEntry{{Key: "a", Text: "A"}, {Key: "keep", Text: "Keep"}},
				targets:   map[string]map[string]string{"en": {"keep": "Keep"}},
				state:     st, writeState: true,
			})
			root := filepath.Dir(configPath)
			targetBefore := readFile(t, filepath.Join(root, "targets", "en.json"))
			stateBefore := readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))
			reportPath := filepath.Join(root, "reports", "failure.json")
			client, _ := translate.New(fake.server.URL, 5*time.Second)
			report, err := Run(context.Background(), configPath, client, Options{ReportPath: reportPath})
			if err == nil {
				t.Fatal("expected error")
			}
			if report == nil || report.Status != "failed" || report.Error == "" {
				t.Fatalf("unexpected report: %#v", report)
			}
			if string(readFile(t, filepath.Join(root, "targets", "en.json"))) != string(targetBefore) {
				t.Fatal("target changed after API failure")
			}
			if string(readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))) != string(stateBefore) {
				t.Fatal("state changed after API failure")
			}
			if _, err := os.Stat(reportPath); err != nil {
				t.Fatalf("failure report missing: %v", err)
			}
		})
	}
}

func TestPlaceholderMismatchRejectsWholeSync(t *testing.T) {
	fake := newFakeAPI(t, func(req translate.Request) (int, string) {
		return 200, `{"translated_text":"Hello"}`
	})
	st := model.NewState()
	st.Locale("en").Entries["keep"] = &model.EntryState{SourceHash: hashx.Text("Keep"), TargetHash: hashx.Text("Keep")}
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "greet", Text: "Hello {name}"}, {Key: "keep", Text: "Keep"}},
		targets:   map[string]map[string]string{"en": {"keep": "Keep"}},
		state:     st, writeState: true,
	})
	root := filepath.Dir(configPath)
	targetBefore := readFile(t, filepath.Join(root, "targets", "en.json"))
	stateBefore := readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))
	reportPath := filepath.Join(root, "reports", "rejected.json")
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	report, err := Run(context.Background(), configPath, client, Options{ReportPath: reportPath})
	if err == nil {
		t.Fatal("expected placeholder rejection")
	}
	if len(report.Rejected) != 1 || report.Rejected[0].Key != "greet" {
		t.Fatalf("unexpected rejected list: %#v", report.Rejected)
	}
	if len(report.Rejected[0].Issues) == 0 {
		t.Fatal("rejection has no issues")
	}
	if string(readFile(t, filepath.Join(root, "targets", "en.json"))) != string(targetBefore) {
		t.Fatal("broken translation applied to target")
	}
	if string(readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))) != string(stateBefore) {
		t.Fatal("state changed after rejection")
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("rejection report missing: %v", err)
	}
}

func TestMaxLengthIsWarningAndStillApplies(t *testing.T) {
	fake := newFakeAPI(t, func(req translate.Request) (int, string) {
		return 200, `{"translated_text":"very long translation"}`
	})
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "short", Text: "Short", MaxLength: 3}},
		targets:   map[string]map[string]string{"en": {}},
	})
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	report, err := Run(context.Background(), configPath, client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 1 || report.Warnings[0].Kind != "max_length" {
		t.Fatalf("unexpected warnings: %#v", report.Warnings)
	}
	if readTarget(t, configPath, "en")["short"] != "very long translation" {
		t.Fatal("warning blocked translation")
	}
}

func TestLockPreventsConcurrentSync(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "a", Text: "A"}},
		targets:   map[string]map[string]string{"en": {}},
		state:     model.NewState(), writeState: true,
	})
	lockPath := filepath.Join(filepath.Dir(configPath), ".gamel10n.lock")
	writeFile(t, lockPath, []byte("pid=123\n"))
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	if _, err := Run(context.Background(), configPath, client, Options{}); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("want lock error, got %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatal("API called while locked")
	}
}

func TestCommitFailureRollsBackTargetAndState(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	st := model.NewState()
	st.Locale("en").Entries["keep"] = &model.EntryState{SourceHash: hashx.Text("Keep"), TargetHash: hashx.Text("Keep")}
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "a", Text: "A"}, {Key: "b", Text: "B"}, {Key: "keep", Text: "Keep"}},
		targets:   map[string]map[string]string{"en": {"keep": "Keep"}},
		state:     st, writeState: true,
	})
	root := filepath.Dir(configPath)
	targetBefore := readFile(t, filepath.Join(root, "targets", "en.json"))
	stateBefore := readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))
	installed := 0
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	report, err := Run(context.Background(), configPath, client, Options{Hook: func(event string) error {
		if event == "installed" {
			installed++
			if installed == 2 {
				return errors.New("simulated install failure")
			}
		}
		return nil
	}})
	if err == nil {
		t.Fatal("expected commit failure")
	}
	if report == nil || report.Status != "failed" || !strings.Contains(report.Error, "commit failed") {
		t.Fatalf("unexpected report: %#v", report)
	}
	if string(readFile(t, filepath.Join(root, "targets", "en.json"))) != string(targetBefore) {
		t.Fatal("target not rolled back")
	}
	if string(readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))) != string(stateBefore) {
		t.Fatal("state not rolled back")
	}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && strings.Contains(info.Name(), ".gamel10n-bak-") {
			t.Errorf("backup left behind: %s", path)
		}
		return nil
	})
}

func TestConcurrentTargetEditAbortsWithoutOverwrite(t *testing.T) {
	st := model.NewState()
	st.Locale("en").Entries["keep"] = &model.EntryState{SourceHash: hashx.Text("Keep"), TargetHash: hashx.Text("Keep")}
	var fake *fakeAPI
	fake = newFakeAPI(t, func(req translate.Request) (int, string) {
		// Simulate an external editor while the API call is in flight.
		writeFile(t, filepath.Join(filepath.Dir(fake.configPath), "targets", "en.json"), []byte("{\"keep\":\"external\",\"a\":\"external a\"}\n"))
		return translateOK(req)
	})
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "a", Text: "A"}, {Key: "keep", Text: "Keep"}},
		targets:   map[string]map[string]string{"en": {"keep": "Keep"}},
		state:     st, writeState: true,
	})
	fake.configPath = configPath
	root := filepath.Dir(configPath)
	stateBefore := readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	report, err := Run(context.Background(), configPath, client, Options{})
	if err == nil || !strings.Contains(err.Error(), "changed during sync") {
		t.Fatalf("want concurrent-change error, got %v", err)
	}
	if report == nil || report.Status != "failed" {
		t.Fatalf("unexpected report: %#v", report)
	}
	target := string(readFile(t, filepath.Join(root, "targets", "en.json")))
	if !strings.Contains(target, "external a") {
		t.Fatalf("external edit was overwritten: %s", target)
	}
	if string(readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))) != string(stateBefore) {
		t.Fatal("state changed despite abort")
	}
}

func TestDeletedKeysPreservedByDefaultAndPrunedExplicitly(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	st := model.NewState()
	st.Locale("en").Entries["a"] = &model.EntryState{SourceHash: hashx.Text("A"), TargetHash: hashx.Text("A")}
	st.Locale("en").Entries["x"] = &model.EntryState{SourceHash: hashx.Text("X"), TargetHash: hashx.Text("X")}
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "a", Text: "A"}},
		targets:   map[string]map[string]string{"en": {"a": "A", "x": "X"}},
		state:     st, writeState: true,
	})
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	report, err := Run(context.Background(), configPath, client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts["deleted"] != 1 || report.Status != "no_changes" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if readTarget(t, configPath, "en")["x"] != "X" {
		t.Fatal("deleted target was not preserved")
	}

	report, err = Run(context.Background(), configPath, client, Options{PruneDeleted: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 1 {
		t.Fatalf("pruned = %d", report.Pruned)
	}
	if _, ok := readTarget(t, configPath, "en")["x"]; ok {
		t.Fatal("deleted target not pruned")
	}
	stored := readState(t, configPath)
	if stored.Entry("en", "x") != nil {
		t.Fatal("deleted state entry not pruned")
	}
}

type stalledTranslator struct {
	started chan<- struct{}
}

func (s stalledTranslator) Translate(ctx context.Context, req translate.Request) (string, error) {
	select {
	case s.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return "", ctx.Err()
}

type cancelingTranslator struct {
	cancel context.CancelFunc
}

func (c cancelingTranslator) Translate(ctx context.Context, req translate.Request) (string, error) {
	c.cancel()
	return "EN:" + req.Text, nil
}

func TestStyleOnlySentForJapaneseTargets(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages:      []string{"en", "ja"},
		sourceLanguage: "fr",
		source:         []model.SourceEntry{{Key: "a", Text: "A"}},
		targets:        map[string]map[string]string{"en": {}, "ja": {}},
		style:          "polite",
	})
	client, err := translate.New(fake.server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), configPath, client, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("calls = %d", len(fake.calls))
	}
	styles := map[string]string{}
	for _, call := range fake.calls {
		if call.Source != "fr" {
			t.Errorf("source language changed: %q", call.Source)
		}
		styles[call.Target] = call.Style
	}
	if styles["en"] != "" {
		t.Errorf("English target sent style %q", styles["en"])
	}
	if styles["ja"] != "polite" {
		t.Errorf("Japanese target style = %q", styles["ja"])
	}
}

func TestUnsupportedLanguageFailsBeforeTranslation(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	fake.languages = []string{"ja", "en"}
	st := model.NewState()
	st.Locale("en").Entries["keep"] = &model.EntryState{SourceHash: hashx.Text("Keep"), TargetHash: hashx.Text("Keep")}
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en", "pt-BR"},
		source:    []model.SourceEntry{{Key: "a", Text: "A"}, {Key: "keep", Text: "Keep"}},
		targets:   map[string]map[string]string{"en": {"keep": "Keep"}, "pt-BR": {}},
		state:     st, writeState: true,
	})
	root := filepath.Dir(configPath)
	targetBefore := readFile(t, filepath.Join(root, "targets", "en.json"))
	client, err := translate.New(fake.server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), configPath, client, Options{})
	if err == nil {
		t.Fatal("unsupported target accepted")
	}
	if !strings.Contains(err.Error(), `"pt-BR"`) || !strings.Contains(err.Error(), `"pt-br"`) {
		t.Fatalf("error lacks configured and canonical locale: %v", err)
	}
	if report == nil || report.Status != "failed" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("translation called before language check: %#v", fake.calls)
	}
	if string(readFile(t, filepath.Join(root, "targets", "en.json"))) != string(targetBefore) {
		t.Fatal("target changed after unsupported language")
	}
	if _, err := os.Stat(filepath.Join(root, ".gamel10n.lock")); !os.IsNotExist(err) {
		t.Fatal("lock was not released")
	}
}

func TestManualUnsupportedLocaleDoesNotBlockSupportedTranslation(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	fake.languages = []string{"ja", "en"}
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en", "pt-BR"},
		source:    []model.SourceEntry{{Key: "a", Text: "A"}},
		targets:   map[string]map[string]string{"en": {}, "pt-BR": {"a": "Manual Portuguese"}},
	})
	client, err := translate.New(fake.server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), configPath, client, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].Target != "en" {
		t.Fatalf("calls: %#v", fake.calls)
	}
	if !strings.Contains(string(readFile(t, filepath.Join(filepath.Dir(configPath), "targets", "pt-BR.json"))), "Manual Portuguese") {
		t.Fatal("manual locale lost")
	}
}

func TestCancelDuringStalledTranslationPreservesFilesAndLock(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	st := model.NewState()
	st.Locale("en").Entries["keep"] = &model.EntryState{SourceHash: hashx.Text("Keep"), TargetHash: hashx.Text("Keep")}
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "a", Text: "A"}, {Key: "keep", Text: "Keep"}},
		targets:   map[string]map[string]string{"en": {"keep": "Keep"}},
		state:     st, writeState: true,
	})
	root := filepath.Dir(configPath)
	targetBefore := readFile(t, filepath.Join(root, "targets", "en.json"))
	stateBefore := readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, configPath, stalledTranslator{started: started}, Options{})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("translation did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled sync succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled sync did not return")
	}
	if string(readFile(t, filepath.Join(root, "targets", "en.json"))) != string(targetBefore) {
		t.Fatal("target changed after cancellation")
	}
	if string(readFile(t, filepath.Join(root, "state", "gamel10n-state.json"))) != string(stateBefore) {
		t.Fatal("state changed after cancellation")
	}
	if _, err := os.Stat(filepath.Join(root, ".gamel10n.lock")); !os.IsNotExist(err) {
		t.Fatal("lock was not released")
	}

	client, err := translate.New(fake.server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), configPath, client, Options{})
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if report.Status != "success" {
		t.Fatalf("second sync status = %s", report.Status)
	}
	if readTarget(t, configPath, "en")["a"] != "EN:A" {
		t.Fatal("second sync did not translate")
	}
}

func TestCanceledContextAbortsWithoutTranslationBeforeWrite(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "k", Text: "Hello"}},
		targets:   map[string]map[string]string{"en": {"k": "Konnichiwa human"}},
	})
	root := filepath.Dir(configPath)
	targetBefore := readFile(t, filepath.Join(root, "targets", "en.json"))
	client, err := translate.New(fake.server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := Run(ctx, configPath, client, Options{})
	if err == nil || !strings.Contains(err.Error(), "sync canceled") {
		t.Fatalf("want cancellation error, got %v", err)
	}
	if report == nil || report.Status != "failed" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("API called after cancellation: %#v", fake.calls)
	}
	if string(readFile(t, filepath.Join(root, "targets", "en.json"))) != string(targetBefore) {
		t.Fatal("target changed after cancellation")
	}
	if _, err := os.Stat(filepath.Join(root, "state", "gamel10n-state.json")); !os.IsNotExist(err) {
		t.Fatal("state was written after cancellation")
	}
	if _, err := os.Stat(filepath.Join(root, ".gamel10n.lock")); !os.IsNotExist(err) {
		t.Fatal("lock was not released")
	}
}

func TestCancellationAfterTranslationAbortsBeforeCommit(t *testing.T) {
	configPath := writeProject(t, "http://127.0.0.1:1", projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "a", Text: "A"}},
		targets:   map[string]map[string]string{"en": {}},
	})
	root := filepath.Dir(configPath)
	targetBefore := readFile(t, filepath.Join(root, "targets", "en.json"))
	ctx, cancel := context.WithCancel(context.Background())
	report, err := Run(ctx, configPath, cancelingTranslator{cancel: cancel}, Options{})
	if err == nil || !strings.Contains(err.Error(), "sync canceled") {
		t.Fatalf("want cancellation error, got %v", err)
	}
	if report == nil || report.Status != "failed" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if string(readFile(t, filepath.Join(root, "targets", "en.json"))) != string(targetBefore) {
		t.Fatal("target committed after cancellation")
	}
	if _, err := os.Stat(filepath.Join(root, "state", "gamel10n-state.json")); !os.IsNotExist(err) {
		t.Fatal("state committed after cancellation")
	}
	if _, err := os.Stat(filepath.Join(root, ".gamel10n.lock")); !os.IsNotExist(err) {
		t.Fatal("lock was not released")
	}
}

func TestGlossaryContextAndMaxTokensReachAPI(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	configPath := writeProject(t, fake.server.URL, projectSetup{
		languages: []string{"en"},
		source:    []model.SourceEntry{{Key: "a", Text: "勇者", Context: "menu"}},
		targets:   map[string]map[string]string{"en": {}},
		style:     "default", maxTokens: 64,
		glossary: []model.GlossaryTerm{
			{Source: "勇者", Target: "Hero", Locale: "en"},
			{Source: "魔王", Target: "Demon King", Locale: "fr"},
			{Source: "HP", Target: "HP"},
		},
	})
	client, _ := translate.New(fake.server.URL, 5*time.Second)
	if _, err := Run(context.Background(), configPath, client, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls = %d", len(fake.calls))
	}
	req := fake.calls[0]
	if req.Source != "ja" || req.Target != "en" || req.Style != "" || req.Context != "menu" || req.MaxTokens != 64 {
		t.Fatalf("unexpected request: %#v", req)
	}
	if len(req.Glossary) != 2 {
		t.Fatalf("glossary = %#v", req.Glossary)
	}
	if req.Glossary[0].Target != "Hero" || req.Glossary[1].Target != "HP" {
		t.Fatalf("unexpected glossary: %#v", req.Glossary)
	}
}
