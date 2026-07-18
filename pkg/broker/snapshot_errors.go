package broker

import (
	"errors"
	"fmt"
	"time"
)

// ErrSnapshotRateLimited marks a non-streaming snapshot request that was not
// sent because the broker's shared request budget was exhausted.
var ErrSnapshotRateLimited = errors.New("broker snapshot rate limited")

// SnapshotRateLimitError carries the remaining time until another snapshot
// call can be attempted. It intentionally keeps the broker-neutral sentinel
// in this package so API and UI layers do not need to depend on an adapter.
type SnapshotRateLimitError struct {
	retryAfter time.Duration
	cause      error
}

func (e *SnapshotRateLimitError) Error() string {
	if e == nil {
		return ErrSnapshotRateLimited.Error()
	}
	if e.cause != nil {
		return e.cause.Error()
	}
	return fmt.Sprintf("%s; retry after %s", ErrSnapshotRateLimited, e.retryAfter.Round(time.Millisecond))
}

func (e *SnapshotRateLimitError) Unwrap() error { return ErrSnapshotRateLimited }

// NewSnapshotRateLimitError constructs a rate-limit error. Non-positive retry
// values are normalized to one second so HTTP Retry-After never advertises an
// immediate retry that would be rejected again.
func NewSnapshotRateLimitError(retryAfter time.Duration, cause error) error {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	return &SnapshotRateLimitError{retryAfter: retryAfter, cause: cause}
}

// SnapshotRetryAfter extracts the retry delay from a wrapped rate-limit error.
func SnapshotRetryAfter(err error) (time.Duration, bool) {
	var target *SnapshotRateLimitError
	if !errors.As(err, &target) || target == nil {
		return 0, false
	}
	return target.retryAfter, true
}

// SymbolScopedSnapshotError marks a batch snapshot failure caused by one or
// more symbols in the request. Callers may isolate the failing symbols by
// retrying smaller batches; transport and service errors must remain unmarked.
type SymbolScopedSnapshotError struct {
	err error
}

func (e *SymbolScopedSnapshotError) Error() string { return e.err.Error() }
func (e *SymbolScopedSnapshotError) Unwrap() error { return e.err }

func NewSymbolScopedSnapshotError(err error) error {
	if err == nil {
		return nil
	}
	return &SymbolScopedSnapshotError{err: err}
}

func IsSymbolScopedSnapshotError(err error) bool {
	var target *SymbolScopedSnapshotError
	return errors.As(err, &target)
}
