package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// SessionResolver abstracts repository lookups for tests.
type SessionResolver interface {
	Configured() bool
	ResolveUserFromSession(ctx context.Context, tokenHash string) (*User, error)
}

// SessionManager abstracts session create/destroy and password checks.
type SessionManager interface {
	AuthenticatePassword(ctx context.Context, username, password string) error
	CreateSession(ctx context.Context, tokenHash, username string, expiresAt time.Time) error
	DeleteSession(ctx context.Context, tokenHash string) error
}

// Service validates PHP-compatible sessions from the jrs_session cookie.
type Service struct {
	repo       SessionResolver
	cookieName string
	disabled   bool
	logger     *slog.Logger
	// embedResolver optionally authenticates emb_ Bearer tokens (required for embedding).
	embedResolver EmbedTokenResolver
	patPrefixes   []string
}

// EmbedTokenResolver validates short-lived embed tokens (emb_…).
type EmbedTokenResolver interface {
	ResolveEmbedToken(ctx context.Context, raw string) (*User, error)
}

// SetEmbedTokenResolver wires embed token authentication into Bearer handling.
func (s *Service) SetEmbedTokenResolver(r EmbedTokenResolver) {
	if s == nil {
		return
	}
	s.embedResolver = r
}

// Options configures the authentication service.
type Options struct {
	CookieName   string
	AuthDisabled bool
	Logger       *slog.Logger
	// PATPrefixes overrides the accepted Bearer prefixes. Empty uses jrs_ only.
	PATPrefixes []string
}

