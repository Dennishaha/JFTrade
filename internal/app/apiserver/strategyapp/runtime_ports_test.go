package strategyapp

import (
	"context"
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type strategyMarketDataReaderStub struct {
	broker.MarketDataReader
	funds     *broker.FundsSnapshot
	positions []broker.PositionSnapshot
}

func (s *strategyMarketDataReaderStub) QueryFunds(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
	return s.funds, nil
}

func (s *strategyMarketDataReaderStub) QueryPositions(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	return s.positions, nil
}

type strategyBrokerStub struct {
	broker.Broker
	trading broker.TradingService
	reader  broker.MarketDataReader
}

func (s *strategyBrokerStub) Trading() broker.TradingService      { return s.trading }
func (s *strategyBrokerStub) MarketData() broker.MarketDataReader { return s.reader }

type strategyProviderStub struct {
	marketdata.Provider
	descriptor marketdata.ProviderDescriptor
}

func (s *strategyProviderStub) Descriptor(context.Context) (marketdata.ProviderDescriptor, error) {
	return s.descriptor, nil
}

type strategyTradingServiceStub struct {
	placeOrder     trading.ExecutionOrder
	placeErr       error
	cancelResponse trading.ExecutionCommandResponse
	cancelErr      error
}

func (s strategyTradingServiceStub) PlaceExecutionOrder(context.Context, trading.ExecutionOrderCommand) (trading.ExecutionOrder, error) {
	return s.placeOrder, s.placeErr
}

func (s strategyTradingServiceStub) CancelExecutionOrder(context.Context, string) (trading.ExecutionCommandResponse, error) {
	return s.cancelResponse, s.cancelErr
}

func TestAccountResolverRequiresExactTradableBrokerAndDelegatesReads(t *testing.T) {
	resolverCalls := 0
	reader := &strategyMarketDataReaderStub{
		funds:     &broker.FundsSnapshot{AccountID: "account-1"},
		positions: []broker.PositionSnapshot{{Symbol: "US.AAPL"}},
	}
	tradable := &strategyBrokerStub{trading: &struct{ broker.TradingService }{}, reader: reader}
	resolve := AccountResolver(func(id string) broker.Broker {
		resolverCalls++
		if id == "broker-b" {
			return tradable
		}
		return nil
	})
	if resolve(strategy.InstanceBinding{}) != nil {
		t.Fatal("binding without broker account resolved")
	}
	if AccountResolver(nil)(strategy.InstanceBinding{BrokerAccount: &strategy.BrokerAccountBinding{BrokerID: "broker-b"}}) != nil {
		t.Fatal("nil broker resolver returned an account source")
	}
	if resolve(strategy.InstanceBinding{BrokerAccount: &strategy.BrokerAccountBinding{BrokerID: "broker-a"}}) != nil {
		t.Fatal("unbound broker resolved through fallback")
	}
	source := resolve(strategy.InstanceBinding{BrokerAccount: &strategy.BrokerAccountBinding{BrokerID: " broker-b "}})
	if source == nil {
		t.Fatal("bound broker did not resolve")
	}
	funds, err := source.QueryBrokerFunds(t.Context(), broker.ReadQuery{})
	if err != nil || funds.AccountID != "account-1" {
		t.Fatalf("QueryBrokerFunds = %+v, %v", funds, err)
	}
	positions, err := source.QueryBrokerPositions(t.Context(), broker.ReadQuery{})
	if err != nil || len(positions) != 1 || positions[0].Symbol != "US.AAPL" {
		t.Fatalf("QueryBrokerPositions = %+v, %v", positions, err)
	}
	if resolverCalls != 2 {
		t.Fatalf("resolver calls = %d, want 2", resolverCalls)
	}
	noTrading := &strategyBrokerStub{reader: reader}
	if AccountResolver(func(string) broker.Broker { return noTrading })(strategy.InstanceBinding{BrokerAccount: &strategy.BrokerAccountBinding{BrokerID: "x"}}) != nil {
		t.Fatal("non-tradable broker resolved")
	}
	noReader := &strategyBrokerStub{trading: &struct{ broker.TradingService }{}}
	if AccountResolver(func(string) broker.Broker { return noReader })(strategy.InstanceBinding{BrokerAccount: &strategy.BrokerAccountBinding{BrokerID: "x"}}) != nil {
		t.Fatal("broker without account reader resolved")
	}
}

func TestMarketDataCapabilitiesReadsRuntimeDescriptor(t *testing.T) {
	provider := &strategyProviderStub{descriptor: marketdata.ProviderDescriptor{
		SelectionID: "futu", Capabilities: marketdata.ProviderCapabilities{StreamingCandles: true},
	}}
	runtime, err := marketdataapp.NewRuntime(marketdataapp.RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	capabilities, err := MarketDataCapabilities(marketdata.NewService(runtime))(t.Context())
	if err != nil || !capabilities.StreamingCandles {
		t.Fatalf("capabilities = %+v, %v", capabilities, err)
	}
}

func TestTradeCommandsMapPlaceCancelAndDefensiveFailures(t *testing.T) {
	commands := TradeCommands(nil)
	if _, err := commands.PlaceExecutionOrder(t.Context(), trading.ExecutionOrderCommand{}); err == nil {
		t.Fatal("nil service place error = nil")
	}
	if _, err := commands.CancelExecutionOrder(t.Context(), "order-1"); err == nil {
		t.Fatal("nil service cancel error = nil")
	}

	internalID := "order-1"
	stub := strategyTradingServiceStub{
		placeOrder:     trading.ExecutionOrder{InternalOrderID: internalID},
		cancelResponse: trading.ExecutionCommandResponse{InternalOrderID: &internalID},
	}
	commands = tradeCommands(stub)
	placed, err := commands.PlaceExecutionOrder(t.Context(), trading.ExecutionOrderCommand{})
	if err != nil || placed.InternalOrderID != internalID {
		t.Fatalf("PlaceExecutionOrder = %+v, %v", placed, err)
	}
	canceled, err := commands.CancelExecutionOrder(t.Context(), internalID)
	if err != nil || canceled.InternalOrderID != internalID {
		t.Fatalf("CancelExecutionOrder = %+v, %v", canceled, err)
	}

	wantErr := errors.New("cancel failed")
	commands = tradeCommands(strategyTradingServiceStub{cancelErr: wantErr})
	if _, err := commands.CancelExecutionOrder(t.Context(), internalID); !errors.Is(err, wantErr) {
		t.Fatalf("cancel error = %v", err)
	}
	commands = tradeCommands(strategyTradingServiceStub{})
	if _, err := commands.CancelExecutionOrder(t.Context(), internalID); err == nil {
		t.Fatal("missing internal order id error = nil")
	}
}
