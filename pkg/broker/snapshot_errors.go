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

// SnapshotAvailabilityKind describes a known quote-access failure that can be
// served by a broker's non-streaming fallback path. It intentionally excludes
// transport, cancellation, timeout, and request-rate errors.
type SnapshotAvailabilityKind string

const (
	SnapshotAvailabilityEntitlement SnapshotAvailabilityKind = "entitlement"
	SnapshotAvailabilityUnsupported SnapshotAvailabilityKind = "unsupported"
	SnapshotAvailabilityQuota       SnapshotAvailabilityKind = "subscription_quota"
)

// SnapshotAvailabilityError preserves the upstream message while making the
// fallback eligibility available without parsing adapter-specific text.
type SnapshotAvailabilityError struct {
	kind  SnapshotAvailabilityKind
	cause error
}

func (e *SnapshotAvailabilityError) Error() string {
	if e == nil || e.cause == nil {
		return "broker snapshot availability is unavailable"
	}
	return e.cause.Error()
}

func (e *SnapshotAvailabilityError) Unwrap() error { return e.cause }

// NewSnapshotAvailabilityError annotates a known availability failure.
func NewSnapshotAvailabilityError(kind SnapshotAvailabilityKind, cause error) error {
	if cause == nil {
		return nil
	}
	return &SnapshotAvailabilityError{kind: kind, cause: cause}
}

// SnapshotAvailability extracts the adapter-neutral availability kind.
func SnapshotAvailability(err error) (SnapshotAvailabilityKind, bool) {
	var target *SnapshotAvailabilityError
	if !errors.As(err, &target) || target == nil {
		return "", false
	}
	return target.kind, true
}

// IsSnapshotFallbackEligible reports whether an error is safe to route to a
// broker-provided delayed snapshot source.
func IsSnapshotFallbackEligible(err error) bool {
	kind, ok := SnapshotAvailability(err)
	if !ok {
		return false
	}
	switch kind {
	case SnapshotAvailabilityEntitlement, SnapshotAvailabilityUnsupported, SnapshotAvailabilityQuota:
		return true
	default:
		return false
	}
}