// NewService constructs an authentication service.
func NewService(repo SessionResolver, opts Options) *Service {
	if opts.CookieName == "" {
		opts.CookieName = "jrs_session"
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	prefixes := append([]string{}, opts.PATPrefixes...)
	return &Service{
		repo:        repo,
		cookieName:  opts.CookieName,
		disabled:    opts.AuthDisabled,
		logger:      opts.Logger,
		patPrefixes: prefixes,
	}
}

func (s *Service) acceptedPATPrefixes() []string {
	if s != nil && len(s.patPrefixes) > 0 {
		return s.patPrefixes
	}
	return []string{PersonalAccessTokenPrefix}
}

// AuthDisabled reports whether JRS_AUTH_DISABLED is active.
func (s *Service) AuthDisabled() bool {
	return s != nil && s.disabled
}

// RepositoryConfigured reports whether session lookups can hit the database.
func (s *Service) RepositoryConfigured() bool {
	return s != nil && s.repo != nil && s.repo.Configured()
}

// CookieName returns the session cookie name.
func (s *Service) CookieName() string {
	if s == nil || s.cookieName == "" {
		return "jrs_session"
	}
	return s.cookieName
}

// ResolveResult is the outcome of attempting to authenticate a request.
type ResolveResult struct {
	User   *User
	Status ResolveStatus
	Err    error
}

// ResolveStatus classifies authentication outcomes.
type ResolveStatus string

const (
	StatusAuthenticated ResolveStatus = "authenticated"
	StatusAnonymous     ResolveStatus = "anonymous"
	StatusInvalid       ResolveStatus = "invalid"
	StatusExpired       ResolveStatus = "expired"
	StatusDisabledUser  ResolveStatus = "disabled_user"
	StatusAuthDisabled  ResolveStatus = "auth_disabled"
	StatusDatabaseError ResolveStatus = "database_error"
)

// AuthenticateRequest resolves the caller from Bearer PAT or jrs_session cookie.
//
// Order (Report Server API-token design):
//  1. Authorization: Bearer <token> when present → PAT only (no cookie fallback)
//  2. Else jrs_session cookie → session auth
//  3. Else anonymous
//
// Missing / invalid credentials yield anonymous statuses without rejecting the
// request (optional middleware). Database failures are never collapsed into
// anonymous access when a credential was supplied.
func (s *Service) AuthenticateRequest(ctx context.Context, r *http.Request) ResolveResult {
	if s == nil {
		return ResolveResult{Status: StatusDatabaseError, Err: ErrDatabase}
	}
	if s.disabled {
		return ResolveResult{User: LocalDevAdmin(), Status: StatusAuthDisabled}
	}

	rawBearer, bearerPresent := ParseBearerToken(r.Header.Get("Authorization"))
	if bearerPresent {
		return s.authenticateBearer(ctx, rawBearer, ClientIPFromRequest(r))
	}

	// Embed cookie set by /embed/* after validating emb_ token (required for embedding).
	if emb := readSessionCookie(r, "jrs_embed_token"); emb != "" && s.embedResolver != nil {
		return s.authenticateBearer(ctx, emb, ClientIPFromRequest(r))
	}

	raw := readSessionCookie(r, s.CookieName())
	if raw == "" {
		return ResolveResult{Status: StatusAnonymous, Err: ErrNoSession}
	}

	if !s.RepositoryConfigured() {
		return ResolveResult{Status: StatusDatabaseError, Err: fmt.Errorf("%w: repository not configured", ErrDatabase)}
	}

	hash, err := HashSessionToken(raw)
	if err != nil {
		return ResolveResult{Status: StatusInvalid, Err: ErrInvalidSession}
	}

	user, err := s.repo.ResolveUserFromSession(ctx, hash)
	if err == nil && user != nil && user.ID > 0 {
		s.logger.Debug("session authenticated",
			"user_id", user.ID,
			"username", user.Username,
			"token_hash_prefix", TokenHashPrefix(hash),
		)
		return ResolveResult{User: user, Status: StatusAuthenticated}
	}

	switch {
	case errors.Is(err, ErrExpiredSession):
		return ResolveResult{Status: StatusExpired, Err: ErrExpiredSession}
	case errors.Is(err, ErrDisabledUser):
		return ResolveResult{Status: StatusDisabledUser, Err: ErrDisabledUser}
	case errors.Is(err, ErrDatabase):
		return ResolveResult{Status: StatusDatabaseError, Err: err}
	case errors.Is(err, ErrInvalidSession), errors.Is(err, ErrEmptyToken), err != nil:
		return ResolveResult{Status: StatusInvalid, Err: ErrInvalidSession}
	default:
		return ResolveResult{Status: StatusInvalid, Err: ErrInvalidSession}
	}
}

func (s *Service) authenticateBearer(ctx context.Context, rawToken, clientIP string) ResolveResult {
	if rawToken == "" {
		return ResolveResult{Status: StatusInvalid, Err: ErrTokenInvalid}
	}

	// Short-lived embed tokens (emb_) — required for /embed/* browser embedding.
	if s.embedResolver != nil && strings.HasPrefix(rawToken, "emb_") {
		user, err := s.embedResolver.ResolveEmbedToken(ctx, rawToken)
		if err == nil && user != nil && (user.ID > 0 || (user.Username == "local-dev" && user.IsAdmin)) {
			s.logger.Debug("embed token authenticated",
				"user_id", user.ID,
				"username", user.Username,
			)
			return ResolveResult{User: user, Status: StatusAuthenticated}
		}
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "expired") {
			return ResolveResult{Status: StatusExpired, Err: err}
		}
		return ResolveResult{Status: StatusInvalid, Err: ErrTokenInvalid}
	}

	if !s.RepositoryConfigured() {
		return ResolveResult{Status: StatusDatabaseError, Err: fmt.Errorf("%w: repository not configured", ErrDatabase)}
	}
	user, err := s.ValidatePersonalAccessToken(ctx, rawToken, clientIP)
	if err == nil && user != nil && user.ID > 0 {
		s.logger.Debug("bearer token authenticated",
			"user_id", user.ID,
			"username", user.Username,
		)
		return ResolveResult{User: user, Status: StatusAuthenticated}
	}
	switch {
	case errors.Is(err, ErrTokenUnsupported):
		return ResolveResult{Status: StatusDatabaseError, Err: err}
	case errors.Is(err, ErrDatabase):
		return ResolveResult{Status: StatusDatabaseError, Err: err}
	default:
		return ResolveResult{Status: StatusInvalid, Err: ErrTokenInvalid}
	}
}

