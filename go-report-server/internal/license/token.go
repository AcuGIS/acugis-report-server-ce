package license

import (
	"encoding/base64"
	"strings"
)

const (
	// TokenMarker is the versioned AcuGIS license format prefix.
	TokenMarker = "ACL1"
	tokenParts  = 3
)

// ParseToken splits an ACL1 token into its encoded payload and signature.
// It does not decode, verify, or interpret the payload.
func ParseToken(token string) (payloadB64, sigB64 string, err error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", errInvalid()
	}
	parts := strings.Split(token, ".")
	if len(parts) != tokenParts {
		return "", "", errInvalid()
	}
	if parts[0] != TokenMarker {
		return "", "", errInvalid()
	}
	if parts[1] == "" || parts[2] == "" {
		return "", "", errInvalid()
	}
	return parts[1], parts[2], nil
}

func signedMessage(payloadB64 string) []byte {
	return []byte(TokenMarker + "." + payloadB64)
}

func decodeB64(raw string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errInvalid()
	}
	if len(b) == 0 {
		return nil, errInvalid()
	}
	return b, nil
}
