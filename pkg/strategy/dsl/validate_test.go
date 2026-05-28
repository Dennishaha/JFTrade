package dsl

import "testing"

func TestValidateScriptAcceptsMinimalHookBlocks(t *testing.T) {
	script := `strategy Mean Revert
version 0.1.0
symbol 00700
interval 1m

on init:
  log "boot"

on kline_close:
  buy shares 100`

	if err := ValidateScript(script); err != nil {
		t.Fatalf("ValidateScript() error = %v", err)
	}
}

func TestValidateScriptRejectsMissingHookBody(t *testing.T) {
	script := `strategy Mean Revert
on kline_close:`

	err := ValidateScript(script)
	if err == nil {
		t.Fatal("ValidateScript() error = nil, want missing hook body error")
	}
}

func TestValidateScriptRejectsUnsupportedTopLevelStatement(t *testing.T) {
	script := `strategy Mean Revert
when market_open:
  log "x"`

	err := ValidateScript(script)
	if err == nil {
		t.Fatal("ValidateScript() error = nil, want unsupported statement error")
	}
}

func TestValidateScriptRejectsInvalidIndicatorBinding(t *testing.T) {
	script := `strategy Mean Revert
on kline_close:
  let fast = ma(EMA, nope, day)
  log "x"`

	err := ValidateScript(script)
	if err == nil {
		t.Fatal("ValidateScript() error = nil, want invalid indicator binding error")
	}
}
