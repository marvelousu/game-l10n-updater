package translate

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

type redirectTransport struct{ calls int }

func (r *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls++
	if r.calls == 1 {
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": {"http://example.com/leak"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"translated_text":"leaked"}`)), Request: req}, nil
}

func TestInjectedHTTPClientRejectsNonLoopbackRedirect(t *testing.T) {
	transport := &redirectTransport{}
	borrowed := &http.Client{Transport: transport}
	client, err := NewWithHTTPClient("http://127.0.0.1:8787", borrowed)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Translate(context.Background(), Request{Text: "Private source", Source: "en", Target: "ja"})
	if err == nil || transport.calls != 1 {
		t.Fatalf("redirect escaped guard: calls%d err%v", transport.calls, err)
	}
	if borrowed.CheckRedirect != nil {
		t.Fatal("constructor changed caller's HTTP client")
	}
}

func TestHTTPErrorTruncationKeepsValidJapanese(t *testing.T) {
	err := parseHTTPError(503, []byte(strings.Repeat("翻訳", 400))).(*APIError)
	if !utf8.ValidString(err.Message) || len([]rune(err.Message)) != 303 || !strings.HasSuffix(err.Message, "...") {
		t.Fatalf("invalid truncated error: %q", err.Message)
	}
}
