package geoserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultFillLimit matches Designer report preview and the other collection providers.
	DefaultFillLimit = 5000
	wfsPageSize      = 1000
	maxResponseBytes = 8 << 20
)

// FetchTypedRows executes a WFS attribute query and returns []map[string]any
// for JRMapCollectionDataSource. Geometry is not copied into the row.
func FetchTypedRows(cfg Credentials, sql string, params map[string]any, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = DefaultFillLimit
	}
	query, err := translateQuery(sql, params)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	endpoint, err := wfsEndpoint(cfg.URL)
	if err != nil {
		return nil, err
	}
	fields, _ := describeFields(ctx, endpoint, cfg, query.TypeName)
	rows, err := fetchRows(ctx, endpoint, cfg, query, fields, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, nil
}

func wfsEndpoint(base string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "", fmt.Errorf("GeoServer base URL is required")
	}
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid GeoServer URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", fmt.Errorf("GeoServer URL must use http or https")
	}
	if u.User != nil {
		return "", fmt.Errorf("GeoServer URL must not include credentials")
	}
	lower := strings.ToLower(base)
	for _, suf := range []string{"/wms", "/wfs", "/ows"} {
		if strings.HasSuffix(lower, suf) {
			base = base[:len(base)-len(suf)]
			break
		}
	}
	return strings.TrimRight(base, "/") + "/wfs", nil
}

// NormalizeBaseURL checks that base is an http(s) GeoServer URL without embedded credentials.
// The stored catalog value stays the base URL. WFS requests still append /wfs.
func NormalizeBaseURL(base string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if _, err := wfsEndpoint(base); err != nil {
		return "", err
	}
	return base, nil
}

func applyBasicAuth(req *http.Request, cfg Credentials) {
	user := strings.TrimSpace(cfg.Username)
	if user == "" {
		return
	}
	token := base64.StdEncoding.EncodeToString([]byte(user + ":" + cfg.Password))
	req.Header.Set("Authorization", "Basic "+token)
}

func wfsGet(ctx context.Context, rawURL string, cfg Credentials) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build WFS request: %w", err)
	}
	req.Header.Set("Accept", "application/json, application/xml, text/xml, */*")
	applyBasicAuth(req, cfg)
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("WFS request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("WFS response: %w", err)
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, fmt.Errorf("WFS response too large")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed (%s)", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if msg := exceptionText(body); msg != "" {
			return nil, fmt.Errorf("WFS returned %s: %s", resp.Status, msg)
		}
		return nil, fmt.Errorf("WFS returned %s", resp.Status)
	}
	if msg := exceptionText(body); msg != "" {
		return nil, fmt.Errorf("WFS error: %s", msg)
	}
	return body, nil
}

func exceptionText(body []byte) string {
	if !bytes.Contains(body, []byte("<")) {
		return ""
	}
	snippet := strings.ToLower(string(body))
	if !strings.Contains(snippet, "exception") {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	var capture bool
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "ExceptionText" || t.Name.Local == "ServiceException" {
				capture = true
			}
		case xml.EndElement:
			if t.Name.Local == "ExceptionText" || t.Name.Local == "ServiceException" {
				capture = false
			}
		case xml.CharData:
			if capture {
				text := strings.TrimSpace(string(t))
				if text != "" {
					if b.Len() > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(text)
				}
			}
		}
	}
	msg := strings.TrimSpace(b.String())
	if msg == "" {
		return "GeoServer returned an exception report"
	}
	return msg
}

func describeFields(ctx context.Context, endpoint string, cfg Credentials, typeName string) ([]fieldType, error) {
	body, err := describeFeatureType(ctx, endpoint, cfg, typeName, "2.0.0")
	if err != nil {
		body, err = describeFeatureType(ctx, endpoint, cfg, typeName, "1.1.0")
		if err != nil {
			return nil, err
		}
	}
	return parseDescribeFeatureType(body)
}

func describeFeatureType(ctx context.Context, endpoint string, cfg Credentials, typeName, version string) ([]byte, error) {
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", version)
	q.Set("request", "DescribeFeatureType")
	if version == "2.0.0" {
		q.Set("typeNames", typeName)
	} else {
		q.Set("typeName", typeName)
	}
	return wfsGet(ctx, endpoint+"?"+q.Encode(), cfg)
}

func parseDescribeFeatureType(body []byte) ([]fieldType, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	var cols []fieldType
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("DescribeFeatureType XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "sequence" {
				depth++
				continue
			}
			if t.Name.Local != "element" || depth == 0 {
				continue
			}
			name, xsd, ok := namedElement(t.Attr)
			if !ok || name == "" || strings.EqualFold(name, "boundedBy") {
				continue
			}
			ft := mapXSDType(xsd)
			ft.Name = name
			cols = append(cols, ft)
		case xml.EndElement:
			if t.Name.Local == "sequence" && depth > 0 {
				depth--
			}
		}
	}
	return cols, nil
}

func namedElement(attrs []xml.Attr) (name, xsd string, ok bool) {
	for _, a := range attrs {
		switch a.Name.Local {
		case "name":
			name = strings.TrimSpace(a.Value)
		case "type":
			xsd = strings.TrimSpace(a.Value)
		case "ref":
			if name == "" {
				ref := strings.TrimSpace(a.Value)
				if i := strings.LastIndex(ref, ":"); i >= 0 {
					name = ref[i+1:]
				} else {
					name = ref
				}
			}
		}
	}
	if name == "" {
		return "", "", false
	}
	return name, xsd, true
}

