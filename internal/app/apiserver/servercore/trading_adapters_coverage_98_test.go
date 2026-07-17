package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type failingTradingAdapterExchange struct {
	*strategyRuntimeStubExchange
	placeErr  error
	cancelErr error
}

func (e *failingTradingAdapterExchange) PlaceBrokerOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	if e.placeErr != nil {
		return nil, e.placeErr
	}
	return e.strategyRuntimeStubExchange.PlaceBrokerOrder(ctx, query)
}

func (e *failingTradingAdapterExchange) CancelBrokerOrder(ctx context.Context, query broker.ReadQuery, order broker.CancelOrder) error {
	if e.cancelErr != nil {
		return e.cancelErr
	}
	return e.strategyRuntimeStubExchange.CancelBrokerOrder(ctx, query, order)
}

func newTradingAdapterCoverageServer(t *testing.T) *Server {
	t.Helper()
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	return newTestServer(t, store)
}

func recordCancelableExecutionOrder(server *Server, brokerOrderID string, symbol string, status string) trdsrv.ExecutionOrder {
	return server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      brokerOrderID,
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "US",
		Symbol:             symbol,
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             status,
		RequestedQuantity:  1,
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
}

func TestCoverage98TradingAdapterCancellationRejectsInvalidPersistedOrders(t *testing.T) {
	server := newTradingAdapterCoverageServer(t)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	if _, err := server.cancelExecutionOrder(t.Context(), "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing cancel error = %v", err)
	}

	terminal := recordCancelableExecutionOrder(server, "1001", "US.AAPL", "FILLED")
	if _, err := server.cancelExecutionOrder(t.Context(), terminal.InternalOrderID); err == nil || !strings.Contains(err.Error(), "already terminal") {
		t.Fatalf("terminal cancel error = %v", err)
	}

	missingBrokerID := recordCancelableExecutionOrder(server, "", "US.AAPL", "SUBMITTED")
	if _, err := server.cancelExecutionOrder(t.Context(), missingBrokerID.InternalOrderID); err == nil || !strings.Contains(err.Error(), "missing broker order id") {
		t.Fatalf("missing broker id error = %v", err)
	}

	invalidBrokerID := recordCancelableExecutionOrder(server, "not-a-number", "US.AAPL", "SUBMITTED")
	if _, err := server.cancelExecutionOrder(t.Context(), invalidBrokerID.InternalOrderID); err == nil || !strings.Contains(err.Error(), "invalid broker order id") {
		t.Fatalf("invalid broker id error = %v", err)
	}

	missingSymbol := recordCancelableExecutionOrder(server, "1002", "", "SUBMITTED")
	if _, err := server.cancelExecutionOrder(t.Context(), missingSymbol.InternalOrderID); err == nil || !strings.Contains(err.Error(), "missing symbol") {
		t.Fatalf("missing symbol error = %v", err)
	}

	valid := recordCancelableExecutionOrder(server, "1003", "US.AAPL", "SUBMITTED")
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return nil }
	if _, err := server.cancelExecutionOrder(t.Context(), valid.InternalOrderID); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unavailable exchange error = %v", err)
	}
}

func TestCoverage98TradingAdapterPropagatesBrokerFailuresAndPersistsAcceptedCancel(t *testing.T) {
	server := newTradingAdapterCoverageServer(t)
	backendFailure := errors.New("broker transport unavailable")
	failing := &failingTradingAdapterExchange{strategyRuntimeStubExchange: newStrategyRuntimeStubExchange(), placeErr: backendFailure}
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return failing }

	if _, err := server.placeExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{
		Symbol: "US.AAPL",
		Query:  broker.PlaceOrderQuery{ReadQuery: broker.ReadQuery{BrokerID: "futu", Market: "US"}, Symbol: "US.AAPL", Quantity: 1},
	}); !errors.Is(err, backendFailure) {
		t.Fatalf("place broker failure = %v, want %v", err, backendFailure)
	}

	failing.placeErr = nil
	accepted := recordCancelableExecutionOrder(server, "2001", "US.AAPL", "SUBMITTED")
	if _, err := server.cancelExecutionOrder(t.Context(), accepted.InternalOrderID); err != nil {
		t.Fatalf("accepted cancel error = %v", err)
	}
	updated, ok := server.executionOrders.order(accepted.InternalOrderID)
	if !ok || updated.Status != trdsrv.OrderStatusCancelRequested {
		t.Fatalf("accepted cancel persisted state = %#v / found=%v", updated, ok)
	}

	failing.cancelErr = backendFailure
	next := recordCancelableExecutionOrder(server, "2002", "US.AAPL", "SUBMITTED")
	before, ok := server.executionOrders.order(next.InternalOrderID)
	if !ok {
		t.Fatal("expected pending order before failed cancellation")
	}
	if _, err := server.cancelExecutionOrder(t.Context(), next.InternalOrderID); !errors.Is(err, backendFailure) {
		t.Fatalf("cancel broker failure = %v, want %v", err, backendFailure)
	}
	if unchanged, ok := server.executionOrders.order(next.InternalOrderID); !ok || unchanged.Status != before.Status {
		t.Fatalf("failed cancel mutated persisted order = %#v / found=%v", unchanged, ok)
	}

	var nilServer *Server
	if got := nilServer.defaultTradingEnvironment(); got != "SIMULATE" {
		t.Fatalf("nil server default environment = %q", got)
	}
}
