//go:build linux || darwin

package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marvelousu/game-l10n-updater/internal/cli"
	"github.com/marvelousu/game-l10n-updater/internal/config"
)

func TestSignalHelper(t *testing.T) {
	if os.Getenv("GAMEL10N_SIGNAL_HELPER") != "1" {
		return
	}
	os.Args = []string{"gamel10n", "sync", "-config", os.Getenv("GAMEL10N_SIGNAL_CONFIG")}
	main()
}

func TestInterruptReleasesLockAndAllowsNextSync(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/languages" {
			io.WriteString(w, `{"languages":[{"code":"ja"},{"code":"en"}]}`)
			return
		}
		io.Copy(io.Discard, r.Body)
		if calls.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return
		}
		io.WriteString(w, `{"translated_text":"Hello, {name}!"}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	if code := cli.Run([]string{"init", "-dir", dir, "-source-lang", "ja", "-languages", "en"}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("init exit%d", code)
	}
	configPath := filepath.Join(dir, "gamel10n.json")
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Config.API.URL = server.URL
	data, err := config.Marshal(loaded.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	source := []byte(`{"schema_version":1,"entries":[{"key":"ui.greet","text":"Hi, {name}!"}]}`)
	if err := os.WriteFile(filepath.Join(dir, "source", "catalog.json"), source, 0600); err != nil {
		t.Fatal(err)
	}
	targetPath, statePath := filepath.Join(dir, "targets", "en.json"), filepath.Join(dir, "state", "gamel10n-state.json")
	beforeTarget, _ := os.ReadFile(targetPath)
	beforeState, _ := os.ReadFile(statePath)
	cmd := exec.Command(os.Args[0], "-test.run=^TestSignalHelper$")
	cmd.Env = append(os.Environ(), "GAMEL10N_SIGNAL_HELPER=1", "GAMEL10N_SIGNAL_CONFIG="+configPath)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("sync did not reach backend")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatalf("interrupted sync succeeded: %s", output.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("interrupted sync did not exit")
	}
	if _, err := os.Stat(filepath.Join(dir, ".gamel10n.lock")); !os.IsNotExist(err) {
		t.Fatalf("stale lock: %v", err)
	}
	afterTarget, _ := os.ReadFile(targetPath)
	afterState, _ := os.ReadFile(statePath)
	if !bytes.Equal(beforeTarget, afterTarget) || !bytes.Equal(beforeState, afterState) {
		t.Fatal("interrupted sync changed translation or state")
	}
	var retry bytes.Buffer
	if code := cli.Run([]string{"sync", "-config", configPath}, &retry, &retry); code != 0 {
		t.Fatalf("retry exit%d: %s", code, retry.String())
	}
}
