package auth_test

import (
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

func TestHashSessionTokenPHPCompatible(t *testing.T) {
	// Golden fixtures match PHP hash('sha256', $raw, false) lowercase hex.
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name: "ascii",
			raw:  "hello",
			want: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		},
		{
			name: "known_fixture",
			raw:  "jrs-test-token-not-a-secret",
			want: "277d2e5ba4b855d61a450755352c91f582b320dbdc7402d31c426cf8fd08599f",
		},
		{
			name: "punctuation",
			raw:  "a+b/c=_-~.!?",
			want: "7d86b5a8c4805551d30b129bf00cf90882931a1a59b0cb11f7117bc23f29b6b7",
		},
		{
			name: "long_token",
			raw:  "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-_",
			want: "88b97bc371988d5ce4ec28665260a0128d40f591f809a2d6c98ae7c537d8545d",
		},
		{
			name:    "empty",
			raw:     "",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := auth.HashSessionToken(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
			if got != strings.ToLower(got) {
				t.Fatal("hash must be lowercase hex")
			}
			if len(got) != 64 {
				t.Fatalf("hash len=%d", len(got))
			}
		})
	}
}

func TestTokenHashPrefixSafe(t *testing.T) {
	h := "277d2e5ba4b855d61a450755352c91f582b320dbdc7402d31c426cf8fd08599f"
	p := auth.TokenHashPrefix(h)
	if p != "277d2e5ba4b8" {
		t.Fatalf("prefix=%q", p)
	}
	if auth.TokenHashPrefix("short") != "" {
		t.Fatal("expected empty prefix for short input")
	}
}
