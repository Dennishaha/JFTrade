package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func TestBrokerAdapterDiscoverAccountsAndTradingBridge(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{
		testSimulateHKCashAccount(),
		testRealHKMarginAccount(),
	})
	server.setPlacedOrderResponse(9001, "FT-9001")
	defer server.stop()

	adapter := newTestBrokerAdapter(t, server)
	ctx := t.Context()

	accounts, err := adapter.DiscoverAccounts(ctx)
	if err != nil {
		t.Fatalf("DiscoverAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %#v", accounts)
	}
	accountsByID := make(map[string]broker.Account, len(accounts))
	for _, account := range accounts {
		if account.BrokerID != "futu" {
			t.Fatalf("BrokerID = %q, want futu", account.BrokerID)
		}
		accountsByID[account.ID] = account
	}
	simulate := accountsByID["1001"]
	if simulate.ID != "1001" {
		t.Fatalf("expected simulated account 1001, got %#v", accounts)
	}
	if got := simulate.TradingEnvironment; got != "SIMULATE" {
		t.Fatalf("TradingEnvironment = %q, want SIMULATE", got)
	}
	if simulate.SecurityFirm == nil || *simulate.SecurityFirm != "FUTUSECURITIES" {
		t.Fatalf("SecurityFirm = %#v, want FUTUSECURITIES", simulate.SecurityFirm)
	}
	if _, ok := accountsByID["1002"]; !ok {
		t.Fatalf("expected real account 1002, got %#v", accounts)
	}

	price := 320.5
	timeInForce := "GTC"
	placed, err := adapter.Trading().PlaceOrder(ctx, broker.PlaceOrderQuery{
		ReadQuery: broker.ReadQuery{
			AccountID:          "1001",
			TradingEnvironment: "SIMULATE",
			Market:             "HK",
		},
		Symbol:        "HK.00700",
		Side:          "BUY",
		OrderType:     "LIMIT",
		Price:         new(price),
		Quantity:      100,
		TimeInForce:   new(timeInForce),
		ClientOrderID: "adapter-order-9001",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if placed == nil {
		t.Fatal("expected place-order result")
	}
	if got := placed.BrokerOrderID; got != "9001" {
		t.Fatalf("BrokerOrderID = %q, want 9001", got)
	}
	if placed.BrokerOrderIDEx == nil || *placed.BrokerOrderIDEx != "FT-9001" {
		t.Fatalf("BrokerOrderIDEx = %#v, want FT-9001", placed.BrokerOrderIDEx)
	}
	if got := placed.Status; got != "SUBMITTED" {
		t.Fatalf("Status = %q, want SUBMITTED", got)
	}
	if got := server.placeOrderCallCount(); got != 1 {
		t.Fatalf("expected one Trd_PlaceOrder call, got %d", got)
	}
	if request := server.lastPlaceOrderRequest(); request == nil {
		t.Fatal("expected place-order request to be captured")
	} else {
		if got := request.GetCode(); got != "00700" {
			t.Fatalf("Code = %q, want 00700", got)
		}
		if got := request.GetRemark(); got != "adapter-order-9001" {
			t.Fatalf("Remark = %q, want adapter-order-9001", got)
		}
	}

	err = adapter.Trading().CancelOrders(ctx, broker.ReadQuery{
		AccountID:          "1001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
	}, broker.CancelOrder{
		OrderID: 9001,
		Symbol:  "HK.00700",
	})
	if err != nil {
		t.Fatalf("CancelOrders: %v", err)
	}
	if got := server.modifyOrderCallCount(); got != 1 {
		t.Fatalf("expected one Trd_ModifyOrder call, got %d", got)
	}
}

func TestBrokerAdapterQueryMarketRulesUsesSecurityInfoLotSize(t *testing.T) {
	server := startQuoteOpenDServer(t)
	lotSize := int32(100)
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{{
		Basic: testHK00700StaticBasic(lotSize),
	}})
	defer server.stop()

	provider, ok := newTestBrokerAdapter(t, server).(broker.MarketRuleProvider)
	if !ok {
		t.Fatal("futu adapter should implement broker.MarketRuleProvider")
	}
	rules, err := provider.QueryMarketRules(t.Context(), broker.MarketRuleQuery{
		ReadQuery: broker.ReadQuery{Market: "HK"},
		Symbols:   []string{"HK.00700"},
	})
	if err != nil {
		t.Fatalf("QueryMarketRules: %v", err)
	}
	if len(rules.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", rules.Warnings)
	}
	if len(rules.Rules) != 1 {
		t.Fatalf("rules = %#v, want one rule", rules.Rules)
	}
	rule := rules.Rules[0]
	if rule.Symbol != "HK.00700" || rule.LotSize == nil || *rule.LotSize != 100 {
		t.Fatalf("rule = %#v, want HK.00700 lotSize 100", rule)
	}
}

