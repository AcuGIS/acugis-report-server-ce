package auth

import (
	"context"
)

type ctxKey int

const userContextKey ctxKey = 1

// WithUser attaches an authenticated (or synthetic) user to the request context.
// Do not store raw session tokens in context.
func WithUser(ctx context.Context, user *User) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext returns the user attached by middleware, if any.
func UserFromContext(ctx context.Context) (*User, bool) {
	if ctx == nil {
		return nil, false
	}
	u, ok := ctx.Value(userContextKey).(*User)
	if !ok || u == nil {
		return nil, false
	}
	return u, true
}

// IsAuthenticated reports whether the context has a non-anonymous user.
// Synthetic local-dev admin (auth disabled) counts as authenticated for
// authorization bypass purposes.
func IsAuthenticated(ctx context.Context) bool {
	u, ok := UserFromContext(ctx)
	if !ok || u == nil {
		return false
	}
	if u.Username == "local-dev" && u.IsAdmin {
		return true
	}
	return u.ID > 0
}

// FromContext returns the context user or a nil-safe anonymous placeholder.
// Prefer UserFromContext for new code.
func FromContext(ctx context.Context) *User {
	if u, ok := UserFromContext(ctx); ok {
		return u
	}
	return &User{}
}
