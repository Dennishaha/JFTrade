package servercoretest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	pkgbacktest "github.com/jftrade/jftrade-main/pkg/backtest"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestBacktestSyncUsesAssembledMarketDataRuntime(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if err := store.SaveBacktestMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveBacktestMarketDataProvider: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp := postBacktestSync(t, srv.URL, btsrv.SyncRequest{
		Market: "US", Code: "AAPL", Intervals: []string{"2m"},
		Since: "2026-08-03T00:00:00Z", Until: "2026-08-04T00:00:00Z",
		RehabType: "none", SessionScope: "regular",
	})
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read sync response: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "provider futu does not support interval 2m") {
		t.Fatalf("sync status = %d, body = %s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "market-data provider runtime is unavailable") {
		t.Fatalf("backtest sync retained unavailable runtime: %s", body)
	}
}

func TestBacktestSyncRejectsActualAKShareOneYearUSFiveMinuteRange(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if err := store.SaveBacktestMarketDataProvider(jfsettings.MarketDataProviderAKShare); err != nil {
		t.Fatalf("SaveBacktestMarketDataProvider: %v", err)
	}
	const definitionID = "actual-akshare-us-five-minute"
	saveSeededStrategyDefinition(t, store, stratsrv.Definition{
		ID: definitionID, Name: "Actual AKShare US 5m", Version: "0.1.0",
		Runtime: stratsrv.RuntimePinePlan, SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol: "US.AAPL", Interval: "5m", Script: liveBABAOneYearStrategy(),
	})
	srv := newHTTPTestServer(t, store)

	resp := postBacktestSync(t, srv.URL, btsrv.SyncRequest{
		Market: "US", Code: "AAPL", Intervals: []string{"5m"},
		StartDate: "2025-07-13", EndDate: "2026-07-13",
		RehabType: "none", SessionScope: "regular",
	})
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read sync response: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "provider akshare limits 5m history to 5 days") {
		t.Fatalf("AKShare one-year 5m sync status = %d, body = %s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "market-data provider runtime is unavailable") {
		t.Fatalf("AKShare sync retained unavailable runtime: %s", body)
	}

	runResp := postBacktestRun(t, srv.URL, btsrv.StartRequest{
		DefinitionID: definitionID, Market: "US", Code: "AAPL", Interval: "5m",
		StartDate: "2025-07-13", EndDate: "2026-07-13",
		RehabType: "none", InitialBalance: 1_000_000,
	})
	defer func() { jftradeCheckTestError(t, runResp.Body.Close()) }()
	runBody, err := io.ReadAll(runResp.Body)
	if err != nil {
		t.Fatalf("read start response: %v", err)
	}
	if runResp.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(runBody), "backtest K-line data is not ready") {
		t.Fatalf("AKShare no-cache start status = %d, body = %s", runResp.StatusCode, runBody)
	}

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/backtests")
	if err != nil {
		t.Fatalf("GET backtests: %v", err)
	}
	var listEnvelope struct {
		Data struct {
			Runs []btsrv.RunState `json:"runs"`
		} `json:"data"`
	}
	decodeBacktestResponse(t, listResp, &listEnvelope)
	if len(listEnvelope.Data.Runs) != 0 {
		t.Fatalf("missing-coverage start persisted failed runs: %+v", listEnvelope.Data.Runs)
	}
}

