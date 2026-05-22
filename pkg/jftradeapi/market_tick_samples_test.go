package jftradeapi

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

func TestRecordTickerSampleDeduplicatesUnchangedQuote(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	quoteTime := time.Date(2026, time.May, 19, 15, 24, 26, 0, time.UTC)
	ticker := &bbgotypes.Ticker{
		Time:   quoteTime,
		Last:   fixedpoint.NewFromFloat(700.1),
		Buy:    fixedpoint.NewFromFloat(700.0),
		Sell:   fixedpoint.NewFromFloat(700.2),
		Open:   fixedpoint.NewFromFloat(698.0),
		High:   fixedpoint.NewFromFloat(701.0),
		Low:    fixedpoint.NewFromFloat(697.5),
		Volume: fixedpoint.NewFromFloat(12345),
	}

	first := server.recordTickerSample("HK.00700", ticker)
	second := server.recordTickerSample("HK.00700", ticker)
	if first == nil || second == nil {
		t.Fatal("expected samples to be recorded")
	}
	if first.ObservedAt != second.ObservedAt {
		t.Fatalf("expected unchanged quote to reuse latest sample, got %s then %s", first.ObservedAt, second.ObservedAt)
	}

	if got := server.tickCache.count("HK.00700"); got != 1 {
		t.Fatalf("expected one cached sample, got %d", got)
	}
}

func TestRecordTickerSampleInheritsLatestSnapshotFields(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)

	previousClose := decimal.RequireFromString("698.9")
	lastClose := decimal.RequireFromString("698.1")
	prePrice := 699.5
	afterPrice := 701.2
	overnightPrice := 697.8
	seedCachedTickSample(server, marketTickSample{
		InstrumentID:       "HK.00700",
		Market:             "HK",
		Symbol:             "00700",
		Price:              decimal.RequireFromString("700.1"),
		Bid:                decimal.RequireFromString("700.0"),
		Ask:                decimal.RequireFromString("700.2"),
		PreviousClosePrice: &previousClose,
		LastClosePrice:     &lastClose,
		Volume:             12345,
		Turnover:           998877.5,
		QuoteAt:            time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		ObservedAt:         time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		Source:             "bbgo:futu",
		Session:            "regular",
		PreMarket:          &futu.ExtendedMarketQuote{Price: &prePrice},
		AfterMarket:        &futu.ExtendedMarketQuote{Price: &afterPrice},
		Overnight:          &futu.ExtendedMarketQuote{Price: &overnightPrice},
	})

	ticker := &bbgotypes.Ticker{
		Time:   time.Date(2026, time.May, 22, 8, 0, 0, 0, time.UTC),
		Last:   fixedpoint.NewFromFloat(701.1),
		Buy:    fixedpoint.NewFromFloat(701.0),
		Sell:   fixedpoint.NewFromFloat(701.2),
		Volume: fixedpoint.NewFromFloat(22222),
	}

	sample := server.recordTickerSample("HK.00700", ticker)
	if sample == nil {
		t.Fatal("expected sample to be recorded")
	}
	if sample.PreviousClosePrice == nil || !sample.PreviousClosePrice.Equal(previousClose) {
		t.Fatalf("PreviousClosePrice = %#v", sample.PreviousClosePrice)
	}
	if sample.LastClosePrice == nil || !sample.LastClosePrice.Equal(lastClose) {
		t.Fatalf("LastClosePrice = %#v", sample.LastClosePrice)
	}
	if sample.Turnover != 998877.5 {
		t.Fatalf("Turnover = %v", sample.Turnover)
	}
	if sample.PreMarket == nil || sample.PreMarket.Price == nil || *sample.PreMarket.Price != prePrice {
		t.Fatalf("PreMarket = %#v", sample.PreMarket)
	}
	if sample.AfterMarket == nil || sample.AfterMarket.Price == nil || *sample.AfterMarket.Price != afterPrice {
		t.Fatalf("AfterMarket = %#v", sample.AfterMarket)
	}
	if sample.Overnight == nil || sample.Overnight.Price == nil || *sample.Overnight.Price != overnightPrice {
		t.Fatalf("Overnight = %#v", sample.Overnight)
	}
}

