package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// configureHTTPAuth installs a temporary DuckDB HTTP secret for the session.
// Uses DuckDB Secrets Manager (TYPE HTTP) — never embeds credentials in URLs or SQL queries.
// The CREATE SECRET statement is never logged.
func configureHTTPAuth(ctx context.Context, db *sql.DB, cfg Config) error {
	auth := AuthType(cfg)
	if auth == AuthNone {
		return nil
	}

	entries := FileEntries(cfg)
	scope := ""
	if len(entries) > 0 {
		scope = AuthScopeURL(entries[0].RemoteSource())
	}

	var secretSQL string
	var err error
	switch auth {
	case AuthBearer:
		token := BearerToken(cfg)
		if token == "" {
			return fmt.Errorf("Bearer Token is required")
		}
		secretSQL, err = buildBearerSecretSQL(token, scope)
	case AuthBasic, AuthHeaders:
		headers, herr := HTTPAuthHeaders(cfg)
		if herr != nil {
			return herr
		}
		if len(headers) == 0 {
			return nil
		}
		secretSQL, err = buildHeadersSecretSQL(headers, scope)
	default:
		return nil
	}
	if err != nil {
		return err
	}

	_, _ = db.ExecContext(ctx, "DROP SECRET IF EXISTS "+duckHTTPSecretName)

	if _, err := db.ExecContext(ctx, secretSQL); err != nil {
		return FormatHTTPAuthError(fmt.Errorf("failed to configure HTTP authentication: %w", err), CollectSecretSnippets(cfg)...)
	}
	return nil
}

func buildBearerSecretSQL(token, scope string) (string, error) {
	var b strings.Builder
	b.WriteString("CREATE OR REPLACE SECRET ")
	b.WriteString(duckHTTPSecretName)
	b.WriteString(" (TYPE HTTP, BEARER_TOKEN ")
	b.WriteString(sqlStringLiteral(token))
	if scope != "" {
		b.WriteString(", SCOPE ")
		b.WriteString(sqlStringLiteral(scope))
	}
	b.WriteString(")")
	return b.String(), nil
}

func buildHeadersSecretSQL(headers map[string]string, scope string) (string, error) {
	if len(headers) == 0 {
		return "", fmt.Errorf("no HTTP headers to configure")
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var mapParts []string
	for _, k := range keys {
		v := headers[k]
		mapParts = append(mapParts, fmt.Sprintf("%s: %s", sqlStringLiteral(k), sqlStringLiteral(v)))
	}

	var b strings.Builder
	b.WriteString("CREATE OR REPLACE SECRET ")
	b.WriteString(duckHTTPSecretName)
	b.WriteString(" (TYPE HTTP, EXTRA_HTTP_HEADERS MAP {")
	b.WriteString(strings.Join(mapParts, ", "))
	b.WriteString("}")
	if scope != "" {
		b.WriteString(", SCOPE ")
		b.WriteString(sqlStringLiteral(scope))
	}
	b.WriteString(")")
	return b.String(), nil
}
