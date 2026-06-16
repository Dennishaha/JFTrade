package futu

import (
	"context"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

// --- broker.OrderBookSubscriber implementation ---

func (a *futuAdapter) SubscribeOrderBook(ctx context.Context, req broker.OrderBookSubscribeRequest) error {
	return a.exchange.withClient(ctx, func(client *opend.Client) error {
		securities, err := securitiesFromSymbols(req.Symbols)
		if err != nil {
			return err
		}
		return client.SubscribeQuotes(ctx, opend.QuoteSubRequest{
			Securities:  securities,
			SubTypes:    []qotcommonpb.SubType{qotcommonpb.SubType_SubType_OrderBook},
			IsSubscribe: true,
			IsRegPush:   new(true),
		})
	})
}
