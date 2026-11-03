package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// mountStaticAssets serves PHP-tree UI assets so Go pages can reuse the same
// CSS/JS without inventing a parallel design system.
//
// When BasePath is /go, assets are also served under /go/shared/ui so a reverse
// proxy that only forwards /go/* still reaches shell CSS/JS.
func (s *Server) mountStaticAssets(root chi.Router, mounted chi.Router) {
	appRoot := strings.TrimSpace(s.cfg.Paths.AppRoot)
	if appRoot == "" {
		return
	}

	sharedUI := filepath.Join(appRoot, "shared", "ui")
	if dirExists(sharedUI) {
		h := pathTailFileServer("/shared/ui", sharedUI)
		root.Handle("/shared/ui/*", h)
		if root != mounted {
			mounted.Handle("/shared/ui/*", h)
		}
	}

	// Always register this literal path so /reports/{key} never treats
	// "portal.css" as a report key (browsers then see text/plain + nosniff).
	portalCSS := filepath.Join(appRoot, "reports", "portal.css")
	mounted.Get("/reports/portal.css", func(w http.ResponseWriter, r *http.Request) {
		if !fileExists(portalCSS) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, portalCSS)
	})

	thumbDir := strings.TrimSpace(s.cfg.Paths.ThumbnailDir)
	if thumbDir == "" {
		thumbDir = filepath.Join(appRoot, "assets", "reports")
	}
	if dirExists(thumbDir) {
		mounted.Handle("/assets/reports/*", pathTailFileServer("/assets/reports", thumbDir))
	}

	// Map viewer (view.php) loads ../assets/js/map-style.js from /reports/{key}.
	assetsJS := filepath.Join(appRoot, "assets", "js")
	if dirExists(assetsJS) {
		mounted.Handle("/assets/js/*", pathTailFileServer("/assets/js", assetsJS))
	}
}

// pathTailFileServer serves files from dir using the path after marker
// (works whether the request path is /shared/ui/x or /go/shared/ui/x).
func pathTailFileServer(marker, dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	marker = strings.TrimRight(marker, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		idx := strings.Index(path, marker+"/")
		if idx < 0 {
			if strings.HasSuffix(path, marker) {
				http.NotFound(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}
		rel := path[idx+len(marker):]
		if rel == "" || rel == "/" {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		u := *r.URL
		u.Path = rel
		r2.URL = &u
		fs.ServeHTTP(w, r2)
	})
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
