package futu

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/exchange"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func TestRegistration(t *testing.T) {
	if !types.ExchangeName("futu").IsValid() {
		t.Fatal("futu should be registered as a valid bbgo exchange via init()")
	}
	ex, err := exchange.New(types.ExchangeName("futu"), exchange.Options{"OPEND_ADDR": "127.0.0.1:11110"})
	if err != nil {
		t.Fatalf("exchange.New: %v", err)
	}
	if ex.Name() != Name {
		t.Fatalf("ex.Name() = %s", ex.Name())
	}
}

func TestConstructorFallsBackToDefaultAddress(t *testing.T) {
	t.Setenv("FUTU_OPEND_ADDR", "")
	ex, err := exchange.New(types.ExchangeName("futu"), exchange.Options{})
	if err != nil {
		t.Fatalf("expected default OpenD address fallback, got error: %v", err)
	}
	if ex.Name() != Name {
		t.Fatalf("ex.Name() = %s", ex.Name())
	}
}

func TestQueryMarketsReturnsBootstrapMarket(t *testing.T) {
	ex := NewExchange("127.0.0.1:11110")
	markets, err := ex.QueryMarkets(t.Context())
	if err != nil {
		t.Fatalf("QueryMarkets: %v", err)
	}
	market, ok := markets["HK.00700"]
	if !ok {
		t.Fatalf("expected bootstrap market HK.00700, got %#v", markets)
	}
	if market.Exchange != Name || market.QuoteCurrency != "HKD" {
		t.Fatalf("unexpected bootstrap market: %#v", market)
	}
}

func TestEnsureMarketWithContextAppliesBrokerLotSize(t *testing.T) {
	server := startQuoteOpenDServer(t)
	lotSize := int32(100)
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{{
		Basic: testHK00700StaticBasic(lotSize),
	}})
	defer server.stop()

	ex := NewExchange(server.addr)
	market, err := ex.EnsureMarketWithContext(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("EnsureMarketWithContext: %v", err)
	}
	if market.MinQuantity.Float64() != 100 || market.StepSize.Float64() != 100 {
		t.Fatalf("market quantity constraints = min %s step %s, want 100/100", market.MinQuantity, market.StepSize)
	}

	markets, err := ex.QueryMarkets(t.Context())
	if err != nil {
		t.Fatalf("QueryMarkets: %v", err)
	}
	stored := markets["HK.00700"]
	if stored.MinQuantity.Float64() != 100 || stored.StepSize.Float64() != 100 {
		t.Fatalf("stored market quantity constraints = min %s step %s, want 100/100", stored.MinQuantity, stored.StepSize)
	}
}

func TestEnsureMarketWithContextFallsBackToSecuritySnapshotLotSize(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setStaticInfoError(-1, 0, "未知的协议ID")
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	defer server.stop()

	ex := NewExchange(server.addr)
	market, warnings, err := ex.EnsureMarketWithDiagnostics(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("EnsureMarketWithDiagnostics: %v", err)
	}
	if market.MinQuantity.Float64() != 100 || market.StepSize.Float64() != 100 {
		t.Fatalf("market quantity constraints = min %s step %s, want 100/100", market.MinQuantity, market.StepSize)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "QuerySecuritySnapshot fallback") {
		t.Fatalf("warnings = %#v, want snapshot fallback warning", warnings)
	}
	if got := server.staticInfoCalls.Load(); got != 1 {
		t.Fatalf("static info calls = %d, want 1", got)
	}
	if got := server.securitySnapshotCalls.Load(); got != 1 {
		t.Fatalf("security snapshot calls = %d, want 1", got)
	}
}

func TestEnsureMarketWithContextReturnsInferredMarketWhenStaticInfoUnavailable(t *testing.T) {
	ex := NewExchange("127.0.0.1:1")
	market, err := ex.EnsureMarketWithContext(t.Context(), "HK.00700")
	if err == nil {
		t.Fatal("EnsureMarketWithContext error = nil, want OpenD connection error")
	}
	if market.Symbol != "HK.00700" || market.StepSize.Float64() != 1 || market.MinQuantity.Float64() != 1 {
		t.Fatalf("fallback market = %#v", market)
	}
}

