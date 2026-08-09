package marketdataapp

import (
	"context"
	"fmt"
	"testing"
	"time"

	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/shopspring/decimal"
)

func testMarketDataProtoKLine(at time.Time, open, high, low, close float64, volume int64) fututestkit.KLine {
	return fututestkit.KLine{At: at, Open: open, High: high, Low: low, Close: close, Volume: volume}
}

func TestMarketCandlesResponseUsesExchangeResolvedSessionsForUSIntraday(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	quoteServer.SetHistoryPagesBySession(map[string][][]fututestkit.KLine{
		"RTH": {
			{testMarketDataProtoKLine(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC), 110, 111, 109, 110.5, 1000)},
		},
		"ETH": {
			{testMarketDataProtoKLine(time.Date(2026, time.May, 20, 21, 0, 0, 0, time.UTC), 120, 121, 119, 120.5, 1000)},
		},
		"ALL": {
			{testMarketDataProtoKLine(time.Date(2026, time.May, 20, 2, 0, 0, 0, time.UTC), 90, 91, 89, 90.5, 1000)},
		},
	})

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	start := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.May, 20, 23, 0, 0, 0, time.UTC)
	response, err := harness.Adapters.CandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/US/NVDA",
		map[string][]string{
			"period":   {"1m"},
			"limit":    {"3"},
			"fromTime": {start.Format(time.RFC3339Nano)},
			"toTime":   {end.Format(time.RFC3339Nano)},
		},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	candles, ok := response["candles"].([]map[string]any)
	if !ok {
		t.Fatalf("candles payload type = %T", response["candles"])
	}
	if len(candles) != 3 {
		t.Fatalf("len(candles) = %d, want 3", len(candles))
	}
	sessionsByOpen := make(map[string]string, len(candles))
	for _, candle := range candles {
		open, ok := candle["open"].(string)
		if !ok {
			t.Fatalf("open payload type = %T", candle["open"])
		}
		session, ok := candle["session"].(string)
		if !ok {
			t.Fatalf("session payload type = %T", candle["session"])
		}
		sessionsByOpen[open] = session
	}
	if got := sessionsByOpen["90"]; got != "overnight" {
		t.Fatalf("overnight candle session = %q, want overnight", got)
	}
	if got := sessionsByOpen["110"]; got != "regular" {
		t.Fatalf("RTH-routed candle session = %q, want regular", got)
	}
	if got := sessionsByOpen["120"]; got != "after" {
		t.Fatalf("ETH-routed candle session = %q, want after", got)
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["session"]; got != "all" {
		t.Fatalf("meta session = %v, want all", got)
	}
	if got := meta["extendedHours"]; got != true {
		t.Fatalf("extendedHours = %v, want true", got)
	}
}