func TestLiveBacktestHistoricalProviders(t *testing.T) {
	if os.Getenv("JFTRADE_LIVE_MARKETDATA_SMOKE") != "1" {
		t.Skip("set JFTRADE_LIVE_MARKETDATA_SMOKE=1 to query live historical providers")
	}
	until := time.Now().UTC().Truncate(24 * time.Hour)
	since := until.AddDate(0, 0, -10)
	for _, test := range []struct {
		provider jfsettings.ActiveMarketDataProvider
		market   string
		code     string
	}{
		{provider: jfsettings.MarketDataProviderYFinance, market: "US", code: "AAPL"},
		{provider: jfsettings.MarketDataProviderAKShare, market: "SH", code: "600519"},
	} {
		t.Run(string(test.provider), func(t *testing.T) {
			store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
			if err != nil {
				t.Fatalf("NewSettingsStore: %v", err)
			}
			if err := store.SaveBacktestMarketDataProvider(test.provider); err != nil {
				t.Fatalf("SaveBacktestMarketDataProvider: %v", err)
			}
			srv := newHTTPTestServer(t, store)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()

			resp := postBacktestSync(t, srv.URL, btsrv.SyncRequest{
				Market: test.market, Code: test.code, Intervals: []string{"1d"},
				Since: since.Format(time.RFC3339), Until: until.Format(time.RFC3339),
				RehabType: "none", SessionScope: "regular",
			})
			var envelope struct {
				Data btsrv.SyncStarted `json:"data"`
			}
			decodeBacktestResponse(t, resp, &envelope)
			if envelope.Data.MarketDataProvider != string(test.provider) {
				t.Fatalf("started provider = %q", envelope.Data.MarketDataProvider)
			}
			waitForLiveProviderSync(t, ctx, srv.URL, envelope.Data.TaskID, string(test.provider))
		})
	}
}

func TestLiveBABAOneYearBacktestProviderSwitchMatrix(t *testing.T) {
	if os.Getenv("JFTRADE_LIVE_MARKETDATA_BACKTEST") != "1" {
		t.Skip("set JFTRADE_LIVE_MARKETDATA_BACKTEST=1 to sync and backtest live BABA history")
	}
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to include Futu in the live BABA provider matrix")
	}
	if strings.TrimSpace(os.Getenv("JFTRADE_PINEWORKER_BUNDLE")) == "" {
		t.Fatal("JFTRADE_PINEWORKER_BUNDLE is required for the live BABA backtest")
	}

	runtimeDir := t.TempDir()
	klineDBPath := filepath.Join(runtimeDir, "backtest.db")
	runDBPath := filepath.Join(runtimeDir, "backtest-runs.db")
	t.Setenv("JFTRADE_BACKTEST_DB", klineDBPath)
	t.Setenv("JFTRADE_BACKTEST_RUN_DB", runDBPath)
	store, err := servercore.NewSettingsStore(filepath.Join(runtimeDir, "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := store.SaveIntegration(jfsettings.BrokerIntegration{
		Enabled: true,
		Config:  settingsfile.DefaultFutuConfig(),
	}); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	if err := store.SaveBacktestMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveBacktestMarketDataProvider: %v", err)
	}
	const definitionID = "live-baba-one-year-provider-switch"
	saveSeededStrategyDefinition(t, store, stratsrv.Definition{
		ID:           definitionID,
		Name:         "Live BABA One Year Provider Switch",
		Version:      "0.1.0",
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.BABA",
		Interval:     "1d",
		Script:       liveBABAOneYearStrategy(),
	})
	handler, srv := newHTTPTestServerWithHandler(t, store)

	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load New York timezone: %v", err)
	}
	previousDay := time.Now().In(location).AddDate(0, 0, -1)
	endDate := time.Date(previousDay.Year(), previousDay.Month(), previousDay.Day(), 0, 0, 0, 0, location)
	startDate := endDate.AddDate(-1, 0, 0)
	startLabel, endLabel := startDate.Format(time.DateOnly), endDate.Format(time.DateOnly)
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Minute)
	defer cancel()

	providers := []jfsettings.ActiveMarketDataProvider{
		jfsettings.MarketDataProviderYFinance,
		jfsettings.MarketDataProviderAKShare,
		jfsettings.MarketDataProviderFutu,
		jfsettings.MarketDataProviderYFinance,
	}
	wantCached := map[string]int{"futu": 0, "yfinance": 0, "akshare": 0}
	runProviders := make(map[string]string, len(providers))
	for index, provider := range providers {
		providerID := string(provider)
		stepName := providerID
		if index == len(providers)-1 {
			stepName += "-reacquire"
		}
		if ok := t.Run(stepName, func(t *testing.T) {
			putBacktestMarketDataProvider(t, srv.URL, provider)
			assertBacktestMarketDataProvider(t, srv.URL, provider)
			assertCachedProviderCounts(t, klineDBPath, wantCached)

			progress := syncLiveBABAHistory(t, ctx, srv.URL, providerID, startLabel, endLabel)
			if progress.CompletedIntervals != 1 || progress.CompletedBatches == 0 {
				t.Fatalf("BABA sync progress = %+v", progress)
			}
			cached := readCachedDailyKLines(t, klineDBPath, providerID, startDate)
			assertOneYearBABAHistory(t, cached)
			wantCached[providerID] = len(cached)
			assertCachedProviderCounts(t, klineDBPath, wantCached)

			run := runLiveBABAOneYearBacktest(
				t, ctx, srv.URL, definitionID, providerID, startLabel, endLabel,
			)
			runProviders[run.ID] = providerID
			assertBacktestRunListed(t, srv.URL, run.ID, providerID)
			t.Logf(
				"BABA %s integration passed: range=%s..%s cached=%d replayed=%d trades=%d orders=%d finalBalance=%.2f",
				providerID, startLabel, endLabel, len(cached), len(run.Result.Candles),
				run.Result.TotalTrades, len(run.Result.OrderBook), run.Result.FinalBalance,
			)
		}); !ok {
			t.FailNow()
		}
	}
	assertProviderCacheTables(t, klineDBPath)

	srv.Close()
	if err := handler.Close(); err != nil {
		t.Fatalf("close live provider matrix handler: %v", err)
	}
	assertPersistedBacktestProviders(t, runDBPath, runProviders)
}

