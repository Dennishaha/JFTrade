package futu

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	"github.com/shopspring/decimal"
)

func TestExchangeAccountPushMarketWarningAndEmptySymbolBoundaries(t *testing.T) {
	dead := NewExchangeWithConfig(opend.Config{Addr: "127.0.0.1:1", RequestTimeout: 30 * time.Millisecond})
	if err := dead.SubscribeTradeAccountPush(t.Context(), []uint64{1}); err == nil {
		t.Fatal("unavailable account push subscription error = nil")
	}
	server, exchange := coverageMarginExchange(t)
	if _, err := exchange.ensureClient(t.Context()); err != nil {
		t.Fatalf("preconnect account push client error = %v", err)
	}
	server.setDropProto(opend.ProtoTrdSubAccPush)
	if err := exchange.SubscribeTradeAccountPush(t.Context(), []uint64{1}); err == nil {
		t.Fatal("account push transport error = nil")
	}
	if _, _, err := futuSecurityFromSymbol(" "); err == nil {
		t.Fatal("empty Futu security symbol error = nil")
	}

	server, exchange = coverageMarginExchange(t)
	server.setStaticInfos(nil)
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	market, err := exchange.EnsureMarketWithContext(t.Context(), "HK.00700")
	if err != nil || market.MinQuantity.Float64() != 100 {
		t.Fatalf("market fallback rules = %#v, %v", market, err)
	}
}

func TestMaxTradeQuantityInvalidSecurityAfterAccountResolution(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{testSimulateHKCashAccount()})
	if _, err := exchange.QueryBrokerMaxTradeQuantity(t.Context(), BrokerMaxTradeQuantityQuery{Symbol: "BAD", OrderType: "LIMIT", Price: 1}); err == nil {
		t.Fatal("invalid max-trade-quantity security error = nil")
	}
}

func TestBasicQuoteMissingInvalidAndSubscriptionCacheBoundaries(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	if _, err := basicQotForSymbol(map[string]*qotcommonpb.BasicQot{"US.AAPL": {}}, "HK.00700"); err == nil {
		t.Fatal("missing requested quote error = nil")
	}
	if got := basicQotMapFromProto([]*qotcommonpb.BasicQot{{Security: &qotcommonpb.Security{Market: new(int32(-1)), Code: new("BAD")}}}); len(got) != 0 {
		t.Fatalf("invalid returned quote map = %#v", got)
	}

	server.setBasicQuotes(nil)
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}
	security, canonical, _ := futuSecurityFromSymbol("HK.00700")
	request := basicQotRequest{canonical: canonical, security: security}
	if err := exchange.ensureBasicQotSubscriptions(t.Context(), client, nil); err != nil {
		t.Fatalf("empty BasicQot ensure error = %v", err)
	}
	if err := exchange.ensureBasicQotSubscriptions(t.Context(), client, []basicQotRequest{request}); err != nil {
		t.Fatalf("BasicQot ensure error = %v", err)
	}
	if err := exchange.ensureBasicQotSubscriptions(t.Context(), client, []basicQotRequest{request}); err != nil {
		t.Fatalf("cached BasicQot ensure error = %v", err)
	}
}

func TestCurrentKLineExtendedErrorBlankAndInvalidRequestBoundaries(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	security, canonical, _ := futuSecurityFromSymbol("US.AAPL")
	klines, err := exchange.queryCurrentKLines(t.Context(), security, canonical, types.Interval5m, qotcommonpb.KLType_KLType_5Min)
	if err != nil || klines != nil {
		t.Fatalf("unsubscribed extended current K-lines = %#v, %v", klines, err)
	}
	if _, err := exchange.klineSubscriptionRequest("BAD", types.Interval5m); err == nil {
		t.Fatal("invalid K-line request symbol error = nil")
	}

	if err := exchange.SubscribeKLine(t.Context(), "HK.00700", types.Interval5m); err != nil {
		t.Fatalf("SubscribeKLine() error = %v", err)
	}
	hkSecurity, hkCanonical, _ := futuSecurityFromSymbol("HK.00700")
	server.setCurrentKLineError(1, "current denied")
	if _, err := exchange.queryCurrentKLines(t.Context(), hkSecurity, hkCanonical, types.Interval5m, qotcommonpb.KLType_KLType_5Min); err == nil {
		t.Fatal("current K-line protocol error = nil")
	}

	server, exchange = coverageMarginExchange(t)
	if err := exchange.SubscribeKLine(t.Context(), "HK.00700", types.Interval5m); err != nil {
		t.Fatalf("SubscribeKLine(blank) error = %v", err)
	}
	blank := testHistoryKLine(time.Now().UTC(), 99)
	blank.IsBlank = new(true)
	server.setCurrentKLines([]*qotcommonpb.KLine{blank, testHistoryKLine(time.Now().UTC(), 100)})
	klines, err = exchange.queryCurrentKLines(t.Context(), hkSecurity, hkCanonical, types.Interval5m, qotcommonpb.KLType_KLType_5Min)
	if err != nil || len(klines) != 1 {
		t.Fatalf("blank current K-line filtering = %#v, %v", klines, err)
	}
}

func TestSecuritySnapshotInvalidRowsErrorsAndEmptyOptionMerge(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	invalidSnapshot := testTencentSecuritySnapshot()
	invalidSnapshot.Basic.Security = &qotcommonpb.Security{Market: new(int32(-1)), Code: new("BAD")}
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{invalidSnapshot})
	if _, err := exchange.querySecuritySnapshotList(t.Context(), []string{"HK.00700"}); err == nil {
		t.Fatal("invalid snapshot row error = nil")
	}
	server.setStaticInfoError(1, 2, "static denied")
	if _, err := exchange.queryStaticInfoList(t.Context(), []string{"HK.00700"}); err == nil {
		t.Fatal("static info protocol error = nil")
	}
	invalidInfo := testTencentStaticInfo()
	invalidInfo.Basic.Security = &qotcommonpb.Security{Market: new(int32(-1)), Code: new("BAD")}
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{invalidInfo})
	if _, err := exchange.queryStaticInfoList(t.Context(), []string{"HK.00700"}); err == nil {
		t.Fatal("invalid static info row error = nil")
	}

	details := &SecurityDetails{}
	optionInfo := testTencentStaticInfo()
	optionInfo.OptionExData = &qotcommonpb.OptionStaticExData{
		Type: new(int32(qotcommonpb.OptionType_OptionType_Call)), StrikePrice: new(123.0),
	}
	mergeStaticInfoIntoSecurityDetails(details, optionInfo)
	if details.Option == nil || !details.Option.StrikePrice.Equal(decimal.NewFromInt(123)) {
		t.Fatalf("merged empty option details = %#v", details.Option)
	}
}
