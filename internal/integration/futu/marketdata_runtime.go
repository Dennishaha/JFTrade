package futu

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	"github.com/jftrade/jftrade-main/pkg/market"
)

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
	ConfigSource func() MarketDataConfig
	NewExchange  func(MarketDataConfig) *pkgfutu.Exchange
	OnExchange   func(*pkgfutu.Exchange)
	Now          func() time.Time
}

// MarketDataRuntime owns the broker-specific exchange lifecycle and protocol
// conversion. Freshness, demand, cache, polling, and backoff stay in marketdata.
type MarketDataRuntime struct {
	configSource func() MarketDataConfig
	newExchange  func(MarketDataConfig) *pkgfutu.Exchange
	onExchange   func(*pkgfutu.Exchange)
	now          func() time.Time

	mu         sync.Mutex
	exchange   *pkgfutu.Exchange
	key        string
	generation uint64
	closed     bool
	creating   bool
	createDone chan struct{}
	wg         sync.WaitGroup
}

func NewMarketDataRuntime(options MarketDataRuntimeOptions) *MarketDataRuntime {
	r := &MarketDataRuntime{
		configSource: options.ConfigSource,
		newExchange:  options.NewExchange,
		onExchange:   options.OnExchange,
		now:          options.Now,
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
	if r.now == nil {
		r.now = time.Now
	}
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

		r.mu.Lock()
		valid := !r.closed && r.generation == generation && r.configSource().key() == key
		var previous *pkgfutu.Exchange
		if valid {
			previous = r.exchange
			if candidate != nil && r.onExchange != nil {
				r.onExchange(candidate)
			}
			r.exchange = candidate
			r.key = key
		}
		r.creating = false
		close(done)
		r.wg.Done()
		r.mu.Unlock()

		if !valid {
			if candidate != nil {
				_ = candidate.Close()
			}
			return nil
		}
		if previous != nil && previous != candidate {
			_ = previous.Close()
		}
		return candidate
	}
}

func (r *MarketDataRuntime) Exchange() *pkgfutu.Exchange {
	return r.Ensure()
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
	r.key = ""
	r.mu.Unlock()
	if exchange != nil {
		_ = exchange.Close()
	}
}

func (r *MarketDataRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.generation++
	exchange := r.exchange
	r.exchange = nil
	r.key = ""
	r.mu.Unlock()
	if exchange != nil {
		_ = exchange.Close()
	}
	r.wg.Wait()
	return nil
}

func (r *MarketDataRuntime) QueryTickers(ctx context.Context, instrumentIDs []string) (map[string]marketdata.Tick, error) {
	exchange := r.Ensure()
	if exchange == nil {
		return nil, fmt.Errorf("futu marketdata runtime unavailable")
	}
	tickers, err := exchange.QueryTickers(ctx, instrumentIDs...)
	if err != nil {
		return nil, err
	}
	result := make(map[string]marketdata.Tick, len(tickers))
	for instrumentID, ticker := range tickers {
		if tick := tickFromTicker(instrumentID, &ticker, r.now().UTC()); tick != nil {
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
		return nil, err
	}
	return tickFromSnapshot(instrumentID, snapshot, r.now().UTC()), nil
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
	return &marketdata.Tick{
		InstrumentID: instrumentID, Market: resolvedMarket, Symbol: symbol,
		Price: price, Bid: bid, Ask: ask,
		OpenPrice: optionalDecimal(ticker.Open), HighPrice: optionalDecimal(ticker.High), LowPrice: optionalDecimal(ticker.Low),
		Volume: ticker.Volume.Float64(), QuoteAt: quoteAt.Format(time.RFC3339Nano),
		ObservedAt: observedAt.Format(time.RFC3339Nano), Source: "bbgo:futu",
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
	session := market.ClassifySession(instrumentID, observedAt)
	return &marketdata.Tick{
		InstrumentID: instrumentID, Market: resolvedMarket, Symbol: symbol,
		Price: price, Bid: price, Ask: price, Volume: trade.Quantity.Float64(),
		QuoteAt: quoteAt.Format(time.RFC3339Nano), ObservedAt: observedAt.Format(time.RFC3339Nano),
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
	return &marketdata.Tick{
		InstrumentID: instrumentID, Market: resolvedMarket, Symbol: symbol,
		Price: snapshot.Price, Bid: snapshot.Bid, Ask: snapshot.Ask,
		OpenPrice: snapshot.OpenPrice, HighPrice: snapshot.HighPrice, LowPrice: snapshot.LowPrice,
		PreviousClosePrice: snapshot.PreviousClosePrice, LastClosePrice: snapshot.LastClosePrice,
		Volume: snapshot.Volume, Turnover: snapshot.Turnover,
		QuoteAt:    snapshot.QuoteAt.UTC().Format(time.RFC3339Nano),
		ObservedAt: observedAt.Format(time.RFC3339Nano), Source: "bbgo:futu",
		Session: string(snapshot.Session), ExtendedHours: snapshot.ExtendedHours,
		PreMarket: extendedQuote(snapshot.PreMarket), AfterMarket: extendedQuote(snapshot.AfterMarket),
		Overnight: extendedQuote(snapshot.Overnight), Kind: marketdata.TickKindQuote,
	}
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
