package server

import (
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/acugis/acugis-report-server-go/internal/embedorigins"
)

func (s *Server) effectiveEmbedOrigins() []string {
	if s != nil && s.deps.EmbedOrigins != nil {
		return s.deps.EmbedOrigins.Effective(s.cfg.EmbedAllowedOrigins)
	}
	if s == nil {
		return nil
	}
	return s.cfg.EmbedAllowedOrigins
}

func (s *Server) embedOriginAllowed(origin string) bool {
	if s != nil && s.deps.EmbedOrigins != nil {
		return s.deps.EmbedOrigins.Allows(s.cfg.EmbedAllowedOrigins, origin)
	}
	return s.cfg.EmbedOriginAllowed(origin)
}

func (s *Server) embedFrameAncestors() string {
	return embedorigins.FrameAncestors(s.effectiveEmbedOrigins())
}

// applyAuthenticatedEmbedFraming replaces SAMEORIGIN on an authenticated
// /reports/execute response. The same-origin embed viewer is an ancestor of
// that document, so a non-empty external allow-list is emitted as
// frame-ancestors 'self' plus those origins. 'self' is not an Allowed Origin.
// An empty external list stays frame-ancestors 'none'.
func (s *Server) applyAuthenticatedEmbedFraming(w http.ResponseWriter) {
	if w == nil {
		return
	}
	h := w.Header()
	h.Del("X-Frame-Options")
	origins := s.effectiveEmbedOrigins()
	sources := embedorigins.FrameAncestors(origins)
	if len(origins) > 0 {
		sources = "'self' " + sources
	}
	h.Set("Content-Security-Policy", "frame-ancestors "+sources)
}

func (s *Server) embedAllowedOriginsJS() template.JS {
	origins := s.effectiveEmbedOrigins()
	if origins == nil {
		origins = []string{}
	}
	b, err := json.Marshal(origins)
	if err != nil || len(b) == 0 {
		return "[]"
	}
	return template.JS(b)
}
