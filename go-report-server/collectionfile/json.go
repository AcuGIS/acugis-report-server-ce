package collectionfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// MaxJSONFields is the largest union of fields a record collection may expose.
	MaxJSONFields = 512
	// MaxJSONCollections is the largest number of record arrays discovered in one document.
	MaxJSONCollections = 64
	// MaxFlattenDepth is how many nested objects are expanded into dot-qualified fields.
	// Deeper objects are kept as a JSON string on the last accepted prefix.
	MaxFlattenDepth = 8
	// maxDocumentDepth rejects pathological nesting before it can overflow the decoder.
	maxDocumentDepth = 32
	// maxDiscoverDepth is how many object keys a record-array path may contain.
	maxDiscoverDepth = 8
	// maxFieldNameLen rejects enormous keys before they become report fields.
	maxFieldNameLen = 256
)

// RootCollection is the record collection for a root array, or for a root
// object that itself has scalar fields.
const RootCollection = "root"

// CollectionInfo is one array of objects the designer can select.
type CollectionInfo struct {
	Path string
	Rows int
}

// Document is a parsed JSON document and the record collections found in it.
type Document struct {
	collections []CollectionInfo
	nodes       map[string][]any
}

// Collections returns discovered record collections in a stable order.
// Nested arrays are listed before a root object row.
func (d *Document) Collections() []CollectionInfo {
	if d == nil {
		return nil
	}
	out := make([]CollectionInfo, len(d.collections))
	copy(out, d.collections)
	return out
}

// Table materializes one record collection. An empty path uses the default,
// which is the first nested array, or root when that is the only collection.
func (d *Document) Table(path string) (*Table, error) {
	if d == nil {
		return nil, fmt.Errorf("JSON document has no record collection")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		if len(d.collections) == 0 {
			return nil, fmt.Errorf("JSON document has no record collection")
		}
		path = d.collections[0].Path
	}
	rows, ok := d.nodes[path]
	if !ok {
		return nil, fmt.Errorf("JSON document has no record collection %q", path)
	}
	return buildJSONTable(rows)
}

// Select chooses the record collection for a query.
// A FROM table that matches a discovered collection wins. Otherwise the
// configured path is used. An empty configuration uses the default collection.
func (d *Document) Select(configured, sql string) (string, error) {
	if d == nil || len(d.collections) == 0 {
		return "", fmt.Errorf("JSON document has no record collection")
	}
	if name := tableFromSQL(sql); name != "" {
		if _, ok := d.nodes[name]; ok {
			return name, nil
		}
	}
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if _, ok := d.nodes[configured]; ok {
			return configured, nil
		}
		return "", fmt.Errorf("JSON document has no record collection %q", configured)
	}
	return d.collections[0].Path, nil
}

// ParseJSON reads one JSON document and discovers record collections.
// Numbers stay exact until they are classified as Long or Double.
func ParseJSON(r io.Reader) (*Document, error) {
	if r == nil {
		return nil, fmt.Errorf("JSON document is not valid")
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("JSON document is not valid")
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("JSON file exceeds the %d byte limit", MaxBytes)
	}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || !utf8.Valid(data) {
		return nil, fmt.Errorf("JSON document is not valid")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := decodeJSONValue(dec, 0)
	if err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("JSON document is not valid")
	}
	return documentFromValue(value)
}

type jsonObject struct {
	keys []string
	vals map[string]any
}

func decodeJSONValue(dec *json.Decoder, depth int) (any, error) {
	if depth > maxDocumentDepth {
		return nil, fmt.Errorf("JSON document is nested too deeply")
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("JSON document is not valid")
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := &jsonObject{vals: map[string]any{}}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, fmt.Errorf("JSON document is not valid")
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, fmt.Errorf("JSON document is not valid")
				}
				val, err := decodeJSONValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				if _, exists := obj.vals[key]; exists {
					obj.vals[key] = val
					continue
				}
				obj.keys = append(obj.keys, key)
				obj.vals[key] = val
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim('}') {
				return nil, fmt.Errorf("JSON document is not valid")
			}
			return obj, nil
		case '[':
			var arr []any
			for dec.More() {
				val, err := decodeJSONValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim(']') {
				return nil, fmt.Errorf("JSON document is not valid")
			}
			if arr == nil {
				arr = []any{}
			}
			return arr, nil
		default:
			return nil, fmt.Errorf("JSON document is not valid")
		}
	case string, bool, json.Number, nil:
		return t, nil
	default:
		return nil, fmt.Errorf("JSON document is not valid")
	}
}

