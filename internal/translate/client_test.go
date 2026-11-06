package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTranslateRequestAndResponse(t *testing.T) {
	var got Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/translate" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"translated_text":"Hello {name}","translatedText":"ignored","model":"fake"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	text, err := client.Translate(context.Background(), Request{
		Text: "こんにちは {name}", Source: "ja", Target: "en", Style: "",
		Glossary: []GlossaryEntry{{Source: "勇者", Target: "Hero"}}, Context: "button", MaxTokens: 128,
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hello {name}" {
		t.Fatalf("text = %q", text)
	}
	if got.Text != "こんにちは {name}" || got.Source != "ja" || got.Target != "en" || got.Context != "button" || got.MaxTokens != 128 {
		t.Fatalf("unexpected request: %#v", got)
	}
	if len(got.Glossary) != 1 || got.Glossary[0].Source != "勇者" || got.Glossary[0].Target != "Hero" {
		t.Fatalf("unexpected glossary: %#v", got.Glossary)
	}
}

func TestTranslateIncludesAllContractFields(t *testing.T) {
	var raw []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"translated_text":"ok"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Translate(context.Background(), Request{Text: "x", Source: "ja", Target: "en"}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"text", "source", "target", "style", "glossary", "context", "max_tokens"} {
		if !strings.Contains(string(raw), `"`+field+`"`) {
			t.Errorf("payload missing %q: %s", field, raw)
		}
	}
	if !strings.Contains(string(raw), `"glossary":[]`) {
		t.Errorf("empty glossary must be an array: %s", raw)
	}
}

func TestTranslateFailures(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{"http error", http.StatusBadGateway, `{"error":{"code":"backend","message":"boom"}}`, "backend"},
		{"malformed", http.StatusOK, `{not json`, "malformed_response"},
		{"truncated", http.StatusOK, `{"translated_text":"Hello`, "malformed_response"},
		{"empty", http.StatusOK, `{"translated_text":"   "}`, "empty_translation"},
		{"trailing", http.StatusOK, `{"translated_text":"x"} {}`, "malformed_response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := New(server.URL, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Translate(context.Background(), Request{Text: "x", Source: "ja", Target: "en"})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}

func TestNewRejectsRemoteEndpoint(t *testing.T) {
	if _, err := New("http://example.com:8787", time.Second); err == nil {
		t.Fatal("remote endpoint accepted")
	}
}

func TestTranslateNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()
	client, err := New(url, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Translate(context.Background(), Request{Text: "x", Source: "ja", Target: "en"})
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("want network error, got %v", err)
	}
}

func TestRedirectToRemoteIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/translate", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := New(server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Translate(context.Background(), Request{Text: "x", Source: "ja", Target: "en"})
	if err == nil || !strings.Contains(err.Error(), "redirect rejected") {
		t.Fatalf("want redirect rejection, got %v", err)
	}
}
