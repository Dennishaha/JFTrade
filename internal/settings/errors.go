package settings

import "errors"

var ErrBadRequest = errors.New("settings bad request")

type badRequestError struct {
	message string
}

func (e badRequestError) Error() string {
	return e.message
}

func (e badRequestError) Is(target error) bool {
	return target == ErrBadRequest
}

func BadRequestError(message string) error {
	return badRequestError{message: message}
}
