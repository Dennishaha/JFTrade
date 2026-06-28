package backtest

import (
	"context"
	"strings"
	"testing"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestServiceDefaultBacktestRequiresPineWorkerRunner(t *testing.T) {
	svc := NewService()
	result := svc.runBacktest(context.Background(), bt.RunConfig{
		Symbol:   "US.AAPL",
		Interval: "1m",
	})
	if result == nil {
		t.Fatal("runBacktest returned nil")
	}
	if !strings.Contains(result.Error, "pine worker runner is not configured") {
		t.Fatalf("default runBacktest error = %q", result.Error)
	}
}

func TestServiceDefaultBacktestUsesConfiguredPineWorkerRunner(t *testing.T) {
	svc := NewService(WithPineWorkerRunner(fakeServicePineWorkerRunner{}))
	result := svc.runBacktest(context.Background(), bt.RunConfig{
		DBPath:   "/path/does/not/exist.db",
		Symbol:   "US.AAPL",
		Interval: "1m",
	})
	if result == nil {
		t.Fatal("runBacktest returned nil")
	}
	if !strings.Contains(result.Error, "backtest database not found") {
		t.Fatalf("runBacktest error = %q, want DB error from RunWithPineWorker", result.Error)
	}
}

type fakeServicePineWorkerRunner struct{}

func (fakeServicePineWorkerRunner) RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	return pineworker.RunScriptResponse{}, nil
}
