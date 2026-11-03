//go:build oss

package modules

import "testing"

func TestOSSOmitsProfessionalModules(t *testing.T) {
	for _, id := range []ID{
		IDAdHoc, IDDashboards, IDMapReports, IDQGIS, IDArcGIS,
		IDDataIntel, IDAudit, IDUsage, IDGovernance, IDAuditExport, IDSSO,
	} {
		if Has(id) {
			t.Errorf("OSS build registered professional module %s", id)
		}
	}
}
