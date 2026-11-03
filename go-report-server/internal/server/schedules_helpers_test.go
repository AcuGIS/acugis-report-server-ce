package server

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParsePostedFormMultipart(t *testing.T) {
	var body strings.Builder
	w := multipart.NewWriter(&body)
	_ = w.WriteField("update", "1")
	_ = w.WriteField("id", "42")
	_ = w.WriteField("report_name", "demo")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/action/schedule.php", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", w.FormDataContentType())

	if err := parsePostedForm(req); err != nil {
		t.Fatal(err)
	}
	if got := req.FormValue("update"); got != "1" {
		t.Fatalf("update=%q", got)
	}
	if got := req.FormValue("id"); got != "42" {
		t.Fatalf("id=%q", got)
	}
	post := formToMap(req)
	if strFromMap(post, "update") != "1" || strFromMap(post, "id") != "42" {
		t.Fatalf("post=%v", post)
	}
}

func TestParsePostedFormURLEncoded(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/action/schedule.php", strings.NewReader("run_now=1&id=7"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := parsePostedForm(req); err != nil {
		t.Fatal(err)
	}
	if req.FormValue("run_now") != "1" || req.FormValue("id") != "7" {
		t.Fatalf("form=%v", req.PostForm)
	}
}
