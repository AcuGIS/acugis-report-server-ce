// Package excelfile reads one .xlsx workbook into typed row maps for the
// existing collection query evaluator. It does not calculate formulas, and it
// does not interpret worksheets as a SQL database.
//
// Hidden and very-hidden worksheets are omitted. A visible sheet is listed when
// it contains any cell value. Header validation runs when that sheet is read.
//
// A formula cell contributes its cached result. A formula with no cached
// result is an error. This package does not calculate formulas.
package excelfile

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"github.com/xuri/excelize/v2"
)

const (
	// MaxFileBytes is the largest compressed workbook that will be opened.
	MaxFileBytes = 32 << 20
	// MaxUnzipBytes is the largest total uncompressed workbook content.
	MaxUnzipBytes = 64 << 20
	// MaxXMLBytes is the largest single workbook XML part.
	MaxXMLBytes = 32 << 20
	// MaxSheets is the most visible worksheets a workbook may contain.
	MaxSheets = 64
	// MaxColumns is the most columns a worksheet may use.
	MaxColumns = 256
	// MaxRows is the most worksheet rows that will be read, including the header.
	// Exceeding it is an error. Rows are not silently truncated.
	MaxRows = 100000
)

// Workbook is an open .xlsx document.
type Workbook struct {
	file     *excelize.File
	date1904 bool
}

// Open reads one workbook. size is the compressed byte length when known.
// A negative size means the length is not known and the reader is capped.
func Open(r io.Reader, size int64) (*Workbook, error) {
	if r == nil {
		return nil, fmt.Errorf("workbook is not a valid .xlsx file")
	}
	if size > MaxFileBytes {
		return nil, fmt.Errorf("Excel file exceeds the %d byte limit", MaxFileBytes)
	}
	var payload []byte
	var err error
	if size >= 0 {
		payload, err = io.ReadAll(io.LimitReader(r, size))
		if err != nil {
			return nil, fmt.Errorf("workbook is not a valid .xlsx file")
		}
	} else {
		payload, err = io.ReadAll(io.LimitReader(r, MaxFileBytes+1))
		if err != nil {
			return nil, fmt.Errorf("workbook is not a valid .xlsx file")
		}
		if len(payload) > MaxFileBytes {
			return nil, fmt.Errorf("Excel file exceeds the %d byte limit", MaxFileBytes)
		}
	}
	if len(payload) < 4 || payload[0] != 'P' || payload[1] != 'K' {
		return nil, fmt.Errorf("workbook is not a valid .xlsx file")
	}
	f, err := excelize.OpenReader(bytes.NewReader(payload), excelize.Options{
		UnzipSizeLimit:    MaxUnzipBytes,
		UnzipXMLSizeLimit: MaxXMLBytes,
	})
	if err != nil {
		return nil, openError(err)
	}
	wb := &Workbook{file: f}
	props, perr := f.GetWorkbookProps()
	if perr != nil {
		_ = f.Close()
		return nil, openError(perr)
	}
	if props.Date1904 != nil && *props.Date1904 {
		wb.date1904 = true
	}
	return wb, nil
}

// OpenFile opens a regular .xlsx file.
func OpenFile(path string) (*Workbook, error) {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("workbook is not a valid .xlsx file")
	}
	if st.Size() > MaxFileBytes {
		return nil, fmt.Errorf("Excel file exceeds the %d byte limit", MaxFileBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("workbook is not a valid .xlsx file")
	}
	defer f.Close()
	return Open(f, st.Size())
}

