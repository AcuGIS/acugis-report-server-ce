package report

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// EnvURL and EnvToken are the environment variables used by NewClientFromEnv.
const (
	EnvURL   = "ACUGIS_REPORT_URL"
	EnvToken = "ACUGIS_REPORT_TOKEN"
)

// Client is the AcuGIS Report Server SDK entry point.
type Client struct {
	baseURL    string
	token      string
	timeout    time.Duration
	tlsVerify  bool
	httpClient *http.Client
	headers    http.Header
	maxRetries int
	retryWait  time.Duration

	tr *transport.Client

	Auth         *AuthService
	Reports      *ReportsService
	Folders      *FoldersService
	Resources    *ResourcesService
	Datasources  *DatasourcesService
	MapReports   *MapReportsService
	EmbeddedMaps *EmbeddedMapsService
	Health       *HealthService

	// Reserved stubs (future APIs).
	Users      *ReservedService
	Groups     *ReservedService
	Schedules  *ReservedService
	Jobs       *ReservedService
	Recipients *ReservedService
}

// NewClient creates a client for the given base URL (e.g. https://reports.example.com).
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	c := &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		timeout:   60 * time.Second,
		tlsVerify: true,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: c.timeout}
	} else if c.timeout > 0 && c.httpClient.Timeout == 0 {
		c.httpClient.Timeout = c.timeout
	}
	applyTLS(c)

	c.tr = &transport.Client{
		BaseURL:    c.baseURL,
		Token:      c.token,
		HTTPClient: c.httpClient,
		UserAgent:  UserAgent,
		Headers:    c.headers.Clone(),
		MaxRetries: c.maxRetries,
		RetryWait:  c.retryWait,
		MapError:   mapHTTPError,
	}

	c.Auth = &AuthService{c: c}
	c.Reports = &ReportsService{c: c}
	c.Folders = &FoldersService{c: c}
	c.Resources = &ResourcesService{c: c}
	c.Datasources = &DatasourcesService{c: c}
	c.MapReports = &MapReportsService{c: c}
	c.EmbeddedMaps = &EmbeddedMapsService{c: c}
	c.Health = &HealthService{c: c}

	c.Users = &ReservedService{name: "Users"}
	c.Groups = &ReservedService{name: "Groups"}
	c.Schedules = &ReservedService{name: "Schedules"}
	c.Jobs = &ReservedService{name: "Jobs"}
	c.Recipients = &ReservedService{name: "Recipients"}

	return c, nil
}

// NewClientFromEnv builds a client from ACUGIS_REPORT_URL and ACUGIS_REPORT_TOKEN.
func NewClientFromEnv(opts ...Option) (*Client, error) {
	baseURL := strings.TrimSpace(os.Getenv(EnvURL))
	token := strings.TrimSpace(os.Getenv(EnvToken))
	if baseURL == "" {
		return nil, fmt.Errorf("%s is required", EnvURL)
	}
	all := append([]Option{WithToken(token)}, opts...)
	return NewClient(baseURL, all...)
}

// BaseURL returns the configured server base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// Token returns the configured bearer token (may be empty).
func (c *Client) Token() string { return c.token }

// Transport exposes the low-level transport client for advanced use.
func (c *Client) Transport() *transport.Client { return c.tr }

func mapHTTPError(status int, body []byte) error {
	var parsed ErrorBody
	if len(body) > 0 && json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		return newHTTPError(status, body, &parsed)
	}
	return newHTTPError(status, body, nil)
}

func decodeError(err error, body []byte) error {
	if err == nil {
		return nil
	}
	// Prefer already-mapped typed errors from transport.
	switch err.(type) {
	case *AuthenticationError, *PermissionDeniedError, *ValidationError,
		*NotFoundError, *ConflictError, *ServerError, *NotImplementedError,
		*NetworkError, *TimeoutError, *InvalidResponseError:
		return err
	}
	if strings.HasPrefix(err.Error(), "decode response:") {
		return &InvalidResponseError{
			baseError: baseError{message: err.Error(), body: append([]byte(nil), body...)},
			err:       err,
		}
	}
	return wrapTransportError(err)
}

func decodeAs(body []byte, out any) error {
	return json.Unmarshal(body, out)
}
