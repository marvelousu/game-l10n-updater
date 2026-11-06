package netx

import "testing"

func TestValidateLoopback(t *testing.T) {
	valid := []string{
		"http://127.0.0.1:8787",
		"http://127.0.0.2:8787/translate",
		"http://localhost:8787",
		"http://[::1]:8787",
		"https://localhost",
	}
	for _, raw := range valid {
		if err := ValidateLoopback(raw); err != nil {
			t.Errorf("ValidateLoopback(%q) = %v", raw, err)
		}
	}
	invalid := []string{
		"http://example.com:8787",
		"http://192.168.1.2:8787",
		"http://0.0.0.0:8787",
		"ftp://127.0.0.1:8787",
		"http://user:pass@127.0.0.1:8787",
		"127.0.0.1:8787",
		"",
	}
	for _, raw := range invalid {
		if err := ValidateLoopback(raw); err == nil {
			t.Errorf("ValidateLoopback(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestTranslateEndpoint(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:8787":            "http://127.0.0.1:8787/translate",
		"http://127.0.0.1:8787/":           "http://127.0.0.1:8787/translate",
		"http://127.0.0.1:8787/translate":  "http://127.0.0.1:8787/translate",
		"http://127.0.0.1:8787/api":        "http://127.0.0.1:8787/api/translate",
		"http://localhost:8787/translate/": "http://localhost:8787/translate",
	}
	for raw, want := range cases {
		got, err := TranslateEndpoint(raw)
		if err != nil {
			t.Fatalf("TranslateEndpoint(%q): %v", raw, err)
		}
		if got != want {
			t.Errorf("TranslateEndpoint(%q) = %q, want %q", raw, got, want)
		}
	}
}
