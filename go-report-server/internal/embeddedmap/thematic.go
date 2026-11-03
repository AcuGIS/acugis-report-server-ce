package embeddedmap

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// StyledFeature carries draw styles for a GeoJSON feature.
type StyledFeature struct {
	Feature map[string]any
	Fill    string
	Stroke  string
	Size    float64 // point radius / graduated size
}

// LegendEntry is one legend row.
type LegendEntry struct {
	Color string
	Label string
}

// ThematicResult is symbology preparation output.
type ThematicResult struct {
	Features        []StyledFeature
	ContextFeatures []StyledFeature
	ZoomFeatures    []map[string]any
	Legend          []LegendEntry
	Title           string
}

// PrepareThematic applies map-type symbology to a feature collection.
func PrepareThematic(features []map[string]any, context []map[string]any, emb Definition, reportName string) ThematicResult {
	opts := emb
	title := opts.Title
	if title == "" && opts.ShowTitle {
		title = reportName
	}

	switch opts.MapType {
	case "choropleth", "graduated_symbols":
		return prepareNumeric(features, opts, title)
	case "categorized":
		return prepareCategorized(features, opts, title)
	case "overview_highlight", "selected_features":
		return prepareHighlight(features, context, opts, title, opts.MapType == "overview_highlight")
	default: // locator
		return prepareLocator(features, context, opts, title)
	}
}

func prepareLocator(features, context []map[string]any, emb Definition, title string) ThematicResult {
	out := ThematicResult{Title: title, ZoomFeatures: features}
	fill := "#2b6cb0"
	stroke := "#1a365d"
	for _, f := range features {
		out.Features = append(out.Features, StyledFeature{Feature: f, Fill: fill, Stroke: stroke, Size: 5})
	}
	if emb.GrayContext && len(context) > 0 {
		for _, f := range context {
			out.ContextFeatures = append(out.ContextFeatures, StyledFeature{
				Feature: f, Fill: "#d0d0d0", Stroke: "#a0a0a0", Size: 4,
			})
		}
	}
	if emb.IncludeLegend {
		out.Legend = []LegendEntry{{Color: fill, Label: "Features"}}
	}
	return out
}

func prepareHighlight(features, context []map[string]any, emb Definition, title string, forceGray bool) ThematicResult {
	out := ThematicResult{Title: title, ZoomFeatures: features}
	fill := "#e53e3e"
	stroke := "#9b2c2c"
	for _, f := range features {
		out.Features = append(out.Features, StyledFeature{Feature: f, Fill: fill, Stroke: stroke, Size: 6})
	}
	if forceGray || emb.GrayContext {
		for _, f := range context {
			out.ContextFeatures = append(out.ContextFeatures, StyledFeature{
				Feature: f, Fill: "#cfcfcf", Stroke: "#9e9e9e", Size: 4,
			})
		}
	}
	if emb.IncludeLegend {
		out.Legend = []LegendEntry{{Color: fill, Label: "Selected"}}
	}
	return out
}

func prepareNumeric(features []map[string]any, emb Definition, title string) ThematicResult {
	out := ThematicResult{Title: title, ZoomFeatures: features}
	field := emb.Field
	values := make([]float64, 0, len(features))
	for _, f := range features {
		if v, ok := featureNumber(f, field); ok {
			values = append(values, v)
		}
	}
	if len(values) == 0 {
		return prepareLocator(features, nil, emb, title)
	}
	breaks := classBreaks(values, emb.Classes, emb.Classification)
	ramp := ColorRamps[emb.ColorRamp]
	if len(ramp) == 0 {
		ramp = ColorRamps["Blue"]
	}
	colors := interpolateRamp(ramp, len(breaks)-1)

	for _, f := range features {
		v, ok := featureNumber(f, field)
		idx := 0
		if ok {
			idx = classIndex(v, breaks)
		}
		if idx < 0 {
			idx = 0
		}
		if idx >= len(colors) {
			idx = len(colors) - 1
		}
		size := 5.0
		if emb.MapType == "graduated_symbols" {
			size = 3 + float64(idx)*2.5
		}
		out.Features = append(out.Features, StyledFeature{
			Feature: f, Fill: colors[idx], Stroke: darken(colors[idx]), Size: size,
		})
	}
	if emb.IncludeLegend {
		for i := 0; i < len(colors); i++ {
			lo, hi := breaks[i], breaks[i+1]
			out.Legend = append(out.Legend, LegendEntry{
				Color: colors[i],
				Label: fmt.Sprintf("%.2f - %.2f", lo, hi),
			})
		}
	}
	if emb.ShowTitle && emb.Title == "" && field != "" {
		out.Title = field
	}
	return out
}

