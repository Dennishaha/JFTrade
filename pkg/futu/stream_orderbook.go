package futu

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
)

// --- Order Book Push (Stream) ---

// connectOpenDOrderBook registers the push handler for order book depth
// and ensures push subscriptions for all subscribed securities.
func (s *Stream) connectOpenDOrderBook(ctx context.Context) error {
	client, err := s.exchange.ensureClient(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.callbackClient != client {
		client.SubscribeOrderBook(s.handleOrderBookPush)
	}
	streamCtx := s.ctx
	s.mu.Unlock()

	requests, err := orderBookRequestsFromSubscriptions(s.GetSubscriptions())
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return fmt.Errorf("no order book subscriptions")
	}
	if err := s.exchange.ensureOrderBookPushSubscriptions(ctx, client, requests); err != nil {
		return err
	}
	if streamCtx != nil {
		go s.watchClientLoop(streamCtx, client)
	}
	return nil
}

// handleOrderBookPush is the typed handler for Qot_UpdateOrderBook (3013) pushes.
func (s *Stream) handleOrderBookPush(s2c *qotupdateorderbookpb.S2C) {
	if !s.isActive() || s2c == nil {
		return
	}
	canonical, err := futuSymbolFromSecurity(s2c.GetSecurity())
	if err != nil {
		return
	}

	// Emit a single best bid/ask snapshot so downstream consumers never observe
	// a half-empty BookTicker caused by split bid/ask events.
	bookTicker := types.BookTicker{Symbol: canonical}
	if bids := s2c.GetOrderBookBidList(); len(bids) > 0 {
		bookTicker.Buy = fixedpoint.NewFromFloat(bids[0].GetPrice())
		bookTicker.BuySize = fixedpoint.NewFromFloat(float64(bids[0].GetVolume()))
	}
	if asks := s2c.GetOrderBookAskList(); len(asks) > 0 {
		bookTicker.Sell = fixedpoint.NewFromFloat(asks[0].GetPrice())
		bookTicker.SellSize = fixedpoint.NewFromFloat(float64(asks[0].GetVolume()))
	}
	if !bookTicker.Buy.IsZero() || !bookTicker.Sell.IsZero() {
		s.EmitBookTickerUpdate(bookTicker)
	}
}

// orderBookRequestsFromSubscriptions extracts order book subscription requests
// from the bbgo stream subscription list.
func orderBookRequestsFromSubscriptions(subscriptions []types.Subscription) ([]orderBookRequest, error) {
	requests := make([]orderBookRequest, 0, len(subscriptions))
	seen := map[string]struct{}{}
	for _, subscription := range subscriptions {
		if subscription.Channel != types.BookTickerChannel {
			continue
		}
		security, canonical, err := futuSecurityFromSymbol(subscription.Symbol)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		requests = append(requests, orderBookRequest{canonical: canonical, security: security})
	}
	return requests, nil
}
