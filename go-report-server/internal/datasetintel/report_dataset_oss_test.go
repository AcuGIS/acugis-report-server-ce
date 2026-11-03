//go:build oss

package datasetintel

import (
	"reflect"
	"testing"
)

func TestOSSBuildExcludesReportDatasets(t *testing.T) {
	rt := reflect.TypeOf(&Service{})
	for _, name := range []string{"DiscoverReportDatasets", "ProfileReportDataset", "ListReportDatasets"} {
		if _, ok := rt.MethodByName(name); ok {
			t.Fatalf("OSS build includes %s", name)
		}
	}
}
