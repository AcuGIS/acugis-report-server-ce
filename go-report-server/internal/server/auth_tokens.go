package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
)

func (s *Server) requireAuthenticatedAPIUser(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "auth disabled"})
		return nil, false
	}
	user := auth.FromContext(r.Context())
	if user == nil || !auth.IsAuthenticated(r.Context()) || user.ID <= 0 {
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return nil, false
	}
	if s.deps.Auth == nil || !s.deps.Auth.RepositoryConfigured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return nil, false
	}
	return user, true
}

func (s *Server) handleAPIAuthMe(w http.ResponseWriter, r *http.Request) {
	caps := s.editionCapabilities()
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		u := auth.LocalDevAdmin()
		s.writeJSON(w, http.StatusOK, map[string]any{
			"id":            u.ID,
			"username":      u.Username,
			"email":         u.Email,
			"groups":        []string{},
			"is_admin":      true,
			"permissions":   []string{},
			"auth_disabled": true,
			"edition":       caps.Edition,
			"capabilities":  caps,
		})
		return
	}
	user := auth.FromContext(r.Context())
	if user == nil || !auth.IsAuthenticated(r.Context()) || user.ID <= 0 {
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	groups := user.Groups
	if groups == nil {
		groups = []string{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"id":           user.ID,
		"username":     user.Username,
		"email":        user.Email,
		"groups":       groups,
		"is_admin":     user.IsPlatformAdmin(),
		"permissions":  []string{}, // reserved; tokens inherit full user ACL
		"edition":      caps.Edition,
		"capabilities": caps,
	})
}

func (s *Server) handleAPIAuthTokensList(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuthenticatedAPIUser(w, r)
	if !ok {
		return
	}
	tokens, err := s.deps.Auth.ListPersonalAccessTokens(r.Context(), user.ID)
	if err != nil {
		s.writeTokenAPIError(w, err, "could not list tokens")
		return
	}
	if tokens == nil {
		tokens = []auth.PersonalAccessToken{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

type createTokenRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	ExpiresAt   *string `json:"expires_at"`
	ExpiresIn   string  `json:"expires_in"` // never | 30d | 90d | 1y | custom (custom uses expires_at)
}

func (s *Server) handleAPIAuthTokenCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuthenticatedAPIUser(w, r)
	if !ok {
		return
	}
	var input createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	expiresAt, err := parseTokenExpiration(input.ExpiresIn, input.ExpiresAt)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	meta, plaintext, err := s.deps.Auth.CreatePersonalAccessToken(
		r.Context(),
		user.ID,
		input.Name,
		input.Description,
		expiresAt,
		auth.ClientIPFromRequest(r),
	)
	if err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategorySecurity,
			Action:     audit.ActionCreate,
			ObjectType: audit.ObjectToken,
			ObjectName: strings.TrimSpace(input.Name),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
		})
		s.writeTokenAPIError(w, err, "could not create token")
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategorySecurity,
		Action:     audit.ActionCreate,
		ObjectType: audit.ObjectToken,
		ObjectID:   strconv.FormatInt(meta.ID, 10),
		ObjectName: meta.Name,
		Status:     audit.StatusSuccess,
	})
	s.writeJSON(w, http.StatusCreated, map[string]any{
		"token":      plaintext,
		"id":         meta.ID,
		"created_at": meta.CreatedAt,
		"name":       meta.Name,
		"message":    "This token will only be shown once.",
	})
}

type updateTokenRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	ExpiresAt   *string `json:"expires_at"`
	ExpiresIn   *string `json:"expires_in"`
}

func (s *Server) handleAPIAuthTokenUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuthenticatedAPIUser(w, r)
	if !ok {
		return
	}
	tokenID, ok := parsePathInt64(w, r, "id")
	if !ok {
		return
	}
	var input updateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	upd := auth.PersonalAccessTokenUpdate{
		Name:        input.Name,
		Description: input.Description,
	}
	if input.ExpiresIn != nil || input.ExpiresAt != nil {
		expiresIn := ""
		if input.ExpiresIn != nil {
			expiresIn = *input.ExpiresIn
		}
		exp, err := parseTokenExpiration(expiresIn, input.ExpiresAt)
		if err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		upd.SetExpires = true
		upd.ExpiresAt = exp
	}
	meta, err := s.deps.Auth.UpdatePersonalAccessToken(r.Context(), user.ID, tokenID, upd)
	if err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategorySecurity,
			Action:     audit.ActionUpdate,
			ObjectType: audit.ObjectToken,
			ObjectID:   strconv.FormatInt(tokenID, 10),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
		})
		s.writeTokenAPIError(w, err, "could not update token")
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategorySecurity,
		Action:     audit.ActionUpdate,
		ObjectType: audit.ObjectToken,
		ObjectID:   strconv.FormatInt(meta.ID, 10),
		ObjectName: meta.Name,
		Status:     audit.StatusSuccess,
	})
	s.writeJSON(w, http.StatusOK, meta)
}

func (s *Server) handleAPIAuthTokenDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuthenticatedAPIUser(w, r)
	if !ok {
		return
	}
	tokenID, ok := parsePathInt64(w, r, "id")
	if !ok {
		return
	}
	purge := strings.EqualFold(r.URL.Query().Get("purge"), "1") ||
		strings.EqualFold(r.URL.Query().Get("purge"), "true")
	action := audit.ActionRevoke
	if purge {
		action = audit.ActionDelete
	}
	var err error
	if purge {
		err = s.deps.Auth.DeletePersonalAccessToken(r.Context(), user.ID, tokenID)
	} else {
		err = s.deps.Auth.RevokePersonalAccessToken(r.Context(), user.ID, tokenID)
	}
	if err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategorySecurity,
			Action:     action,
			ObjectType: audit.ObjectToken,
			ObjectID:   strconv.FormatInt(tokenID, 10),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
		})
		s.writeTokenAPIError(w, err, "could not revoke token")
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategorySecurity,
		Action:     action,
		ObjectType: audit.ObjectToken,
		ObjectID:   strconv.FormatInt(tokenID, 10),
		Status:     audit.StatusSuccess,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIAuthTokenRotate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuthenticatedAPIUser(w, r)
	if !ok {
		return
	}
	tokenID, ok := parsePathInt64(w, r, "id")
	if !ok {
		return
	}
	meta, plaintext, err := s.deps.Auth.RotatePersonalAccessToken(
		r.Context(),
		user.ID,
		tokenID,
		auth.ClientIPFromRequest(r),
	)
	if err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategorySecurity,
			Action:     audit.ActionChange,
			ObjectType: audit.ObjectToken,
			ObjectID:   strconv.FormatInt(tokenID, 10),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error()), "operation": "rotate"},
		})
		s.writeTokenAPIError(w, err, "could not rotate token")
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategorySecurity,
		Action:     audit.ActionChange,
		ObjectType: audit.ObjectToken,
		ObjectID:   strconv.FormatInt(meta.ID, 10),
		ObjectName: meta.Name,
		Status:     audit.StatusSuccess,
		Details:    map[string]any{"operation": "rotate"},
	})
	s.writeJSON(w, http.StatusCreated, map[string]any{
		"token":      plaintext,
		"id":         meta.ID,
		"created_at": meta.CreatedAt,
		"name":       meta.Name,
		"message":    "This token will only be shown once. The previous token has been revoked.",
	})
}

func (s *Server) writeTokenAPIError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, auth.ErrTokenUnsupported):
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "personal access tokens are not available"})
	case errors.Is(err, auth.ErrTokenNotFound):
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
	case errors.Is(err, auth.ErrDatabase):
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
	default:
		msg := err.Error()
		if strings.Contains(msg, "is required") || strings.Contains(msg, "must be") {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
			return
		}
		s.logger.Error("token api error", "error", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fallback})
	}
}

func parsePathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, name))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid id"}`))
		return 0, false
	}
	return id, true
}

func parseTokenExpiration(expiresIn string, expiresAt *string) (*time.Time, error) {
	expiresIn = strings.ToLower(strings.TrimSpace(expiresIn))
	switch expiresIn {
	case "", "never":
		if expiresAt == nil || strings.TrimSpace(*expiresAt) == "" {
			return nil, nil
		}
		// Fall through to parse custom date when expires_at provided without expires_in.
	case "30d", "30 days":
		t := time.Now().UTC().Add(30 * 24 * time.Hour)
		return &t, nil
	case "90d", "90 days":
		t := time.Now().UTC().Add(90 * 24 * time.Hour)
		return &t, nil
	case "1y", "1 year", "365d":
		t := time.Now().UTC().Add(365 * 24 * time.Hour)
		return &t, nil
	case "custom":
		if expiresAt == nil || strings.TrimSpace(*expiresAt) == "" {
			return nil, errors.New("expires_at is required for custom expiration")
		}
	default:
		if expiresIn != "" && expiresAt == nil {
			return nil, errors.New("expires_in must be never, 30d, 90d, 1y, or custom")
		}
	}

	if expiresAt == nil || strings.TrimSpace(*expiresAt) == "" {
		return nil, nil
	}
	raw := strings.TrimSpace(*expiresAt)
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		parsed, err = time.Parse("2006-01-02", raw)
		if err != nil {
			return nil, errors.New("expires_at must be an RFC3339 timestamp or YYYY-MM-DD date")
		}
		parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, time.UTC)
	}
	if !parsed.After(time.Now()) {
		return nil, errors.New("expiration must be in the future")
	}
	return &parsed, nil
}