func TestRecordTradeTickSampleInheritsLatestQuoteFields(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)

	openPrice := decimal.RequireFromString("698.0")
	highPrice := decimal.RequireFromString("705.0")
	lowPrice := decimal.RequireFromString("697.2")
	previousClose := decimal.RequireFromString("697.0")
	lastClose := decimal.RequireFromString("696.8")
	seedCachedTickSample(server, marketTickSample{
		InstrumentID:       "HK.00700",
		Market:             "HK",
		Symbol:             "00700",
		Price:              decimal.RequireFromString("700.1"),
		Bid:                decimal.RequireFromString("700.0"),
		Ask:                decimal.RequireFromString("700.2"),
		OpenPrice:          &openPrice,
		HighPrice:          &highPrice,
		LowPrice:           &lowPrice,
		PreviousClosePrice: &previousClose,
		LastClosePrice:     &lastClose,
		Volume:             43210,
		Turnover:           1234567.8,
		QuoteAt:            time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		ObservedAt:         time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		Source:             "bbgo:futu",
		Session:            "regular",
	})

	trade := bbgotypes.Trade{
		Symbol:   "HK.00700",
		Price:    fixedpoint.NewFromFloat(702.3),
		Quantity: fixedpoint.NewFromFloat(0),
		Time:     bbgotypes.Time(time.Date(2026, time.May, 22, 8, 1, 0, 0, time.UTC)),
	}

	sample := server.recordTradeTickSample(trade)
	if sample == nil {
		t.Fatal("expected trade sample to be recorded")
	}
	if !sample.Bid.Equal(decimal.RequireFromString("700.0")) {
		t.Fatalf("Bid = %s", sample.Bid)
	}
	if !sample.Ask.Equal(decimal.RequireFromString("700.2")) {
		t.Fatalf("Ask = %s", sample.Ask)
	}
	if sample.OpenPrice == nil || !sample.OpenPrice.Equal(openPrice) {
		t.Fatalf("OpenPrice = %#v", sample.OpenPrice)
	}
	if sample.HighPrice == nil || !sample.HighPrice.Equal(highPrice) {
		t.Fatalf("HighPrice = %#v", sample.HighPrice)
	}
	if sample.LowPrice == nil || !sample.LowPrice.Equal(lowPrice) {
		t.Fatalf("LowPrice = %#v", sample.LowPrice)
	}
	if sample.PreviousClosePrice == nil || !sample.PreviousClosePrice.Equal(previousClose) {
		t.Fatalf("PreviousClosePrice = %#v", sample.PreviousClosePrice)
	}
	if sample.LastClosePrice == nil || !sample.LastClosePrice.Equal(lastClose) {
		t.Fatalf("LastClosePrice = %#v", sample.LastClosePrice)
	}
	if sample.Turnover != 1234567.8 {
		t.Fatalf("Turnover = %v", sample.Turnover)
	}
	if sample.Volume != 43210 {
		t.Fatalf("Volume = %v", sample.Volume)
	}
}

func TestResolveLiveTickSampleSessionReclassifiesUSBoundary(t *testing.T) {
	latest := &marketTickSample{
		InstrumentID:  "US.AAPL",
		Session:       string(futu.MarketSessionPre),
		ExtendedHours: true,
	}

	regularClock := time.Date(2026, time.January, 7, 16, 0, 0, 0, time.UTC)
	session, extendedHours := resolveLiveTickSampleSession("US.AAPL", regularClock, latest)
	if session != string(futu.MarketSessionRegular) {
		t.Fatalf("session = %s", session)
	}
	if extendedHours {
		t.Fatal("expected regular session to clear extendedHours")
	}
}
