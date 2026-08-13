package model

import "errors"

func InputRequestErrorKind(err error) string {
	switch {
	case errors.Is(err, ErrInputRequestInvalid):
		return "invalid"
	case errors.Is(err, ErrInputRequestNotFound):
		return "not_found"
	case errors.Is(err, ErrInputRequestAlreadyAnswered), errors.Is(err, ErrInputRequestConflict):
		return "conflict"
	default:
		return "internal"
	}
}
