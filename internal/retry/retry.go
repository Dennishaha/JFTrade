// Package retry provides a thin exponential-backoff retry helper implemented
// with the standard library only. It preserves shouldRetry / notifyRetry
// hooks so callers retain control over which errors are retryable and how
// progress is reported.
package retry

import (
	"fmt"
	"strings"
	"time"
)

// ShouldRetryFunc decides whether err should trigger a retry.
// When nil the default is to retry every non-nil error.
type ShouldRetryFunc func(err error) bool

// NotifyFunc is called before each retry sleep (not on the initial attempt).
// attempt counts retries starting from 1.  delay is the sleep duration
// before the next attempt.
type NotifyFunc func(attempt int, err error, delay time.Duration)

// Config describes an exponential-backoff retry policy.
type Config struct {
	// BaseDelay is the initial wait before the first retry.
	BaseDelay time.Duration

	// MaxDelay caps the exponential growth.
	MaxDelay time.Duration

	// MaxRetries is the maximum number of retry attempts (beyond the
	// initial try).  The total number of attempts is MaxRetries+1.
	MaxRetries int

	// ShouldRetry is checked before each retry.  When nil every
	// non-nil error triggers a retry.
	ShouldRetry ShouldRetryFunc

	// Notify is called before each retry sleep.
	Notify NotifyFunc
}

// Do executes operation with up to cfg.MaxRetries+1 total attempts.
//
// Exponential-backoff delays double deterministically from BaseDelay and are
// capped at MaxDelay. A zero BaseDelay is valid and disables sleeps, which
// keeps tests fast while production callers can still opt into real waiting
// explicitly. cfg.ShouldRetry is consulted after each failure; if it returns
// false the error is returned immediately without further retries.
// cfg.Notify is called before each retry sleep (not on the initial
// attempt) so callers can record progress (e.g. IncrementRetries).
func Do(operation func() error, cfg Config) error {
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 30 * time.Second
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(0)
			if cfg.BaseDelay > 0 {
				delay = cfg.BaseDelay << (attempt - 1)
				if delay > cfg.MaxDelay || delay < cfg.BaseDelay {
					delay = cfg.MaxDelay
				}
			}
			if cfg.Notify != nil {
				cfg.Notify(attempt, lastErr, delay)
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		lastErr = operation()
		if lastErr == nil {
			return nil
		}
		if cfg.ShouldRetry != nil && !cfg.ShouldRetry(lastErr) {
			return lastErr
		}
	}
	return fmt.Errorf("retry exhausted after %d retries: %w", cfg.MaxRetries, lastErr)
}

// FutuRateLimitShouldRetry returns true when err originates from a Futu
// OpenD rate-limit response (Chinese "频率太高" message or retType=-1).
func FutuRateLimitShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "频率太高") || strings.Contains(s, "retType=-1")
}
