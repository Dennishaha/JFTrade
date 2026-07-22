package servercore

import (
	"math"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/shopspring/decimal"
)

func TestMarketTradeFromTickUsesExplicitVolumeDelta(t *testing.T) {
	at := time.Date(2026, time.July, 21, 9, 31, 0, 0, time.UTC)
	trade, ok := marketTradeFromTick(mdsrv.Tick{
		InstrumentID: "HK.00700",
		Price:        decimal.RequireFromString("321.5"),
		Volume:       1_200_000,
		VolumeDelta:  25,
		QuoteAt:      at.Format(time.RFC3339Nano),
		Kind:         mdsrv.TickKindTrade,
	})
	if !ok {
		t.Fatal("marketTradeFromTick rejected a valid trade tick")
	}
	if trade.Quantity.Float64() != 25 || trade.Price.Float64() != 321.5 || trade.Time.Time() != at {
		t.Fatalf("market trade = %#v", trade)
	}
}

func TestMarketTradeFromTickRejectsAmbiguousOrInvalidDelta(t *testing.T) {
	for _, tick := range []mdsrv.Tick{
		{Kind: mdsrv.TickKindQuote, Price: decimal.RequireFromString("1")},
		{Kind: mdsrv.TickKindTrade, Price: decimal.RequireFromString("1"), VolumeDelta: -1},
		{Kind: mdsrv.TickKindTrade, Price: decimal.RequireFromString("1"), VolumeDelta: math.NaN()},
		{Kind: mdsrv.TickKindTrade, Price: decimal.RequireFromString("1"), VolumeDelta: math.Inf(1)},
	} {
		if _, ok := marketTradeFromTick(tick); ok {
			t.Fatalf("marketTradeFromTick accepted %#v", tick)
		}
	}
}