func prepareCategorized(features []map[string]any, emb Definition, title string) ThematicResult {
	out := ThematicResult{Title: title, ZoomFeatures: features}
	field := emb.Field
	cats := uniqueCategories(features, field)
	if len(cats) == 0 {
		return prepareLocator(features, nil, emb, title)
	}
	if len(cats) > 12 {
		cats = cats[:12]
	}
	ramp := ColorRamps[emb.ColorRamp]
	if len(ramp) == 0 {
		ramp = ColorRamps["Blue"]
	}
	colors := interpolateRamp(ramp, len(cats))
	colorBy := map[string]string{}
	for i, c := range cats {
		colorBy[c] = colors[i]
	}
	other := "#9e9e9e"
	for _, f := range features {
		cat := featureString(f, field)
		col, ok := colorBy[cat]
		if !ok {
			col = other
		}
		out.Features = append(out.Features, StyledFeature{
			Feature: f, Fill: col, Stroke: darken(col), Size: 5,
		})
	}
	if emb.IncludeLegend {
		for _, c := range cats {
			out.Legend = append(out.Legend, LegendEntry{Color: colorBy[c], Label: c})
		}
	}
	return out
}

func featureProps(f map[string]any) map[string]any {
	if p, ok := f["properties"].(map[string]any); ok {
		return p
	}
	return nil
}

func featureNumber(f map[string]any, field string) (float64, bool) {
	props := featureProps(f)
	if props == nil || field == "" {
		return 0, false
	}
	v, ok := props[field]
	if !ok {
		// try unqualified
		if i := strings.LastIndex(field, "."); i >= 0 {
			v, ok = props[field[i+1:]]
		}
	}
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return n, err == nil
	}
	return 0, false
}

func featureString(f map[string]any, field string) string {
	props := featureProps(f)
	if props == nil {
		return ""
	}
	v, ok := props[field]
	if !ok {
		return ""
	}
	return strings.TrimSpace(asString(v))
}

