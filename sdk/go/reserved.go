package report

// ReservedService is a placeholder for future Users/Groups/Schedules/Jobs/Recipients APIs.
type ReservedService struct {
	name string
}

// NotImplemented always returns ErrNotImplemented.
func (s *ReservedService) NotImplemented() error {
	return ErrNotImplemented
}
