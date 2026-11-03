package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"log/slog"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIEndpoints(t *testing.T) {
	cfg := testConfig()
	cfg.AppVersion = "1.2.3-openapi"
	s := New(cfg, Dependencies{}, slog.Default())

	t.Run("yaml", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, "yaml") {
			t.Fatalf("Content-Type=%q", ct)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		info, _ := doc["info"].(map[string]any)
		if info["version"] != "1.2.3-openapi" {
			t.Fatalf("version=%v", info["version"])
		}
		paths, _ := doc["paths"].(map[string]any)
		if _, ok := paths["/api/v1/auth/login"]; !ok {
			t.Fatal("missing login path")
		}
	})

	t.Run("json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		var doc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		info, _ := doc["info"].(map[string]any)
		if info["version"] != "1.2.3-openapi" {
			t.Fatalf("version=%v", info["version"])
		}
	})

	t.Run("swagger", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "/api/openapi.yaml") {
			t.Fatal("swagger html missing openapi url")
		}
		if !strings.Contains(body, "SwaggerUIBundle") {
			t.Fatal("swagger html missing SwaggerUIBundle")
		}
	})
}

func TestOpenAPIWithBasePath(t *testing.T) {
	cfg := testConfig()
	cfg.BasePath = "/go"
	cfg.AppVersion = "test"
	s := New(cfg, Dependencies{}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/go/swagger", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `url: "/go/api/openapi.yaml"`) {
		body := rec.Body.String()
		if len(body) > 200 {
			body = body[:200]
		}
		t.Fatalf("expected base-path openapi url, got: %s", body)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/go/api/openapi.json", nil)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("json status=%d", rec2.Code)
	}
}
