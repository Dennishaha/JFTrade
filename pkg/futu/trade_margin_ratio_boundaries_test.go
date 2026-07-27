package futu

import (
	"errors"
	"testing"
	"time"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func TestMarginRatioRecoveryAndErrorClassificationBoundaries(t *testing.T) {
	if isUnknownStockError(nil) || isMarginRatioRateLimitedError(nil) {
		t.Fatal("nil margin errors must not be classified")
	}
	if !isUnknownStockError(errors.New("OpenD: unknown stock AAPL")) || !isUnknownStockError(errors.New("未知股票 00700")) {
		t.Fatal("unknown-stock errors were not classified")
	}
	if !isMarginRatioRateLimitedError(errors.New("rate limit: too high request frequency")) || !isMarginRatioRateLimitedError(errors.New("频率太高")) {
		t.Fatal("rate-limit errors were not classified")
	}

	for _, tc := range []struct {
		err  error
		code string
		ok   bool
	}{
		{err: nil},
		{err: errors.New("other broker error")},
		{err: errors.New("unknown stock")},
		{err: errors.New("unknown security \"\"")},
		{err: errors.New("unknown security 'aapl',"), code: "AAPL", ok: true},
		{err: errors.New("未知股票 (00700)"), code: "00700", ok: true},
	} {
		code, ok := extractUnknownStockCode(tc.err)
		if code != tc.code || ok != tc.ok {
			t.Fatalf("extractUnknownStockCode(%v) = %q/%v, want %q/%v", tc.err, code, ok, tc.code, tc.ok)
		}
	}

	server, exchange := coverageMarginExchange(t)
	server.setMarginRatios([]*trdgetmarginratiopb.MarginRatioInfo{{Security: testHKSecurity("00700")}})
	server.setStrictMarginRatios(true)
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}
	header := &trdcommonpb.TrdHeader{TrdEnv: new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)), AccID: new(uint64(1001)), TrdMarket: new(int32(trdcommonpb.TrdMarket_TrdMarket_HK))}
	if infos, err := marginRatioInfoListWithUnknownStockRecovery(t.Context(), client, header, nil); err != nil || len(infos) != 0 {
		t.Fatalf("empty margin-ratio lookup = %#v, %v", infos, err)
	}
	infos, err := marginRatioInfoListWithUnknownStockRecovery(t.Context(), client, header, []*qotcommonpb.Security{
		testHKSecurity("00700"),
		testHKSecurity("99999"),
	})
	if err != nil || len(infos) != 1 || infos[0].GetSecurity().GetCode() != "00700" {
		t.Fatalf("unknown-stock recovery = %#v, %v", infos, err)
	}
}

func TestMarginRatioCacheReturnsDefensiveFreshSnapshots(t *testing.T) {
	exchange := NewExchange("")
	if cached, ok := exchange.getMarginRatioCache("", time.Second); ok || cached != nil {
		t.Fatalf("empty cache key = %#v/%v", cached, ok)
	}
	exchange.marginRatioCache["stale"] = marginRatioCacheEntry{updatedAt: time.Now().UTC().Add(-time.Minute)}
	if cached, ok := exchange.getMarginRatioCache("stale", time.Second); ok || cached != nil {
		t.Fatalf("stale cache entry = %#v/%v", cached, ok)
	}

	snapshots := []BrokerMarginRatioSnapshot{{Symbol: "HK.00700"}}
	exchange.setMarginRatioCache("fresh", snapshots)
	snapshots[0].Symbol = "MUTATED"
	cached, ok := exchange.getMarginRatioCache("fresh", time.Second)
	if !ok || len(cached) != 1 || cached[0].Symbol != "HK.00700" {
		t.Fatalf("cached snapshots = %#v/%v", cached, ok)
	}
	cached[0].Symbol = "MUTATED-READ"
	again, ok := exchange.getMarginRatioCache("fresh", time.Second)
	if !ok || again[0].Symbol != "HK.00700" {
		t.Fatalf("cache exposed internal snapshot slice: %#v", again)
	}
}