func syncLiveBABAHistory(
	t *testing.T,
	ctx context.Context,
	baseURL string,
	providerID string,
	startLabel string,
	endLabel string,
) *pkgbacktest.SyncProgress {
	t.Helper()
	syncResp := postBacktestSync(t, baseURL, btsrv.SyncRequest{
		Market: "US", Code: "BABA", Intervals: []string{"1d"},
		StartDate: startLabel, EndDate: endLabel,
		RehabType: "none", SessionScope: "regular",
	})
	var syncEnvelope struct {
		Data btsrv.SyncStarted `json:"data"`
	}
	decodeBacktestResponse(t, syncResp, &syncEnvelope)
	if syncEnvelope.Data.Symbol != "US.BABA" || syncEnvelope.Data.MarketDataProvider != providerID {
		t.Fatalf("BABA sync source = %+v", syncEnvelope.Data)
	}
	return waitForLiveProviderSync(t, ctx, baseURL, syncEnvelope.Data.TaskID, providerID)
}

func runLiveBABAOneYearBacktest(
	t *testing.T,
	ctx context.Context,
	baseURL, definitionID, providerID, startLabel, endLabel string,
) btsrv.RunState {
	t.Helper()
	regularOnly := false
	runResp := postBacktestRun(t, baseURL, btsrv.StartRequest{
		DefinitionID:     definitionID,
		Market:           "US",
		Code:             "BABA",
		Interval:         "1d",
		StartDate:        startLabel,
		EndDate:          endLabel,
		RehabType:        "none",
		InitialBalance:   100000,
		UseExtendedHours: &regularOnly,
	})
	var queued struct {
		Data btsrv.RunState `json:"data"`
	}
	decodeBacktestResponse(t, runResp, &queued)
	if queued.Data.ID == "" || queued.Data.Status != "queued" {
		t.Fatalf("BABA queued run = %+v", queued.Data)
	}
	run := waitForLiveBacktest(t, ctx, baseURL, queued.Data.ID)
	if run.Status != "completed" || run.MarketDataProvider != providerID || run.Result == nil {
		t.Fatalf("BABA run = %+v", run)
	}
	if run.Result.Error != "" || run.Result.MarketDataProvider != providerID ||
		len(run.Result.Candles) < 240 || run.Result.QuoteCurrency != "USD" {
		t.Fatalf(
			"BABA result source=%q currency=%q error=%q candles=%d",
			run.Result.MarketDataProvider, run.Result.QuoteCurrency,
			run.Result.Error, len(run.Result.Candles),
		)
	}
	if run.Result.TotalTrades == 0 || len(run.Result.OrderBook) == 0 {
		t.Fatalf("BABA result trades=%d orders=%d", run.Result.TotalTrades, len(run.Result.OrderBook))
	}
	assertNoProviderRuntimeWarning(t, run.Result)
	return run
}

