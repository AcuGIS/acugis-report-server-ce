//go:build oss

package mapreport

import "net/url"

var MapBasemaps = []struct {
	Key   string
	Label string
}{}

var MapColorRamps []string

var MapInitialZoom = []struct {
	Key   string
	Label string
}{}

func MapDefinitionFromForm(form url.Values) map[string]any {
	return map[string]any{}
}