func TestMarketCandlesResponseOmitsSessionMetadataForDailyCandles(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	labelAt := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	quoteServer.SetHistoryPages([][]fututestkit.KLine{{
		testMarketDataProtoKLine(labelAt, 100, 101, 99, 100.5, 1000),
	}})

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	response, err := harness.Adapters.CandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/US/NVDA",
		map[string][]string{
			"period":   {"1d"},
			"limit":    {"1"},
			"fromTime": {labelAt.Add(-24 * time.Hour).Format(time.RFC3339Nano)},
			"toTime":   {labelAt.Add(24 * time.Hour).Format(time.RFC3339Nano)},
		},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	candles, ok := response["candles"].([]map[string]any)
	if !ok {
		t.Fatalf("candles payload type = %T", response["candles"])
	}
	if len(candles) != 1 {
		t.Fatalf("len(candles) = %d, want 1", len(candles))
	}
	if _, exists := candles[0]["session"]; exists {
		t.Fatalf("expected daily candle to omit session, got %v", candles[0]["session"])
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if _, exists := meta["session"]; exists {
		t.Fatalf("expected daily candle meta to omit session, got %v", meta["session"])
	}
	if got := meta["extendedHours"]; got != false {
		t.Fatalf("extendedHours = %v, want false", got)
	}
}

func TestMarketCandlesResponseRejectsInvalidSessionsBeforeFutuAccess(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()
	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	_, err := harness.Adapters.CandlesResponse(
		t.Context(), "/api/v1/market-data/candles/US/NVDA",
		map[string][]string{"period": {"1m"}, "sessions": {"regular,unknown"}},
	)
	if err == nil {
		t.Fatal("invalid sessions error = nil")
	}
}

func TestMarketCandlesResponseClassifiesUnknownUSSessionAsDataError(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()
	quoteServer.SetHistoryPages([][]fututestkit.KLine{{
		testMarketDataProtoKLine(time.Date(2026, time.May, 24, 12, 0, 0, 0, time.UTC), 100, 101, 99, 100.5, 1000),
	}})
	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	_, err := harness.Adapters.CandlesResponse(
		t.Context(), "/api/v1/market-data/candles/US/NVDA",
		map[string][]string{"period": {"1m"}, "limit": {"1"}},
	)
	if err == nil {
		t.Fatal("unknown session error = nil")
	}
}

func decimalPointer(v *float64) *decimal.Decimal {
	if v == nil {
		return nil
	}
	return new(decimal.NewFromFloat(*v))
}

func assertSnapshotResponse(t *testing.T, response map[string]any, instrumentID string, fromCache bool, source string) {
	t.Helper()
	request, ok := response["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", response["request"])
	}
	if got := request["instrumentId"]; got != instrumentID {
		t.Fatalf("instrumentId = %v, want %s", got, instrumentID)
	}
	snapshot, ok := response["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot payload type = %T", response["snapshot"])
	}
	if got := snapshot["price"]; got == nil {
		t.Fatal("expected snapshot price")
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["fromCache"]; got != fromCache {
		t.Fatalf("fromCache = %v, want %v", got, fromCache)
	}
	if got := meta["source"]; got != source {
		t.Fatalf("source = %v, want %s", got, source)
	}
	if got := meta["instrumentId"]; got != instrumentID {
		t.Fatalf("meta instrumentId = %v, want %s", got, instrumentID)
	}
}

func assertTickCandlesResponse(t *testing.T, response map[string]any, instrumentID string, fromCache bool, wantCount int) {
	t.Helper()
	request, ok := response["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", response["request"])
	}
	instrument, ok := request["instrument"].(map[string]any)
	if !ok {
		t.Fatalf("instrument payload type = %T", request["instrument"])
	}
	if got := instrument["instrumentId"]; got != instrumentID {
		t.Fatalf("instrumentId = %v, want %s", got, instrumentID)
	}
	totalReturned, ok := response["totalReturned"].(int)
	if !ok {
		t.Fatalf("totalReturned payload type = %T", response["totalReturned"])
	}
	if totalReturned != wantCount {
		t.Fatalf("totalReturned = %d, want %d", totalReturned, wantCount)
	}
	candles, ok := response["candles"].([]map[string]any)
	if !ok {
		t.Fatalf("candles payload type = %T", response["candles"])
	}
	if len(candles) != wantCount {
		t.Fatalf("len(candles) = %d, want %d", len(candles), wantCount)
	}
	if wantCount > 0 {
		if _, ok := candles[0]["open"].(string); !ok {
			t.Fatalf("tick candle open payload type = %T", candles[0]["open"])
		}
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["fromCache"]; got != fromCache {
		t.Fatalf("fromCache = %v, want %v", got, fromCache)
	}
}

func TestMarketSnapshotResponseUsesFreshCache(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()
	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())

	instrumentID := "HK.00700"
	now := time.Now().UTC().Truncate(time.Second)
	harness.Service.Seed(mdsrv.Tick{
		InstrumentID:       instrumentID,
		Market:             "HK",
		Symbol:             "00700",
		Price:              decimal.RequireFromString("321.4"),
		Bid:                decimal.RequireFromString("321.3"),
		Ask:                decimal.RequireFromString("321.5"),
		PreviousClosePrice: decimalPointer(new(318.9)),
		Volume:             decimal.NewFromInt(1282100),
		Turnover:           decimal.RequireFromString("411020000"),
		QuoteAt:            now.Format(time.RFC3339Nano),
		ObservedAt:         now.Format(time.RFC3339Nano),
		Source:             "bbgo:futu:stream",
		Session:            "regular",
	})

	response, err := harness.Adapters.SnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, instrumentID, true, "bbgo:futu:stream")
	if got := response["snapshot"].(map[string]any)["at"]; got != now.Format(time.RFC3339Nano) {
		t.Fatalf("snapshot at = %v", got)
	}
	if got := response["snapshot"].(map[string]any)["turnover"]; got != "411020000" {
		t.Fatalf("snapshot turnover = %v", got)
	}
	if got := response["snapshot"].(map[string]any)["volume"]; got != "1282100" {
		t.Fatalf("snapshot volume = %#v, want decimal string", got)
	}
}

