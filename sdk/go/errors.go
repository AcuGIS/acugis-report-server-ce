package report

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// APIError is the common interface for typed SDK errors.
type APIError interface {
	error
	StatusCode() int
	Body() []byte
	Message() string
}

type baseError struct {
	status  int
	message string
	body    []byte
	code    string
	hint    string
}

func (e *baseError) Error() string {
	if e.status > 0 {
		return fmt.Sprintf("%s (HTTP %d)", e.message, e.status)
	}
	return e.message
}

func (e *baseError) StatusCode() int { return e.status }
func (e *baseError) Body() []byte    { return e.body }
func (e *baseError) Message() string { return e.message }
func (e *baseError) Code() string    { return e.code }
func (e *baseError) Hint() string    { return e.hint }

// AuthenticationError is returned for HTTP 401.
type AuthenticationError struct{ baseError }

// PermissionDeniedError is returned for HTTP 403.
type PermissionDeniedError struct{ baseError }

// ValidationError is returned for HTTP 400 / 422.
type ValidationError struct{ baseError }

// NotFoundError is returned for HTTP 404.
type NotFoundError struct{ baseError }

// ConflictError is returned for HTTP 409.
type ConflictError struct{ baseError }

// ServerError is returned for HTTP 5xx.
type ServerError struct{ baseError }

// NetworkError wraps transport failures.
type NetworkError struct {
	baseError
	err error
}

func (e *NetworkError) Unwrap() error { return e.err }

// TimeoutError wraps deadline / timeout failures.
type TimeoutError struct {
	baseError
	err error
}

func (e *TimeoutError) Unwrap() error { return e.err }

// InvalidResponseError is returned when a response cannot be decoded.
type InvalidResponseError struct {
	baseError
	err error
}

func (e *InvalidResponseError) Unwrap() error { return e.err }

// NotImplementedError is returned for reserved / unimplemented APIs.
type NotImplementedError struct{ baseError }

// ErrNotImplemented is the sentinel returned by reserved service stubs.
var ErrNotImplemented error = &NotImplementedError{baseError{
	status:  http.StatusNotImplemented,
	message: "not implemented",
}}

// ErrorBody is the JSON error envelope from the API.
type ErrorBody struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Hint    string `json:"hint,omitempty"`
	Service string `json:"service,omitempty"`
}

func newHTTPError(status int, body []byte, parsed *ErrorBody) error {
	msg := strings.TrimSpace(string(body))
	code, hint := "", ""
	if parsed != nil {
		if parsed.Error != "" {
			msg = parsed.Error
		}
		code = parsed.Code
		hint = parsed.Hint
	}
	if msg == "" {
		msg = http.StatusText(status)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
	}
	base := baseError{status: status, message: msg, body: append([]byte(nil), body...), code: code, hint: hint}
	switch {
	case status == http.StatusUnauthorized:
		return &AuthenticationError{base}
	case status == http.StatusForbidden:
		return &PermissionDeniedError{base}
	case status == http.StatusNotFound:
		return &NotFoundError{base}
	case status == http.StatusConflict:
		return &ConflictError{base}
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return &ValidationError{base}
	case status >= 500:
		return &ServerError{base}
	case status == http.StatusNotImplemented:
		return &NotImplementedError{base}
	default:
		if status >= 400 {
			return &ValidationError{base}
		}
		return &ServerError{base}
	}
}

func wrapTransportError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*NotImplementedError); ok {
		return err
	}
	msg := err.Error()
	if isTimeout(err) {
		return &TimeoutError{baseError: baseError{message: "request timed out"}, err: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return &NetworkError{baseError: baseError{message: msg}, err: err}
	}
	return &NetworkError{baseError: baseError{message: msg}, err: err}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded")
}
