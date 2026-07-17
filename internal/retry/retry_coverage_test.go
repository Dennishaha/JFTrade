package retry

import (
	"errors"
	"testing"
)

func TestDoCoverageForDefaultsAndEventualSuccess(t *testing.T) {
	attempts := 0
	err := Do(func() error {
		attempts++
		return nil
	}, Config{MaxDelay: 0, MaxRetries: -3})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want one after negative retries normalize to zero", attempts)
	}

	if FutuRateLimitShouldRetry(nil) {
		t.Fatal("nil Futu error should not be retryable")
	}
}

func TestDoCoverageForEventualRetrySuccess(t *testing.T) {
	attempts := 0
	err := Do(func() error {
		attempts++
		if attempts == 1 {
			return errors.New("transient")
		}
		return nil
	}, Config{MaxRetries: 1})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want two", attempts)
	}
}
