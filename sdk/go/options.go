package report

import (
	"crypto/tls"
	"net/http"
	"time"
)

// Option configures a Client.
type Option func(*Client)

// WithToken sets the Bearer token.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.timeout = d
		if c.httpClient != nil {
			c.httpClient.Timeout = d
		}
	}
}

// WithTLSVerify enables or disables TLS certificate verification.
// Pass false only for local/dev environments.
func WithTLSVerify(verify bool) Option {
	return func(c *Client) {
		c.tlsVerify = verify
	}
}

// WithHTTPClient replaces the underlying HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
		if hc != nil && c.timeout > 0 && hc.Timeout == 0 {
			hc.Timeout = c.timeout
		}
	}
}

// WithHeader adds a default header on every request.
func WithHeader(key, value string) Option {
	return func(c *Client) {
		if c.headers == nil {
			c.headers = make(http.Header)
		}
		c.headers.Add(key, value)
	}
}

// WithRetries enables optional retries for safe GET requests only.
// Mutating methods (POST/PUT/DELETE/multipart) are never retried.
func WithRetries(maxRetries int, wait time.Duration) Option {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.retryWait = wait
	}
}

func applyTLS(c *Client) {
	if c.tlsVerify {
		return
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: c.timeout}
	}
	tr, _ := c.httpClient.Transport.(*http.Transport)
	if tr == nil {
		tr = http.DefaultTransport.(*http.Transport).Clone()
		c.httpClient.Transport = tr
	} else {
		tr = tr.Clone()
		c.httpClient.Transport = tr
	}
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{}
	} else {
		tr.TLSClientConfig = tr.TLSClientConfig.Clone()
	}
	tr.TLSClientConfig.InsecureSkipVerify = true
}
