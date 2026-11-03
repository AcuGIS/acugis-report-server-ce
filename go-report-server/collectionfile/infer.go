package collectionfile

import (
	"strconv"
	"strings"
	"time"
	"unicode"
)

// cell is one parsed field before the column kind is fixed.
type cell struct {
	raw    string
	quoted bool
	kind   Kind
	value  any
	skip   bool // null: does not vote on the column type
}

func classifyCell(raw string, quoted bool) cell {
	c := cell{raw: raw, quoted: quoted}
	if quoted && raw == "" {
		c.kind = KindString
		c.value = ""
		c.skip = true
		return c
	}
	if !quoted && (raw == "" || raw == `\N` || strings.EqualFold(raw, "NULL")) {
		c.skip = true
		c.value = nil
		return c
	}
	if k, v, ok := typedToken(raw); ok {
		c.kind = k
		c.value = v
		return c
	}
	c.kind = KindString
	c.value = raw
	return c
}

func typedToken(raw string) (Kind, any, bool) {
	if raw == "" {
		return "", nil, false
	}
	switch strings.ToLower(raw) {
	case "true":
		return KindBoolean, true, true
	case "false":
		return KindBoolean, false, true
	}
	if t, ok := parseDateTime(raw); ok {
		if isDateOnly(raw) {
			return KindDate, Date{Time: t}, true
		}
		return KindDateTime, DateTime{Time: t}, true
	}
	if isIntegerToken(raw) {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			return KindInteger, n, true
		}
		return "", nil, false
	}
	if isDecimalToken(raw) {
		f, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return KindDecimal, f, true
		}
	}
	return "", nil, false
}

func isDateOnly(raw string) bool {
	if len(raw) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", raw)
	return err == nil
}

func parseDateTime(raw string) (time.Time, bool) {
	layouts := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func isIntegerToken(s string) bool {
	if s == "" || s == "+" || s == "-" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i++
	}
	if i >= len(s) {
		return false
	}
	if s[i] == '+' {
		return false
	}
	digits := s[i:]
	if digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	// Leading zeros are codes ("007"), not integers. Zero itself is an integer.
	if len(digits) > 1 && digits[0] == '0' {
		return false
	}
	return true
}

func isDecimalToken(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t") {
		return false
	}
	if !strings.ContainsAny(s, ".eE") {
		return false
	}
	// Reject locale forms such as 1,25 and ambiguous dates.
	for _, r := range s {
		if unicode.IsLetter(r) && r != 'e' && r != 'E' {
			return false
		}
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// mergeKind combines two non-null sample kinds.
// Integer and decimal become decimal. Date and datetime become datetime.
// Any other mix becomes string.
func mergeKind(current, next Kind) Kind {
	if current == "" {
		return next
	}
	if next == "" || current == next {
		return current
	}
	if current == KindString || next == KindString {
		return KindString
	}
	if (current == KindInteger && next == KindDecimal) || (current == KindDecimal && next == KindInteger) {
		return KindDecimal
	}
	if (current == KindDate && next == KindDateTime) || (current == KindDateTime && next == KindDate) {
		return KindDateTime
	}
	return KindString
}

func finalizeCell(final Kind, c cell) any {
	if c.skip && c.value == nil {
		return nil
	}
	if c.skip && c.kind == KindString && c.value == "" {
		if final == KindString {
			return ""
		}
		return nil
	}
	switch final {
	case KindString:
		if c.value == nil {
			return nil
		}
		return c.raw
	case KindInteger:
		if c.kind == KindInteger {
			return c.value
		}
		return nil
	case KindDecimal:
		switch c.kind {
		case KindDecimal:
			return c.value
		case KindInteger:
			return float64(c.value.(int64))
		default:
			return nil
		}
	case KindBoolean:
		if c.kind == KindBoolean {
			return c.value
		}
		return nil
	case KindDate:
		if c.kind == KindDate {
			return c.value
		}
		return nil
	case KindDateTime:
		switch c.kind {
		case KindDateTime:
			return c.value
		case KindDate:
			return DateTime{Time: c.value.(Date).Time}
		default:
			return nil
		}
	default:
		if c.value == nil {
			return nil
		}
		return c.raw
	}
}

func inferColumnKind(cells []cell, sampleLimit int) Kind {
	if sampleLimit <= 0 {
		sampleLimit = DefaultSampleLimit
	}
	var kind Kind
	seen := 0
	for _, c := range cells {
		if c.skip {
			continue
		}
		if seen >= sampleLimit {
			break
		}
		seen++
		kind = mergeKind(kind, c.kind)
		if kind == KindString {
			break
		}
	}
	if kind == "" {
		return KindString
	}
	return kind
}

// widen walks every cell so a value past the sample can still widen the column.
func widenKind(kind Kind, cells []cell) Kind {
	for _, c := range cells {
		if c.skip {
			continue
		}
		kind = mergeKind(kind, c.kind)
		if kind == KindString {
			return kind
		}
	}
	return kind
}
