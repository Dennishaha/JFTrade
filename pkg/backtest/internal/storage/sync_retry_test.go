package storage

import (
	"errors"
	"strings"
	"testing"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/internal/retry"
)

func TestRateLimitRetryExhaustsAndTracksRetries(t *testing.T) {
	progress := NewSyncProgress("retry-exhaust", "HK.00700", time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC))
	calls := 0
	cfg := syncRetryConfig(progress)
	cfg.BaseDelay = 0 // eliminate sleeps in test
	err := retry.Do(func() error {
		calls++
		return errors.New("retType=-1: 频率太高")
	}, cfg)
	if err == nil {
		t.Fatal("expected rate-limit retry to fail")
	}
	if !strings.Contains(err.Error(), "retry exhausted after 3 retries") {
		t.Fatalf("unexpected retry exhaustion error: %v", err)
	}
	if calls != 4 {
		t.Fatalf("retry calls = %d, want 4", calls)
	}

	snapshot := progress.Snapshot()
	if snapshot == nil {
		t.Fatal("expected retry snapshot")
	}
	if snapshot.Retries != 3 {
		t.Fatalf("retry count = %d, want 3", snapshot.Retries)
	}
}

func TestRateLimitRetryHandlesNilProgress(t *testing.T) {
	calls := 0
	cfg := syncRetryConfig(nil)
	cfg.BaseDelay = 0 // eliminate sleeps in test
	cfg.MaxRetries = 2
	err := retry.Do(func() error {
		calls++
		if calls == 1 {
			return errors.New("retType=-1: 频率太高")
		}
		return nil
	}, cfg)
	if err != nil {
		t.Fatalf("retry.Do() with nil progress error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("retry calls with nil progress = %d, want 2", calls)
	}
}

func TestRateLimitRetryReturnsImmediatelyForNonRetryableError(t *testing.T) {
	progress := NewSyncProgress("retry-non-retryable", "HK.00700", time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC))
	wantErr := errors.New("temporary network reset")
	calls := 0
	cfg := syncRetryConfig(progress)
	cfg.BaseDelay = 0 // eliminate sleeps in test
	err := retry.Do(func() error {
		calls++
		return wantErr
	}, cfg)
	if !errors.Is(err, wantErr) {
		t.Fatalf("retry.Do() error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("non-retryable calls = %d, want 1", calls)
	}

	snapshot := progress.Snapshot()
	if snapshot == nil {
		t.Fatal("expected non-retryable snapshot")
	}
	if snapshot.Retries != 0 {
		t.Fatalf("non-retryable retries = %d, want 0", snapshot.Retries)
	}
}

func TestSyncHistoryRequestEndTimeAlignsIntradayRequestsToClosedLabelTime(t *testing.T) {
	requestedEnd := time.Date(2026, time.May, 20, 23, 59, 59, 999000000, time.UTC)

	got := syncHistoryRequestEndTime(bbgotypes.Interval1m, requestedEnd)
	want := time.Date(2026, time.May, 21, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("syncHistoryRequestEndTime(1m) = %s, want %s", got, want)
	}
}

func TestSyncHistoryRequestEndTimeKeepsDailyRequestsOnClosedBoundary(t *testing.T) {
	requestedEnd := time.Date(2026, time.May, 20, 23, 59, 59, 999000000, time.UTC)

	got := syncHistoryRequestEndTime(bbgotypes.Interval1d, requestedEnd)
	if !got.Equal(requestedEnd) {
		t.Fatalf("syncHistoryRequestEndTime(1d) = %s, want %s", got, requestedEnd)
	}
}