func TestBrokerAdapterQueryMarketRulesFallsBackToSecuritySnapshotLotSize(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setStaticInfoError(-1, 0, "未知的协议ID")
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	defer server.stop()

	provider, ok := newTestBrokerAdapter(t, server).(broker.MarketRuleProvider)
	if !ok {
		t.Fatal("futu adapter should implement broker.MarketRuleProvider")
	}
	rules, err := provider.QueryMarketRules(t.Context(), broker.MarketRuleQuery{
		ReadQuery: broker.ReadQuery{Market: "HK"},
		Symbols:   []string{"HK.00700"},
	})
	if err != nil {
		t.Fatalf("QueryMarketRules: %v", err)
	}
	if len(rules.Warnings) != 1 || !strings.Contains(rules.Warnings[0], "QuerySecuritySnapshot fallback") || !strings.Contains(rules.Warnings[0], "未知的协议ID") {
		t.Fatalf("warnings = %#v, want fallback warning with primary failure", rules.Warnings)
	}
	if len(rules.Rules) != 1 {
		t.Fatalf("rules = %#v, want one rule", rules.Rules)
	}
	rule := rules.Rules[0]
	if rule.Symbol != "HK.00700" || rule.LotSize == nil || *rule.LotSize != 100 {
		t.Fatalf("rule = %#v, want HK.00700 snapshot lotSize 100", rule)
	}
	if got := server.staticInfoCalls.Load(); got != 1 {
		t.Fatalf("static info calls = %d, want 1", got)
	}
	if got := server.securitySnapshotCalls.Load(); got != 1 {
		t.Fatalf("security snapshot calls = %d, want 1", got)
	}
}

