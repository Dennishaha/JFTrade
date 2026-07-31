package marketdataapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestWatchlistSnapshotSourceRoutesFutuAndYFinance(t *testing.T) {
	futu := &watchlistSourceStub{quotes: []watchlist.Quote{{
		InstrumentID: "HK.00700",
		Source:       "futu:security-snapshot",
	}}}
	runtime := &watchlistRuntimeStub{providerID: ProviderFutu}
	source := NewWatchlistSnapshotSource(
		func() WatchlistQuoteRuntime { return runtime },
		futu,
	)
	quotes, _, err := source.BatchSnapshots(t.Context(), []string{"HK.00700"})
	if err != nil || len(quotes) != 1 || quotes[0].Source != "futu:security-snapshot" {
		t.Fatalf("Futu quotes = %#v, err=%v", quotes, err)
	}

	previousClose := decimal.NewFromInt(100)
	runtime.providerID = ProviderYFinance
	runtime.ticks = map[string]marketdata.Tick{
		"US.AAPL": {
			InstrumentID:       "US.AAPL",
			Price:              decimal.NewFromInt(105),
			PreviousClosePrice: &previousClose,
			Volume:             decimal.NewFromInt(123),
			Turnover:           decimal.NewFromInt(456),
			ObservedAt:         "2026-07-29T10:00:00Z",
			QuoteAt:            "2026-07-29T09:59:00Z",
			Session:            "pre",
			Source:             "yfinance",
		},
	}
	quotes, itemErrors, err := source.BatchSnapshots(
		t.Context(),
		[]string{"US.AAPL", "US.MISSING"},
	)
	if err != nil || len(quotes) != 1 || len(itemErrors) != 1 {
		t.Fatalf("yfinance quotes = %#v, errors=%#v, err=%v", quotes, itemErrors, err)
	}
	quote := quotes[0]
	if quote.Price == nil || *quote.Price != 105 ||
		quote.Change == nil || *quote.Change != 5 ||
		quote.ChangePercent == nil || *quote.ChangePercent != 5 ||
		quote.Volume == nil || *quote.Volume != 123 ||
		quote.Turnover == nil || *quote.Turnover != 456 ||
		quote.Extended == nil || quote.Extended.Pre == nil ||
		quote.UpdateTime == nil {
		t.Fatalf("converted yfinance quote = %#v", quote)
	}
	if futu.calls != 1 || runtime.calls != 1 {
		t.Fatalf("route calls: Futu=%d yfinance=%d", futu.calls, runtime.calls)
	}
	if ttl := source.(watchlist.QuoteCachePolicySource).QuoteCacheTTL(); ttl != 15*time.Second {
		t.Fatalf("yfinance quote cache TTL = %s", ttl)
	}
	runtime.providerID = ProviderFutu
	if ttl := source.(watchlist.QuoteCachePolicySource).QuoteCacheTTL(); ttl != 0 {
		t.Fatalf("Futu quote cache TTL = %s, want default", ttl)
	}
}

