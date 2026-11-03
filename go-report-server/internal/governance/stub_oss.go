//go:build oss

package governance

type Service struct{}

func (s *Service) Configured() bool { return false }

func (s *Service) Delete(ctx interface{}, objectType, objectID string) error { return nil }

const (
	ObjectReport        = "report"
	ObjectDataset       = "dataset"
	ObjectReportDataset = "report_dataset"
	ObjectView          = "view"
	ObjectDashboard     = "dashboard"
)