func acquireMarketDataTestSubscription(t *testing.T, harness *marketDataTestHarness, ref mdsrv.InstrumentRef) {
	t.Helper()
	// These tests assert the synchronous cache-miss request count. Keep the
	// independently tested fallback collector from racing that direct read.
	harness.Service.ResetCollector()
	_, err := harness.Service.AcquireSubscription(t.Context(), "market-snapshot-test", []mdsrv.InstrumentRef{ref})
	if err != nil {
		t.Fatalf("AcquireSubscription(%#v): %v", ref, err)
	}
	if err := harness.Runtime.ReconcileSubscriptions(t.Context(), []mdsrv.InstrumentRef{ref}); err != nil {
		t.Fatalf("ReconcileSubscriptions(%#v): %v", ref, err)
	}
	if state := harness.Runtime.SubscriptionState(); state["ownActiveCount"] != 1 {
		t.Fatalf("ReconcileSubscriptions(%#v) did not establish the physical lease: %#v", ref, state)
	}
}

func TestMarketSnapshotResponseQueriesQuoteSnapshotOnCacheMiss(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	acquireMarketDataTestSubscription(t, harness, mdsrv.InstrumentRef{Channel: "SNAPSHOT", Market: "HK", Symbol: "00700"})
	response, err := harness.Adapters.SnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, "HK.00700", false, "bbgo:futu")
	if got := quoteServer.BasicQuoteCallCount(); got != 1 {
		t.Fatalf("expected one GetBasicQot call, got %d", got)
	}
	if got := response["snapshot"].(map[string]any)["price"]; got != "321.4" {
		t.Fatalf("snapshot price = %v", got)
	}
}

func TestMarketSnapshotResponseForceRefreshBypassesCache(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	acquireMarketDataTestSubscription(t, harness, mdsrv.InstrumentRef{Channel: "SNAPSHOT", Market: "HK", Symbol: "00700"})
	harness.Service.Seed(mdsrv.Tick{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("999.9"),
		Bid:          decimal.RequireFromString("999.8"),
		Ask:          decimal.RequireFromString("1000.0"),
		Volume:       decimal.NewFromInt(1),
		QuoteAt:      time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
		ObservedAt:   time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
		Source:       "bbgo:futu:stream",
		Session:      "regular",
	})

	response, err := harness.Adapters.SnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{"refresh": {"true"}},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, "HK.00700", false, "bbgo:futu")
	if got := quoteServer.BasicQuoteCallCount(); got != 1 {
		t.Fatalf("expected one forced GetBasicQot call, got %d", got)
	}
	if got := response["snapshot"].(map[string]any)["price"]; got != "321.4" {
		t.Fatalf("forced refresh snapshot price = %v", got)
	}
}

func TestMarketSnapshotResponseRejectsInvalidRefreshQuery(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()
	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())

	if _, err := harness.Adapters.SnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{"refresh": {"sometimes"}},
	); err == nil {
		t.Fatal("marketSnapshotResponse invalid refresh error = nil")
	}
}

func TestMarketCandlesTickResponseUsesFreshCache(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()
	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())

	instrumentID := "HK.00700"
	now := time.Now().UTC().Truncate(time.Second)
	harness.Service.Seed(mdsrv.Tick{
		InstrumentID: instrumentID,
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		Bid:          decimal.RequireFromString("321.3"),
		Ask:          decimal.RequireFromString("321.5"),
		Volume:       decimal.NewFromInt(1282100),
		QuoteAt:      now.Format(time.RFC3339Nano),
		ObservedAt:   now.Format(time.RFC3339Nano),
		Source:       "bbgo:futu:stream",
		Session:      "regular",
	})

	response, err := harness.Adapters.CandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	assertTickCandlesResponse(t, response, instrumentID, true, 1)
	if got := response["candles"].([]map[string]any)[0]["at"]; got != now.Format(time.RFC3339Nano) {
		t.Fatalf("tick candle at = %v", got)
	}
}

