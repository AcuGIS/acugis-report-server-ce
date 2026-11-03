package cassandra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// AstraEndpoint returns the cleaned API endpoint URL from Host.
func AstraEndpoint(cfg Config) string {
	return strings.TrimRight(strings.TrimSpace(cfg.Host), "/")
}

// AstraToken returns the application token (stored via credential store as Password).
func AstraToken(cfg Config) string {
	return strings.TrimSpace(cfg.Password)
}

// AstraKeyspace returns the working keyspace (default_keyspace when empty).
func AstraKeyspace(cfg Config) string {
	ks := strings.TrimSpace(cfg.Database)
	if ks == "" {
		return "default_keyspace"
	}
	return ks
}

type astraClient struct {
	endpoint string
	token    string
	http     *http.Client
}

func newAstraClient(cfg Config) (*astraClient, error) {
	ep := AstraEndpoint(cfg)
	if ep == "" {
		return nil, fmt.Errorf("Astra API endpoint is required")
	}
	if !strings.HasPrefix(strings.ToLower(ep), "https://") {
		return nil, fmt.Errorf("Astra API endpoint must be an https:// URL")
	}
	tok := AstraToken(cfg)
	if tok == "" {
		return nil, fmt.Errorf("Astra application token is required")
	}
	return &astraClient{
		endpoint: ep,
		token:    tok,
		http:     &http.Client{Timeout: 45 * time.Second},
	}, nil
}

func (c *astraClient) post(ctx context.Context, path string, body any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := c.endpoint + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Token", c.token)

	log.Printf("[acugis-cassandra-astra] POST %s", redactAstraURL(u))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, friendlyAstraErr(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, friendlyAstraErr(err)
	}
	var parsed map[string]any
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("invalid Astra Data API response: %s", shortErr(string(data)))
		}
	} else {
		parsed = map[string]any{}
	}
	if errs, ok := parsed["errors"].([]any); ok && len(errs) > 0 {
		return nil, fmt.Errorf("%s", astraErrorMessage(errs))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return nil, friendlyAstraHTTP(resp.StatusCode, msg)
	}
	return parsed, nil
}

func redactAstraURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.Path
}

func astraErrorMessage(errs []any) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		msg, _ := m["message"].(string)
		code, _ := m["errorCode"].(string)
		if msg == "" {
			msg, _ = m["errorMessage"].(string)
		}
		if code != "" && msg != "" {
			parts = append(parts, code+": "+msg)
		} else if msg != "" {
			parts = append(parts, msg)
		} else if code != "" {
			parts = append(parts, code)
		}
	}
	if len(parts) == 0 {
		return "Astra Data API request failed"
	}
	return shortErr(strings.Join(parts, "; "))
}

func friendlyAstraErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline"):
		return fmt.Errorf("Astra connection timed out: %s", shortErr(msg))
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "dial tcp"),
		strings.Contains(lower, "connection refused"):
		return fmt.Errorf("unable to reach Astra API endpoint: %s", shortErr(msg))
	default:
		return fmt.Errorf("Astra Data API error: %s", shortErr(msg))
	}
}

func friendlyAstraHTTP(code int, body string) error {
	lower := strings.ToLower(body)
	switch {
	case code == 401 || code == 403 || strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "authentication") || strings.Contains(lower, "token"):
		return fmt.Errorf("Astra authentication failed: check application token")
	case code == 404:
		return fmt.Errorf("Astra resource not found: %s", shortErr(body))
	default:
		return fmt.Errorf("Astra Data API HTTP %d: %s", code, shortErr(body))
	}
}

