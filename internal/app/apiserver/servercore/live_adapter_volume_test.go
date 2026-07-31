package servercore

import (
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
		Volume:       decimal.RequireFromString("1200000.5"),
		VolumeDelta:  decimal.RequireFromString("25.125"),
		QuoteAt:      at.Format(time.RFC3339Nano),
		Kind:         mdsrv.TickKindTrade,
	})
	if !ok {
		t.Fatal("marketTradeFromTick rejected a valid trade tick")
	}
	if trade.Quantity.String() != "25.125" || trade.Price.String() != "321.5" || trade.Time.Time() != at {
		t.Fatalf("market trade = %#v", trade)
	}
}

func TestMarketTradeFromTickKeepsDecimalVolumeWhenLegacyQuantityOverflows(t *testing.T) {
	trade, ok := marketTradeFromTick(mdsrv.Tick{
		InstrumentID: "US.AAPL",
		Price:        decimal.RequireFromString("100"),
		Volume:       decimal.RequireFromString("9007199254740995"),
		VolumeDelta:  decimal.RequireFromString("9007199254740993"),
		Kind:         mdsrv.TickKindTrade,
	})
	if !ok {
		t.Fatal("marketTradeFromTick discarded an otherwise valid Decimal trade")
	}
	if !trade.Quantity.IsZero() {
		t.Fatalf("legacy quantity = %s, want zero outside fixedpoint range", trade.Quantity)
	}
	if trade.VolumeDelta == nil || trade.VolumeDelta.String() != "9007199254740993" ||
		trade.CumulativeVolume == nil || trade.CumulativeVolume.String() != "9007199254740995" {
		t.Fatalf("Decimal volume fields = delta:%v cumulative:%v", trade.VolumeDelta, trade.CumulativeVolume)
	}
}

func TestMarketTradeFromTickRejectsAmbiguousOrInvalidDelta(t *testing.T) {
	for _, tick := range []mdsrv.Tick{
		{Kind: mdsrv.TickKindQuote, Price: decimal.RequireFromString("1")},
		{Kind: mdsrv.TickKindTrade, Price: decimal.RequireFromString("1"), VolumeDelta: decimal.NewFromInt(-1)},
	} {
		if _, ok := marketTradeFromTick(tick); ok {
			t.Fatalf("marketTradeFromTick accepted %#v", tick)
		}
	}
}
