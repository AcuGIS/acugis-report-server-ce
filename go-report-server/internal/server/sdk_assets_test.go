package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"
)

func TestSDKAssetsMatchPackagedPaths(t *testing.T) {
	// The handler also checks paths relative to the process working directory.
	// Leave the module tree so a developer build cannot satisfy a missing package file.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Error(err)
		}
	})
	dist := filepath.Join(root, "sdk", "javascript", "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{
		"acugis-report.js",
		"acugis-report.min.js",
		"acugis-report.esm.js",
		"index.d.ts",
	}
	for _, name := range files {
		body := "/* " + name + " */\nAcuGIS.embedReport = function(){};\n"
		if strings.HasSuffix(name, ".d.ts") {
			body = "export function embedReport(): void;\n"
		}
		if err := os.WriteFile(filepath.Join(dist, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = root
	s := New(cfg, Dependencies{}, slog.Default())
	handler := s.Handler()

	for _, name := range files {
		for _, path := range []string{"/sdk/" + name, "/sdk/v1/" + name} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), name) && !strings.Contains(rec.Body.String(), "embedReport") {
				t.Fatalf("%s body %q", path, rec.Body.String())
			}
			ctype := rec.Header().Get("Content-Type")
			switch {
			case strings.HasSuffix(name, ".d.ts"):
				if !strings.Contains(ctype, "typescript") {
					t.Fatalf("%s content-type %q", path, ctype)
				}
			default:
				if !strings.Contains(ctype, "javascript") {
					t.Fatalf("%s content-type %q", path, ctype)
				}
			}
		}
	}

	missing := httptest.NewRequest(http.MethodGet, "/sdk/acugis-report.js", nil)
	os.Remove(filepath.Join(dist, "acugis-report.js"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, missing)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "SDK asset not found") {
		t.Fatalf("missing asset status=%d body=%s", rec.Code, rec.Body.String())
	}

	// embed.js is not one of the files the server serves.
	other := httptest.NewRequest(http.MethodGet, "/sdk/embed.js", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, other)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("embed.js status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "SDK asset not found") {
		t.Fatalf("embed.js used the missing-asset response: %s", rec.Body.String())
	}
}