func astraTestConnection(cfg Config) (*TestResult, error) {
	client, err := newAstraClient(cfg)
	if err != nil {
		return nil, err
	}
	ks := AstraKeyspace(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tables, err := client.listTables(ctx, ks, false)
	if err != nil {
		return nil, err
	}
	result := &TestResult{
		Connected:     true,
		Database:      ks,
		Driver:        "Astra Data API",
		SchemaCount:   1,
		TableCount:    len(tables),
		Message:       fmt.Sprintf("Connected · Astra Data API · keyspace %s · %d table(s)", ks, len(tables)),
		ServerVersion: "Data API",
	}
	log.Printf("[acugis-cassandra-astra] test ok endpoint=%s keyspace=%s tables=%d",
		redactAstraURL(client.endpoint), ks, len(tables))
	return result, nil
}

type astraTableDesc struct {
	Name       string
	Columns    []columnMeta
	Definition map[string]any
}

func (c *astraClient) listTables(ctx context.Context, keyspace string, explain bool) ([]astraTableDesc, error) {
	path := "/api/json/v1/" + url.PathEscape(keyspace)
	body := map[string]any{
		"listTables": map[string]any{
			"options": map[string]any{"explain": explain},
		},
	}
	resp, err := c.post(ctx, path, body)
	if err != nil {
		return nil, err
	}
	status, _ := resp["status"].(map[string]any)
	rawTables, _ := status["tables"].([]any)
	out := make([]astraTableDesc, 0, len(rawTables))
	for _, raw := range rawTables {
		switch t := raw.(type) {
		case string:
			out = append(out, astraTableDesc{Name: t})
		case map[string]any:
			name, _ := t["name"].(string)
			def, _ := t["definition"].(map[string]any)
			out = append(out, astraTableDesc{
				Name:       name,
				Definition: def,
				Columns:    astraColumnsFromDefinition(def),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func astraColumnsFromDefinition(def map[string]any) []columnMeta {
	if def == nil {
		return nil
	}
	colsRaw, _ := def["columns"].(map[string]any)
	pk, _ := def["primaryKey"].(map[string]any)
	partitionBy := stringSlice(pk["partitionBy"])
	if len(partitionBy) == 0 {
		partitionBy = stringSlice(pk["partitionKey"])
	}
	partitionSort, _ := pk["partitionSort"].(map[string]any)

	partitionSet := map[string]int{}
	for i, n := range partitionBy {
		partitionSet[n] = i
	}
	clusteringSet := map[string]int{}
	ci := 0
	for name := range partitionSort {
		clusteringSet[name] = ci
		ci++
	}

	var out []columnMeta
	for name, raw := range colsRaw {
		typ := astraColumnType(raw)
		kind := "regular"
		pos := 0
		if i, ok := partitionSet[name]; ok {
			kind = "partition_key"
			pos = i
		} else if i, ok := clusteringSet[name]; ok {
			kind = "clustering"
			pos = i
		}
		out = append(out, columnMeta{
			Name:         name,
			Type:         typ,
			Kind:         kind,
			Position:     pos,
			IsPrimaryKey: kind == "partition_key" || kind == "clustering",
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := kindOrder(out[i].Kind), kindOrder(out[j].Kind)
		if ki != kj {
			return ki < kj
		}
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func astraColumnType(raw any) string {
	switch t := raw.(type) {
	case string:
		return t
	case map[string]any:
		typ, _ := t["type"].(string)
		if typ == "" {
			return "text"
		}
		if typ == "UNSUPPORTED" {
			if api, ok := t["apiSupport"].(map[string]any); ok {
				if cql, _ := api["cqlDefinition"].(string); cql != "" {
					return cql
				}
			}
			return "text"
		}
		if typ == "list" || typ == "set" {
			if vt, _ := t["valueType"].(string); vt != "" {
				return typ + "<" + vt + ">"
			}
		}
		if typ == "map" {
			kt, _ := t["keyType"].(string)
			vt, _ := t["valueType"].(string)
			if kt != "" && vt != "" {
				return "map<" + kt + ", " + vt + ">"
			}
		}
		return typ
	default:
		return "text"
	}
}

func stringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func astraListSchemas(cfg Config) ([]SchemaInfo, error) {
	ks := AstraKeyspace(cfg)
	return []SchemaInfo{{Name: ks}}, nil
}

func astraListTables(cfg Config, schema string) ([]TableInfo, error) {
	client, err := newAstraClient(cfg)
	if err != nil {
		return nil, err
	}
	ks := strings.TrimSpace(schema)
	if ks == "" {
		ks = AstraKeyspace(cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tables, err := client.listTables(ctx, ks, false)
	if err != nil {
		return nil, err
	}
	out := make([]TableInfo, 0, len(tables))
	for _, t := range tables {
		if t.Name == "" {
			continue
		}
		out = append(out, TableInfo{Schema: ks, Name: t.Name, Type: "table"})
	}
	return out, nil
}

func astraListColumns(cfg Config, schema, table string) ([]ColumnInfo, error) {
	client, err := newAstraClient(cfg)
	if err != nil {
		return nil, err
	}
	ks := strings.TrimSpace(schema)
	if ks == "" {
		ks = AstraKeyspace(cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tables, err := client.listTables(ctx, ks, true)
	if err != nil {
		return nil, err
	}
	for _, t := range tables {
		if strings.EqualFold(t.Name, table) {
			return toColumnInfos(t.Columns), nil
		}
	}
	return nil, fmt.Errorf("unknown table: %s.%s", ks, table)
}

func astraListPrimaryKeys(cfg Config, schema, table string) ([]string, error) {
	cols, err := astraListColumns(cfg, schema, table)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range cols {
		if c.IsPrimaryKey || strings.Contains(strings.ToLower(c.UDTName), "clustering") {
			out = append(out, c.Name)
		}
	}
	return out, nil
}

var (
	reFromTable = regexp.MustCompile(`(?is)\bFROM\s+([A-Za-z0-9_."]+)\s*(?:;|$|\bWHERE\b|\bLIMIT\b|\bALLOW\b|\bORDER\b)`)
	reWhereEq   = regexp.MustCompile(`(?i)([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\$P\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
	reWhereIn   = regexp.MustCompile(`(?i)([A-Za-z_][A-Za-z0-9_]*)\s+IN\s*\(\s*\$P\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\s*\)`)
)

func parseAstraTableRef(query, defaultKeyspace string) (keyspace, table string, err error) {
	q := strings.TrimSpace(stripCQLComments(query))
	if q == "" {
		return "", "", fmt.Errorf("query is empty")
	}
	m := reFromTable.FindStringSubmatch(q)
	if len(m) < 2 {
		return "", "", fmt.Errorf("Astra Data API queries must be SELECT … FROM keyspace.table (mapped to Data API find, not CQL)")
	}
	ref := strings.Trim(m[1], `"`)
	parts := strings.Split(ref, ".")
	switch len(parts) {
	case 1:
		return defaultKeyspace, unquoteIdent(parts[0]), nil
	case 2:
		return unquoteIdent(parts[0]), unquoteIdent(parts[1]), nil
	default:
		return "", "", fmt.Errorf("invalid table reference: %s", ref)
	}
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.ReplaceAll(s[1:len(s)-1], `""`, `"`)
	}
	return s
}

func astraFilterFromQuery(query string, params map[string]any) map[string]any {
	filter := map[string]any{}
	for _, m := range reWhereIn.FindAllStringSubmatch(query, -1) {
		if len(m) != 3 {
			continue
		}
		col, pname := m[1], m[2]
		v, ok := params[pname]
		if !ok {
			continue
		}
		filter[col] = map[string]any{"$in": coerceSlice(v)}
	}
	for _, m := range reWhereEq.FindAllStringSubmatch(query, -1) {
		if len(m) != 3 {
			continue
		}
		col, pname := m[1], m[2]
		if _, exists := filter[col]; exists {
			continue
		}
		v, ok := params[pname]
		if !ok {
			continue
		}
		filter[col] = map[string]any{"$eq": coerceBindValue(v)}
	}
	if len(filter) == 0 {
		return map[string]any{}
	}
	return filter
}

func (c *astraClient) findRows(ctx context.Context, keyspace, table string, filter map[string]any, limit int) ([]map[string]any, error) {
	path := "/api/json/v1/" + url.PathEscape(keyspace) + "/" + url.PathEscape(table)
	find := map[string]any{
		"filter": filter,
	}
	if limit > 0 {
		find["options"] = map[string]any{"limit": limit}
	}
	resp, err := c.post(ctx, path, map[string]any{"find": find})
	if err != nil {
		return nil, err
	}
	data, _ := resp["data"].(map[string]any)
	docs, _ := data["documents"].([]any)
	out := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		m, ok := d.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, m)
	}
	// Follow a single next page when needed to approach the limit.
	next, _ := data["nextPageState"].(string)
	for next != "" && (limit <= 0 || len(out) < limit) {
		find["options"] = map[string]any{
			"limit":            limit,
			"pageState":        next,
			"initialPageState": next,
		}
		resp, err = c.post(ctx, path, map[string]any{"find": find})
		if err != nil {
			break
		}
		data, _ = resp["data"].(map[string]any)
		docs, _ = data["documents"].([]any)
		if len(docs) == 0 {
			break
		}
		for _, d := range docs {
			m, ok := d.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, m)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		next, _ = data["nextPageState"].(string)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func astraPreviewQuery(cfg Config, query string, params map[string]any, limit int) (*PreviewResult, error) {
	if err := ensureReadOnly(query); err != nil {
		return nil, err
	}
	client, err := newAstraClient(cfg)
	if err != nil {
		return nil, err
	}
	ks, table, err := parseAstraTableRef(query, AstraKeyspace(cfg))
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	rows, err := client.findRows(ctx, ks, table, astraFilterFromQuery(query, params), limit)
	if err != nil {
		return nil, err
	}
	log.Printf("[acugis-cassandra-astra] find keyspace=%s table=%s rows=%d duration=%s",
		ks, table, len(rows), time.Since(start))
	return mapsToPreview(rows), nil
}

func astraFetchTypedRows(cfg Config, query string, params map[string]any, limit int) ([]map[string]any, error) {
	if err := ensureReadOnly(query); err != nil {
		return nil, err
	}
	client, err := newAstraClient(cfg)
	if err != nil {
		return nil, err
	}
	ks, table, err := parseAstraTableRef(query, AstraKeyspace(cfg))
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 5000
	}
	colTypes := map[string]string{}
	if cols, cErr := astraListColumns(cfg, ks, table); cErr == nil {
		for _, c := range cols {
			colTypes[c.Name] = c.DataType
		}
	} else {
		log.Printf("[acugis-cassandra-astra] column metadata unavailable for %s.%s: %v", ks, table, cErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	rows, err := client.findRows(ctx, ks, table, astraFilterFromQuery(query, params), limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		converted := ConvertAstraRow(colTypes, row)
		if astraDebugEnabled() {
			for name, v := range converted {
				debugTypeLog(name, colTypes[name], MapJavaClass(colTypes[name]), row[name], v)
			}
		}
		out = append(out, converted)
	}
	log.Printf("[acugis-cassandra-astra] fill rows=%d keyspace=%s table=%s", len(out), ks, table)
	return out, nil
}

func mapsToPreview(rows []map[string]any) *PreviewResult {
	colSet := map[string]bool{}
	var cols []string
	for _, row := range rows {
		for k := range row {
			if strings.HasPrefix(k, "$") {
				continue
			}
			if !colSet[k] {
				colSet[k] = true
				cols = append(cols, k)
			}
		}
	}
	sort.Strings(cols)
	result := &PreviewResult{Columns: cols}
	for _, row := range rows {
		r := make([]string, len(cols))
		for i, c := range cols {
			r[i] = formatCell(row[c])
		}
		result.Rows = append(result.Rows, r)
	}
	return result
}
