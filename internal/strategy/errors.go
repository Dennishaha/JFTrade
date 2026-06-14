package strategy

import "errors"

var (
	ErrBadRequest = errors.New("strategy bad request")
	ErrBusy       = errors.New("strategy busy")
	ErrNotFound   = errors.New("strategy not found")
	ErrUpstream   = errors.New("strategy upstream failed")
)

type classifiedError struct {
	kind    error
	message string
}

func (e classifiedError) Error() string {
	return e.message
}

func (e classifiedError) Is(target error) bool {
	return target == e.kind
}

func BadRequestError(message string) error {
	return classifiedError{kind: ErrBadRequest, message: message}
}

func BusyError(message string) error {
	return classifiedError{kind: ErrBusy, message: message}
}

func NotFoundError(message string) error {
	return classifiedError{kind: ErrNotFound, message: message}
}

func UpstreamError(message string) error {
	return classifiedError{kind: ErrUpstream, message: message}
}
