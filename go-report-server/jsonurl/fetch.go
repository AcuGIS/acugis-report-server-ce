// Package jsonurl fetches one HTTP or HTTPS JSON document.
// It is not a REST client: GET only, no headers, no body, no authentication,
// and no parameter substitution. Destinations are checked before the request
// is sent and again for every redirect.
package jsonurl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// MaxBytes is the largest response body that will be kept.
	MaxBytes = 32 << 20
	// MaxRedirects is the number of redirects followed after the original request.
	MaxRedirects = 5

	requestTimeout = 30 * time.Second
	dialTimeout    = 10 * time.Second
)

var (
	// ErrInvalid means the URL is malformed or is not an HTTP(S) URL.
	ErrInvalid = errors.New("JSON URL is not valid")
	// ErrRejected means the destination is not allowed.
	ErrRejected = errors.New("JSON URL is not allowed")
	// ErrFailed means the request did not complete.
	ErrFailed = errors.New("JSON URL request failed")
	// ErrTooLarge means the response exceeded MaxBytes.
	ErrTooLarge = errors.New("JSON response is too large")
)

type policy struct {
	permitPrivate bool
	maxBytes      int64
	timeout       time.Duration
	maxRedirects  int
}

func defaultPolicy() policy {
	return policy{
		maxBytes:     MaxBytes,
		timeout:      requestTimeout,
		maxRedirects: MaxRedirects,
	}
}

// Validate checks the URL shape and rejects literal private, loopback,
// link-local, and metadata destinations. It does not resolve DNS and does
// not send a request.
func Validate(raw string) error {
	return validateRaw(raw, defaultPolicy(), false)
}

// Get fetches the document with GET. Redirects are limited and rechecked.
// Errors do not include the destination address or response body.
func Get(ctx context.Context, raw string) ([]byte, error) {
	return fetch(ctx, raw, defaultPolicy())
}

func fetch(ctx context.Context, raw string, pol policy) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parsed, err := parseURL(raw)
	if err != nil {
		return nil, err
	}
	if err := screen(parsed, pol, true); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, pol.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, ErrInvalid
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "AcuGIS-JSON-Document")
	client := &http.Client{
		Timeout: pol.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > pol.maxRedirects {
				return ErrRejected
			}
			return screen(req.URL, pol, true)
		},
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialChecked(ctx, network, addr, pol)
			},
			TLSHandshakeTimeout:   dialTimeout,
			ResponseHeaderTimeout: pol.timeout,
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     false,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, ErrRejected) {
			return nil, ErrRejected
		}
		if errors.Is(err, ErrInvalid) {
			return nil, ErrInvalid
		}
		return nil, ErrFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("JSON URL returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > pol.maxBytes {
		return nil, ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, pol.maxBytes+1))
	if err != nil {
		return nil, ErrFailed
	}
	if int64(len(body)) > pol.maxBytes {
		return nil, ErrTooLarge
	}
	return body, nil
}

func validateRaw(raw string, pol policy, resolve bool) error {
	parsed, err := parseURL(raw)
	if err != nil {
		return err
	}
	return screen(parsed, pol, resolve)
}

func parseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return nil, ErrInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return nil, ErrInvalid
	}
	if parsed.User != nil {
		return nil, ErrRejected
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrRejected
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return nil, ErrInvalid
	}
	if parsed.Opaque != "" {
		return nil, ErrInvalid
	}
	return parsed, nil
}

func screen(parsed *url.URL, pol policy, resolve bool) error {
	if parsed == nil {
		return ErrInvalid
	}
	if parsed.User != nil {
		return ErrRejected
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrRejected
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || strings.Contains(host, "%") {
		return ErrInvalid
	}
	if blockedHost(host) {
		return ErrRejected
	}
	if ip := net.ParseIP(host); ip != nil {
		if !allowedIP(ip, pol.permitPrivate) {
			return ErrRejected
		}
		return nil
	}
	if ambiguousNumericHost(host) {
		return ErrRejected
	}
	if !resolve {
		return nil
	}
	ips, err := lookupHost(context.Background(), host)
	if err != nil || len(ips) == 0 {
		return ErrFailed
	}
	for _, ip := range ips {
		if !allowedIP(ip, pol.permitPrivate) {
			return ErrRejected
		}
	}
	return nil
}

func blockedHost(host string) bool {
	switch host {
	case "localhost", "localhost.localdomain", "metadata", "metadata.google.internal", "metadata.goog", "instance-data", "kubernetes", "kubernetes.default", "kubernetes.default.svc":
		return true
	}
	for _, suffix := range []string{".localhost", ".localdomain", ".local", ".internal"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func ambiguousNumericHost(host string) bool {
	if host == "" {
		return false
	}
	digits := true
	for _, r := range host {
		if r < '0' || r > '9' {
			digits = false
			break
		}
	}
	if digits {
		return true
	}
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return net.ParseIP(host) == nil
}

func allowedIP(ip net.IP, permitPrivate bool) bool {
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	if !permitPrivate && (ip.IsLoopback() || ip.IsPrivate()) {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		if len(ip) == net.IPv6len && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
			return false
		}
		if ip[0] == 0x01 && ip[1] == 0x00 {
			return false
		}
		return true
	}
	if v4[0] == 0 || v4[0] >= 240 {
		return false
	}
	if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	if v4[0] == 192 && v4[1] == 0 && v4[2] == 0 {
		return false
	}
	if v4[0] == 192 && v4[1] == 0 && v4[2] == 2 {
		return false
	}
	if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
		return false
	}
	if v4[0] == 198 && v4[1] == 51 && v4[2] == 100 {
		return false
	}
	if v4[0] == 203 && v4[1] == 0 && v4[2] == 113 {
		return false
	}
	return true
}

var lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

func dialChecked(ctx context.Context, network, addr string, pol policy) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, ErrInvalid
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if blockedHost(host) || ambiguousNumericHost(host) {
		return nil, ErrRejected
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		found, err := lookupHost(ctx, host)
		if err != nil || len(found) == 0 {
			return nil, ErrFailed
		}
		ips = found
	}
	for _, ip := range ips {
		if !allowedIP(ip, pol.permitPrivate) {
			return nil, ErrRejected
		}
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	var last error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	_ = last
	return nil, ErrFailed
}
