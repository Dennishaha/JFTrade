package futu

import (
	"context"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

// --- broker.OrderBookSubscriber implementation ---

func (a *futuAdapter) SubscribeOrderBook(ctx context.Context, req broker.OrderBookSubscribeRequest) error {
	for _, symbol := range req.Symbols {
		if err := a.exchange.SubscribeOrderBook(ctx, symbol, true); err != nil {
			return err
		}
	}
	return nil
}
