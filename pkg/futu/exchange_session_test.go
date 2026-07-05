package futu

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestSessionFromExtendedBlocksClockGuardsStaleExtendedData(t *testing.T) {
	priceOf := func(v float64) *ExtendedMarketQuote {
		return &ExtendedMarketQuote{Price: new(decimal.NewFromFloat(v))}
	}
	// Use a Tuesday in early January (EST, no DST ambiguity).
	// 10:00 UTC = 05:00 EST (pre-market window).
	preMarketClock := time.Date(2025, time.January, 7, 10, 0, 0, 0, time.UTC)
	// 16:00 UTC = 11:00 EST (regular session).
	regularClock := time.Date(2025, time.January, 7, 16, 0, 0, 0, time.UTC)
	// 22:00 UTC = 17:00 EST (after-hours).
	afterClock := time.Date(2025, time.January, 7, 22, 0, 0, 0, time.UTC)
	// 02:00 UTC = 21:00 EST previous day (overnight).
	overnightClock := time.Date(2025, time.January, 8, 2, 0, 0, 0, time.UTC)
	holidayClock := time.Date(2026, time.June, 19, 16, 0, 0, 0, time.UTC)

	stalePre := priceOf(195.0)
	staleAfter := priceOf(198.0)
	staleOvernight := priceOf(200.0)

	cases := []struct {
		name      string
		now       time.Time
		pre       *ExtendedMarketQuote
		after     *ExtendedMarketQuote
		overnight *ExtendedMarketQuote
		want      market.Session
	}{
		{
			name:      "regular clock ignores stale overnight and after blocks",
			now:       regularClock,
			pre:       stalePre,
			after:     staleAfter,
			overnight: staleOvernight,
			want:      market.SessionRegular,
		},
		{
			name:      "pre-market clock ignores stale overnight block",
			now:       preMarketClock,
			pre:       priceOf(196.5),
			after:     staleAfter,
			overnight: staleOvernight,
			want:      market.SessionPre,
		},
		{
			name:      "after clock with after data returns after",
			now:       afterClock,
			pre:       stalePre,
			after:     priceOf(199.5),
			overnight: staleOvernight,
			want:      market.SessionAfter,
		},
		{
			name:      "overnight clock with overnight data returns overnight",
			now:       overnightClock,
			pre:       stalePre,
			after:     staleAfter,
			overnight: priceOf(201.25),
			want:      market.SessionOvernight,
		},
		{
			name:      "after clock without after data falls back to clock",
			now:       afterClock,
			pre:       nil,
			after:     nil,
			overnight: nil,
			want:      market.SessionAfter,
		},
		{
			name:      "holiday stays closed even with stale premarket data",
			now:       holidayClock,
			pre:       priceOf(196.5),
			after:     staleAfter,
			overnight: staleOvernight,
			want:      market.SessionClosed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sessionFromExtendedBlocksAt("US.AAPL", tc.pre, tc.after, tc.overnight, tc.now)
			if got != tc.want {
				t.Fatalf("session=%s, want %s", got, tc.want)
			}
		})
	}
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func jftradePanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
