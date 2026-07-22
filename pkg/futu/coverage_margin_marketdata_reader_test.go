package futu

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

func TestMarketDataReaderSurfacesTransportAndPayloadBoundaries(t *testing.T) {
	server, reader := coverageMarginMarketDataReader(t)
	ctx := t.Context()

	if _, err := reader.QueryQuote(ctx, broker.QuoteQuery{Symbols: []string{"BAD"}}); err == nil {
		t.Fatal("QueryQuote(invalid symbol) error = nil")
	}
	if _, err := reader.QueryQuote(ctx, broker.QuoteQuery{Symbols: []string{"HK.00700"}}); !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("QueryQuote(missing lease) error = %v", err)
	}
	if server.basicQotCallCount() != 0 {
		t.Fatalf("missing quote lease reached GetBasicQot %d times", server.basicQotCallCount())
	}
	if err := reader.exchange.SubscribeBasicQuote(ctx, "HK.00700", false); err != nil {
		t.Fatalf("SubscribeBasicQuote: %v", err)
	}
	server.setBasicQotError(1, 10, "quote entitlement denied")
	if _, err := reader.QueryQuote(ctx, broker.QuoteQuery{Symbols: []string{"HK.00700"}}); err == nil {
		t.Fatal("QueryQuote(OpenD error) error = nil")
	}
	server.clearBasicQotError()
	quote, err := reader.QueryQuote(ctx, broker.QuoteQuery{Symbols: []string{"HK.00700"}})
	if err != nil || quote == nil || len(quote.Quotes) != 1 || quote.Quotes[0].Symbol != "HK.00700" {
		t.Fatalf("QueryQuote(fallback quote) = %#v, %v; want generated HK quote", quote, err)
	}
	if err := reader.exchange.SubscribeBasicQuote(ctx, "US.NVDA", false); err != nil {
		t.Fatalf("SubscribeBasicQuote(US.NVDA): %v", err)
	}
	server.setBasicQuotes(basicQotListForSecurities([]*qotcommonpb.Security{testHKSecurity("00700")}))
	if _, err := reader.QueryQuote(ctx, broker.QuoteQuery{Symbols: []string{"HK.00700", "US.NVDA"}}); err == nil || !strings.Contains(err.Error(), "US.NVDA") {
		t.Fatalf("QueryQuote(partial payload) error = %v", err)
	}

	if _, err := reader.QueryKLines(ctx, broker.KLineQuery{Symbol: "BAD", Period: "5m"}); err == nil {
		t.Fatal("QueryKLines(invalid symbol) error = nil")
	}
	if _, err := reader.QueryKLines(ctx, broker.KLineQuery{Symbol: "HK.00700", Period: "invalid"}); err == nil {
		t.Fatal("QueryKLines(invalid period) error = nil")
	}
	server.setHistorySessionError(0, 1, "history unavailable")
	if _, err := reader.QueryKLines(ctx, broker.KLineQuery{Symbol: "HK.00700", Period: "5m"}); err == nil {
		t.Fatal("QueryKLines(OpenD error) error = nil")
	}
	server.setHistoryPages(nil)

	if _, err := reader.QuerySecurityInfo(ctx, broker.SecurityInfoQuery{Symbols: []string{"BAD"}}); err == nil {
		t.Fatal("QuerySecurityInfo(invalid symbol) error = nil")
	}
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{testTencentStaticInfo()})
	info, err := reader.QuerySecurityInfo(ctx, broker.SecurityInfoQuery{Symbols: []string{"HK.00700"}})
	if err != nil || info == nil || len(info.Securities) != 1 || info.Securities[0].Symbol != "HK.00700" {
		t.Fatalf("QuerySecurityInfo(valid entry) = %#v, %v", info, err)
	}

	server.setSearchQuotes([]*qotgetsearchquotepb.SearchQuote{{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
		Code:   new("00700"),
		Name:   new("Tencent"),
	}})
	search, err := reader.QuerySecuritySearch(ctx, broker.SecuritySearchQuery{Keyword: "AAPL"})
	if err != nil || search == nil || len(search.Entries) != 1 || search.Entries[0].Symbol != "HK.00700" {
		t.Fatalf("QuerySecuritySearch(valid entry) = %#v, %v", search, err)
	}
	server.setSearchQuoteError(1, 11, "catalog unavailable")
	if _, err := reader.QuerySecuritySearch(ctx, broker.SecuritySearchQuery{Keyword: "AAPL", Limit: 10}); err == nil {
		t.Fatal("QuerySecuritySearch(OpenD error) error = nil")
	}

	if _, err := reader.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{Symbols: []string{"BAD"}}); err == nil {
		t.Fatal("QuerySecuritySnapshot(invalid symbol) error = nil")
	}
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	snapshots, err := reader.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{Symbols: []string{"HK.00700"}})
	if err != nil || snapshots == nil || len(snapshots.Snapshots) != 1 || snapshots.Snapshots[0].Symbol != "HK.00700" {
		t.Fatalf("QuerySecuritySnapshot(valid entry) = %#v, %v", snapshots, err)
	}
	server.setSecuritySnapshots(nil)
	resetSecuritySnapshotCoordinator(reader.exchange)
	emptySnapshots, err := reader.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
		ReadQuery: broker.ReadQuery{AccountID: "account-1"},
		Symbols:   []string{"HK.00700"},
	})
	if err != nil || emptySnapshots == nil || emptySnapshots.AccountID != "account-1" || len(emptySnapshots.Snapshots) != 0 {
		t.Fatalf("QuerySecuritySnapshot(empty payload) = %#v, %v", emptySnapshots, err)
	}

	if _, err := reader.QueryOrderBook(ctx, broker.OrderBookQuery{Symbol: "BAD", Num: 5}); err == nil {
		t.Fatal("QueryOrderBook(invalid symbol) error = nil")
	}
}

