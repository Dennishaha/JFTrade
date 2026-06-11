package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestInstantiateStoredDefinitionNormalizesLegacySourceFormatToDSL(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "legacy-breakout",
		Name:         "Legacy Breakout",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: "legacy-v0",
		Symbol:       "00700",
		Interval:     "1m",
		Script:       "//@version=6\nstrategy(\"Legacy Breakout\", overlay=true)\nlog.info(\"close\")",
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	createResp, err := http.Post(srv.URL+"/api/v1/strategy-definitions/legacy-breakout/instantiate", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST instantiate: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST normalized legacy source format instantiate status = %d, want %d", createResp.StatusCode, http.StatusOK)
	}
	var createEnvelope struct {
		OK   bool             `json:"ok"`
		Data strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode normalized instantiate: %v", err)
	}
	if createEnvelope.Data.SourceFormat != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("expected normalized Pine source format, got %+v", createEnvelope.Data)
	}
	if createEnvelope.Data.Runtime != strategyRuntimePinePlan {
		t.Fatalf("expected normalized Pine runtime, got %+v", createEnvelope.Data)
	}
}

func TestStrategyDefinitionPreviewUsesRequestedSymbolAndExtendedHours(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "dsl-preview-day-window",
		Name:         "Pine Preview Window",
		Version:      "0.1.0",
		Description:  "preview route should respect symbol and extended-hours",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "HK.00700",
		Interval:     "5m",
		Script: `//@version=6
strategy("Pine Preview Window", overlay=true)
slow = ta.sma(close, 66)
log.info("close")`,
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	defaultResp, err := http.Get(srv.URL + "/api/v1/strategy-definitions/dsl-preview-day-window?interval=5m")
	if err != nil {
		t.Fatalf("GET default strategy preview: %v", err)
	}
	defer defaultResp.Body.Close()
	if defaultResp.StatusCode != http.StatusOK {
		t.Fatalf("GET default strategy preview status = %d", defaultResp.StatusCode)
	}
	var defaultEnvelope struct {
		OK   bool                       `json:"ok"`
		Data strategyDefinitionResponse `json:"data"`
	}
	if err := json.NewDecoder(defaultResp.Body).Decode(&defaultEnvelope); err != nil {
		t.Fatalf("decode default strategy preview: %v", err)
	}
	if defaultEnvelope.Data.DerivedWarmupBars != 66 {
		t.Fatalf("default derivedWarmupBars = %d, want 66", defaultEnvelope.Data.DerivedWarmupBars)
	}

	previewResp, err := http.Get(srv.URL + "/api/v1/strategy-definitions/dsl-preview-day-window?interval=5m&symbol=US.AAPL&useExtendedHours=true")
	if err != nil {
		t.Fatalf("GET extended strategy preview: %v", err)
	}
	defer previewResp.Body.Close()
	if previewResp.StatusCode != http.StatusOK {
		t.Fatalf("GET extended strategy preview status = %d", previewResp.StatusCode)
	}
	var previewEnvelope struct {
		OK   bool                       `json:"ok"`
		Data strategyDefinitionResponse `json:"data"`
	}
	if err := json.NewDecoder(previewResp.Body).Decode(&previewEnvelope); err != nil {
		t.Fatalf("decode extended strategy preview: %v", err)
	}
	if previewEnvelope.Data.DerivedWarmupBars != 66 {
		t.Fatalf("extended derivedWarmupBars = %d, want 66", previewEnvelope.Data.DerivedWarmupBars)
	}
	if previewEnvelope.Data.DerivedWarmupInterval != "5m" {
		t.Fatalf("extended derivedWarmupInterval = %q, want 5m", previewEnvelope.Data.DerivedWarmupInterval)
	}
}
