package contentresource

import "testing"

func TestCanonicalResourcePath(t *testing.T) {
	cases := []struct {
		folder, leaf, want string
	}{
		{"", "family.png", "/family.png"},
		{"/david", "family.png", "/david/family.png"},
		{"david", "family.png", "/david/family.png"},
		{"/david/kids", `family.png`, "/david/kids/family.png"},
		{"/david", `C:\tmp\family.png`, "/david/family.png"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := CanonicalResourcePath(tc.folder, tc.leaf); got != tc.want {
			t.Errorf("CanonicalResourcePath(%q,%q)=%q want %q", tc.folder, tc.leaf, got, tc.want)
		}
	}
}

func TestNormalizeResourceLookupPath(t *testing.T) {
	cases := map[string]string{
		"david/family.png":       "/david/family.png",
		"/david/family.png":      "/david/family.png",
		`david\family.png`:       "/david/family.png",
		"https://x/y.png":        "",
		"../secret.png":          "",
		"":                       "",
	}
	for in, want := range cases {
		if got := NormalizeResourceLookupPath(in); got != want {
			t.Errorf("NormalizeResourceLookupPath(%q)=%q want %q", in, got, want)
		}
	}
}
