//go:build oss

package embeddedmap

import (
	"context"
	"fmt"

	"github.com/acugis/acugis-report-server-go/internal/qgis"
)

func (s *Service) renderQGISPNG(context.Context, Definition, string) ([]byte, error) {
	return nil, fmt.Errorf("QGIS embedded maps are not included in this build")
}

func SnapshotQGISBaseURL(context.Context, qgis.Store, *Definition) {}
