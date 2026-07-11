package watchlist

import "errors"

var (
	ErrUnavailable          = errors.New("watchlist service is unavailable")
	ErrNotFound             = errors.New("watchlist resource not found")
	ErrConflict             = errors.New("watchlist state conflict")
	ErrValidation           = errors.New("invalid watchlist request")
	ErrProtectedGroup       = errors.New("protected watchlist group cannot be deleted")
	ErrAmbiguousRemoteGroup = errors.New("remote watchlist group name is ambiguous")
	ErrPreviewExpired       = errors.New("watchlist import preview has expired")
	ErrStalePreview         = errors.New("watchlist import preview is stale")
)