// Close releases the workbook.
func (w *Workbook) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// Sheets returns visible worksheets that contain at least one cell value.
// Hidden worksheets are omitted. The order is the workbook order.
func (w *Workbook) Sheets() ([]string, error) {
	if w == nil || w.file == nil {
		return nil, fmt.Errorf("workbook is not a valid .xlsx file")
	}
	names := w.file.GetSheetList()
	visible := make([]string, 0, len(names))
	for _, name := range names {
		ok, err := w.file.GetSheetVisible(name)
		if err != nil {
			return nil, fmt.Errorf("workbook is not a valid .xlsx file")
		}
		if !ok {
			continue
		}
		visible = append(visible, name)
	}
	if len(visible) > MaxSheets {
		return nil, fmt.Errorf("workbook exceeds the %d worksheet limit", MaxSheets)
	}
	if len(visible) == 0 {
		return nil, fmt.Errorf("workbook has no visible worksheets")
	}
	out := make([]string, 0, len(visible))
	for _, name := range visible {
		grid, err := readGrid(w.file, name)
		if err != nil {
			return nil, err
		}
		if !gridHasValue(grid) {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("workbook has no tabular worksheets")
	}
	return out, nil
}

// Table reads one worksheet into a typed collection table.
func (w *Workbook) Table(sheet string) (*collectionfile.Table, error) {
	if w == nil || w.file == nil {
		return nil, fmt.Errorf("workbook is not a valid .xlsx file")
	}
	sheet = strings.TrimSpace(sheet)
	if sheet == "" {
		return nil, fmt.Errorf("workbook has no worksheet")
	}
	visible, err := w.file.GetSheetVisible(sheet)
	if err != nil || !visible {
		return nil, fmt.Errorf("workbook has no worksheet %q", sheet)
	}
	grid, err := readGrid(w.file, sheet)
	if err != nil {
		return nil, err
	}
	if !gridHasValue(grid) {
		return nil, fmt.Errorf("worksheet %q has no tabular data", sheet)
	}
	merges, err := mergeRanges(w.file, sheet)
	if err != nil {
		return nil, fmt.Errorf("worksheet %q could not be read", sheet)
	}
	headerRow, header, err := headerColumns(sheet, grid, merges)
	if err != nil {
		return nil, err
	}
	if err := ambiguousHeader(sheet, headerRow, header, merges); err != nil {
		return nil, err
	}
	rows, kinds, err := w.readRecords(sheet, grid, headerRow, header)
	if err != nil {
		return nil, err
	}
	cols := make([]collectionfile.Column, len(header))
	for i, h := range header {
		kind := kinds[i]
		if kind == "" {
			kind = collectionfile.KindString
		}
		cols[i] = collectionfile.Column{Name: h.name, Kind: kind}
	}
	return &collectionfile.Table{Columns: cols, Rows: rows}, nil
}

// Select chooses the worksheet for a query. A FROM name that matches a sheet
// wins. Otherwise the configured sheet is used. An unknown name is an error.
func Select(sheets []string, configured, sql string) (string, error) {
	if len(sheets) == 0 {
		return "", fmt.Errorf("workbook has no tabular worksheets")
	}
	if name := SheetFromSQL(sql); name != "" {
		if found, ok := matchSheet(sheets, name); ok {
			return found, nil
		}
		return "", fmt.Errorf("workbook has no worksheet %q", name)
	}
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if found, ok := matchSheet(sheets, configured); ok {
			return found, nil
		}
		return "", fmt.Errorf("workbook has no worksheet %q", configured)
	}
	return sheets[0], nil
}

var fromSheetRE = regexp.MustCompile(`(?is)\bfrom\s+(?:"(?:[^"]|"")*"\s*\.\s*)?(?:"((?:[^"]|"")*)"|([A-Za-z_][A-Za-z0-9_]*))`)

// SheetFromSQL returns the worksheet named by the last FROM identifier.
// Schema qualifiers are ignored. An empty result means the query has no table.
func SheetFromSQL(sql string) string {
	m := fromSheetRE.FindStringSubmatch(sql)
	if m == nil {
		return ""
	}
	name := m[1]
	if name == "" {
		name = m[2]
	}
	return strings.ReplaceAll(strings.TrimSpace(name), `""`, `"`)
}

func matchSheet(sheets []string, name string) (string, bool) {
	for _, sheet := range sheets {
		if sheet == name {
			return sheet, true
		}
	}
	var folded string
	for _, sheet := range sheets {
		if strings.EqualFold(sheet, name) {
			if folded != "" && folded != sheet {
				return "", false
			}
			folded = sheet
		}
	}
	if folded != "" {
		return folded, true
	}
	return "", false
}

func openError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "password") || strings.Contains(msg, "encrypt"):
		return fmt.Errorf("password-protected Excel workbooks are not supported")
	case strings.Contains(msg, "unzip size exceeds"):
		return fmt.Errorf("Excel file exceeds the %d byte limit", MaxUnzipBytes)
	default:
		return fmt.Errorf("workbook is not a valid .xlsx file")
	}
}

