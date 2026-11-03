package arcgis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectionAnonymousFeatureServer(t *testing.T) {
	var tokenCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			tokenCalls++
			http.Error(w, "token should not be requested", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"layers": []any{map[string]any{"id": 0, "name": "Neighborhoods"}},
		})
	}))
	defer srv.Close()

	msg, err := TestConnection(context.Background(), Credentials{
		Name: "NeighborhoodsFS",
		URL:  srv.URL + "/arcgis/rest/services/Neighborhoods/FeatureServer/0?f=pjson",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tokenCalls != 0 {
		t.Fatalf("anonymous test requested a token %d times", tokenCalls)
	}
	if !strings.Contains(msg, "Connected") {
		t.Fatalf("message=%q", msg)
	}
}

func TestConnectionAuthenticatedFeatureServer(t *testing.T) {
	var sawPassword string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "tokens") {
			_ = r.ParseForm()
			sawPassword = r.Form.Get("password")
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok-1"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"layers": []any{map[string]any{"id": 2, "name": "Parcels"}},
		})
	}))
	defer srv.Close()

	msg, err := TestConnection(context.Background(), Credentials{
		URL:      srv.URL + "/arcgis/rest/services/Parcels/MapServer",
		Username: "editor",
		Password: "s3cret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sawPassword != "s3cret" {
		t.Fatalf("token password=%q", sawPassword)
	}
	if !strings.Contains(msg, "Connected") {
		t.Fatalf("message=%q", msg)
	}
}

func TestConnectionInvalidURL(t *testing.T) {
	_, err := TestConnection(context.Background(), Credentials{URL: "ftp://services.example.com/FeatureServer"})
	if err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("err=%v", err)
	}
	_, err = TestConnection(context.Background(), Credentials{URL: "https://services.example.com/arcgis/rest/services/NotAService"})
	if err == nil || !strings.Contains(err.Error(), "FeatureServer or MapServer") {
		t.Fatalf("err=%v", err)
	}
}

func TestConnectionInvalidCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": 400, "message": "Invalid username or password."},
		})
	}))
	defer srv.Close()

	_, err := TestConnection(context.Background(), Credentials{
		URL:      srv.URL + "/arcgis/rest/services/Secure/FeatureServer",
		Username: "editor",
		Password: "wrong",
	})
	if err == nil || !strings.Contains(err.Error(), "Invalid username or password") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "internal") {
		t.Fatalf("generic internal error: %v", err)
	}
}

func TestConnectionTokenFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"token":""}`))
	}))
	defer srv.Close()

	_, err := TestConnection(context.Background(), Credentials{
		URL:      srv.URL + "/arcgis/rest/services/Secure/FeatureServer",
		Username: "editor",
		Password: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "ArcGIS token request failed") {
		t.Fatalf("err=%v", err)
	}
}
