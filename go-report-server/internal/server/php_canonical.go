package server

import (
	"net/http"
	"strings"
)

// phpPathAliases maps legacy browser *.php paths to a clean canonical path
// when stripping the extension would not match the already-public URL.
var phpPathAliases = map[string]string{
	"/reports/execute_report.php": "/reports/execute",
}

// phpCanonicalMiddleware serves extensionless public URLs as canonical:
//   GET/HEAD  *.php  -> 301 to the clean path (query string preserved)
//   POST/PUT/… *.php -> internally rewrite to the clean path (method and body kept)
//
// REST/API paths under /api/ are not rewritten so existing PHP-shaped API
// aliases such as /api/resource-sync/index.php keep working.
func (s *Server) phpCanonicalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean, ok := s.canonicalPublicPHPPath(r.URL.Path)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			target := clean
			if q := r.URL.RawQuery; q != "" {
				target += "?" + q
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		default:
			r2 := r.Clone(r.Context())
			r2.URL.Path = clean
			r2.URL.RawPath = ""
			if q := r.URL.RawQuery; q != "" {
				r2.RequestURI = clean + "?" + q
			} else {
				r2.RequestURI = clean
			}
			next.ServeHTTP(w, r2)
		}
	})
}

// canonicalPublicPHPPath returns the clean public URL for a browser-facing
// *.php request path (including JRS_BASE_PATH). ok is false when the path is
// not a public PHP URL or is an API endpoint.
func (s *Server) canonicalPublicPHPPath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	base := s.mountPath()
	appPath := path
	if base != "" && (path == base || strings.HasPrefix(path, base+"/")) {
		appPath = strings.TrimPrefix(path, base)
		if appPath == "" {
			appPath = "/"
		}
	}
	if strings.HasPrefix(appPath, "/api/") {
		return "", false
	}
	cleanApp, ok := stripPublicPHP(appPath)
	if !ok || cleanApp == appPath {
		return "", false
	}
	if base == "" {
		return cleanApp, true
	}
	return base + cleanApp, true
}

// stripPublicPHP maps a mount-relative *.php path to its canonical public path.
func stripPublicPHP(path string) (string, bool) {
	if alias, ok := phpPathAliases[path]; ok {
		return alias, true
	}
	if !strings.HasSuffix(path, ".php") {
		return path, false
	}
	trimmed := strings.TrimSuffix(path, ".php")
	if strings.HasSuffix(trimmed, "/index") {
		return strings.TrimSuffix(trimmed, "index"), true
	}
	if trimmed == "" || trimmed == "/" {
		return "/", true
	}
	return trimmed, true
}
