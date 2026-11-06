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
	for _, field := range []string{"text", "source", "target", "glossary", "context", "max_tokens"} {
		if !strings.Contains(string(raw), `"`+field+`"`) {
			t.Errorf("payload missing %q: %s", field, raw)
		}
	}
	if strings.Contains(string(raw), `"style"`) {
		t.Errorf("empty style must be omitted: %s", raw)
	}
	if !strings.Contains(string(raw), `"glossary":[]`) {
		t.Errorf("empty glossary must be an array: %s", raw)
	}
}

func TestStyleIsSentOnlyForJapaneseTarget(t *testing.T) {
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

	if _, err := client.Translate(context.Background(), Request{Text: "x", Source: "en", Target: "ja", Style: "polite"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"style":"polite"`) {
		t.Fatalf("Japanese target lost style: %s", raw)
	}

	if _, err := client.Translate(context.Background(), Request{Text: "x", Source: "ja", Target: "en", Style: "polite"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"style"`) {
		t.Fatalf("non-Japanese target sent style: %s", raw)
	}
}

func TestWireLanguagesAreCanonical(t *testing.T) {
	cases := map[string]string{
		"ja":      "ja",
		"EN":      "en",
		"zh_CN":   "zh-hans",
		"zh-cn":   "zh-hans",
		"zh-SG":   "zh-hans",
		"zh_TW":   "zh-hant",
		"zh-HK":   "zh-hant",
		"pt-BR":   "pt-br",
		"pt_BR":   "pt-br",
		"zh-hant": "zh-hant",
	}
	for input, want := range cases {
		if got := CanonicalLanguage(input); got != want {
			t.Errorf("CanonicalLanguage(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTranslateSendsCanonicalLanguageCodes(t *testing.T) {
	var got Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"translated_text":"ok"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Translate(context.Background(), Request{Text: "x", Source: "zh_TW", Target: "zh-CN", Style: "polite"}); err != nil {
		t.Fatal(err)
	}
	if got.Source != "zh-hant" || got.Target != "zh-hans" || got.Style != "" {
		t.Fatalf("unexpected wire request: %#v", got)
	}
}

func TestSupportedLanguages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/languages" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"languages":[{"code":"ja","name":"Japanese"},{"code":"zh-hans","name":"Chinese"}],"count":2}`))
	}))
	defer server.Close()
	client, err := New(server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := client.SupportedLanguages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 2 || codes[0] != "ja" || codes[1] != "zh-hans" {
		t.Fatalf("unexpected languages: %#v", codes)
	}
}

func TestSupportedLanguagesFailures(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{"http error", http.StatusBadGateway, `{"error":{"code":"backend","message":"boom"}}`, "backend"},
		{"malformed", http.StatusOK, `{bad`, "malformed_response"},
		{"empty", http.StatusOK, `{"languages":[]}`, "languages is empty"},
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
			_, err = client.SupportedLanguages(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want error containing %q, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestDefaultTimeoutIsLongerThanServerTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	client, err := New(server.URL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if client.http.Timeout != DefaultTimeout || DefaultTimeout != 210*time.Second {
		t.Fatalf("timeout = %s, want 210s", client.http.Timeout)
	}

	short, err := New(server.URL, 45*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if short.http.Timeout != 45*time.Second {
		t.Fatalf("explicit timeout = %s, want 45s", short.http.Timeout)
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
