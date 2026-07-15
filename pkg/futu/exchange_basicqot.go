package futu

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

// --- BasicQot query methods (quote snapshots & tickers) ---

// basicQotRequest groups a canonical symbol with its parsed protobuf Security.
type basicQotRequest struct {
	canonical string
	security  *qotcommonpb.Security
}

func (e *Exchange) QueryTicker(ctx context.Context, symbol string) (*types.Ticker, error) {
	basicQot, err := e.queryBasicQot(ctx, symbol)
	if err != nil {
		return nil, err
	}
	return tickerFromBasicQot(basicQot), nil
}

func (e *Exchange) QueryTickers(ctx context.Context, symbol ...string) (map[string]types.Ticker, error) {
	quotes, err := e.queryBasicQotList(ctx, symbol)
	if err != nil {
		return nil, err
	}
	tickers := make(map[string]types.Ticker, len(quotes))
	for currentSymbol, basicQot := range quotes {
		ticker := tickerFromBasicQot(basicQot)
		if ticker != nil {
			tickers[currentSymbol] = *ticker
		}
	}
	return tickers, nil
}

// QueryQuoteSnapshot returns BasicQot fields, including US pre-market,
// after-hours, and overnight quote blocks when OpenD provides them.
func (e *Exchange) QueryQuoteSnapshot(ctx context.Context, symbol string) (*QuoteSnapshot, error) {
	basicQot, err := e.queryBasicQot(ctx, symbol)
	if err != nil {
		return nil, err
	}
	canonical, _ := futuSymbolFromSecurity(basicQot.GetSecurity())
	snapshot := quoteSnapshotFromBasicQot(basicQot, canonical)
	if snapshot != nil {
		e.RecordMarketSessionSample(canonical, snapshot.Session, snapshot.QuoteAt)
	}
	return snapshot, nil
}

func (e *Exchange) queryBasicQot(ctx context.Context, symbol string) (*qotcommonpb.BasicQot, error) {
	quotes, err := e.queryBasicQotList(ctx, []string{symbol})
	if err != nil {
		return nil, err
	}
	return basicQotForSymbol(quotes, symbol)
}

func basicQotForSymbol(quotes map[string]*qotcommonpb.BasicQot, symbol string) (*qotcommonpb.BasicQot, error) {
	canonical := strings.TrimSpace(strings.ToUpper(symbol))
	quote := quotes[canonical]
	if quote == nil {
		return nil, fmt.Errorf("opend GetBasicQot returned no quotes for %s", symbol)
	}
	return quote, nil
}

func basicQotRequestsFromSymbols(symbols []string) ([]basicQotRequest, error) {
	requests := make([]basicQotRequest, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		security, canonical, err := futuSecurityFromSymbol(symbol)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		requests = append(requests, basicQotRequest{canonical: canonical, security: security})
	}
	return requests, nil
}

func (e *Exchange) queryBasicQotList(ctx context.Context, symbols []string) (map[string]*qotcommonpb.BasicQot, error) {
	requests, err := basicQotRequestsFromSymbols(symbols)
	if err != nil {
		return nil, err
	}
	if len(requests) == 0 {
		return map[string]*qotcommonpb.BasicQot{}, nil
	}
	if err := e.requireBasicQotSubscriptions(requests); err != nil {
		return nil, err
	}

	reqStart := time.Now()
	securityList := make([]*qotcommonpb.Security, 0, len(requests))
	for _, request := range requests {
		securityList = append(securityList, request.security)
	}
	request := &qotgetbasicqotpb.Request{C2S: &qotgetbasicqotpb.C2S{SecurityList: securityList}}
	var response qotgetbasicqotpb.Response
	if err := e.withClient(ctx, func(client *opend.Client) error {
		if err := e.requireBasicQotSubscriptions(requests); err != nil {
			return err
		}
		callStart := time.Now()
		if err := client.Call(ctx, opend.ProtoGetBasicQot, request, &response); err != nil {
			return err
		}
		callElapsed := time.Since(callStart)

		log.Printf("futu GetBasicQot symbols=%d rpc=%v total=%v",
			len(requests), callElapsed, time.Since(reqStart))
		return nil
	}); err != nil {
		log.Printf("futu GetBasicQot symbols=%d failed after %v: %v",
			len(requests), time.Since(reqStart), err)
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend GetBasicQot retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}

	quotes := basicQotMapFromProto(response.GetS2C().GetBasicQotList())
	if len(quotes) == 0 {
		return nil, fmt.Errorf("opend GetBasicQot returned no quotes")
	}
	return quotes, nil
}

