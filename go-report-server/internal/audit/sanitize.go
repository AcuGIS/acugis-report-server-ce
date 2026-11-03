package audit

import (
	"encoding/json"
	"regexp"
	"strings"
)

var secretNamePattern = regexp.MustCompile(`(?i)(password|passwd|secret|token|apikey|api[_-]?key|authorization|credential|private[_-]?key|bearer|cookie|session)`)

// urlUserinfoPattern matches scheme://user:password@host credentials in free-form text.
// Replacement keeps the scheme and host while redacting the userinfo segment.
var urlUserinfoPattern = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*):\/\/([^\/\s"'<>]+)@`)

// SanitizeError returns a short, safe error description for audit details (no stack traces).
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return SanitizeErrorMessage(err.Error())
}

// SanitizeErrorMessage sanitizes a free-form error string.
// Newlines are collapsed, credential-bearing URL userinfo is redacted, and length is capped.
func SanitizeErrorMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = redactURLUserinfo(msg)
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}

// redactURLUserinfo replaces user:pass@ in URLs with [redacted]@.
func redactURLUserinfo(s string) string {
	if s == "" || !strings.Contains(s, "@") || !strings.Contains(s, "://") {
		return s
	}
	return urlUserinfoPattern.ReplaceAllString(s, `${1}://[redacted]@`)
}

// IsSecretParamName reports whether a parameter/field name suggests a secret.
func IsSecretParamName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	return secretNamePattern.MatchString(name)
}

// SanitizeParams copies report/query parameters, omitting secret-named keys.
// Values are preserved as JSON-compatible types; maps/slices are walked recursively.
func SanitizeParams(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		if IsSecretParamName(k) {
			out[k] = "[redacted]"
			continue
		}
		out[k] = sanitizeValue(v)
	}
	return out
}

func sanitizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return SanitizeParams(t)
	case map[string]string:
		m := make(map[string]any, len(t))
		for k, val := range t {
			if IsSecretParamName(k) {
				m[k] = "[redacted]"
			} else {
				m[k] = val
			}
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = sanitizeValue(item)
		}
		return out
	case []string:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = item
		}
		return out
	default:
		return v
	}
}

// SanitizeDetailsMap removes secret keys from a details map and caps nested depth.
func SanitizeDetailsMap(details map[string]any) map[string]any {
	if details == nil {
		return nil
	}
	return SanitizeParams(details)
}

// DetailsJSON marshals sanitized details to JSON bytes for storage.
func DetailsJSON(details map[string]any) ([]byte, error) {
	clean := SanitizeDetailsMap(details)
	if clean == nil {
		return nil, nil
	}
	return json.Marshal(clean)
}
