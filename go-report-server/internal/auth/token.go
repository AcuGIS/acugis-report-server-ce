package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashSessionToken transforms a raw jrs_session cookie value the same way PHP
// jrs_auth_session_token_hash does: SHA-256 over the raw bytes, lowercase hex.
//
//	Browser cookie: raw token
//	Database value: lowercase hex SHA-256 digest (64 chars)
func HashSessionToken(raw string) (string, error) {
	if raw == "" {
		return "", ErrEmptyToken
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}

// TokenHashPrefix returns a short, log-safe prefix of a token hash (never the
// full hash). Empty input yields an empty string.
func TokenHashPrefix(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) < 12 {
		return ""
	}
	return hash[:12]
}

// GroupsIncludeAdmin mirrors PHP jrs_auth_groups_include_admin.
func GroupsIncludeAdmin(groups []string) bool {
	for _, g := range groups {
		n := strings.ToLower(strings.TrimSpace(g))
		switch n {
		case "administrator", "administrators", "admin", "admins", "dashboard-admin":
			return true
		}
	}
	return false
}
