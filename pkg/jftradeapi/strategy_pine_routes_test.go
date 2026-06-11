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

	body := []byte(`{"sourceFormat":"pine-v6","includeAst":true,"script":"//@version=6\nstrategy(\"Analyze\", overlay=true)\nfast = ta.ema(close, 8)\nif close > close[1]\n    strategy.entry(\"Long\", strategy.long, qty=1)"}`)
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
			AST map[string]any `json:"ast"`
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
	if len(envelope.Data.Requirements.Indicators) != 1 {
		t.Fatalf("indicators = %#v, want one", envelope.Data.Requirements.Indicators)
	}
	if envelope.Data.AST == nil {
		t.Fatal("ast missing")
	}
}

func TestAnalyzeStrategyPineRouteReportsUnsupportedSyntax(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"script":"//@version=6\nstrategy(\"Analyze\", overlay=true)\nfor i = 0 to 2\n    log.info(\"x\")"}`)
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
	if len(envelope.Data.Diagnostics) == 0 || envelope.Data.Diagnostics[0].Code != "PINE_FOR_UNSUPPORTED" || envelope.Data.Diagnostics[0].Line != 3 {
		t.Fatalf("diagnostics = %#v", envelope.Data.Diagnostics)
	}
}
