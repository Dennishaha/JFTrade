package marketdataapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/watchlist"
)

// WatchlistQuoteRuntime is the narrow active-provider surface used by the
// watchlist quote adapter.
type WatchlistQuoteRuntime interface {
	ActiveProviderID() string
	QueryTickers(context.Context, []string) (map[string]marketdata.Tick, error)
	QuotePollingPolicy() marketdata.QuotePollingPolicy
}

type watchlistRuntimeProvider func() WatchlistQuoteRuntime

type watchlistSnapshotSource struct {
	runtime watchlistRuntimeProvider
	futu    watchlist.BatchSnapshotSource
	now     func() time.Time
}

// NewWatchlistSnapshotSource preserves Futu's quota-free SecuritySnapshot path
// while routing yfinance selections through the active provider runtime.
func NewWatchlistSnapshotSource(
	runtime func() WatchlistQuoteRuntime,
	futu watchlist.BatchSnapshotSource,
) watchlist.BatchSnapshotSource {
	return &watchlistSnapshotSource{runtime: runtime, futu: futu, now: time.Now}
}

func (s *watchlistSnapshotSource) BatchSnapshots(
	ctx context.Context,
	instrumentIDs []string,
) ([]watchlist.Quote, []watchlist.QuoteError, error) {
	if s == nil {
		return nil, nil, watchlist.ErrUnavailable
	}
	runtime := WatchlistQuoteRuntime(nil)
	if s.runtime != nil {
		runtime = s.runtime()
	}
	if runtime == nil || runtime.ActiveProviderID() == ProviderFutu {
		if s.futu == nil {
			return nil, nil, watchlist.ErrUnavailable
		}
		return s.futu.BatchSnapshots(ctx, instrumentIDs)
	}
	if runtime.ActiveProviderID() != ProviderYFinance {
		return nil, nil, fmt.Errorf(
			"%w: unsupported active market-data provider %q",
			watchlist.ErrUnavailable,
			runtime.ActiveProviderID(),
		)
	}
	ticks, err := runtime.QueryTickers(ctx, instrumentIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: yfinance snapshots: %w", watchlist.ErrUnavailable, err)
	}
	return watchlistQuotesFromTicks(instrumentIDs, ticks, s.currentTime())
}

func (s *watchlistSnapshotSource) QuoteCacheTTL() time.Duration {
	if s == nil || s.runtime == nil {
		return 0
	}
	runtime := s.runtime()
	if runtime == nil || runtime.ActiveProviderID() != ProviderYFinance {
		return 0
	}
	return runtime.QuotePollingPolicy().Interval
}

func (s *watchlistSnapshotSource) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func watchlistQuotesFromTicks(
	instrumentIDs []string,
	ticks map[string]marketdata.Tick,
	now time.Time,
) ([]watchlist.Quote, []watchlist.QuoteError, error) {
	quotes := make([]watchlist.Quote, 0, len(ticks))
	itemErrors := make([]watchlist.QuoteError, 0)
	for _, instrumentID := range instrumentIDs {
		tick, ok := ticks[instrumentID]
		if !ok {
			itemErrors = append(itemErrors, watchlist.QuoteError{
				InstrumentID: instrumentID,
				Code:         "SNAPSHOT_NOT_RETURNED",
				Message:      "active market-data provider did not return a snapshot",
			})
			continue
		}
		quotes = append(quotes, watchlistQuoteFromTick(tick, now))
	}
	return quotes, itemErrors, nil
}

func watchlistQuoteFromTick(tick marketdata.Tick, fallback time.Time) watchlist.Quote {
	observedAt := parsedQuoteTime(tick.ObservedAt, fallback)
	updateTime := parsedOptionalQuoteTime(tick.QuoteAt)
	price := tick.Price.InexactFloat64()
	previousClose := watchlistComparisonClose(tick)
	quote := watchlist.Quote{
		InstrumentID:  tick.InstrumentID,
		Source:        strings.TrimSpace(tick.Source),
		Price:         &price,
		PreviousClose: previousClose,
		Volume:        positiveFloat(tick.Volume),
		Turnover:      positiveDecimalFloat(tick.Turnover),
		Session:       strings.TrimSpace(tick.Session),
		ObservedAt:    observedAt,
		UpdateTime:    updateTime,
	}
	if quote.Source == "" {
		quote.Source = "yfinance"
	}
	if previousClose != nil {
		change := price - *previousClose
		quote.Change = &change
		if *previousClose != 0 {
			percent := change / *previousClose * 100
			quote.ChangePercent = &percent
		}
	}
	quote.Extended = extendedQuoteFromTick(tick, quote)
	return quote
}

