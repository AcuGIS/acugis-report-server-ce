package license

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultServerURL is the production AcuGIS licensing authority.
	DefaultServerURL = "https://license.acugis.com"

	maxResponseBody  = 64 << 10
	connectTimeout   = 10 * time.Second
	requestTimeout   = 30 * time.Second
	handshakeTimeout = 10 * time.Second
	responseHeaderTO = 20 * time.Second
)

// APIError is a licensing HTTP failure. Message is a fixed local string.
// The raw response body is not retained.
type APIError struct {
	StatusCode    int
	Code          string
	Message       string
	Transport     bool
	Authoritative bool
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// Result is a successful activate, refresh, or trial response. License is the
// ACL1 token and must be verified locally before it is stored.
type Result struct {
	License        string
	LicenseID      string
	ProductCode    string
	ExpiresAt      string
	InstallationID string
	LicenseType    string
	IssuedAt       string
}

// TrialStatus is the license service eligibility snapshot.
type TrialStatus struct {
	ProductCode    string
	InstallationID string
	TrialEnabled   bool
	TrialDays      int
	Eligible       bool
	Used           bool
	Status         string
	LicenseType    string
	IssuedAt       string
	ExpiresAt      string
}

type tokenRequest struct {
	ActivationKey  string `json:"activation_key,omitempty"`
	License        string `json:"license,omitempty"`
	InstallationID string `json:"installation_id"`
	ProductCode    string `json:"product_code"`
	ProductVersion string `json:"product_version,omitempty"`
}

type tokenResponse struct {
	License        string `json:"license"`
	LicenseID      string `json:"license_id"`
	ProductCode    string `json:"product_code"`
	ExpiresAt      string `json:"expires_at"`
	InstallationID string `json:"installation_id"`
	LicenseType    string `json:"license_type"`
	IssuedAt       string `json:"issued_at"`
}

type trialStatusResponse struct {
	ProductCode    string `json:"product_code"`
	InstallationID string `json:"installation_id"`
	TrialEnabled   bool   `json:"trial_enabled"`
	TrialDays      int    `json:"trial_days"`
	Eligible       bool   `json:"eligible"`
	Used           bool   `json:"used"`
	Status         string `json:"status"`
	LicenseType    string `json:"license_type"`
	IssuedAt       string `json:"issued_at"`
	ExpiresAt      string `json:"expires_at"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Client calls the public activation and refresh API.
type Client struct {
	ServerURL           string
	ActivateEndpoint    string
	RefreshEndpoint     string
	TrialEndpoint       string
	TrialStatusEndpoint string
	HTTP                *http.Client
	// ProductCode is sent as product_code. Empty means Report Server Professional.
	ProductCode string
}

func (c *Client) client() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTPClient()
}

func defaultHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second}
	return &http.Client{
		Timeout: requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   handshakeTimeout,
			ResponseHeaderTimeout: responseHeaderTO,
			ForceAttemptHTTP2:     true,
			// Certificate verification uses the default TLS config.
		},
	}
}

// Activate posts an activation key. The key is not logged by this client.
func (c *Client) Activate(ctx context.Context, activationKey, installationID, productVersion string) (*Result, error) {
	endpoint, err := c.endpoint(c.ActivateEndpoint, "/v1/activate")
	if err != nil {
		return nil, err
	}
	activationKey = strings.TrimSpace(activationKey)
	installationID = strings.ToLower(strings.TrimSpace(installationID))
	if activationKey == "" || !ValidInstallationID(installationID) {
		return nil, &APIError{Code: "invalid_request", Message: "activation key and installation ID are required", Authoritative: true}
	}
	return c.post(ctx, endpoint, tokenRequest{
		ActivationKey:  activationKey,
		InstallationID: installationID,
		ProductCode:    c.productCode(),
		ProductVersion: strings.TrimSpace(productVersion),
	})
}

// Refresh posts the current ACL1 as refresh proof.
func (c *Client) Refresh(ctx context.Context, token, installationID string) (*Result, error) {
	endpoint, err := c.endpoint(c.RefreshEndpoint, "/v1/refresh")
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	installationID = strings.ToLower(strings.TrimSpace(installationID))
	if token == "" || !ValidInstallationID(installationID) {
		return nil, &APIError{Code: "invalid_request", Message: "license and installation ID are required", Authoritative: true}
	}
	return c.post(ctx, endpoint, tokenRequest{
		License:        token,
		InstallationID: installationID,
		ProductCode:    c.productCode(),
	})
}

func (c *Client) productCode() string {
	if c != nil {
		if code := strings.TrimSpace(c.ProductCode); code != "" {
			return code
		}
	}
	return ProductReportServerProfessional
}

func (c *Client) endpoint(explicit, path string) (string, error) {
	if u := strings.TrimSpace(explicit); u != "" {
		return u, nil
	}
	base := DefaultServerURL
	if c != nil && strings.TrimSpace(c.ServerURL) != "" {
		base = strings.TrimSpace(c.ServerURL)
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") {
		return "", &APIError{Code: "invalid_request", Message: "license server URL must be https", Authoritative: true}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (c *Client) post(ctx context.Context, endpoint string, body tokenRequest) (*Result, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AcuGIS-Report-Server/1")
	resp, err := c.client().Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &APIError{Code: "timeout", Message: trialServiceUnavailable, Transport: true}
		}
		return nil, &APIError{Code: "transport", Message: trialServiceUnavailable, Transport: true}
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return nil, &APIError{Code: "transport", Message: trialServiceUnavailable, Transport: true}
	}
	if len(payload) > maxResponseBody {
		return nil, &APIError{Code: "invalid", Message: "licensing response was too large", Authoritative: true}
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, &APIError{Code: "transport", Message: "licensing server returned a redirect", Transport: true}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, apiErrorFrom(resp.StatusCode, payload)
	}
	var out tokenResponse
	if err := json.Unmarshal(payload, &out); err != nil || strings.TrimSpace(out.License) == "" {
		return nil, &APIError{Code: "invalid", Message: "licensing response was malformed", Authoritative: true}
	}
	return &Result{
		License:        strings.TrimSpace(out.License),
		LicenseID:      strings.TrimSpace(out.LicenseID),
		ProductCode:    strings.TrimSpace(out.ProductCode),
		ExpiresAt:      strings.TrimSpace(out.ExpiresAt),
		InstallationID: strings.TrimSpace(out.InstallationID),
		LicenseType:    strings.TrimSpace(out.LicenseType),
		IssuedAt:       strings.TrimSpace(out.IssuedAt),
	}, nil
}

// StartTrial asks the license service for a 14-day ACL1. Duration and
// expiration are not sent. The returned token must still be verified locally.
func (c *Client) StartTrial(ctx context.Context, installationID string) (*Result, error) {
	endpoint, err := c.endpoint(c.TrialEndpoint, "/v1/trial")
	if err != nil {
		return nil, err
	}
	installationID = strings.ToLower(strings.TrimSpace(installationID))
	if !ValidInstallationID(installationID) {
		return nil, &APIError{Code: "installation_id_invalid", Message: "The Report Server installation ID is invalid.", Authoritative: true}
	}
	return c.post(ctx, endpoint, tokenRequest{
		InstallationID: installationID,
		ProductCode:    c.productCode(),
	})
}

// TrialStatus asks whether this installation may start a trial.
// A missing configuration returns ok=false and does not call the default host.
// That keeps tests that override only activate/refresh from contacting production.
func (c *Client) TrialStatus(ctx context.Context, installationID string) (*TrialStatus, bool, error) {
	endpoint, ok, err := c.trialStatusEndpoint()
	if err != nil || !ok {
		return nil, ok, err
	}
	installationID = strings.ToLower(strings.TrimSpace(installationID))
	if !ValidInstallationID(installationID) {
		return nil, true, &APIError{Code: "installation_id_invalid", Message: "The Report Server installation ID is invalid.", Authoritative: true}
	}
	raw, err := json.Marshal(tokenRequest{
		InstallationID: installationID,
		ProductCode:    c.productCode(),
	})
	if err != nil {
		return nil, true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, true, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AcuGIS-Report-Server/1")
	resp, err := c.client().Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, true, &APIError{Code: "timeout", Message: trialServiceUnavailable, Transport: true}
		}
		return nil, true, &APIError{Code: "transport", Message: trialServiceUnavailable, Transport: true}
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return nil, true, &APIError{Code: "transport", Message: trialServiceUnavailable, Transport: true}
	}
	if len(payload) > maxResponseBody {
		return nil, true, &APIError{Code: "invalid", Message: "licensing response was too large", Authoritative: true}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, true, apiErrorFrom(resp.StatusCode, payload)
	}
	var out trialStatusResponse
	if err := json.Unmarshal(payload, &out); err != nil || strings.TrimSpace(out.ProductCode) == "" {
		return nil, true, &APIError{Code: "invalid", Message: "licensing response was malformed", Authoritative: true}
	}
	return &TrialStatus{
		ProductCode:    strings.TrimSpace(out.ProductCode),
		InstallationID: strings.TrimSpace(out.InstallationID),
		TrialEnabled:   out.TrialEnabled,
		TrialDays:      out.TrialDays,
		Eligible:       out.Eligible,
		Used:           out.Used,
		Status:         strings.TrimSpace(out.Status),
		LicenseType:    strings.TrimSpace(out.LicenseType),
		IssuedAt:       strings.TrimSpace(out.IssuedAt),
		ExpiresAt:      strings.TrimSpace(out.ExpiresAt),
	}, true, nil
}

func (c *Client) trialStatusEndpoint() (string, bool, error) {
	if c != nil && strings.TrimSpace(c.TrialStatusEndpoint) != "" {
		return strings.TrimSpace(c.TrialStatusEndpoint), true, nil
	}
	if c != nil && strings.TrimSpace(c.ServerURL) != "" {
		u, err := c.endpoint("", "/v1/trial/status")
		return u, err == nil, err
	}
	if c == nil || (strings.TrimSpace(c.ActivateEndpoint) == "" && strings.TrimSpace(c.RefreshEndpoint) == "" && strings.TrimSpace(c.TrialEndpoint) == "") {
		u, err := c.endpoint("", "/v1/trial/status")
		if err != nil {
			return "", false, err
		}
		return u, true, nil
	}
	return "", false, nil
}

func apiErrorFrom(status int, payload []byte) error {
	var body errorResponse
	_ = json.Unmarshal(payload, &body)
	code := strings.TrimSpace(body.Error)
	if code == "" {
		if status >= 500 {
			code = "server_error"
		} else {
			code = "invalid"
		}
	}
	api := &APIError{
		StatusCode: status,
		Code:       code,
		Message:    safeAPIMessage(code, status),
	}
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusNotFound:
		api.Authoritative = true
	default:
		if status >= 500 || status == http.StatusTooManyRequests {
			api.Transport = true
		} else {
			api.Authoritative = true
		}
	}
	return api
}

func safeAPIMessage(code string, status int) string {
	switch code {
	case "invalid_activation_key":
		return "The activation key was not accepted."
	case "license_inactive":
		return "The license is not active."
	case "license_expired":
		return "The license has expired."
	case "refresh_denied":
		return "Refresh was denied."
	case "activation_limit_reached":
		return "This license is already activated on another installation."
	case "invalid_request":
		return "The licensing request was rejected."
	case "trial_already_used":
		return "The free trial has already been used on this installation."
	case "trial_not_available":
		return "A free trial is not available for this product."
	case "unknown_product":
		return "The licensing service could not complete the request."
	case "installation_id_invalid":
		return "The Report Server installation ID is invalid."
	case "server_error", "internal_error":
		return "The licensing server returned an error."
	default:
		if status >= 500 {
			return "The licensing server returned an error."
		}
		if status == http.StatusTooManyRequests {
			return "Too many trial requests. Please try again later."
		}
		return "The licensing request failed."
	}
}

const trialServiceUnavailable = "The licensing service is temporarily unavailable. Please try again later."

// IsTransport reports a failure that must not discard a locally valid token.
func IsTransport(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Transport
}

// IsAuthoritative reports a central decision such as suspended, revoked, or rejected.
func IsAuthoritative(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Authoritative
}

// FormatRefreshFailure returns a log/UI-safe reason.
func FormatRefreshFailure(err error) string {
	var api *APIError
	if errors.As(err, &api) {
		return api.Message
	}
	if err == nil {
		return ""
	}
	return "refresh failed"
}
