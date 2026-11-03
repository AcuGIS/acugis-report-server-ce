package server

import (
	"encoding/json"
	"html/template"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

func (s *Server) handleDesignerAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s == nil || !s.designerConfigured() {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	rel = path.Clean("/" + rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." || rel == "index.html" || strings.Contains(rel, "..") || strings.ContainsAny(rel, `\:`) {
		http.NotFound(w, r)
		return
	}
	root := strings.TrimSpace(s.cfg.Designer.AssetsDir)
	if root == "" {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.Abs(root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	if !withinDir(root, full) {
		http.NotFound(w, r)
		return
	}
	st, err := os.Lstat(full)
	if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
		http.NotFound(w, r)
		return
	}
	user := auth.FromContext(r.Context())
	if err := s.requireDesignerUser(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.designerEntitled() {
		s.writeAuthzJSON(w, errDesignerNotLicensed)
		return
	}
	ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(full)))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, full)
}

func withinDir(root, full string) bool {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

type designerBoot struct {
	ReportServerMode bool             `json:"reportServerMode"`
	ReportKey        string           `json:"reportKey"`
	SessionID        string           `json:"sessionId"`
	ReportName       string           `json:"reportName"`
	ReturnURL        string           `json:"returnUrl"`
	Permissions      map[string]bool  `json:"permissions"`
	Folders          []designerFolder `json:"folders"`
}

type designerFolder struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// designerReturnURL is the Reports landing injected into the Designer bootstrap.
// Standalone Report Server uses the Reports hub. QCarta has no hub routes, so
// it returns the QCarta reports catalog. The browser cannot choose this URL.
func (s *Server) designerReturnURL() string {
	if s != nil && s.qcartaProfile() {
		return s.goURL("/reports")
	}
	if s == nil {
		return "/hubs/reports"
	}
	return s.goURL("/hubs/reports")
}

func (s *Server) renderDesignerSPA(w http.ResponseWriter, key, sessionID, reportName string, folders []designerFolder) error {
	root := strings.TrimSpace(s.cfg.Designer.AssetsDir)
	indexPath := filepath.Join(root, "index.html")
	raw, err := os.ReadFile(indexPath)
	if err != nil || len(raw) == 0 {
		return errDesignerUnavailable
	}
	if !bytesContains(raw, []byte("id=\"root\"")) && !bytesContains(raw, []byte("id='root'")) {
		return errDesignerUnavailable
	}
	boot := designerBoot{
		ReportServerMode: true,
		ReportKey:        key,
		SessionID:        sessionID,
		ReportName:       reportName,
		ReturnURL:        s.designerReturnURL(),
		Permissions:      map[string]bool{"edit": true},
		Folders:          folders,
	}
	payload, err := json.Marshal(boot)
	if err != nil {
		return errDesignerUnavailable
	}
	payload = []byte(strings.ReplaceAll(string(payload), "<", `\u003c`))
	title := template.HTMLEscapeString(reportName + " — AcuGIS Report Designer")
	script := `<script>window.__ACUGIS_DESIGNER__ = ` + string(payload) + `;</script>`
	html := string(raw)
	if strings.Contains(html, "</head>") {
		html = strings.Replace(html, "</head>", script+"</head>", 1)
	} else {
		html = script + html
	}
	if strings.Contains(html, "<title>") && strings.Contains(html, "</title>") {
		start := strings.Index(html, "<title>")
		end := strings.Index(html, "</title>")
		if start >= 0 && end > start {
			html = html[:start] + "<title>" + title + html[end:]
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err = w.Write([]byte(html))
	return err
}

func bytesContains(b, sub []byte) bool {
	return strings.Contains(string(b), string(sub))
}
