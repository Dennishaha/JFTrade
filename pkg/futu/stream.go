package futu

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	qotupdatebasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdatebasicqot"
)

// Stream translates Futu OpenD quote pushes into bbgo stream callbacks.
type Stream struct {
	types.StandardStream

	exchange       *Exchange
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	callbackClient *opend.Client
	closeOnce      sync.Once
}

// NewStream constructs a Stream tied to the given Exchange.
func NewStream(ex *Exchange) *Stream {
	s := &Stream{StandardStream: types.NewStandardStream(), exchange: ex}
	s.SetPublicOnly()
	return s
}

func (s *Stream) Connect(ctx context.Context) error {
	streamCtx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx = streamCtx
	s.cancel = cancel
	s.mu.Unlock()

	if err := s.connectOpenDBasicQot(ctx); err != nil {
		cancel()
		return err
	}

	go s.reconnectLoop(streamCtx)
	s.EmitStart()
	return nil
}

func (s *Stream) Close() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.ctx = nil
	s.callbackClient = nil
	s.mu.Unlock()
	s.closeOnce.Do(func() { close(s.CloseC) })
	s.EmitDisconnect()
	return nil
}

func (s *Stream) reconnectLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.CloseC:
			return
		case <-s.ReconnectC:
			_ = s.connectOpenDBasicQot(ctx)
		}
	}
}

// watchClientLoop monitors the bound OpenD client and triggers a stream
// reconnect when the underlying TCP session terminates (keepalive failure,
// peer close, or `Exchange.invalidateClient` on a recoverable RPC error).
// Without this watcher, `connectOpenDBasicQot` would never re-run after the
// cached client was replaced, so push subscriptions and the per-client
// callback handle would be lost — leaving the websocket on the fallback
// poller indefinitely.
func (s *Stream) watchClientLoop(ctx context.Context, client *opend.Client) {
	if client == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-s.CloseC:
		return
	case <-client.Done():
		select {
		case <-ctx.Done():
		case <-s.CloseC:
		case s.ReconnectC <- struct{}{}:
		default:
		}
	}
}

func (s *Stream) connectOpenDBasicQot(ctx context.Context) error {
	client, err := s.exchange.ensureClient(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.callbackClient != client {
		client.Subscribe(opend.ProtoQotUpdateBasicQot, s.handleBasicQotPush)
		s.callbackClient = client
	}
	streamCtx := s.ctx
	s.mu.Unlock()

	requests, err := basicQotRequestsFromSubscriptions(s.GetSubscriptions())
	if err != nil {
		return err
	}
	if err := s.exchange.ensureBasicQotPushSubscriptions(ctx, client, requests); err != nil {
		return err
	}
	if streamCtx != nil {
		go s.watchClientLoop(streamCtx, client)
	}
	s.EmitConnect()
	return nil
}

func basicQotRequestsFromSubscriptions(subscriptions []types.Subscription) ([]basicQotRequest, error) {
	requests := make([]basicQotRequest, 0, len(subscriptions))
	seen := map[string]struct{}{}
	for _, subscription := range subscriptions {
		if subscription.Channel != types.BookTickerChannel && subscription.Channel != types.MarketTradeChannel {
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
		requests = append(requests, basicQotRequest{canonical: canonical, security: security})
	}
	return requests, nil
}

func (s *Stream) handleBasicQotPush(frame codec.Frame) {
	if !s.isActive() {
		return
	}
	var response qotupdatebasicqotpb.Response
	if err := proto.Unmarshal(frame.Body, &response); err != nil || response.GetRetType() != 0 {
		return
	}
	for _, basicQot := range response.GetS2C().GetBasicQotList() {
		s.emitBasicQot(basicQot)
	}
}

func (s *Stream) isActive() bool {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	default:
		return true
	}
}

func (s *Stream) emitBasicQot(basicQot *qotcommonpb.BasicQot) {
	canonical, err := futuSymbolFromSecurity(basicQot.GetSecurity())
	if err != nil {
		return
	}
	ticker := tickerFromBasicQot(basicQot)
	if ticker == nil || ticker.Last.IsZero() {
		return
	}

	s.EmitBookTickerUpdate(types.BookTicker{
		Symbol: canonical,
		Buy:    ticker.Buy,
		Sell:   ticker.Sell,
	})

	tradeTime := ticker.Time
	if tradeTime.IsZero() {
		tradeTime = time.Now().UTC()
	}
	s.EmitMarketTrade(types.Trade{
		Exchange: Name,
		Symbol:   canonical,
		Price:    ticker.Last,
		Quantity: ticker.Volume,
		Time:     types.Time(tradeTime),
	})
}

func (e *Exchange) ensureBasicQotPushSubscriptions(ctx context.Context, client *opend.Client, requests []basicQotRequest) error {
	e.mu.Lock()
	missing := make([]basicQotRequest, 0, len(requests))
	for _, request := range requests {
		if e.subscriptions.hasBasicQotPush(request.canonical) {
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
	if err := subscribeBasicQotPush(ctx, client, securityList); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	for _, request := range missing {
		e.subscriptions.markBasicQotPush(request.canonical)
	}
	return nil
}

func subscribeBasicQotPush(ctx context.Context, client *opend.Client, securities []*qotcommonpb.Security) error {
	if len(securities) == 0 {
		return nil
	}
	request := &qotsubpb.Request{C2S: &qotsubpb.C2S{
		SecurityList:     securities,
		SubTypeList:      []int32{int32(qotcommonpb.SubType_SubType_Basic)},
		IsSubOrUnSub:     proto.Bool(true),
		IsRegOrUnRegPush: proto.Bool(true),
		IsFirstPush:      proto.Bool(true),
	}}
	var response qotsubpb.Response
	if err := client.Call(ctx, opend.ProtoQotSub, request, &response); err != nil {
		return err
	}
	if response.GetRetType() != 0 {
		return fmt.Errorf("opend Qot_Sub push retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	return nil
}
