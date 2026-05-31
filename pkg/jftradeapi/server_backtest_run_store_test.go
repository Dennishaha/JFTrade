package jftradeapi

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
	server := NewServer(store)

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

	reloadedStore, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore reload: %v", err)
	}
	reloadedServer := NewServer(reloadedStore)

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
	server := NewServer(store)

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
	defer srv.Close()

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
