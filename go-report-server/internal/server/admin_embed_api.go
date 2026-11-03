package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/embedorigins"
)

type embedOriginBody struct {
	Origin string `json:"origin"`
}

func (s *Server) requireEmbedAdmin(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return nil, false
	}
	if s.deps.EmbedOrigins == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "embedded analytics is unavailable"})
		return nil, false
	}
	return user, true
}

func (s *Server) handleAPIAdminEmbedOriginsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireEmbedAdmin(w, r); !ok {
		return
	}
	s.writeEmbedOriginList(w, http.StatusOK)
}

func (s *Server) handleAPIAdminEmbedOriginsAdd(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireEmbedAdmin(w, r)
	if !ok {
		return
	}
	body, ok := s.readEmbedOriginBody(w, r)
	if !ok {
		return
	}
	uid := int64(0)
	if user != nil {
		uid = user.ID
	}
	created, err := s.deps.EmbedOrigins.Add(r.Context(), s.cfg.EmbedAllowedOrigins, body.Origin, uid)
	if err != nil {
		s.writeEmbedOriginErr(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	s.writeEmbedOriginList(w, status)
}

func (s *Server) handleAPIAdminEmbedOriginsDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireEmbedAdmin(w, r); !ok {
		return
	}
	body, ok := s.readEmbedOriginBody(w, r)
	if !ok {
		return
	}
	if err := s.deps.EmbedOrigins.Remove(r.Context(), s.cfg.EmbedAllowedOrigins, body.Origin); err != nil {
		s.writeEmbedOriginErr(w, err)
		return
	}
	s.writeEmbedOriginList(w, http.StatusOK)
}

func (s *Server) readEmbedOriginBody(w http.ResponseWriter, r *http.Request) (embedOriginBody, bool) {
	var body embedOriginBody
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid origin"})
		return body, false
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid origin"})
		return body, false
	}
	return body, true
}

func (s *Server) writeEmbedOriginList(w http.ResponseWriter, status int) {
	s.writeJSON(w, status, map[string]any{
		"origins": s.deps.EmbedOrigins.Entries(s.cfg.EmbedAllowedOrigins),
	})
}

func (s *Server) writeEmbedOriginErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, embedorigins.ErrInvalid):
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Enter an origin like https://app.example.com."})
	case errors.Is(err, embedorigins.ErrSystem):
		s.writeJSON(w, http.StatusConflict, map[string]string{"error": "This origin is system configured and cannot be removed."})
	case errors.Is(err, embedorigins.ErrNotFound):
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "That origin is not in the list."})
	case errors.Is(err, embedorigins.ErrUnavailable):
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "embedded analytics is unavailable"})
	default:
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update allowed origins"})
	}
}
