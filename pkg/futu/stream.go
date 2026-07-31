package futu

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/market"
	"github.com/shopspring/decimal"
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
	connectMu      sync.Mutex
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	callbackClient *opend.Client
	generation     uint64
	closed         bool
	closeOnce      sync.Once
	workerWG       sync.WaitGroup
	tradeVolumes   map[string]streamTradeVolume
}

type streamTradeVolume struct {
	tradingDay string
	session    market.Session
	cumulative decimal.Decimal
}

// NewStream constructs a Stream tied to the given Exchange.
func NewStream(ex *Exchange) *Stream {
	s := &Stream{StandardStream: types.NewStandardStream(), exchange: ex}
	s.SetPublicOnly()
	return s
}

func (s *Stream) Connect(ctx context.Context) error {
	s.connectMu.Lock()
	defer s.connectMu.Unlock()

	streamCtx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return opend.ErrClosed
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	generation := s.generation
	s.ctx = streamCtx
	s.cancel = cancel
	s.mu.Unlock()

	if err := s.connectOpenDBasicQot(ctx); err != nil {
		cancel()
		return err
	}

	if err := s.connectOpenDOrderBook(ctx); err != nil {
		// OrderBook push is optional; log and continue.
		// It may fail if no OrderBook subscriptions exist, which is normal.
		log.Printf("futu stream: order book push connection skipped: %v (continuing)", err)
	}

	s.startWorker(generation, func() { s.reconnectLoop(streamCtx) })
	s.EmitStart()
	return nil
}

func (s *Stream) Close() error {
	s.connectMu.Lock()
	emitDisconnect := false
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.generation++
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		s.ctx = nil
		s.callbackClient = nil
		s.mu.Unlock()
		close(s.CloseC)
		s.workerWG.Wait()
		emitDisconnect = true
	})
	s.connectMu.Unlock()
	if emitDisconnect {
		s.EmitDisconnect()
	}
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
			jftradeErr1 := s.connectOpenDBasicQot(ctx)
			besteffort.LogError(jftradeErr1)
			jftradeErr2 := s.connectOpenDOrderBook(ctx)
			besteffort.LogError(jftradeErr2)
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
	generation := s.generation
	s.mu.Unlock()

	requests, err := basicQotRequestsFromSubscriptions(s.GetSubscriptions())
	if err != nil {
		return err
	}
	if err := s.exchange.ensureBasicQotPushSubscriptions(ctx, client, requests); err != nil {
		return err
	}
	if streamCtx != nil {
		s.startWorker(generation, func() { s.watchClientLoop(streamCtx, client) })
	}
	s.EmitConnect()
	return nil
}

func (s *Stream) startWorker(generation uint64, worker func()) bool {
	if worker == nil {
		return false
	}
	s.mu.Lock()
	if s.closed || s.generation != generation {
		s.mu.Unlock()
		return false
	}
	s.workerWG.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.workerWG.Done()
		worker()
	}()
	return true
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
	snapshot := quoteSnapshotFromBasicQot(basicQot, canonical)
	s.emitBasicQotSnapshot(basicQot, canonical, snapshot)
}

func (s *Stream) emitBasicQotSnapshot(basicQot *qotcommonpb.BasicQot, canonical string, snapshot *QuoteSnapshot) {
	s.exchange.RecordMarketSessionSample(canonical, snapshot.Session, snapshot.QuoteAt)
	ticker := tickerFromBasicQot(basicQot)
	if ticker == nil || ticker.Last.IsZero() {
		return
	}

	s.EmitBookTickerUpdate(types.BookTicker{
		Symbol: canonical,
		Buy:    ticker.Buy,
		Sell:   ticker.Sell,
	})
	if snapshot.Volume.IsNegative() {
		return
	}

	tradeTime := ticker.Time
	quantity := s.nextTradeQuantity(canonical, snapshot.Session, tradeTime, snapshot.Volume)
	cumulativeVolume := snapshot.Volume
	volumeDelta := quantity
	s.EmitMarketTrade(types.Trade{
		Exchange:         Name,
		Symbol:           canonical,
		Price:            ticker.Last,
		Quantity:         legacyFixedpointVolume(quantity),
		VolumeDelta:      &volumeDelta,
		CumulativeVolume: &cumulativeVolume,
		Time:             types.Time(tradeTime),
	})
}

func (s *Stream) nextTradeQuantity(symbol string, session market.Session, at time.Time, cumulative decimal.Decimal) decimal.Decimal {
	if cumulative.IsNegative() {
		return decimal.Zero
	}
	tradingDay := at.UTC().Format("2006-01-02")
	if profile, ok := market.ProfileForSymbol(symbol); ok && profile.Location != nil {
		tradingDay = at.In(profile.Location).Format("2006-01-02")
	}
	if key, ok := market.TradingDayKey(symbol, at, true); ok {
		tradingDay = key
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tradeVolumes == nil {
		s.tradeVolumes = make(map[string]streamTradeVolume)
	}
	previous, exists := s.tradeVolumes[symbol]
	s.tradeVolumes[symbol] = streamTradeVolume{tradingDay: tradingDay, session: session, cumulative: cumulative}
	if !exists || previous.tradingDay != tradingDay || previous.session != session {
		return decimal.Zero
	}
	delta := cumulative.Sub(previous.cumulative)
	if delta.IsNegative() {
		return decimal.Zero
	}
	return delta
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
		e.subscriptions.markBasicQot(request.canonical)
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
		IsSubOrUnSub:     new(true),
		IsRegOrUnRegPush: new(true),
		IsFirstPush:      new(true),
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

// --- Order Book push moved to stream_orderbook.go ---