func readGrid(f *excelize.File, sheet string) ([][]string, error) {
	rows, err := f.Rows(sheet)
	if err != nil {
		return nil, fmt.Errorf("worksheet %q could not be read", sheet)
	}
	grid := make([][]string, 0, 32)
	excelRow := 0
	var rowsErr error
	for rows.Next() {
		excelRow++
		if excelRow > MaxRows {
			rowsErr = fmt.Errorf("worksheet exceeds the %d row limit", MaxRows)
			break
		}
		cols, cerr := rows.Columns(excelize.Options{RawCellValue: true})
		if cerr != nil {
			rowsErr = fmt.Errorf("worksheet %q could not be read", sheet)
			break
		}
		if len(cols) > MaxColumns {
			rowsErr = fmt.Errorf("worksheet exceeds the %d column limit", MaxColumns)
			break
		}
		for len(grid) < excelRow-1 {
			grid = append(grid, nil)
		}
		grid = append(grid, cols)
	}
	if err := rows.Error(); err != nil && rowsErr == nil {
		rowsErr = fmt.Errorf("worksheet %q could not be read", sheet)
	}
	_ = rows.Close()
	if rowsErr != nil {
		return nil, rowsErr
	}
	if err := rejectBlankFormulas(f, sheet, grid); err != nil {
		return nil, err
	}
	for len(grid) > 0 && rowEmpty(grid[len(grid)-1]) {
		grid = grid[:len(grid)-1]
	}
	return grid, nil
}

func rejectBlankFormulas(f *excelize.File, sheet string, grid [][]string) error {
	for i, row := range grid {
		for c, raw := range row {
			if raw != "" {
				continue
			}
			cell, err := excelize.CoordinatesToCellName(c+1, i+1)
			if err != nil {
				return fmt.Errorf("worksheet %q could not be read", sheet)
			}
			formula, err := f.GetCellFormula(sheet, cell)
			if err != nil {
				return fmt.Errorf("worksheet %q could not be read", sheet)
			}
			if strings.TrimSpace(formula) != "" {
				return fmt.Errorf("worksheet %q cell %s has a formula with no cached result", sheet, cell)
			}
		}
	}
	return nil
}

func gridHasValue(grid [][]string) bool {
	for _, row := range grid {
		if !rowEmpty(row) {
			return true
		}
	}
	return false
}

func rowEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

type merge struct {
	r1, c1, r2, c2 int
}

func mergeRanges(f *excelize.File, sheet string) ([]merge, error) {
	cells, err := f.GetMergeCells(sheet, true)
	if err != nil {
		return nil, err
	}
	out := make([]merge, 0, len(cells))
	for _, cell := range cells {
		ref := ""
		if len(cell) > 0 {
			ref = cell[0]
		}
		parts := strings.Split(ref, ":")
		if len(parts) != 2 {
			continue
		}
		c1, r1, err1 := excelize.CellNameToCoordinates(parts[0])
		c2, r2, err2 := excelize.CellNameToCoordinates(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}
		if c1 > c2 {
			c1, c2 = c2, c1
		}
		if r1 > r2 {
			r1, r2 = r2, r1
		}
		out = append(out, merge{r1: r1, c1: c1, r2: r2, c2: c2})
	}
	return out, nil
}

type headerCol struct {
	name string
	col  int // 0-based
}

func headerColumns(sheet string, grid [][]string, merges []merge) (int, []headerCol, error) {
	for i, row := range grid {
		if rowEmpty(row) {
			continue
		}
		excelRow := i + 1
		if title, ok := titleRow(excelRow, row, merges); ok {
			if !hasLaterValue(grid, i) {
				return 0, nil, fmt.Errorf("worksheet %q has no tabular data", sheet)
			}
			_ = title
			continue
		}
		cols, err := headersFromRow(sheet, excelRow, row, merges)
		if err != nil {
			return 0, nil, err
		}
		return excelRow, cols, nil
	}
	return 0, nil, fmt.Errorf("worksheet %q has no tabular data", sheet)
}

func hasLaterValue(grid [][]string, index int) bool {
	for _, row := range grid[index+1:] {
		if !rowEmpty(row) {
			return true
		}
	}
	return false
}

