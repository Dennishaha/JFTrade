package futu

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/exchange"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	qotupdatebasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdatebasicqot"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetacclistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetacclist"
	trdgetfundspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetfunds"
	trdgethistoryorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderfilllist"
	trdgethistoryorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderlist"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
	trdgetorderfeepb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfee"
	trdgetorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfilllist"
	trdgetorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderlist"
	trdgetpositionlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetpositionlist"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
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

func TestQueryTickerReusesSingleOpenDConnection(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
			TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             proto.Uint64(1001),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
			SecurityFirm:      proto.Int32(int32(trdcommonpb.SecurityFirm_SecurityFirm_FutuSecurities)),
			SimAccType:        proto.Int32(int32(trdcommonpb.SimAccType_SimAccType_Stock)),
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
			TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             proto.Uint64(1001),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
		},
	})
	server.setFunds(&trdcommonpb.Funds{
		CashInfoList: []*trdcommonpb.AccCashInfo{{
			Currency:         proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
			Cash:             proto.Float64(10000),
			AvailableBalance: proto.Float64(9200),
			NetCashPower:     proto.Float64(15000),
		}},
		Cash:              proto.Float64(10000),
		FrozenCash:        proto.Float64(800),
		AvlWithdrawalCash: proto.Float64(9200),
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
			TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             proto.Uint64(1001),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
		},
	})
	server.setOrders([]*trdcommonpb.Order{
		{
			OrderID:         proto.Uint64(2001),
			Code:            proto.String("HK.00700"),
			TrdSide:         proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
			OrderType:       proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
			OrderStatus:     proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
			Qty:             proto.Float64(100),
			Price:           proto.Float64(320),
			FillQty:         proto.Float64(25),
			FillAvgPrice:    proto.Float64(319.5),
			CreateTimestamp: proto.Float64(float64(time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC).Unix())),
			UpdateTimestamp: proto.Float64(float64(time.Date(2026, time.May, 20, 9, 31, 0, 0, time.UTC).Unix())),
			TimeInForce:     proto.Int32(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
			Currency:        proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
			TrdMarket:       proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		{
			OrderID:     proto.Uint64(2002),
			Code:        proto.String("HK.00700"),
			TrdSide:     proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Sell)),
			OrderType:   proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
			OrderStatus: proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Cancelled_All)),
			Qty:         proto.Float64(50),
			Price:       proto.Float64(330),
			UpdateTime:  proto.String("2026-05-20 09:32:00"),
			CreateTime:  proto.String("2026-05-20 09:30:30"),
			TrdMarket:   proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
			Currency:    proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setHistoryOrders([]*trdcommonpb.Order{
		{
			OrderID:      proto.Uint64(2001),
			OrderIDEx:    proto.String("EXT-2001"),
			Code:         proto.String("HK.00700"),
			Name:         proto.String("Tencent"),
			TrdSide:      proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
			OrderType:    proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
			OrderStatus:  proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_All)),
			Qty:          proto.Float64(100),
			Price:        proto.Float64(320),
			FillQty:      proto.Float64(100),
			FillAvgPrice: proto.Float64(319.8),
			CreateTime:   proto.String("2026-05-20 09:30:00"),
			UpdateTime:   proto.String("2026-05-20 09:35:00"),
			TrdMarket:    proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		{
			OrderID:     proto.Uint64(2002),
			OrderIDEx:   proto.String("EXT-2002"),
			Code:        proto.String("HK.00700"),
			Name:        proto.String("Tencent"),
			OrderStatus: proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Cancelled_All)),
			Qty:         proto.Float64(50),
			Price:       proto.Float64(330),
			CreateTime:  proto.String("2026-05-19 09:30:00"),
			UpdateTime:  proto.String("2026-05-19 09:32:00"),
			TrdMarket:   proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setHistoryFills([]*trdcommonpb.OrderFill{{
		FillID:     proto.Uint64(3001),
		FillIDEx:   proto.String("FILL-3001"),
		OrderID:    proto.Uint64(2001),
		OrderIDEx:  proto.String("EXT-2001"),
		Code:       proto.String("HK.00700"),
		Name:       proto.String("Tencent"),
		TrdSide:    proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        proto.Float64(100),
		Price:      proto.Float64(319.8),
		CreateTime: proto.String("2026-05-20 09:35:00"),
		TrdMarket:  proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		Status:     proto.Int32(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setOrderFees([]*trdcommonpb.OrderFee{{
		OrderIDEx: proto.String("EXT-2001"),
		FeeAmount: proto.Float64(12.5),
		FeeList: []*trdcommonpb.OrderFeeItem{{
			Title: proto.String("BROKERAGE"),
			Value: proto.Float64(10),
		}, {
			Title: proto.String("STAMP_DUTY"),
			Value: proto.Float64(2.5),
		}},
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	server.setMarginRatios([]*trdgetmarginratiopb.MarginRatioInfo{{
		Security:        &qotcommonpb.Security{Market: proto.Int32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: proto.String("00700")},
		IsLongPermit:    proto.Bool(true),
		IsShortPermit:   proto.Bool(false),
		ShortFeeRate:    proto.Float64(1.25),
		AlertLongRatio:  proto.Float64(0.3),
		AlertShortRatio: proto.Float64(0.4),
		ImLongRatio:     proto.Float64(0.5),
		McmLongRatio:    proto.Float64(0.6),
		MmLongRatio:     proto.Float64(0.7),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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

func TestQueryBrokerCashFlowsReturnsFlowSummary(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setCashFlows([]*trdflowsummarypb.FlowSummaryInfo{{
		CashFlowID:        proto.Uint64(5001),
		ClearingDate:      proto.String("2026-05-20"),
		SettlementDate:    proto.String("2026-05-21"),
		Currency:          proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		CashFlowType:      proto.String("DIVIDEND"),
		CashFlowDirection: proto.Int32(int32(trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_In)),
		CashFlowAmount:    proto.Float64(88.8),
		CashFlowRemark:    proto.String("cash-flow-test"),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	server.setMaxTrdQtys(&trdcommonpb.MaxTrdQtys{
		MaxCashBuy:          proto.Float64(1000),
		MaxCashAndMarginBuy: proto.Float64(2000),
		MaxPositionSell:     proto.Float64(500),
		MaxSellShort:        proto.Float64(300),
		MaxBuyBack:          proto.Float64(150),
		LongRequiredIM:      proto.Float64(10),
		ShortRequiredIM:     proto.Float64(12),
		Session:             proto.Int32(int32(commonpb.Session_Session_RTH)),
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.setPlacedOrderResponse(9001, "FT-9001")
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
		RetType: proto.Int32(0),
		S2C: &notifypb.S2C{
			Type: proto.Int32(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
			ConnectStatus: &notifypb.ConnectStatus{
				QotLogined: proto.Bool(true),
				TrdLogined: proto.Bool(false),
			},
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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

func TestQueryTickersBatchesBasicQotRequests(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	tickers, err := ex.QueryTickers(t.Context(), "HK.00700", "US.NVDA")
	if err != nil {
		t.Fatalf("QueryTickers: %v", err)
	}
	if len(tickers) != 2 {
		t.Fatalf("expected 2 batched tickers, got %d", len(tickers))
	}
	if got := server.acceptCount(); got != 1 {
		t.Fatalf("expected one OpenD TCP session, got %d", got)
	}
	if got := server.subCallCount(); got != 1 {
		t.Fatalf("expected one batched Qot_Sub call, got %d", got)
	}
	if got := server.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one batched GetBasicQot call, got %d", got)
	}
	if _, ok := tickers["US.NVDA"]; !ok {
		t.Fatalf("expected batched quote for US.NVDA, got %#v", tickers)
	}
}

func TestQueryKLinesSplitsUSHistoricalRequestsBySessionAndMergesResults(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setHistoryPagesBySession(map[int32][][]*qotcommonpb.KLine{
		int32(commonpb.Session_Session_RTH): {
			{testHistoryKLine(time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC), 110)},
		},
		int32(commonpb.Session_Session_ETH): {
			{testHistoryKLine(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC), 100)},
		},
		int32(commonpb.Session_Session_ALL): {
			{
				testHistoryKLine(time.Date(2026, time.May, 20, 2, 0, 0, 0, time.UTC), 90),
				testHistoryKLine(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC), 95),
				testHistoryKLine(time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC), 105),
			},
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := time.Date(2026, time.May, 20, 8, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "US.NVDA", types.Interval1m, types.KLineQueryOptions{Limit: 3, StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 3 {
		t.Fatalf("expected three merged session klines, got %d", len(klines))
	}
	if got := server.historyKLCallCount(); got != 3 {
		t.Fatalf("expected three RequestHistoryKL calls, got %d", got)
	}
	if !server.lastHistoryExtendedTime() {
		t.Fatal("expected US intraday RequestHistoryKL to set extendedTime=true")
	}
	if got := server.historySessionCalls(); len(got) != 3 || got[0] != int32(commonpb.Session_Session_RTH) || got[1] != int32(commonpb.Session_Session_ETH) || got[2] != int32(commonpb.Session_Session_ALL) {
		t.Fatalf("expected RTH/ETH/ALL route calls, got %#v", got)
	}
	if got := klines[1].Open.Float64(); got != 100 {
		t.Fatalf("expected ETH route candle to win over ALL duplicate, got %v", got)
	}
	if got := klines[2].Open.Float64(); got != 110 {
		t.Fatalf("expected RTH route candle to win over ALL duplicate, got %v", got)
	}
	if session, ok := ex.ResolveKLineSession(klines[0]); !ok || session != MarketSessionOvernight {
		t.Fatalf("expected overnight session tag, got %s ok=%v", session, ok)
	}
	if session, ok := ex.ResolveKLineSession(klines[1]); !ok || session != MarketSessionPre {
		t.Fatalf("expected ETH route to resolve pre session, got %s ok=%v", session, ok)
	}
	if session, ok := ex.ResolveKLineSession(klines[2]); !ok || session != MarketSessionRegular {
		t.Fatalf("expected RTH route to resolve regular session, got %s ok=%v", session, ok)
	}
}

func TestResolveHistoricalRequestSessionUsesRouteForRTHAndOvernight(t *testing.T) {
	preClockKLine := types.KLine{
		Symbol:    "US.AAPL",
		StartTime: types.Time(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.May, 20, 10, 0, 59, 0, time.UTC)),
	}
	if session := resolveHistoricalMarketSession(commonpb.Session_Session_RTH, "US.AAPL", preClockKLine); session != MarketSessionRegular {
		t.Fatalf("expected RTH route to force regular session, got %s", session)
	}
	if session := resolveHistoricalMarketSession(commonpb.Session_Session_OVERNIGHT, "US.AAPL", preClockKLine); session != MarketSessionOvernight {
		t.Fatalf("expected overnight route to force overnight session, got %s", session)
	}
}

func TestQueryKLinesFallsBackToSessionAllWhenHistoricalRouteUnsupported(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setHistoryPagesBySession(map[int32][][]*qotcommonpb.KLine{
		int32(commonpb.Session_Session_RTH): {
			{testHistoryKLine(time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC), 110)},
		},
		int32(commonpb.Session_Session_ALL): {
			{
				testHistoryKLine(time.Date(2026, time.May, 20, 2, 0, 0, 0, time.UTC), 90),
				testHistoryKLine(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC), 100),
				testHistoryKLine(time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC), 110),
			},
		},
	})
	server.setHistorySessionError(int32(commonpb.Session_Session_ETH), 1, "session is invalid")
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := time.Date(2026, time.May, 20, 8, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "US.NVDA", types.Interval1m, types.KLineQueryOptions{Limit: 3, StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 3 {
		t.Fatalf("expected fallback Session_ALL history to return three klines, got %d", len(klines))
	}
	if got := server.historySessionCalls(); len(got) != 3 || got[0] != int32(commonpb.Session_Session_RTH) || got[1] != int32(commonpb.Session_Session_ETH) || got[2] != int32(commonpb.Session_Session_ALL) {
		t.Fatalf("expected RTH/ETH then fallback Session_ALL, got %#v", got)
	}
	if session, ok := ex.ResolveKLineSession(klines[0]); !ok || session != MarketSessionOvernight {
		t.Fatalf("expected fallback ALL route to classify overnight candle, got %s ok=%v", session, ok)
	}
}

func TestShouldFallbackHistoricalKLineSplitRecognizesChineseSupportedSessionsMessage(t *testing.T) {
	plan := historicalKLineRequestPlanAll()
	err := &historicalKLineRequestError{
		session: plan.session,
		retType: 1,
		errCode: 0,
		retMsg:  "获取历史K线的时段仅支持设置 RTH，ETH，ALL",
	}
	if !shouldFallbackHistoricalKLineSplit(err, plan) {
		t.Fatal("expected supported-session-list message to trigger fallback")
	}
}

func TestQueryKLinesNormalizesIntradayHistoryLabelToBucketStart(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	labelAt := time.Date(2026, time.May, 20, 10, 55, 0, 0, time.UTC)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(labelAt, 100)}})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := labelAt.Add(-time.Hour)
	end := labelAt.Add(time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1m, types.KLineQueryOptions{Limit: 1, StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 1 {
		t.Fatalf("expected one kline, got %d", len(klines))
	}

	wantStart := labelAt.Add(-time.Minute)
	wantEnd := labelAt.Add(-time.Millisecond)
	if !klines[0].StartTime.Time().Equal(wantStart) {
		t.Fatalf("StartTime = %s, want %s", klines[0].StartTime.Time(), wantStart)
	}
	if !klines[0].EndTime.Time().Equal(wantEnd) {
		t.Fatalf("EndTime = %s, want %s", klines[0].EndTime.Time(), wantEnd)
	}
}

func TestQueryKLinesKeepsDailyHistoryLabelAsBucketStart(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	labelAt := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(labelAt, 100)}})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := labelAt.Add(-24 * time.Hour)
	end := labelAt.Add(24 * time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1d, types.KLineQueryOptions{Limit: 1, StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 1 {
		t.Fatalf("expected one kline, got %d", len(klines))
	}
	if !klines[0].StartTime.Time().Equal(labelAt) {
		t.Fatalf("StartTime = %s, want %s", klines[0].StartTime.Time(), labelAt)
	}
}

func TestQueryKLinesFollowsHistoryPaginationAndKeepsLatestLimit(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	oldAt := time.Date(2026, time.May, 15, 10, 0, 0, 0, time.UTC)
	recentAt := time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC)
	server.setHistoryPages([][]*qotcommonpb.KLine{
		{
			testHistoryKLine(oldAt, 100),
			testHistoryKLine(oldAt.Add(5*time.Minute), 101),
		},
		{
			testHistoryKLine(recentAt, 200),
			testHistoryKLine(recentAt.Add(5*time.Minute), 201),
		},
	})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := oldAt.Add(-time.Hour)
	end := recentAt.Add(time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval5m, types.KLineQueryOptions{Limit: 2, StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if got := server.historyKLCallCount(); got != 2 {
		t.Fatalf("expected two paginated RequestHistoryKL calls, got %d", got)
	}
	if len(klines) != 2 {
		t.Fatalf("expected latest two klines, got %d", len(klines))
	}
	if !klines[0].StartTime.Time().Equal(recentAt.Add(-5*time.Minute)) || !klines[1].StartTime.Time().Equal(recentAt) {
		t.Fatalf("expected latest page to be retained, got %#v", klines)
	}
}

func TestQueryKLinesIncludesCurrentRealtimeBucketFromGetKL(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	historyLabelAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	currentLabelAt := historyLabelAt.Add(time.Minute)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(historyLabelAt, 100)}})
	server.setCurrentKLines([]*qotcommonpb.KLine{testCurrentKLine(currentLabelAt, 101, 106, 99, 103, 500)})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := historyLabelAt.Add(-time.Hour)
	end := currentLabelAt.Add(time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1m, types.KLineQueryOptions{Limit: 2, StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if got := server.currentKLCallCount(); got != 1 {
		t.Fatalf("expected one GetKL call, got %d", got)
	}
	if len(klines) != 2 {
		t.Fatalf("expected closed and current kline, got %d", len(klines))
	}

	if !klines[0].StartTime.Time().Equal(historyLabelAt.Add(-time.Minute)) {
		t.Fatalf("first StartTime = %s, want %s", klines[0].StartTime.Time(), historyLabelAt.Add(-time.Minute))
	}
	if !klines[1].StartTime.Time().Equal(historyLabelAt) {
		t.Fatalf("current StartTime = %s, want %s", klines[1].StartTime.Time(), historyLabelAt)
	}
	if klines[1].Open.Float64() != 101 || klines[1].High.Float64() != 106 || klines[1].Low.Float64() != 99 || klines[1].Close.Float64() != 103 {
		t.Fatalf("unexpected current kline OHLC: %#v", klines[1])
	}
	if klines[1].Volume.Float64() != 500 {
		t.Fatalf("current Volume = %v, want 500", klines[1].Volume.Float64())
	}
	if klines[1].Closed {
		t.Fatal("expected current GetKL candle to remain open")
	}
}

func TestStreamConnectEmitsBasicQotPushAsBBGOEvents(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	stream := NewStream(ex)
	stream.Subscribe(types.MarketTradeChannel, "HK.00700", types.SubscribeOptions{})
	trades := make(chan types.Trade, 1)
	bookTickers := make(chan types.BookTicker, 1)
	stream.OnMarketTrade(func(trade types.Trade) {
		trades <- trade
	})
	stream.OnBookTickerUpdate(func(bookTicker types.BookTicker) {
		bookTickers <- bookTicker
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := stream.Connect(ctx); err != nil {
		t.Fatalf("stream.Connect: %v", err)
	}
	defer stream.Close()

	select {
	case trade := <-trades:
		if trade.Symbol != "HK.00700" || trade.Price.Float64() != 700 {
			t.Fatalf("unexpected market trade: %+v", trade)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for market trade push")
	}

	select {
	case bookTicker := <-bookTickers:
		if bookTicker.Symbol != "HK.00700" || bookTicker.Buy.Float64() != 700 || bookTicker.Sell.Float64() != 700 {
			t.Fatalf("unexpected book ticker: %+v", bookTicker)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for book ticker push")
	}

	if got := server.pushSubCallCount(); got != 1 {
		t.Fatalf("expected one push Qot_Sub call, got %d", got)
	}
}

func TestStreamConnectRebuildsClosedCachedOpenDClient(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	if _, err := ex.QueryTicker(t.Context(), "HK.00700"); err != nil {
		t.Fatalf("QueryTicker: %v", err)
	}
	if client := ex.Client(); client != nil {
		_ = client.Close()
	}

	stream := NewStream(ex)
	stream.Subscribe(types.MarketTradeChannel, "HK.00700", types.SubscribeOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := stream.Connect(ctx); err != nil {
		t.Fatalf("stream.Connect after cached client close: %v", err)
	}
	defer stream.Close()

	if got := server.acceptCount(); got < 2 {
		t.Fatalf("expected stream to create a fresh OpenD session, got %d accepts", got)
	}
}

type quoteOpenDServer struct {
	addr                  string
	accepts               atomic.Int32
	initRecvNotify        atomic.Bool
	accountListCalls      atomic.Int32
	fundsCalls            atomic.Int32
	positionListCalls     atomic.Int32
	orderListCalls        atomic.Int32
	historyOrderListCalls atomic.Int32
	historyFillListCalls  atomic.Int32
	orderFeeCalls         atomic.Int32
	marginRatioCalls      atomic.Int32
	flowSummaryCalls      atomic.Int32
	maxTrdQtysCalls       atomic.Int32
	placeOrderCalls       atomic.Int32
	modifyOrderCalls      atomic.Int32
	qotSubCalls           atomic.Int32
	pushSubCalls          atomic.Int32
	basicQotCalls         atomic.Int32
	accountMu             sync.Mutex
	accounts              []*trdcommonpb.TrdAcc
	tradeMu               sync.Mutex
	funds                 *trdcommonpb.Funds
	positions             []*trdcommonpb.Position
	orders                []*trdcommonpb.Order
	historyOrders         []*trdcommonpb.Order
	orderFills            []*trdcommonpb.OrderFill
	historyFills          []*trdcommonpb.OrderFill
	orderFees             []*trdcommonpb.OrderFee
	marginRatios          []*trdgetmarginratiopb.MarginRatioInfo
	cashFlows             []*trdflowsummarypb.FlowSummaryInfo
	maxTrdQtys            *trdcommonpb.MaxTrdQtys
	placedOrderID         uint64
	placedOrderIDEx       string
	lastPlaceOrder        *trdplaceorderpb.C2S
	lastModifyOrder       *trdmodifyorderpb.C2S
	lastMaxTrdQtys        *trdgetmaxtrdqtyspb.C2S
	historyKLCalls        atomic.Int32
	currentKLCalls        atomic.Int32
	historyExtended       atomic.Bool
	historySession        atomic.Int32
	historyMu             sync.Mutex
	historyPages          [][]*qotcommonpb.KLine
	historyPagesBySession map[int32][][]*qotcommonpb.KLine
	historySessionErrors  map[int32]*historypb.Response
	historySessionCallLog []int32
	historyRouteCallCount map[int32]int
	currentKLines         []*qotcommonpb.KLine
	notifyMu              sync.Mutex
	notifyAfterInit       *notifypb.Response
	listener              net.Listener
	stopOnce              sync.Once
	shutdownCompleted     chan struct{}
}

func (s *quoteOpenDServer) setHistoryPages(pages [][]*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = pages
	s.historyPagesBySession = nil
	s.historySessionErrors = nil
	s.historySessionCallLog = nil
	s.historyRouteCallCount = nil
}

func (s *quoteOpenDServer) setHistoryPagesBySession(pages map[int32][][]*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = nil
	s.historySessionErrors = nil
	s.historySessionCallLog = nil
	s.historyRouteCallCount = make(map[int32]int, len(pages))
	s.historyPagesBySession = make(map[int32][][]*qotcommonpb.KLine, len(pages))
	for session, sessionPages := range pages {
		clonedPages := make([][]*qotcommonpb.KLine, 0, len(sessionPages))
		for _, page := range sessionPages {
			clonedPages = append(clonedPages, append([]*qotcommonpb.KLine(nil), page...))
		}
		s.historyPagesBySession[session] = clonedPages
	}
}

func (s *quoteOpenDServer) setHistorySessionError(session int32, retType int32, retMsg string) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	if s.historySessionErrors == nil {
		s.historySessionErrors = make(map[int32]*historypb.Response)
	}
	s.historySessionErrors[session] = &historypb.Response{
		RetType: proto.Int32(retType),
		RetMsg:  proto.String(retMsg),
	}
}

func (s *quoteOpenDServer) setCurrentKLines(klines []*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.currentKLines = klines
}

func (s *quoteOpenDServer) setNotifyAfterInit(response *notifypb.Response) {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	s.notifyAfterInit = response
}

func (s *quoteOpenDServer) setAccounts(accounts []*trdcommonpb.TrdAcc) {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	s.accounts = append([]*trdcommonpb.TrdAcc(nil), accounts...)
}

func (s *quoteOpenDServer) setFunds(funds *trdcommonpb.Funds) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.funds = funds
}

func (s *quoteOpenDServer) setPositions(positions []*trdcommonpb.Position) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.positions = append([]*trdcommonpb.Position(nil), positions...)
}

func (s *quoteOpenDServer) setOrders(orders []*trdcommonpb.Order) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.orders = append([]*trdcommonpb.Order(nil), orders...)
}

func (s *quoteOpenDServer) setHistoryOrders(orders []*trdcommonpb.Order) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.historyOrders = append([]*trdcommonpb.Order(nil), orders...)
}

func (s *quoteOpenDServer) setHistoryFills(fills []*trdcommonpb.OrderFill) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.historyFills = append([]*trdcommonpb.OrderFill(nil), fills...)
}

func (s *quoteOpenDServer) setOrderFills(fills []*trdcommonpb.OrderFill) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.orderFills = append([]*trdcommonpb.OrderFill(nil), fills...)
}

func (s *quoteOpenDServer) setOrderFees(fees []*trdcommonpb.OrderFee) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.orderFees = append([]*trdcommonpb.OrderFee(nil), fees...)
}

func (s *quoteOpenDServer) setMarginRatios(ratios []*trdgetmarginratiopb.MarginRatioInfo) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.marginRatios = append([]*trdgetmarginratiopb.MarginRatioInfo(nil), ratios...)
}

func (s *quoteOpenDServer) setCashFlows(flows []*trdflowsummarypb.FlowSummaryInfo) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.cashFlows = append([]*trdflowsummarypb.FlowSummaryInfo(nil), flows...)
}

func (s *quoteOpenDServer) setMaxTrdQtys(maxQtys *trdcommonpb.MaxTrdQtys) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.maxTrdQtys = maxQtys
}

func (s *quoteOpenDServer) setPlacedOrderResponse(orderID uint64, orderIDEx string) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.placedOrderID = orderID
	s.placedOrderIDEx = orderIDEx
}

func startQuoteOpenDServer(t *testing.T) *quoteOpenDServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &quoteOpenDServer{
		addr:              listener.Addr().String(),
		listener:          listener,
		shutdownCompleted: make(chan struct{}),
	}
	go server.acceptLoop()
	return server
}

func (s *quoteOpenDServer) stop() {
	s.stopOnce.Do(func() {
		_ = s.listener.Close()
		<-s.shutdownCompleted
	})
}

func (s *quoteOpenDServer) acceptLoop() {
	defer close(s.shutdownCompleted)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.accepts.Add(1)
		go s.handleConn(conn)
	}
}

func (s *quoteOpenDServer) handleConn(conn net.Conn) {
	defer conn.Close()
	for {
		header := make([]byte, codec.HeaderLen)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		bodyLen := int(uint32(header[12]) | uint32(header[13])<<8 | uint32(header[14])<<16 | uint32(header[15])<<24)
		packet := make([]byte, codec.HeaderLen+bodyLen)
		copy(packet, header)
		if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
			return
		}
		frame, err := codec.Decode(packet)
		if err != nil {
			return
		}

		var response proto.Message
		switch frame.Header.ProtoID {
		case opend.ProtoInitConnect:
			request := &initpb.Request{}
			_ = proto.Unmarshal(frame.Body, request)
			s.initRecvNotify.Store(request.GetC2S().GetRecvNotify())
			response = &initpb.Response{
				RetType: proto.Int32(0),
				S2C: &initpb.S2C{
					ServerVer:         proto.Int32(700),
					LoginUserID:       proto.Uint64(1),
					ConnID:            proto.Uint64(42),
					ConnAESKey:        proto.String("0123456789abcdef"),
					KeepAliveInterval: proto.Int32(10),
				},
			}
		case opend.ProtoQotSub:
			s.qotSubCalls.Add(1)
			request := &qotsubpb.Request{}
			_ = proto.Unmarshal(frame.Body, request)
			isPushSub := request.GetC2S().GetIsRegOrUnRegPush()
			if isPushSub {
				s.pushSubCalls.Add(1)
			}
			response = &qotsubpb.Response{RetType: proto.Int32(0)}
		case opend.ProtoTrdGetAccList:
			s.accountListCalls.Add(1)
			response = s.accountListResponse()
		case opend.ProtoTrdGetFunds:
			s.fundsCalls.Add(1)
			response = s.fundsResponse(frame.Body)
		case opend.ProtoTrdGetPositionList:
			s.positionListCalls.Add(1)
			response = s.positionListResponse(frame.Body)
		case opend.ProtoTrdGetOrderList:
			s.orderListCalls.Add(1)
			response = s.orderListResponse(frame.Body)
		case opend.ProtoTrdGetOrderFillList:
			s.orderListCalls.Add(1)
			response = s.orderFillListResponse(frame.Body)
		case opend.ProtoTrdGetHistoryOrderList:
			s.historyOrderListCalls.Add(1)
			response = s.historyOrderListResponse(frame.Body)
		case opend.ProtoTrdGetHistoryOrderFillList:
			s.historyFillListCalls.Add(1)
			response = s.historyOrderFillListResponse(frame.Body)
		case opend.ProtoTrdGetOrderFee:
			s.orderFeeCalls.Add(1)
			response = s.orderFeeResponse(frame.Body)
		case opend.ProtoTrdGetMarginRatio:
			s.marginRatioCalls.Add(1)
			response = s.marginRatioResponse(frame.Body)
		case opend.ProtoTrdFlowSummary:
			s.flowSummaryCalls.Add(1)
			response = s.flowSummaryResponse(frame.Body)
		case opend.ProtoTrdGetMaxTrdQtys:
			s.maxTrdQtysCalls.Add(1)
			response = s.maxTrdQtysResponse(frame.Body)
		case opend.ProtoTrdPlaceOrder:
			s.placeOrderCalls.Add(1)
			response = s.placeOrderResponse(frame.Body)
		case opend.ProtoTrdModifyOrder:
			s.modifyOrderCalls.Add(1)
			response = s.modifyOrderResponse(frame.Body)
		case opend.ProtoGetBasicQot:
			s.basicQotCalls.Add(1)
			response = s.basicQotResponse(frame.Body)
		case opend.ProtoGetKL:
			s.currentKLCalls.Add(1)
			response = s.currentKLResponse(frame.Body)
		case opend.ProtoRequestHistoryKL:
			s.historyKLCalls.Add(1)
			response = s.historyKLResponse(frame.Body)
		default:
			return
		}

		body, err := proto.Marshal(response)
		if err != nil {
			return
		}
		packet, err = codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
		if err != nil {
			return
		}
		if _, err := conn.Write(packet); err != nil {
			return
		}
		if frame.Header.ProtoID == opend.ProtoInitConnect {
			if err := s.writeNotifyAfterInit(conn); err != nil {
				return
			}
		}
		if frame.Header.ProtoID == opend.ProtoQotSub {
			request := &qotsubpb.Request{}
			_ = proto.Unmarshal(frame.Body, request)
			if request.GetC2S().GetIsRegOrUnRegPush() {
				if err := s.writeBasicQotPush(conn, request.GetC2S().GetSecurityList()); err != nil {
					return
				}
			}
		}
	}
}

func (s *quoteOpenDServer) writeNotifyAfterInit(conn net.Conn) error {
	s.notifyMu.Lock()
	response := s.notifyAfterInit
	s.notifyMu.Unlock()
	if response == nil {
		return nil
	}

	time.Sleep(25 * time.Millisecond)
	body, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	packet, err := codec.Encode(opend.ProtoNotify, 0, body)
	if err != nil {
		return err
	}
	_, err = conn.Write(packet)
	return err
}

func (s *quoteOpenDServer) historyKLResponse(body []byte) *historypb.Response {
	request := &historypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &historypb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.historyExtended.Store(request.GetC2S().GetExtendedTime())
	s.historySession.Store(request.GetC2S().GetSession())
	s.historyMu.Lock()
	s.historySessionCallLog = append(s.historySessionCallLog, request.GetC2S().GetSession())
	if response := s.historySessionErrors[request.GetC2S().GetSession()]; response != nil {
		s.historyMu.Unlock()
		return response
	}
	if len(s.historyPagesBySession) > 0 {
		session := request.GetC2S().GetSession()
		pages := s.historyPagesBySession[session]
		pageIndex := s.historyRouteCallCount[session]
		if pageIndex >= len(pages) && len(pages) > 0 {
			pageIndex = len(pages) - 1
		}
		s.historyRouteCallCount[session]++
		response := &historypb.Response{
			RetType: proto.Int32(0),
			S2C: &historypb.S2C{
				Security: request.GetC2S().GetSecurity(),
			},
		}
		if len(pages) > 0 {
			response.S2C.KlList = pages[pageIndex]
			if pageIndex < len(pages)-1 {
				response.S2C.NextReqKey = []byte{byte(pageIndex + 1)}
			}
		}
		s.historyMu.Unlock()
		return response
	}
	if len(s.historyPages) > 0 {
		pageIndex := int(s.historyKLCalls.Load()) - 1
		if pageIndex < 0 {
			pageIndex = 0
		}
		if pageIndex >= len(s.historyPages) {
			pageIndex = len(s.historyPages) - 1
		}
		response := &historypb.Response{
			RetType: proto.Int32(0),
			S2C: &historypb.S2C{
				Security: request.GetC2S().GetSecurity(),
				KlList:   s.historyPages[pageIndex],
			},
		}
		if pageIndex < len(s.historyPages)-1 {
			response.S2C.NextReqKey = []byte{byte(pageIndex + 1)}
		}
		s.historyMu.Unlock()
		return response
	}
	s.historyMu.Unlock()

	startAt := time.Date(2026, time.May, 20, 8, 0, 0, 0, time.UTC)
	return &historypb.Response{
		RetType: proto.Int32(0),
		S2C: &historypb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList: []*qotcommonpb.KLine{
				{
					Time:       proto.String(startAt.Format("2006-01-02 15:04:05")),
					Timestamp:  proto.Float64(float64(startAt.Unix())),
					IsBlank:    proto.Bool(false),
					OpenPrice:  proto.Float64(100),
					HighPrice:  proto.Float64(101),
					LowPrice:   proto.Float64(99),
					ClosePrice: proto.Float64(100.5),
					Volume:     proto.Int64(1000),
					Turnover:   proto.Float64(100500),
				},
			},
		},
	}
}

func (s *quoteOpenDServer) currentKLResponse(body []byte) *qotgetklpb.Response {
	request := &qotgetklpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetklpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	response := &qotgetklpb.Response{
		RetType: proto.Int32(0),
		S2C: &qotgetklpb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList:   s.currentKLines,
		},
	}
	return response
}

func testHistoryKLine(at time.Time, price float64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       proto.String(at.Format("2006-01-02 15:04:05")),
		Timestamp:  proto.Float64(float64(at.Unix())),
		IsBlank:    proto.Bool(false),
		OpenPrice:  proto.Float64(price),
		HighPrice:  proto.Float64(price + 1),
		LowPrice:   proto.Float64(price - 1),
		ClosePrice: proto.Float64(price + 0.5),
		Volume:     proto.Int64(1000),
		Turnover:   proto.Float64(price * 1000),
	}
}

func testCurrentKLine(at time.Time, open float64, high float64, low float64, close float64, volume int64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       proto.String(at.Format("2006-01-02 15:04:05")),
		Timestamp:  proto.Float64(float64(at.Unix())),
		IsBlank:    proto.Bool(false),
		OpenPrice:  proto.Float64(open),
		HighPrice:  proto.Float64(high),
		LowPrice:   proto.Float64(low),
		ClosePrice: proto.Float64(close),
		Volume:     proto.Int64(volume),
		Turnover:   proto.Float64(close * float64(volume)),
	}
}

