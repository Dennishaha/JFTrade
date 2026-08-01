package futu

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/live"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/trading"
	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	"github.com/jftrade/jftrade-main/pkg/market"
)

const BrokerID = "futu"

// RuntimeExchange is the broker-neutral execution surface needed by strategy
// sessions. Futu-only protocol and push methods remain behind MarketDataRuntime.
type RuntimeExchange interface {
	bbgotypes.Exchange
	EnsureMarket(string)
}

type MarketDataConfig struct {
	Enabled      bool
	Host         string
	APIPort      int
	WebSocketKey string
}

func (c MarketDataConfig) key() string {
	if !c.Enabled {
		return ""
	}
	return net.JoinHostPort(strings.TrimSpace(c.Host), strconv.Itoa(c.APIPort)) + "|" + c.WebSocketKey
}

type MarketDataRuntimeOptions struct {
	ConfigSource         func() MarketDataConfig
	NewExchange          func(MarketDataConfig) *pkgfutu.Exchange
	CloseExchange        func(*pkgfutu.Exchange) error
	OnExchange           func(*pkgfutu.Exchange)
	OnBroker             func(broker.Broker)
	OnSystemNotification func(live.Notification)
	Now                  func() time.Time
}

// MarketDataRuntime owns the broker-specific exchange lifecycle and protocol
// conversion. Freshness, demand, cache, polling, and backoff stay in marketdata.
type MarketDataRuntime struct {
	configSource  func() MarketDataConfig
	newExchange   func(MarketDataConfig) *pkgfutu.Exchange
	closeExchange func(*pkgfutu.Exchange) error
	onExchange    func(*pkgfutu.Exchange)
	onBroker      func(broker.Broker)
	onSystemNote  func(live.Notification)
	now           func() time.Time

	mu                     sync.Mutex
	exchange               *pkgfutu.Exchange
	brokerAdapter          broker.Broker
	key                    string
	generation             uint64
	closed                 bool
	creating               bool
	createDone             chan struct{}
	wg                     sync.WaitGroup
	subscriptionReconciler *marketDataSubscriptionReconciler
	inflightCloseErr       error
	closeOnce              sync.Once
	closeErr               error
}

func NewMarketDataRuntime(options MarketDataRuntimeOptions) *MarketDataRuntime {
	r := &MarketDataRuntime{
		configSource:  options.ConfigSource,
		newExchange:   options.NewExchange,
		closeExchange: options.CloseExchange,
		onExchange:    options.OnExchange,
		onBroker:      options.OnBroker,
		onSystemNote:  options.OnSystemNotification,
		now:           options.Now,
	}
	if r.newExchange == nil {
		r.newExchange = func(config MarketDataConfig) *pkgfutu.Exchange {
			return pkgfutu.NewExchangeWithConfig(opend.Config{
				Addr:             net.JoinHostPort(config.Host, strconv.Itoa(config.APIPort)),
				WebSocketKey:     config.WebSocketKey,
				HandshakeTimeout: 3 * time.Second,
				RequestTimeout:   8 * time.Second,
			})
		}
	}
	if r.closeExchange == nil {
		r.closeExchange = func(exchange *pkgfutu.Exchange) error {
			return exchange.Close()
		}
	}
	if r.now == nil {
		r.now = time.Now
	}
	r.subscriptionReconciler = newMarketDataSubscriptionReconciler(func() physicalSubscriptionExchange {
		exchange := r.Ensure()
		if exchange == nil {
			return nil
		}
		return exchange
	}, r.now)
	return r
}

func (r *MarketDataRuntime) Ensure() *pkgfutu.Exchange {
	if r == nil || r.configSource == nil {
		return nil
	}
	config := r.configSource()
	key := config.key()
	if key == "" {
		r.Reset()
		return nil
	}

	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil
		}
		if r.exchange != nil && r.key == key {
			exchange := r.exchange
			r.mu.Unlock()
			return exchange
		}
		if r.creating {
			done := r.createDone
			r.mu.Unlock()
			<-done
			continue
		}
		r.creating = true
		r.createDone = make(chan struct{})
		done := r.createDone
		generation := r.generation
		r.wg.Add(1)
		r.mu.Unlock()

		candidate := r.newExchange(config)
		return r.finishExchangeCreation(candidate, key, generation, done)
	}
}

