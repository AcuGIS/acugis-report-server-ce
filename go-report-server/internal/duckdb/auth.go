package duckdb

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// AuthType returns normalized authentication mode for a DuckDB connection.
func AuthType(cfg Config) string {
	if cfg.Options == nil {
		return AuthNone
	}
	return NormalizeAuthType(cfg.Options[OptionAuthType])
}

// NormalizeAuthType maps user/option strings to none|basic|bearer|headers.
func NormalizeAuthType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case AuthBasic, "basic_auth", "basicauth":
		return AuthBasic
	case AuthBearer, "bearer_token", "token":
		return AuthBearer
	case AuthHeaders, "custom", "custom_headers", "header", "http_headers":
		return AuthHeaders
	case AuthNone, "", "anonymous", "public":
		return AuthNone
	default:
		return AuthNone
	}
}

// SourceType returns the remote source family (currently always http for Data Files).
func SourceType(cfg Config) string {
	if cfg.Options != nil {
		s := strings.ToLower(strings.TrimSpace(cfg.Options[OptionSourceType]))
		if s != "" {
			return s
		}
	}
	return SourceTypeHTTP
}

// HTTPHeaderKeys returns configured custom header names (values are secrets).
func HTTPHeaderKeys(cfg Config) []string {
	if cfg.Options == nil {
		return nil
	}
	raw := strings.TrimSpace(cfg.Options[OptionHTTPHeaderKeys])
	if raw == "" {
		return nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil
	}
	out := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		lk := strings.ToLower(k)
		if _, ok := seen[lk]; ok {
			continue
		}
		seen[lk] = struct{}{}
		out = append(out, k)
	}
	return out
}

// EncodeHTTPHeaderKeys serializes header names for connection options.
func EncodeHTTPHeaderKeys(keys []string) string {
	norm := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		lk := strings.ToLower(k)
		if _, ok := seen[lk]; ok {
			continue
		}
		seen[lk] = struct{}{}
		norm = append(norm, k)
	}
	if len(norm) == 0 {
		return "[]"
	}
	b, err := json.Marshal(norm)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// ParseHeaderSecretValues parses the secret payload for custom headers mode.
func ParseHeaderSecretValues(secret string) map[string]string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(secret), &m); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}

// EncodeHeaderSecretValues serializes header values for secure storage.
func EncodeHeaderSecretValues(values map[string]string) string {
	if len(values) == 0 {
		return "{}"
	}
	b, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// MergeHeaderSecrets merges newly provided header values into the existing secret map.
// Empty new values leave the existing secret for that key unchanged.
func MergeHeaderSecrets(existingSecret string, keepKeys []string, updates map[string]string) string {
	current := ParseHeaderSecretValues(existingSecret)
	keep := make(map[string]string, len(keepKeys))
	updateByLower := map[string]string{}
	for k, v := range updates {
		updateByLower[strings.ToLower(strings.TrimSpace(k))] = v
	}
	for _, key := range keepKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		lk := strings.ToLower(key)
		if nv, ok := updateByLower[lk]; ok && strings.TrimSpace(nv) != "" {
			keep[key] = nv
			continue
		}
		if v, ok := current[key]; ok {
			keep[key] = v
			continue
		}
		for ek, ev := range current {
			if strings.EqualFold(ek, key) {
				keep[key] = ev
				break
			}
		}
	}
	return EncodeHeaderSecretValues(keep)
}

// MergeHeaderSecretJSON merges a JSON object of new header values into the existing secret JSON.
func MergeHeaderSecretJSON(existingJSON, updatesJSON string, keepKeys []string) string {
	return MergeHeaderSecrets(existingJSON, keepKeys, ParseHeaderSecretValues(updatesJSON))
}

// NormalizeAuthOptions writes canonical auth-related options for persistence.
func NormalizeAuthOptions(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.Options == nil {
		cfg.Options = map[string]string{}
	}
	mode := ConnectionMode(*cfg)
	if mode != ModeFiles {
		delete(cfg.Options, OptionAuthType)
		delete(cfg.Options, OptionHTTPHeaderKeys)
		delete(cfg.Options, OptionSourceType)
		cfg.Username = ""
		return
	}
	cfg.Options[OptionSourceType] = SourceTypeHTTP
	auth := AuthType(*cfg)
	cfg.Options[OptionAuthType] = auth
	switch auth {
	case AuthBasic:
		cfg.Options[OptionHTTPHeaderKeys] = "[]"
	case AuthBearer:
		cfg.Username = ""
		cfg.Options[OptionHTTPHeaderKeys] = "[]"
	case AuthHeaders:
		cfg.Username = ""
		cfg.Options[OptionHTTPHeaderKeys] = EncodeHTTPHeaderKeys(HTTPHeaderKeys(*cfg))
	default:
		cfg.Username = ""
		cfg.Options[OptionAuthType] = AuthNone
		cfg.Options[OptionHTTPHeaderKeys] = "[]"
	}
}

