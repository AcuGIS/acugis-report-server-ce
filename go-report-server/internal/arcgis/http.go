package arcgis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBytes caps one ArcGIS REST body. Designer uses the same cap.
const maxResponseBytes int64 = 8 << 20

// NormalizeServiceURL accepts a FeatureServer or MapServer URL, including a
// layer URL such as .../FeatureServer/0. The layer id and query string are
// removed. Credentials must not be embedded in the URL.
func NormalizeServiceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("service URL is required")
	}
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" {
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
		default:
			return "", fmt.Errorf("service URL must use http or https")
		}
	}
	clean := CleanServiceURL(raw)
	if err := validateServiceURL(clean); err != nil {
		return "", err
	}
	lower := strings.ToLower(clean)
	if !strings.Contains(lower, "/featureserver") && !strings.Contains(lower, "/mapserver") {
		return "", fmt.Errorf("service URL must be an ArcGIS FeatureServer or MapServer URL")
	}
	return clean, nil
}

// TestConnection checks an ArcGIS Feature Server the same way report fill does:
// optional token, then the service layer list. Anonymous services omit username
// and password. Errors are safe to show in the datasource UI.
func TestConnection(ctx context.Context, cred Credentials) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	serviceURL, err := NormalizeServiceURL(cred.URL)
	if err != nil {
		return "", err
	}
	layers, err := browseService(ctx, serviceURL, cred.Username, cred.Password)
	if err != nil {
		return "", friendlyTestError(err)
	}
	n := 0
	for _, layer := range layers {
		if strings.TrimSpace(layer.Name) != "" {
			n++
		}
	}
	if n == 0 {
		return "", fmt.Errorf("the service responded but published no layers")
	}
	return fmt.Sprintf("Connected (%d layer(s))", n), nil
}

func friendlyTestError(err error) error {
	if err == nil {
		return nil
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("could not reach the ArcGIS service")
	}
	msg := strings.TrimSpace(err.Error())
	msg = strings.TrimPrefix(msg, "browse layers failed: ")
	low := strings.ToLower(msg)
	if msg == "" || strings.Contains(low, "invalid character") || strings.Contains(low, "unexpected eof") {
		return fmt.Errorf("ArcGIS service returned an unreadable response")
	}
	if strings.Contains(low, "token generation failed") {
		return fmt.Errorf("ArcGIS token request failed")
	}
	return fmt.Errorf("%s", msg)
}

func validateServiceURL(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		return fmt.Errorf("service URL is required")
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("invalid service URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("service URL must use http or https")
	}
	if u.User != nil {
		return fmt.Errorf("service URL must not include credentials")
	}
	return nil
}

func httpClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after too many redirects")
			}
			if req.URL == nil {
				return fmt.Errorf("invalid service URL")
			}
			return validateServiceURL(req.URL.String())
		},
	}
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = maxResponseBytes
	}
	buf, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > max {
		return nil, fmt.Errorf("response too large")
	}
	return buf, nil
}

func tokenEndpointFromServiceURL(serviceURL string) string {
	u, err := url.Parse(serviceURL)
	if err != nil {
		return "https://www.arcgis.com/sharing/rest/generateToken"
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if strings.HasSuffix(host, ".arcgis.com") || host == "arcgis.com" {
		return "https://www.arcgis.com/sharing/rest/generateToken"
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/arcgis/tokens/", scheme, u.Host)
}

func urlWithToken(rawURL, token string) string {
	if strings.TrimSpace(token) == "" {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "token=" + url.QueryEscape(token)
}

func getToken(ctx context.Context, serviceURL, username, password string) (string, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return "", nil
	}
	form := url.Values{}
	form.Set("f", "json")
	form.Set("username", username)
	form.Set("password", password)
	// Same referer Designer uses. The token is passed as a query parameter;
	// credentials are not stored in JRXML.
	form.Set("referer", "https://acugis-report-designer.local")
	form.Set("expiration", "60")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpointFromServiceURL(serviceURL), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient(30 * time.Second).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := readLimited(resp.Body, maxResponseBytes)
	if err != nil {
		return "", err
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return "", err
	}
	if errObj, ok := body["error"].(map[string]any); ok {
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "token generation failed"
		}
		return "", fmt.Errorf("%s", msg)
	}
	token, _ := body["token"].(string)
	if token == "" {
		return "", fmt.Errorf("token generation failed")
	}
	return token, nil
}

func jsonRequest(ctx context.Context, rawURL, serviceURL string, token *string, username, password string) (map[string]any, error) {
	callURL := rawURL
	if token != nil && strings.TrimSpace(*token) != "" {
		callURL = urlWithToken(rawURL, *token)
	}
	client := httpClient(90 * time.Second)
	var body map[string]any
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, callURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "AcuGIS-Report-Server/1.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				continue
			}
			return nil, err
		}
		data, readErr := readLimited(resp.Body, maxResponseBytes)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < 2 && !strings.Contains(readErr.Error(), "too large") {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				continue
			}
			return nil, readErr
		}
		decodeErr := json.Unmarshal(data, &body)
		if decodeErr != nil {
			lastErr = decodeErr
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				continue
			}
			return nil, decodeErr
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if errObj, ok := body["error"].(map[string]any); ok {
		code := int64(0)
		switch v := errObj["code"].(type) {
		case float64:
			code = int64(v)
		case int64:
			code = v
		}
		if (code == 498 || code == 499) && username != "" && password != "" && token != nil {
			newToken, tokErr := getToken(ctx, serviceURL, username, password)
			if tokErr == nil && newToken != "" {
				*token = newToken
				return jsonRequest(ctx, rawURL, serviceURL, token, "", "")
			}
		}
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "arcgis request failed"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return body, nil
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		var i int
		fmt.Sscanf(n, "%d", &i)
		return i
	default:
		return 0
	}
}
