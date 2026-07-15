package futu

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestQuoteSnapshotUsesCompleteActiveExtendedBlock(t *testing.T) {
	afterHours := time.Date(2026, time.July, 15, 22, 0, 0, 0, time.UTC)
	quote := &qotcommonpb.BasicQot{
		Security:  &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: new("AAPL")},
		CurPrice:  new(200.0),
		HighPrice: new(201.0),
		LowPrice:  new(199.0),
		Volume:    new(int64(10)),
		Turnover:  new(1000.0),
		AfterMarket: &qotcommonpb.PreAfterMarketData{
			Price: new(202.0), HighPrice: new(203.0), LowPrice: new(198.0), Volume: new(int64(20)), Turnover: new(2000.0),
		},
	}
	snapshot := quoteSnapshotFromBasicQotAt(quote, "US.AAPL", afterHours)
	if snapshot.Session != market.SessionAfter || snapshot.Price.String() != "202" || snapshot.HighPrice.String() != "203" ||
		snapshot.LowPrice.String() != "198" || snapshot.Volume != 20 || snapshot.Turnover.String() != "2000" {
		t.Fatalf("complete after-hours snapshot = %#v", snapshot)
	}
	preMarket := &ExtendedMarketQuote{}
	if got := activeExtendedQuoteForSession(market.SessionPre, preMarket, nil, nil); got != preMarket {
		t.Fatalf("pre-market active quote = %#v", got)
	}
	if got := activeExtendedQuoteForSession(market.SessionOvernight, nil, nil, snapshot.AfterMarket); got != snapshot.AfterMarket {
		t.Fatalf("overnight active quote = %#v", got)
	}
	resolved := futuQuoteTimeAt(0, "not-a-time", "UNKNOWN.BAD", time.Time{})
	if resolved.IsZero() {
		t.Fatal("zero recorded time fallback remained zero")
	}
}

func TestMarketDataAdapterSkipsNilRowsAndReturnsProtocolFailures(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	reader := &futuMarketDataReader{exchange: exchange}
	quote := quoteSnapshotFromProtoList("account", []*qotcommonpb.BasicQot{nil, {Security: testHKSecurity("00700"), CurPrice: new(300.0)}})
	if len(quote.Quotes) != 1 {
		t.Fatalf("quoteSnapshotFromProtoList(nil row) = %#v", quote)
	}

	info := securityInfoSnapshotFromProtoList("account", []*qotcommonpb.SecurityStaticInfo{nil, {}})
	if len(info.Securities) != 0 {
		t.Fatalf("securityInfoSnapshotFromProtoList(nil rows) = %#v", info)
	}

	search := securitySearchSnapshotFromProtoList("account", []*qotgetsearchquotepb.SearchQuote{nil})
	if len(search.Entries) != 0 {
		t.Fatalf("securitySearchSnapshotFromProtoList(nil row) = %#v", search)
	}

	server.setStaticInfos(nil)
	server.setSecuritySnapshotError(1, 9, "snapshot failure")
	if _, err := reader.QueryMarketRules(t.Context(), broker.MarketRuleQuery{Symbols: []string{"HK.00700"}}); err == nil {
		t.Fatal("QueryMarketRules(snapshot-only failure) error = nil")
	}
	if _, err := reader.QuerySecuritySnapshot(t.Context(), broker.SecuritySnapshotQuery{Symbols: []string{"HK.00700"}}); err == nil {
		t.Fatal("QuerySecuritySnapshot(protocol failure) error = nil")
	}

	result := securitySnapshotResultFromProtoList("account", []*qotgetsecuritysnapshotpb.Snapshot{nil}, time.Now())
	if len(result.Snapshots) != 0 {
		t.Fatalf("securitySnapshotResultFromProtoList(nil row) = %#v", result)
	}
	if _, ok := securitySnapshotItemFromProto(nil, time.Now()); ok {
		t.Fatal("nil security snapshot converted successfully")
	}
}