func (s *quoteOpenDServer) writeBasicQotPush(conn net.Conn, securities []*qotcommonpb.Security) error {
	response := &qotupdatebasicqotpb.Response{
		RetType: proto.Int32(0),
		S2C:     &qotupdatebasicqotpb.S2C{BasicQotList: basicQotListForSecurities(securities)},
	}
	body, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	packet, err := codec.Encode(opend.ProtoQotUpdateBasicQot, 0, body)
	if err != nil {
		return err
	}
	_, err = conn.Write(packet)
	return err
}

func (s *quoteOpenDServer) basicQotResponse(body []byte) *qotgetbasicqotpb.Response {
	request := &qotgetbasicqotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetbasicqotpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}

	quotes := basicQotListForSecurities(request.GetC2S().GetSecurityList())

	return &qotgetbasicqotpb.Response{
		RetType: proto.Int32(0),
		S2C: &qotgetbasicqotpb.S2C{
			BasicQotList: quotes,
		},
	}
}

func (s *quoteOpenDServer) accountListResponse() *trdgetacclistpb.Response {
	s.accountMu.Lock()
	accounts := append([]*trdcommonpb.TrdAcc(nil), s.accounts...)
	s.accountMu.Unlock()
	return &trdgetacclistpb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgetacclistpb.S2C{
			AccList: accounts,
		},
	}
}