func TestWatchlistSnapshotSourceReportsUnavailableBoundaries(t *testing.T) {
	var nilSource *watchlistSnapshotSource
	if _, _, err := nilSource.BatchSnapshots(t.Context(), nil); !errors.Is(err, watchlist.ErrUnavailable) {
		t.Fatalf("nil source error = %v", err)
	}
	source := NewWatchlistSnapshotSource(nil, nil)
	if _, _, err := source.BatchSnapshots(t.Context(), nil); !errors.Is(err, watchlist.ErrUnavailable) {
		t.Fatalf("missing Futu source error = %v", err)
	}

	runtime := &watchlistRuntimeStub{providerID: ProviderYFinance, err: errors.New("sidecar down")}
	source = NewWatchlistSnapshotSource(func() WatchlistQuoteRuntime { return runtime }, &watchlistSourceStub{})
	if _, _, err := source.BatchSnapshots(t.Context(), []string{"US.AAPL"}); !errors.Is(err, watchlist.ErrUnavailable) {
		t.Fatalf("yfinance query error = %v", err)
	}
	runtime.providerID = "unknown"
	if _, _, err := source.BatchSnapshots(t.Context(), nil); !errors.Is(err, watchlist.ErrUnavailable) {
		t.Fatalf("unknown provider error = %v", err)
	}

	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	quote := watchlistQuoteFromTick(marketdata.Tick{
		InstrumentID: "US.AAPL",
		Price:        decimal.NewFromInt(1),
		ObservedAt:   "invalid",
		Session:      "after",
	}, now)
	if !quote.ObservedAt.Equal(now) || quote.Extended == nil || quote.Extended.After == nil {
		t.Fatalf("fallback converted quote = %#v", quote)
	}

	previous := decimal.NewFromInt(1)
	postPrice := decimal.NewFromFloat(1.25)
	postChange := decimal.NewFromFloat(0.25)
	postTime := "2026-07-29T20:00:00Z"
	quote = watchlistQuoteFromTick(marketdata.Tick{
		InstrumentID:       "US.AAPL",
		Price:              decimal.NewFromInt(1),
		PreviousClosePrice: &previous,
		ObservedAt:         now.Format(time.RFC3339),
		Session:            "closed",
		AfterMarket: &marketdata.ExtendedQuote{
			Price: &postPrice, ChangeVal: &postChange, QuoteTime: postTime,
		},
	}, now)
	if quote.Extended == nil || quote.Extended.After == nil ||
		quote.Extended.After.Price == nil || *quote.Extended.After.Price != 1.25 ||
		quote.Extended.After.Change == nil || *quote.Extended.After.Change != 0.25 ||
		quote.Extended.After.UpdateTime == nil ||
		!quote.Extended.After.UpdateTime.Equal(time.Date(2026, 7, 29, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("closed quote did not preserve after-market block = %#v", quote)
	}
}

func TestWatchlistQuoteUsesPriorCloseForClosedRegularYahooQuote(t *testing.T) {
	regularClose := decimal.RequireFromString("333.43")
	priorClose := decimal.RequireFromString("338.20")
	afterHours := decimal.RequireFromString("312.33")
	afterHoursChange := decimal.RequireFromString("-6.33")
	quote := watchlistQuoteFromTick(marketdata.Tick{
		InstrumentID:       "US.AAPL",
		Price:              regularClose,
		PreviousClosePrice: &regularClose,
		LastClosePrice:     &priorClose,
		Session:            "closed",
		ObservedAt:         "2026-07-31T08:00:00Z",
		AfterMarket: &marketdata.ExtendedQuote{
			Price:      &afterHours,
			ChangeRate: &afterHoursChange,
		},
	}, time.Time{})

	if quote.Price == nil || !decimal.NewFromFloat(*quote.Price).Round(2).Equal(regularClose) ||
		quote.PreviousClose == nil || !decimal.NewFromFloat(*quote.PreviousClose).Round(2).Equal(priorClose) ||
		quote.Change == nil || !decimal.NewFromFloat(*quote.Change).Round(2).Equal(decimal.RequireFromString("-4.77")) {
		t.Fatalf("closed regular quote = %#v", quote)
	}
	if quote.ChangePercent == nil || !decimal.NewFromFloat(*quote.ChangePercent).Round(2).Equal(decimal.RequireFromString("-1.41")) {
		t.Fatalf("closed regular change percent = %#v", quote.ChangePercent)
	}
	if quote.Extended == nil || quote.Extended.After == nil ||
		quote.Extended.After.ChangePercent == nil ||
		!decimal.NewFromFloat(*quote.Extended.After.ChangePercent).Round(2).Equal(afterHoursChange) {
		t.Fatalf("after-hours comparison reference changed: %#v", quote.Extended)
	}
}

type watchlistRuntimeStub struct {
	providerID string
	ticks      map[string]marketdata.Tick
	err        error
	calls      int
}

func (s *watchlistRuntimeStub) ActiveProviderID() string {
	return s.providerID
}

func (s *watchlistRuntimeStub) QueryTickers(
	context.Context,
	[]string,
) (map[string]marketdata.Tick, error) {
	s.calls++
	return s.ticks, s.err
}

func (s *watchlistRuntimeStub) QuotePollingPolicy() marketdata.QuotePollingPolicy {
	return marketdata.QuotePollingPolicy{Interval: 15 * time.Second}
}

type watchlistSourceStub struct {
	quotes []watchlist.Quote
	errors []watchlist.QuoteError
	err    error
	calls  int
}

func (s *watchlistSourceStub) BatchSnapshots(
	context.Context,
	[]string,
) ([]watchlist.Quote, []watchlist.QuoteError, error) {
	s.calls++
	return s.quotes, s.errors, s.err
}