func (r *MarketDataRuntime) finishExchangeCreation(
	candidate *pkgfutu.Exchange,
	key string,
	generation uint64,
	done chan struct{},
) *pkgfutu.Exchange {
	r.mu.Lock()
	closed := r.closed
	valid := !closed && r.generation == generation && r.configSource().key() == key
	var previous *pkgfutu.Exchange
	if valid {
		previous = r.exchange
		if candidate != nil && r.onSystemNote != nil {
			candidate.OnSystemNotify(func(response *notifypb.Response) {
				if note := LiveNotificationFromResponse(response); note != nil {
					r.onSystemNote(*note)
				}
			})
		}
		if candidate != nil && r.onExchange != nil {
			r.onExchange(candidate)
		}
		var activeBroker broker.Broker
		if candidate != nil {
			activeBroker = pkgfutu.NewBrokerAdapter(candidate)
		}
		r.exchange = candidate
		r.brokerAdapter = activeBroker
		r.key = key
		if activeBroker != nil && r.onBroker != nil {
			r.onBroker(activeBroker)
		}
	}
	r.creating = false
	close(done)
	r.mu.Unlock()

	if !valid {
		r.closeDiscardedExchange(candidate, closed)
		r.wg.Done()
		return nil
	}
	if previous != nil && previous != candidate {
		besteffort.LogError(r.closeExchange(previous))
	}
	r.wg.Done()
	return candidate
}

func (r *MarketDataRuntime) closeDiscardedExchange(candidate *pkgfutu.Exchange, closing bool) {
	if candidate == nil {
		return
	}
	closeErr := r.closeExchange(candidate)
	if closeErr == nil {
		return
	}
	if !closing {
		besteffort.LogError(closeErr)
		return
	}
	r.mu.Lock()
	r.inflightCloseErr = errors.Join(
		r.inflightCloseErr,
		fmt.Errorf("in-flight Futu exchange close: %w", closeErr),
	)
	r.mu.Unlock()
}

func (r *MarketDataRuntime) Exchange() *pkgfutu.Exchange {
	return r.Ensure()
}

// BBGOExchange exposes only the stable exchange contract needed by strategy
// execution; callers never receive the concrete Futu implementation type.
func (r *MarketDataRuntime) BBGOExchange() RuntimeExchange {
	exchange := r.Ensure()
	if exchange == nil {
		return nil
	}
	return exchange
}

// Broker returns the broker-neutral adapter for the active exchange.
func (r *MarketDataRuntime) Broker() broker.Broker {
	exchange := r.Ensure()
	if exchange == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.exchange != exchange {
		return nil
	}
	return r.brokerAdapter
}

// OwnsBroker reports whether the adapter belongs to the current exchange
// generation.
func (r *MarketDataRuntime) OwnsBroker(candidate broker.Broker) bool {
	if r == nil || candidate == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.brokerAdapter != nil && r.brokerAdapter == candidate
}

// QueryOrderBook keeps the Futu subscription sentinel behind the integration
// boundary so marketdata callers only observe the domain-level lease error.
func (r *MarketDataRuntime) QueryOrderBook(
	ctx context.Context,
	query broker.OrderBookQuery,
) (*broker.OrderBookSnapshot, error) {
	active := r.Broker()
	if active == nil {
		return nil, fmt.Errorf("futu marketdata runtime unavailable")
	}
	reader := active.MarketData()
	if reader == nil {
		return nil, fmt.Errorf("broker market data not available")
	}
	snapshot, err := reader.QueryOrderBook(ctx, query)
	if err != nil {
		return nil, translateSubscriptionRequiredError(err, "ORDER_BOOK", "")
	}
	return snapshot, nil
}

// QuerySecurityDetails returns the stable JSON-ready security representation.
func (r *MarketDataRuntime) QuerySecurityDetails(ctx context.Context, instrumentID string) (map[string]any, error) {
	exchange := r.Ensure()
	if exchange == nil {
		return nil, fmt.Errorf("futu marketdata runtime unavailable")
	}
	details, err := exchange.QuerySecurityDetails(ctx, instrumentID)
	if err != nil {
		return nil, err
	}
	return SecurityDetailsMap(details), nil
}

// QueryKLines keeps Futu protocol/session conversion inside the integration.
func (r *MarketDataRuntime) QueryKLines(
	ctx context.Context,
	instrumentID string,
	interval bbgotypes.Interval,
	options bbgotypes.KLineQueryOptions,
) ([]bbgotypes.KLine, error) {
	exchange := r.Ensure()
	if exchange == nil {
		return nil, fmt.Errorf("futu marketdata runtime unavailable")
	}
	klines, err := exchange.QueryKLines(ctx, instrumentID, interval, options)
	if err != nil {
		return nil, translateSubscriptionRequiredError(err, "KLINE", string(interval))
	}
	return klines, nil
}