func fetchRows(ctx context.Context, endpoint string, cfg Credentials, query *wfsQuery, fields []fieldType, limit int) ([]map[string]any, error) {
	byName := map[string]fieldType{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	outFields := make([]string, 0, len(query.Fields))
	for _, name := range query.Fields {
		if ft, ok := byName[name]; ok && ft.Geometry {
			continue
		}
		outFields = append(outFields, name)
	}
	if len(query.Fields) > 0 && len(outFields) == 0 {
		return nil, fmt.Errorf("GeoServer tabular reports use attributes; select at least one non-geometry field")
	}
	rows := make([]map[string]any, 0)
	seen := map[string]struct{}{}
	start := 0
	pages := 0
	maxPages := limit/wfsPageSize + 2
	for len(rows) < limit {
		pages++
		if pages > maxPages {
			return nil, fmt.Errorf("GeoServer WFS paging did not advance; stopped to avoid an unbounded read")
		}
		count := wfsPageSize
		if remain := limit - len(rows); remain < count {
			count = remain
		}
		page, err := getFeaturePage(ctx, endpoint, cfg, query, outFields, start, count)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		added := 0
		for _, feat := range page {
			key := feat.ID
			if key == "" {
				raw, _ := json.Marshal(feat.Properties)
				key = string(raw)
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, typedProperties(feat.Properties, outFields, byName))
			added++
			if len(rows) >= limit {
				break
			}
		}
		if added == 0 || len(page) < count {
			break
		}
		next := start + len(page)
		if next <= start {
			break
		}
		start = next
	}
	return rows, nil
}

type wfsFeature struct {
	ID         string
	Properties map[string]any
}

func getFeaturePage(ctx context.Context, endpoint string, cfg Credentials, query *wfsQuery, fields []string, start, count int) ([]wfsFeature, error) {
	body, err := getFeature(ctx, endpoint, cfg, query, fields, start, count, "2.0.0")
	if err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "version") || strings.Contains(low, "outputformat") || strings.Contains(low, "typenames") {
			body, err = getFeature(ctx, endpoint, cfg, query, fields, start, count, "1.1.0")
		}
	}
	if err != nil {
		return nil, err
	}
	return parseFeatureCollection(body)
}

func getFeature(ctx context.Context, endpoint string, cfg Credentials, query *wfsQuery, fields []string, start, count int, version string) ([]byte, error) {
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", version)
	q.Set("request", "GetFeature")
	q.Set("outputFormat", "application/json")
	if version == "2.0.0" {
		q.Set("typeNames", query.TypeName)
		q.Set("count", strconv.Itoa(count))
	} else {
		q.Set("typeName", query.TypeName)
		q.Set("maxFeatures", strconv.Itoa(count))
	}
	q.Set("startIndex", strconv.Itoa(start))
	if len(fields) > 0 {
		q.Set("propertyName", strings.Join(fields, ","))
	}
	if strings.TrimSpace(query.CQL) != "" {
		q.Set("CQL_FILTER", query.CQL)
	}
	raw := endpoint + "?" + q.Encode()
	if strings.TrimSpace(query.SortBy) != "" {
		raw += "&sortBy=" + query.SortBy
	}
	return wfsGet(ctx, raw, cfg)
}

func typedProperties(props map[string]any, fields []string, byName map[string]fieldType) map[string]any {
	names := fields
	if len(names) == 0 {
		names = make([]string, 0, len(props))
		for k := range props {
			if ft, ok := byName[k]; ok && ft.Geometry {
				continue
			}
			names = append(names, k)
		}
	}
	row := make(map[string]any, len(names))
	for _, name := range names {
		ft := byName[name]
		v, ok := props[name]
		if !ok || v == nil {
			row[name] = nil
			continue
		}
		row[name] = coerceProperty(v, ft)
	}
	return row
}

func parseFeatureCollection(body []byte) ([]wfsFeature, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var raw struct {
		Type     string `json:"type"`
		Features []struct {
			ID         string         `json:"id"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("WFS GeoJSON: %w", err)
	}
	if raw.Type != "" && !strings.EqualFold(raw.Type, "FeatureCollection") {
		return nil, fmt.Errorf("WFS response was %s, expected a GeoJSON FeatureCollection", raw.Type)
	}
	out := make([]wfsFeature, 0, len(raw.Features))
	for _, f := range raw.Features {
		props := f.Properties
		if props == nil {
			props = map[string]any{}
		}
		out = append(out, wfsFeature{ID: f.ID, Properties: props})
	}
	return out, nil
}

// TestConnection checks that the base URL answers WFS GetCapabilities.
// It does not run a feature query and does not change WFS fill behavior.
func TestConnection(ctx context.Context, cfg Credentials) (string, error) {
	endpoint, err := wfsEndpoint(cfg.URL)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", "2.0.0")
	q.Set("request", "GetCapabilities")
	body, err := wfsGet(ctx, endpoint+"?"+q.Encode(), cfg)
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "capabilities") {
		return "", fmt.Errorf("GeoServer did not return a WFS capabilities document")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return "Connected (anonymous)", nil
	}
	return "Connected", nil
}
