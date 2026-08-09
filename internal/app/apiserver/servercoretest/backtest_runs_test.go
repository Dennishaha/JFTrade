package servercoretest

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
)

func seedBacktestRuns(t *testing.T, store *servercore.SettingsStore, runs ...*btsrv.RunState) {
	t.Helper()
	runStore, err := backteststore.New(backteststore.DerivePath(store.Path()))
	if err != nil {
		t.Fatalf("open backtest run store: %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := runStore.(interface{ Close() error }); ok {
			jftradeCheckTestError(t, closer.Close())
		}
	})
	for _, run := range runs {
		if err := runStore.Add(run); err != nil {
			t.Fatalf("persist backtest run %s: %v", run.ID, err)
		}
	}
}

func TestNewServerReloadsPersistedBacktestRuns(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	completedRun := &btsrv.RunState{
		ID:     "bt-reload-completed",
		Status: "completed",
		Request: btsrv.StartRequest{
			DefinitionID: "dsl-reload-completed",
			Symbol:       "US.AAPL",
			Interval:     "5m",
			StartTime:    "2026-05-01T00:00:00Z",
			EndTime:      "2026-05-02T00:00:00Z",
		},
		Result: &btsrv.RunResult{
			Symbol:       "US.AAPL",
			Interval:     "5m",
			StartTime:    "2026-05-01T00:00:00Z",
			EndTime:      "2026-05-02T00:00:00Z",
			FinalBalance: 100123,
		},
		CreatedAt: "2026-05-30T00:00:00Z",
		UpdatedAt: "2026-05-30T00:00:01Z",
	}
	runningRun := &btsrv.RunState{
		ID:     "bt-reload-running",
		Status: "running",
		Request: btsrv.StartRequest{
			DefinitionID: "dsl-reload-running",
			Symbol:       "US.TSLA",
			Interval:     "1m",
			StartTime:    "2026-05-03T00:00:00Z",
			EndTime:      "2026-05-04T00:00:00Z",
		},
		CreatedAt: "2026-05-30T00:00:02Z",
		UpdatedAt: "2026-05-30T00:00:03Z",
	}
	seedBacktestRuns(t, store, completedRun, runningRun)

	srv := newHTTPTestServer(t, store)

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/backtests")
	if err != nil {
		t.Fatalf("GET backtests: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET backtests status = %d", listResp.StatusCode)
	}
	var listEnvelope struct {
		Data struct {
			Runs []btsrv.RunState `json:"runs"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listEnvelope.Data.Runs) != 2 {
		t.Fatalf("expected 2 reloaded runs, got %+v", listEnvelope.Data.Runs)
	}
	byID := make(map[string]btsrv.RunState, len(listEnvelope.Data.Runs))
	for _, run := range listEnvelope.Data.Runs {
		byID[run.ID] = run
	}
	if byID[completedRun.ID].Status != "completed" {
		t.Fatalf("unexpected reloaded completed run: %+v", byID[completedRun.ID])
	}
	if byID[runningRun.ID].Status != "failed" {
		t.Fatalf("unexpected reloaded running run: %+v", byID[runningRun.ID])
	}

	detailResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/backtests/"+runningRun.ID)
	if err != nil {
		t.Fatalf("GET backtest detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, detailResp.Body.Close()) }()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("GET backtest detail status = %d", detailResp.StatusCode)
	}
	var detailEnvelope struct {
		Data btsrv.RunState `json:"data"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detailEnvelope); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailEnvelope.Data.Result == nil || !strings.Contains(detailEnvelope.Data.Result.Error, backteststore.RecoveredRunErrorText) {
		t.Fatalf("expected recovered error on reloaded running run: %+v", detailEnvelope.Data.Result)
	}
}

func TestBacktestRouteDeletesTerminalRuns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	completedRun := &btsrv.RunState{
		ID:     "bt-delete-completed",
		Status: "completed",
		Request: btsrv.StartRequest{
			DefinitionID:   "dsl-delete-completed",
			Symbol:         "US.AAPL",
			Interval:       "1m",
			InitialBalance: 10000,
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	// Any non-terminal status (starting, paused, ...) must stay undeletable.
	// "running"/"queued" are recovered to "failed" on reload, so use a
	// non-recovered non-terminal status to exercise the route guard.
	pendingRun := &btsrv.RunState{
		ID:     "bt-delete-pending",
		Status: "starting",
		Request: btsrv.StartRequest{
			DefinitionID:   "dsl-delete-pending",
			Symbol:         "US.AAPL",
			Interval:       "1m",
			InitialBalance: 10000,
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	seedBacktestRuns(t, store, completedRun, pendingRun)

	srv := newHTTPTestServer(t, store)

	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/backtests/"+completedRun.ID, nil)
	if err != nil {
		t.Fatalf("build delete backtest request: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE backtest: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE backtest status = %d, want %d", deleteResp.StatusCode, http.StatusOK)
	}
	var deleteEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Deleted bool   `json:"deleted"`
			ID      string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(deleteResp.Body).Decode(&deleteEnvelope); err != nil {
		t.Fatalf("decode delete backtest response: %v", err)
	}
	if !deleteEnvelope.Data.Deleted || deleteEnvelope.Data.ID != completedRun.ID {
		t.Fatalf("unexpected delete backtest response: %+v", deleteEnvelope.Data)
	}

	blockedReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/backtests/"+pendingRun.ID, nil)
	if err != nil {
		t.Fatalf("build delete pending backtest request: %v", err)
	}
	blockedResp, err := http.DefaultClient.Do(blockedReq)
	if err != nil {
		t.Fatalf("DELETE pending backtest: %v", err)
	}
	defer func() { jftradeCheckTestError(t, blockedResp.Body.Close()) }()
	if blockedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("DELETE pending backtest status = %d, want %d", blockedResp.StatusCode, http.StatusBadRequest)
	}
}

func TestBacktestListReturnsLightweightRunsAndResultReturnsDetail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	run := &btsrv.RunState{
		ID:     "bt-summary-detail",
		Status: "completed",
		Request: btsrv.StartRequest{
			DefinitionID:   "dsl-summary-detail",
			Symbol:         "US.NVDA",
			Interval:       "5m",
			InitialBalance: 10000,
		},
		Result: &btsrv.RunResult{
			Symbol:       "US.NVDA",
			Interval:     "5m",
			FinalBalance: 10001,
			PnLCurve:     []btsrv.PnLPoint{{Time: "2026-01-01T00:00:00Z", Equity: 10001}},
			Candles:      []btsrv.Candle{{Time: "2026-01-01T00:00:00Z", Open: "1", High: "2", Low: "1", Close: "2", Volume: "100"}},
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	seedBacktestRuns(t, store, run)

	srv := newHTTPTestServer(t, store)

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/backtests")
	if err != nil {
		t.Fatalf("GET backtests: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET backtests status = %d", listResp.StatusCode)
	}
	var listEnvelope struct {
		Data struct {
			Runs []btsrv.RunState `json:"runs"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listEnvelope.Data.Runs) != 1 {
		t.Fatalf("unexpected list response: %+v", listEnvelope.Data.Runs)
	}
	if listEnvelope.Data.Runs[0].Result != nil {
		t.Fatalf("list response included result: %+v", listEnvelope.Data.Runs[0].Result)
	}

	detailResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/backtests/"+run.ID)
	if err != nil {
		t.Fatalf("GET backtest detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, detailResp.Body.Close()) }()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("GET backtest detail status = %d", detailResp.StatusCode)
	}
	var detailEnvelope struct {
		Data btsrv.RunState `json:"data"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detailEnvelope); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailEnvelope.Data.Result == nil || len(detailEnvelope.Data.Result.Candles) != 1 || len(detailEnvelope.Data.Result.PnLCurve) != 1 {
		t.Fatalf("detail response missing full series: %+v", detailEnvelope.Data.Result)
	}
}

func TestBacktestRoutesCreateRuntimeLayoutForMissingBacktestDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(t.TempDir(), "missing", "nested", "backtest.db"))

	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/backtests")
	if err != nil {
		t.Fatalf("GET backtests: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET backtests status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
