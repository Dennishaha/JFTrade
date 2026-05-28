package dsl

import (
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestParseScriptBuildsProgramSkeleton(t *testing.T) {
	script := `strategy Mean Revert
version 0.1.0
symbol 00700
interval 1m

on init:
  log "boot"

on kline_close:
  let fast = ma(EMA, 5, day)
  if cross_over(fast, slow):
    buy shares 100 policy flat_only limit 500
  else:
    protect auto stop_loss 1 day 2% window session
  notify "done"`

	program, err := ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	if program.SourceFormat != sourceFormatDSLV1 {
		t.Fatalf("program.SourceFormat = %q, want %q", program.SourceFormat, sourceFormatDSLV1)
	}
	if program.Metadata.Name != "Mean Revert" {
		t.Fatalf("program.Metadata.Name = %q, want %q", program.Metadata.Name, "Mean Revert")
	}
	if len(program.Hooks) != 2 {
		t.Fatalf("len(program.Hooks) = %d, want 2", len(program.Hooks))
	}

	if program.Hooks[0].Kind != strategyir.HookInit {
		t.Fatalf("program.Hooks[0].Kind = %q, want %q", program.Hooks[0].Kind, strategyir.HookInit)
	}
	if _, ok := program.Hooks[0].Statements[0].(*strategyir.LogStmt); !ok {
		t.Fatalf("hook 0 first statement = %T, want *strategyir.LogStmt", program.Hooks[0].Statements[0])
	}

	klineHook := program.Hooks[1]
	if klineHook.Kind != strategyir.HookKLineClose {
		t.Fatalf("program.Hooks[1].Kind = %q, want %q", klineHook.Kind, strategyir.HookKLineClose)
	}
	if len(klineHook.Statements) != 3 {
		t.Fatalf("len(klineHook.Statements) = %d, want 3", len(klineHook.Statements))
	}

	letStmt, ok := klineHook.Statements[0].(*strategyir.LetStmt)
	if !ok {
		t.Fatalf("kline statement 0 = %T, want *strategyir.LetStmt", klineHook.Statements[0])
	}
	if letStmt.Name != "fast" {
		t.Fatalf("letStmt.Name = %q, want %q", letStmt.Name, "fast")
	}

	ifStmt, ok := klineHook.Statements[1].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("kline statement 1 = %T, want *strategyir.IfStmt", klineHook.Statements[1])
	}
	if ifStmt.Condition != "cross_over(fast, slow)" {
		t.Fatalf("ifStmt.Condition = %q, want %q", ifStmt.Condition, "cross_over(fast, slow)")
	}
	if len(ifStmt.Then) != 1 || len(ifStmt.Else) != 1 {
		t.Fatalf("if branches = %d/%d, want 1/1", len(ifStmt.Then), len(ifStmt.Else))
	}

	orderStmt, ok := ifStmt.Then[0].(*strategyir.OrderStmt)
	if !ok {
		t.Fatalf("if then statement = %T, want *strategyir.OrderStmt", ifStmt.Then[0])
	}
	if orderStmt.Action != strategyir.OrderActionBuy {
		t.Fatalf("orderStmt.Action = %q, want %q", orderStmt.Action, strategyir.OrderActionBuy)
	}
	if orderStmt.QuantityMode != "shares" {
		t.Fatalf("orderStmt.QuantityMode = %q, want %q", orderStmt.QuantityMode, "shares")
	}
	if orderStmt.EntryPolicy != "flat_only" {
		t.Fatalf("orderStmt.EntryPolicy = %q, want %q", orderStmt.EntryPolicy, "flat_only")
	}
	if orderStmt.OrderType != "LIMIT" {
		t.Fatalf("orderStmt.OrderType = %q, want %q", orderStmt.OrderType, "LIMIT")
	}

	protectStmt, ok := ifStmt.Else[0].(*strategyir.ProtectStmt)
	if !ok {
		t.Fatalf("if else statement = %T, want *strategyir.ProtectStmt", ifStmt.Else[0])
	}
	if protectStmt.WindowPolicy != "session" {
		t.Fatalf("protectStmt.WindowPolicy = %q, want %q", protectStmt.WindowPolicy, "session")
	}

	if _, ok := klineHook.Statements[2].(*strategyir.NotifyStmt); !ok {
		t.Fatalf("kline statement 2 = %T, want *strategyir.NotifyStmt", klineHook.Statements[2])
	}
}

func TestParseScriptRejectsDanglingElse(t *testing.T) {
	_, err := ParseScript(`on kline_close:
  else:
    log "x"`)
	if err == nil {
		t.Fatal("ParseScript() error = nil, want dangling else error")
	}
}

func TestParseScriptRejectsInvalidExpression(t *testing.T) {
	_, err := ParseScript(`on kline_close:
  let signal = close +
  log "x"`)
	if err == nil {
		t.Fatal("ParseScript() error = nil, want invalid expression error")
	}
}
