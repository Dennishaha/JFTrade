package futu

import (
	"context"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

// orderBookRequest tracks a pending order book subscription.
type orderBookRequest struct {
	canonical string
	security  *qotcommonpb.Security
}

// QueryOrderBook fetches the current order book depth for a single security.
// The security must already be subscribed with SubType_OrderBook.
func (e *Exchange) QueryOrderBook(ctx context.Context, symbol string, num int32) (*opend.OrderBookResult, error) {
	security, canonical, err := futuSecurityFromSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if err := e.requireOrderBookSubscription(canonical); err != nil {
		return nil, err
	}

	var result *opend.OrderBookResult
	if err := e.withClient(ctx, func(client *opend.Client) error {
		res, err := client.GetOrderBook(ctx, opend.OrderBookRequest{
			Security: security,
			Num:      num,
		})
		if err != nil {
			return err
		}
		result = res
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *Exchange) requireOrderBookSubscription(canonical string) error {
	e.ConnectionGeneration()
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.subscriptions.hasOrderBook(canonical) {
		return &SubscriptionRequiredError{Channel: "ORDER_BOOK", Symbol: canonical}
	}
	return nil
}

// SubscribeOrderBook establishes the order-book subscription required by
// QueryOrderBook. When push is true it also registers order-book pushes on the
// current OpenD connection.
func (e *Exchange) SubscribeOrderBook(ctx context.Context, symbol string, push bool) error {
	security, canonical, err := futuSecurityFromSymbol(symbol)
	if err != nil {
		return err
	}
	request := orderBookRequest{canonical: canonical, security: security}
	return e.withClient(ctx, func(client *opend.Client) error {
		if push {
			return e.ensureOrderBookPushSubscriptions(ctx, client, []orderBookRequest{request})
		}
		return e.ensureOrderBookSubscriptions(ctx, client, []orderBookRequest{request})
	})
}

// UnsubscribeOrderBook unregisters pushes and releases the order-book
// subscription owned by this Exchange connection. Missing subscriptions are
// treated as already released.
func (e *Exchange) UnsubscribeOrderBook(ctx context.Context, symbol string) error {
	security, canonical, err := futuSecurityFromSymbol(symbol)
	if err != nil {
		return err
	}
	e.mu.Lock()
	exists := e.subscriptions.hasOrderBook(canonical) || e.subscriptions.hasOrderBookPush(canonical)
	e.mu.Unlock()
	if !exists {
		return nil
	}
	request := opend.QuoteSubRequest{
		Securities:  []*qotcommonpb.Security{security},
		SubTypes:    []qotcommonpb.SubType{qotcommonpb.SubType_SubType_OrderBook},
		IsSubscribe: false,
		IsRegPush:   new(false),
	}
	if security.GetMarket() == int32(qotcommonpb.QotMarket_QotMarket_HK_Security) {
		request.IsSubOrderBookDetail = new(true)
	}
	if err := e.withClient(ctx, func(client *opend.Client) error {
		return client.SubscribeQuotes(ctx, request)
	}); err != nil {
		return err
	}
	e.mu.Lock()
	e.subscriptions.unmarkOrderBook(canonical)
	e.subscriptions.unmarkOrderBookPush(canonical)
	e.mu.Unlock()
	return nil
}

// ensureOrderBookSubscriptions ensures the given securities are subscribed
// for order book without registering push delivery.
func (e *Exchange) ensureOrderBookSubscriptions(ctx context.Context, client *opend.Client, requests []orderBookRequest) error {
	e.mu.Lock()
	missing := make([]orderBookRequest, 0, len(requests))
	for _, req := range requests {
		if e.subscriptions.hasOrderBook(req.canonical) {
			continue
		}
		missing = append(missing, req)
	}
	e.mu.Unlock()
	if len(missing) == 0 {
		return nil
	}

	for _, batch := range groupOrderBookRequestsForPush(missing) {
		securityList := make([]*qotcommonpb.Security, 0, len(batch.requests))
		for _, req := range batch.requests {
			securityList = append(securityList, req.security)
		}
		subReq := opend.QuoteSubRequest{
			Securities:  securityList,
			SubTypes:    []qotcommonpb.SubType{qotcommonpb.SubType_SubType_OrderBook},
			IsSubscribe: true,
			IsFirstPush: new(false),
		}
		if batch.withDetail {
			subReq.IsSubOrderBookDetail = new(true)
		}
		if err := client.SubscribeQuotes(ctx, subReq); err != nil {
			return err
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	for _, req := range missing {
		e.subscriptions.markOrderBook(req.canonical)
	}
	return nil
}

// ensureOrderBookPushSubscriptions ensures push registration for the given
// order book subscriptions. Used by the stream layer.
func (e *Exchange) ensureOrderBookPushSubscriptions(ctx context.Context, client *opend.Client, requests []orderBookRequest) error {
	e.mu.Lock()
	missing := make([]orderBookRequest, 0, len(requests))
	for _, req := range requests {
		if e.subscriptions.hasOrderBookPush(req.canonical) {
			continue
		}
		missing = append(missing, req)
	}
	e.mu.Unlock()
	if len(missing) == 0 {
		return nil
	}

	for _, batch := range groupOrderBookRequestsForPush(missing) {
		securityList := make([]*qotcommonpb.Security, 0, len(batch.requests))
		for _, req := range batch.requests {
			securityList = append(securityList, req.security)
		}
		subReq := opend.QuoteSubRequest{
			Securities:  securityList,
			SubTypes:    []qotcommonpb.SubType{qotcommonpb.SubType_SubType_OrderBook},
			IsSubscribe: true,
			IsRegPush:   new(true),
		}
		if batch.withDetail {
			subReq.IsSubOrderBookDetail = new(true)
		}
		if err := client.SubscribeQuotes(ctx, subReq); err != nil {
			return err
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	for _, req := range missing {
		e.subscriptions.markOrderBook(req.canonical)
		e.subscriptions.markOrderBookPush(req.canonical)
	}
	return nil
}

type orderBookPushBatch struct {
	requests   []orderBookRequest
	withDetail bool
}

func groupOrderBookRequestsForPush(requests []orderBookRequest) []orderBookPushBatch {
	hkRequests := make([]orderBookRequest, 0, len(requests))
	otherRequests := make([]orderBookRequest, 0, len(requests))
	for _, req := range requests {
		if req.security != nil && req.security.GetMarket() == int32(qotcommonpb.QotMarket_QotMarket_HK_Security) {
			hkRequests = append(hkRequests, req)
			continue
		}
		otherRequests = append(otherRequests, req)
	}

	batches := make([]orderBookPushBatch, 0, 2)
	if len(hkRequests) > 0 {
		batches = append(batches, orderBookPushBatch{requests: hkRequests, withDetail: true})
	}
	if len(otherRequests) > 0 {
		batches = append(batches, orderBookPushBatch{requests: otherRequests})
	}
	return batches
}

// isHKMarket returns true if any of the securities belongs to HK market,
// where SF行情 detail subscription is available.
func isHKMarket(securities []*qotcommonpb.Security) bool {
	for _, s := range securities {
		if s.GetMarket() == int32(qotcommonpb.QotMarket_QotMarket_HK_Security) {
			return true
		}
	}
	return false
}