func TestMarketRuleFallbacksExplainTheirSourceAndFailures(t *testing.T) {
	server, reader := coverageMarginMarketDataReader(t)
	ctx := t.Context()
	query := broker.MarketRuleQuery{Symbols: []string{"HK.00700"}}

	if _, err := reader.QueryMarketRules(ctx, broker.MarketRuleQuery{}); err == nil {
		t.Fatal("QueryMarketRules(empty symbols) error = nil")
	}

	server.setStaticInfoError(1, 1, "static metadata unavailable")
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	rules, err := reader.QueryMarketRules(ctx, query)
	if err != nil || rules == nil || len(rules.Rules) != 1 || len(rules.Warnings) != 1 {
		t.Fatalf("snapshot fallback rules = %#v, %v", rules, err)
		return
	}
	if !strings.Contains(rules.Warnings[0], "QuerySecuritySnapshot fallback") || !strings.Contains(rules.Warnings[0], "static metadata unavailable") {
		t.Fatalf("fallback warning = %q", rules.Warnings[0])
	}

	server.setStaticInfoError(1, 1, "static metadata unavailable")
	server.setSecuritySnapshots(nil)
	resetSecuritySnapshotCoordinator(reader.exchange)
	if _, err := reader.QueryMarketRules(ctx, query); err == nil || !strings.Contains(err.Error(), "returned no market rules") {
		t.Fatalf("failed fallback error = %v", err)
	}

	server.setStaticInfos(nil)
	server.setSecuritySnapshots(nil)
	resetSecuritySnapshotCoordinator(reader.exchange)
	if _, err := reader.QueryMarketRules(ctx, query); err == nil || !strings.Contains(err.Error(), "returned no market rules") {
		t.Fatalf("empty primary and fallback error = %v", err)
	}
}

func TestMarketDataRuleHelpersRejectIncompleteBrokerPayloads(t *testing.T) {
	lot := int32(100)
	infoRules := marketRulesFromSecurityInfo(&broker.SecurityInfoSnapshot{AccountID: "acc", Securities: []broker.SecurityInfoItem{
		{Symbol: " ", LotSize: &lot},
		{Symbol: "HK.00700"},
		{Symbol: "HK.00700", LotSize: &lot},
	}})
	if infoRules.AccountID != "acc" || len(infoRules.Rules) != 1 || infoRules.Rules[0].LotSize == nil || *infoRules.Rules[0].LotSize != lot {
		t.Fatalf("marketRulesFromSecurityInfo = %#v", infoRules)
	}
	if rules := marketRulesFromSecurityInfo(nil); len(rules.Rules) != 0 {
		t.Fatalf("marketRulesFromSecurityInfo(nil) = %#v", rules)
	}

	snapshotRules := marketRulesFromSecuritySnapshot(&broker.SecuritySnapshotResult{AccountID: "acc", Snapshots: []broker.SecuritySnapshotItem{
		{Symbol: "", LotSize: &lot},
		{Symbol: "HK.00700"},
		{Symbol: "HK.00700", LotSize: &lot},
	}})
	if snapshotRules.AccountID != "acc" || len(snapshotRules.Rules) != 1 || snapshotRules.Rules[0].LotSize == nil || *snapshotRules.Rules[0].LotSize != lot {
		t.Fatalf("marketRulesFromSecuritySnapshot = %#v", snapshotRules)
	}
	if rules := marketRulesFromSecuritySnapshot(nil); len(rules.Rules) != 0 {
		t.Fatalf("marketRulesFromSecuritySnapshot(nil) = %#v", rules)
	}

	for _, tc := range []struct {
		input string
		want  string
	}{
		{input: "cnsh", want: "SH"},
		{input: "cnsz", want: "SZ"},
		{input: "hkfuture", want: "HK_FUTURE"},
		{input: "cc", want: "CRYPTO"},
		{input: "us", want: "US"},
	} {
		if got := canonicalSearchQuoteMarketPrefix(tc.input); got != tc.want {
			t.Fatalf("canonicalSearchQuoteMarketPrefix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func coverageMarginMarketDataReader(t *testing.T) (*quoteOpenDServer, *futuMarketDataReader) {
	t.Helper()
	server := startQuoteOpenDServer(t)
	t.Cleanup(server.stop)
	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	t.Cleanup(func() { jftradeCheckTestError(t, exchange.Close()) })
	return server, &futuMarketDataReader{exchange: exchange}
}
