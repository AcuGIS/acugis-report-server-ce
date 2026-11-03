package reports

import "errors"

var (
	ErrNotFound   = errors.New("report not found")
	ErrInvalidKey = errors.New("invalid report key")
)