func (s *quoteOpenDServer) fundsResponse(body []byte) *trdgetfundspb.Response {
	request := &trdgetfundspb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetfundspb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	funds := s.funds
	s.tradeMu.Unlock()
	return &trdgetfundspb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgetfundspb.S2C{
			Header: normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			Funds:  normalizeTestFunds(funds),
		},
	}
}

func (s *quoteOpenDServer) positionListResponse(body []byte) *trdgetpositionlistpb.Response {
	request := &trdgetpositionlistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetpositionlistpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	positions := append([]*trdcommonpb.Position(nil), s.positions...)
	s.tradeMu.Unlock()
	normalized := make([]*trdcommonpb.Position, 0, len(positions))
	for _, position := range positions {
		normalized = append(normalized, normalizeTestPosition(position))
	}
	return &trdgetpositionlistpb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgetpositionlistpb.S2C{
			Header:       normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			PositionList: normalized,
		},
	}
}

func (s *quoteOpenDServer) orderListResponse(body []byte) *trdgetorderlistpb.Response {
	request := &trdgetorderlistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetorderlistpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	orders := append([]*trdcommonpb.Order(nil), s.orders...)
	s.tradeMu.Unlock()
	normalizedOrders := make([]*trdcommonpb.Order, 0, len(orders))
	for _, order := range orders {
		normalizedOrders = append(normalizedOrders, normalizeTestOrder(order))
	}
	orders = normalizedOrders
	if codes := request.GetC2S().GetFilterConditions().GetCodeList(); len(codes) > 0 {
		filtered := make([]*trdcommonpb.Order, 0, len(orders))
		for _, order := range orders {
			if order == nil {
				continue
			}
			for _, code := range codes {
				if strings.EqualFold(order.GetCode(), code) {
					filtered = append(filtered, order)
					break
				}
			}
		}
		orders = filtered
	}
	return &trdgetorderlistpb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgetorderlistpb.S2C{
			Header:    normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderList: orders,
		},
	}
}