func (r *MarketDataRuntime) ResolveKLineSession(kline bbgotypes.KLine) (market.Session, bool) {
	exchange := r.Ensure()
	if exchange == nil {
		return market.SessionUnknown, false
	}
	return exchange.ResolveKLineSession(kline)
}

// OnOrderBookUpdate registers a neutral symbol callback for depth pushes.
func (r *MarketDataRuntime) OnOrderBookUpdate(handler func(string)) func() {
	exchange := r.Ensure()
	if exchange == nil {
		return func() {}
	}
	return exchange.OnOrderBookUpdate(handler)
}

// EnsureSystemNotifications connects the integration-owned OpenD session and
// activates the notification converter installed during exchange creation.
func (r *MarketDataRuntime) EnsureSystemNotifications(ctx context.Context) error {
	exchange := r.Ensure()
	if exchange == nil {
		return fmt.Errorf("futu marketdata runtime unavailable")
	}
	return exchange.EnsureSystemNotifications(ctx)
}

// SubscribeOrderUpdates hides Futu protobuf callbacks behind trading's port.
func (r *MarketDataRuntime) SubscribeOrderUpdates(
	ctx context.Context,
	accounts []trading.Account,
	handler trading.OrderUpdateHandler,
) (trading.OrderUpdateSubscription, error) {
	exchange := r.Ensure()
	if exchange == nil {
		return noOpOrderUpdateSubscription{}, nil
	}
	return NewOrderUpdatesAdapter(exchange).Subscribe(ctx, accounts, handler)
}

func (r *MarketDataRuntime) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.generation++
	exchange := r.exchange
	r.exchange = nil
	r.brokerAdapter = nil
	r.key = ""
	r.mu.Unlock()
	if exchange != nil {
		jftradeErr3 := r.closeExchange(exchange)
		besteffort.LogError(jftradeErr3)
	}
	if r.subscriptionReconciler != nil {
		r.subscriptionReconciler.ResetPhysicalSubscriptions()
	}
}

func (r *MarketDataRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.generation++
		exchange := r.exchange
		r.exchange = nil
		r.brokerAdapter = nil
		r.key = ""
		r.mu.Unlock()

		var closeErr error
		if exchange != nil {
			if err := r.closeExchange(exchange); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("active Futu exchange close: %w", err))
			}
		}
		r.wg.Wait()
		r.mu.Lock()
		closeErr = errors.Join(closeErr, r.inflightCloseErr)
		r.mu.Unlock()
		if r.subscriptionReconciler != nil {
			r.subscriptionReconciler.ResetPhysicalSubscriptions()
		}
		r.closeErr = closeErr
	})
	return r.closeErr
}

func (r *MarketDataRuntime) ReconcileSubscriptions(ctx context.Context, desired []marketdata.InstrumentRef) error {
	if r == nil || r.subscriptionReconciler == nil {
		return nil
	}
	return r.subscriptionReconciler.ReconcileSubscriptions(ctx, desired)
}

func (r *MarketDataRuntime) SubscriptionState() map[string]any {
	if r == nil || r.subscriptionReconciler == nil {
		return nil
	}
	return r.subscriptionReconciler.SubscriptionState()
}

func (r *MarketDataRuntime) QueryTickers(ctx context.Context, instrumentIDs []string) (map[string]marketdata.Tick, error) {
	exchange := r.Ensure()
	if exchange == nil {
		return nil, fmt.Errorf("futu marketdata runtime unavailable")
	}
	snapshots, err := exchange.QueryQuoteSnapshots(ctx, instrumentIDs...)
	if err != nil {
		return nil, translateSubscriptionRequiredError(err, "TICK", "")
	}
	result := make(map[string]marketdata.Tick, len(snapshots))
	for instrumentID, snapshot := range snapshots {
		if tick := tickFromSnapshot(instrumentID, &snapshot, r.now().UTC()); tick != nil {
			result[instrumentID] = *tick
		}
	}
	return result, nil
}

func (r *MarketDataRuntime) QueryTicker(ctx context.Context, instrumentID string) (*marketdata.Tick, error) {
	ticks, err := r.QueryTickers(ctx, []string{instrumentID})
	if err != nil {
		return nil, err
	}
	tick := ticks[instrumentID]
	if tick.InstrumentID == "" {
		return nil, fmt.Errorf("futu returned no ticker for %s", instrumentID)
	}
	return &tick, nil
}

