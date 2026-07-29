package servercore

import (
	"context"
	"errors"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestExecutionNotificationHandlesUnrelatedAndPartialFillEvents(t *testing.T) {
	server := &Server{}
	server.notifyExecutionOrderLifecycle(trdsrv.ExecutionOrder{}, nil)
	server.notifyExecutionOrderLifecycle(trdsrv.ExecutionOrder{Status: "UNKNOWN"}, &trdsrv.ExecutionOrderEvent{})

	if _, ok := executionNotificationForStatus(trdsrv.ExecutionOrder{Status: trdsrv.OrderStatusSubmitted}, &trdsrv.ExecutionOrderEvent{EventType: "OTHER"}); ok {
		t.Fatal("unrelated submitted event produced notification")
	}
	partial, ok := executionNotificationForStatus(trdsrv.ExecutionOrder{Status: trdsrv.OrderStatusPartiallyFilled}, &trdsrv.ExecutionOrderEvent{})
	if !ok || partial.Level != "info" || partial.Category != "broker.order.fill" {
		t.Fatalf("partial fill notification = %#v, %v", partial, ok)
	}
	if note := baseExecutionNotification(trdsrv.ExecutionOrder{}, "category"); note.BrokerID != "unknown" {
		t.Fatalf("default notification broker = %#v", note)
	}
	if got := executionOrderNotificationMessage(trdsrv.ExecutionOrder{InternalOrderID: "exec-empty"}); got != "exec-empty" {
		t.Fatalf("empty order message = %q", got)
	}
	symbol, side, brokerID := "AAPL", "BUY", "broker-1"
	quantity, filled := 2.0, 1.0
	message := executionOrderNotificationMessage(trdsrv.ExecutionOrder{
		TradingEnvironment: "SIMULATE",
		Symbol:             &symbol,
		Side:               &side,
		RequestedQuantity:  &quantity,
		FilledQuantity:     &filled,
		BrokerOrderID:      &brokerID,
	})
	for _, part := range []string{"SIMULATE", "AAPL", "BUY", "qty", "filled", "brokerOrderId"} {
		if !strings.Contains(message, part) {
			t.Fatalf("full order message %q missing %q", message, part)
		}
	}
}

func TestNodeRuntimeDependencyMessagesHandleInvalidAndTruncatedOutput(t *testing.T) {
	restoreRuntimeDependencyProbe(t,
		func(path string) (string, error) { return path, nil },
		func(context.Context, string, ...string) ([]byte, error) { return []byte("v22.0.0"), nil },
	)
	//nolint:staticcheck // Exercise the helper's explicit nil-context fallback.
	if got := checkNodeRuntimeDependency(nil, jfsettings.PineWorkerSettings{}); got["status"] != runtimeDependencyStatusOK {
		t.Fatalf("nil-context dependency = %#v", got)
	}
	configuredMissing := nodeMissingMessage("/missing/node", []string{"ignored"}, errors.New("missing"))
	if !strings.Contains(configuredMissing, "Configured") {
		t.Fatalf("configured missing message = %q", configuredMissing)
	}
	defaultMissing := nodeMissingMessage("", nil, errors.New("missing"))
	if !strings.Contains(defaultMissing, "Tried: node") {
		t.Fatalf("default missing message = %q", defaultMissing)
	}
	if got := summarizeDependencyCommandError(errors.New("boom"), nil); got != "boom" {
		t.Fatalf("empty command output summary = %q", got)
	}
	longOutput := strings.Repeat("x", 600)
	if got := summarizeDependencyCommandError(errors.New("boom"), []byte(longOutput)); len(got) > 506 {
		t.Fatalf("long command output was not truncated: %d", len(got))
	}
	for _, raw := range []string{"", "1.2.3.4", "1..2"} {
		if _, err := parseDependencyNodeVersion(raw); err == nil {
			t.Fatalf("invalid node version %q accepted", raw)
		}
	}
}
