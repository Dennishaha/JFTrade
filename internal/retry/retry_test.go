package retry

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestDoRetriesWithDeterministicBackoff(t *testing.T) {
	retryableErr := errors.New("retType=-1: rate limited")
	calls := 0
	var delays []time.Duration

	err := Do(func() error {
		calls++
		return retryableErr
	}, Config{
		BaseDelay:   time.Millisecond,
		MaxDelay:    2 * time.Millisecond,
		MaxRetries:  3,
		ShouldRetry: func(error) bool { return true },
		Notify: func(attempt int, err error, delay time.Duration) {
			delays = append(delays, delay)
		},
	})

	if err == nil {
		t.Fatal("expected retry exhaustion error")
	}
	if calls != 4 {
		t.Fatalf("calls = %d, want 4", calls)
	}
	wantDelays := []time.Duration{time.Millisecond, 2 * time.Millisecond, 2 * time.Millisecond}
	if !reflect.DeepEqual(delays, wantDelays) {
		t.Fatalf("delays = %v, want %v", delays, wantDelays)
	}
}

func TestDoZeroBaseDelayDoesNotSleep(t *testing.T) {
	start := time.Now()
	err := Do(func() error {
		return errors.New("retType=-1: rate limited")
	}, Config{
		BaseDelay:   0,
		MaxRetries:  3,
		ShouldRetry: func(error) bool { return true },
	})
	if err == nil {
		t.Fatal("expected retry exhaustion error")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("zero base delay slept too long: %s", elapsed)
	}
}

func TestDoReturnsNonRetryableErrorImmediately(t *testing.T) {
	wantErr := errors.New("plain network error")
	calls := 0
	err := Do(func() error {
		calls++
		return wantErr
	}, Config{
		BaseDelay:   time.Millisecond,
		MaxRetries:  3,
		ShouldRetry: func(error) bool { return false },
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestFutuRateLimitShouldRetry(t *testing.T) {
	if !FutuRateLimitShouldRetry(errors.New("retType=-1: request failed")) {
		t.Fatal("expected retType=-1 to be retryable")
	}
	if !FutuRateLimitShouldRetry(errors.New("频率太高")) {
		t.Fatal("expected Chinese rate-limit message to be retryable")
	}
	if FutuRateLimitShouldRetry(errors.New("connection refused")) {
		t.Fatal("expected non-rate-limit error to be non-retryable")
	}
}
