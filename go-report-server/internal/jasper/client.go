package jasper

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/config"
)

const (
	proxyHeaderName  = "X-JRS-Proxied-By"
	proxyHeaderValue = "acugis-report-server-go"
	maxDiagnosticBody = 4096
)

// Client is the low-level HTTP transport to Tomcat/JasperReports.
// Other packages must not speak to Tomcat directly; use Gateway instead.
type Client struct {
	reportURL  string
	imageURL   string
	timeout    time.Duration
	httpClient *http.Client
}

// New constructs a Jasper client from configuration.
func New(cfg config.JasperConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = config.DefaultJasperTimeout
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// Disable response header timeout at transport; overall deadline comes from context.
		ResponseHeaderTimeout: 0,
	}
	return &Client{
		reportURL: strings.TrimRight(cfg.ReportURL, "/"),
		imageURL:  strings.TrimRight(cfg.ImageURL, "/"),
		timeout:   timeout,
		httpClient: &http.Client{
			// Timeout is enforced via request context so streaming can use the full budget.
			Timeout: 0,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// ReportURL returns the configured report execution base URL.
func (c *Client) ReportURL() string {
	if c == nil {
		return ""
	}
	return c.reportURL
}

// ImageURL returns the configured report image base URL.
func (c *Client) ImageURL() string {
	if c == nil {
		return ""
	}
	return c.imageURL
}

// Timeout returns the configured request timeout.
func (c *Client) Timeout() time.Duration {
	if c == nil || c.timeout <= 0 {
		return config.DefaultJasperTimeout
	}
	return c.timeout
}

// Configured reports whether both Jasper endpoints are set.
func (c *Client) Configured() bool {
	return c != nil && c.reportURL != "" && c.imageURL != ""
}

// HealthCheckResult is returned by CheckHealth.
type HealthCheckResult struct {
	Configured   bool          `json:"configured"`
	Reachability Reachability  `json:"reachability"`
	Message      string        `json:"message"`
	CheckedURL   string        `json:"checked_url,omitempty"`
	StatusCode   int           `json:"status_code,omitempty"`
	Duration     time.Duration `json:"-"`
}

// Reachability describes whether Jasper was checked.
type Reachability string

const (
	ReachabilityUnknown     Reachability = "unknown"
	ReachabilityNotChecked  Reachability = "not_checked"
	ReachabilityReachable   Reachability = "reachable"
	ReachabilityUnreachable Reachability = "unreachable"
)

// CheckHealth probes TCP connectivity to the report URL host without issuing
// a report execution request.
func (c *Client) CheckHealth(ctx context.Context) HealthCheckResult {
	if c == nil || !c.Configured() {
		return HealthCheckResult{
			Configured:   false,
			Reachability: ReachabilityUnknown,
			Message:      "Jasper endpoints are not configured",
		}
	}
	start := time.Now()
	u := c.reportURL
	hostPort, err := hostPortFromURL(u)
	if err != nil {
		return HealthCheckResult{
			Configured:   true,
			Reachability: ReachabilityUnreachable,
			Message:      "invalid jasper report URL",
			CheckedURL:   u,
			Duration:     time.Since(start),
		}
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return HealthCheckResult{
			Configured:   true,
			Reachability: ReachabilityUnreachable,
			Message:      "jasper TCP endpoint unreachable",
			CheckedURL:   u,
			Duration:     time.Since(start),
		}
	}
	_ = conn.Close()
	return HealthCheckResult{
		Configured:   true,
		Reachability: ReachabilityReachable,
		Message:      "jasper TCP endpoint reachable",
		CheckedURL:   u,
		Duration:     time.Since(start),
	}
}

func hostPortFromURL(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	host := req.URL.Host
	if host == "" {
		return "", fmt.Errorf("missing host")
	}
	if !strings.Contains(host, ":") {
		if req.URL.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	return host, nil
}

// DoStream performs a context-aware HTTP request and returns the live response.
// The caller must close resp.Body. Unlike Do, this does not buffer the body.
func (c *Client) DoStream(ctx context.Context, req *http.Request) (*http.Response, error) {
	if c == nil || c.httpClient == nil {
		return nil, ErrUnavailable("jasper client is not configured", nil)
	}
	req = req.WithContext(ctx)
	if req.Header.Get(proxyHeaderName) == "" {
		req.Header.Set(proxyHeaderName, proxyHeaderValue)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, Translate(err)
	}
	return resp, nil
}

// Do performs a context-aware HTTP request and reads at most maxDiagnosticBody
// bytes for diagnostics. Prefer DoStream for report/image proxies.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	resp, err := c.DoStream(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxDiagnosticBody)
	body, err := io.ReadAll(limited)
	if err != nil {
		return resp, nil, fmt.Errorf("read jasper response: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp, body, nil
}

// buildURL joins a base URL with an optional raw query string without
// re-encoding parameter values.
func buildURL(base, rawQuery string) string {
	base = strings.TrimRight(base, "/")
	if rawQuery == "" {
		return base
	}
	return base + "?" + rawQuery
}
