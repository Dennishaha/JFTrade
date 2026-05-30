package backtest

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/sirupsen/logrus"

	"github.com/jftrade/jftrade-main/pkg/futu"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

const realUSMarch2026DoubleMATemplateScript = `strategy DoubleMovingAverage
version 0.1.0
symbol US.TME
interval 5m

on init:
  log "double moving average template initialized"

on kline_close:
  let dma_fast_ma = ma(MA, 5, day)
  let dma_slow_ma = ma(MA, 20, day)
  if cross_over(dma_fast_ma, dma_slow_ma):
    buy shares 100 policy same_direction type MARKET
  else:
    if cross_under(dma_fast_ma, dma_slow_ma):
      sell shares 100 policy same_direction type MARKET
`

const realUSTMEProtectSessionScript = `strategy ProtectSessionWindow
version 0.1.0
symbol US.TME
interval 5m

on init:
	log "protect session template initialized"

on kline_close:
	buy shares 1 policy same_direction type MARKET
	protect auto stopLoss 2 hour 2 window session
	protect auto takeProfit 2 hour 3 window session
	protect auto trailingStop 2 hour 1.5 window session
`

const defaultRealChainSavedDoubleMAStrategyDefinitionID = "dsl-double-moving-average"

type realChainProfileFixture struct {
	dbPath           string
	symbol           string
	interval         types.Interval
	replayStart      time.Time
	replayEnd        time.Time
	syncStart        time.Time
	warmupCandles    int
	syncDuration     time.Duration
	reusedExistingDB bool
	rowsSyncWindow   int
	rowsReplaySlice  int
	progress         *SyncProgress
	runConfig        RunConfig
}

type realChainFixtureOptions struct {
	progressName      string
	symbol            string
	interval          types.Interval
	replayStart       time.Time
	replayEnd         time.Time
	useExtendedHours  bool
	strategyScript    string
	sourceFormat      string
	strategyLabelHint string
}

type realChainSavedDefinition struct {
	id           string
	name         string
	sourceFormat string
	script       string
}

func TestRealUSMarch2026DoubleMATemplateProfile(t *testing.T) {
	fixture := prepareRealUSMarch2026DoubleMAFixture(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	runStartedAt := time.Now()
	result := Run(ctx, fixture.runConfig)
	runDuration := time.Since(runStartedAt)
	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("Run() error = %s", result.Error)
	}

	snapshot := fixture.progress.Snapshot()
	t.Logf(
		"real-chain sync profile: symbol=%s interval=%s syncRange=[%s,%s] replayRange=[%s,%s] warmupCandles=%d duration=%s reusedExistingDB=%v rows(syncWindow)=%d rows(replaySlice)=%d completedBatches=%d retries=%d status=%s db=%s",
		fixture.symbol,
		fixture.interval,
		fixture.syncStart.Format(time.RFC3339),
		fixture.replayEnd.Format(time.RFC3339),
		fixture.replayStart.Format(time.RFC3339),
		fixture.replayEnd.Format(time.RFC3339),
		fixture.warmupCandles,
		fixture.syncDuration,
		fixture.reusedExistingDB,
		fixture.rowsSyncWindow,
		fixture.rowsReplaySlice,
		snapshot.CompletedBatches,
		snapshot.Retries,
		snapshot.Status,
		fixture.dbPath,
	)
	t.Logf(
		"real-chain run profile: duration=%s candles=%d pnlCurve=%d trades=%d orderBook=%d finalBalance=%.2f pnl=%.2f runtimeErrors=%d",
		runDuration,
		len(result.Candles),
		len(result.PnLCurve),
		len(result.Trades),
		len(result.OrderBook),
		result.FinalBalance,
		result.PnL,
		len(result.RuntimeErrors),
	)

	if fixture.rowsReplaySlice == 0 {
		t.Fatal("expected synced replay rows for US.TME March 2026")
	}
	if len(result.Candles) == 0 {
		t.Fatal("expected replay candles for US.TME March 2026 run")
	}
}

