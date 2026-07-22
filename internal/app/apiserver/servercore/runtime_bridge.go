package servercore

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
)

// strategyRuntimeBrokerBridge adapts a futu.Exchange with the broker.Broker abstraction.
type strategyRuntimeBrokerBridge struct {
	*futu.Exchange
	broker broker.Broker
}

func (b *strategyRuntimeBrokerBridge) QueryBrokerFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
	reader := b.broker.MarketData()
	if reader == nil {
		return nil, fmt.Errorf("broker market data not available")
	}
	return reader.QueryFunds(ctx, query)
}

func (b *strategyRuntimeBrokerBridge) QueryBrokerPositions(ctx context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	reader := b.broker.MarketData()
	if reader == nil {
		return nil, fmt.Errorf("broker market data not available")
	}
	return reader.QueryPositions(ctx, query)
}

func (b *strategyRuntimeBrokerBridge) PlaceBrokerOrder(_ context.Context, _ broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	// Strategy placements are intentionally routed through
	// trading.Service.PlaceExecutionOrder. Keeping this compatibility method
	// fail-closed prevents a future runtime caller from bypassing pre-trade risk.
	return nil, fmt.Errorf("direct strategy runtime order placement is disabled; use the trading service pre-trade boundary")
}

func (b *strategyRuntimeBrokerBridge) CancelBrokerOrder(ctx context.Context, query broker.ReadQuery, order broker.CancelOrder) error {
	trading := b.broker.Trading()
	if trading == nil {
		return fmt.Errorf("broker trading not available")
	}
	return trading.CancelOrders(ctx, query, order)
}
