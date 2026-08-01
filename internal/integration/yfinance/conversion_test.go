package yfinance

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func number(value string) *json.Number {
	result := json.Number(value)
	return &result
}

func validSnapshot() remoteSnapshot {
	return remoteSnapshot{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL",
		Price: number("99.75"), Bid: number("0"), Ask: nil,
		OpenPrice: number("99"), HighPrice: number("101"), LowPrice: number("98"),
		PreviousClosePrice: number("98.5"), LastClosePrice: number("98.5"), Volume: number("123"), Turnover: number("12345.5"),
		RegularQuote:     &remoteSnapshotQuote{Price: number("99.75"), QuoteAt: "2026-07-29T19:59:59Z"},
		PreMarketQuote:   &remoteSnapshotQuote{Price: number("99.5"), ChangeValue: number("-1.2"), QuoteAt: "2026-07-29T12:00:00Z"},
		AfterMarketQuote: &remoteSnapshotQuote{Price: number("100.25"), QuoteAt: "2026-07-29T21:00:00Z"},
		QuoteAt:          "2026-07-29T19:59:59Z", ObservedAt: "", Source: "",
	}
}

func validRemoteCandles() remoteCandles {
	return remoteCandles{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Period: "1d",
		ExtendedHours: true, TotalReturned: 1, Source: "",
		Candles: []remoteCandle{{
			At: "2026-07-29T13:30:00Z", Open: number("99"), High: number("102"),
			Low: number("98"), Close: number("101.5"), Volume: number("500"),
		}},
	}
}

var afterMarketNow = time.Date(2026, time.July, 29, 21, 15, 0, 0, time.UTC)

func TestSnapshotConversionUsesPriceFallbacksAndCanonicalTimes(t *testing.T) {
	response := validSnapshot()
	expected := normalizedInstrument{market: "US", symbol: "AAPL", id: "US.AAPL"}
	tick, err := convertSnapshot(response, expected, afterMarketNow)
	if err != nil {
		t.Fatalf("convertSnapshot: %v", err)
	}

	if !tick.Bid.Equal(tick.Price) || !tick.Ask.Equal(tick.Price) ||
		!tick.Price.Equal(decimal.RequireFromString("100.25")) ||
		tick.QuoteAt != "2026-07-29T21:00:00Z" ||
		tick.ObservedAt != afterMarketNow.Format(time.RFC3339Nano) ||
		tick.Source != sourceID || tick.Session != "after" || !tick.ExtendedHours ||
		tick.PreviousClosePrice == nil || !tick.PreviousClosePrice.Equal(decimal.RequireFromString("99.75")) ||
		tick.LastClosePrice == nil || !tick.LastClosePrice.Equal(decimal.RequireFromString("98.5")) ||
		tick.PreMarket == nil || tick.AfterMarket == nil ||
		tick.PreMarket.ChangeVal == nil || !tick.PreMarket.ChangeVal.Equal(decimal.RequireFromString("-1.2")) ||
		tick.AfterMarket.QuoteTime != "2026-07-29T21:00:00Z" ||
		tick.AfterMarket.ExchangeTimezone != "America/New_York" ||
		tick.AfterMarket.SessionEndAt != "2026-07-30T00:00:00Z" {
		t.Fatalf("converted tick = %#v", tick)
	}

	response.QuoteAt = ""
	response.AfterMarketQuote = nil
	response.ObservedAt = "2026-07-29T14:45:00Z"
	tick, err = convertSnapshot(response, expected, testNow)
	if err != nil || tick.QuoteAt != "" || tick.ObservedAt != response.ObservedAt {
		t.Fatalf("snapshot without quote time = %#v, err=%v", tick, err)
	}
}

