package server

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

// handleReportThumbnail mirrors reports/report_thumbnail.php?report=.
func (s *Server) handleReportThumbnail(w http.ResponseWriter, r *http.Request) {
	key := jasper.NormalizeReportKey(r.URL.Query().Get("report"))
	s.serveReportThumbnail(w, r, key)
}

// handleAPIReportThumbnail mirrors GET /api/v1/reports/{key}/thumbnail.
func (s *Server) handleAPIReportThumbnail(w http.ResponseWriter, r *http.Request) {
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	s.serveReportThumbnail(w, r, key)
}

func (s *Server) serveReportThumbnail(w http.ResponseWriter, r *http.Request, key string) {
	if !jasper.IsValidReportKey(key) {
		http.NotFound(w, r)
		return
	}
	user := auth.FromContext(r.Context())
	ok, err := s.canViewReport(r, user, key)
	if err != nil {
		s.writeAuthzJSON(w, errStoreUnavailable)
		return
	}
	if !ok {
		s.writeAuthzJSON(w, errPermissionDenied)
		return
	}
	if s.deps.Reports == nil {
		http.NotFound(w, r)
		return
	}
	path := s.deps.Reports.ThumbnailPath(key)
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, path)
}
