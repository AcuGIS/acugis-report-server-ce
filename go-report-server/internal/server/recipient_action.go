package server

import (
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

func (s *Server) handleRecipientAction(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}

	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"success": false,
			"message": "Method not allowed.",
		})
		return
	}

	repo := s.deps.Recipients
	if repo == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "Database unavailable.",
		})
		return
	}

	if err := parsePostedForm(r); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Failed to parse form.",
		})
		return
	}

	if r.FormValue("delete") != "" {
		id, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("id")))
		if err := repo.Delete(r.Context(), id); err != nil {
			s.writeJSON(w, http.StatusOK, map[string]any{
				"success": false,
				"message": "Delete failed.",
			})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Recipient deleted.",
		})
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	if email == "" || !validEmailAddress(email) {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Valid email is required.",
		})
		return
	}

	var idPtr *int
	if raw := strings.TrimSpace(r.FormValue("id")); raw != "" {
		if id, err := strconv.Atoi(raw); err == nil && id > 0 {
			idPtr = &id
		}
	}

	if err := repo.Save(r.Context(), idPtr, name, email, notes); err != nil {
		msg := "Save failed."
		if err == schedules.ErrUnavailable {
			msg = "Database unavailable."
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": msg})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Recipient saved.",
	})
}

func validEmailAddress(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}