func TestVolumeConversionPreservesLargeFractionalDecimalValues(t *testing.T) {
	const rawVolume = "9007199254740993.25"
	want := decimal.RequireFromString(rawVolume)
	expected := normalizedInstrument{market: "US", symbol: "AAPL", id: "US.AAPL"}

	snapshot := validSnapshot()
	snapshot.Volume = number(rawVolume)
	snapshot.AfterMarketQuote.Volume = number(rawVolume)
	tick, err := convertSnapshot(snapshot, expected, afterMarketNow)
	if err != nil {
		t.Fatalf("convertSnapshot: %v", err)
	}
	if !tick.Volume.Equal(want) || tick.AfterMarket == nil ||
		tick.AfterMarket.Volume == nil || !tick.AfterMarket.Volume.Equal(want) {
		t.Fatalf("snapshot volume = %s, after-market = %#v", tick.Volume, tick.AfterMarket)
	}
	serialized := marketdata.SnapshotJSON(tick)
	extended := serialized["extended"].(map[string]any)
	afterMarket := extended["afterMarket"].(map[string]any)
	if serialized["volume"] != rawVolume || afterMarket["volume"] != rawVolume {
		t.Fatalf("serialized volume = %#v", serialized)
	}

	candles := validRemoteCandles()
	candles.Candles[0].Volume = number(rawVolume)
	response, err := convertCandles(candles, expected, "1d", 10, testNow)
	if err != nil {
		t.Fatalf("convertCandles: %v", err)
	}
	converted := response["candles"].([]map[string]any)
	if converted[0]["volume"] != rawVolume {
		t.Fatalf("candle volume = %#v, want %s", converted[0]["volume"], rawVolume)
	}
}

func TestSnapshotConversionKeepsOriginalCloseForNonUSClosedMarkets(t *testing.T) {
	response := validSnapshot()
	response.Market = "HK"
	response.Symbol = "00700"
	response.InstrumentID = "HK.00700"
	response.PreviousClosePrice = number("90")
	response.LastClosePrice = number("89")
	response.RegularQuote = &remoteSnapshotQuote{Price: number("100")}

	tick, err := convertSnapshot(
		response,
		normalizedInstrument{market: "HK", symbol: "00700", id: "HK.00700"},
		testNow,
	)
	if err != nil {
		t.Fatalf("convertSnapshot: %v", err)
	}
	if tick.PreviousClosePrice == nil || !tick.PreviousClosePrice.Equal(decimal.RequireFromString("90")) ||
		tick.LastClosePrice == nil || !tick.LastClosePrice.Equal(decimal.RequireFromString("89")) {
		t.Fatalf("non-US close prices = previous=%v last=%v", tick.PreviousClosePrice, tick.LastClosePrice)
	}
}

func TestSnapshotConversionUsesCalendarForLatestClosedSessionAfterMarket(t *testing.T) {
	response := validSnapshot()
	response.Symbol = "BABA"
	response.InstrumentID = "US.BABA"
	response.Price = number("122.25")
	response.RegularQuote = &remoteSnapshotQuote{
		Price: number("122.25"), QuoteAt: "2026-07-31T20:02:27Z",
	}
	response.AfterMarketQuote = &remoteSnapshotQuote{
		Price: number("121.80"), QuoteAt: "2026-07-31T23:59:51Z",
	}
	response.PreMarketQuote = nil
	response.QuoteAt = "2026-07-31T20:02:27Z"
	response.ObservedAt = "2026-08-01T12:00:00Z"
	expected := normalizedInstrument{market: "US", symbol: "BABA", id: "US.BABA"}

	tick, err := convertSnapshot(response, expected, testNow)
	if err != nil {
		t.Fatalf("convertSnapshot: %v", err)
	}
	if tick.Session != "closed" || !tick.Price.Equal(decimal.RequireFromString("122.25")) ||
		tick.AfterMarket == nil || !tick.AfterMarket.Price.Equal(decimal.RequireFromString("121.80")) ||
		tick.AfterMarket.QuoteTime != "2026-07-31T23:59:51Z" ||
		tick.AfterMarket.SessionEndAt != "2026-08-01T00:00:00Z" {
		t.Fatalf("closed-session tick = %#v", tick)
	}

	response.AfterMarketQuote.QuoteAt = "2026-07-30T23:59:51Z"
	tick, err = convertSnapshot(response, expected, testNow)
	if err != nil || tick.AfterMarket != nil {
		t.Fatalf("stale after-market quote = %#v, err=%v", tick.AfterMarket, err)
	}

	response.AfterMarketQuote.QuoteAt = "2026-08-01T00:00:01Z"
	tick, err = convertSnapshot(response, expected, testNow)
	if err != nil || tick.AfterMarket != nil {
		t.Fatalf("out-of-window after-market quote = %#v, err=%v", tick.AfterMarket, err)
	}

	response.AfterMarketQuote.QuoteAt = ""
	tick, err = convertSnapshot(response, expected, testNow)
	if err != nil || tick.AfterMarket != nil {
		t.Fatalf("missing-time after-market quote = %#v, err=%v", tick.AfterMarket, err)
	}
}