func TestBrokerAdapterMarketDataReaderTradingSnapshots(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{testSimulateHKCashAccount()})
	server.setFunds(&trdcommonpb.Funds{
		Currency:          new(int32(trdcommonpb.Currency_Currency_HKD)),
		Cash:              new(float64(10000)),
		AvlWithdrawalCash: new(float64(9200)),
		FrozenCash:        new(float64(800)),
		NetCashPower:      new(float64(15000)),
		CashInfoList: []*trdcommonpb.AccCashInfo{{
			Currency:         new(int32(trdcommonpb.Currency_Currency_HKD)),
			Cash:             new(float64(10000)),
			AvailableBalance: new(float64(9200)),
			NetCashPower:     new(float64(15000)),
		}},
	})
	setTestPositions(server,
		&trdcommonpb.Position{
			Code:             new("HK.00700"),
			Name:             new("Tencent"),
			Qty:              new(float64(100)),
			CanSellQty:       new(float64(80)),
			Price:            new(float64(320)),
			DilutedCostPrice: new(float64(300.5)),
			AverageCostPrice: new(float64(299.5)),
			Val:              new(float64(32000)),
			UnrealizedPL:     new(float64(1950)),
			RealizedPL:       new(float64(120)),
			AveragePlRatio:   new(float64(0.12)),
			Currency:         new(int32(trdcommonpb.Currency_Currency_HKD)),
			TrdMarket:        new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		&trdcommonpb.Position{
			Code:             new("US.NVDA"),
			Name:             new("NVIDIA"),
			Qty:              new(float64(10)),
			CanSellQty:       new(float64(10)),
			Price:            new(float64(130)),
			CostPrice:        new(float64(101.25)),
			AverageCostPrice: new(float64(100.25)),
			Val:              new(float64(1300)),
			PlVal:            new(float64(9.5)),
			PlRatio:          new(float64(0.06)),
			Currency:         new(int32(trdcommonpb.Currency_Currency_USD)),
			TrdMarket:        new(int32(trdcommonpb.TrdMarket_TrdMarket_US)),
		},
	)
	server.setOrders([]*trdcommonpb.Order{{
		OrderID:      new(uint64(2001)),
		OrderIDEx:    new("EXT-2001"),
		Code:         new("HK.00700"),
		Name:         new("Tencent"),
		TrdSide:      new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:    new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus:  new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		Qty:          new(float64(100)),
		Price:        new(float64(320)),
		FillQty:      new(float64(25)),
		FillAvgPrice: new(319.5),
		CreateTime:   new("2026-05-20 09:30:00"),
		UpdateTime:   new("2026-05-20 09:31:00"),
		Remark:       new("swing-entry"),
		LastErrMsg:   new("pending-review"),
		TimeInForce:  new(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
		Currency:     new(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:    new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	server.setHistoryOrders([]*trdcommonpb.Order{
		{
			OrderID:      new(uint64(2101)),
			OrderIDEx:    new("HEX-2101"),
			Code:         new("HK.00700"),
			Name:         new("Tencent"),
			TrdSide:      new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
			OrderType:    new(int32(trdcommonpb.OrderType_OrderType_Normal)),
			OrderStatus:  new(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_All)),
			Qty:          new(float64(100)),
			Price:        new(float64(318)),
			FillQty:      new(float64(100)),
			FillAvgPrice: new(317.8),
			CreateTime:   new("2026-05-20 09:30:00"),
			UpdateTime:   new("2026-05-20 09:35:00"),
			TrdMarket:    new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		{
			OrderID:     new(uint64(2102)),
			OrderIDEx:   new("HEX-2102"),
			Code:        new("HK.00700"),
			OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Cancelled_All)),
			Qty:         new(float64(20)),
			Price:       new(float64(319)),
			CreateTime:  new("2026-05-19 09:30:00"),
			UpdateTime:  new("2026-05-19 09:31:00"),
			TrdMarket:   new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
	})
	setTestOrderFills(server, []*trdcommonpb.OrderFill{{
		FillID:     new(uint64(3001)),
		FillIDEx:   new("FILL-3001"),
		OrderID:    new(uint64(2001)),
		OrderIDEx:  new("EXT-2001"),
		Code:       new("HK.00700"),
		Name:       new("Tencent"),
		TrdSide:    new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        new(float64(25)),
		Price:      new(319.5),
		CreateTime: new("2026-05-20 09:35:00"),
		TrdMarket:  new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		Status:     new(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
	}})
	server.setHistoryFills([]*trdcommonpb.OrderFill{{
		FillID:     new(uint64(3101)),
		FillIDEx:   new("HFILL-3101"),
		OrderID:    new(uint64(2101)),
		OrderIDEx:  new("HEX-2101"),
		Code:       new("HK.00700"),
		Name:       new("Tencent"),
		TrdSide:    new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        new(float64(100)),
		Price:      new(317.8),
		CreateTime: new("2026-05-20 09:35:00"),
		TrdMarket:  new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		Status:     new(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
	}})
	defer server.stop()

	reader := newTestBrokerAdapter(t, server).MarketData()
	ctx := t.Context()

	funds, err := reader.QueryFunds(ctx, broker.ReadQuery{
		AccountID:          "1001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
	})
	if err != nil {
		t.Fatalf("QueryFunds: %v", err)
	}
	if funds == nil {
		t.Fatal("expected funds snapshot")
	}
	if funds.Currency == nil || *funds.Currency != "HKD" {
		t.Fatalf("Currency = %#v, want HKD", funds.Currency)
	}
	if funds.AvailableWithdrawalCash == nil || *funds.AvailableWithdrawalCash != 9200 {
		t.Fatalf("AvailableWithdrawalCash = %#v, want 9200", funds.AvailableWithdrawalCash)
	}
	if len(funds.CurrencyBalances) != 1 || funds.CurrencyBalances[0].Currency != "HKD" {
		t.Fatalf("CurrencyBalances = %#v, want one HKD entry", funds.CurrencyBalances)
	}

	positions, err := reader.QueryPositions(ctx, broker.ReadQuery{
		AccountID:          "1001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
	})
	if err != nil {
		t.Fatalf("QueryPositions: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %#v", positions)
	}
	if positions[0].CostPrice == nil || *positions[0].CostPrice != 300.5 {
		t.Fatalf("Tencent CostPrice = %#v, want diluted cost 300.5", positions[0].CostPrice)
	}
	if positions[1].CostPrice == nil || *positions[1].CostPrice != 101.25 {
		t.Fatalf("NVIDIA CostPrice = %#v, want fallback cost 101.25", positions[1].CostPrice)
	}
	if positions[1].UnrealizedPnl == nil || *positions[1].UnrealizedPnl != 9.5 {
		t.Fatalf("NVIDIA UnrealizedPnl = %#v, want fallback PL 9.5", positions[1].UnrealizedPnl)
	}
	if positions[1].PnlRatio == nil || *positions[1].PnlRatio != 0.06 {
		t.Fatalf("NVIDIA PnlRatio = %#v, want fallback ratio 0.06", positions[1].PnlRatio)
	}
	if positions[1].Currency == nil || *positions[1].Currency != "USD" {
		t.Fatalf("NVIDIA Currency = %#v, want USD", positions[1].Currency)
	}
	if got := int(server.positionListCalls.Load()); got != 1 {
		t.Fatalf("expected one Trd_GetPositionList call, got %d", got)
	}

	orders, err := reader.QueryOrders(ctx, broker.ReadQuery{
		AccountID:          "1001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
	}, "HK.00700")
	if err != nil {
		t.Fatalf("QueryOrders: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected one order, got %#v", orders)
	}
	if got := orders[0].BrokerOrderID; got != "2001" {
		t.Fatalf("BrokerOrderID = %q, want 2001", got)
	}
	if orders[0].BrokerOrderIDEx == nil || *orders[0].BrokerOrderIDEx != "EXT-2001" {
		t.Fatalf("BrokerOrderIDEx = %#v, want EXT-2001", orders[0].BrokerOrderIDEx)
	}
	if got := orders[0].Status; got != "SUBMITTED" {
		t.Fatalf("Status = %q, want SUBMITTED", got)
	}
	if orders[0].Remark == nil || *orders[0].Remark != "swing-entry" {
		t.Fatalf("Remark = %#v, want swing-entry", orders[0].Remark)
	}
	if orders[0].LastError == nil || *orders[0].LastError != "pending-review" {
		t.Fatalf("LastError = %#v, want pending-review", orders[0].LastError)
	}

	historyOrders, err := reader.QueryHistoryOrders(ctx, broker.OrderHistoryQuery{
		ReadQuery: broker.ReadQuery{
			AccountID:          "1001",
			TradingEnvironment: "SIMULATE",
			Market:             "HK",
		},
		Symbol:   "HK.00700",
		Statuses: []string{"FILLED_ALL"},
	})
	if err != nil {
		t.Fatalf("QueryHistoryOrders: %v", err)
	}
	if len(historyOrders) != 1 {
		t.Fatalf("expected one filtered history order, got %#v", historyOrders)
	}
	if got := historyOrders[0].Status; got != "FILLED_ALL" {
		t.Fatalf("Status = %q, want FILLED_ALL", got)
	}
	if got := server.historyOrderListCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetHistoryOrderList call, got %d", got)
	}

	orderFills, err := reader.QueryOrderFills(ctx, broker.OrderFillQuery{
		ReadQuery: broker.ReadQuery{
			AccountID:          "1001",
			TradingEnvironment: "SIMULATE",
			Market:             "HK",
		},
		Symbol: "HK.00700",
	})
	if err != nil {
		t.Fatalf("QueryOrderFills: %v", err)
	}
	if len(orderFills) != 1 {
		t.Fatalf("expected one order fill, got %#v", orderFills)
	}
	if got := orderFills[0].BrokerFillID; got != "3001" {
		t.Fatalf("BrokerFillID = %q, want 3001", got)
	}
	if orderFills[0].Status == nil || *orderFills[0].Status != "OK" {
		t.Fatalf("Status = %#v, want OK", orderFills[0].Status)
	}

	historyFills, err := reader.QueryHistoryOrderFills(ctx, broker.OrderFillHistoryQuery{
		ReadQuery: broker.ReadQuery{
			AccountID:          "1001",
			TradingEnvironment: "SIMULATE",
			Market:             "HK",
		},
		Symbol: "HK.00700",
	})
	if err != nil {
		t.Fatalf("QueryHistoryOrderFills: %v", err)
	}
	if len(historyFills) != 1 {
		t.Fatalf("expected one history fill, got %#v", historyFills)
	}
	if got := historyFills[0].BrokerFillIDEx; got == nil || *got != "HFILL-3101" {
		t.Fatalf("BrokerFillIDEx = %#v, want HFILL-3101", got)
	}
	if got := server.historyOrderFillListCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetHistoryOrderFillList call, got %d", got)
	}
}

func TestBrokerAdapterMarketDataReaderAccountAnalytics(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{testRealHKMarginAccount()})
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

	reader := newTestBrokerAdapter(t, server).MarketData()
	ctx := t.Context()

	orderFees, err := reader.QueryOrderFees(ctx, broker.OrderFeeQuery{
		ReadQuery: broker.ReadQuery{
			AccountID:          "1002",
			TradingEnvironment: "REAL",
			Market:             "HK",
		},
		OrderIDExList: []string{"EXT-2001"},
	})
	if err != nil {
		t.Fatalf("QueryOrderFees: %v", err)
	}
	if len(orderFees) != 1 {
		t.Fatalf("expected one order-fee entry, got %#v", orderFees)
	}
	if orderFees[0].FeeAmount == nil || *orderFees[0].FeeAmount != 12.5 {
		t.Fatalf("FeeAmount = %#v, want 12.5", orderFees[0].FeeAmount)
	}
	if len(orderFees[0].FeeItems) != 2 {
		t.Fatalf("FeeItems = %#v, want 2 entries", orderFees[0].FeeItems)
	}

	marginRatios, err := reader.QueryMarginRatios(ctx, broker.MarginRatioQuery{
		ReadQuery: broker.ReadQuery{
			AccountID:          "1002",
			TradingEnvironment: "REAL",
			Market:             "HK",
		},
		Symbols: []string{"HK.00700"},
	})
	if err != nil {
		t.Fatalf("QueryMarginRatios: %v", err)
	}
	if len(marginRatios) != 1 {
		t.Fatalf("expected one margin ratio, got %#v", marginRatios)
	}
	if got := marginRatios[0].Symbol; got != "HK.00700" {
		t.Fatalf("Symbol = %q, want HK.00700", got)
	}
	if marginRatios[0].ShortFeeRate == nil || *marginRatios[0].ShortFeeRate != 1.25 {
		t.Fatalf("ShortFeeRate = %#v, want 1.25", marginRatios[0].ShortFeeRate)
	}

	cashFlows, err := reader.QueryCashFlows(ctx, broker.CashFlowQuery{
		ReadQuery: broker.ReadQuery{
			AccountID:          "1002",
			TradingEnvironment: "REAL",
			Market:             "HK",
		},
		ClearingDate: "2026-05-20",
		Direction:    "IN",
	})
	if err != nil {
		t.Fatalf("QueryCashFlows: %v", err)
	}
	if len(cashFlows) != 1 {
		t.Fatalf("expected one cash-flow entry, got %#v", cashFlows)
	}
	if cashFlows[0].CashFlowDirection == nil || *cashFlows[0].CashFlowDirection != "IN" {
		t.Fatalf("CashFlowDirection = %#v, want IN", cashFlows[0].CashFlowDirection)
	}
	if cashFlows[0].CashFlowAmount == nil || *cashFlows[0].CashFlowAmount != 88.8 {
		t.Fatalf("CashFlowAmount = %#v, want 88.8", cashFlows[0].CashFlowAmount)
	}

	maxQty, err := reader.QueryMaxTradeQuantity(ctx, broker.MaxTradeQuantityQuery{
		ReadQuery: broker.ReadQuery{
			AccountID:          "1002",
			TradingEnvironment: "REAL",
			Market:             "HK",
		},
		Symbol:    "HK.00700",
		OrderType: "LIMIT",
		Price:     320.5,
	})
	if err != nil {
		t.Fatalf("QueryMaxTradeQuantity: %v", err)
	}
	if maxQty == nil {
		t.Fatal("expected max-trade-quantity snapshot")
	}
	if maxQty.MaxCashBuy != 1000 {
		t.Fatalf("MaxCashBuy = %v, want 1000", maxQty.MaxCashBuy)
	}
	if maxQty.MaxCashAndMarginBuy == nil || *maxQty.MaxCashAndMarginBuy != 2000 {
		t.Fatalf("MaxCashAndMarginBuy = %#v, want 2000", maxQty.MaxCashAndMarginBuy)
	}
	if maxQty.Session == nil || *maxQty.Session != "RTH" {
		t.Fatalf("Session = %#v, want RTH", maxQty.Session)
	}
}

func TestBrokerAdapterQuoteKLinesSubscriptionsAndValidation(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	adapter := newTestBrokerAdapter(t, server)
	reader := adapter.MarketData()
	ctx := t.Context()

	quote, err := reader.QueryQuote(ctx, broker.QuoteQuery{
		ReadQuery: broker.ReadQuery{AccountID: "1001"},
		Symbols:   []string{"HK.00700", "US.NVDA"},
	})
	if err != nil {
		t.Fatalf("QueryQuote: %v", err)
	}
	if quote == nil {
		t.Fatal("expected quote snapshot")
	}
	if got := quote.Symbol; got != "HK.00700" {
		t.Fatalf("Symbol = %q, want HK.00700", got)
	}
	if len(quote.Quotes) != 2 {
		t.Fatalf("expected 2 quotes, got %#v", quote.Quotes)
	}
	if got := quote.Quotes[1].Symbol; got != "US.NVDA" {
		t.Fatalf("second quote symbol = %q, want US.NVDA", got)
	}
	if got := quote.Quotes[0].LastPrice; got != 700 {
		t.Fatalf("first quote price = %v, want 700", got)
	}

	klines, err := reader.QueryKLines(ctx, broker.KLineQuery{
		ReadQuery: broker.ReadQuery{AccountID: "1001"},
		Symbol:    "HK.00700",
		Period:    "1m",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if klines == nil {
		t.Fatal("expected kline snapshot")
	}
	if len(klines.KLines) != 1 {
		t.Fatalf("expected one kline, got %#v", klines.KLines)
	}
	if got := klines.KLines[0].Time; got != "2026-05-20 08:00:00" {
		t.Fatalf("KLine time = %q, want 2026-05-20 08:00:00", got)
	}
	if klines.KLines[0].Volume == nil || *klines.KLines[0].Volume != 1000 {
		t.Fatalf("KLine volume = %#v, want 1000", klines.KLines[0].Volume)
	}

	quoteSubscriber, ok := adapter.(broker.QuoteSubscriber)
	if !ok {
		t.Fatal("expected adapter to implement broker.QuoteSubscriber")
	}
	if err := quoteSubscriber.SubscribeQuotes(ctx, broker.QuoteSubscribeRequest{
		Symbols: []string{"HK.00700"},
	}); err != nil {
		t.Fatalf("SubscribeQuotes: %v", err)
	}

	orderBookSubscriber, ok := adapter.(broker.OrderBookSubscriber)
	if !ok {
		t.Fatal("expected adapter to implement broker.OrderBookSubscriber")
	}
	if err := orderBookSubscriber.SubscribeOrderBook(ctx, broker.OrderBookSubscribeRequest{
		Symbols: []string{"US.NVDA"},
	}); err != nil {
		t.Fatalf("SubscribeOrderBook: %v", err)
	}
	if got := server.pushSubCallCount(); got != 2 {
		t.Fatalf("expected two push subscriptions, got %d", got)
	}

	if _, err := reader.QueryQuote(ctx, broker.QuoteQuery{}); err == nil || err.Error() != "futu: QueryQuote requires at least one symbol" {
		t.Fatalf("QueryQuote empty error = %v", err)
	}
	if _, err := reader.QueryKLines(ctx, broker.KLineQuery{}); err == nil || err.Error() != "futu: QueryKLines requires a symbol" {
		t.Fatalf("QueryKLines empty error = %v", err)
	}
	if _, err := reader.QuerySecurityInfo(ctx, broker.SecurityInfoQuery{}); err == nil || err.Error() != "futu: QuerySecurityInfo requires at least one symbol" {
		t.Fatalf("QuerySecurityInfo empty error = %v", err)
	}
	if _, err := reader.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{}); err == nil || err.Error() != "futu: QuerySecuritySnapshot requires at least one symbol" {
		t.Fatalf("QuerySecuritySnapshot empty error = %v", err)
	}
	if _, err := reader.QueryOrderBook(ctx, broker.OrderBookQuery{}); err == nil || err.Error() != "futu: QueryOrderBook requires a symbol" {
		t.Fatalf("QueryOrderBook empty error = %v", err)
	}
}

func TestBrokerAdapterUnlockTradeBridge(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	adapter := newTestBrokerAdapter(t, server)
	unlocker, ok := adapter.(broker.UnlockTrader)
	if !ok {
		t.Fatal("expected adapter to implement broker.UnlockTrader")
	}

	err := unlocker.UnlockTrade(t.Context(), broker.UnlockTradeRequest{
		Unlock:      true,
		PasswordMD5: "dummy-md5",
	})
	if err != nil {
		t.Fatalf("UnlockTrade: %v", err)
	}
	if got := server.unlockTradeCalls.Load(); got != 1 {
		t.Fatalf("expected one Trd_UnlockTrade call, got %d", got)
	}
	if server.lastUnlockTrade == nil || !server.lastUnlockTrade.GetUnlock() {
		t.Fatalf("unlock request = %#v, want unlock=true", server.lastUnlockTrade)
	}
	if got := server.lastUnlockTrade.GetPwdMD5(); got != "dummy-md5" {
		t.Fatalf("PwdMD5 = %q, want dummy-md5", got)
	}
}

func newTestBrokerAdapter(t testing.TB, server *quoteOpenDServer) broker.Broker {
	t.Helper()
	ex := NewExchangeWithConfig(opend.Config{
		Addr:           server.addr,
		RequestTimeout: 2 * time.Second,
	})
	t.Cleanup(func() {
		jftradeCheckTestError(t, ex.Close())
	})
	return NewBrokerAdapter(ex)
}

func testSimulateHKCashAccount() *trdcommonpb.TrdAcc {
	return &trdcommonpb.TrdAcc{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
		SecurityFirm:      new(int32(trdcommonpb.SecurityFirm_SecurityFirm_FutuSecurities)),
		SimAccType:        new(int32(trdcommonpb.SimAccType_SimAccType_Stock)),
	}
}

func testRealHKMarginAccount() *trdcommonpb.TrdAcc {
	return &trdcommonpb.TrdAcc{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(1002)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
		SecurityFirm:      new(int32(trdcommonpb.SecurityFirm_SecurityFirm_FutuSecurities)),
	}
}

func setTestPositions(server *quoteOpenDServer, positions ...*trdcommonpb.Position) {
	server.tradeMu.Lock()
	defer server.tradeMu.Unlock()
	server.positions = append([]*trdcommonpb.Position(nil), positions...)
}

func setTestOrderFills(server *quoteOpenDServer, fills []*trdcommonpb.OrderFill) {
	server.tradeMu.Lock()
	defer server.tradeMu.Unlock()
	server.orderFills = append([]*trdcommonpb.OrderFill(nil), fills...)
}
