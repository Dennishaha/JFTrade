package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/backtest"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestBacktestRouteUsesDerivedStrategyWarmup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-route-auto-warmup.db")
	t.Setenv("JFTRADE_BACKTEST_DB", dbPath)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	worker := newFakeStrategyRuntimePineWorker()
	server.backtestSvc.SetPineWorkerRunner(worker)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "dsl-auto-warmup-route",
		Name:         "Pine Auto Warmup Route",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL",
		Interval:     "1m",
		Script: `//@version=6
strategy("Pine Auto Warmup Route", overlay=true)
fast = ta.sma(close, 1)
slow = ta.sma(close, 20)
strategy.entry("Long", strategy.long, qty=1)`,
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}

	klineStore, err := backtest.NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := make([]bbgotypes.KLine, 0, 25)
	for index := range 25 {
		startAt := baseStart.Add(time.Duration(index) * time.Minute)
		openPrice := 100.0
		closePrice := 100.0
		switch {
		case index == 20:
			closePrice = 120.0
		case index > 20:
			openPrice = 120.0
			closePrice = 121.0
		}
		klines = append(klines, bbgotypes.KLine{
			StartTime: bbgotypes.Time(startAt),
			EndTime:   bbgotypes.Time(startAt.Add(time.Minute - time.Millisecond)),
			Interval:  bbgotypes.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(openPrice),
			High:      fixedpoint.NewFromFloat(closePrice + 1),
			Low:       fixedpoint.NewFromFloat(openPrice - 1),
			Close:     fixedpoint.NewFromFloat(closePrice),
			Volume:    fixedpoint.NewFromFloat(1000),
		})
	}
	if err := klineStore.InsertKLines(klines, "forward"); err != nil {
		jftradeErr1 := klineStore.Close()
		jftradeCheckTestError(t, jftradeErr1)
		t.Fatalf("InsertKLines: %v", err)
	}
	if err := klineStore.Close(); err != nil {
		t.Fatalf("klineStore.Close: %v", err)
	}
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		barIndex := len(request.Candles) - 1
		formalStart := klines[20].StartTime.Time().UnixMilli()
		for index, candle := range request.Candles {
			if candle.OpenTime >= formalStart {
				barIndex = index
				break
			}
		}
		return pineworker.RunScriptResponse{
			JobID: request.JobID,
			OrderIntents: []pineworker.OrderIntent{{
				Kind:        "entry",
				ID:          "Long",
				Direction:   "long",
				Quantity:    1,
				HasQuantity: true,
				BarIndex:    barIndex,
				Time:        request.Candles[barIndex].OpenTime,
			}},
		}
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body, jftradeErr2 := json.Marshal(map[string]any{
		"definitionId":   "dsl-auto-warmup-route",
		"symbol":         "US.AAPL",
		"interval":       "1m",
		"startTime":      klines[20].StartTime.Time().Format(time.RFC3339),
		"endTime":        klines[24].EndTime.Time().Format(time.RFC3339),
		"initialBalance": 10000,
		"rehabType":      "forward",
	})
	jftradeCheckTestError(t, jftradeErr2)
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/backtests", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST backtest: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST backtest status = %d", createResp.StatusCode)
	}
	var createEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode backtest create response: %v", err)
	}

	var runEnvelope struct {
		OK   bool             `json:"ok"`
		Data backtestRunState `json:"data"`
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resultResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/backtests/"+createEnvelope.Data.ID)
		if err != nil {
			t.Fatalf("GET backtest result: %v", err)
		}
		if resultResp.StatusCode != http.StatusOK {
			jftradeCheckTestError(t, resultResp.Body.Close())
			t.Fatalf("GET backtest result status = %d", resultResp.StatusCode)
		}
		if err := json.NewDecoder(resultResp.Body).Decode(&runEnvelope); err != nil {
			jftradeCheckTestError(t, resultResp.Body.Close())
			t.Fatalf("decode backtest result: %v", err)
		}
		jftradeCheckTestError(t, resultResp.Body.Close())
		if runEnvelope.Data.Status == "completed" || runEnvelope.Data.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if runEnvelope.Data.Status != "completed" {
		if runEnvelope.Data.Result != nil {
			t.Fatalf("backtest status = %s, error = %q", runEnvelope.Data.Status, runEnvelope.Data.Result.Error)
		}
		t.Fatalf("backtest status = %s, expected completed", runEnvelope.Data.Status)
	}
	if runEnvelope.Data.Result == nil {
		t.Fatal("expected backtest result payload")
	}
	if runEnvelope.Data.Result.Error != "" {
		t.Fatalf("backtest result error = %q", runEnvelope.Data.Result.Error)
	}
	if runEnvelope.Data.Result.TotalTrades == 0 {
		if request, ok := worker.lastRequest(); ok {
			formalStart := klines[20].StartTime.Time().UnixMilli()
			formalIndex := -1
			for index, candle := range request.Candles {
				if candle.OpenTime >= formalStart {
					formalIndex = index
					break
				}
			}
			t.Fatalf("TotalTrades = %d, want > 0 (worker candles=%d formalIndex=%d)", runEnvelope.Data.Result.TotalTrades, len(request.Candles), formalIndex)
		}
		t.Fatalf("TotalTrades = %d, want > 0", runEnvelope.Data.Result.TotalTrades)
	}
	if len(runEnvelope.Data.Result.DrawdownCurve) != len(runEnvelope.Data.Result.PnLCurve) {
		t.Fatalf("DrawdownCurve len = %d, want %d", len(runEnvelope.Data.Result.DrawdownCurve), len(runEnvelope.Data.Result.PnLCurve))
	}
	if runEnvelope.Data.Result.MaxDrawdown < 0 {
		t.Fatalf("MaxDrawdown = %f, want >= 0", runEnvelope.Data.Result.MaxDrawdown)
	}
	if runEnvelope.Data.Result.CurrentDrawdown < 0 {
		t.Fatalf("CurrentDrawdown = %f, want >= 0", runEnvelope.Data.Result.CurrentDrawdown)
	}
	if len(runEnvelope.Data.Result.OrderBook) == 0 {
		t.Fatal("expected order book entries from auto warmup backtest")
	}
}
