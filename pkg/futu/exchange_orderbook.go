package futu

import (
	"context"

	"google.golang.org/protobuf/proto"

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

	var result *opend.OrderBookResult
	if err := e.withClient(ctx, func(client *opend.Client) error {
		if err := e.ensureOrderBookSubscriptions(ctx, client, []orderBookRequest{{canonical: canonical, security: security}}); err != nil {
			return err
		}
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

// ensureOrderBookSubscriptions ensures the given securities are subscribed
// for order book (non-push). Used before QueryOrderBook REST calls.
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

	securityList := make([]*qotcommonpb.Security, 0, len(missing))
	for _, req := range missing {
		securityList = append(securityList, req.security)
	}
	if err := client.SubscribeQuotes(ctx, opend.QuoteSubRequest{
		Securities:  securityList,
		SubTypes:    []qotcommonpb.SubType{qotcommonpb.SubType_SubType_OrderBook},
		IsSubscribe: true,
		IsFirstPush: proto.Bool(false),
	}); err != nil {
		return err
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
			IsRegPush:   proto.Bool(true),
		}
		if batch.withDetail {
			subReq.IsSubOrderBookDetail = proto.Bool(true)
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