func (r *MarketDataRuntime) QuerySnapshot(ctx context.Context, instrumentID string) (*marketdata.Tick, error) {
	exchange := r.Ensure()
	if exchange == nil {
		return nil, fmt.Errorf("futu marketdata runtime unavailable")
	}
	snapshot, err := exchange.QueryQuoteSnapshot(ctx, instrumentID)
	if err != nil {
		return nil, translateSubscriptionRequiredError(err, "SNAPSHOT", "")
	}
	return tickFromSnapshot(instrumentID, snapshot, r.now().UTC()), nil
}

func translateSubscriptionRequiredError(err error, channel, interval string) error {
	if !errors.Is(err, pkgfutu.ErrSubscriptionRequired) {
		return err
	}
	var required *pkgfutu.SubscriptionRequiredError
	if errors.As(err, &required) {
		if required.Symbol != "" {
			_, market, symbol, ok := marketdata.NormalizeInstrumentID(required.Symbol)
			if ok {
				return marketdata.NewSubscriptionRequiredError(channel, market, symbol, interval)
			}
		}
		if interval == "" {
			interval = required.Interval
		}
	}
	return marketdata.NewSubscriptionRequiredError(channel, "", "", interval)
}

func (r *MarketDataRuntime) NewStream(instrumentIDs []string, handler marketdata.PushTickHandler) (marketdata.PushStream, error) {
	exchange := r.Ensure()
	if exchange == nil {
		return nil, fmt.Errorf("futu marketdata runtime unavailable")
	}
	stream := exchange.NewStream()
	stream.SetPublicOnly()
	for _, instrumentID := range instrumentIDs {
		stream.Subscribe(bbgotypes.MarketTradeChannel, instrumentID, bbgotypes.SubscribeOptions{})
	}
	stream.OnMarketTrade(func(trade bbgotypes.Trade) {
		if handler == nil {
			return
		}
		if tick := tickFromTrade(trade, r.now().UTC()); tick != nil {
			handler(*tick)
		}
	})
	return stream, nil
}

func tickFromTicker(instrumentID string, ticker *bbgotypes.Ticker, observedAt time.Time) *marketdata.Tick {
	if ticker == nil {
		return nil
	}
	priceFixed := ticker.Last
	if priceFixed.IsZero() {
		priceFixed = ticker.GetValidPrice()
	}
	if priceFixed.IsZero() {
		return nil
	}
	instrumentID, resolvedMarket, symbol, ok := marketdata.NormalizeInstrumentID(instrumentID)
	if !ok {
		return nil
	}
	price := decimal.RequireFromString(priceFixed.String())
	bid, ask := price, price
	if !ticker.Buy.IsZero() {
		bid = decimal.RequireFromString(ticker.Buy.String())
	}
	if !ticker.Sell.IsZero() {
		ask = decimal.RequireFromString(ticker.Sell.String())
	}
	quoteAt := observedAt
	if !ticker.Time.IsZero() {
		quoteAt = ticker.Time.UTC()
	}
	session := market.ClassifySession(instrumentID, observedAt)
	volume := decimalFromFixedpoint(ticker.Volume)
	if volume.IsNegative() {
		volume = decimal.Zero
	}
	return &marketdata.Tick{
		InstrumentID: instrumentID, Market: resolvedMarket, Symbol: symbol,
		Price: price, Bid: bid, Ask: ask,
		OpenPrice: optionalDecimal(ticker.Open), HighPrice: optionalDecimal(ticker.High), LowPrice: optionalDecimal(ticker.Low),
		Volume: volume, QuoteAt: quoteAt.UTC().Format(time.RFC3339Nano),
		ObservedAt: observedAt.UTC().Format(time.RFC3339Nano), Source: "bbgo:futu",
		Session: string(session), ExtendedHours: market.IsExtendedSession(session), Kind: marketdata.TickKindQuote,
	}
}

