package futu

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func TestTradeReadConversionBoundaries(t *testing.T) {
	if got := balanceMapFromBrokerFunds(nil); len(got) != 0 {
		t.Fatalf("nil funds balances = %#v", got)
	}
	currency := "EUR"
	balances := balanceMapFromBrokerFunds(&BrokerFundsSnapshot{Market: "HK", Currency: &currency})
	if _, ok := balances["EUR"]; !ok {
		t.Fatalf("explicit-currency balances = %#v", balances)
	}
	if got := bbgoAccountTypeFromRuntimeAccountType("derivatives"); got != types.AccountTypeFutures {
		t.Fatalf("derivatives account type = %s", got)
	}
	if got := bbgoAccountTypeFromRuntimeAccountType("unknown"); got != types.AccountTypeSpot {
		t.Fatalf("fallback account type = %s", got)
	}
	if got := parseBrokerOrderTimeInLocation("2026-07-15 10:00:00", nil); got.IsZero() {
		t.Fatal("nil-location order time was not parsed")
	}
	if got := formatBrokerOrderTimeAt(nil, "invalid", "UNKNOWN.BAD", time.Time{}); got == "" {
		t.Fatal("zero recorded-at fallback is empty")
	}
	if got := brokerOrderTimeSymbol("UNKNOWN", " raw "); got != "RAW" {
		t.Fatalf("unknown market time symbol = %q", got)
	}
	if got := resolveBrokerOrderMarket(999, "US.AAPL", "HK"); got != "US" {
		t.Fatalf("unknown runtime market fallback = %q", got)
	}
	if got := marketFromSymbol("CN.600000", ""); got != "CN" {
		t.Fatalf("CN-prefixed market = %q", got)
	}
}

func TestTradeReadHelperBoundaries(t *testing.T) {
	if got := brokerOrderStatusFilterValues(nil); got != nil {
		t.Fatalf("empty status filter = %#v", got)
	}
	statuses := brokerOrderStatusFilterValues([]string{" ", "submitted", "SUBMITTED"})
	if len(statuses) != 1 {
		t.Fatalf("deduplicated status filters = %#v", statuses)
	}
	if got := fixedpointFromDifference(nil, nil, new(3.0)); got != fixedpoint.NewFromInt(3) {
		t.Fatalf("fallback difference = %s", got)
	}
	if got := fixedpointFromDifference(nil, nil, nil); got != fixedpoint.Zero {
		t.Fatalf("zero difference = %s", got)
	}
	if got := optionalFloat64Value(nil); got != 0 {
		t.Fatalf("nil float value = %v", got)
	}
	if got := parseUint64("invalid"); got != 0 {
		t.Fatalf("invalid uint64 = %d", got)
	}
}

func TestTradeProtoConversionSkipsNilFeeAndInvalidMarginSecurity(t *testing.T) {
	fee := brokerOrderFeeSnapshotFromProto(resolvedTradeAccount{}, &trdcommonpb.OrderFee{
		FeeList: []*trdcommonpb.OrderFeeItem{nil, {Title: new("commission"), Value: new(1.0)}},
	})
	if len(fee.FeeItems) != 1 {
		t.Fatalf("fee items = %#v", fee.FeeItems)
	}
	margin := brokerMarginRatioSnapshotFromProto(resolvedTradeAccount{Market: "HK"}, &trdcommonpbPlaceholderMarginRatioInfo)
	if margin.Symbol != "" {
		t.Fatalf("invalid margin security symbol = %q", margin.Symbol)
	}
}

func TestAccountAndPushConversionBoundaries(t *testing.T) {
	accounts := runtimeAccountsFromProto([]*trdcommonpb.TrdAcc{
		nil,
		{CardNum: new("card"), TrdEnv: new(int32(-1))},
		{UniCardNum: new("uni")},
		{AccID: new(uint64(2)), TrdEnv: new(int32(trdcommonpb.TrdEnv_TrdEnv_Real))},
		{AccID: new(uint64(1)), TrdEnv: new(int32(trdcommonpb.TrdEnv_TrdEnv_Real))},
	})
	if len(accounts) != 4 || accounts[0].TradingEnvironment != "REAL" || accounts[0].AccountID != "1" {
		t.Fatalf("runtime accounts = %#v", accounts)
	}
	foundUni := false
	for _, account := range accounts {
		foundUni = foundUni || account.AccountID == "uni"
	}
	if !foundUni {
		t.Fatalf("universal-card fallback accounts = %#v", accounts)
	}
	candidates := []resolvedTradeAccount{
		{TradingEnvironment: "UNKNOWN", AccountID: "2", Market: "US"},
		{TradingEnvironment: "REAL", AccountID: "9", Market: "HK"},
		{TradingEnvironment: "REAL", AccountID: "1", Market: "US"},
		{TradingEnvironment: "REAL", AccountID: "1", Market: "HK"},
	}
	sortResolvedTradeAccounts(candidates)
	if candidates[0].Market != "HK" || candidates[3].TradingEnvironment != "UNKNOWN" {
		t.Fatalf("sorted account candidates = %#v", candidates)
	}
	if got := resolvedTradeAccountFromHeader(nil, "US"); got.Market != "US" {
		t.Fatalf("nil push header account = %#v", got)
	}
	if got := resolvedTradeAccountFromHeader(&trdcommonpb.TrdHeader{TrdMarket: new(int32(-1))}, "HK"); got.Market != "HK" {
		t.Fatalf("unknown push market account = %#v", got)
	}
}

var trdcommonpbPlaceholderMarginRatioInfo = func() trdgetmarginratiopb.MarginRatioInfo {
	return trdgetmarginratiopb.MarginRatioInfo{Security: &qotcommonpb.Security{Market: new(int32(-1)), Code: new("BAD")}}
}()