func (s *quoteOpenDServer) historyOrderListResponse(body []byte) *trdgethistoryorderlistpb.Response {
	request := &trdgethistoryorderlistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgethistoryorderlistpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	orders := append([]*trdcommonpb.Order(nil), s.historyOrders...)
	s.tradeMu.Unlock()
	normalizedOrders := make([]*trdcommonpb.Order, 0, len(orders))
	for _, order := range orders {
		normalizedOrders = append(normalizedOrders, normalizeTestOrder(order))
	}
	orders = filterTestOrdersByConditions(normalizedOrders, request.GetC2S().GetFilterConditions(), request.GetC2S().GetFilterStatusList())
	return &trdgethistoryorderlistpb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgethistoryorderlistpb.S2C{
			Header:    normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderList: orders,
		},
	}
}

func (s *quoteOpenDServer) historyOrderFillListResponse(body []byte) *trdgethistoryorderfilllistpb.Response {
	request := &trdgethistoryorderfilllistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgethistoryorderfilllistpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	fills := append([]*trdcommonpb.OrderFill(nil), s.historyFills...)
	s.tradeMu.Unlock()
	filtered := filterTestFillsByConditions(fills, request.GetC2S().GetFilterConditions())
	return &trdgethistoryorderfilllistpb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgethistoryorderfilllistpb.S2C{
			Header:        normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderFillList: filtered,
		},
	}
}