func TestMarketCandlesTickResponseQueriesTickerOnCacheMiss(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	acquireMarketDataTestSubscription(t, harness, mdsrv.InstrumentRef{Channel: "TICK", Market: "HK", Symbol: "00700"})
	response, err := harness.Adapters.CandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	assertTickCandlesResponse(t, response, "HK.00700", false, 1)
	if got := quoteServer.BasicQuoteCallCount(); got != 1 {
		t.Fatalf("expected one GetBasicQot call, got %d", got)
	}
	if got := response["candles"].([]map[string]any)[0]["period"]; got != "tick" {
		t.Fatalf("tick candle period = %v", got)
	}
}

func TestMarketCandlesTickResponseFallsBackToCachedCandlesOnTickerError(t *testing.T) {
	harness := newMarketDataQuoteHarness(t, "127.0.0.1:1")

	instrumentID := "HK.00700"
	observedAt := time.Now().UTC().Add(-1 * time.Minute).Truncate(time.Second)
	harness.Service.Seed(mdsrv.Tick{
		InstrumentID: instrumentID,
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		Bid:          decimal.RequireFromString("321.3"),
		Ask:          decimal.RequireFromString("321.5"),
		Volume:       decimal.NewFromInt(1282100),
		QuoteAt:      observedAt.Format(time.RFC3339Nano),
		ObservedAt:   observedAt.Format(time.RFC3339Nano),
		Source:       "bbgo:futu:fallback",
		Session:      "regular",
	})

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	response, err := harness.Adapters.CandlesResponse(
		ctx,
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse fallback: %v", err)
	}

	assertTickCandlesResponse(t, response, instrumentID, true, 1)
	if got := response["candles"].([]map[string]any)[0]["at"]; got != observedAt.Format(time.RFC3339Nano) {
		t.Fatalf("fallback tick candle at = %v", got)
	}
}

