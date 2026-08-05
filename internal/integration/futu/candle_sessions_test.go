package futu

import (
	"slices"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestMarketSessionsForCandleSessions(t *testing.T) {
	got := MarketSessionsForCandleSessions([]marketdata.CandleSession{
		marketdata.CandleSessionRegular,
		marketdata.CandleSessionExtended,
		marketdata.CandleSessionOvernight,
	})
	want := []market.Session{market.SessionRegular, market.SessionPre, market.SessionAfter, market.SessionOvernight}
	if !slices.Equal(got, want) {
		t.Fatalf("mapped sessions = %#v, want %#v", got, want)
	}
}
