package arcgis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultFillLimit is the report execution cap when the caller does not pass
// a positive limit. It is the same cap Designer report preview and Cassandra /
// BigQuery collection fill use. Data Preview's 100-row default is not applied.
const DefaultFillLimit = 5000

// maxFeaturePageSize is the largest ArcGIS resultRecordCount per request.
// Designer pages at 1000. A layer maxRecordCount smaller than this becomes the
// page size so a full service page is not treated as the end of the result.
const maxFeaturePageSize = 1000

type querySpec struct {
	LayerURL   string
	OutFields  []string
	Where      string
	OrderBy    string
	ReturnGeom bool
	PageSize   int
}

// FetchTypedRows executes a Feature Server attribute query and returns
// []map[string]any for JRMapCollectionDataSource. Numeric attributes stay
// numeric. Epoch milliseconds stay int64 so they are not rendered in
// scientific notation; Jasper coerces them to java.util.Date only when the
// report field is a date. Ordinary integers are not turned into dates.
func FetchTypedRows(cfg Credentials, sql string, params map[string]any, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = DefaultFillLimit
	}
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return nil, fmt.Errorf("ArcGIS report has no query")
	}
	parsed, err := parseGeneratedSQL(sql)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	layer, base, err := resolveLayer(ctx, cfg.URL, cfg.Username, cfg.Password, parsed.Table)
	if err != nil {
		return nil, err
	}
	if !layer.SupportsQuery {
		return nil, fmt.Errorf("layer %q does not support query", layer.Name)
	}
	token, err := getToken(ctx, base, cfg.Username, cfg.Password)
	if err != nil {
		return nil, err
	}
	tokenRef := token

	outFields := parsed.Fields
	if len(outFields) == 0 {
		cols, ferr := fetchLayerFields(ctx, layer.LayerURL, base, cfg.Username, cfg.Password, &tokenRef)
		if ferr == nil {
			for _, c := range cols {
				if !c.IsGeometry {
					outFields = append(outFields, c.Name)
				}
			}
		}
	}

	pageSize := maxFeaturePageSize
	if layer.MaxRecordCount > 0 && layer.MaxRecordCount < pageSize {
		pageSize = layer.MaxRecordCount
	}
	spec := querySpec{
		LayerURL:   layer.LayerURL,
		OutFields:  outFields,
		Where:      rewriteWhereForArcGIS(parsed.Where, params),
		OrderBy:    rewriteOrderBy(parsed.OrderBy),
		ReturnGeom: false,
		PageSize:   pageSize,
	}
	rows, err := queryFeatures(ctx, base, cfg.Username, cfg.Password, &tokenRef, spec, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, nil
}

func queryFeatures(ctx context.Context, serviceURL, username, password string, token *string, spec querySpec, limit int) ([]map[string]any, error) {
	if strings.TrimSpace(spec.LayerURL) == "" {
		return nil, fmt.Errorf("layer URL is required")
	}
	if limit <= 0 {
		limit = 100
	}
	where := strings.TrimSpace(spec.Where)
	if where == "" {
		where = "1=1"
	}
	outFields := "*"
	if len(spec.OutFields) > 0 {
		outFields = strings.Join(spec.OutFields, ",")
	}
	pageSize := spec.PageSize
	if pageSize <= 0 || pageSize > maxFeaturePageSize {
		pageSize = maxFeaturePageSize
	}

	typed := make([]map[string]any, 0)
	for offset := 0; len(typed) < limit; {
		remain := limit - len(typed)
		count := pageSize
		if remain < count {
			count = remain
		}
		q := url.Values{}
		q.Set("f", "pjson")
		q.Set("where", where)
		q.Set("outFields", outFields)
		q.Set("returnGeometry", strconv.FormatBool(spec.ReturnGeom))
		q.Set("resultOffset", strconv.Itoa(offset))
		q.Set("resultRecordCount", strconv.Itoa(count))
		if strings.TrimSpace(spec.OrderBy) != "" {
			q.Set("orderByFields", spec.OrderBy)
		}
		callURL := strings.TrimSuffix(spec.LayerURL, "/") + "/query?" + q.Encode()
		body, err := jsonRequest(ctx, callURL, serviceURL, token, username, password)
		if err != nil {
			return nil, fmt.Errorf("feature query failed: %w", err)
		}
		rawFeatures, _ := body["features"].([]any)
		if len(rawFeatures) == 0 {
			break
		}
		for _, rf := range rawFeatures {
			fm, ok := rf.(map[string]any)
			if !ok {
				continue
			}
			attrs, _ := fm["attributes"].(map[string]any)
			if attrs == nil {
				attrs = map[string]any{}
			}
			typed = append(typed, typedRow(attrs, spec.OutFields))
			if len(typed) >= limit {
				break
			}
		}
		offset += len(rawFeatures)
		exceeded, _ := body["exceededTransferLimit"].(bool)
		if len(rawFeatures) < count && !exceeded {
			break
		}
	}
	return typed, nil
}

func typedRow(attrs map[string]any, fields []string) map[string]any {
	names := fields
	if len(names) == 0 {
		names = make([]string, 0, len(attrs))
		for k := range attrs {
			names = append(names, k)
		}
		sort.Strings(names)
	}
	row := make(map[string]any, len(names))
	for _, c := range names {
		v, ok := attrs[c]
		if !ok || v == nil {
			row[c] = nil
			continue
		}
		row[c] = collectionValue(v)
	}
	return row
}

// collectionValue preserves an ArcGIS attribute for Jasper collection fill.
// Numbers stay numbers so date coercion can see epoch milliseconds only when
// the report field is declared as a date. Strings stay strings.
func collectionValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case string, bool:
		return t
	case float64:
		return jsonNumberValue(t)
	case float32:
		return jsonNumberValue(float64(t))
	case int:
		return int64(t)
	case int64:
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return jsonNumberValue(f)
		}
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func jsonNumberValue(f float64) any {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	if f == math.Trunc(f) && f >= float64(math.MinInt64) && f <= float64(math.MaxInt64) {
		return int64(f)
	}
	return f
}
