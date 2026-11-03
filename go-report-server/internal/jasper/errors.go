package jasper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ErrorCode classifies gateway failures for HTTP translation.
type ErrorCode string

const (
	CodeBadRequest     ErrorCode = "bad_request"
	CodeUnauthorized   ErrorCode = "unauthorized"
	CodeForbidden      ErrorCode = "forbidden"
	CodeNotFound       ErrorCode = "not_found"
	CodeTimeout        ErrorCode = "timeout"
	CodeUnavailable    ErrorCode = "unavailable"
	CodeBadGateway     ErrorCode = "bad_gateway"
	CodeInternal       ErrorCode = "internal"
)

// Error is a gateway error safe to return to clients.
// Internal details (including Java stack traces) stay in Internal only.
type Error struct {
	Code     ErrorCode
	Message  string
	Internal error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Internal
}

// HTTPStatus maps the error to an HTTP status code.
func (e *Error) HTTPStatus() int {
	if e == nil {
		return http.StatusInternalServerError
	}
	switch e.Code {
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeTimeout:
		return http.StatusGatewayTimeout
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	case CodeBadGateway:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// PublicMessage returns a client-safe message with no stack traces.
func (e *Error) PublicMessage() string {
	if e == nil {
		return "internal server error"
	}
	if e.Message != "" {
		return sanitizeClientMessage(e.Message)
	}
	switch e.Code {
	case CodeBadRequest:
		return "bad request"
	case CodeUnauthorized:
		return "unauthorized"
	case CodeForbidden:
		return "forbidden"
	case CodeNotFound:
		return "not found"
	case CodeTimeout:
		return "jasper request timed out"
	case CodeUnavailable:
		return "jasper engine unavailable"
	case CodeBadGateway:
		return "jasper proxy error"
	default:
		return "internal server error"
	}
}

func sanitizeClientMessage(msg string) string {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "exception") ||
		strings.Contains(lower, "at net.sf.jasperreports") ||
		strings.Contains(lower, "java.") ||
		strings.Contains(msg, "\tat ") {
		return "jasper engine error"
	}
	return msg
}

func newError(code ErrorCode, message string, internal error) *Error {
	return &Error{Code: code, Message: message, Internal: internal}
}

// AsError extracts a *Error from err, if present.
func AsError(err error) (*Error, bool) {
	var ge *Error
	if errors.As(err, &ge) {
		return ge, true
	}
	return nil, false
}

// Translate maps transport and validation failures to gateway errors.
func Translate(err error) *Error {
	if err == nil {
		return nil
	}
	if ge, ok := AsError(err); ok {
		return ge
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
		return newError(CodeTimeout, "jasper request timed out", err)
	}
	if errors.Is(err, context.Canceled) {
		return newError(CodeTimeout, "jasper request canceled", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return newError(CodeTimeout, "jasper request timed out", err)
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return newError(CodeUnavailable, "jasper engine unavailable", err)
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "connectex") {
		return newError(CodeUnavailable, "jasper engine unavailable", err)
	}
	return newError(CodeInternal, "unexpected jasper error", err)
}

func isTimeout(err error) bool {
	type timeout interface{ Timeout() bool }
	var t timeout
	return errors.As(err, &t) && t.Timeout()
}

// ErrBadRequest creates a 400 gateway error.
func ErrBadRequest(message string) *Error {
	return newError(CodeBadRequest, message, nil)
}

// ErrNotFound creates a 404 gateway error.
func ErrNotFound(message string) *Error {
	return newError(CodeNotFound, message, nil)
}

// ErrForbidden creates a 403 gateway error.
func ErrForbidden(message string) *Error {
	return newError(CodeForbidden, message, nil)
}

// ErrUnauthorized creates a 401 gateway error.
func ErrUnauthorized(message string) *Error {
	return newError(CodeUnauthorized, message, nil)
}

// ErrUnavailable creates a 503 gateway error.
func ErrUnavailable(message string, internal error) *Error {
	return newError(CodeUnavailable, message, internal)
}

// FormatInternal returns a log-safe summary of an internal error without
// dumping large Java stack traces into structured fields repeatedly.
func FormatInternal(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 512 {
		return s[:512] + "…"
	}
	return s
}

func wrapf(code ErrorCode, message string, err error) *Error {
	if err == nil {
		return newError(code, message, nil)
	}
	return newError(code, message, fmt.Errorf("%s: %w", message, err))
}
