package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrorMapper converts an HTTP status + body into a typed error.
type ErrorMapper func(status int, body []byte) error

// Client is the low-level HTTP helper used by the public SDK.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	UserAgent  string
	Headers    http.Header
	MaxRetries int
	RetryWait  time.Duration
	MapError   ErrorMapper
}

// Request describes a single HTTP call.
type Request struct {
	Method      string
	Path        string
	Query       url.Values
	Headers     http.Header
	Body        io.Reader
	ContentType string
	// SafeGET enables optional retries (only when Method is GET).
	SafeGET bool
}

// Response is a completed HTTP response with body bytes already read
// (except when Stream is true — then Body is the response body stream).
type Response struct {
	StatusCode  int
	Header      http.Header
	Body        []byte
	ContentType string
	// StreamBody is non-nil when DoStream was used; caller must Close.
	StreamBody io.ReadCloser
}

// DoJSON sends a JSON request and unmarshals a JSON response into out (optional).
func (c *Client) DoJSON(ctx context.Context, req Request, in any, out any) (*Response, error) {
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		req.Body = bytes.NewReader(b)
		req.ContentType = "application/json"
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		// Still attempt decode so callers can inspect partial payloads (e.g. /ready 503).
		if out != nil && resp != nil && len(resp.Body) > 0 {
			_ = json.Unmarshal(resp.Body, out)
		}
		return resp, err
	}
	if out != nil && len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, out); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}

// DoFormURLEncoded sends application/x-www-form-urlencoded.
func (c *Client) DoFormURLEncoded(ctx context.Context, req Request, form url.Values, out any) (*Response, error) {
	req.Body = strings.NewReader(form.Encode())
	req.ContentType = "application/x-www-form-urlencoded"
	resp, err := c.Do(ctx, req)
	if err != nil {
		if out != nil && resp != nil && len(resp.Body) > 0 {
			_ = json.Unmarshal(resp.Body, out)
		}
		return resp, err
	}
	if out != nil && len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, out); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}

// MultipartFile is a file part for multipart uploads.
type MultipartFile struct {
	FieldName string
	FileName  string
	Reader    io.Reader
}

// DoMultipart builds and sends a multipart/form-data request.
func (c *Client) DoMultipart(ctx context.Context, req Request, fields map[string]string, files []MultipartFile, multiFields map[string][]string, out any) (*Response, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	for k, vs := range multiFields {
		for _, v := range vs {
			if err := w.WriteField(k, v); err != nil {
				return nil, err
			}
		}
	}
	for _, f := range files {
		if f.Reader == nil {
			continue
		}
		part, err := w.CreateFormFile(f.FieldName, f.FileName)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(part, f.Reader); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req.Body = &buf
	req.ContentType = w.FormDataContentType()
	resp, err := c.Do(ctx, req)
	if err != nil {
		if out != nil && resp != nil && len(resp.Body) > 0 {
			_ = json.Unmarshal(resp.Body, out)
		}
		return resp, err
	}
	if out != nil && len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, out); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}

// DoBinary performs a request and returns the raw body bytes.
func (c *Client) DoBinary(ctx context.Context, req Request) (*Response, error) {
	return c.Do(ctx, req)
}

// DoStream performs a request and returns an open body stream on success.
func (c *Client) DoStream(ctx context.Context, req Request) (*Response, error) {
	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	httpResp, err := c.doWithRetry(httpReq, req.SafeGET && strings.EqualFold(req.Method, http.MethodGet))
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode >= 400 {
		defer httpResp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
		return nil, c.mapError(httpResp.StatusCode, body)
	}
	return &Response{
		StatusCode:  httpResp.StatusCode,
		Header:      httpResp.Header.Clone(),
		ContentType: httpResp.Header.Get("Content-Type"),
		StreamBody:  httpResp.Body,
	}, nil
}

// Do executes a request, reads the body, and maps error statuses.
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	httpResp, err := c.doWithRetry(httpReq, req.SafeGET && strings.EqualFold(req.Method, http.MethodGet))
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	resp := &Response{
		StatusCode:  httpResp.StatusCode,
		Header:      httpResp.Header.Clone(),
		Body:        body,
		ContentType: httpResp.Header.Get("Content-Type"),
	}
	if httpResp.StatusCode >= 400 {
		return resp, c.mapError(httpResp.StatusCode, body)
	}
	return resp, nil
}

func (c *Client) buildRequest(ctx context.Context, req Request) (*http.Request, error) {
	u, err := c.resolveURL(req.Path, req.Query)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, u, req.Body)
	if err != nil {
		return nil, err
	}
	if req.ContentType != "" {
		httpReq.Header.Set("Content-Type", req.ContentType)
	}
	if c.UserAgent != "" {
		httpReq.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}
	for k, vs := range c.Headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	for k, vs := range req.Headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	return httpReq, nil
}

func (c *Client) resolveURL(path string, query url.Values) (string, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return u.String(), nil
}

func (c *Client) doWithRetry(req *http.Request, allowRetry bool) (*http.Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	maxAttempts := 1
	if allowRetry && c.MaxRetries > 0 {
		maxAttempts = c.MaxRetries + 1
	}
	wait := c.RetryWait
	if wait <= 0 {
		wait = 200 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Replayable body for retries when GetBody is set.
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt+1 >= maxAttempts || !allowRetry {
				return nil, err
			}
			time.Sleep(wait * time.Duration(attempt+1))
			continue
		}
		if allowRetry && attempt+1 < maxAttempts && (resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504) {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			time.Sleep(wait * time.Duration(attempt+1))
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func (c *Client) mapError(status int, body []byte) error {
	if c.MapError != nil {
		return c.MapError(status, body)
	}
	return fmt.Errorf("HTTP %d: %s", status, strings.TrimSpace(string(body)))
}

// EncodeQuery builds url.Values from a map supporting scalars and slices.
func EncodeQuery(params map[string]any) url.Values {
	q := make(url.Values)
	for k, v := range params {
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t != "" {
				q.Add(k, t)
			}
		case []string:
			for _, s := range t {
				q.Add(k, s)
			}
		case []any:
			for _, item := range t {
				q.Add(k, fmt.Sprint(item))
			}
		case bool:
			q.Add(k, strconv.FormatBool(t))
		case int:
			q.Add(k, strconv.Itoa(t))
		case int64:
			q.Add(k, strconv.FormatInt(t, 10))
		case float64:
			q.Add(k, strconv.FormatFloat(t, 'f', -1, 64))
		case fmt.Stringer:
			q.Add(k, t.String())
		default:
			q.Add(k, fmt.Sprint(t))
		}
	}
	return q
}
