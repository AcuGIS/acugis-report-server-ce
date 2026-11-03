package auth

import "errors"

// Sentinel outcomes for session resolution. Handlers map these to HTTP status
// codes without exposing SQL or token material.
var (
	ErrNoSession            = errors.New("authentication: no session")
	ErrInvalidSession       = errors.New("authentication: invalid session")
	ErrExpiredSession       = errors.New("authentication: expired session")
	ErrDisabledUser         = errors.New("authentication: disabled user")
	ErrEmptyToken           = errors.New("authentication: empty token")
	ErrDatabase             = errors.New("authentication: database unavailable")
	ErrUnauthenticated      = errors.New("authentication: required")
	ErrInvalidCredentials   = errors.New("authentication: invalid credentials")
	ErrSessionWriteDisabled = errors.New("authentication: session writes unavailable")
)

// IsAuthError reports whether err is a known authentication outcome.
func IsAuthError(err error) bool {
	return errors.Is(err, ErrNoSession) ||
		errors.Is(err, ErrInvalidSession) ||
		errors.Is(err, ErrExpiredSession) ||
		errors.Is(err, ErrDisabledUser) ||
		errors.Is(err, ErrEmptyToken) ||
		errors.Is(err, ErrDatabase) ||
		errors.Is(err, ErrUnauthenticated) ||
		errors.Is(err, ErrInvalidCredentials) ||
		errors.Is(err, ErrSessionWriteDisabled) ||
		errors.Is(err, ErrTokenInvalid) ||
		errors.Is(err, ErrTokenUnsupported) ||
		errors.Is(err, ErrTokenNotFound)
}
