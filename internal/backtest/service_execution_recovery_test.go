package backtest

import (
	"context"
	"strings"
	"testing"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestBacktestExecutionPersistsFailureWhenRunnerReturnsNil(t *testing.T) {
	runs := newMemoryRunStore()
	service := newTestBacktestService(runs, func(context.Context, bt.RunConfig) *bt.RunResult {
		return nil
	})

	started, err := service.Start(context.Background(), validStartRequest())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	finished := waitForRunStatus(t, runs, started.ID, "failed")
	if finished.Result == nil || finished.Result.Error != "backtest returned no result" {
		t.Fatalf("nil-runner result = %#v", finished.Result)
	}
}

func TestBacktestExecutionRecoversRunnerPanicIntoFailedRun(t *testing.T) {
	runs := newMemoryRunStore()
	service := newTestBacktestService(runs, func(context.Context, bt.RunConfig) *bt.RunResult {
		panic("indicator state corrupted")
	})

	started, err := service.Start(context.Background(), validStartRequest())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	finished := waitForRunStatus(t, runs, started.ID, "failed")
	if finished.Result == nil || !strings.Contains(finished.Result.Error, "backtest panic: indicator state corrupted") {
		t.Fatalf("panic recovery result = %#v", finished.Result)
	}
}

func TestStartScriptRejectsBlankResearchScript(t *testing.T) {
	service := NewService()
	if _, err := service.StartScript(context.Background(), ScriptStartRequest{Script: " \n\t "}); err == nil || !IsRequestError(err) || !strings.Contains(err.Error(), "script is required") {
		t.Fatalf("StartScript blank err = %v", err)
	}
}