func titleRow(excelRow int, row []string, merges []merge) (merge, bool) {
	nonEmpty := -1
	count := 0
	for i, cell := range row {
		if strings.TrimSpace(cell) == "" {
			continue
		}
		count++
		nonEmpty = i
	}
	if count != 1 || nonEmpty < 0 {
		return merge{}, false
	}
	col := nonEmpty + 1
	for _, m := range merges {
		if m.r1 == excelRow && m.r2 == excelRow && m.c1 == col && m.c2 > m.c1 {
			return m, true
		}
	}
	return merge{}, false
}

func headersFromRow(sheet string, excelRow int, row []string, merges []merge) ([]headerCol, error) {
	first, last := -1, -1
	for i, cell := range row {
		if strings.TrimSpace(cell) == "" {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	if first < 0 {
		return nil, fmt.Errorf("worksheet %q has no tabular data", sheet)
	}
	out := make([]headerCol, 0, last-first+1)
	seen := map[string]struct{}{}
	for i := first; i <= last; i++ {
		name := strings.TrimSpace(row[i])
		if name == "" {
			if mergeMakesHeaderAmbiguous(merges, excelRow, i+1) {
				return nil, fmt.Errorf("worksheet %q has merged cells that make the header ambiguous", sheet)
			}
			return nil, fmt.Errorf("worksheet %q has a blank header", sheet)
		}
		if !utf8.ValidString(name) {
			return nil, fmt.Errorf("worksheet %q has a header that is not valid text", sheet)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("worksheet %q has duplicate header %q", sheet, name)
		}
		seen[name] = struct{}{}
		out = append(out, headerCol{name: name, col: i})
	}
	return out, nil
}

func mergeMakesHeaderAmbiguous(merges []merge, excelRow, col int) bool {
	for _, m := range merges {
		if excelRow < m.r1 || excelRow > m.r2 || col < m.c1 || col > m.c2 {
			continue
		}
		if m.c1 != m.c2 || m.r1 != m.r2 {
			return true
		}
	}
	return false
}

func ambiguousHeader(sheet string, headerRow int, header []headerCol, merges []merge) error {
	if len(header) == 0 {
		return fmt.Errorf("worksheet %q has no tabular data", sheet)
	}
	minCol := header[0].col + 1
	maxCol := header[len(header)-1].col + 1
	for _, m := range merges {
		if headerRow < m.r1 || headerRow > m.r2 {
			continue
		}
		overlaps := m.c2 >= minCol && m.c1 <= maxCol
		if !overlaps {
			continue
		}
		wide := m.c1 != m.c2
		tall := m.r1 != m.r2
		if wide || tall {
			return fmt.Errorf("worksheet %q has merged cells that make the header ambiguous", sheet)
		}
	}
	return nil
}

type cellValue struct {
	kind  collectionfile.Kind
	value any
	blank bool
}

func (w *Workbook) readRecords(sheet string, grid [][]string, headerRow int, header []headerCol) ([]map[string]any, []collectionfile.Kind, error) {
	samples := make([][]cellValue, len(header))
	var records [][]cellValue
	for i := headerRow; i < len(grid); i++ {
		row := grid[i]
		if rowEmpty(row) {
			continue
		}
		excelRow := i + 1
		rec := make([]cellValue, len(header))
		for c, h := range header {
			raw := ""
			if h.col < len(row) {
				raw = row[h.col]
			}
			cell, err := excelize.CoordinatesToCellName(h.col+1, excelRow)
			if err != nil {
				return nil, nil, fmt.Errorf("worksheet %q could not be read", sheet)
			}
			val, err := w.cellValue(sheet, cell, raw)
			if err != nil {
				return nil, nil, err
			}
			rec[c] = val
			if !val.blank {
				samples[c] = append(samples[c], val)
			}
		}
		records = append(records, rec)
	}
	kinds := make([]collectionfile.Kind, len(header))
	for i := range header {
		kinds[i] = mergedKind(samples[i])
	}
	rows := make([]map[string]any, len(records))
	for r, rec := range records {
		row := make(map[string]any, len(header))
		for c, h := range header {
			row[h.name] = coerce(kinds[c], rec[c])
		}
		rows[r] = row
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, kinds, nil
}

func (w *Workbook) cellValue(sheet, cell, raw string) (cellValue, error) {
	formula, err := w.file.GetCellFormula(sheet, cell)
	if err != nil {
		return cellValue{}, fmt.Errorf("worksheet %q could not be read", sheet)
	}
	typ, err := w.file.GetCellType(sheet, cell)
	if err != nil {
		return cellValue{}, fmt.Errorf("worksheet %q could not be read", sheet)
	}
	if typ == excelize.CellTypeError {
		return cellValue{}, fmt.Errorf("worksheet %q cell %s contains an Excel error", sheet, cell)
	}
	if strings.TrimSpace(formula) != "" && raw == "" {
		return cellValue{}, fmt.Errorf("worksheet %q cell %s has a formula with no cached result", sheet, cell)
	}
	if raw == "" && typ != excelize.CellTypeBool {
		return cellValue{blank: true}, nil
	}
	switch typ {
	case excelize.CellTypeBool:
		return cellValue{kind: collectionfile.KindBoolean, value: raw == "1" || strings.EqualFold(raw, "true")}, nil
	case excelize.CellTypeSharedString, excelize.CellTypeInlineString, excelize.CellTypeFormula:
		return cellValue{kind: collectionfile.KindString, value: raw}, nil
	case excelize.CellTypeDate:
		return parseISODate(raw)
	default:
		return w.numericOrText(sheet, cell, raw)
	}
}

func (w *Workbook) numericOrText(sheet, cell, raw string) (cellValue, error) {
	if n, ok := integerLiteral(raw); ok {
		if _, hasTime, isDate := w.dateFormat(sheet, cell); isDate {
			return excelSerial(float64(n), w.date1904, hasTime), nil
		}
		return cellValue{kind: collectionfile.KindInteger, value: n}, nil
	}
	if f, ok := floatLiteral(raw); ok {
		if _, hasTime, isDate := w.dateFormat(sheet, cell); isDate {
			return excelSerial(f, w.date1904, hasTime), nil
		}
		return cellValue{kind: collectionfile.KindDecimal, value: f}, nil
	}
	return cellValue{kind: collectionfile.KindString, value: raw}, nil
}

func excelSerial(serial float64, date1904, hasTime bool) cellValue {
	t, err := excelize.ExcelDateToTime(serial, date1904)
	if err != nil {
		if serial == math.Trunc(serial) && serial >= math.MinInt64 && serial <= math.MaxInt64 {
			return cellValue{kind: collectionfile.KindInteger, value: int64(serial)}
		}
		return cellValue{kind: collectionfile.KindDecimal, value: serial}
	}
	t = t.UTC()
	if hasTime {
		return cellValue{kind: collectionfile.KindDateTime, value: collectionfile.DateTime{Time: t}}
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return cellValue{kind: collectionfile.KindDate, value: collectionfile.Date{Time: day}}
}

func (w *Workbook) dateFormat(sheet, cell string) (isDate, hasTime, ok bool) {
	styleID, err := w.file.GetCellStyle(sheet, cell)
	if err != nil || styleID == 0 {
		return false, false, false
	}
	style, err := w.file.GetStyle(styleID)
	if err != nil || style == nil {
		return false, false, false
	}
	if style.CustomNumFmt != nil && strings.TrimSpace(*style.CustomNumFmt) != "" {
		isDate, hasTime = classifyFormatCode(*style.CustomNumFmt)
		return isDate, hasTime, isDate
	}
	isDate, hasTime = classifyNumFmtID(style.NumFmt)
	return isDate, hasTime, isDate
}

func classifyNumFmtID(id int) (isDate, hasTime bool) {
	switch id {
	case 14, 15, 16, 17, 27, 28, 29, 30, 31, 36, 50, 51, 54, 57, 58:
		return true, false
	case 18, 19, 20, 21, 22, 32, 33, 34, 35, 45, 46, 47, 52, 53, 55, 56:
		return true, true
	default:
		return false, false
	}
}

func classifyFormatCode(code string) (isDate, hasTime bool) {
	cleaned := stripFormatNoise(code)
	lower := strings.ToLower(cleaned)
	if i := strings.Index(lower, ";"); i >= 0 {
		lower = lower[:i]
	}
	lower = strings.ReplaceAll(lower, "am/pm", "")
	lower = strings.ReplaceAll(lower, "a/p", "")
	hasY := strings.Contains(lower, "y")
	hasD := strings.Contains(lower, "d")
	hasH := strings.Contains(lower, "h")
	hasS := strings.Contains(lower, "s")
	hasM := strings.Contains(lower, "m")
	if !(hasY || hasD || hasH || hasS || hasM) {
		return false, false
	}
	return true, hasH || hasS
}

func stripFormatNoise(code string) string {
	var b strings.Builder
	r := []rune(code)
	for i := 0; i < len(r); i++ {
		switch r[i] {
		case '"':
			i++
			for i < len(r) && r[i] != '"' {
				i++
			}
		case '\\':
			if i+1 < len(r) {
				i++
			}
		case '[':
			j := i + 1
			for j < len(r) && r[j] != ']' {
				j++
			}
			inner := strings.ToLower(strings.TrimSpace(string(r[i+1 : j])))
			switch inner {
			case "h", "hh", "m", "mm", "s", "ss":
				b.WriteString(inner)
			}
			i = j
		default:
			b.WriteRune(r[i])
		}
	}
	return b.String()
}

func integerLiteral(raw string) (int64, bool) {
	if raw == "" || strings.ContainsAny(raw, ".eE+") {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	return n, err == nil
}

func floatLiteral(raw string) (float64, bool) {
	if raw == "" || strings.ContainsAny(raw, " \t") {
		return 0, false
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func parseISODate(raw string) (cellValue, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		"2006-01-02",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	var parsed time.Time
	var ok bool
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			parsed = t.UTC()
			ok = true
			break
		}
	}
	if !ok {
		return cellValue{kind: collectionfile.KindString, value: raw}, nil
	}
	if len(raw) <= 10 {
		day := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
		return cellValue{kind: collectionfile.KindDate, value: collectionfile.Date{Time: day}}, nil
	}
	return cellValue{kind: collectionfile.KindDateTime, value: collectionfile.DateTime{Time: parsed}}, nil
}

func mergedKind(samples []cellValue) collectionfile.Kind {
	var kind collectionfile.Kind
	for _, sample := range samples {
		if sample.blank {
			continue
		}
		kind = mergeKind(kind, sample.kind)
	}
	if kind == "" {
		return collectionfile.KindString
	}
	return kind
}

func mergeKind(current, next collectionfile.Kind) collectionfile.Kind {
	if current == "" {
		return next
	}
	if next == "" || current == next {
		return current
	}
	if current == collectionfile.KindString || next == collectionfile.KindString {
		return collectionfile.KindString
	}
	if (current == collectionfile.KindInteger && next == collectionfile.KindDecimal) || (current == collectionfile.KindDecimal && next == collectionfile.KindInteger) {
		return collectionfile.KindDecimal
	}
	if (current == collectionfile.KindDate && next == collectionfile.KindDateTime) || (current == collectionfile.KindDateTime && next == collectionfile.KindDate) {
		return collectionfile.KindDateTime
	}
	return collectionfile.KindString
}

func coerce(kind collectionfile.Kind, cell cellValue) any {
	if cell.blank {
		return nil
	}
	switch kind {
	case collectionfile.KindString:
		if cell.kind == collectionfile.KindString {
			if s, ok := cell.value.(string); ok {
				return s
			}
		}
		return stringify(cell.value)
	case collectionfile.KindInteger:
		if cell.kind == collectionfile.KindInteger {
			return cell.value
		}
		return nil
	case collectionfile.KindDecimal:
		switch cell.kind {
		case collectionfile.KindDecimal:
			return cell.value
		case collectionfile.KindInteger:
			return float64(cell.value.(int64))
		default:
			return nil
		}
	case collectionfile.KindBoolean:
		if cell.kind == collectionfile.KindBoolean {
			return cell.value
		}
		return nil
	case collectionfile.KindDate:
		if cell.kind == collectionfile.KindDate {
			return cell.value
		}
		return nil
	case collectionfile.KindDateTime:
		switch cell.kind {
		case collectionfile.KindDateTime:
			return cell.value
		case collectionfile.KindDate:
			d := cell.value.(collectionfile.Date)
			return collectionfile.DateTime{Time: d.Time}
		default:
			return nil
		}
	default:
		return stringify(cell.value)
	}
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case collectionfile.Date:
		return t.Time.UTC().Format("2006-01-02")
	case collectionfile.DateTime:
		return t.Time.UTC().Format(time.RFC3339)
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(t)
	}
}
