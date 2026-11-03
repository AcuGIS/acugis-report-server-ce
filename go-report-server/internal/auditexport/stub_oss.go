//go:build oss

package auditexport

type Service struct{}

func (s *Service) Configured() bool { return false }