func extendedQuoteFromTick(tick marketdata.Tick, quote watchlist.Quote) *watchlist.ExtendedQuote {
	observedAt := quote.ObservedAt
	// Extended-session changes are relative to the latest regular-session
	// close. A closed-session watchlist quote uses LastClosePrice for its own
	// daily change, so do not reuse that display reference for after-hours data.
	previousClose := decimalFloat(tick.PreviousClosePrice)
	pre := extendedQuoteBlockFromTick(tick.PreMarket, observedAt, previousClose)
	after := extendedQuoteBlockFromTick(tick.AfterMarket, observedAt, previousClose)
	overnight := extendedQuoteBlockFromTick(tick.Overnight, observedAt, previousClose)
	active := quoteBlockFromQuote(quote)
	switch strings.ToLower(quote.Session) {
	case "pre":
		if pre == nil {
			pre = active
		}
	case "after":
		if after == nil {
			after = active
		}
	}
	if pre == nil && after == nil && overnight == nil {
		return nil
	}
	return &watchlist.ExtendedQuote{Pre: pre, After: after, Overnight: overnight}
}

func watchlistComparisonClose(tick marketdata.Tick) *float64 {
	if strings.EqualFold(strings.TrimSpace(tick.Session), "closed") &&
		tick.LastClosePrice != nil {
		// The active snapshot price is the latest regular close while the
		// provider is closed. Compare it with the prior trading-day close so
		// the watchlist agrees with the main quote card instead of showing 0%.
		return decimalFloat(tick.LastClosePrice)
	}
	return decimalFloat(tick.PreviousClosePrice)
}

func quoteBlockFromQuote(quote watchlist.Quote) *watchlist.QuoteBlock {
	if quote.Price == nil {
		return nil
	}
	return &watchlist.QuoteBlock{
		Price:         quote.Price,
		Change:        quote.Change,
		ChangePercent: quote.ChangePercent,
		ObservedAt:    quote.ObservedAt,
		UpdateTime:    quote.UpdateTime,
	}
}

func extendedQuoteBlockFromTick(
	value *marketdata.ExtendedQuote,
	observedAt time.Time,
	previousClose *float64,
) *watchlist.QuoteBlock {
	if value == nil || value.Price == nil {
		return nil
	}
	block := &watchlist.QuoteBlock{
		Price:         decimalFloat(value.Price),
		Change:        decimalFloat(value.ChangeVal),
		ChangePercent: decimalFloat(value.ChangeRate),
		ObservedAt:    observedAt,
		UpdateTime:    parsedOptionalQuoteTime(value.QuoteTime),
	}
	if block.Change == nil && previousClose != nil {
		change := *block.Price - *previousClose
		block.Change = &change
	}
	if block.ChangePercent == nil && block.Change != nil && previousClose != nil && *previousClose != 0 {
		percent := *block.Change / *previousClose * 100
		block.ChangePercent = &percent
	}
	return block
}

func decimalFloat(value *decimal.Decimal) *float64 {
	if value == nil {
		return nil
	}
	converted := value.InexactFloat64()
	return &converted
}

func positiveDecimalFloat(value decimal.Decimal) *float64 {
	if !value.IsPositive() {
		return nil
	}
	converted := value.InexactFloat64()
	return &converted
}

func positiveFloat(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func parsedQuoteTime(value string, fallback time.Time) time.Time {
	if parsed := parsedOptionalQuoteTime(value); parsed != nil {
		return *parsed
	}
	return fallback.UTC()
}

func parsedOptionalQuoteTime(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}