func TestInferMarketUsesMarketProfiles(t *testing.T) {
	cases := []struct {
		symbol          string
		wantSymbol      string
		wantQuote       string
		wantPriceDigits int
		wantTick        float64
	}{
		{"US.AAPL", "US.AAPL", "USD", 2, 0.01},
		{"HK.00700", "HK.00700", "HKD", 3, 0.001},
		{"SH.600519", "SH.600519", "CNY", 2, 0.01},
		{"SZ.000001", "SZ.000001", "CNY", 2, 0.01},
		{"CNSH.600519", "SH.600519", "CNY", 2, 0.01},
	}
	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			got := inferMarket(tc.symbol)
			if got.Symbol != tc.wantSymbol || got.QuoteCurrency != tc.wantQuote || got.PricePrecision != tc.wantPriceDigits {
				t.Fatalf("inferMarket = %#v", got)
			}
			if got.TickSize.Float64() != tc.wantTick {
				t.Fatalf("TickSize = %s, want %v", got.TickSize.String(), tc.wantTick)
			}
		})
	}
}

func TestFutuSecurityFromSymbolUsesMarketParser(t *testing.T) {
	cases := []struct {
		symbol        string
		wantMarket    qotcommonpb.QotMarket
		wantCode      string
		wantCanonical string
	}{
		{"HK.00700", qotcommonpb.QotMarket_QotMarket_HK_Security, "00700", "HK.00700"},
		{"HK:00700", qotcommonpb.QotMarket_QotMarket_HK_Security, "00700", "HK.00700"},
		{"US.AAPL", qotcommonpb.QotMarket_QotMarket_US_Security, "AAPL", "US.AAPL"},
		{"SH.600519", qotcommonpb.QotMarket_QotMarket_CNSH_Security, "600519", "SH.600519"},
		{"SZ.000001", qotcommonpb.QotMarket_QotMarket_CNSZ_Security, "000001", "SZ.000001"},
	}
	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			security, canonical, err := futuSecurityFromSymbol(tc.symbol)
			if err != nil {
				t.Fatalf("futuSecurityFromSymbol: %v", err)
			}
			if canonical != tc.wantCanonical || qotcommonpb.QotMarket(security.GetMarket()) != tc.wantMarket || security.GetCode() != tc.wantCode {
				t.Fatalf("security=%#v canonical=%s", security, canonical)
			}
		})
	}

	if _, _, err := futuSecurityFromSymbol("CN.600519"); err == nil || !strings.Contains(err.Error(), "requires an exchange-qualified symbol") {
		t.Fatalf("CN.600519 error = %v", err)
	}
}

func testHK00700StaticBasic(lotSize int32) *qotcommonpb.SecurityStaticBasic {
	market := int32(qotcommonpb.QotMarket_QotMarket_HK_Security)
	secType := int32(qotcommonpb.SecurityType_SecurityType_Eqty)
	return &qotcommonpb.SecurityStaticBasic{
		Security: &qotcommonpb.Security{
			Market: &market,
			Code:   new("00700"),
		},
		Id:       new(int64(700)),
		LotSize:  &lotSize,
		SecType:  &secType,
		Name:     new("Tencent"),
		ListTime: new("2004-06-16"),
	}
}

func TestQueryTickerReusesSingleOpenDConnection(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	firstTicker, err := ex.QueryTicker(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("first QueryTicker: %v", err)
	}
	secondTicker, err := ex.QueryTicker(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("second QueryTicker: %v", err)
	}
	if firstTicker == nil || secondTicker == nil {
		t.Fatal("expected non-nil tickers")
	}
	if got := server.acceptCount(); got != 1 {
		t.Fatalf("expected one OpenD TCP session, got %d", got)
	}
	if !server.lastInitRecvNotify() {
		t.Fatal("expected InitConnect to request OpenD notifications")
	}
	if got := server.subCallCount(); got != 1 {
		t.Fatalf("expected one Qot_Sub call, got %d", got)
	}
	if got := server.basicQotCallCount(); got != 2 {
		t.Fatalf("expected two GetBasicQot calls, got %d", got)
	}
	if firstTicker.Last.Float64() != secondTicker.Last.Float64() {
		t.Fatalf("expected stable quote price, got %f and %f", firstTicker.Last.Float64(), secondTicker.Last.Float64())
	}
}

