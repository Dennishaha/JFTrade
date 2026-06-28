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

func (b *strategyRuntimeBrokerBridge) PlaceBrokerOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	trading := b.broker.Trading()
	if trading == nil {
		return nil, fmt.Errorf("broker trading not available")
	}
	return trading.PlaceOrder(ctx, query)
}