func (e *Exchange) requireBasicQotSubscriptions(requests []basicQotRequest) error {
	e.ConnectionGeneration()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, request := range requests {
		if !e.subscriptions.hasBasicQot(request.canonical) {
			return &SubscriptionRequiredError{Channel: "BASIC", Symbol: request.canonical}
		}
	}
	return nil
}

func basicQotMapFromProto(protoQuotes []*qotcommonpb.BasicQot) map[string]*qotcommonpb.BasicQot {
	quotes := make(map[string]*qotcommonpb.BasicQot, len(protoQuotes))
	for _, quote := range protoQuotes {
		canonical, err := futuSymbolFromSecurity(quote.GetSecurity())
		if err != nil {
			continue
		}
		quotes[canonical] = quote
	}
	return quotes
}

func (e *Exchange) ensureBasicQotSubscriptions(ctx context.Context, client *opend.Client, requests []basicQotRequest) error {
	e.mu.Lock()
	missing := make([]basicQotRequest, 0, len(requests))
	for _, request := range requests {
		if e.subscriptions.hasBasicQot(request.canonical) {
			continue
		}
		missing = append(missing, request)
	}
	e.mu.Unlock()
	if len(missing) == 0 {
		return nil
	}

	securityList := make([]*qotcommonpb.Security, 0, len(missing))
	for _, request := range missing {
		securityList = append(securityList, request.security)
	}
	if err := subscribeBasicQot(ctx, client, securityList); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	for _, request := range missing {
		e.subscriptions.markBasicQot(request.canonical)
	}
	return nil
}

// SubscribeBasicQuote establishes the Basic quote subscription used by live
// K-line aggregation. When push is true it also registers quote pushes on the
// current OpenD connection.
func (e *Exchange) SubscribeBasicQuote(ctx context.Context, symbol string, push bool) error {
	security, canonical, err := futuSecurityFromSymbol(symbol)
	if err != nil {
		return err
	}
	request := basicQotRequest{canonical: canonical, security: security}
	return e.withClient(ctx, func(client *opend.Client) error {
		if push {
			return e.ensureBasicQotPushSubscriptions(ctx, client, []basicQotRequest{request})
		}
		return e.ensureBasicQotSubscriptions(ctx, client, []basicQotRequest{request})
	})
}

// UnsubscribeBasicQuote unregisters pushes and releases the Basic quote
// subscription owned by this Exchange connection.
func (e *Exchange) UnsubscribeBasicQuote(ctx context.Context, symbol string) error {
	security, canonical, err := futuSecurityFromSymbol(symbol)
	if err != nil {
		return err
	}
	e.mu.Lock()
	exists := e.subscriptions.hasBasicQot(canonical) || e.subscriptions.hasBasicQotPush(canonical)
	e.mu.Unlock()
	if !exists {
		return nil
	}
	if err := e.withClient(ctx, func(client *opend.Client) error {
		return setBasicQotSubscription(ctx, client, []*qotcommonpb.Security{security}, false, new(false))
	}); err != nil {
		return err
	}
	e.mu.Lock()
	e.subscriptions.unmarkBasicQot(canonical)
	e.subscriptions.unmarkBasicQotPush(canonical)
	e.mu.Unlock()
	return nil
}

func subscribeBasicQot(ctx context.Context, client *opend.Client, securities []*qotcommonpb.Security) error {
	// Intentionally omit IsRegOrUnRegPush: per Qot_Sub.proto, "该参数不指定不做
	// 注册反注册操作" — leaving it unset preserves any push registration the
	// stream layer has already installed on this OpenD connection. Sending
	// `false` here would explicitly toggle push state and could silently
	// unregister Qot_UpdateBasicQot pushes for these securities.
	return setBasicQotSubscription(ctx, client, securities, true, nil)
}

func setBasicQotSubscription(ctx context.Context, client *opend.Client, securities []*qotcommonpb.Security, subscribe bool, registerPush *bool) error {
	request := &qotsubpb.Request{C2S: &qotsubpb.C2S{
		SecurityList:     securities,
		SubTypeList:      []int32{int32(qotcommonpb.SubType_SubType_Basic)},
		IsSubOrUnSub:     new(subscribe),
		IsRegOrUnRegPush: registerPush,
	}}
	var response qotsubpb.Response
	if err := client.Call(ctx, opend.ProtoQotSub, request, &response); err != nil {
		return err
	}
	if response.GetRetType() != 0 {
		return fmt.Errorf("opend Qot_Sub retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	return nil
}
