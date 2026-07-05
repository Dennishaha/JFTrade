package backtest

import (
	"errors"
	"strings"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestServiceQueryMethodsHandleNilStoresAndFullList(t *testing.T) {
	empty := NewService()
	if empty.List() != nil || empty.ListFull() != nil {
		t.Fatalf("nil run store lists = %#v / %#v, want nil", empty.List(), empty.ListFull())
	}
	if status, ok := empty.GetStatus("missing"); status != nil || ok {
		t.Fatalf("nil run store GetStatus = %#v/%v, want nil/false", status, ok)
	}
	if result, ok, err := empty.GetResult("missing"); result != nil || ok || err == nil || !strings.Contains(err.Error(), "run store not configured") {
		t.Fatalf("nil run store GetResult = %#v/%v/%v", result, ok, err)
	}
	if deleted, ok, err := empty.Delete("missing"); deleted != nil || ok || err == nil || !strings.Contains(err.Error(), "run store not configured") {
		t.Fatalf("nil run store Delete = %#v/%v/%v", deleted, ok, err)
	}
	if empty.Cancel("missing") {
		t.Fatal("nil run store Cancel = true, want false")
	}
	if progress, ok := empty.GetSyncProgress("missing"); progress != nil || ok {
		t.Fatalf("nil sync store GetSyncProgress = %#v/%v, want nil/false", progress, ok)
	}
	if progress, ok := empty.CancelSync("missing"); progress != nil || ok {
		t.Fatalf("nil sync store CancelSync = %#v/%v, want nil/false", progress, ok)
	}

	runs := newMemoryRunStore()
	run := &RunState{
		ID:        "full-run",
		Status:    "completed",
		Request:   validStartRequest(),
		Result:    &bt.RunResult{Symbol: "US.AAPL", FinalBalance: 12345},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runs.Add(run); err != nil {
		t.Fatalf("runs.Add() error = %v", err)
	}
	withRuns := NewService(WithRunStore(runs))
	full := withRuns.ListFull()
	if len(full) != 1 || full[0].Result == nil || full[0].Result.FinalBalance != 12345 {
		t.Fatalf("ListFull() = %#v, want complete run result", full)
	}
	light := withRuns.List()
	if len(light) != 1 || light[0].Result != nil {
		t.Fatalf("List() = %#v, want lightweight run without result", light)
	}
}

func TestServiceCoverageAndNormalizationBoundaries(t *testing.T) {
	inner := errors.New("invalid request")
	reqErr := requestErrorf("%w", inner)
	if !IsRequestError(reqErr) || !errors.Is(reqErr, inner) || errors.Unwrap(reqErr) == nil {
		t.Fatalf("request error wrapping = %v", reqErr)
	}

	expectedCoverageErr := errors.New("missing K-line coverage")
	checkCalls := 0
	svc := NewService(
		WithDBPathFn(func() string { return "/tmp/coverage-check.db" }),
		WithKLineCoverageCheckFn(func(dbPath, symbol, interval string, since, until time.Time, rehabType, sessionScope string) error {
			checkCalls++
			if dbPath != "/tmp/coverage-check.db" || symbol != "US.AAPL" || interval != "1m" || rehabType != "forward" || sessionScope != "regular" {
				t.Fatalf("coverage args = %s %s %s %s %s", dbPath, symbol, interval, rehabType, sessionScope)
			}
			if checkCalls == 1 {
				return nil
			}
			return expectedCoverageErr
		}),
	)
	since := time.Date(2024, time.January, 2, 14, 30, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	covered, err := svc.hasKLineCoverage("US.AAPL", "1m", since, until, "forward", "regular")
	if err != nil || !covered {
		t.Fatalf("hasKLineCoverage(success) = %v/%v", covered, err)
	}
	covered, err = svc.hasKLineCoverage("US.AAPL", "1m", since, until, "forward", "regular")
	if !errors.Is(err, expectedCoverageErr) || covered {
		t.Fatalf("hasKLineCoverage(error) = %v/%v", covered, err)
	}

	noChecker := NewService(WithDBPathFn(func() string { return "/dev/null/jftrade-backtest.db" }))
	covered, err = noChecker.hasKLineCoverage("US.AAPL", "1m", since, until, "forward", "regular")
	if err == nil || covered || !strings.Contains(err.Error(), "open backtest store") {
		t.Fatalf("hasKLineCoverage(open failure) = %v/%v", covered, err)
	}

	if normalizeRehabTypeName(" none ") != "none" || normalizeRehabTypeName("BACKWARD") != "backward" || normalizeRehabTypeName("bad") != "forward" {
		t.Fatalf("normalizeRehabTypeName returned unexpected values")
	}
	if normalizeBacktestInstrumentType(" ETF ") != "etf" || normalizeBacktestInstrumentType("fund") != "etf" || normalizeBacktestInstrumentType("option") != "stock" {
		t.Fatalf("normalizeBacktestInstrumentType returned unexpected values")
	}
	useExtended := true
	useRegular := false
	if backtestReadSessionScope(nil) != "auto" || backtestReadSessionScope(&useExtended) != "extended" || backtestReadSessionScope(&useRegular) != "regular" {
		t.Fatalf("backtestReadSessionScope returned unexpected values")
	}
	if backtestSyncSessionScope(nil) != "legacy" || backtestSyncSessionScope(&useExtended) != "extended" {
		t.Fatalf("backtestSyncSessionScope returned unexpected values")
	}
	if got := dataSyncKey("US.AAPL", "1m", since, until, "forward", "regular"); !strings.Contains(got, "US.AAPL|1m|") || !strings.HasSuffix(got, "|forward|regular") {
		t.Fatalf("dataSyncKey = %q", got)
	}
	if !isMissingKLineCoverageError(expectedCoverageErr) || isMissingKLineCoverageError(errors.New("network down")) {
		t.Fatalf("isMissingKLineCoverageError classification failed")
	}
}
