package server

import (
	"net"
	"net/http"
	"strings"
)

func (s *Server) absoluteURLFromRequest(r *http.Request, path string) string {
	rel := s.goURL(path)
	if r == nil {
		return rel
	}
	host := publicRequestHost(r)
	if host == "" {
		return rel
	}
	return requestScheme(r) + "://" + host + rel
}

// publicRequestHost is the browser-facing host for this request.
// The supported Apache proxy sends X-Forwarded-Host. A direct connection
// has only r.Host. An empty forwarded header falls through to r.Host.
func publicRequestHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	if host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); host != "" {
		return host
	}
	return strings.TrimSpace(r.Host)
}

// requestScheme reports https when the connection is TLS or the supported
// proxy set X-Forwarded-Proto to https. Otherwise the request is http.
func requestScheme(r *http.Request) string {
	if r != nil && (r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")) {
		return "https"
	}
	return "http"
}

// canonicalHost compares hostnames without default ports.
// :443 is omitted for https and :80 is omitted for http. Any other port
// stays part of the identity, so a different port is a different host.
func canonicalHost(hostport, scheme string) string {
	host, port := splitHostPort(strings.TrimSpace(hostport))
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "" {
		return ""
	}
	switch port {
	case "443":
		if strings.EqualFold(scheme, "https") {
			port = ""
		}
	case "80":
		if strings.EqualFold(scheme, "http") {
			port = ""
		}
	}
	if port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

func splitHostPort(hostport string) (host, port string) {
	if hostport == "" {
		return "", ""
	}
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return h, p
	}
	return hostport, ""
}