func liveBABAOneYearStrategy() string {
	return `//@version=6
strategy("Live Multi Provider BABA One Year", overlay=true, initial_capital=100000, default_qty_type=strategy.fixed, default_qty_value=1)
if bar_index == 10
    strategy.entry("Long", strategy.long, qty=1)
if bar_index == 20
    strategy.close("Long")
if bar_index == 30
    strategy.entry("Short", strategy.short, qty=1)
if bar_index == 40
    strategy.close("Short")
plot(close)`
}

func putBacktestMarketDataProvider(
	t *testing.T,
	baseURL string,
	provider jfsettings.ActiveMarketDataProvider,
) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"activeProvider": provider})
	if err != nil {
		t.Fatalf("encode backtest provider setting: %v", err)
	}
	settings := requestBacktestMarketDataProviderSettings(
		t, http.MethodPut, baseURL, bytes.NewReader(body),
	)
	assertProviderSettings(t, settings, provider)
}

func assertBacktestMarketDataProvider(
	t *testing.T,
	baseURL string,
	provider jfsettings.ActiveMarketDataProvider,
) {
	t.Helper()
	settings := requestBacktestMarketDataProviderSettings(t, http.MethodGet, baseURL, nil)
	assertProviderSettings(t, settings, provider)
}

type backtestProviderSettings struct {
	ActiveProvider     jfsettings.ActiveMarketDataProvider `json:"activeProvider"`
	AvailableProviders []marketdata.ProviderDescriptor     `json:"availableProviders"`
}