func (s *quoteOpenDServer) orderFillListResponse(body []byte) *trdgetorderfilllistpb.Response {
	request := &trdgetorderfilllistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetorderfilllistpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	fills := append([]*trdcommonpb.OrderFill(nil), s.orderFills...)
	s.tradeMu.Unlock()
	filtered := filterTestFillsByConditions(fills, request.GetC2S().GetFilterConditions())
	return &trdgetorderfilllistpb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgetorderfilllistpb.S2C{
			Header:        normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderFillList: filtered,
		},
	}
}

func (s *quoteOpenDServer) orderFeeResponse(body []byte) *trdgetorderfeepb.Response {
	request := &trdgetorderfeepb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetorderfeepb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	fees := append([]*trdcommonpb.OrderFee(nil), s.orderFees...)
	s.tradeMu.Unlock()
	if ids := request.GetC2S().GetOrderIdExList(); len(ids) > 0 {
		filtered := make([]*trdcommonpb.OrderFee, 0, len(fees))
		for _, fee := range fees {
			if fee == nil {
				continue
			}
			for _, id := range ids {
				if strings.EqualFold(strings.TrimSpace(fee.GetOrderIDEx()), strings.TrimSpace(id)) {
					filtered = append(filtered, fee)
					break
				}
			}
		}
		fees = filtered
	}
	return &trdgetorderfeepb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgetorderfeepb.S2C{
			Header:       normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderFeeList: fees,
		},
	}
}

