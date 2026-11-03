package schedules

import "errors"

var (
	ErrNotFound     = errors.New("schedule not found")
	ErrUnavailable  = errors.New("schedule database unavailable")
	ErrTableMissing = errors.New("jasper_schedule table missing")
)
