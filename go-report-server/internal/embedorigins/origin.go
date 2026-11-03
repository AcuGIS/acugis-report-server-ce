// Package embedorigins is the single allow-list for Embedded Analytics.
//
// Environment origins are the process startup value of
// ACUGIS_EMBED_ALLOWED_ORIGINS (alias JRS_EMBED_ALLOWED_ORIGINS), kept with
// their existing parsing rules. Admin origins are exact, normalized origins
// persisted in PostgreSQL. Every embed security check uses the merged list.
package embedorigins

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

var ErrInvalid = errors.New("invalid origin")

// FrameAncestors returns a Content-Security-Policy frame-ancestors source list.
// An empty list is 'none', matching the historical config behavior.
func FrameAncestors(origins []string) string {
	if len(origins) == 0 {
		return "'none'"
	}
	return strings.Join(origins, " ")
}

// NormalizeOrigin accepts an exact http(s) origin and returns the canonical
// form: lowercase scheme and host, default ports removed.
//
// This is the Admin UI rule. Environment entries are not passed through it,
// so existing values such as "*" stay valid system origins.
func NormalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return "", ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalid
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalid
	}
	if u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return "", ErrInvalid
	}
	if u.Path != "" || u.RawPath != "" {
		return "", ErrInvalid
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.Contains(host, "*") || strings.ContainsAny(host, "/?#@") {
		return "", ErrInvalid
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return "", ErrInvalid
		}
		if (scheme == "http" && n == 80) || (scheme == "https" && n == 443) {
			port = ""
		} else {
			port = strconv.Itoa(n)
		}
	}
	var b strings.Builder
	b.Grow(len(scheme) + len(host) + len(port) + 4)
	b.WriteString(scheme)
	b.WriteString("://")
	if strings.Contains(host, ":") {
		b.WriteByte('[')
		b.WriteString(host)
		b.WriteByte(']')
	} else {
		b.WriteString(host)
	}
	if port != "" {
		b.WriteByte(':')
		b.WriteString(port)
	}
	return b.String(), nil
}