func tickFromTrade(trade bbgotypes.Trade, observedAt time.Time) *marketdata.Tick {
	instrumentID, resolvedMarket, symbol, ok := marketdata.NormalizeInstrumentID(trade.Symbol)
	if !ok || trade.Price.IsZero() {
		return nil
	}
	price := decimal.RequireFromString(trade.Price.String())
	quoteAt := observedAt
	if !trade.Time.Time().IsZero() {
		quoteAt = trade.Time.Time().UTC()
	}
	cumulativeVolume := decimal.Zero
	if trade.CumulativeVolume != nil {
		cumulativeVolume = *trade.CumulativeVolume
		if cumulativeVolume.IsNegative() {
			cumulativeVolume = decimal.Zero
		}
	}
	volumeDelta := decimalFromFixedpoint(trade.Quantity)
	if trade.VolumeDelta != nil {
		volumeDelta = *trade.VolumeDelta
	}
	if volumeDelta.IsNegative() {
		volumeDelta = decimal.Zero
	}
	session := market.ClassifySession(instrumentID, observedAt)
	return &marketdata.Tick{
		InstrumentID: instrumentID, Market: resolvedMarket, Symbol: symbol,
		Price: price, Bid: price, Ask: price,
		Volume: cumulativeVolume, VolumeDelta: volumeDelta,
		QuoteAt: quoteAt.UTC().Format(time.RFC3339Nano), ObservedAt: observedAt.UTC().Format(time.RFC3339Nano),
		Source: "bbgo:futu:stream", Session: string(session),
		ExtendedHours: market.IsExtendedSession(session), Kind: marketdata.TickKindTrade,
	}
}

func tickFromSnapshot(instrumentID string, snapshot *pkgfutu.QuoteSnapshot, observedAt time.Time) *marketdata.Tick {
	if snapshot == nil || snapshot.Price.IsZero() {
		return nil
	}
	instrumentID, resolvedMarket, symbol, ok := marketdata.NormalizeInstrumentID(instrumentID)
	if !ok {
		return nil
	}
	preMarket := extendedQuote(snapshot.PreMarket)
	afterMarket := extendedQuote(snapshot.AfterMarket)
	overnight := extendedQuote(snapshot.Overnight)
	attachFutuSessionWindow(instrumentID, preMarket, market.SessionPre, snapshot.QuoteAt)
	attachFutuSessionWindow(instrumentID, afterMarket, market.SessionAfter, snapshot.QuoteAt)
	attachFutuSessionWindow(instrumentID, overnight, market.SessionOvernight, snapshot.QuoteAt)
	return &marketdata.Tick{
		InstrumentID: instrumentID, Market: resolvedMarket, Symbol: symbol,
		Price: snapshot.Price, Bid: snapshot.Bid, Ask: snapshot.Ask,
		OpenPrice: snapshot.OpenPrice, HighPrice: snapshot.HighPrice, LowPrice: snapshot.LowPrice,
		PreviousClosePrice: snapshot.PreviousClosePrice, LastClosePrice: snapshot.LastClosePrice,
		Volume: snapshot.Volume, Turnover: snapshot.Turnover,
		QuoteAt:    snapshot.QuoteAt.UTC().Format(time.RFC3339Nano),
		ObservedAt: observedAt.UTC().Format(time.RFC3339Nano), Source: "bbgo:futu",
		Session: string(snapshot.Session), ExtendedHours: snapshot.ExtendedHours,
		PreMarket: preMarket, AfterMarket: afterMarket,
		Overnight: overnight, Kind: marketdata.TickKindQuote,
	}
}

func attachFutuSessionWindow(
	instrumentID string,
	quote *marketdata.ExtendedQuote,
	session market.Session,
	tradingDay time.Time,
) {
	if quote == nil {
		return
	}
	window, ok := market.ResolveTradingDaySessionWindow(instrumentID, tradingDay, session)
	if !ok {
		return
	}
	quote.TradingDate = window.TradingDate
	quote.ExchangeTimezone = window.Timezone
	quote.SessionStartAt = window.StartAt.Format(time.RFC3339Nano)
	quote.SessionEndAt = window.EndAt.Format(time.RFC3339Nano)
}

func decimalFromFixedpoint(value fixedpoint.Value) decimal.Decimal {
	if value.IsInf() {
		return decimal.Zero
	}
	parsed, err := decimal.NewFromString(value.String())
	if err != nil {
		return decimal.Zero
	}
	return parsed
}

func optionalDecimal(value interface {
	IsZero() bool
	String() string
}) *decimal.Decimal {
	if value.IsZero() {
		return nil
	}
	return new(decimal.RequireFromString(value.String()))
}

func extendedQuote(quote *pkgfutu.ExtendedMarketQuote) *marketdata.ExtendedQuote {
	if quote == nil {
		return nil
	}
	return &marketdata.ExtendedQuote{
		Price: quote.Price, HighPrice: quote.HighPrice, LowPrice: quote.LowPrice,
		Volume: quote.Volume, Turnover: quote.Turnover, ChangeVal: quote.ChangeVal,
		ChangeRate: quote.ChangeRate, Amplitude: quote.Amplitude, QuoteTime: quote.QuoteTime,
	}
}
