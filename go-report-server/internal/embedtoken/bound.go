package embedtoken

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

const (
	// MaxBoundParameters is the maximum number of bound parameters on one token.
	MaxBoundParameters = 32
	// MaxBoundNameLen is the maximum length of a bound parameter name.
	MaxBoundNameLen = 64
	// MaxBoundValues is the maximum number of values for one bound parameter.
	MaxBoundValues = 32
	// MaxBoundValueLen is the maximum length of one bound parameter value.
	MaxBoundValueLen = 2048
)

// BoundParameterError describes a rejected bound parameter on mint or issue.
type BoundParameterError struct {
	Name   string
	Reason string
}

func (e *BoundParameterError) Error() string {
	if e == nil {
		return "invalid bound parameter"
	}
	if e.Name == "" {
		return "invalid bound parameter: " + e.Reason
	}
	return "invalid bound parameter " + e.Name + ": " + e.Reason
}

// ParseMintParameters decodes the POST /api/v1/embed/token "parameters" object.
// A missing field, JSON null, or empty object means no bound parameters.
//
// Each value must be a JSON string, number, boolean, or an array of those.
// Names and values are then checked by NormalizeBoundParameters.
func ParseMintParameters(raw json.RawMessage) (map[string][]string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, &BoundParameterError{Reason: "parameters must be a JSON object"}
	}
	if err := dec.Decode(&struct{}{}); err != nil && !errors.Is(err, io.EOF) {
		return nil, &BoundParameterError{Reason: "parameters must be a single JSON object"}
	}
	if len(obj) == 0 {
		return nil, nil
	}
	out := make(map[string][]string, len(obj))
	for name, value := range obj {
		vals, err := coerceBoundValue(value)
		if err != nil {
			return nil, &BoundParameterError{Name: strings.TrimSpace(name), Reason: err.Error()}
		}
		out[name] = vals
	}
	return NormalizeBoundParameters(out)
}

// NormalizeBoundParameters validates and copies bound parameters.
// A nil or empty map returns nil, nil (no bound parameters).
//
// A name is rejected when any of the following is true:
//   - it is empty or longer than MaxBoundNameLen
//   - it is not an identifier matching ^[A-Za-z][A-Za-z0-9_]*$
//   - it starts with "_" (Jasper/transport keys such as _repName, _repFormat, _dataSource)
//   - it is a reserved transport name: datasource, report, et, embed, toolbar,
//     parameters, export, zoom, close, autoresize, pagination (case-insensitive)
//   - it is a Jasper built-in: any name starting with "REPORT_", or
//     REPORT_PARAMETERS_MAP, REPORT_CONNECTION, REPORT_DATA_SOURCE, REPORT_MAX_COUNT,
//     REPORT_LOCALE, REPORT_RESOURCE_BUNDLE, REPORT_TIME_ZONE, REPORT_FORMAT_FACTORY,
//     REPORT_CLASS_LOADER, REPORT_URL_HANDLER_FACTORY, REPORT_FILE_RESOLVER,
//     REPORT_SCRIPTLET, REPORT_VIRTUALIZER, REPORT_TEMPLATES, SORT_FIELDS, FILTER,
//     REPORT_DIR, SUBREPORT_DIR, P_MAP_IMAGE (case-insensitive)
//
// Values must be non-empty as a list (at least one), at most MaxBoundValues,
// and each value at most MaxBoundValueLen. An empty string is a legal value.
// There may be at most MaxBoundParameters names.
func NormalizeBoundParameters(in map[string][]string) (map[string][]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > MaxBoundParameters {
		return nil, &BoundParameterError{Reason: fmt.Sprintf("at most %d bound parameters are allowed", MaxBoundParameters)}
	}
	out := make(map[string][]string, len(in))
	for rawName, vals := range in {
		name := strings.TrimSpace(rawName)
		if err := validateBoundName(name); err != nil {
			return nil, err
		}
		if _, exists := out[name]; exists {
			return nil, &BoundParameterError{Name: name, Reason: "duplicate parameter"}
		}
		if len(vals) == 0 {
			return nil, &BoundParameterError{Name: name, Reason: "at least one value is required"}
		}
		if len(vals) > MaxBoundValues {
			return nil, &BoundParameterError{Name: name, Reason: fmt.Sprintf("at most %d values are allowed", MaxBoundValues)}
		}
		cp := make([]string, len(vals))
		for i, v := range vals {
			if len(v) > MaxBoundValueLen {
				return nil, &BoundParameterError{Name: name, Reason: "value is too long"}
			}
			cp[i] = v
		}
		out[name] = cp
	}
	return out, nil
}

func validateBoundName(name string) error {
	if name == "" {
		return &BoundParameterError{Reason: "parameter name is empty"}
	}
	if len(name) > MaxBoundNameLen {
		return &BoundParameterError{Name: name, Reason: "name is too long"}
	}
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		case r == '_' && i > 0:
		case r >= '0' && r <= '9' && i > 0:
		default:
			return &BoundParameterError{Name: name, Reason: "name must match ^[A-Za-z][A-Za-z0-9_]*$"}
		}
	}
	if strings.HasPrefix(name, "_") {
		return &BoundParameterError{Name: name, Reason: "system parameters cannot be bound"}
	}
	upper := strings.ToUpper(name)
	if _, ok := reservedBoundNames[upper]; ok || strings.HasPrefix(upper, "REPORT_") {
		return &BoundParameterError{Name: name, Reason: "system parameters cannot be bound"}
	}
	return nil
}

// reservedBoundNames are transport and Jasper built-in names, uppercased.
var reservedBoundNames = map[string]struct{}{
	"DATASOURCE":                 {},
	"REPORT":                     {},
	"ET":                         {},
	"EMBED":                      {},
	"TOOLBAR":                    {},
	"PARAMETERS":                 {},
	"EXPORT":                     {},
	"ZOOM":                       {},
	"CLOSE":                      {},
	"AUTORESIZE":                 {},
	"PAGINATION":                 {},
	"REPORT_PARAMETERS_MAP":      {},
	"REPORT_CONNECTION":          {},
	"REPORT_DATA_SOURCE":         {},
	"REPORT_MAX_COUNT":           {},
	"REPORT_LOCALE":              {},
	"REPORT_RESOURCE_BUNDLE":     {},
	"REPORT_TIME_ZONE":           {},
	"REPORT_FORMAT_FACTORY":      {},
	"REPORT_CLASS_LOADER":        {},
	"REPORT_URL_HANDLER_FACTORY": {},
	"REPORT_FILE_RESOLVER":       {},
	"REPORT_SCRIPTLET":           {},
	"REPORT_VIRTUALIZER":         {},
	"REPORT_TEMPLATES":           {},
	"SORT_FIELDS":                {},
	"FILTER":                     {},
	"REPORT_DIR":                 {},
	"SUBREPORT_DIR":              {},
	"P_MAP_IMAGE":                {},
}

func coerceBoundValue(v any) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, fmt.Errorf("null is not allowed")
	case string, bool, json.Number:
		s, err := coerceBoundScalar(t)
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	case []any:
		if len(t) == 0 {
			return nil, fmt.Errorf("at least one value is required")
		}
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, err := coerceBoundScalar(item)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("must be a string, number, boolean, or array of those")
	}
}

func coerceBoundScalar(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case json.Number:
		s := t.String()
		if s == "" || strings.IndexFunc(s, unicode.IsSpace) >= 0 {
			return "", fmt.Errorf("invalid number")
		}
		return s, nil
	case nil:
		return "", fmt.Errorf("null is not allowed")
	default:
		return "", fmt.Errorf("must be a string, number, or boolean")
	}
}
