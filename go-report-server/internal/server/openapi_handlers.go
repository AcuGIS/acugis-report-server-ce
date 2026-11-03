package server

import (
	"net/http"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/openapi"
)

func (s *Server) handleOpenAPIYAML(w http.ResponseWriter, _ *http.Request) {
	body, err := openapi.MarshalYAML(s.cfg.AppVersion)
	if err != nil {
		s.logger.Error("openapi yaml", "error", err)
		http.Error(w, "openapi unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) handleOpenAPIJSON(w http.ResponseWriter, _ *http.Request) {
	body, err := openapi.MarshalJSON(s.cfg.AppVersion)
	if err != nil {
		s.logger.Error("openapi json", "error", err)
		http.Error(w, "openapi unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	base := strings.TrimRight(s.cfg.BasePath, "/")
	if base == "/" {
		base = ""
	}
	specURL := base + "/api/openapi.yaml"
	html := swaggerUIHTML(specURL)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func swaggerUIHTML(specURL string) string {
	const prefix = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AcuGIS Report Server API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css" crossorigin>
  <style>
    body { margin: 0; background: #fafafa; }
    .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js" crossorigin></script>
  <script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-standalone-preset.js" crossorigin></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "`
	const suffix = `",
      dom_id: '#swagger-ui',
      deepLinking: true,
      persistAuthorization: true,
      tryItOutEnabled: true,
      displayRequestDuration: true,
      filter: true,
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
      layout: "StandaloneLayout",
      supportedSubmitMethods: ['get','post','put','delete','patch'],
      requestInterceptor: (req) => {
        req.credentials = 'same-origin';
        return req;
      }
    });
  </script>
</body>
</html>`
	return prefix + specURL + suffix
}