// CheckToken validates a raw session token (for jrs-auth-check). Never logs the token.
func (s *Service) CheckToken(ctx context.Context, raw string) ResolveResult {
	if s == nil {
		return ResolveResult{Status: StatusDatabaseError, Err: ErrDatabase}
	}
	if s.disabled {
		return ResolveResult{User: LocalDevAdmin(), Status: StatusAuthDisabled}
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ResolveResult{Status: StatusAnonymous, Err: ErrNoSession}
	}
	if !s.RepositoryConfigured() {
		return ResolveResult{Status: StatusDatabaseError, Err: fmt.Errorf("%w: repository not configured", ErrDatabase)}
	}
	hash, err := HashSessionToken(raw)
	if err != nil {
		return ResolveResult{Status: StatusInvalid, Err: ErrInvalidSession}
	}
	user, err := s.repo.ResolveUserFromSession(ctx, hash)
	if err == nil && user != nil && user.ID > 0 {
		return ResolveResult{User: user, Status: StatusAuthenticated}
	}
	switch {
	case errors.Is(err, ErrExpiredSession):
		return ResolveResult{Status: StatusExpired, Err: ErrExpiredSession}
	case errors.Is(err, ErrDisabledUser):
		return ResolveResult{Status: StatusDisabledUser, Err: ErrDisabledUser}
	case errors.Is(err, ErrDatabase):
		return ResolveResult{Status: StatusDatabaseError, Err: err}
	default:
		return ResolveResult{Status: StatusInvalid, Err: ErrInvalidSession}
	}
}

func readSessionCookie(r *http.Request, name string) string {
	if r == nil {
		return ""
	}
	c, err := r.Cookie(name)
	if err != nil || c == nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

func (s *Service) sessionManager() SessionManager {
	if s == nil || s.repo == nil {
		return nil
	}
	m, ok := s.repo.(SessionManager)
	if !ok {
		return nil
	}
	return m
}

// Login verifies credentials and creates a PHP-compatible session cookie + DB row.
// Mirrors PHP jrs_auth_login. Returns ErrInvalidCredentials on failure (same UX message).
func (s *Service) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password string) error {
	if s == nil {
		return ErrDatabase
	}
	if s.disabled {
		return ErrInvalidCredentials
	}
	mgr := s.sessionManager()
	if mgr == nil || !s.RepositoryConfigured() {
		return fmt.Errorf("%w: session manager not configured", ErrDatabase)
	}

	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return ErrInvalidCredentials
	}
	if err := mgr.AuthenticatePassword(ctx, username, password); err != nil {
		return err
	}
	return s.createSessionCookie(ctx, w, r, username)
}

// CreateSessionForUsername creates a normal Report Server session for an already
// authenticated identity (e.g. after successful OIDC). Does not verify a password.
func (s *Service) CreateSessionForUsername(ctx context.Context, w http.ResponseWriter, r *http.Request, username string) error {
	if s == nil {
		return ErrDatabase
	}
	if s.disabled {
		return ErrInvalidCredentials
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return ErrInvalidCredentials
	}
	return s.createSessionCookie(ctx, w, r, username)
}

func (s *Service) createSessionCookie(ctx context.Context, w http.ResponseWriter, r *http.Request, username string) error {
	mgr := s.sessionManager()
	if mgr == nil || !s.RepositoryConfigured() {
		return fmt.Errorf("%w: session manager not configured", ErrDatabase)
	}
	token, err := RandomSessionToken()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	hash, err := HashSessionToken(token)
	if err != nil {
		return ErrInvalidSession
	}
	expiresAt := time.Now().Add(time.Duration(SessionTTLSeconds) * time.Second)
	if err := mgr.CreateSession(ctx, hash, username, expiresAt); err != nil {
		return err
	}
	SetSessionCookie(w, r, s.CookieName(), token, expiresAt)
	s.logger.Info("session created",
		"username", username,
		"token_hash_prefix", TokenHashPrefix(hash),
	)
	return nil
}

// Logout deletes the current session row (when present) and clears the cookie.
// Mirrors PHP jrs_auth_logout; always clears the cookie even if DB delete fails.
func (s *Service) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if s == nil {
		return
	}
	raw := readSessionCookie(r, s.CookieName())
	if raw != "" {
		if hash, err := HashSessionToken(raw); err == nil {
			if mgr := s.sessionManager(); mgr != nil {
				if err := mgr.DeleteSession(ctx, hash); err != nil {
					s.logger.Warn("session delete failed",
						"token_hash_prefix", TokenHashPrefix(hash),
						"error", err,
					)
				} else {
					s.logger.Info("session destroyed",
						"token_hash_prefix", TokenHashPrefix(hash),
					)
				}
			}
		}
	}
	ClearSessionCookie(w, r, s.CookieName())
}
