package jftradeapi

import (
	"strings"
	"time"

	bbgofixedpoint "github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/market"
)

const (
	maxTickCacheSamples = 30000
	tickCacheRetention  = 30 * time.Minute
)

type marketTickSample struct {
	InstrumentID       string
	Market             string
	Symbol             string
	Price              decimal.Decimal
	Bid                decimal.Decimal
	Ask                decimal.Decimal
	OpenPrice          *decimal.Decimal
	HighPrice          *decimal.Decimal
	LowPrice           *decimal.Decimal
	PreviousClosePrice *decimal.Decimal
	LastClosePrice     *decimal.Decimal // 始终 = GetLastClosePrice()（上个交易日收盘）
	Volume             float64
	Turnover           decimal.Decimal
	QuoteAt            string
	ObservedAt         string
	Source             string
	Session            string
	ExtendedHours      bool
	PreMarket          *futu.ExtendedMarketQuote
	AfterMarket        *futu.ExtendedMarketQuote
	Overnight          *futu.ExtendedMarketQuote
}

func tickerTimestamp(ticker *bbgotypes.Ticker) string {
	resolvedAt := time.Now().UTC()
	if ticker != nil && !ticker.Time.IsZero() {
		resolvedAt = ticker.Time.UTC()
	}
	return resolvedAt.Format(time.RFC3339Nano)
}

func resolveLiveTickSampleSession(instrumentID string, observedAt time.Time, latest *marketTickSample) (string, bool) {
	session := market.ClassifySession(instrumentID, observedAt)
	if session != market.SessionUnknown || latest == nil {
		return string(session), market.IsExtendedSession(session)
	}
	return latest.Session, latest.ExtendedHours
}

// tickerOptionalDecimal converts a bbgo fixedpoint.Value to *decimal.Decimal,
// returning nil when the value is zero (meaning "not available").
func tickerOptionalDecimal(value bbgofixedpoint.Value) *decimal.Decimal {
	if value.IsZero() {
		return nil
	}
	d := decimal.RequireFromString(value.String())
	return &d
}

// decimalPtr converts a *float64 from a Futu proto adapter into *decimal.Decimal.
// decimal.NewFromFloat uses the shortest float64 string representation, so
// proto values like 23.65 arrive as exactly "23.65" without IEEE-754 noise.
func decimalPtr(v *float64) *decimal.Decimal {
	if v == nil {
		return nil
	}
	d := decimal.NewFromFloat(*v)
	return &d
}