func documentFromValue(value any) (*Document, error) {
	nodes := map[string][]any{}
	var list []CollectionInfo
	switch t := value.(type) {
	case []any:
		if !isRecordArray(t) {
			return nil, fmt.Errorf("JSON document cannot be represented as records")
		}
		nodes[RootCollection] = t
		list = append(list, CollectionInfo{Path: RootCollection, Rows: len(t)})
	case *jsonObject:
		if err := walkJSONObject(t, "", 0, nodes, &list); err != nil {
			return nil, err
		}
		if hasJSONScalar(t, 0) {
			if _, exists := nodes[RootCollection]; !exists {
				if len(list) >= MaxJSONCollections {
					return nil, fmt.Errorf("JSON document has too many record collections")
				}
				nodes[RootCollection] = []any{t}
				list = append(list, CollectionInfo{Path: RootCollection, Rows: 1})
			}
		}
	default:
		return nil, fmt.Errorf("JSON document cannot be represented as records")
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("JSON document has no record collection")
	}
	return &Document{collections: list, nodes: nodes}, nil
}

func walkJSONObject(obj *jsonObject, path string, depth int, nodes map[string][]any, list *[]CollectionInfo) error {
	if obj == nil || depth >= maxDiscoverDepth {
		return nil
	}
	for _, key := range obj.keys {
		if key == "" || strings.Contains(key, ".") {
			continue
		}
		next := key
		if path != "" {
			next = path + "." + key
		}
		switch child := obj.vals[key].(type) {
		case []any:
			if !isRecordArray(child) {
				continue
			}
			if _, exists := nodes[next]; exists {
				continue
			}
			if len(*list) >= MaxJSONCollections {
				return fmt.Errorf("JSON document has too many record collections")
			}
			nodes[next] = child
			*list = append(*list, CollectionInfo{Path: next, Rows: len(child)})
		case *jsonObject:
			if err := walkJSONObject(child, next, depth+1, nodes, list); err != nil {
				return err
			}
		}
	}
	return nil
}

func isRecordArray(arr []any) bool {
	for _, el := range arr {
		if _, ok := el.(*jsonObject); !ok {
			return false
		}
	}
	return true
}

