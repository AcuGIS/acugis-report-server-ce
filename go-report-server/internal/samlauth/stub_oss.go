//go:build oss

package samlauth

import "context"

type Service struct{}

type Settings struct {
	ProviderName string
}

func (s *Service) FeatureAvailable() bool { return false }
func (s *Service) LoadSettings(context.Context) (Settings, error) {
	return Settings{}, nil
}

func (Settings) ReadyForLogin() bool { return false }
