package folders

import "errors"

var (
	ErrUnavailable     = errors.New("content folders unavailable")
	ErrNotFound        = errors.New("folder not found")
	ErrItemNotFound    = errors.New("content item not found")
	ErrBadRequest      = errors.New("invalid request")
	ErrConflict        = errors.New("name already exists in this folder")
	ErrCycle           = errors.New("circular parent relationship")
	ErrRootImmutable   = errors.New("root Home folder cannot be modified that way")
	ErrNotEmpty        = errors.New("folder is not empty")
	ErrConfirmRequired = errors.New("recursive delete requires confirmation")
	ErrRecursiveDenied = errors.New("recursive delete is disabled")
	ErrPermission      = errors.New("permission denied")
)