func TestSnapshotConversionRejectsContractViolations(t *testing.T) {
	expected := normalizedInstrument{market: "US", symbol: "AAPL", id: "US.AAPL"}
	tests := []func(*remoteSnapshot){
		func(value *remoteSnapshot) { value.InstrumentID = "US.MSFT"; value.Symbol = "MSFT" },
		func(value *remoteSnapshot) { value.Price = nil },
		func(value *remoteSnapshot) { value.Price = number("-1") },
		func(value *remoteSnapshot) { value.Bid = number("-1") },
		func(value *remoteSnapshot) { value.Ask = number("-1") },
		func(value *remoteSnapshot) { value.Volume = number("-1") },
		func(value *remoteSnapshot) { value.Turnover = number("-1") },
		func(value *remoteSnapshot) { value.QuoteAt = "not-time" },
		func(value *remoteSnapshot) { value.ObservedAt = "not-time" },
	}
	for index, mutate := range tests {
		response := validSnapshot()
		response.ObservedAt = "2026-07-29T14:45:00Z"
		mutate(&response)
		if _, err := convertSnapshot(response, expected, testNow); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
}

func TestCandleConversionBuildsNeutralResponseAndRejectsDrift(t *testing.T) {
	expected := normalizedInstrument{market: "US", symbol: "AAPL", id: "US.AAPL"}
	response := validRemoteCandles()
	result, err := convertCandles(response, expected, "1d", 10, testNow)
	if err != nil {
		t.Fatalf("convertCandles: %v", err)
	}
	candles := result["candles"].([]map[string]any)
	meta := result["meta"].(map[string]any)
	if len(candles) != 1 || candles[0]["close"] != "101.5" || candles[0]["session"] != nil ||
		meta["source"] != sourceID || meta["extendedHours"] != false {
		t.Fatalf("converted candles = %#v", result)
	}
	if _, exists := meta["session"]; exists {
		t.Fatalf("session metadata should be omitted: %#v", meta)
	}

	response = validRemoteCandles()
	response.Period = "1m"
	response.Candles[0].At = "2026-07-29T12:00:00Z"
	result, err = convertCandles(response, expected, "1m", 10, testNow)
	if err != nil {
		t.Fatalf("convertCandles pre-market: %v", err)
	}
	candles = result["candles"].([]map[string]any)
	if candles[0]["session"] != "pre" || result["meta"].(map[string]any)["session"] != "all" {
		t.Fatalf("pre-market candle normalization = %#v", result)
	}

	tests := []func(*remoteCandles){
		func(value *remoteCandles) { value.InstrumentID = "US.MSFT"; value.Symbol = "MSFT" },
		func(value *remoteCandles) { value.Period = "1h" },
		func(value *remoteCandles) { value.TotalReturned = 2 },
		func(value *remoteCandles) { value.Candles[0].At = "bad" },
		func(value *remoteCandles) { value.Candles[0].Open = nil },
		func(value *remoteCandles) { value.Candles[0].High = number("-2") },
		func(value *remoteCandles) { value.Candles[0].Volume = number("-1") },
	}
	for index, mutate := range tests {
		value := validRemoteCandles()
		mutate(&value)
		if _, err := convertCandles(value, expected, "1d", 10, testNow); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
}

func TestCandleConversionUsesEarlyCloseCalendarAndDropsClosedBars(t *testing.T) {
	expected := normalizedInstrument{market: "US", symbol: "AAPL", id: "US.AAPL"}
	response := remoteCandles{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Period: "1m",
		ExtendedHours: true, TotalReturned: 3, Source: sourceID,
		Candles: []remoteCandle{
			{At: "2026-11-27T17:59:00Z", Open: number("100"), High: number("101"), Low: number("99"), Close: number("100"), Volume: number("10")},
			{At: "2026-11-27T21:59:00Z", Open: number("101"), High: number("102"), Low: number("100"), Close: number("101"), Volume: number("11")},
			{At: "2026-11-27T22:01:00Z", Open: number("102"), High: number("103"), Low: number("101"), Close: number("102"), Volume: number("12")},
		},
	}

	result, err := convertCandles(response, expected, "1m", 10, testNow)
	if err != nil {
		t.Fatalf("convertCandles: %v", err)
	}
	candles := result["candles"].([]map[string]any)
	if len(candles) != 2 || candles[0]["session"] != "regular" || candles[1]["session"] != "after" {
		t.Fatalf("early-close candles = %#v", candles)
	}
}

func TestMarketAndCandidateConversionRejectsMalformedProviderData(t *testing.T) {
	if _, err := convertMarkets(nil); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("empty markets error = %v", err)
	}
	valid := remoteMarketProfile{
		Code: "US", ResolvedMarket: "US", PreferredPrefix: "US",
		DisplayName: "United States", QuoteCurrency: "USD", Timezone: "America/New_York",
		RegularSessions: []remoteTradingWindow{{StartMinute: 570, EndMinute: 960}},
		Precision:       remotePrecision{Price: 2, Quote: 2}, TickSize: json.Number("0.01"),
	}
	for index, mutate := range []func(*remoteMarketProfile){
		func(value *remoteMarketProfile) { value.Code = "HK" },
		func(value *remoteMarketProfile) { value.ResolvedMarket = "HK" },
		func(value *remoteMarketProfile) { value.RegularSessions[0].EndMinute = 500 },
		func(value *remoteMarketProfile) { value.TickSize = json.Number("0") },
	} {
		value := valid
		value.RegularSessions = append([]remoteTradingWindow(nil), valid.RegularSessions...)
		mutate(&value)
		if _, err := convertMarket(value); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("market case %d error = %v", index, err)
		}
	}
	for _, profile := range []remoteMarketProfile{
		{Code: "HK", ResolvedMarket: "HK", PreferredPrefix: "HK", QuoteCurrency: "HKD", Timezone: "Asia/Hong_Kong", TickSize: json.Number("0.001")},
		{Code: "SH", ResolvedMarket: "CN", PreferredPrefix: "SH", QuoteCurrency: "CNY", Timezone: "Asia/Shanghai", TickSize: json.Number("0.01")},
		{Code: "SZ", ResolvedMarket: "CN", PreferredPrefix: "SZ", QuoteCurrency: "CNY", Timezone: "Asia/Shanghai", TickSize: json.Number("0.01")},
	} {
		converted, err := convertMarket(profile)
		if err != nil || converted["code"] != profile.Code || converted["preferredPrefix"] != profile.PreferredPrefix {
			t.Fatalf("market profile %q conversion = %#v, err=%v", profile.Code, converted, err)
		}
	}

	entry := remoteInstrument{
		Market: "US", ResolvedMarket: "US", InstrumentID: "US.AAPL",
		Code: "AAPL", Symbol: "AAPL", Selectable: false,
	}
	candidates, err := convertCandidates([]remoteInstrument{entry})
	if err != nil || len(candidates) != 1 || candidates[0].UnavailableReason == "" ||
		candidates[0].Source != sourceID {
		t.Fatalf("convertCandidates = %#v, err=%v", candidates, err)
	}
	for index, mutate := range []func(*remoteInstrument){
		func(value *remoteInstrument) { value.InstrumentID = "broken" },
		func(value *remoteInstrument) { value.Code = "MSFT" },
		func(value *remoteInstrument) { value.ResolvedMarket = "HK" },
	} {
		value := entry
		mutate(&value)
		if _, err := convertCandidates([]remoteInstrument{value}); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("candidate case %d error = %v", index, err)
		}
	}
	china, err := convertCandidates([]remoteInstrument{{
		Market: "SH", ResolvedMarket: "CN", InstrumentID: "SH.600519", Code: "600519", Symbol: "600519", Selectable: true,
	}})
	if err != nil || len(china) != 1 || china[0].Market != "SH" || china[0].InstrumentID != "SH.600519" {
		t.Fatalf("China candidate conversion = %#v, err=%v", china, err)
	}
}

