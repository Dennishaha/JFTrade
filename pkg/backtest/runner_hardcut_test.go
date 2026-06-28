package backtest

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunDirectGoPineRunnerIsDisabled(t *testing.T) {
	result := Run(context.Background(), RunConfig{
		Symbol:    "US.AAPL",
		Interval:  "1m",
		StartTime: time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC),
		EndTime:   time.Date(2026, time.June, 29, 13, 35, 0, 0, time.UTC),
	})

	if result == nil {
		t.Fatal("Run returned nil")
	}
	if !strings.Contains(result.Error, "direct Go Pine backtest runner has been removed") {
		t.Fatalf("Run error = %q", result.Error)
	}
}
