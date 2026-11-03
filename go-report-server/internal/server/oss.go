//go:build oss

package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/explorer"
)

func (s *Server) cleanupGovernance(context.Context, string, string) {}

func (s *Server) loginSSOState(*http.Request) loginSSOView {
	return loginSSOView{LocalEnabled: true}
}

func (s *Server) mountProfessional(chi.Router) {}

func (s *Server) explorerSvc() *explorer.Service {
	return s.deps.Explorer
}

func explorerOwner(user *auth.User) string {
	if user == nil || user.Username == "" {
		return "anonymous"
	}
	return user.Username
}

func explorerViewRef(v *explorer.SavedView) string {
	if v == nil {
		return ""
	}
	return explorer.ViewRef(v.OwnerUsername, v.Key)
}

func (s *Server) loadExplorerView(*http.Request, string, string) (*explorer.SavedView, error) {
	return nil, explorer.ErrNotFound
}

func (s *Server) canViewExplorerView(*http.Request, *auth.User, *explorer.SavedView) (bool, error) {
	return false, nil
}

func (s *Server) canEditExplorerView(*http.Request, *auth.User, *explorer.SavedView) (bool, error) {
	return false, nil
}

func (s *Server) writeExplorerErr(w http.ResponseWriter, err error) {
	if errors.Is(err, explorer.ErrNotFound) {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	msg := "request failed"
	if err != nil && err.Error() != "" {
		msg = err.Error()
	}
	s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

func (s *Server) handleQGISConnectionsPage(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *Server) handleArcGISConnectionsPage(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}