func (s *Server) recordQuoteSnapshotSample(instrumentID string, snapshot *futu.QuoteSnapshot) *marketTickSample {
	if snapshot == nil || snapshot.Price.IsZero() {
		return nil
	}
	parts := strings.SplitN(instrumentID, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	sample := marketTickSample{
		InstrumentID:       instrumentID,
		Market:             parts[0],
		Symbol:             parts[1],
		Price:              snapshot.Price,
		Bid:                snapshot.Bid,
		Ask:                snapshot.Ask,
		OpenPrice:          snapshot.OpenPrice,
		HighPrice:          snapshot.HighPrice,
		LowPrice:           snapshot.LowPrice,
		PreviousClosePrice: snapshot.PreviousClosePrice,
		LastClosePrice:     snapshot.LastClosePrice,
		Volume:             snapshot.Volume,
		Turnover:           snapshot.Turnover,
		QuoteAt:            snapshot.QuoteAt.UTC().Format(time.RFC3339Nano),
		ObservedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Source:             "bbgo:futu",
		Session:            string(snapshot.Session),
		ExtendedHours:      snapshot.ExtendedHours,
		PreMarket:          snapshot.PreMarket,
		AfterMarket:        snapshot.AfterMarket,
		Overnight:          snapshot.Overnight,
	}
	return s.storeTickerSample(sample)
}

func (s *Server) recordTickerSample(instrumentID string, ticker *bbgotypes.Ticker) *marketTickSample {
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
	parts := strings.SplitN(instrumentID, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	price := decimal.RequireFromString(priceFixed.String())
	bid := price
	if !ticker.Buy.IsZero() {
		bid = decimal.RequireFromString(ticker.Buy.String())
	}
	ask := price
	if !ticker.Sell.IsZero() {
		ask = decimal.RequireFromString(ticker.Sell.String())
	}
	observedAt := time.Now().UTC()
	latest := s.latestTickerSample(instrumentID, tickCacheRetention)
	session, extendedHours := resolveLiveTickSampleSession(instrumentID, observedAt, latest)
	sample := marketTickSample{
		InstrumentID:  instrumentID,
		Market:        parts[0],
		Symbol:        parts[1],
		Price:         price,
		Bid:           bid,
		Ask:           ask,
		OpenPrice:     tickerOptionalDecimal(ticker.Open),
		HighPrice:     tickerOptionalDecimal(ticker.High),
		LowPrice:      tickerOptionalDecimal(ticker.Low),
		Volume:        ticker.Volume.Float64(),
		Turnover:      decimal.Zero,
		QuoteAt:       tickerTimestamp(ticker),
		ObservedAt:    observedAt.Format(time.RFC3339Nano),
		Source:        "bbgo:futu",
		Session:       session,
		ExtendedHours: extendedHours,
	}
	// The ticker stream does not carry PreviousClosePrice or extended-session
	// quote blocks (PreMarket/AfterMarket/Overnight). Inherit those from the
	// most-recent cached sample so they are not silently dropped when a live
	// ticker event overwrites the snapshot used by the frontend.
	inheritLatestTickSampleContext(&sample, latest)
	return s.storeTickerSample(sample)
}

func (s *Server) recordTradeTickSample(trade bbgotypes.Trade) *marketTickSample {
	instrumentID := strings.ToUpper(strings.TrimSpace(trade.Symbol))
	if instrumentID == "" || trade.Price.IsZero() {
		return nil
	}
	parts := strings.SplitN(instrumentID, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	price := decimal.RequireFromString(trade.Price.String())
	observedAt := time.Now().UTC()
	quoteAt := observedAt
	if !trade.Time.Time().IsZero() {
		quoteAt = trade.Time.Time().UTC()
	}
	latest := s.latestTickerSample(instrumentID, tickCacheRetention)
	session, extendedHours := resolveLiveTickSampleSession(instrumentID, observedAt, latest)
	sample := marketTickSample{
		InstrumentID:  instrumentID,
		Market:        parts[0],
		Symbol:        parts[1],
		Price:         price,
		Bid:           price,
		Ask:           price,
		Volume:        trade.Quantity.Float64(),
		QuoteAt:       quoteAt.Format(time.RFC3339Nano),
		ObservedAt:    observedAt.Format(time.RFC3339Nano),
		Source:        "bbgo:futu:stream",
		Session:       session,
		ExtendedHours: extendedHours,
	}
	inheritLatestTickSampleContext(&sample, latest)
	inheritLatestTradeBookAndVolume(&sample, latest)
	return s.storeTickerSample(sample)
}

func inheritLatestTickSampleContext(sample *marketTickSample, latest *marketTickSample) {
	if sample == nil || latest == nil {
		return
	}
	if sample.OpenPrice == nil {
		sample.OpenPrice = latest.OpenPrice
	}
	if sample.HighPrice == nil {
		sample.HighPrice = latest.HighPrice
	}
	if sample.LowPrice == nil {
		sample.LowPrice = latest.LowPrice
	}
	if sample.PreviousClosePrice == nil {
		sample.PreviousClosePrice = latest.PreviousClosePrice
	}
	if sample.LastClosePrice == nil {
		sample.LastClosePrice = latest.LastClosePrice
	}
	if sample.PreMarket == nil {
		sample.PreMarket = latest.PreMarket
	}
	if sample.AfterMarket == nil {
		sample.AfterMarket = latest.AfterMarket
	}
	if sample.Overnight == nil {
		sample.Overnight = latest.Overnight
	}
	if sample.Turnover.IsZero() {
		sample.Turnover = latest.Turnover
	}
}

func inheritLatestTradeBookAndVolume(sample *marketTickSample, latest *marketTickSample) {
	if sample == nil || latest == nil {
		return
	}
	sample.Bid = latest.Bid
	sample.Ask = latest.Ask
	if sample.Volume == 0 {
		sample.Volume = latest.Volume
	}
}
