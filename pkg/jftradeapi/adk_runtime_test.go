package jftradeapi

import (
	"strings"
	"testing"
)

func TestValidateADKStrategyDraftScriptRejectsTradingViewPineScript(t *testing.T) {
	script := `//@version=6
strategy("TME_Bollinger_RSI_V1")
// TME Bollinger Bands + RSI Mean Reversion Strategy
plot(close)`

	err := validateADKStrategyDraftScript(script)
	if err == nil {
		t.Fatal("validateADKStrategyDraftScript() error = nil, want pine rejection")
	}
	if !strings.Contains(err.Error(), "JFTrade DSL v1") || !strings.Contains(err.Error(), "TradingView Pine Script") {
		t.Fatalf("validateADKStrategyDraftScript() error = %q, want DSL/Pine hint", err)
	}
}

func TestValidateADKStrategyDraftScriptAcceptsJFTradeDSL(t *testing.T) {
	script := `strategy Mean Revert
version 0.1.0
symbol US.TME
interval 1m

on kline_close:
  log "ready"`

	if err := validateADKStrategyDraftScript(script); err != nil {
		t.Fatalf("validateADKStrategyDraftScript() error = %v", err)
	}
}