func requestBacktestMarketDataProviderSettings(
	t *testing.T,
	method, baseURL string,
	body io.Reader,
) backtestProviderSettings {
	t.Helper()
	req, err := http.NewRequestWithContext(
		t.Context(), method, baseURL+"/api/v1/settings/backtest-market-data-provider", body,
	)
	if err != nil {
		t.Fatalf("build %s backtest provider settings request: %v", method, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s backtest provider settings: %v", method, err)
	}
	var envelope struct {
		Data backtestProviderSettings `json:"data"`
	}
	decodeBacktestResponse(t, resp, &envelope)
	return envelope.Data
}

func assertProviderSettings(
	t *testing.T,
	settings backtestProviderSettings,
	want jfsettings.ActiveMarketDataProvider,
) {
	t.Helper()
	if settings.ActiveProvider != want {
		t.Fatalf("active backtest provider = %q, want %q", settings.ActiveProvider, want)
	}
	wantIDs := map[string]bool{"futu": false, "yfinance": false, "akshare": false}
	for _, descriptor := range settings.AvailableProviders {
		if _, tracked := wantIDs[descriptor.SelectionID]; tracked {
			if !descriptor.Capabilities.HistoricalCandles {
				t.Fatalf("provider %s does not advertise historical candles", descriptor.SelectionID)
			}
			wantIDs[descriptor.SelectionID] = true
		}
	}
	for providerID, found := range wantIDs {
		if !found {
			t.Fatalf("provider catalog does not contain %s: %+v", providerID, settings.AvailableProviders)
		}
	}
}

func assertCachedProviderCounts(
	t *testing.T,
	dbPath string,
	want map[string]int,
) {
	t.Helper()
	for _, providerID := range []string{"futu", "yfinance", "akshare"} {
		count := cachedProviderRowCount(t, dbPath, providerID)
		if count != want[providerID] {
			t.Fatalf(
				"cached %s BABA candles = %d, want %d; provider cache changed outside its sync",
				providerID, count, want[providerID],
			)
		}
	}
}

func cachedProviderRowCount(t *testing.T, dbPath, providerID string) int {
	t.Helper()
	store, err := pkgbacktest.NewKLineStore(dbPath, providerID)
	if err != nil {
		t.Fatalf("open %s provider cache: %v", providerID, err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	tableName := pkgbacktest.KLineTableNameForProviderAndSessionScope(
		providerID, "US.BABA", bbgotypes.Interval1d, "none", pkgbacktest.KLineSessionScopeRegular,
	)
	var exists int
	if err := store.DB().QueryRowContext(
		t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName,
	).Scan(&exists); err != nil {
		t.Fatalf("query %s provider cache table: %v", providerID, err)
	}
	if exists == 0 {
		return 0
	}
	store.SetRehabType("none")
	store.SetReadSessionScope("regular")
	rows, err := store.QueryKLinesForward(nil, "US.BABA", bbgotypes.Interval1d, time.Time{}, 1000)
	if err != nil {
		t.Fatalf("read %s provider cache rows: %v", providerID, err)
	}
	return len(rows)
}

func assertProviderCacheTables(t *testing.T, dbPath string) {
	t.Helper()
	store, err := pkgbacktest.NewKLineStore(dbPath, "futu")
	if err != nil {
		t.Fatalf("open provider cache catalog: %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	for _, providerID := range []string{"futu", "yfinance", "akshare"} {
		tableName := pkgbacktest.KLineTableNameForProviderAndSessionScope(
			providerID, "US.BABA", bbgotypes.Interval1d, "none", pkgbacktest.KLineSessionScopeRegular,
		)
		var count int
		if err := store.DB().QueryRowContext(
			t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName,
		).Scan(&count); err != nil {
			t.Fatalf("query %s cache table: %v", providerID, err)
		}
		if count != 1 {
			t.Fatalf("provider cache table %q count = %d, want 1", tableName, count)
		}
	}
}

func assertBacktestRunListed(t *testing.T, baseURL, runID, providerID string) {
	t.Helper()
	resp, err := jftradeTestHTTPGet(t, baseURL+"/api/v1/backtests")
	if err != nil {
		t.Fatalf("GET backtest history: %v", err)
	}
	var envelope struct {
		Data struct {
			Runs []btsrv.RunState `json:"runs"`
		} `json:"data"`
	}
	decodeBacktestResponse(t, resp, &envelope)
	for _, run := range envelope.Data.Runs {
		if run.ID == runID {
			if run.MarketDataProvider != providerID {
				t.Fatalf("listed run %s provider = %q, want %q", runID, run.MarketDataProvider, providerID)
			}
			return
		}
	}
	t.Fatalf("completed run %s is missing from history", runID)
}

func assertPersistedBacktestProviders(t *testing.T, dbPath string, want map[string]string) {
	t.Helper()
	store, err := backteststore.New(dbPath)
	if err != nil {
		t.Fatalf("reopen backtest run store: %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	for runID, providerID := range want {
		run, ok, err := store.GetFull(runID)
		if err != nil || !ok || run == nil || run.Result == nil {
			t.Fatalf("reload run %s = (%+v, %v, %v)", runID, run, ok, err)
		}
		if run.MarketDataProvider != providerID || run.Result.MarketDataProvider != providerID {
			t.Fatalf(
				"reloaded run %s providers = state:%q result:%q, want %q",
				runID, run.MarketDataProvider, run.Result.MarketDataProvider, providerID,
			)
		}
		assertNoProviderRuntimeWarning(t, run.Result)
	}
}

func assertNoProviderRuntimeWarning(t *testing.T, result *pkgbacktest.RunResult) {
	t.Helper()
	messages := []string{result.Error}
	messages = append(messages, result.Warnings...)
	messages = append(messages, result.Logs...)
	messages = append(messages, result.RuntimeErrors...)
	combined := strings.ToLower(strings.Join(messages, "\n"))
	for _, forbidden := range []string{
		"runtime_warming",
		"runtime is warming",
		"runtime unavailable",
		"market-data provider runtime is unavailable",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("backtest result contains provider runtime warning %q: %s", forbidden, combined)
		}
	}
}

func readCachedDailyKLines(t *testing.T, dbPath, providerID string, startDate time.Time) []bbgotypes.KLine {
	t.Helper()
	store, err := pkgbacktest.NewKLineStore(dbPath, providerID)
	if err != nil {
		t.Fatalf("open %s backtest cache: %v", providerID, err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	store.SetRehabType("none")
	store.SetReadSessionScope("regular")
	rows, err := store.QueryKLinesForward(nil, "US.BABA", bbgotypes.Interval1d, startDate.UTC(), 400)
	if err != nil {
		t.Fatalf("query cached BABA daily candles: %v", err)
	}
	return rows
}

func assertOneYearBABAHistory(t *testing.T, rows []bbgotypes.KLine) {
	t.Helper()
	if len(rows) < 240 || len(rows) > 270 {
		t.Fatalf("cached BABA daily candles = %d, want one trading year", len(rows))
	}
	if span := rows[len(rows)-1].EndTime.Time().Sub(rows[0].StartTime.Time()); span < 330*24*time.Hour {
		t.Fatalf("cached BABA history span = %s, want at least 330 days", span)
	}
	for index, row := range rows {
		if row.Symbol != "US.BABA" || row.Open.Sign() <= 0 || row.High.Sign() <= 0 ||
			row.Low.Sign() <= 0 || row.Close.Sign() <= 0 || row.Volume.Sign() < 0 {
			t.Fatalf("invalid cached BABA candle %d: %+v", index, row)
		}
		if row.High.Compare(row.Open) < 0 || row.High.Compare(row.Close) < 0 ||
			row.Low.Compare(row.Open) > 0 || row.Low.Compare(row.Close) > 0 ||
			!row.EndTime.Time().After(row.StartTime.Time()) {
			t.Fatalf("inconsistent cached BABA candle %d: %+v", index, row)
		}
		if index > 0 && !row.StartTime.Time().After(rows[index-1].StartTime.Time()) {
			t.Fatalf(
				"cached BABA candles are unordered or duplicated at %d: previous=%s current=%s",
				index, rows[index-1].StartTime.Time(), row.StartTime.Time(),
			)
		}
	}
}

func postBacktestSync(t testing.TB, baseURL string, request btsrv.SyncRequest) *http.Response {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode sync request: %v", err)
	}
	resp, err := jftradeTestHTTPPost(t, baseURL+"/api/v1/backtests/sync", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST backtest sync: %v", err)
	}
	return resp
}

func waitForLiveProviderSync(t *testing.T, ctx context.Context, baseURL, taskID, providerID string) *btsrv.SyncProgress {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("wait for %s sync: %v", providerID, ctx.Err())
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/backtests/sync/"+taskID, nil)
			if err != nil {
				t.Fatalf("build progress request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET sync progress: %v", err)
			}
			var envelope struct {
				Data btsrv.SyncProgress `json:"data"`
			}
			decodeBacktestResponse(t, resp, &envelope)
			if envelope.Data.MarketDataProvider != providerID {
				t.Fatalf("progress provider = %q, want %q", envelope.Data.MarketDataProvider, providerID)
			}
			switch envelope.Data.Status {
			case "completed":
				return &envelope.Data
			case "failed", "cancelled":
				t.Fatalf("%s sync status = %s: %s", providerID, envelope.Data.Status, envelope.Data.Error)
			}
		}
	}
}

func postBacktestRun(t testing.TB, baseURL string, request btsrv.StartRequest) *http.Response {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode backtest request: %v", err)
	}
	resp, err := jftradeTestHTTPPost(t, baseURL+"/api/v1/backtests", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST backtest: %v", err)
	}
	return resp
}

func waitForLiveBacktest(t *testing.T, ctx context.Context, baseURL, runID string) btsrv.RunState {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("wait for BABA backtest: %v", ctx.Err())
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/backtests/"+runID, nil)
			if err != nil {
				t.Fatalf("build backtest result request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET backtest result: %v", err)
			}
			var envelope struct {
				Data btsrv.RunState `json:"data"`
			}
			decodeBacktestResponse(t, resp, &envelope)
			switch envelope.Data.Status {
			case "completed":
				return envelope.Data
			case "failed", "cancelled":
				t.Fatalf("BABA backtest status = %s: %+v", envelope.Data.Status, envelope.Data.Result)
			}
		}
	}
}

func decodeBacktestResponse(t testing.TB, resp *http.Response, target any) {
	t.Helper()
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("backtest response status = %d, body = %s", resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode backtest response: %v", err)
	}
}