func (s *quoteOpenDServer) marginRatioResponse(body []byte) *trdgetmarginratiopb.Response {
	request := &trdgetmarginratiopb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetmarginratiopb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	ratios := append([]*trdgetmarginratiopb.MarginRatioInfo(nil), s.marginRatios...)
	s.tradeMu.Unlock()
	return &trdgetmarginratiopb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgetmarginratiopb.S2C{
			Header:              normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			MarginRatioInfoList: ratios,
		},
	}
}

func (s *quoteOpenDServer) flowSummaryResponse(body []byte) *trdflowsummarypb.Response {
	request := &trdflowsummarypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdflowsummarypb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	flows := append([]*trdflowsummarypb.FlowSummaryInfo(nil), s.cashFlows...)
	s.tradeMu.Unlock()
	if direction := request.GetC2S().GetCashFlowDirection(); direction != 0 {
		filtered := make([]*trdflowsummarypb.FlowSummaryInfo, 0, len(flows))
		for _, flow := range flows {
			if flow != nil && flow.GetCashFlowDirection() == direction {
				filtered = append(filtered, flow)
			}
		}
		flows = filtered
	}
	return &trdflowsummarypb.Response{
		RetType: proto.Int32(0),
		S2C: &trdflowsummarypb.S2C{
			Header:              normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			FlowSummaryInfoList: flows,
		},
	}
}

func (s *quoteOpenDServer) maxTrdQtysResponse(body []byte) *trdgetmaxtrdqtyspb.Response {
	request := &trdgetmaxtrdqtyspb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetmaxtrdqtyspb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.tradeMu.Lock()
	if request.GetC2S() != nil {
		s.lastMaxTrdQtys = proto.Clone(request.GetC2S()).(*trdgetmaxtrdqtyspb.C2S)
	}
	maxQtys := s.maxTrdQtys
	s.tradeMu.Unlock()
	return &trdgetmaxtrdqtyspb.Response{
		RetType: proto.Int32(0),
		S2C: &trdgetmaxtrdqtyspb.S2C{
			Header:     normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			MaxTrdQtys: maxQtys,
		},
	}
}

