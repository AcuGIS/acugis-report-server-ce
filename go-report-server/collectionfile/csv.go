package collectionfile

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// ParseDelimiter resolves a delimiter name.
// Names are comma, tab, semicolon, pipe, and auto.
func ParseDelimiter(name string) (delim rune, auto bool, err error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto", "detect":
		return 0, true, nil
	case "comma", ",":
		return ',', false, nil
	case "tab", "tsv", "\\t", "\t":
		return '\t', false, nil
	case "semicolon", ";":
		return ';', false, nil
	case "pipe", "|":
		return '|', false, nil
	default:
		return 0, false, fmt.Errorf("unsupported CSV delimiter %q (use comma, tab, semicolon, pipe, or auto)", name)
	}
}

// DelimiterName is the stable option stored on a connection and in JRXML.
func DelimiterName(delim rune, auto bool) string {
	if auto {
		return "auto"
	}
	switch delim {
	case '\t':
		return "tab"
	case ';':
		return "semicolon"
	case '|':
		return "pipe"
	default:
		return "comma"
	}
}

// ParseCSV reads a headered CSV, TSV, or other single-byte delimited text file.
// Empty unquoted cells and the unquoted tokens NULL and \N are null.
// A quoted empty cell is a blank string. Those two results are not interchangeable.
func ParseCSV(r io.Reader, opt Options) (*Table, error) {
	if r == nil {
		return nil, fmt.Errorf("CSV file is empty")
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("CSV file exceeds the %d byte limit", MaxBytes)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("CSV file has no header row")
	}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("CSV file has no header row")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("CSV file is not valid UTF-8")
	}
	delim := opt.Delimiter
	if opt.Auto {
		delim = detectDelimiter(data)
	} else if delim == 0 {
		delim = ','
	}
	if delim > 0xFF {
		return nil, fmt.Errorf("CSV delimiter must be a single byte")
	}
	records, err := readRecords(data, delim)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("CSV file has no header row")
	}
	flags, err := quoteFlags(data, delim)
	if err != nil {
		return nil, err
	}
	if len(flags) != len(records) {
		return nil, fmt.Errorf("CSV quote scan did not match the parsed rows")
	}
	header := records[0]
	if len(header) == 0 || (len(header) == 1 && strings.TrimSpace(header[0]) == "" && len(records) == 1) {
		return nil, fmt.Errorf("CSV file has no header row")
	}
	names := make([]string, len(header))
	seen := map[string]int{}
	for i, h := range header {
		name := strings.TrimSpace(h)
		if name == "" {
			return nil, fmt.Errorf("CSV header column %d is empty", i+1)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("CSV header %q is duplicated", name)
		}
		seen[name] = i
		names[i] = name
	}
	cols := make([][]cell, len(names))
	for i := range cols {
		cols[i] = make([]cell, 0, len(records)-1)
	}
	for recNo, rec := range records[1:] {
		if len(rec) == 1 && rec[0] == "" && len(flags) > recNo+1 && len(flags[recNo+1]) == 1 && !flags[recNo+1][0] {
			// A blank line is a malformed row, not a skipped record.
		}
		if len(rec) != len(names) {
			return nil, fmt.Errorf("CSV row %d has %d fields, header has %d", recNo+2, len(rec), len(names))
		}
		if len(flags[recNo+1]) != len(rec) {
			return nil, fmt.Errorf("CSV row %d quote scan did not match the parsed fields", recNo+2)
		}
		for i, raw := range rec {
			cols[i] = append(cols[i], classifyCell(raw, flags[recNo+1][i]))
		}
	}
	table := &Table{
		Columns:   make([]Column, len(names)),
		Rows:      make([]map[string]any, len(records)-1),
		Delimiter: delim,
	}
	kinds := make([]Kind, len(names))
	for i, name := range names {
		kind := widenKind(inferColumnKind(cols[i], opt.SampleLimit), cols[i])
		kinds[i] = kind
		table.Columns[i] = Column{Name: name, Kind: kind}
	}
	for r := range table.Rows {
		row := make(map[string]any, len(names))
		for c, name := range names {
			row[name] = finalizeCell(kinds[c], cols[c][r])
		}
		table.Rows[r] = row
	}
	return table, nil
}

func readRecords(data []byte, delim rune) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.ReuseRecord = false
	r.LazyQuotes = false
	var out [][]string
	line := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("CSV row %d: %w", line, err)
		}
		cp := make([]string, len(rec))
		copy(cp, rec)
		out = append(out, cp)
	}
	return out, nil
}

func detectDelimiter(data []byte) rune {
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
		if i := bytes.LastIndexByte(sample, '\n'); i > 0 {
			sample = sample[:i]
		}
	}
	candidates := []rune{',', '\t', ';', '|'}
	best := ','
	bestScore := -1
	for _, d := range candidates {
		r := csv.NewReader(bytes.NewReader(sample))
		r.Comma = d
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		r.ReuseRecord = false
		var counts []int
		for n := 0; n < 20; n++ {
			rec, err := r.Read()
			if err != nil {
				break
			}
			if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
				continue
			}
			counts = append(counts, len(rec))
		}
		if len(counts) == 0 {
			continue
		}
		consistent := true
		for _, c := range counts[1:] {
			if c != counts[0] {
				consistent = false
				break
			}
		}
		score := 0
		if consistent && counts[0] > 1 {
			score = counts[0]*100 + len(counts)
		} else if counts[0] > 1 {
			score = counts[0]
		}
		if score > bestScore {
			bestScore = score
			best = d
		}
	}
	return best
}

// quoteFlags reports whether each field encoding/csv returned was quoted.
// Quoted empty fields are blank strings. Unquoted empty fields are null.
func quoteFlags(data []byte, comma rune) ([][]bool, error) {
	s := string(data)
	n := len(s)
	i := 0
	var rows [][]bool
	for i < n {
		var flags []bool
		for {
			if i >= n {
				break
			}
			if s[i] == '"' {
				flags = append(flags, true)
				i++
				closed := false
				for i < n {
					if s[i] == '"' {
						if i+1 < n && s[i+1] == '"' {
							i += 2
							continue
						}
						i++
						closed = true
						break
					}
					i++
				}
				if !closed {
					return nil, fmt.Errorf("CSV quote scan: unterminated quoted field")
				}
			} else {
				flags = append(flags, false)
				for i < n && rune(s[i]) != comma && s[i] != '\n' && s[i] != '\r' {
					if s[i] == '"' {
						return nil, fmt.Errorf("CSV quote scan: bare quote")
					}
					i++
				}
			}
			if i >= n {
				break
			}
			if rune(s[i]) == comma {
				i++
				continue
			}
			if s[i] == '\r' {
				i++
				if i < n && s[i] == '\n' {
					i++
				}
			} else if s[i] == '\n' {
				i++
			} else {
				return nil, fmt.Errorf("CSV quote scan: unexpected byte")
			}
			break
		}
		if len(flags) == 0 {
			break
		}
		rows = append(rows, flags)
	}
	return rows, nil
}
