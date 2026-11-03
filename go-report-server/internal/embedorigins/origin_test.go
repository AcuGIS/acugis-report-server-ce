package embedorigins

import "testing"

func TestNormalizeOrigin(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"https://APP.Example.com":      "https://app.example.com",
		"https://app.example.com:443":  "https://app.example.com",
		"http://localhost:80":          "http://localhost",
		"http://127.0.0.1:80":          "http://127.0.0.1",
		"https://app.example.com:8443": "https://app.example.com:8443",
		"http://localhost:3000":        "http://localhost:3000",
		"HTTP://LocalHost:3000":        "http://localhost:3000",
		"http://[::1]:3000":            "http://[::1]:3000",
		"https://[::1]:443":            "https://[::1]",
	}
	for raw, want := range cases {
		got, err := NormalizeOrigin(raw)
		if err != nil {
			t.Fatalf("NormalizeOrigin(%q): %v", raw, err)
		}
		if got != want {
			t.Fatalf("NormalizeOrigin(%q)=%q want %q", raw, got, want)
		}
	}
}

func TestNormalizeOriginRejects(t *testing.T) {
	t.Parallel()
	rejected := []string{
		"",
		"   ",
		"*",
		"*.example.com",
		"https://*.example.com",
		"example.com",
		"https://app.example.com/path",
		"https://app.example.com/",
		"https://app.example.com?x=1",
		"https://app.example.com#frag",
		"javascript:alert(1)",
		"data:text/html,hi",
		"https://user:pass@app.example.com",
		"https://user@app.example.com",
		"http://localhost:3000/app",
		"https://app.example.com:443/path",
		"ftp://app.example.com",
		"https://app.example.com:0",
		"https://app.example.com:99999",
		"https://app.example.com extra",
	}
	for _, raw := range rejected {
		if _, err := NormalizeOrigin(raw); err == nil {
			t.Fatalf("expected reject for %q", raw)
		}
	}
}

func TestFrameAncestorsEmptyIsNone(t *testing.T) {
	t.Parallel()
	if got := FrameAncestors(nil); got != "'none'" {
		t.Fatalf("nil: %q", got)
	}
	if got := FrameAncestors([]string{}); got != "'none'" {
		t.Fatalf("empty: %q", got)
	}
	if got := FrameAncestors([]string{"https://app.example.com", "*"}); got != "https://app.example.com *" {
		t.Fatalf("joined: %q", got)
	}
}
