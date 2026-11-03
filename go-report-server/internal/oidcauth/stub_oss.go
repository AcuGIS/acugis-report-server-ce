//go:build oss

package oidcauth

import (
	"context"

	"github.com/acugis/acugis-report-server-go/internal/extid"
)

type Service struct{}

type Settings struct {
	ProviderName     string
	LocalAuthEnabled bool
}

func (s *Service) FeatureAvailable() bool { return false }
func (s *Service) EndSessionURL(context.Context, string) (string, bool) {
	return "", false
}
func (s *Service) LoadSettings(context.Context) (Settings, error) {
	return Settings{LocalAuthEnabled: true}, nil
}
func (s *Service) ExtRepository() *extid.Repository { return nil }

func (Settings) ReadyForLogin() bool { return false }