func hasJSONScalar(obj *jsonObject, depth int) bool {
	if obj == nil {
		return false
	}
	for _, key := range obj.keys {
		switch child := obj.vals[key].(type) {
		case *jsonObject:
			if depth+1 >= MaxFlattenDepth || hasJSONScalar(child, depth+1) {
				return true
			}
		case []any:
			if !isRecordArray(child) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

type jsonField struct {
	name  string
	cells []cell
}

func buildJSONTable(records []any) (*Table, error) {
	fields := []*jsonField{}
	index := map[string]int{}
	for row, rec := range records {
		obj, ok := rec.(*jsonObject)
		if !ok {
			return nil, fmt.Errorf("JSON document cannot be represented as records")
		}
		flat := map[string]any{}
		var order []string
		if err := flattenJSON(obj, "", 0, flat, &order); err != nil {
			return nil, err
		}
		for _, name := range order {
			if _, ok := index[name]; ok {
				continue
			}
			if len(fields) >= MaxJSONFields {
				return nil, fmt.Errorf("JSON document has too many fields")
			}
			missing := make([]cell, row)
			for i := range missing {
				missing[i] = cell{skip: true, value: nil}
			}
			index[name] = len(fields)
			fields = append(fields, &jsonField{name: name, cells: missing})
		}
		for _, field := range fields {
			if value, ok := flat[field.name]; ok {
				field.cells = append(field.cells, jsonCell(value))
			} else {
				field.cells = append(field.cells, cell{skip: true, value: nil})
			}
		}
	}
	table := &Table{
		Columns: make([]Column, len(fields)),
		Rows:    make([]map[string]any, len(records)),
	}
	for i := range table.Rows {
		table.Rows[i] = map[string]any{}
	}
	for i, field := range fields {
		kind := widenKind(inferColumnKind(field.cells, DefaultSampleLimit), field.cells)
		table.Columns[i] = Column{Name: field.name, Kind: kind}
		for row, c := range field.cells {
			table.Rows[row][field.name] = finalizeCell(kind, c)
		}
	}
	return table, nil
}

func flattenJSON(obj *jsonObject, prefix string, depth int, out map[string]any, order *[]string) error {
	if obj == nil {
		return nil
	}
	for _, key := range obj.keys {
		name := key
		if prefix != "" {
			name = prefix + "." + key
		}
		if len(name) > maxFieldNameLen {
			return fmt.Errorf("JSON document has a field name that is too long")
		}
		value := obj.vals[key]
		switch child := value.(type) {
		case *jsonObject:
			if depth >= MaxFlattenDepth {
				text, err := marshalJSONValue(child)
				if err != nil {
					return fmt.Errorf("JSON document is not valid")
				}
				if err := addJSONField(out, order, name, text); err != nil {
					return err
				}
				continue
			}
			if err := flattenJSON(child, name, depth+1, out, order); err != nil {
				return err
			}
		case []any:
			text, err := marshalJSONValue(child)
			if err != nil {
				return fmt.Errorf("JSON document is not valid")
			}
			if err := addJSONField(out, order, name, text); err != nil {
				return err
			}
		default:
			if err := addJSONField(out, order, name, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func addJSONField(out map[string]any, order *[]string, name string, value any) error {
	if _, exists := out[name]; exists {
		return fmt.Errorf("JSON document has duplicate field %q", name)
	}
	out[name] = value
	*order = append(*order, name)
	return nil
}

func jsonCell(value any) cell {
	switch t := value.(type) {
	case nil:
		return cell{skip: true, value: nil}
	case bool:
		raw := "false"
		if t {
			raw = "true"
		}
		return cell{raw: raw, kind: KindBoolean, value: t}
	case string:
		if tm, ok := parseDateTime(t); ok {
			if isDateOnly(t) {
				return cell{raw: t, kind: KindDate, value: Date{Time: tm}}
			}
			return cell{raw: t, kind: KindDateTime, value: DateTime{Time: tm}}
		}
		return cell{raw: t, kind: KindString, value: t, quoted: true}
	case json.Number:
		return numberCell(t)
	default:
		raw := fmt.Sprint(t)
		return cell{raw: raw, kind: KindString, value: raw, quoted: true}
	}
}

func numberCell(n json.Number) cell {
	raw := n.String()
	if raw == "" {
		return cell{skip: true, value: nil}
	}
	if !strings.ContainsAny(raw, ".eE") {
		if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return cell{raw: raw, kind: KindInteger, value: i}
		}
		return cell{raw: raw, kind: KindString, value: raw, quoted: true}
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return cell{raw: raw, kind: KindString, value: raw, quoted: true}
	}
	return cell{raw: raw, kind: KindDecimal, value: f}
}

func marshalJSONValue(value any) (string, error) {
	var buf bytes.Buffer
	if err := writeJSONValue(&buf, value); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func writeJSONValue(buf *bytes.Buffer, value any) error {
	switch t := value.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		raw, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(raw)
	case json.Number:
		if t.String() == "" {
			buf.WriteString("null")
			return nil
		}
		buf.WriteString(t.String())
	case *jsonObject:
		buf.WriteByte('{')
		for i, key := range t.keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			raw, err := json.Marshal(key)
			if err != nil {
				return err
			}
			buf.Write(raw)
			buf.WriteByte(':')
			if err := writeJSONValue(buf, t.vals[key]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, el := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJSONValue(buf, el); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(raw)
	}
	return nil
}

var jsonFromRE = regexp.MustCompile(`(?is)\bfrom\s+(?:"(?:[^"]|"")*"\s*\.\s*)?(?:"((?:[^"]|"")*)"|([A-Za-z_][A-Za-z0-9_]*))`)

func tableFromSQL(sql string) string {
	m := jsonFromRE.FindStringSubmatch(sql)
	if m == nil {
		return ""
	}
	name := m[1]
	if name == "" {
		name = m[2]
	}
	return strings.ReplaceAll(strings.TrimSpace(name), `""`, `"`)
}