func (s *quoteOpenDServer) placeOrderResponse(body []byte) *trdplaceorderpb.Response {
	request := &trdplaceorderpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdplaceorderpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	if request.GetC2S() == nil {
		return &trdplaceorderpb.Response{RetType: proto.Int32(1), RetMsg: proto.String("missing place order payload")}
	}
	s.tradeMu.Lock()
	s.lastPlaceOrder = proto.Clone(request.GetC2S()).(*trdplaceorderpb.C2S)
	orderID := s.placedOrderID
	orderIDEx := s.placedOrderIDEx
	s.tradeMu.Unlock()
	if orderID == 0 {
		orderID = 9001
	}
	if orderIDEx == "" {
		orderIDEx = strconv.FormatUint(orderID, 10)
	}
	if request.GetC2S().GetPacketID().GetConnID() == 0 {
		return &trdplaceorderpb.Response{RetType: proto.Int32(1), RetMsg: proto.String("missing packet id connID")}
	}
	return &trdplaceorderpb.Response{
		RetType: proto.Int32(0),
		S2C: &trdplaceorderpb.S2C{
			Header:    normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderID:   proto.Uint64(orderID),
			OrderIDEx: proto.String(orderIDEx),
		},
	}
}

func (s *quoteOpenDServer) modifyOrderResponse(body []byte) *trdmodifyorderpb.Response {
	request := &trdmodifyorderpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdmodifyorderpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	if request.GetC2S() == nil {
		return &trdmodifyorderpb.Response{RetType: proto.Int32(1), RetMsg: proto.String("missing modify order payload")}
	}
	s.tradeMu.Lock()
	s.lastModifyOrder = proto.Clone(request.GetC2S()).(*trdmodifyorderpb.C2S)
	s.tradeMu.Unlock()
	if request.GetC2S().GetPacketID().GetConnID() == 0 {
		return &trdmodifyorderpb.Response{RetType: proto.Int32(1), RetMsg: proto.String("missing packet id connID")}
	}
	return &trdmodifyorderpb.Response{
		RetType: proto.Int32(0),
		S2C: &trdmodifyorderpb.S2C{
			Header:    normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderID:   proto.Uint64(request.GetC2S().GetOrderID()),
			OrderIDEx: proto.String(strconv.FormatUint(request.GetC2S().GetOrderID(), 10)),
		},
	}
}

func basicQotListForSecurities(securities []*qotcommonpb.Security) []*qotcommonpb.BasicQot {
	quotes := make([]*qotcommonpb.BasicQot, 0, len(securities))
	baseQuoteTime := time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC)
	for index, security := range securities {
		price := 700.0 + float64(index)
		quotes = append(quotes, &qotcommonpb.BasicQot{
			Security:        security,
			IsSuspended:     proto.Bool(false),
			ListTime:        proto.String("2020-01-01"),
			PriceSpread:     proto.Float64(0.01),
			UpdateTime:      proto.String(baseQuoteTime.Format("2006-01-02 15:04:05")),
			HighPrice:       proto.Float64(price + 1),
			OpenPrice:       proto.Float64(price - 1),
			LowPrice:        proto.Float64(price - 2),
			CurPrice:        proto.Float64(price),
			LastClosePrice:  proto.Float64(price - 0.5),
			Volume:          proto.Int64(1000 + int64(index)*10),
			Turnover:        proto.Float64(price * 1000),
			TurnoverRate:    proto.Float64(1.25),
			Amplitude:       proto.Float64(2.5),
			UpdateTimestamp: proto.Float64(float64(baseQuoteTime.Unix())),
		})
	}
	return quotes
}

func (s *quoteOpenDServer) acceptCount() int {
	return int(s.accepts.Load())
}

func (s *quoteOpenDServer) lastInitRecvNotify() bool {
	return s.initRecvNotify.Load()
}

func (s *quoteOpenDServer) subCallCount() int {
	return int(s.qotSubCalls.Load())
}

func (s *quoteOpenDServer) accountListCallCount() int {
	return int(s.accountListCalls.Load())
}

func (s *quoteOpenDServer) fundsCallCount() int {
	return int(s.fundsCalls.Load())
}

func (s *quoteOpenDServer) orderListCallCount() int {
	return int(s.orderListCalls.Load())
}

func (s *quoteOpenDServer) placeOrderCallCount() int {
	return int(s.placeOrderCalls.Load())
}

func (s *quoteOpenDServer) modifyOrderCallCount() int {
	return int(s.modifyOrderCalls.Load())
}

func (s *quoteOpenDServer) lastPlaceOrderRequest() *trdplaceorderpb.C2S {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	if s.lastPlaceOrder == nil {
		return nil
	}
	return proto.Clone(s.lastPlaceOrder).(*trdplaceorderpb.C2S)
}

func (s *quoteOpenDServer) lastModifyOrderRequest() *trdmodifyorderpb.C2S {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	if s.lastModifyOrder == nil {
		return nil
	}
	return proto.Clone(s.lastModifyOrder).(*trdmodifyorderpb.C2S)
}

func (s *quoteOpenDServer) pushSubCallCount() int {
	return int(s.pushSubCalls.Load())
}

func (s *quoteOpenDServer) basicQotCallCount() int {
	return int(s.basicQotCalls.Load())
}

func (s *quoteOpenDServer) historyKLCallCount() int {
	return int(s.historyKLCalls.Load())
}

func (s *quoteOpenDServer) currentKLCallCount() int {
	return int(s.currentKLCalls.Load())
}

func (s *quoteOpenDServer) lastHistoryExtendedTime() bool {
	return s.historyExtended.Load()
}

func (s *quoteOpenDServer) lastHistorySession() int32 {
	return s.historySession.Load()
}

func (s *quoteOpenDServer) historySessionCalls() []int32 {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	return append([]int32(nil), s.historySessionCallLog...)
}

func (s *quoteOpenDServer) historyOrderListCallCount() int {
	return int(s.historyOrderListCalls.Load())
}

func (s *quoteOpenDServer) historyOrderFillListCallCount() int {
	return int(s.historyFillListCalls.Load())
}

func (s *quoteOpenDServer) orderFeeCallCount() int {
	return int(s.orderFeeCalls.Load())
}

func (s *quoteOpenDServer) marginRatioCallCount() int {
	return int(s.marginRatioCalls.Load())
}

func (s *quoteOpenDServer) flowSummaryCallCount() int {
	return int(s.flowSummaryCalls.Load())
}

func (s *quoteOpenDServer) maxTrdQtysCallCount() int {
	return int(s.maxTrdQtysCalls.Load())
}

func (s *quoteOpenDServer) lastMaxTrdQtysRequest() *trdgetmaxtrdqtyspb.C2S {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	if s.lastMaxTrdQtys == nil {
		return nil
	}
	return proto.Clone(s.lastMaxTrdQtys).(*trdgetmaxtrdqtyspb.C2S)
}

func normalizeTestFunds(funds *trdcommonpb.Funds) *trdcommonpb.Funds {
	if funds == nil {
		funds = &trdcommonpb.Funds{}
	}
	clone := proto.Clone(funds).(*trdcommonpb.Funds)
	if clone.Power == nil {
		clone.Power = proto.Float64(0)
	}
	if clone.TotalAssets == nil {
		clone.TotalAssets = proto.Float64(0)
	}
	if clone.Cash == nil {
		clone.Cash = proto.Float64(0)
	}
	if clone.MarketVal == nil {
		clone.MarketVal = proto.Float64(0)
	}
	if clone.FrozenCash == nil {
		clone.FrozenCash = proto.Float64(0)
	}
	if clone.DebtCash == nil {
		clone.DebtCash = proto.Float64(0)
	}
	if clone.AvlWithdrawalCash == nil {
		clone.AvlWithdrawalCash = proto.Float64(0)
	}
	return clone
}