// HTTPAuthHeaders builds the HTTP headers DuckDB should send for this connection.
func HTTPAuthHeaders(cfg Config) (map[string]string, error) {
	switch AuthType(cfg) {
	case AuthNone:
		return nil, nil
	case AuthBasic:
		user := strings.TrimSpace(cfg.Username)
		pass := cfg.Password
		if user == "" || pass == "" {
			return nil, fmt.Errorf("Basic Auth requires a username and password")
		}
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		return map[string]string{"Authorization": "Basic " + token}, nil
	case AuthBearer:
		token := strings.TrimSpace(cfg.Password)
		if token == "" {
			return nil, fmt.Errorf("Bearer Token is required")
		}
		return map[string]string{"Authorization": "Bearer " + token}, nil
	case AuthHeaders:
		keys := HTTPHeaderKeys(cfg)
		if len(keys) == 0 {
			return nil, fmt.Errorf("at least one custom HTTP header is required")
		}
		values := ParseHeaderSecretValues(cfg.Password)
		out := make(map[string]string, len(keys))
		missing := make([]string, 0)
		for _, k := range keys {
			v := ""
			if x, ok := values[k]; ok {
				v = x
			} else {
				for ek, ev := range values {
					if strings.EqualFold(ek, k) {
						v = ev
						break
					}
				}
			}
			if strings.TrimSpace(v) == "" {
				missing = append(missing, k)
				continue
			}
			out[k] = v
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("missing secret value for header(s): %s", strings.Join(missing, ", "))
		}
		return out, nil
	default:
		return nil, nil
	}
}

// BearerToken returns the bearer token when auth type is bearer.
func BearerToken(cfg Config) string {
	if AuthType(cfg) != AuthBearer {
		return ""
	}
	return strings.TrimSpace(cfg.Password)
}

// AuthScopeURL returns a DuckDB secret SCOPE prefix for the remote source.
func AuthScopeURL(source string) string {
	source = strings.TrimSpace(source)
	u, err := url.Parse(source)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + u.Host + "/"
}

// RedactRemoteURL removes userinfo and query/fragment values for safe display and logs.
func RedactRemoteURL(raw string) string {
	return RedactURL(raw)
}

// SanitizeRemoteError redacts credentials and sensitive URL query parameters from an error.
func SanitizeRemoteError(err error) error {
	if err == nil {
		return nil
	}
	return FormatHTTPAuthError(err)
}

// RedactURL removes userinfo and query/fragment values for safe display and logs.
func RedactURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			return raw[:i] + "?…"
		}
		return raw
	}
	u.User = nil
	if u.RawQuery != "" || u.Fragment != "" {
		u.RawQuery = ""
		u.Fragment = ""
		return u.String() + "?…"
	}
	return u.String()
}

// SanitizeErrorMessage redacts credentials and sensitive URL query parameters from error text.
func SanitizeErrorMessage(msg string, secrets ...string) string {
	out := msg
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if s == "" || len(s) < 4 {
			continue
		}
		out = strings.ReplaceAll(out, s, "********")
	}
	return redactURLsInText(out)
}

func redactURLsInText(s string) string {
	lower := strings.ToLower(s)
	var b strings.Builder
	i := 0
	for i < len(s) {
		idx := strings.Index(lower[i:], "http://")
		idxs := strings.Index(lower[i:], "https://")
		next := -1
		if idx >= 0 && (idxs < 0 || idx < idxs) {
			next = i + idx
		} else if idxs >= 0 {
			next = i + idxs
		}
		if next < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i:next])
		end := next
		for end < len(s) {
			c := s[end]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '"' || c == '\'' || c == ')' || c == ',' || c == ';' || c == '<' || c == '>' {
				break
			}
			end++
		}
		b.WriteString(RedactURL(s[next:end]))
		i = end
	}
	return b.String()
}

// FormatHTTPAuthError maps transport / auth failures to a safe user-facing message.
func FormatHTTPAuthError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := SanitizeErrorMessage(err.Error(), secrets...)
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		return fmt.Errorf("Remote source returned HTTP 401 Unauthorized")
	case strings.Contains(lower, "403") || strings.Contains(lower, "forbidden"):
		return fmt.Errorf("Remote source returned HTTP 403 Forbidden")
	case strings.Contains(lower, "404") || strings.Contains(lower, "not found"):
		return fmt.Errorf("Remote source returned HTTP 404 Not Found")
	default:
		return fmt.Errorf("%s", msg)
	}
}

// CollectSecretSnippets returns credential strings that must never appear in logs/errors.
func CollectSecretSnippets(cfg Config) []string {
	var out []string
	if p := strings.TrimSpace(cfg.Password); p != "" {
		out = append(out, p)
		for _, v := range ParseHeaderSecretValues(p) {
			if strings.TrimSpace(v) != "" {
				out = append(out, v)
			}
		}
	}
	if AuthType(cfg) == AuthBearer {
		if t := strings.TrimSpace(cfg.Password); t != "" {
			out = append(out, "Bearer "+t)
		}
	}
	if AuthType(cfg) == AuthBasic {
		user := strings.TrimSpace(cfg.Username)
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + cfg.Password))
		out = append(out, "Basic "+token, token)
	}
	return out
}

// AuthDisplayLabel returns a non-sensitive auth label for datasource lists.
func AuthDisplayLabel(cfg Config) string {
	if ConnectionMode(cfg) != ModeFiles {
		return "—"
	}
	switch AuthType(cfg) {
	case AuthBasic:
		return "Basic Auth"
	case AuthBearer:
		return "Bearer Token"
	case AuthHeaders:
		return "Custom Headers"
	default:
		return "None"
	}
}

// ListSubtitle returns a non-sensitive datasource list subtitle.
func ListSubtitle(cfg Config) string {
	if ConnectionMode(cfg) == ModeFiles {
		entries := FileEntries(cfg)
		if len(entries) == 0 {
			return "DuckDB · Data Files"
		}
		_, resolved, err := ReaderFunctionForEntry(entries[0])
		if err != nil {
			return "DuckDB · Remote File"
		}
		return "DuckDB · Remote " + FormatDisplayName(resolved)
	}
	path := DatabasePath(cfg)
	if path == "" {
		return "DuckDB"
	}
	base := path
	base = strings.ReplaceAll(base, `\`, `/`)
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	return "DuckDB · " + base
}