func TestSecurityConversionValidatesIdentityAndDefaultsSource(t *testing.T) {
	expected := normalizedInstrument{market: "US", symbol: "AAPL", id: "US.AAPL"}
	response := remoteSecurity{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL",
		Name: "Apple", SecurityType: "EQUITY", MarketCap: number("3000000000000"),
		DividendYield: number("0.004"),
	}
	result, err := convertSecurity(response, expected, testNow)
	if err != nil {
		t.Fatalf("convertSecurity: %v", err)
	}
	security := result["security"].(map[string]any)
	meta := result["meta"].(map[string]any)
	dividendYield, ok := security["dividendYield"].(*float64)
	if security["instrumentId"] != "US.AAPL" || security["marketCap"] == nil || !ok || dividendYield == nil ||
		*dividendYield != 0.4 || meta["source"] != sourceID {
		t.Fatalf("converted security = %#v", result)
	}
	response.InstrumentID = "US.MSFT"
	response.Symbol = "MSFT"
	if _, err := convertSecurity(response, expected, testNow); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("mismatched security error = %v", err)
	}
}

func TestNormalizationAndNumericHelpersCoverAliasesAndBoundaries(t *testing.T) {
	identity, err := normalizeIdentity("AMEX", "spy", "")
	if err != nil || identity.id != "US.SPY" {
		t.Fatalf("normalizeIdentity alias = %#v, err=%v", identity, err)
	}
	identity, err = normalizeIdentity("", "", "NYSE:BRK.B")
	if err != nil || identity.symbol != "BRK.B" || identity.id != "US.BRK.B" {
		t.Fatalf("normalizeIdentity qualified = %#v, err=%v", identity, err)
	}
	identity, err = normalizeIdentity("HK", "700", "")
	if err != nil || identity.market != "HK" || identity.symbol != "00700" || identity.id != "HK.00700" {
		t.Fatalf("HK normalizeIdentity = %#v, err=%v", identity, err)
	}
	identity, err = normalizeIdentity("CN", "SH.600519", "")
	if err != nil || identity.market != "SH" || identity.id != "SH.600519" {
		t.Fatalf("CN qualified normalizeIdentity = %#v, err=%v", identity, err)
	}
	identity, err = normalizeIdentity("", "600519.SS", "")
	if err != nil || identity.market != "SH" || identity.id != "SH.600519" {
		t.Fatalf("Yahoo suffix normalizeIdentity = %#v, err=%v", identity, err)
	}
	if _, err := normalizeIdentity("CN", "600519", ""); err == nil {
		t.Fatal("CN bare code was accepted")
	}
	if canonicalQualifiedSymbol("plain") != "PLAIN" || normalizeLimit(0, 20, 100) != 20 ||
		normalizeLimit(200, 20, 100) != 100 || normalizeLimit(5, 20, 100) != 5 {
		t.Fatal("normalization helpers returned unexpected values")
	}

	if value, err := nonNegativeDecimal("turnover", nil); err != nil || !value.IsZero() {
		t.Fatalf("nil nonNegativeDecimal = %s, err=%v", value, err)
	}
	if value, err := nonNegativeFloat("volume", nil); err != nil || value != 0 {
		t.Fatalf("nil nonNegativeFloat = %f, err=%v", value, err)
	}
	if optionalDecimal(nil) != nil || optionalDecimal(number("bad")) != nil {
		t.Fatal("optionalDecimal invalid boundaries")
	}
	if _, err := responseTime("at", "", time.Time{}); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("empty required time error = %v", err)
	}
}