func normalizeTestTrdHeader(header *trdcommonpb.TrdHeader) *trdcommonpb.TrdHeader {
	if header == nil {
		header = &trdcommonpb.TrdHeader{}
	}
	clone := proto.Clone(header).(*trdcommonpb.TrdHeader)
	if clone.TrdEnv == nil {
		clone.TrdEnv = proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate))
	}
	if clone.AccID == nil {
		clone.AccID = proto.Uint64(1001)
	}
	if clone.TrdMarket == nil {
		clone.TrdMarket = proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK))
	}
	return clone
}

func normalizeTestPosition(position *trdcommonpb.Position) *trdcommonpb.Position {
	if position == nil {
		position = &trdcommonpb.Position{}
	}
	clone := proto.Clone(position).(*trdcommonpb.Position)
	if clone.PositionID == nil {
		clone.PositionID = proto.Uint64(1)
	}
	if clone.PositionSide == nil {
		clone.PositionSide = proto.Int32(1)
	}
	if clone.Code == nil {
		clone.Code = proto.String("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = proto.String(clone.GetCode())
	}
	if clone.Qty == nil {
		clone.Qty = proto.Float64(0)
	}
	if clone.CanSellQty == nil {
		clone.CanSellQty = proto.Float64(0)
	}
	if clone.Price == nil {
		clone.Price = proto.Float64(0)
	}
	if clone.Val == nil {
		clone.Val = proto.Float64(0)
	}
	if clone.PlVal == nil {
		clone.PlVal = proto.Float64(0)
	}
	return clone
}

func normalizeTestOrder(order *trdcommonpb.Order) *trdcommonpb.Order {
	if order == nil {
		order = &trdcommonpb.Order{}
	}
	clone := proto.Clone(order).(*trdcommonpb.Order)
	if clone.TrdSide == nil {
		clone.TrdSide = proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy))
	}
	if clone.OrderType == nil {
		clone.OrderType = proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal))
	}
	if clone.OrderStatus == nil {
		clone.OrderStatus = proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted))
	}
	if clone.OrderID == nil {
		clone.OrderID = proto.Uint64(1)
	}
	if clone.OrderIDEx == nil {
		clone.OrderIDEx = proto.String(strconv.FormatUint(clone.GetOrderID(), 10))
	}
	if clone.Code == nil {
		clone.Code = proto.String("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = proto.String(clone.GetCode())
	}
	if clone.Qty == nil {
		clone.Qty = proto.Float64(0)
	}
	if clone.CreateTime == nil {
		clone.CreateTime = proto.String("2026-05-20 09:30:00")
	}
	if clone.UpdateTime == nil {
		clone.UpdateTime = proto.String(clone.GetCreateTime())
	}
	return clone
}

func filterTestOrdersByConditions(orders []*trdcommonpb.Order, filter *trdcommonpb.TrdFilterConditions, statuses []int32) []*trdcommonpb.Order {
	filtered := make([]*trdcommonpb.Order, 0, len(orders))
	for _, order := range orders {
		if order == nil {
			continue
		}
		if filter != nil && len(filter.GetCodeList()) > 0 {
			matchedCode := false
			for _, code := range filter.GetCodeList() {
				if strings.EqualFold(strings.TrimSpace(order.GetCode()), strings.TrimSpace(code)) {
					matchedCode = true
					break
				}
			}
			if !matchedCode {
				continue
			}
		}
		if len(statuses) > 0 {
			matchedStatus := false
			for _, status := range statuses {
				if order.GetOrderStatus() == status {
					matchedStatus = true
					break
				}
			}
			if !matchedStatus {
				continue
			}
		}
		filtered = append(filtered, order)
	}
	return filtered
}

func filterTestFillsByConditions(fills []*trdcommonpb.OrderFill, filter *trdcommonpb.TrdFilterConditions) []*trdcommonpb.OrderFill {
	filtered := make([]*trdcommonpb.OrderFill, 0, len(fills))
	for _, fill := range fills {
		if fill == nil {
			continue
		}
		if filter != nil && len(filter.GetCodeList()) > 0 {
			matchedCode := false
			for _, code := range filter.GetCodeList() {
				if strings.EqualFold(strings.TrimSpace(fill.GetCode()), strings.TrimSpace(code)) {
					matchedCode = true
					break
				}
			}
			if !matchedCode {
				continue
			}
		}
		filtered = append(filtered, fill)
	}
	return filtered
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for !condition() {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for condition")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestSessionFromExtendedBlocksClockGuardsStaleExtendedData(t *testing.T) {
	priceOf := func(v float64) *ExtendedMarketQuote {
		p := decimal.NewFromFloat(v)
		return &ExtendedMarketQuote{Price: &p}
	}
	// Use a Tuesday in early January (EST, no DST ambiguity).
	// 10:00 UTC = 05:00 EST (pre-market window).
	preMarketClock := time.Date(2025, time.January, 7, 10, 0, 0, 0, time.UTC)
	// 16:00 UTC = 11:00 EST (regular session).
	regularClock := time.Date(2025, time.January, 7, 16, 0, 0, 0, time.UTC)
	// 22:00 UTC = 17:00 EST (after-hours).
	afterClock := time.Date(2025, time.January, 7, 22, 0, 0, 0, time.UTC)
	// 02:00 UTC = 21:00 EST previous day (overnight).
	overnightClock := time.Date(2025, time.January, 8, 2, 0, 0, 0, time.UTC)

	stalePre := priceOf(195.0)
	staleAfter := priceOf(198.0)
	staleOvernight := priceOf(200.0)

	cases := []struct {
		name      string
		now       time.Time
		pre       *ExtendedMarketQuote
		after     *ExtendedMarketQuote
		overnight *ExtendedMarketQuote
		want      MarketSession
	}{
		{
			name:      "regular clock ignores stale overnight and after blocks",
			now:       regularClock,
			pre:       stalePre,
			after:     staleAfter,
			overnight: staleOvernight,
			want:      MarketSessionRegular,
		},
		{
			name:      "pre-market clock ignores stale overnight block",
			now:       preMarketClock,
			pre:       priceOf(196.5),
			after:     staleAfter,
			overnight: staleOvernight,
			want:      MarketSessionPre,
		},
		{
			name:      "after clock with after data returns after",
			now:       afterClock,
			pre:       stalePre,
			after:     priceOf(199.5),
			overnight: staleOvernight,
			want:      MarketSessionAfter,
		},
		{
			name:      "overnight clock with overnight data returns overnight",
			now:       overnightClock,
			pre:       stalePre,
			after:     staleAfter,
			overnight: priceOf(201.25),
			want:      MarketSessionOvernight,
		},
		{
			name:      "after clock without after data falls back to clock",
			now:       afterClock,
			pre:       nil,
			after:     nil,
			overnight: nil,
			want:      MarketSessionAfter,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sessionFromExtendedBlocksAt("US.AAPL", tc.pre, tc.after, tc.overnight, tc.now)
			if got != tc.want {
				t.Fatalf("session=%s, want %s", got, tc.want)
			}
		})
	}
}
