package backtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestBacktestDateRangeRejectsIncompleteAndInvalidMarketInputs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		symbol    string
		startDate string
		endDate   string
		startTime string
		endTime   string
	}{
		{"unsupported market", "UNKNOWN.SYM", "", "", "", ""},
		{"start date without end", "US.AAPL", "2026-01-02", "", "", ""},
		{"invalid start date", "US.AAPL", "2026-02-30", "2026-03-01", "", ""},
		{"invalid end date", "US.AAPL", "2026-03-01", "not-a-date", "", ""},
		{"reversed dates", "US.AAPL", "2026-03-02", "2026-03-01", "", ""},
		{"invalid start time", "US.AAPL", "", "", "not-a-time", "2026-03-01T01:00:00Z"},
		{"invalid end time", "US.AAPL", "", "", "2026-03-01T00:00:00Z", "not-a-time"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, _, err := resolveBacktestTimeRange(tc.symbol, tc.startDate, tc.endDate, tc.startTime, tc.endTime)
			if err == nil || !IsRequestError(err) {
				t.Fatalf("resolveBacktestTimeRange(%+v) error=%v, want request error", tc, err)
			}
		})
	}

	if _, err := parseMarketDate("2026-2-1", time.UTC); err == nil {
		t.Fatal("parseMarketDate accepted a non-zero-padded date")
	}
	if _, err := parseMarketDate("2026-02-30", time.UTC); err == nil {
		t.Fatal("parseMarketDate accepted an impossible date")
	}
	if _, _, _, _, _, err := resolveSyncTimeRange("US.AAPL", "", "", "bad", ""); err == nil || !IsRequestError(err) {
		t.Fatalf("resolveSyncTimeRange invalid since error=%v, want request error", err)
	}
	if _, _, _, _, _, err := resolveSyncTimeRange("US.AAPL", "", "", "", "bad"); err == nil || !IsRequestError(err) {
		t.Fatalf("resolveSyncTimeRange invalid until error=%v, want request error", err)
	}
}

func TestBacktestDataPreparationRejectsInvalidCandidatesBeforeStartingSync(t *testing.T) {
	ctx := context.Background()
	if _, err := NewService().EnsureScriptData(ctx, ScriptStartRequest{}); err == nil || !IsRequestError(err) {
		t.Fatalf("EnsureScriptData empty script error=%v, want request error", err)
	}
	if _, err := NewService().EnsureDefinitionsData(ctx, validStartRequest(), []string{"def-1"}); err == nil || !strings.Contains(err.Error(), "strategy provider not configured") {
		t.Fatalf("EnsureDefinitionsData without provider error=%v", err)
	}
	if _, err := NewService(WithStrategyProvider(fakeStrategyProvider{})).EnsureDefinitionsData(ctx, validStartRequest(), []string{" "}); err == nil || !IsRequestError(err) {
		t.Fatalf("EnsureDefinitionsData empty IDs error=%v, want request error", err)
	}
	providerErr := errors.New("definition storage unavailable")
	if _, err := NewService(WithStrategyProvider(fakeStrategyProvider{err: providerErr})).EnsureDefinitionsData(ctx, validStartRequest(), []string{"def-1"}); !errors.Is(err, providerErr) {
		t.Fatalf("EnsureDefinitionsData provider error=%v, want %v", err, providerErr)
	}
	if _, err := NewService(WithStrategyProvider(fakeStrategyProvider{})).EnsureDefinitionsData(ctx, validStartRequest(), []string{"missing"}); !errors.Is(err, ErrStrategyDefinitionNotFound) {
		t.Fatalf("EnsureDefinitionsData missing definition error=%v", err)
	}
	if _, err := NewService(WithStrategyProvider(fakeStrategyProvider{defs: map[string]StrategyDef{"bad": {ID: "bad"}}})).EnsureDefinitionsData(ctx, validStartRequest(), []string{"bad"}); err == nil {
		t.Fatal("EnsureDefinitionsData accepted an invalid definition")
	}

	start := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	base := preparedBacktest{request: StartRequest{Symbol: "US.AAPL", Interval: "1m"}, queryStart: start, endTime: start.Add(time.Hour)}
	otherSymbol := preparedBacktest{request: StartRequest{Symbol: "HK.00700", Interval: "1m"}, queryStart: start.Add(-time.Hour), endTime: start.Add(2 * time.Hour)}
	if _, _, _, err := combinePreparedBacktests([]preparedBacktest{base, otherSymbol}); err == nil || !IsRequestError(err) {
		t.Fatalf("combinePreparedBacktests mismatched symbol error=%v, want request error", err)
	}
	otherInterval := preparedBacktest{request: StartRequest{Symbol: "US.AAPL", Interval: "5m"}, queryStart: start, endTime: start.Add(time.Hour)}
	if _, _, _, err := combinePreparedBacktests([]preparedBacktest{base, otherInterval}); err == nil || !IsRequestError(err) {
		t.Fatalf("combinePreparedBacktests mismatched interval error=%v, want request error", err)
	}
}

