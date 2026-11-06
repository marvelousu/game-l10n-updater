// Package netx validates the loopback-only translation endpoint.
package netx

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateLoopback accepts only http(s) URLs whose hostname is a loopback IP
// or localhost. This release deliberately cannot call remote services.
func ValidateLoopback(raw string) error {
	u, err := parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("api url scheme must be http or https: %q", raw)
	}
	if u.User != nil {
		return fmt.Errorf("api url must not contain credentials")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("api url must include a host")
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("api url host %q is not loopback (only localhost/127.0.0.0/8/::1 are allowed)", host)
		}
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("api url must not contain a query or fragment")
	}
	return nil
}

// TranslateEndpoint validates raw and returns the POST /translate URL.
func TranslateEndpoint(raw string) (string, error) {
	if err := ValidateLoopback(raw); err != nil {
		return "", err
	}
	u, _ := parse(raw)
	path := strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(path, "/translate") {
		path += "/translate"
	}
	u.Path = path
	u.RawPath = ""
	return u.String(), nil
}

func parse(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("api url must not be empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse api url: %w", err)
	}
	return u, nil
}
