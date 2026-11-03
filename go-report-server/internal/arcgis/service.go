package arcgis

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const schemaLayers = "layers"

// layerInfo is one queryable ArcGIS layer.
type layerInfo struct {
	ID             int
	Name           string
	SupportsQuery  bool
	MaxRecordCount int
	LayerURL       string
}

// CleanServiceURL normalizes a FeatureServer/MapServer URL (strips layer ids and query).
func CleanServiceURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(s), "http://") && !strings.HasPrefix(strings.ToLower(s), "https://") {
		s = "https://" + strings.TrimLeft(s, "/")
	}
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	u.RawQuery = ""
	u.Fragment = ""
	path := strings.TrimSuffix(u.Path, "/")
	lower := strings.ToLower(path)
	if idx := strings.Index(lower, "/featureserver/"); idx >= 0 {
		path = path[:idx+len("/FeatureServer")]
	}
	if idx := strings.Index(strings.ToLower(path), "/mapserver/"); idx >= 0 {
		path = path[:idx+len("/MapServer")]
	}
	u.Path = path
	return strings.TrimSuffix(u.String(), "/")
}

func supportsQuery(layer map[string]any) bool {
	if sq, ok := layer["supportsQuery"].(bool); ok {
		return sq
	}
	if lt, _ := layer["type"].(string); strings.EqualFold(strings.TrimSpace(lt), "Feature Layer") {
		if gt, _ := layer["geometryType"].(string); gt != "" && !strings.EqualFold(gt, "esriGeometryNull") {
			return true
		}
	}
	return false
}

func layerFromBody(id int, layerURL string, body, fallback map[string]any) layerInfo {
	name, _ := body["name"].(string)
	if strings.TrimSpace(name) == "" && fallback != nil {
		name, _ = fallback["name"].(string)
	}
	mrc := 1000
	if v, ok := body["maxRecordCount"].(float64); ok && v > 0 {
		mrc = int(v)
	}
	return layerInfo{
		ID:             id,
		Name:           name,
		SupportsQuery:  supportsQuery(body),
		MaxRecordCount: mrc,
		LayerURL:       layerURL,
	}
}

func browseService(ctx context.Context, serviceURL, username, password string) ([]layerInfo, error) {
	cleanURL := CleanServiceURL(serviceURL)
	if cleanURL == "" {
		return nil, fmt.Errorf("ArcGIS FeatureServer / MapServer URL is required")
	}
	if err := validateServiceURL(cleanURL); err != nil {
		return nil, err
	}
	token, err := getToken(ctx, cleanURL, username, password)
	if err != nil {
		return nil, err
	}
	tokenRef := token
	root, err := jsonRequest(ctx, cleanURL+"?f=pjson", cleanURL, &tokenRef, username, password)
	if err != nil {
		return nil, fmt.Errorf("browse layers failed: %w", err)
	}
	rawLayers, _ := root["layers"].([]any)
	if len(rawLayers) == 0 {
		var layers []layerInfo
		for i := 0; i <= 20; i++ {
			layerURL := fmt.Sprintf("%s/%d", cleanURL, i)
			layerBody, layerErr := jsonRequest(ctx, layerURL+"?f=pjson", cleanURL, &tokenRef, username, password)
			if layerErr != nil {
				continue
			}
			name, _ := layerBody["name"].(string)
			if strings.TrimSpace(name) == "" {
				continue
			}
			layers = append(layers, layerFromBody(i, layerURL, layerBody, nil))
		}
		return layers, nil
	}
	layers := make([]layerInfo, 0, len(rawLayers))
	for _, raw := range rawLayers {
		layer, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := asInt(layer["id"])
		layerURL := fmt.Sprintf("%s/%d", cleanURL, id)
		layerBody, layerErr := jsonRequest(ctx, layerURL+"?f=pjson", cleanURL, &tokenRef, username, password)
		if layerErr != nil {
			layerBody = layer
		}
		layers = append(layers, layerFromBody(id, layerURL, layerBody, layer))
	}
	return layers, nil
}

func resolveLayer(ctx context.Context, serviceURL, username, password, table string) (*layerInfo, string, error) {
	base := CleanServiceURL(serviceURL)
	layers, err := browseService(ctx, base, username, password)
	if err != nil {
		return nil, "", err
	}
	table = strings.TrimSpace(table)
	for i := range layers {
		l := &layers[i]
		name := strings.TrimSpace(l.Name)
		if name == "" {
			name = fmt.Sprintf("layer_%d", l.ID)
		}
		if table != "" && (strings.EqualFold(name, table) || name == table || strconv.Itoa(l.ID) == table) {
			return l, base, nil
		}
	}
	if table == "" && len(layers) == 1 {
		return &layers[0], base, nil
	}
	if table != "" {
		return nil, "", fmt.Errorf("layer %q not found on service", table)
	}
	return nil, "", fmt.Errorf("no layer selected")
}

type fieldInfo struct {
	Name       string
	IsGeometry bool
}

func mapEsriFieldType(esriType string) (isGeometry bool) {
	return strings.TrimSpace(esriType) == "esriFieldTypeGeometry"
}

func fetchLayerFields(ctx context.Context, layerURL, serviceURL, username, password string, token *string) ([]fieldInfo, error) {
	body, err := jsonRequest(ctx, strings.TrimSuffix(layerURL, "/")+"?f=pjson", serviceURL, token, username, password)
	if err != nil {
		return nil, err
	}
	geomType, _ := body["geometryType"].(string)
	rawFields, _ := body["fields"].([]any)
	out := make([]fieldInfo, 0, len(rawFields)+1)
	hasGeomField := false
	for _, raw := range rawFields {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := f["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		esriType, _ := f["type"].(string)
		isGeom := mapEsriFieldType(esriType)
		if isGeom {
			hasGeomField = true
		}
		out = append(out, fieldInfo{Name: name, IsGeometry: isGeom})
	}
	if !hasGeomField && geomType != "" && !strings.EqualFold(geomType, "esriGeometryNull") {
		out = append(out, fieldInfo{Name: "SHAPE", IsGeometry: true})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("layer has no fields")
	}
	return out, nil
}