func TestDiscoverAccountsReusesSingleOpenDConnection(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{
		{
			TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             new(uint64(1001)),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
			SecurityFirm:      new(int32(trdcommonpb.SecurityFirm_SecurityFirm_FutuSecurities)),
			SimAccType:        new(int32(trdcommonpb.SimAccType_SimAccType_Stock)),
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	accounts, err := ex.DiscoverAccounts(t.Context())
	if err != nil {
		t.Fatalf("first DiscoverAccounts: %v", err)
	}
	secondAccounts, err := ex.DiscoverAccounts(t.Context())
	if err != nil {
		t.Fatalf("second DiscoverAccounts: %v", err)
	}
	if len(accounts) != 1 || len(secondAccounts) != 1 {
		t.Fatalf("expected one discovered account, got %#v / %#v", accounts, secondAccounts)
	}
	if got := accounts[0].AccountID; got != "1001" {
		t.Fatalf("AccountID = %q, want 1001", got)
	}
	if got := accounts[0].TradingEnvironment; got != "SIMULATE" {
		t.Fatalf("TradingEnvironment = %q, want SIMULATE", got)
	}
	if got := accounts[0].AccountType; got != "CASH" {
		t.Fatalf("AccountType = %q, want CASH", got)
	}
	if accounts[0].SecurityFirm == nil || *accounts[0].SecurityFirm != "FUTUSECURITIES" {
		t.Fatalf("SecurityFirm = %#v, want FUTUSECURITIES", accounts[0].SecurityFirm)
	}
	if accounts[0].SimulatedAccountType == nil || *accounts[0].SimulatedAccountType != "STOCK" {
		t.Fatalf("SimulatedAccountType = %#v, want STOCK", accounts[0].SimulatedAccountType)
	}
	if len(accounts[0].MarketAuthorities) != 1 || accounts[0].MarketAuthorities[0] != "HK" {
		t.Fatalf("MarketAuthorities = %#v, want [HK]", accounts[0].MarketAuthorities)
	}
	if got := server.acceptCount(); got != 1 {
		t.Fatalf("expected one OpenD TCP session, got %d", got)
	}
	if got := server.accountListCallCount(); got != 2 {
		t.Fatalf("expected two Trd_GetAccList calls, got %d", got)
	}
	if !server.lastInitRecvNotify() {
		t.Fatal("expected InitConnect to request OpenD notifications")
	}
}

func TestQueryAccountBalancesUsesOpenDFundsSnapshot(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{
		{
			TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             new(uint64(1001)),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
		},
	})
	server.setFunds(&trdcommonpb.Funds{
		CashInfoList: []*trdcommonpb.AccCashInfo{{
			Currency:         new(int32(trdcommonpb.Currency_Currency_HKD)),
			Cash:             new(float64(10000)),
			AvailableBalance: new(float64(9200)),
			NetCashPower:     new(float64(15000)),
		}},
		Cash:              new(float64(10000)),
		FrozenCash:        new(float64(800)),
		AvlWithdrawalCash: new(float64(9200)),
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	balances, err := ex.QueryAccountBalances(t.Context())
	if err != nil {
		t.Fatalf("QueryAccountBalances: %v", err)
	}
	balance, ok := balances["HKD"]
	if !ok {
		t.Fatalf("expected HKD balance, got %#v", balances)
	}
	if got := balance.Available.Float64(); got != 9200 {
		t.Fatalf("Available = %v, want 9200", got)
	}
	if got := balance.MaxWithdrawAmount.Float64(); got != 9200 {
		t.Fatalf("MaxWithdrawAmount = %v, want 9200", got)
	}
	if got := server.fundsCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetFunds call, got %d", got)
	}
}

func TestQueryOpenOrdersReturnsActiveOrders(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{
		{
			TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             new(uint64(1001)),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
		},
	})
	server.setOrders([]*trdcommonpb.Order{
		{
			OrderID:         new(uint64(2001)),
			Code:            new("HK.00700"),
			TrdSide:         new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
			OrderType:       new(int32(trdcommonpb.OrderType_OrderType_Normal)),
			OrderStatus:     new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
			Qty:             new(float64(100)),
			Price:           new(float64(320)),
			FillQty:         new(float64(25)),
			FillAvgPrice:    new(319.5),
			CreateTimestamp: new(float64(time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC).Unix())),
			UpdateTimestamp: new(float64(time.Date(2026, time.May, 20, 9, 31, 0, 0, time.UTC).Unix())),
			TimeInForce:     new(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
			Currency:        new(int32(trdcommonpb.Currency_Currency_HKD)),
			TrdMarket:       new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		{
			OrderID:     new(uint64(2002)),
			Code:        new("HK.00700"),
			TrdSide:     new(int32(trdcommonpb.TrdSide_TrdSide_Sell)),
			OrderType:   new(int32(trdcommonpb.OrderType_OrderType_Normal)),
			OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Cancelled_All)),
			Qty:         new(float64(50)),
			Price:       new(float64(330)),
			UpdateTime:  new("2026-05-20 09:32:00"),
			CreateTime:  new("2026-05-20 09:30:30"),
			TrdMarket:   new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
			Currency:    new(int32(trdcommonpb.Currency_Currency_HKD)),
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	orders, err := ex.QueryOpenOrders(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("QueryOpenOrders: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected one active order, got %#v", orders)
	}
	if got := orders[0].OrderID; got != 2001 {
		t.Fatalf("OrderID = %d, want 2001", got)
	}
	if got := orders[0].ExecutedQuantity.Float64(); got != 25 {
		t.Fatalf("ExecutedQuantity = %v, want 25", got)
	}
	if !orders[0].IsWorking {
		t.Fatal("expected order to remain working")
	}
	if got := server.orderListCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetOrderList call, got %d", got)
	}
}

func TestTradeProtocolConstantsMatchOfficialIDs(t *testing.T) {
	if got := opend.ProtoTrdGetMaxTrdQtys; got != 2111 {
		t.Fatalf("ProtoTrdGetMaxTrdQtys = %d, want 2111", got)
	}
	if got := opend.ProtoTrdGetOrderFillList; got != 2211 {
		t.Fatalf("ProtoTrdGetOrderFillList = %d, want 2211", got)
	}
	if got := opend.ProtoTrdGetHistoryOrderFillList; got != 2222 {
		t.Fatalf("ProtoTrdGetHistoryOrderFillList = %d, want 2222", got)
	}
	if got := opend.ProtoTrdGetMarginRatio; got != 2223 {
		t.Fatalf("ProtoTrdGetMarginRatio = %d, want 2223", got)
	}
	if got := opend.ProtoTrdFlowSummary; got != 2226 {
		t.Fatalf("ProtoTrdFlowSummary = %d, want 2226", got)
	}
}

func TestQueryBrokerHistoryOrdersReturnsHistoricalOrders(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setHistoryOrders([]*trdcommonpb.Order{
		{
			OrderID:      new(uint64(2001)),
			OrderIDEx:    new("EXT-2001"),
			Code:         new("HK.00700"),
			Name:         new("Tencent"),
			TrdSide:      new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
			OrderType:    new(int32(trdcommonpb.OrderType_OrderType_Normal)),
			OrderStatus:  new(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_All)),
			Qty:          new(float64(100)),
			Price:        new(float64(320)),
			FillQty:      new(float64(100)),
			FillAvgPrice: new(319.8),
			CreateTime:   new("2026-05-20 09:30:00"),
			UpdateTime:   new("2026-05-20 09:35:00"),
			TrdMarket:    new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		{
			OrderID:     new(uint64(2002)),
			OrderIDEx:   new("EXT-2002"),
			Code:        new("HK.00700"),
			Name:        new("Tencent"),
			OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Cancelled_All)),
			Qty:         new(float64(50)),
			Price:       new(float64(330)),
			CreateTime:  new("2026-05-19 09:30:00"),
			UpdateTime:  new("2026-05-19 09:32:00"),
			TrdMarket:   new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	orders, err := ex.QueryBrokerHistoryOrders(t.Context(), BrokerOrderHistoryQuery{
		BrokerReadQuery: BrokerReadQuery{TradingEnvironment: "SIMULATE", AccountID: "1001", Market: "HK"},
		Symbol:          "HK.00700",
		Statuses:        []string{"FILLED_ALL"},
	})
	if err != nil {
		t.Fatalf("QueryBrokerHistoryOrders: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected one historical order, got %#v", orders)
	}
	if got := orders[0].BrokerOrderIDEx; got == nil || *got != "EXT-2001" {
		t.Fatalf("BrokerOrderIDEx = %#v, want EXT-2001", got)
	}
	if got := server.historyOrderListCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetHistoryOrderList call, got %d", got)
	}
}

func TestQueryBrokerHistoryOrderFillsReturnsHistoricalFills(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setHistoryFills([]*trdcommonpb.OrderFill{{
		FillID:     new(uint64(3001)),
		FillIDEx:   new("FILL-3001"),
		OrderID:    new(uint64(2001)),
		OrderIDEx:  new("EXT-2001"),
		Code:       new("HK.00700"),
		Name:       new("Tencent"),
		TrdSide:    new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        new(float64(100)),
		Price:      new(319.8),
		CreateTime: new("2026-05-20 09:35:00"),
		TrdMarket:  new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		Status:     new(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	fills, err := ex.QueryBrokerHistoryOrderFills(t.Context(), BrokerOrderFillHistoryQuery{
		BrokerReadQuery: BrokerReadQuery{TradingEnvironment: "SIMULATE", AccountID: "1001", Market: "HK"},
		Symbol:          "HK.00700",
		StartTime:       "2026-05-20 00:00:00",
		EndTime:         "2026-05-20 23:59:59",
	})
	if err != nil {
		t.Fatalf("QueryBrokerHistoryOrderFills: %v", err)
	}
	if len(fills) != 1 {
		t.Fatalf("expected one historical fill, got %#v", fills)
	}
	if got := fills[0].BrokerFillID; got != "3001" {
		t.Fatalf("BrokerFillID = %q, want 3001", got)
	}
	if got := server.historyOrderFillListCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetHistoryOrderFillList call, got %d", got)
	}
}

func TestQueryBrokerOrderFeesReturnsFeeBreakdown(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setOrderFees([]*trdcommonpb.OrderFee{{
		OrderIDEx: new("EXT-2001"),
		FeeAmount: new(12.5),
		FeeList: []*trdcommonpb.OrderFeeItem{{
			Title: new("BROKERAGE"),
			Value: new(float64(10)),
		}, {
			Title: new("STAMP_DUTY"),
			Value: new(2.5),
		}},
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	fees, err := ex.QueryBrokerOrderFees(t.Context(), BrokerOrderFeeQuery{
		BrokerReadQuery: BrokerReadQuery{TradingEnvironment: "SIMULATE", AccountID: "1001", Market: "HK"},
		OrderIDExList:   []string{"EXT-2001"},
	})
	if err != nil {
		t.Fatalf("QueryBrokerOrderFees: %v", err)
	}
	if len(fees) != 1 {
		t.Fatalf("expected one fee snapshot, got %#v", fees)
	}
	if got := fees[0].BrokerOrderIDEx; got != "EXT-2001" {
		t.Fatalf("BrokerOrderIDEx = %q, want EXT-2001", got)
	}
	if fees[0].FeeAmount == nil || *fees[0].FeeAmount != 12.5 {
		t.Fatalf("FeeAmount = %#v, want 12.5", fees[0].FeeAmount)
	}
	if len(fees[0].FeeItems) != 2 {
		t.Fatalf("FeeItems = %#v, want 2 entries", fees[0].FeeItems)
	}
	if got := server.orderFeeCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetOrderFee call, got %d", got)
	}
}

func TestQueryBrokerMarginRatiosReturnsMarginData(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	server.setMarginRatios([]*trdgetmarginratiopb.MarginRatioInfo{{
		Security:        &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
		IsLongPermit:    new(true),
		IsShortPermit:   new(false),
		ShortFeeRate:    new(1.25),
		AlertLongRatio:  new(0.3),
		AlertShortRatio: new(0.4),
		ImLongRatio:     new(0.5),
		McmLongRatio:    new(0.6),
		MmLongRatio:     new(0.7),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	ratios, err := ex.QueryBrokerMarginRatios(t.Context(), BrokerMarginRatioQuery{Symbols: []string{"HK.00700"}})
	if err != nil {
		t.Fatalf("QueryBrokerMarginRatios: %v", err)
	}
	if len(ratios) != 1 {
		t.Fatalf("expected one margin ratio entry, got %#v", ratios)
	}
	if got := ratios[0].Symbol; got != "HK.00700" {
		t.Fatalf("Symbol = %q, want HK.00700", got)
	}
	if ratios[0].IsLongPermit == nil || !*ratios[0].IsLongPermit {
		t.Fatalf("IsLongPermit = %#v, want true", ratios[0].IsLongPermit)
	}
	if ratios[0].ShortFeeRate == nil || *ratios[0].ShortFeeRate != 1.25 {
		t.Fatalf("ShortFeeRate = %#v, want 1.25", ratios[0].ShortFeeRate)
	}
	if got := server.marginRatioCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetMarginRatio call, got %d", got)
	}
}

func TestQueryBrokerMarginRatiosSkipsUnknownStock(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	server.setMarginRatios([]*trdgetmarginratiopb.MarginRatioInfo{{
		Security:        &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
		IsLongPermit:    new(true),
		IsShortPermit:   new(false),
		ShortFeeRate:    new(1.25),
		AlertLongRatio:  new(0.3),
		AlertShortRatio: new(0.4),
	}})
	server.setStrictMarginRatios(true)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	ratios, err := ex.QueryBrokerMarginRatios(t.Context(), BrokerMarginRatioQuery{Symbols: []string{"HK.00700", "HK.07226"}})
	if err != nil {
		t.Fatalf("QueryBrokerMarginRatios: %v", err)
	}
	if len(ratios) != 1 {
		t.Fatalf("expected one margin ratio entry, got %#v", ratios)
	}
	if got := ratios[0].Symbol; got != "HK.00700" {
		t.Fatalf("Symbol = %q, want HK.00700", got)
	}
	if got := server.marginRatioCallCount(); got < 2 {
		t.Fatalf("expected fallback retries, got %d Trd_GetMarginRatio calls", got)
	}
}

func TestQueryBrokerMarginRatiosUsesCacheWithinTTL(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	server.setMarginRatios([]*trdgetmarginratiopb.MarginRatioInfo{{
		Security:      &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
		IsLongPermit:  new(true),
		IsShortPermit: new(false),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	first, err := ex.QueryBrokerMarginRatios(t.Context(), BrokerMarginRatioQuery{Symbols: []string{"HK.00700"}})
	if err != nil {
		t.Fatalf("first QueryBrokerMarginRatios: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first result length = %d, want 1", len(first))
	}

	second, err := ex.QueryBrokerMarginRatios(t.Context(), BrokerMarginRatioQuery{Symbols: []string{"HK.00700"}})
	if err != nil {
		t.Fatalf("second QueryBrokerMarginRatios: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second result length = %d, want 1", len(second))
	}
	if got := server.marginRatioCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetMarginRatio call due to cache, got %d", got)
	}
}

func TestQueryBrokerCashFlowsReturnsFlowSummary(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setCashFlows([]*trdflowsummarypb.FlowSummaryInfo{{
		CashFlowID:        new(uint64(5001)),
		ClearingDate:      new("2026-05-20"),
		SettlementDate:    new("2026-05-21"),
		Currency:          new(int32(trdcommonpb.Currency_Currency_HKD)),
		CashFlowType:      new("DIVIDEND"),
		CashFlowDirection: new(int32(trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_In)),
		CashFlowAmount:    new(88.8),
		CashFlowRemark:    new("cash-flow-test"),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	flows, err := ex.QueryBrokerCashFlows(t.Context(), BrokerCashFlowQuery{
		BrokerReadQuery: BrokerReadQuery{TradingEnvironment: "REAL", AccountID: "1001", Market: "HK"},
		ClearingDate:    "2026-05-20",
		Direction:       "IN",
	})
	if err != nil {
		t.Fatalf("QueryBrokerCashFlows: %v", err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected one cash-flow entry, got %#v", flows)
	}
	if flows[0].CashFlowID == nil || *flows[0].CashFlowID != "5001" {
		t.Fatalf("CashFlowID = %#v, want 5001", flows[0].CashFlowID)
	}
	if flows[0].CashFlowAmount == nil || *flows[0].CashFlowAmount != 88.8 {
		t.Fatalf("CashFlowAmount = %#v, want 88.8", flows[0].CashFlowAmount)
	}
	if got := server.flowSummaryCallCount(); got != 1 {
		t.Fatalf("expected one Trd_FlowSummary call, got %d", got)
	}
}

func TestQueryBrokerMaxTradeQuantityReturnsSnapshot(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	server.setMaxTrdQtys(&trdcommonpb.MaxTrdQtys{
		MaxCashBuy:          new(float64(1000)),
		MaxCashAndMarginBuy: new(float64(2000)),
		MaxPositionSell:     new(float64(500)),
		MaxSellShort:        new(float64(300)),
		MaxBuyBack:          new(float64(150)),
		LongRequiredIM:      new(float64(10)),
		ShortRequiredIM:     new(float64(12)),
		Session:             new(int32(commonpb.Session_Session_RTH)),
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	snapshot, err := ex.QueryBrokerMaxTradeQuantity(t.Context(), BrokerMaxTradeQuantityQuery{
		BrokerReadQuery: BrokerReadQuery{TradingEnvironment: "REAL", AccountID: "1001", Market: "HK"},
		Symbol:          "HK.00700",
		OrderType:       "LIMIT",
		Price:           320.5,
	})
	if err != nil {
		t.Fatalf("QueryBrokerMaxTradeQuantity: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected max trade quantity snapshot")
	}
	if snapshot.MaxCashBuy != 1000 {
		t.Fatalf("MaxCashBuy = %v, want 1000", snapshot.MaxCashBuy)
	}
	if snapshot.MaxCashAndMarginBuy == nil || *snapshot.MaxCashAndMarginBuy != 2000 {
		t.Fatalf("MaxCashAndMarginBuy = %#v, want 2000", snapshot.MaxCashAndMarginBuy)
	}
	if snapshot.OrderType != "LIMIT" {
		t.Fatalf("OrderType = %q, want LIMIT", snapshot.OrderType)
	}
	request := server.lastMaxTrdQtysRequest()
	if request == nil {
		t.Fatal("expected max trade quantity request")
	}
	if got := request.GetCode(); got != "00700" {
		t.Fatalf("Code = %q, want 00700", got)
	}
	if got := request.GetOrderType(); got != int32(trdcommonpb.OrderType_OrderType_Normal) {
		t.Fatalf("OrderType = %d, want normal", got)
	}
	if got := server.maxTrdQtysCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetMaxTrdQtys call, got %d", got)
	}
}

func TestSubmitOrderPlacesViaOpenD(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setPlacedOrderResponse(9001, "FT-9001")
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	placed, err := ex.SubmitOrder(t.Context(), types.SubmitOrder{
		ClientOrderID: "execution-test-order",
		Symbol:        "HK.00700",
		Side:          types.SideTypeBuy,
		Type:          types.OrderTypeLimit,
		Quantity:      fixedpoint.NewFromFloat(100),
		Price:         fixedpoint.NewFromFloat(320.5),
		TimeInForce:   types.TimeInForceGTC,
	})
	if err != nil {
		t.Fatalf("SubmitOrder: %v", err)
	}
	if placed == nil {
		t.Fatal("expected placed order")
	}
	if got := placed.OrderID; got != 9001 {
		t.Fatalf("OrderID = %d, want 9001", got)
	}
	request := server.lastPlaceOrderRequest()
	if request == nil {
		t.Fatal("expected place order request to be captured")
	}
	if got := request.GetPacketID().GetConnID(); got != 42 {
		t.Fatalf("PacketID.ConnID = %d, want 42", got)
	}
	if got := request.GetCode(); got != "00700" {
		t.Fatalf("Code = %q, want 00700", got)
	}
	if got := request.GetRemark(); got != "execution-test-order" {
		t.Fatalf("Remark = %q, want execution-test-order", got)
	}
	if got := server.placeOrderCallCount(); got != 1 {
		t.Fatalf("expected one Trd_PlaceOrder call, got %d", got)
	}
}

func TestCancelOrdersUsesModifyOrderCancel(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	err := ex.CancelOrders(t.Context(), types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol: "HK.00700",
			Side:   types.SideTypeBuy,
			Type:   types.OrderTypeLimit,
		},
		OrderID: 9001,
	})
	if err != nil {
		t.Fatalf("CancelOrders: %v", err)
	}
	request := server.lastModifyOrderRequest()
	if request == nil {
		t.Fatal("expected modify order request to be captured")
	}
	if got := request.GetModifyOrderOp(); got != int32(trdcommonpb.ModifyOrderOp_ModifyOrderOp_Cancel) {
		t.Fatalf("ModifyOrderOp = %d, want cancel", got)
	}
	if got := request.GetOrderID(); got != 9001 {
		t.Fatalf("OrderID = %d, want 9001", got)
	}
	if got := request.GetPacketID().GetConnID(); got != 42 {
		t.Fatalf("PacketID.ConnID = %d, want 42", got)
	}
	if got := server.modifyOrderCallCount(); got != 1 {
		t.Fatalf("expected one Trd_ModifyOrder call, got %d", got)
	}
}

func TestEnsureSystemNotificationsBindsSystemPushHandler(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setNotifyAfterInit(&notifypb.Response{
		RetType: new(int32(0)),
		S2C: &notifypb.S2C{
			Type: new(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
			ConnectStatus: &notifypb.ConnectStatus{
				QotLogined: new(true),
				TrdLogined: new(false),
			},
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	received := make(chan *notifypb.Response, 1)
	ex.OnSystemNotify(func(response *notifypb.Response) {
		select {
		case received <- response:
		default:
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ex.EnsureSystemNotifications(ctx); err != nil {
		t.Fatalf("EnsureSystemNotifications: %v", err)
	}

	select {
	case response := <-received:
		if response.GetS2C().GetType() != int32(notifypb.NotifyType_NotifyType_ConnStatus) {
			t.Fatalf("notify type = %d", response.GetS2C().GetType())
		}
		if !response.GetS2C().GetConnectStatus().GetQotLogined() {
			t.Fatal("expected qotLogined=true")
		}
		if response.GetS2C().GetConnectStatus().GetTrdLogined() {
			t.Fatal("expected trdLogined=false")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for system notification")
	}
}

func TestSubscribeTradeAccountPushReplaysOnReconnectedClient(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ex.SubscribeTradeAccountPush(ctx, []uint64{1002, 1001, 1001}); err != nil {
		t.Fatalf("SubscribeTradeAccountPush: %v", err)
	}
	if got := server.tradeAccountPushCallCount(); got != 1 {
		t.Fatalf("trade account push calls = %d, want 1", got)
	}
	if got := server.lastTradeAccountPushIDs(); len(got) != 2 || got[0] != 1001 || got[1] != 1002 {
		t.Fatalf("trade account push ids = %#v, want [1001 1002]", got)
	}
	if err := ex.SubscribeTradeAccountPush(ctx, []uint64{1001, 1002, 1002}); err != nil {
		t.Fatalf("SubscribeTradeAccountPush repeat: %v", err)
	}
	if got := server.tradeAccountPushCallCount(); got != 1 {
		t.Fatalf("trade account push calls after repeat = %d, want 1", got)
	}

	if client := ex.Client(); client != nil {
		jftradeErr1 := client.Close()
		jftradeCheckTestError(t, jftradeErr1)
	}
	if err := ex.Connect(ctx); err != nil {
		t.Fatalf("Connect after client close: %v", err)
	}
	if got := server.tradeAccountPushCallCount(); got != 2 {
		t.Fatalf("trade account push calls after reconnect = %d, want 2", got)
	}
}