func TestBacktestSyncHelpersHandleDefaultsAndAdapterSetupFailures(t *testing.T) {
	defaulted := applyDefaultSyncInstrument(SyncRequest{})
	if defaulted.Market != "HK" || defaulted.Code != "00700" {
		t.Fatalf("applyDefaultSyncInstrument()=%+v, want HK.00700", defaulted)
	}
	if got := parseSyncIntervals([]string{" ", ""}); len(got) != 4 || string(got[0]) != "1m" {
		t.Fatalf("parseSyncIntervals blank=%v, want default intervals", got)
	}
	if parseSyncRehabType("none") != RehabTypeNone || parseSyncRehabType("backward") != RehabTypeBackward {
		t.Fatal("parseSyncRehabType did not preserve explicit modes")
	}
	if _, err := NewService().newSyncer(); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("newSyncer without factory error=%v", err)
	}
	adapterErr := errors.New("OpenD offline")
	if _, err := NewService(WithNewKLineSyncerFn(func(string) (KLineSyncer, error) { return nil, adapterErr })).newSyncer(); !errors.Is(err, adapterErr) {
		t.Fatalf("newSyncer factory error=%v, want %v", err, adapterErr)
	}
}

func TestBacktestDiagnosticHelpersRejectUnexpectedDynamicValues(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("jftradeCheckedTypeAssertion did not panic for an incompatible value")
		}
	}()
	jftradeLogError(errors.New("best effort"), nil, "not an error")
	_ = jftradeCheckedTypeAssertion[string](42)
}

func TestResultViewValidationRejectsMalformedWindowsAndResolutions(t *testing.T) {
	for _, req := range []ResultViewRequest{
		{View: "chart", Cursor: "not-a-cursor"},
		{View: "chart", StartTime: "not-a-time"},
		{View: "chart", EndTime: "not-a-time"},
		{View: "chart", StartTime: "2026-01-03T00:00:00Z", EndTime: "2026-01-02T00:00:00Z"},
	} {
		if _, err := normalizeResultView(req); err == nil || !IsRequestError(err) {
			t.Fatalf("normalizeResultView(%+v) error=%v, want request error", req, err)
		}
	}
	if _, _, err := resultViewCandles(nil, "unsupported", "auto", nil, nil, 10); err == nil || !IsRequestError(err) {
		t.Fatalf("resultViewCandles invalid native interval error=%v", err)
	}
	if _, _, err := resultViewCandles(nil, "1m", "unsupported", nil, nil, 10); err == nil || !IsRequestError(err) {
		t.Fatalf("resultViewCandles invalid resolution error=%v", err)
	}
	if _, _, err := resultViewCandles(nil, "5m", "1m", nil, nil, 10); err == nil || !IsRequestError(err) {
		t.Fatalf("resultViewCandles finer resolution error=%v", err)
	}
	for _, value := range []string{"17s", "17m", "17h", "17d", "17w"} {
		if duration, err := resultViewIntervalDuration(value); err != nil || duration <= 0 {
			t.Fatalf("resultViewIntervalDuration(%q)=%s/%v", value, duration, err)
		}
	}
}