func TestRealUSMarch2026DoubleMATemplateDiagnostics(t *testing.T) {
	fixture := prepareRealUSMarch2026DoubleMAFixture(t)

	store, err := NewFutuKLineStore(fixture.dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	defer store.Close()
	store.SetRehabType("forward")
	store.SetReadSessionScope(KLineSessionScopeRegular)

	diagnostics, err := diagnoseRealUSMarch2026DoubleMATemplate(store, fixture)
	if err != nil {
		t.Fatalf("diagnoseRealUSMarch2026DoubleMATemplate() error = %v", err)
	}

	t.Logf(
		"real-chain template diagnostics: oneMinuteRowsBeforeStart=%d crossoverCount=%d crossunderCount=%d firstCrossOver=%s firstCrossUnder=%s lastFast=%s lastSlow=%s",
		diagnostics.oneMinuteRowsBeforeStart,
		diagnostics.crossOverCount,
		diagnostics.crossUnderCount,
		realChainFormatTime(diagnostics.firstCrossOverAt),
		realChainFormatTime(diagnostics.firstCrossUnderAt),
		realChainFormatFloat(diagnostics.lastFast),
		realChainFormatFloat(diagnostics.lastSlow),
	)
}

func BenchmarkRealUSMarch2026DoubleMATemplateReplay(b *testing.B) {
	fixture := prepareRealUSMarch2026DoubleMAFixture(b)
	ctx := context.Background()

	previousWriter := log.Writer()
	previousLogrusWriter := logrus.StandardLogger().Out
	log.SetOutput(io.Discard)
	logrus.SetOutput(io.Discard)
	b.Cleanup(func() {
		log.SetOutput(previousWriter)
		logrus.SetOutput(previousLogrusWriter)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result := Run(ctx, fixture.runConfig)
		if result == nil {
			b.Fatal("expected run result")
		}
		if result.Error != "" {
			b.Fatalf("Run() error = %s", result.Error)
		}
	}
}

func TestRealUSTME2023To2026SavedDoubleMAStrategyProfileExtended(t *testing.T) {
	fixture := prepareRealUSTME2023To2026SavedDoubleMAStrategyFixture(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	runStartedAt := time.Now()
	result := Run(ctx, fixture.runConfig)
	runDuration := time.Since(runStartedAt)
	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("Run() error = %s", result.Error)
	}

	snapshot := fixture.progress.Snapshot()
	t.Logf(
		"real-chain sync profile: symbol=%s interval=%s syncRange=[%s,%s] replayRange=[%s,%s] warmupCandles=%d duration=%s reusedExistingDB=%v rows(syncWindow)=%d rows(replaySlice)=%d completedBatches=%d retries=%d status=%s db=%s",
		fixture.symbol,
		fixture.interval,
		fixture.syncStart.Format(time.RFC3339),
		fixture.replayEnd.Format(time.RFC3339),
		fixture.replayStart.Format(time.RFC3339),
		fixture.replayEnd.Format(time.RFC3339),
		fixture.warmupCandles,
		fixture.syncDuration,
		fixture.reusedExistingDB,
		fixture.rowsSyncWindow,
		fixture.rowsReplaySlice,
		snapshot.CompletedBatches,
		snapshot.Retries,
		snapshot.Status,
		fixture.dbPath,
	)
	t.Logf(
		"real-chain run profile: duration=%s candles=%d pnlCurve=%d trades=%d orderBook=%d finalBalance=%.2f pnl=%.2f runtimeErrors=%d",
		runDuration,
		len(result.Candles),
		len(result.PnLCurve),
		len(result.Trades),
		len(result.OrderBook),
		result.FinalBalance,
		result.PnL,
		len(result.RuntimeErrors),
	)

	if fixture.rowsReplaySlice == 0 {
		t.Fatal("expected synced replay rows for US.TME 2023-2026 extended-hours")
	}
	if len(result.Candles) == 0 {
		t.Fatal("expected replay candles for US.TME 2023-2026 extended-hours run")
	}
}

func BenchmarkRealUSTME2023To2026SavedDoubleMAStrategyReplayExtended(b *testing.B) {
	fixture := prepareRealUSTME2023To2026SavedDoubleMAStrategyFixture(b)
	ctx := context.Background()

	previousWriter := log.Writer()
	previousLogrusWriter := logrus.StandardLogger().Out
	log.SetOutput(io.Discard)
	logrus.SetOutput(io.Discard)
	b.Cleanup(func() {
		log.SetOutput(previousWriter)
		logrus.SetOutput(previousLogrusWriter)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result := Run(ctx, fixture.runConfig)
		if result == nil {
			b.Fatal("expected run result")
		}
		if result.Error != "" {
			b.Fatalf("Run() error = %s", result.Error)
		}
	}
}

func TestRealUSTME2023To2026ProtectSessionProfileExtended(t *testing.T) {
	fixture := prepareRealUSTME2023To2026ProtectSessionFixture(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	runStartedAt := time.Now()
	result := Run(ctx, fixture.runConfig)
	runDuration := time.Since(runStartedAt)
	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("Run() error = %s", result.Error)
	}

	snapshot := fixture.progress.Snapshot()
	t.Logf(
		"real-chain sync profile: symbol=%s interval=%s syncRange=[%s,%s] replayRange=[%s,%s] warmupCandles=%d duration=%s reusedExistingDB=%v rows(syncWindow)=%d rows(replaySlice)=%d completedBatches=%d retries=%d status=%s db=%s",
		fixture.symbol,
		fixture.interval,
		fixture.syncStart.Format(time.RFC3339),
		fixture.replayEnd.Format(time.RFC3339),
		fixture.replayStart.Format(time.RFC3339),
		fixture.replayEnd.Format(time.RFC3339),
		fixture.warmupCandles,
		fixture.syncDuration,
		fixture.reusedExistingDB,
		fixture.rowsSyncWindow,
		fixture.rowsReplaySlice,
		snapshot.CompletedBatches,
		snapshot.Retries,
		snapshot.Status,
		fixture.dbPath,
	)
	t.Logf(
		"real-chain run profile: duration=%s candles=%d pnlCurve=%d trades=%d orderBook=%d finalBalance=%.2f pnl=%.2f runtimeErrors=%d",
		runDuration,
		len(result.Candles),
		len(result.PnLCurve),
		len(result.Trades),
		len(result.OrderBook),
		result.FinalBalance,
		result.PnL,
		len(result.RuntimeErrors),
	)

	if fixture.rowsReplaySlice == 0 {
		t.Fatal("expected synced replay rows for US.TME 2023-2026 protect session extended-hours")
	}
	if len(result.Candles) == 0 {
		t.Fatal("expected replay candles for US.TME 2023-2026 protect session run")
	}
}

func BenchmarkRealUSTME2023To2026ProtectSessionReplayExtended(b *testing.B) {
	fixture := prepareRealUSTME2023To2026ProtectSessionFixture(b)
	ctx := context.Background()

	previousWriter := log.Writer()
	previousLogrusWriter := logrus.StandardLogger().Out
	log.SetOutput(io.Discard)
	logrus.SetOutput(io.Discard)
	b.Cleanup(func() {
		log.SetOutput(previousWriter)
		logrus.SetOutput(previousLogrusWriter)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result := Run(ctx, fixture.runConfig)
		if result == nil {
			b.Fatal("expected run result")
		}
		if result.Error != "" {
			b.Fatalf("Run() error = %s", result.Error)
		}
	}
}

type realChainTemplateDiagnostics struct {
	oneMinuteRowsBeforeStart int
	crossOverCount           int
	crossUnderCount          int
	firstCrossOverAt         time.Time
	firstCrossUnderAt        time.Time
	lastFast                 float64
	lastSlow                 float64
	hasLastPair              bool
}

func diagnoseRealUSMarch2026DoubleMATemplate(store *FutuKLineStore, fixture realChainProfileFixture) (realChainTemplateDiagnostics, error) {
	oneMinuteRows, err := store.QueryKLinesBackward(nil, fixture.symbol, types.Interval1m, fixture.replayStart, 1)
	if err != nil {
		return realChainTemplateDiagnostics{}, err
	}
	klines, err := loadKLinesInRange(store, fixture.syncStart, fixture.replayEnd, fixture.symbol, fixture.interval)
	if err != nil {
		return realChainTemplateDiagnostics{}, err
	}

	diagnostics := realChainTemplateDiagnostics{oneMinuteRowsBeforeStart: len(oneMinuteRows)}
	previousFast := 0.0
	previousSlow := 0.0
	hasPreviousPair := false

	for index := range klines {
		fast, fastOK := realChainTradingWindowSMA(klines, fixture.symbol, index, 5)
		slow, slowOK := realChainTradingWindowSMA(klines, fixture.symbol, index, 20)
		if !fastOK || !slowOK {
			continue
		}
		endTime := time.Time(klines[index].EndTime)
		if endTime.Before(fixture.replayStart) {
			previousFast = fast
			previousSlow = slow
			hasPreviousPair = true
			continue
		}
		if endTime.After(fixture.replayEnd) {
			break
		}
		if hasPreviousPair {
			if previousFast <= previousSlow && fast > slow {
				diagnostics.crossOverCount++
				if diagnostics.firstCrossOverAt.IsZero() {
					diagnostics.firstCrossOverAt = endTime
				}
			}
			if previousFast >= previousSlow && fast < slow {
				diagnostics.crossUnderCount++
				if diagnostics.firstCrossUnderAt.IsZero() {
					diagnostics.firstCrossUnderAt = endTime
				}
			}
		}
		previousFast = fast
		previousSlow = slow
		hasPreviousPair = true
		diagnostics.lastFast = fast
		diagnostics.lastSlow = slow
		diagnostics.hasLastPair = true
	}

	return diagnostics, nil
}

func loadKLinesInRange(store *FutuKLineStore, since, until time.Time, symbol string, interval types.Interval) ([]types.KLine, error) {
	ch, errCh := store.QueryKLinesCh(since, until, nil, []string{symbol}, []types.Interval{interval})
	klines := make([]types.KLine, 0, 2048)
	for ch != nil || errCh != nil {
		select {
		case kline, ok := <-ch:
			if !ok {
				ch = nil
				continue
			}
			klines = append(klines, kline)
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				return nil, err
			}
		}
	}
	return klines, nil
}

func realChainTradingWindowSMA(klines []types.KLine, symbol string, currentIndex int, period int) (float64, bool) {
	if period <= 0 || currentIndex < 0 || currentIndex >= len(klines) {
		return 0, false
	}
	uniqueDays := 0
	lastKey := int64(0)
	hasKey := false
	sum := 0.0
	count := 0
	for index := currentIndex; index >= 0; index-- {
		labelStart, ok := futu.TradingDayLabelStart(symbol, time.Time(klines[index].EndTime), false)
		if !ok {
			continue
		}
		key := labelStart.Unix()
		if !hasKey || key != lastKey {
			if uniqueDays == period {
				break
			}
			lastKey = key
			hasKey = true
			uniqueDays++
		}
		sum += klines[index].Close.Float64()
		count++
	}
	if uniqueDays < period || count == 0 {
		return 0, false
	}
	return sum / float64(count), true
}

func realChainFormatTime(value time.Time) string {
	if value.IsZero() {
		return "none"
	}
	return value.Format(time.RFC3339)
}

func realChainFormatFloat(value float64) string {
	return fmt.Sprintf("%.6f", value)
}

func prepareRealUSMarch2026DoubleMAFixture(tb testing.TB) realChainProfileFixture {
	return prepareRealChainProfileFixture(tb, realChainFixtureOptions{
		progressName:      "real-us-tme-202603",
		symbol:            "US.TME",
		interval:          types.Interval5m,
		replayStart:       time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		replayEnd:         time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		useExtendedHours:  false,
		strategyScript:    realUSMarch2026DoubleMATemplateScript,
		sourceFormat:      strategydefinition.SourceFormatDSLV1,
		strategyLabelHint: "inline-double-ma-template",
	})
}

func prepareRealUSTME2023To2026SavedDoubleMAStrategyFixture(tb testing.TB) realChainProfileFixture {
	definition := loadRealChainStrategyDefinition(tb, realChainStrategyRuntimeDBPath(tb), realChainSavedDoubleMAStrategyDefinitionID())
	return prepareRealChainProfileFixture(tb, realChainFixtureOptions{
		progressName:      "real-us-tme-202301-202601-extended-saved-double-ma",
		symbol:            "US.TME",
		interval:          types.Interval5m,
		replayStart:       time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC),
		replayEnd:         time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		useExtendedHours:  true,
		strategyScript:    definition.script,
		sourceFormat:      definition.sourceFormat,
		strategyLabelHint: definition.id,
	})
}

func prepareRealUSTME2023To2026ProtectSessionFixture(tb testing.TB) realChainProfileFixture {
	return prepareRealChainProfileFixture(tb, realChainFixtureOptions{
		progressName:      "real-us-tme-202301-202601-extended-protect-session",
		symbol:            "US.TME",
		interval:          types.Interval5m,
		replayStart:       time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC),
		replayEnd:         time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		useExtendedHours:  true,
		strategyScript:    realUSTMEProtectSessionScript,
		sourceFormat:      strategydefinition.SourceFormatDSLV1,
		strategyLabelHint: "inline-protect-session",
	})
}

func prepareRealChainProfileFixture(tb testing.TB, options realChainFixtureOptions) realChainProfileFixture {
	tb.Helper()
	requireRealChainProfile(tb)
	ensureOpenDReachable(tb, realChainProfileOpenDAddr())

	homeDir := tb.TempDir()
	if envSetter, ok := tb.(interface{ Setenv(string, string) }); ok {
		envSetter.Setenv("HOME", homeDir)
	}

	symbol := strings.ToUpper(strings.TrimSpace(options.symbol))
	if symbol == "" {
		tb.Fatal("real chain fixture symbol is required")
	}
	interval := options.interval
	if strings.TrimSpace(string(interval)) == "" {
		tb.Fatal("real chain fixture interval is required")
	}
	replayStart := options.replayStart.UTC()
	replayEnd := options.replayEnd.UTC()
	if !replayEnd.After(replayStart) {
		tb.Fatalf("invalid replay range [%s,%s]", replayStart.Format(time.RFC3339), replayEnd.Format(time.RFC3339))
	}
	strategyScript := strings.TrimSpace(options.strategyScript)
	if strategyScript == "" {
		tb.Fatalf("real chain fixture strategy script is required: %s", options.strategyLabelHint)
	}
	useExtendedHours := options.useExtendedHours
	warmupCandles, err := deriveStrategyWarmupCandles(strategyScript, interval, symbol, realChainBoolPtr(useExtendedHours))
	if err != nil {
		tb.Fatalf("deriveStrategyWarmupCandles() error = %v", err)
	}
	syncStart := replayStart.Add(-interval.Duration() * time.Duration(warmupCandles))
	dbPath := realChainProfileDBPath(tb)
	readSessionScope := resolveBacktestReadSessionScope(realChainBoolPtr(useExtendedHours))

	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		tb.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	defer store.Close()
	store.SetRehabType("forward")
	store.SetReadSessionScope(readSessionScope)

	progressName := strings.TrimSpace(options.progressName)
	if progressName == "" {
		progressName = fmt.Sprintf("real-%s-%s", strings.ToLower(strings.ReplaceAll(symbol, ".", "-")), replayStart.Format("20060102"))
	}
	progress := NewSyncProgress(progressName, symbol, syncStart)
	exchange := futu.NewExchange(realChainProfileOpenDAddr())
	defer func() {
		_ = exchange.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	reusedExistingDB := false
	syncDuration := time.Duration(0)
	if err := store.EnsureCoverage(symbol, interval, syncStart, replayEnd); err == nil {
		reusedExistingDB = true
		progress.MarkCompleted(1, time.Now().UTC())
	} else {
		syncStartedAt := time.Now()
		err = store.SyncKLines(
			ctx,
			exchange,
			symbol,
			[]types.Interval{interval},
			syncStart,
			replayEnd,
			qotcommonpb.RehabType_RehabType_Forward,
			readSessionScope,
			progress,
		)
		syncDuration = time.Since(syncStartedAt)
		if err != nil {
			tb.Fatalf("SyncKLines() error = %v", err)
		}
		if snapshot := progress.Snapshot(); snapshot == nil || snapshot.Status != "completed" {
			if snapshot == nil {
				tb.Fatal("expected sync snapshot")
			}
			tb.Fatalf("sync status = %s, want completed", snapshot.Status)
		}
	}

	rowsSyncWindow, err := countKLinesInRange(store, syncStart, replayEnd, symbol, interval)
	if err != nil {
		tb.Fatalf("count sync window rows: %v", err)
	}
	rowsReplaySlice, err := countKLinesInRange(store, replayStart, replayEnd, symbol, interval)
	if err != nil {
		tb.Fatalf("count replay slice rows: %v", err)
	}

	sourceFormat := strings.TrimSpace(options.sourceFormat)
	if sourceFormat == "" {
		sourceFormat = strategydefinition.SourceFormatDSLV1
	}

	return realChainProfileFixture{
		dbPath:           dbPath,
		symbol:           symbol,
		interval:         interval,
		replayStart:      replayStart,
		replayEnd:        replayEnd,
		syncStart:        syncStart,
		warmupCandles:    warmupCandles,
		syncDuration:     syncDuration,
		reusedExistingDB: reusedExistingDB,
		rowsSyncWindow:   rowsSyncWindow,
		rowsReplaySlice:  rowsReplaySlice,
		progress:         progress,
		runConfig: RunConfig{
			DBPath:           dbPath,
			Symbol:           symbol,
			Interval:         string(interval),
			SourceFormat:     sourceFormat,
			StartTime:        replayStart,
			EndTime:          replayEnd,
			StrategyScript:   strategyScript,
			InitialBalance:   100000,
			RehabType:        "forward",
			UseExtendedHours: realChainBoolPtr(useExtendedHours),
		},
	}
}

func loadRealChainStrategyDefinition(tb testing.TB, dbPath string, definitionID string) realChainSavedDefinition {
	tb.Helper()
	trimmedPath := strings.TrimSpace(dbPath)
	trimmedID := strings.TrimSpace(definitionID)
	if trimmedPath == "" {
		tb.Fatal("real chain strategy db path is required")
	}
	if trimmedID == "" {
		tb.Fatal("real chain strategy definition id is required")
	}
	db, err := sql.Open("sqlite", trimmedPath)
	if err != nil {
		tb.Fatalf("open real chain strategy db: %v", err)
	}
	defer db.Close()

	definition := realChainSavedDefinition{}
	err = db.QueryRow(
		`SELECT id, name, source_format, script FROM strategy_design_definitions WHERE id = ? AND (deleted_at IS NULL OR TRIM(deleted_at) = '')`,
		trimmedID,
	).Scan(&definition.id, &definition.name, &definition.sourceFormat, &definition.script)
	if err != nil {
		if err == sql.ErrNoRows {
			tb.Fatalf("strategy definition %s not found in %s", trimmedID, trimmedPath)
		}
		tb.Fatalf("query strategy definition %s: %v", trimmedID, err)
	}
	definition.script = strings.TrimSpace(definition.script)
	if definition.script == "" {
		tb.Fatalf("strategy definition %s has empty script", trimmedID)
	}
	return definition
}

func realChainSavedDoubleMAStrategyDefinitionID() string {
	if value := strings.TrimSpace(os.Getenv("JFTRADE_REAL_CHAIN_STRATEGY_DEFINITION_ID")); value != "" {
		return value
	}
	return defaultRealChainSavedDoubleMAStrategyDefinitionID
}

func realChainStrategyRuntimeDBPath(tb testing.TB) string {
	tb.Helper()
	if value := strings.TrimSpace(os.Getenv("JFTRADE_REAL_CHAIN_STRATEGY_DB_PATH")); value != "" {
		if _, err := os.Stat(value); err != nil {
			tb.Fatalf("real chain strategy db path %s is not readable: %v", value, err)
		}
		return value
	}
	if value := strings.TrimSpace(os.Getenv("JFTRADE_STRATEGY_RUNTIME_DB")); value != "" {
		if _, err := os.Stat(value); err != nil {
			tb.Fatalf("JFTRADE_STRATEGY_RUNTIME_DB %s is not readable: %v", value, err)
		}
		return value
	}

	candidates := make([]string, 0, 2)
	if repoRoot, ok := realChainRepoRoot(); ok {
		candidates = append(candidates, filepath.Join(repoRoot, "var", "jftrade-api", "strategy-runtime.db"))
	}
	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		candidates = append(candidates, filepath.Join(homeDir, "var", "jftrade-api", "strategy-runtime.db"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	tb.Fatalf("could not locate strategy runtime db; checked %s", strings.Join(candidates, ", "))
	return ""
}

func realChainRepoRoot() (string, bool) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..")), true
}

func countKLinesInRange(store *FutuKLineStore, since, until time.Time, symbol string, interval types.Interval) (int, error) {
	ch, errCh := store.QueryKLinesCh(since, until, nil, []string{symbol}, []types.Interval{interval})
	count := 0
	for ch != nil || errCh != nil {
		select {
		case _, ok := <-ch:
			if !ok {
				ch = nil
				continue
			}
			count++
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				return 0, err
			}
		}
	}
	return count, nil
}

func requireRealChainProfile(tb testing.TB) {
	tb.Helper()
	if testing.Short() {
		tb.Skip("skip real chain profile in -short mode")
	}
	if strings.TrimSpace(os.Getenv("JFTRADE_REAL_CHAIN_PROFILE")) == "" {
		tb.Skip("set JFTRADE_REAL_CHAIN_PROFILE=1 to run real OpenD backtest profiling")
	}
}

func ensureOpenDReachable(tb testing.TB, addr string) {
	tb.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		tb.Skipf("OpenD %s is not reachable: %v", addr, err)
	}
	_ = conn.Close()
}

func realChainProfileOpenDAddr() string {
	if addr := strings.TrimSpace(os.Getenv("JFTRADE_REAL_CHAIN_OPEND_ADDR")); addr != "" {
		return addr
	}
	return futu.DefaultOpenDAddr
}

func realChainProfileDBPath(tb testing.TB) string {
	tb.Helper()
	if dbPath := strings.TrimSpace(os.Getenv("JFTRADE_REAL_CHAIN_DB_PATH")); dbPath != "" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			tb.Fatalf("mkdir real chain db dir: %v", err)
		}
		return dbPath
	}
	return filepath.Join(tb.TempDir(), "us-tme-2026-03-real-chain.db")
}

func realChainBoolPtr(value bool) *bool {
	return &value
}

func (f realChainProfileFixture) String() string {
	return fmt.Sprintf("%s %s [%s,%s]", f.symbol, f.interval, f.replayStart.Format(time.RFC3339), f.replayEnd.Format(time.RFC3339))
}
