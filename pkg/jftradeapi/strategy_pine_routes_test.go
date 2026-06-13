package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAnalyzeStrategyPineRouteReturnsDiagnosticsAndRequirements(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"sourceFormat":"pine-v6","includeAst":true,"script":"//@version=6\nstrategy(\"Analyze\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)\nstart = input.time(timestamp(2026, 1, 1), \"Start\")\nsignalColor = input.color(color.green, \"Signal\")\nfast = ta.ema(close, 8)\navgVol = ta.sma(volume, 20)\nsar = ta.sar(0.02, 0.02, 0.2)\nif barstate.isconfirmed and session.ismarket and dayofweek == dayofweek.monday and time >= start and close > close[1] and volume > avgVol and close > sar\n    strategy.entry(\"Long\", strategy.long)"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			OK           bool             `json:"ok"`
			Diagnostics  []map[string]any `json:"diagnostics"`
			Features     []string         `json:"features"`
			Requirements struct {
				Indicators []map[string]any `json:"indicators"`
			} `json:"requirements"`
			Metadata map[string]any `json:"metadata"`
			AST      map[string]any `json:"ast"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if !envelope.OK || !envelope.Data.OK {
		t.Fatalf("analyze envelope = %#v", envelope)
	}
	if len(envelope.Data.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want empty", envelope.Data.Diagnostics)
	}
	if len(envelope.Data.Features) == 0 {
		t.Fatal("features is empty")
	}
	if len(envelope.Data.Requirements.Indicators) != 3 {
		t.Fatalf("indicators = %#v, want three", envelope.Data.Requirements.Indicators)
	}
	if !stringSliceContains(envelope.Data.Features, "expression.input_defaults") ||
		!stringSliceContains(envelope.Data.Features, "indicator.ma_source_aware") ||
		!stringSliceContains(envelope.Data.Features, "expression.barstate_session") ||
		!stringSliceContains(envelope.Data.Features, "expression.pine_constants") ||
		!stringSliceContains(envelope.Data.Features, "indicator.sar") ||
		!stringSliceContains(envelope.Data.Features, "order.strategy_order_net") ||
		!stringSliceContains(envelope.Data.Features, "order.qty_percent") ||
		!stringSliceContains(envelope.Data.Features, "order.close_all") ||
		!stringSliceContains(envelope.Data.Features, "order.exit_quantity") {
		t.Fatalf("features = %#v", envelope.Data.Features)
	}
	if envelope.Data.Requirements.Indicators[1]["key"] != "ma:SMA:20:volume" {
		t.Fatalf("indicators = %#v, want source-aware volume MA", envelope.Data.Requirements.Indicators)
	}
	if envelope.Data.Requirements.Indicators[2]["key"] != "sar:0.02:0.02:0.2" {
		t.Fatalf("indicators = %#v, want SAR requirement", envelope.Data.Requirements.Indicators)
	}
	if envelope.Data.Metadata["defaultQtyMode"] != "percent_of_equity" || envelope.Data.Metadata["defaultQtyValue"] != "10" || envelope.Data.Metadata["pyramiding"] != float64(2) {
		t.Fatalf("metadata = %#v", envelope.Data.Metadata)
	}
	if envelope.Data.AST == nil {
		t.Fatal("ast missing")
	}
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestAnalyzeStrategyPineRouteReportsUnsupportedSyntax(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"script":"//@version=6\nstrategy(\"Analyze\", overlay=true)\nwhile close > open\n    log.info(\"x\")"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data struct {
			OK          bool `json:"ok"`
			Diagnostics []struct {
				Code string `json:"code"`
				Line int    `json:"line"`
			} `json:"diagnostics"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if envelope.Data.OK {
		t.Fatal("analyze ok = true, want false")
	}
	if len(envelope.Data.Diagnostics) == 0 || envelope.Data.Diagnostics[0].Code != "PINE_WHILE_UNSUPPORTED" || envelope.Data.Diagnostics[0].Line != 3 {
		t.Fatalf("diagnostics = %#v", envelope.Data.Diagnostics)
	}
}