func TestDataReadinessPropagatesCoverageFailuresAndExistingSyncTerminalStates(t *testing.T) {
	start := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	base := preparedBacktest{request: StartRequest{Symbol: "US.AAPL", Interval: "1m"}, queryStart: start, endTime: start.Add(time.Hour)}
	earlierAndLater := preparedBacktest{request: StartRequest{Symbol: "US.AAPL", Interval: "1m"}, queryStart: start.Add(-time.Hour), endTime: start.Add(2 * time.Hour)}
	_, queryStart, endTime, err := combinePreparedBacktests([]preparedBacktest{base, earlierAndLater})
	if err != nil || !queryStart.Equal(earlierAndLater.queryStart) || !endTime.Equal(earlierAndLater.endTime) {
		t.Fatalf("combinePreparedBacktests range=%s..%s err=%v", queryStart, endTime, err)
	}

	coverageUnavailable := errors.New("coverage database unavailable")
	failureService := NewService(WithKLineCoverageCheckFn(func(string, string, string, time.Time, time.Time, string, string) error {
		return coverageUnavailable
	}))
	if _, err := failureService.ensurePreparedData(context.Background(), []preparedBacktest{base}); !errors.Is(err, coverageUnavailable) {
		t.Fatalf("ensurePreparedData coverage failure=%v, want %v", err, coverageUnavailable)
	}

	missingCoverage := errors.New("missing K-line coverage")
	missingService := NewService(WithKLineCoverageCheckFn(func(string, string, string, time.Time, time.Time, string, string) error {
		return missingCoverage
	}))
	if _, err := missingService.ensurePreparedData(context.Background(), []preparedBacktest{base}); err == nil || !strings.Contains(err.Error(), "kline sync adapter not configured") {
		t.Fatalf("ensurePreparedData missing coverage sync setup error=%v", err)
	}

	tasks := newMemorySyncTaskStore()
	service := NewService(WithSyncTaskStore(tasks))
	started := &SyncStarted{TaskID: "sync-existing"}
	progress := bt.NewSyncProgress(started.TaskID, "US.AAPL", start)
	tasks.Add(started.TaskID, progress, nil)
	for _, tc := range []struct {
		name    string
		advance func(*bt.SyncProgress)
		status  string
	}{
		{"queued", func(*bt.SyncProgress) {}, DataStatusSyncing},
		{"failed", func(p *bt.SyncProgress) { p.MarkFailed(errors.New("OpenD unavailable"), start) }, DataStatusSyncFailed},
		{"cancelled", func(p *bt.SyncProgress) { p.MarkCancelled(start) }, DataStatusSyncCancelled},
		{"completed", func(p *bt.SyncProgress) { p.MarkCompleted(1, start) }, DataStatusInsufficientAfterSync},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := bt.NewSyncProgress(started.TaskID+"-"+tc.name, "US.AAPL", start)
			tasks.Add(current.TaskID, current, nil)
			candidate := &SyncStarted{TaskID: current.TaskID}
			tc.advance(current)
			ready, handled := service.readinessForExistingSync("key-"+tc.name, candidate, missingCoverage)
			if !handled || ready == nil || ready.Status != tc.status {
				t.Fatalf("readinessForExistingSync(%s)=%+v handled=%v", tc.name, ready, handled)
			}
		})
	}
	orphaned := &SyncStarted{TaskID: "sync-not-in-task-store"}
	service.dataSyncTasks["orphaned-sync"] = orphaned
	if ready, handled := service.readinessForExistingSync("orphaned-sync", orphaned, missingCoverage); handled || ready != nil {
		t.Fatalf("orphaned existing sync readiness=%+v handled=%v, want retry", ready, handled)
	}
	if _, ok := service.dataSyncTasks["orphaned-sync"]; ok {
		t.Fatal("orphaned sync task was not cleared")
	}
}
