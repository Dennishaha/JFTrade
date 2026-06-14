package servercore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestNewServerReloadsPersistedBacktestRuns(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	completedRun := &backtestRunState{
		ID:     "bt-reload-completed",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID: "dsl-reload-completed",
			Symbol:       "US.AAPL",
			Interval:     "5m",
			StartTime:    "2026-05-01T00:00:00Z",
			EndTime:      "2026-05-02T00:00:00Z",
		},
		Result: &backtest.RunResult{
			Symbol:       "US.AAPL",
			Interval:     "5m",
			StartTime:    "2026-05-01T00:00:00Z",
			EndTime:      "2026-05-02T00:00:00Z",
			FinalBalance: 100123,
		},
		CreatedAt: "2026-05-30T00:00:00Z",
		UpdatedAt: "2026-05-30T00:00:01Z",
	}
	runningRun := &backtestRunState{
		ID:     "bt-reload-running",
		Status: "running",
		Request: backtestStartRequest{
			DefinitionID: "dsl-reload-running",
			Symbol:       "US.TSLA",
			Interval:     "1m",
			StartTime:    "2026-05-03T00:00:00Z",
			EndTime:      "2026-05-04T00:00:00Z",
		},
		CreatedAt: "2026-05-30T00:00:02Z",
		UpdatedAt: "2026-05-30T00:00:03Z",
	}
	if err := server.backtestRuns.add(completedRun); err != nil {
		t.Fatalf("persist completed run: %v", err)
	}
	if err := server.backtestRuns.add(runningRun); err != nil {
		t.Fatalf("persist running run: %v", err)
	}

	// Close the first server so the reloaded store can acquire the DB on Windows.
	_ = server.Close()

	reloadedStore, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore reload: %v", err)
	}
	reloadedServer := newTestServer(t, reloadedStore)

	runs := reloadedServer.backtestRuns.list()
	if len(runs) != 2 {
		t.Fatalf("expected 2 reloaded runs, got %+v", runs)
	}
	byID := make(map[string]*backtestRunState, len(runs))
	for _, run := range runs {
		byID[run.ID] = run
	}
	if byID[completedRun.ID] == nil || byID[completedRun.ID].Status != "completed" {
		t.Fatalf("unexpected reloaded completed run: %+v", byID[completedRun.ID])
	}
	if byID[runningRun.ID] == nil || byID[runningRun.ID].Status != "failed" {
		t.Fatalf("unexpected reloaded running run: %+v", byID[runningRun.ID])
	}
	if byID[runningRun.ID].Result == nil || !strings.Contains(byID[runningRun.ID].Result.Error, recoveredBacktestRunErrorText) {
		t.Fatalf("expected recovered error on reloaded running run: %+v", byID[runningRun.ID])
	}
}

func TestBacktestRouteDeletesTerminalRuns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	completedRun := &backtestRunState{
		ID:     "bt-delete-completed",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID:   "dsl-delete-completed",
			Symbol:         "US.AAPL",
			Interval:       "1m",
			InitialBalance: 10000,
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	runningRun := &backtestRunState{
		ID:     "bt-delete-running",
		Status: "running",
		Request: backtestStartRequest{
			DefinitionID:   "dsl-delete-running",
			Symbol:         "US.AAPL",
			Interval:       "1m",
			InitialBalance: 10000,
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := server.backtestRuns.add(completedRun); err != nil {
		t.Fatalf("persist completed run: %v", err)
	}
	if err := server.backtestRuns.add(runningRun); err != nil {
		t.Fatalf("persist running run: %v", err)
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	deleteReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/backtests/"+completedRun.ID, nil)
	if err != nil {
		t.Fatalf("build delete backtest request: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE backtest: %v", err)
	}
	defer deleteResp.Body.Close()
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
	if _, ok := server.backtestRuns.get(completedRun.ID); ok {
		t.Fatal("expected completed backtest run to be removed")
	}

	blockedReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/backtests/"+runningRun.ID, nil)
	if err != nil {
		t.Fatalf("build delete running backtest request: %v", err)
	}
	blockedResp, err := http.DefaultClient.Do(blockedReq)
	if err != nil {
		t.Fatalf("DELETE running backtest: %v", err)
	}
	defer blockedResp.Body.Close()
	if blockedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("DELETE running backtest status = %d, want %d", blockedResp.StatusCode, http.StatusBadRequest)
	}
	if _, ok := server.backtestRuns.get(runningRun.ID); !ok {
		t.Fatal("running backtest run should not be deleted")
	}
}

func TestBacktestListReturnsLightweightRunsAndResultReturnsDetail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	run := &backtestRunState{
		ID:     "bt-summary-detail",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID:   "dsl-summary-detail",
			Symbol:         "US.NVDA",
			Interval:       "5m",
			InitialBalance: 10000,
		},
		Result: &backtest.RunResult{
			Symbol:       "US.NVDA",
			Interval:     "5m",
			FinalBalance: 10001,
			PnLCurve:     []backtest.PnLPoint{{Time: "2026-01-01T00:00:00Z", Equity: 10001}},
			Candles:      []backtest.Candle{{Time: "2026-01-01T00:00:00Z", Open: "1", High: "2", Low: "1", Close: "2", Volume: "100"}},
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := server.backtestRuns.add(run); err != nil {
		t.Fatalf("persist run: %v", err)
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	listResp, err := http.Get(srv.URL + "/api/v1/backtests")
	if err != nil {
		t.Fatalf("GET backtests: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET backtests status = %d", listResp.StatusCode)
	}
	var listEnvelope struct {
		Data struct {
			Runs []backtestRunState `json:"runs"`
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

	detailResp, err := http.Get(srv.URL + "/api/v1/backtests/" + run.ID)
	if err != nil {
		t.Fatalf("GET backtest detail: %v", err)
	}
	defer detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("GET backtest detail status = %d", detailResp.StatusCode)
	}
	var detailEnvelope struct {
		Data backtestRunState `json:"data"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detailEnvelope); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailEnvelope.Data.Result == nil || len(detailEnvelope.Data.Result.Candles) != 1 || len(detailEnvelope.Data.Result.PnLCurve) != 1 {
		t.Fatalf("detail response missing full series: %+v", detailEnvelope.Data.Result)
	}
}
