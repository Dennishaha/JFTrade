package futu

import (
	"strings"
	"testing"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func TestBrokerFundsSnapshotFromProtoPreservesCurrencyAndMarketBreakdowns(t *testing.T) {
	account := resolvedTradeAccount{
		AccountID:          "ACC-1001",
		TradingEnvironment: "REAL",
		Market:             "US",
		AccountType:        "MARGIN",
	}
	funds := &trdcommonpb.Funds{
		Currency: futuTestPtr(int32(trdcommonpb.Currency_Currency_USD)),
		Cash:     futuTestPtr(12345.0),
		CashInfoList: []*trdcommonpb.AccCashInfo{
			nil,
			{Currency: futuTestPtr(int32(trdcommonpb.Currency_Currency_Unknown)), Cash: futuTestPtr(1.0)},
			{
				Currency:         futuTestPtr(int32(trdcommonpb.Currency_Currency_USD)),
				Cash:             futuTestPtr(12000.0),
				AvailableBalance: futuTestPtr(11800.0),
				NetCashPower:     futuTestPtr(15000.0),
			},
			{
				Currency:         futuTestPtr(int32(trdcommonpb.Currency_Currency_HKD)),
				Cash:             futuTestPtr(8000.0),
				AvailableBalance: futuTestPtr(7800.0),
			},
		},
		MarketInfoList: []*trdcommonpb.AccMarketInfo{
			nil,
			{TrdMarket: futuTestPtr(int32(999999)), Assets: futuTestPtr(1.0)},
			{TrdMarket: futuTestPtr(int32(trdcommonpb.TrdMarket_TrdMarket_US)), Assets: futuTestPtr(90000.0)},
			{TrdMarket: futuTestPtr(int32(trdcommonpb.TrdMarket_TrdMarket_HK)), Assets: futuTestPtr(30000.0)},
		},
	}

	snapshot := brokerFundsSnapshotFromProto(account, funds)
	if snapshot.AccountID != account.AccountID || snapshot.TradingEnvironment != "REAL" || snapshot.Market != "US" || snapshot.AccountType != "MARGIN" {
		t.Fatalf("account context was not preserved: %#v", snapshot)
	}
	if snapshot.Currency == nil || !strings.Contains(*snapshot.Currency, "USD") {
		t.Fatalf("currency = %#v, want USD-derived enum", snapshot.Currency)
	}
	if len(snapshot.CurrencyBalances) != 2 {
		t.Fatalf("currency balances = %#v, want valid USD/HKD rows only", snapshot.CurrencyBalances)
	}
	if snapshot.CurrencyBalances[0].AccountID != account.AccountID || snapshot.CurrencyBalances[0].TradingEnvironment != "REAL" {
		t.Fatalf("currency row account context = %#v", snapshot.CurrencyBalances[0])
	}
	if snapshot.CurrencyBalances[0].Currency != "USD" || ptrFloat64Value(snapshot.CurrencyBalances[0].AvailableWithdrawalCash) != 11800 {
		t.Fatalf("USD currency balance = %#v", snapshot.CurrencyBalances[0])
	}
	if snapshot.CurrencyBalances[1].Currency != "HKD" || ptrFloat64Value(snapshot.CurrencyBalances[1].Cash) != 8000 {
		t.Fatalf("HKD currency balance = %#v", snapshot.CurrencyBalances[1])
	}
	if len(snapshot.MarketAssets) != 2 {
		t.Fatalf("market assets = %#v, want valid US/HK rows only", snapshot.MarketAssets)
	}
	if snapshot.MarketAssets[0].Market != "US" || ptrFloat64Value(snapshot.MarketAssets[0].Assets) != 90000 {
		t.Fatalf("US market asset = %#v", snapshot.MarketAssets[0])
	}
	if snapshot.MarketAssets[1].Market != "HK" || ptrFloat64Value(snapshot.MarketAssets[1].Assets) != 30000 {
		t.Fatalf("HK market asset = %#v", snapshot.MarketAssets[1])
	}
}

func TestBrokerReadProtoSnapshotsNormalizePositionMarginAndQuantityDetails(t *testing.T) {
	account := resolvedTradeAccount{
		AccountID:          "ACC-2002",
		TradingEnvironment: "REAL",
		Market:             "HK",
	}

	position := brokerPositionSnapshotFromProto(account, &trdcommonpb.Position{
		Code:             futuTestPtr(" us.aapl "),
		Name:             futuTestPtr(" Apple "),
		Qty:              futuTestPtr(12.0),
		CanSellQty:       futuTestPtr(8.0),
		Price:            futuTestPtr(189.25),
		CostPrice:        futuTestPtr(180.0),
		DilutedCostPrice: futuTestPtr(181.5),
		AverageCostPrice: futuTestPtr(182.0),
		Val:              futuTestPtr(2271.0),
		PlVal:            futuTestPtr(90.0),
		UnrealizedPL:     futuTestPtr(111.0),
		PlRatio:          futuTestPtr(0.05),
		AveragePlRatio:   futuTestPtr(0.061),
		Currency:         futuTestPtr(int32(trdcommonpb.Currency_Currency_USD)),
	})
	if position.Market != "US" || position.Symbol != "US.AAPL" || position.SymbolName == nil || *position.SymbolName != "Apple" {
		t.Fatalf("position identity = %#v", position)
	}
	if ptrFloat64Value(position.CostPrice) != 181.5 || ptrFloat64Value(position.UnrealizedPnl) != 111 || ptrFloat64Value(position.PnlRatio) != 0.061 {
		t.Fatalf("position preferred cost/pnl fields = %#v", position)
	}

	margin := brokerMarginRatioSnapshotFromProto(account, &trdgetmarginratiopb.MarginRatioInfo{
		Security: &qotcommonpb.Security{
			Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
			Code:   futuTestPtr("aapl"),
		},
		IsLongPermit:    futuTestPtr(true),
		IsShortPermit:   futuTestPtr(false),
		ShortPoolRemain: futuTestPtr(1000.0),
		ShortFeeRate:    futuTestPtr(0.023),
		AlertLongRatio:  futuTestPtr(0.5),
		ImLongRatio:     futuTestPtr(0.3),
		MmShortRatio:    futuTestPtr(0.4),
	})
	if margin.AccountID != account.AccountID || margin.Market != "US" || margin.Symbol != "US.AAPL" {
		t.Fatalf("margin identity = %#v", margin)
	}
	if margin.IsLongPermit == nil || !*margin.IsLongPermit || margin.IsShortPermit == nil || *margin.IsShortPermit {
		t.Fatalf("margin permission fields = %#v", margin)
	}
	if ptrFloat64Value(margin.ShortPoolRemain) != 1000 || ptrFloat64Value(margin.InitialMarginLongRatio) != 0.3 || ptrFloat64Value(margin.MaintenanceShortRatio) != 0.4 {
		t.Fatalf("margin numeric fields = %#v", margin)
	}

	maxQty := brokerMaxTradeQuantitySnapshotFromProto(account, "US.AAPL", "LIMIT", 188.5, &trdcommonpb.MaxTrdQtys{
		MaxCashBuy:          futuTestPtr(10.0),
		MaxCashAndMarginBuy: futuTestPtr(20.0),
		MaxPositionSell:     futuTestPtr(6.0),
		MaxSellShort:        futuTestPtr(4.0),
		MaxBuyBack:          futuTestPtr(3.0),
		LongRequiredIM:      futuTestPtr(1000.0),
		ShortRequiredIM:     futuTestPtr(1200.0),
		Session:             futuTestPtr(int32(commonpb.Session_Session_ETH)),
	})
	if maxQty.AccountID != account.AccountID || maxQty.Symbol != "US.AAPL" || maxQty.OrderType != "LIMIT" || maxQty.Price != 188.5 {
		t.Fatalf("max quantity identity = %#v", maxQty)
	}
	if maxQty.MaxCashBuy != 10 || maxQty.MaxPositionSell != 6 || ptrFloat64Value(maxQty.MaxCashAndMarginBuy) != 20 || ptrFloat64Value(maxQty.MaxSellShort) != 4 {
		t.Fatalf("max quantity size fields = %#v", maxQty)
	}
	if ptrFloat64Value(maxQty.LongRequiredIM) != 1000 || ptrFloat64Value(maxQty.ShortRequiredIM) != 1200 || maxQty.Session == nil || !strings.Contains(*maxQty.Session, "ETH") {
		t.Fatalf("max quantity margin/session fields = %#v", maxQty)
	}

	emptyMaxQty := brokerMaxTradeQuantitySnapshotFromProto(account, "HK.00700", "MARKET", 0, nil)
	if emptyMaxQty == nil || emptyMaxQty.MaxCashBuy != 0 || emptyMaxQty.MaxPositionSell != 0 {
		t.Fatalf("nil max quantity snapshot = %#v, want zero-valued but present", emptyMaxQty)
	}
}

func ptrFloat64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
