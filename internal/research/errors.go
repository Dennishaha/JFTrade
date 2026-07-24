package research

import "errors"

var (
	ErrUnavailable = errors.New("research preset store is unavailable")
	ErrNotFound    = errors.New("research screen preset not found")
	ErrConflict    = errors.New("research screen preset conflict")
	ErrValidation  = errors.New("invalid research screen preset")
)
