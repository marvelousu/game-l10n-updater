package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marvelousu/game-l10n-updater/internal/model"
	"github.com/marvelousu/game-l10n-updater/internal/translate"
)

func TestLockReleaseFailureIsWrittenToReport(t *testing.T) {
	fake := newFakeAPI(t, translateOK)
	configPath := writeProject(t, fake.server.URL, projectSetup{languages: []string{"en"}, source: []model.SourceEntry{{Key: "a", Text: "A"}}, targets: map[string]map[string]string{"en": {}}})
	root := filepath.Dir(configPath)
	reportPath := filepath.Join(root, "report.json")
	client, err := translate.New(fake.server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), configPath, client, Options{ReportPath: reportPath, Hook: func(event string) error {
		if event != "committed" {
			return nil
		}
		path := filepath.Join(root, ".gamel10n.lock")
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(path, "held"), []byte("fixture"), 0600)
	}})
	if err == nil || report.Status != "failed" || !report.WroteFiles {
		t.Fatalf("report%+v error%v", report, err)
	}
	var stored Report
	if err := json.Unmarshal(readFile(t, reportPath), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status != "failed" || !strings.Contains(stored.Error, "remove lock") {
		t.Fatalf("stored report lost failure: %+v", stored)
	}
}

func TestEarlierWarningsRemainInRejectedSyncReport(t *testing.T) {
	fake := newFakeAPI(t, func(req translate.Request) (int, string) {
		if req.Text == "Hi {name}" {
			return http.StatusOK, `{"translated_text":"Bad"}`
		}
		return translateOK(req)
	})
	configPath := writeProject(t, fake.server.URL, projectSetup{languages: []string{"en"}, source: []model.SourceEntry{{Key: "a", Text: "A", MaxLength: 1}, {Key: "b", Text: "Hi {name}"}}, targets: map[string]map[string]string{"en": {}}})
	client, err := translate.New(fake.server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), configPath, client, Options{})
	if err == nil || report.WroteFiles || len(report.Warnings) != 1 || report.Warnings[0].Key != "a" {
		t.Fatalf("warning missing: %+v, %v", report, err)
	}
}