func uniqueCategories(features []map[string]any, field string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range features {
		c := featureString(f, field)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func classBreaks(values []float64, n int, method string) []float64 {
	if n < 3 {
		n = 3
	}
	if n > 7 {
		n = 7
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	minV, maxV := sorted[0], sorted[len(sorted)-1]
	if minV == maxV {
		return []float64{minV, maxV}
	}
	switch method {
	case "equal_interval":
		step := (maxV - minV) / float64(n)
		b := make([]float64, n+1)
		for i := 0; i <= n; i++ {
			b[i] = minV + step*float64(i)
		}
		b[n] = maxV
		return b
	case "natural_breaks":
		return jenksBreaks(sorted, n)
	default: // quantile
		b := make([]float64, n+1)
		b[0] = minV
		b[n] = maxV
		for i := 1; i < n; i++ {
			pos := float64(i) * float64(len(sorted)-1) / float64(n)
			lo := int(math.Floor(pos))
			hi := int(math.Ceil(pos))
			if lo == hi {
				b[i] = sorted[lo]
			} else {
				frac := pos - float64(lo)
				b[i] = sorted[lo]*(1-frac) + sorted[hi]*frac
			}
		}
		return b
	}
}

func classIndex(v float64, breaks []float64) int {
	if len(breaks) < 2 {
		return 0
	}
	for i := 0; i < len(breaks)-1; i++ {
		if i == len(breaks)-2 {
			if v >= breaks[i] && v <= breaks[i+1] {
				return i
			}
		} else if v >= breaks[i] && v < breaks[i+1] {
			return i
		}
	}
	return len(breaks) - 2
}

func interpolateRamp(ramp []string, n int) []string {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []string{ramp[len(ramp)/2]}
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		pos := t * float64(len(ramp)-1)
		lo := int(math.Floor(pos))
		hi := int(math.Ceil(pos))
		if lo == hi {
			out[i] = ramp[lo]
		} else {
			out[i] = mixHex(ramp[lo], ramp[hi], pos-float64(lo))
		}
	}
	return out
}

func mixHex(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	r := int(float64(ar)*(1-t) + float64(br)*t)
	g := int(float64(ag)*(1-t) + float64(bg)*t)
	bl := int(float64(ab)*(1-t) + float64(bb)*t)
	return fmt.Sprintf("#%02x%02x%02x", r, g, bl)
}

func parseHex(c string) (r, g, b int) {
	c = strings.TrimPrefix(strings.TrimSpace(c), "#")
	if len(c) != 6 {
		return 0, 0, 0
	}
	var rv, gv, bv int64
	rv, _ = strconv.ParseInt(c[0:2], 16, 0)
	gv, _ = strconv.ParseInt(c[2:4], 16, 0)
	bv, _ = strconv.ParseInt(c[4:6], 16, 0)
	return int(rv), int(gv), int(bv)
}

func darken(c string) string {
	r, g, b := parseHex(c)
	return fmt.Sprintf("#%02x%02x%02x", r*2/3, g*2/3, b*2/3)
}

// Simplified 1D Jenks natural breaks.
func jenksBreaks(sorted []float64, nClasses int) []float64 {
	n := len(sorted)
	if nClasses >= n {
		b := make([]float64, n)
		copy(b, sorted)
		return append([]float64{sorted[0]}, append(b[1:], sorted[n-1])...)
	}
	// Use quantile as a pragmatic approximation when n is large; full Jenks is O(n^2*k).
	if n > 500 {
		return classBreaks(sorted, nClasses, "quantile")
	}
	mat1 := make([][]int, n+1)
	mat2 := make([][]float64, n+1)
	for i := 0; i <= n; i++ {
		mat1[i] = make([]int, nClasses+1)
		mat2[i] = make([]float64, nClasses+1)
		for j := 0; j <= nClasses; j++ {
			mat2[i][j] = math.Inf(1)
		}
	}
	for j := 1; j <= nClasses; j++ {
		mat1[0][j] = 1
		mat2[0][j] = 0
	}
	for l := 2; l <= n; l++ {
		s1, s2, w := 0.0, 0.0, 0.0
		for m := 1; m <= l; m++ {
			i3 := l - m + 1
			val := sorted[i3-1]
			s2 += val * val
			s1 += val
			w++
			v := s2 - (s1*s1)/w
			i4 := i3 - 1
			if i4 != 0 {
				for j := 2; j <= nClasses; j++ {
					if mat2[l][j] >= v+mat2[i4][j-1] {
						mat1[l][j] = i3
						mat2[l][j] = v + mat2[i4][j-1]
					}
				}
			}
		}
		mat1[l][1] = 1
		mat2[l][1] = s2 - (s1*s1)/w
	}
	k := n
	breaks := make([]float64, nClasses+1)
	breaks[nClasses] = sorted[n-1]
	breaks[0] = sorted[0]
	for j := nClasses; j >= 2; j-- {
		id := mat1[k][j] - 2
		if id < 0 {
			id = 0
		}
		breaks[j-1] = sorted[id]
		k = mat1[k][j] - 1
		if k < 1 {
			k = 1
		}
	}
	return breaks
}
