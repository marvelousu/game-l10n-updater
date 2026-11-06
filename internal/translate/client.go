// Package translate calls the loopback-only POST /translate API.
package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/marvelousu/game-l10n-updater/internal/netx"
)

// DefaultTimeout is the default HTTP timeout. It is longer than the
// translation server's 180 second default so a slow model response is not
// cut off by the client first.
const DefaultTimeout = 210 * time.Second

// GlossaryEntry is one terminology pair sent to the API.
type GlossaryEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Request is the public /translate request body. Style is omitted from the
// wire when empty and is only set by sync for Japanese targets.
type Request struct {
	Text      string          `json:"text"`
	Source    string          `json:"source"`
	Target    string          `json:"target"`
	Style     string          `json:"style,omitempty"`
	Glossary  []GlossaryEntry `json:"glossary"`
	Context   string          `json:"context"`
	MaxTokens int             `json:"max_tokens"`
}

// Response is the success body. translated_text is required by the contract.
type Response struct {
	TranslatedText string `json:"translated_text"`
}

// APIError describes a failed API call.
type APIError struct {
	Kind    string
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("translation API: ")
	if e.Kind != "" {
		b.WriteString(e.Kind)
	}
	if e.Status != 0 {
		fmt.Fprintf(&b, " (HTTP %d)", e.Status)
	}
	if e.Code != "" {
		b.WriteString(" [")
		b.WriteString(e.Code)
		b.WriteString("]")
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// CanonicalLanguage returns the language code used on the API wire. Locale
// keys and file names keep their configured spelling; only the request body is
// normalized here. Matching is case-insensitive, underscores become hyphens,
// and only the Chinese script variants are remapped. Other regional variants
// such as pt-BR are left alone so the /languages preflight can reject them.
func CanonicalLanguage(locale string) string {
	code := strings.ToLower(strings.TrimSpace(locale))
	code = strings.ReplaceAll(code, "_", "-")
	switch code {
	case "zh-cn", "zh-sg":
		return "zh-hans"
	case "zh-tw", "zh-hk":
		return "zh-hant"
	default:
		return code
	}
}

// Client calls one configured /translate endpoint.
type Client struct {
	endpoint          string
	languagesEndpoint string
	http              *http.Client
}

// New validates baseURL and creates a client. Only loopback endpoints are
// accepted, and redirects to non-loopback hosts are rejected.
func New(baseURL string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	hc := &http.Client{Timeout: timeout, CheckRedirect: loopbackRedirect}
	return NewWithHTTPClient(baseURL, hc)
}

func loopbackRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if err := netx.ValidateLoopback(req.URL.String()); err != nil {
		return fmt.Errorf("redirect rejected: %w", err)
	}
	return nil
}

// NewWithHTTPClient is New with an injected HTTP client (used by tests).
func NewWithHTTPClient(baseURL string, httpClient *http.Client) (*Client, error) {
	endpoint, err := netx.TranslateEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	return &Client{
		endpoint:          endpoint,
		languagesEndpoint: strings.TrimSuffix(endpoint, "/translate") + "/languages",
		http:              httpClient,
	}, nil
}

// Endpoint returns the resolved POST URL.
func (c *Client) Endpoint() string { return c.endpoint }

// Translate sends one non-streaming request and returns translated_text.
func (c *Client) Translate(ctx context.Context, req Request) (string, error) {
	req.Source = CanonicalLanguage(req.Source)
	req.Target = CanonicalLanguage(req.Target)
	if req.Target != "ja" {
		req.Style = ""
	}
	if req.Glossary == nil {
		req.Glossary = []GlossaryEntry{}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("encode translate request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build translate request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", &APIError{Kind: "network", Err: err}
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 10<<20)
	raw, readErr := io.ReadAll(limited)
	if readErr != nil {
		return "", &APIError{Kind: "read_response", Status: resp.StatusCode, Err: readErr}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", parseHTTPError(resp.StatusCode, raw)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	var out Response
	if err := dec.Decode(&out); err != nil {
		return "", &APIError{Kind: "malformed_response", Status: resp.StatusCode, Err: err}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return "", &APIError{Kind: "malformed_response", Status: resp.StatusCode, Message: "unexpected trailing JSON value"}
		}
		return "", &APIError{Kind: "malformed_response", Status: resp.StatusCode, Err: err}
	}
	if strings.TrimSpace(out.TranslatedText) == "" {
		return "", &APIError{Kind: "empty_translation", Status: resp.StatusCode, Message: "translated_text is empty"}
	}
	if strings.ContainsRune(out.TranslatedText, 0) {
		return "", &APIError{Kind: "invalid_translation", Status: resp.StatusCode, Message: "translated_text contains NUL"}
	}
	return out.TranslatedText, nil
}

// SupportedLanguages fetches GET /languages once through the same client and
// returns the canonical codes advertised by the translation service.
func (c *Client) SupportedLanguages(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.languagesEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build languages request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, &APIError{Kind: "network", Err: err}
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if readErr != nil {
		return nil, &APIError{Kind: "read_response", Status: resp.StatusCode, Err: readErr}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseHTTPError(resp.StatusCode, raw)
	}

	var doc struct {
		Languages []struct {
			Code string `json:"code"`
		} `json:"languages"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&doc); err != nil {
		return nil, &APIError{Kind: "malformed_response", Status: resp.StatusCode, Err: err}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, &APIError{Kind: "malformed_response", Status: resp.StatusCode, Message: "unexpected trailing JSON value"}
		}
		return nil, &APIError{Kind: "malformed_response", Status: resp.StatusCode, Err: err}
	}
	codes := make([]string, 0, len(doc.Languages))
	for _, item := range doc.Languages {
		code := CanonicalLanguage(item.Code)
		if code != "" {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return nil, &APIError{Kind: "malformed_response", Status: resp.StatusCode, Message: "languages is empty"}
	}
	return codes, nil
}

func parseHTTPError(status int, raw []byte) error {
	var doc struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &doc)
	message := doc.Error.Message
	if message == "" {
		message = strings.TrimSpace(string(raw))
		if len(message) > 300 {
			message = message[:300] + "..."
		}
	}
	return &APIError{
		Kind:    "http",
		Status:  status,
		Code:    doc.Error.Code,
		Message: message,
	}
}
