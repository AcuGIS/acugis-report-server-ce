package spatial

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrNotMapReport    = errors.New("not a map report")
	ErrInvalidReport   = errors.New("invalid report")
	ErrLayerUnresolved = errors.New("map layer could not be resolved")
	ErrQueryFailed     = errors.New("map query failed")
)

var (
	safeIdentRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	safeFieldKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	// Allow ", `, or [] quoting around each part of a Jasper column ref.
	jasperColumnRE = regexp.MustCompile(`^(?:(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)\.){0,2}(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)$`)
)

// IsSafeSQLIdent mirrors PHP jrs_is_safe_sql_ident.
func IsSafeSQLIdent(value string) bool {
	return safeIdentRE.MatchString(value)
}

// IsSafeFieldKey mirrors PHP jrs_is_safe_field_key.
func IsSafeFieldKey(value string) bool {
	return safeFieldKeyRE.MatchString(value)
}

// IsSafeJasperColumnRef mirrors PHP jrs_is_safe_jasper_column_ref.
func IsSafeJasperColumnRef(column string) bool {
	column = strings.TrimSpace(column)
	if column == "" {
		return false
	}
	return jasperColumnRE.MatchString(column)
}

// QuoteSQLIdent mirrors PHP jrs_quote_sql_ident.
func QuoteSQLIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
