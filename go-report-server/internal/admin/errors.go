package admin

import "errors"

var (
	// ErrUnavailable indicates the admin repository pool is not configured.
	ErrUnavailable = errors.New("admin repository unavailable")
	// ErrInvalidInput indicates missing or invalid arguments.
	ErrInvalidInput = errors.New("invalid admin input")
	// ErrSelfDelete blocks deleting the current user account.
	ErrSelfDelete = errors.New("cannot delete own account")
	// ErrProtectedGroup blocks deleting built-in Administrators/Public groups.
	ErrProtectedGroup = errors.New("protected group cannot be deleted")
)