func TestBrokerMarginRatioFallsBackToRecentCacheAndSurfacesInputFailures(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{testRealHKMarginAccount()})
	query := BrokerMarginRatioQuery{
		BrokerReadQuery: BrokerReadQuery{Market: "HK"},
		Symbols:         []string{"HK.00700"},
	}
	cacheKey := marginRatioCacheKey(BrokerReadQuery{TradingEnvironment: "REAL", Market: "HK"}, query.Symbols)
	exchange.marginRatioCache[cacheKey] = marginRatioCacheEntry{
		snapshots: []BrokerMarginRatioSnapshot{{Symbol: "HK.00700"}},
		updatedAt: time.Now().UTC().Add(-marginRatioCacheTTL - time.Second),
	}
	server.setMarginRatioError(1, 9, "rate limit exceeded")
	ratios, err := exchange.QueryBrokerMarginRatios(t.Context(), query)
	if err != nil || len(ratios) != 1 || ratios[0].Symbol != "HK.00700" {
		t.Fatalf("rate-limited margin ratio fallback = %#v, %v", ratios, err)
	}

	server.setMarginRatioError(1, 10, "broker service unavailable")
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}
	header := &trdcommonpb.TrdHeader{}
	if _, err := marginRatioInfoListWithUnknownStockRecovery(t.Context(), client, header, []*qotcommonpb.Security{testHKSecurity("00700")}); err == nil {
		t.Fatal("non-unknown margin-ratio error = nil")
	}

	server.setMarginRatios(nil)
	if _, err := exchange.QueryBrokerMarginRatios(t.Context(), BrokerMarginRatioQuery{Symbols: []string{"BAD"}}); err == nil {
		t.Fatal("invalid margin-ratio symbol error = nil")
	}
	server.setAccounts(nil)
	if _, err := exchange.QueryBrokerMarginRatios(t.Context(), BrokerMarginRatioQuery{Symbols: []string{"HK.00001"}}); err == nil {
		t.Fatal("missing account margin-ratio error = nil")
	}
}

func TestBasicQuoteQueriesHandleEmptyDuplicateAndInvalidRequests(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	if tickers, err := exchange.QueryTickers(t.Context()); err != nil || len(tickers) != 0 {
		t.Fatalf("QueryTickers(empty) = %#v, %v", tickers, err)
	}
	if _, err := exchange.QueryTicker(t.Context(), "BAD"); err == nil {
		t.Fatal("QueryTicker(invalid symbol) error = nil")
	}
	if err := exchange.SubscribeBasicQuote(t.Context(), "HK.00700", false); err != nil {
		t.Fatalf("SubscribeBasicQuote() error = %v", err)
	}

	tickers, err := exchange.QueryTickers(t.Context(), "HK.00700", " hk.00700 ")
	if err != nil || len(tickers) != 1 {
		t.Fatalf("QueryTickers(duplicate symbols) = %#v, %v", tickers, err)
	}

	server.setBasicQuotes(nil)
	if _, err := exchange.QueryTicker(t.Context(), "HK.00700"); err == nil {
		t.Fatal("QueryTicker(empty OpenD response) error = nil")
	}
	if _, err := exchange.QueryQuoteSnapshot(t.Context(), "HK.00700"); err == nil {
		t.Fatal("QueryQuoteSnapshot(empty OpenD response) error = nil")
	}
}

func TestMarginRatioUncachedRecoveryAndConversionBoundaries(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{testRealHKMarginAccount()})
	server.setMarginRatioError(1, 1, "broker unavailable")
	if _, err := exchange.QueryBrokerMarginRatios(t.Context(), BrokerMarginRatioQuery{Symbols: []string{"HK.00700"}}); err == nil {
		t.Fatal("uncached generic margin-ratio error = nil")
	}
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}
	server.setMarginRatioError(1, 1, "unknown stock")
	header := (resolvedTradeAccount{protoAccountID: 1001, protoTrdEnv: int32(trdcommonpb.TrdEnv_TrdEnv_Real), protoTrdMarket: int32(trdcommonpb.TrdMarket_TrdMarket_HK)}).header()
	if _, err := marginRatioInfoListWithUnknownStockRecovery(t.Context(), client, header, []*qotcommonpb.Security{testHKSecurity("00700")}); err == nil {
		t.Fatal("unknown-stock error without code = nil")
	}
	server.setMarginRatioError(1, 1, "unknown stock MISSING")
	if _, err := marginRatioInfoListWithUnknownStockRecovery(t.Context(), client, header, []*qotcommonpb.Security{testHKSecurity("00700")}); err == nil {
		t.Fatal("unmatched unknown-stock error = nil")
	}

	infos := []*trdgetmarginratiopb.MarginRatioInfo{
		nil,
		{Security: testHKSecurity("00700")},
		{Security: &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: new("AAPL")}},
	}
	snapshots := brokerMarginRatioSnapshotsFromProto(resolvedTradeAccount{}, infos)
	if len(snapshots) != 2 || snapshots[0].Symbol != "HK.00700" || snapshots[1].Symbol != "US.AAPL" {
		t.Fatalf("sorted margin-ratio snapshots = %#v", snapshots)
	}
	remaining, removed := removeUnknownMarginSecurity([]*qotcommonpb.Security{nil, testHKSecurity("00700")}, "MISSING")
	if !removed || len(remaining) != 1 {
		t.Fatalf("nil margin security removal = %#v/%v", remaining, removed)
	}
	if key := marginRatioCacheKey(BrokerReadQuery{}, []string{" ", "US.AAPL"}); key == "" {
		t.Fatal("margin cache key unexpectedly empty")
	}
	exchange.setMarginRatioCache("", snapshots)
}