func TestMarketSecurityDetailsResponseQueriesSecuritySnapshot(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	response, err := harness.Adapters.SecurityDetailsResponse(
		t.Context(),
		"/api/v1/market-data/securities/HK/00700",
	)
	if err != nil {
		t.Fatalf("marketSecurityDetailsResponse: %v", err)
	}

	request, ok := response["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", response["request"])
	}
	if got := request["instrumentId"]; got != "HK.00700" {
		t.Fatalf("instrumentId = %v", got)
	}
	security, ok := response["security"].(map[string]any)
	if !ok {
		t.Fatalf("security payload type = %T", response["security"])
	}
	if got := security["name"]; got != "Tencent Holdings" {
		t.Fatalf("security name = %v", got)
	}
	if got := security["exchangeType"]; got != "HK_HKEX" {
		t.Fatalf("exchangeType = %v", got)
	}
	if got := security["currentPrice"]; got != "321.4" {
		t.Fatalf("currentPrice = %v", got)
	}
	equity, ok := security["equity"].(map[string]any)
	if !ok {
		t.Fatalf("equity payload type = %T", security["equity"])
	}
	if got := equity["peRate"]; got != "16.7" {
		t.Fatalf("peRate = %v", got)
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["fromCache"]; got != false {
		t.Fatalf("fromCache = %v", got)
	}
	if got := quoteServer.SecuritySnapshotCallCount(); got != 1 {
		t.Fatalf("expected one GetSecuritySnapshot call, got %d", got)
	}
	if got := quoteServer.StaticInfoCallCount(); got != 1 {
		t.Fatalf("expected one GetStaticInfo call, got %d", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesWarrantBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/HK/21164")
	warrant := assertSecurityTypedBlock(t, security, "warrant")
	assertSecurityProductIdentity(t, security, "cbbc", "derivatives")
	if got := security["securityType"]; got != "Warrant" {
		t.Fatalf("securityType = %v", got)
	}
	if got := warrant["warrantType"]; got != "Bull" {
		t.Fatalf("warrantType = %v", got)
	}
	owner, ok := warrant["owner"].(map[string]any)
	if !ok {
		t.Fatalf("owner payload type = %T", warrant["owner"])
	}
	if got := owner["instrumentId"]; got != "HK.00700" {
		t.Fatalf("owner instrumentId = %v", got)
	}
	if got := warrant["issuerCode"]; got != "SG" {
		t.Fatalf("issuerCode = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesOptionBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/US/AAPL250117C00200000")
	option := assertSecurityTypedBlock(t, security, "option")
	assertSecurityProductIdentity(t, security, "option", "derivatives")
	if got := security["securityType"]; got != "Drvt" {
		t.Fatalf("securityType = %v", got)
	}
	if got := option["optionType"]; got != "Call" {
		t.Fatalf("optionType = %v", got)
	}
	owner, ok := option["owner"].(map[string]any)
	if !ok {
		t.Fatalf("owner payload type = %T", option["owner"])
	}
	if got := owner["instrumentId"]; got != "US.AAPL" {
		t.Fatalf("owner instrumentId = %v", got)
	}
	if got := option["expiryDateDistance"]; got != int32(45) {
		t.Fatalf("expiryDateDistance = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesFutureBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/HK/HSIMAIN")
	future := assertSecurityTypedBlock(t, security, "future")
	assertSecurityProductIdentity(t, security, "future", "derivatives")
	if got := security["securityType"]; got != "Future" {
		t.Fatalf("securityType = %v", got)
	}
	if got := future["isMainContract"]; got != true {
		t.Fatalf("isMainContract = %v", got)
	}
	if got := future["position"]; got != int32(182233) {
		t.Fatalf("position = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesTrustBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/US/SPY")
	trust := assertSecurityTypedBlock(t, security, "trust")
	assertSecurityProductIdentity(t, security, "fund", "securities")
	if got := security["securityType"]; got != "Trust" {
		t.Fatalf("securityType = %v", got)
	}
	if got := trust["assetClass"]; got != "Stock" {
		t.Fatalf("assetClass = %v", got)
	}
	if got := trust["aum"]; got != "580000000000" {
		t.Fatalf("aum = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesIndexBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/HK/HSI")
	index := assertSecurityTypedBlock(t, security, "index")
	assertSecurityProductIdentity(t, security, "index", "securities")
	if got := security["securityType"]; got != "Index" {
		t.Fatalf("securityType = %v", got)
	}
	if got := index["raiseCount"]; got != int32(58) {
		t.Fatalf("raiseCount = %v", got)
	}
	if got := index["fallCount"]; got != int32(21) {
		t.Fatalf("fallCount = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesPlateBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/HK/TECH")
	plate := assertSecurityTypedBlock(t, security, "plate")
	assertSecurityProductIdentity(t, security, "plate", "securities")
	if got := security["securityType"]; got != "Plate" {
		t.Fatalf("securityType = %v", got)
	}
	if got := plate["raiseCount"]; got != int32(42) {
		t.Fatalf("raiseCount = %v", got)
	}
	if got := plate["equalCount"]; got != int32(5) {
		t.Fatalf("equalCount = %v", got)
	}
}

func assertSecurityProductIdentity(
	t *testing.T,
	security map[string]any,
	productClass string,
	marketSegment string,
) {
	t.Helper()
	if got := fmt.Sprint(security["productClass"]); got != productClass {
		t.Fatalf("productClass = %v, want %s", got, productClass)
	}
	if got := fmt.Sprint(security["marketSegment"]); got != marketSegment {
		t.Fatalf("marketSegment = %v, want %s", got, marketSegment)
	}
}

func marketSecurityDetailsResponseForPath(t *testing.T, path string) map[string]any {
	t.Helper()
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	response, err := harness.Adapters.SecurityDetailsResponse(t.Context(), path)
	if err != nil {
		t.Fatalf("marketSecurityDetailsResponse(%s): %v", path, err)
	}
	security, ok := response["security"].(map[string]any)
	if !ok {
		t.Fatalf("security payload type = %T", response["security"])
	}
	return security
}

func assertSecurityTypedBlock(t *testing.T, security map[string]any, key string) map[string]any {
	t.Helper()
	typed, ok := security[key].(map[string]any)
	if !ok {
		t.Fatalf("%s payload type = %T", key, security[key])
	}
	return typed
}
