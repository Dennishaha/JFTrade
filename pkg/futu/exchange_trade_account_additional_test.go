package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func valuePtr[T any](value T) *T {
	return &value
}

func TestDiscoverAccountsDeduplicatesAndFallsBackToCardIdentifier(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{
		{
			TrdEnv:            valuePtr(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             valuePtr(uint64(0)),
			CardNum:           valuePtr("SIM-CARD"),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK), int32(trdcommonpb.TrdMarket_TrdMarket_HK), 999},
			AccType:           valuePtr(int32(999)),
			SecurityFirm:      valuePtr(int32(999)),
		},
		{
			TrdEnv:            valuePtr(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             valuePtr(uint64(0)),
			CardNum:           valuePtr("SIM-CARD"),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
			AccType:           valuePtr(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
		},
		{
			TrdEnv:            valuePtr(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
			AccID:             valuePtr(uint64(1002)),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
			AccType:           valuePtr(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
			SecurityFirm:      valuePtr(int32(trdcommonpb.SecurityFirm_SecurityFirm_FutuSecurities)),
			SimAccType:        valuePtr(int32(trdcommonpb.SimAccType_SimAccType_Stock)),
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	accounts, err := ex.DiscoverAccounts(t.Context())
	if err != nil {
		t.Fatalf("DiscoverAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 deduplicated accounts, got %#v", accounts)
	}

	if got := accounts[0]; got.AccountID != "1002" || got.TradingEnvironment != "REAL" || got.AccountType != "MARGIN" {
		t.Fatalf("unexpected real account normalization: %#v", got)
	}
	if got := accounts[1]; got.AccountID != "SIM-CARD" || got.TradingEnvironment != "SIMULATE" {
		t.Fatalf("unexpected simulated account fallback identifier: %#v", got)
	}
	if got := accounts[1].AccountType; got != "UNKNOWN" {
		t.Fatalf("AccountType = %q, want UNKNOWN for unknown enum", got)
	}
	if accounts[1].SecurityFirm != nil {
		t.Fatalf("SecurityFirm = %#v, want nil for unknown enum", accounts[1].SecurityFirm)
	}
	if got := accounts[1].MarketAuthorities; len(got) != 1 || got[0] != "HK" {
		t.Fatalf("MarketAuthorities = %#v, want deduped [HK]", got)
	}
}

func TestResolveTradeMarketCoversRequestedAndFallbackBranches(t *testing.T) {
	account := &trdcommonpb.TrdAcc{
		TrdMarketAuthList: []int32{
			int32(trdcommonpb.TrdMarket_TrdMarket_Unknown),
			int32(trdcommonpb.TrdMarket_TrdMarket_US),
		},
	}

	market, rawMarket, ok, err := resolveTradeMarket(account, "us")
	if err != nil || !ok {
		t.Fatalf("resolveTradeMarket(requested us) = %q %d %v %v", market, rawMarket, ok, err)
	}
	if market != "US" || rawMarket != int32(trdcommonpb.TrdMarket_TrdMarket_US) {
		t.Fatalf("resolveTradeMarket(requested us) = %q %d, want US / TrdMarket_US", market, rawMarket)
	}

	market, _, ok, err = resolveTradeMarket(&trdcommonpb.TrdAcc{
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
	}, "US")
	if err != nil || ok || market != "" {
		t.Fatalf("resolveTradeMarket(missing requested auth) = %q %v %v, want empty false nil", market, ok, err)
	}

	market, rawMarket, ok, err = resolveTradeMarket(&trdcommonpb.TrdAcc{
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
	}, "")
	if err != nil || !ok {
		t.Fatalf("resolveTradeMarket(default auth) = %q %d %v %v", market, rawMarket, ok, err)
	}
	if market != "US" || rawMarket != int32(trdcommonpb.TrdMarket_TrdMarket_US) {
		t.Fatalf("resolveTradeMarket(default auth) = %q %d, want first valid US auth", market, rawMarket)
	}

	market, rawMarket, ok, err = resolveTradeMarket(&trdcommonpb.TrdAcc{}, "")
	if err != nil || !ok {
		t.Fatalf("resolveTradeMarket(empty auth) = %q %d %v %v", market, rawMarket, ok, err)
	}
	if market != "HK" || rawMarket != int32(trdcommonpb.TrdMarket_TrdMarket_HK) {
		t.Fatalf("resolveTradeMarket(empty auth) = %q %d, want HK default", market, rawMarket)
	}

	if _, _, ok, err = resolveTradeMarket(&trdcommonpb.TrdAcc{}, "bad"); err == nil || ok || !strings.Contains(err.Error(), "unsupported market") {
		t.Fatalf("resolveTradeMarket(unsupported) err = %v, ok = %v", err, ok)
	}
}

func TestQueryBrokerOrdersFiltersAndSortsWorkingOrders(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{
		{
			TrdEnv:            valuePtr(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
			AccID:             valuePtr(uint64(1001)),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           valuePtr(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
		},
		{
			TrdEnv:            valuePtr(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
			AccID:             valuePtr(uint64(1002)),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           valuePtr(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
		},
	})
	server.setOrders([]*trdcommonpb.Order{
		{
			OrderID:     valuePtr(uint64(2001)),
			Code:        valuePtr("hk.00700"),
			OrderStatus: valuePtr(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
			Qty:         valuePtr(100.0),
			CreateTime:  valuePtr("2026-05-20 09:30:00"),
			UpdateTime:  valuePtr("2026-05-20 09:31:00"),
			TrdMarket:   valuePtr(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		{
			OrderID:         valuePtr(uint64(2002)),
			Code:            valuePtr("hk.00700"),
			OrderStatus:     valuePtr(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_Part)),
			Qty:             valuePtr(200.0),
			CreateTimestamp: valuePtr(float64(time.Date(2026, time.May, 20, 9, 32, 0, 0, time.UTC).Unix())),
			UpdateTimestamp: valuePtr(float64(time.Date(2026, time.May, 20, 9, 32, 0, 0, time.UTC).Unix())),
			TrdMarket:       valuePtr(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		{
			OrderID:     valuePtr(uint64(2003)),
			Code:        valuePtr("HK.00700"),
			OrderStatus: valuePtr(int32(trdcommonpb.OrderStatus_OrderStatus_Cancelled_All)),
			Qty:         valuePtr(50.0),
			UpdateTime:  valuePtr("2026-05-20 09:33:00"),
			TrdMarket:   valuePtr(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		},
		{
			OrderID:     valuePtr(uint64(2004)),
			Code:        valuePtr("US.AAPL"),
			OrderStatus: valuePtr(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
			Qty:         valuePtr(10.0),
			UpdateTime:  valuePtr("2026-05-20 09:34:00"),
			TrdMarket:   valuePtr(int32(trdcommonpb.TrdMarket_TrdMarket_US)),
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	orders, err := ex.QueryBrokerOrders(t.Context(), BrokerReadQuery{Market: "HK"}, " hk.00700 ")
	if err != nil {
		t.Fatalf("QueryBrokerOrders: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("expected 2 matching working orders, got %#v", orders)
	}
	if got := orders[0].BrokerOrderID; got != "2002" {
		t.Fatalf("first BrokerOrderID = %q, want newest working order 2002", got)
	}
	if got := orders[1].BrokerOrderID; got != "2001" {
		t.Fatalf("second BrokerOrderID = %q, want 2001", got)
	}
	if got := orders[0].TradingEnvironment; got != "SIMULATE" {
		t.Fatalf("TradingEnvironment = %q, want default simulated account preference", got)
	}
	if got := orders[0].AccountID; got != "1002" {
		t.Fatalf("AccountID = %q, want simulated account 1002", got)
	}
	if got := orders[0].Symbol; got != "HK.00700" {
		t.Fatalf("Symbol = %q, want trimmed uppercase symbol", got)
	}
}

func TestPlaceOrderRequestFromSubmitOrderCoversValidationAndRemarkSemantics(t *testing.T) {
	hkAccount := resolvedTradeAccount{
		AccountID:          "1001",
		TradingEnvironment: "REAL",
		Market:             "HK",
		protoAccountID:     1001,
		protoTrdEnv:        int32(trdcommonpb.TrdEnv_TrdEnv_Real),
		protoTrdMarket:     int32(trdcommonpb.TrdMarket_TrdMarket_HK),
	}
	usAccount := resolvedTradeAccount{
		AccountID:          "1002",
		TradingEnvironment: "REAL",
		Market:             "US",
		protoAccountID:     1002,
		protoTrdEnv:        int32(trdcommonpb.TrdEnv_TrdEnv_Real),
		protoTrdMarket:     int32(trdcommonpb.TrdMarket_TrdMarket_US),
	}

	session := "RTH"
	fillOutsideRTH := true

	if _, err := placeOrderRequestFromSubmitOrder(hkAccount, types.SubmitOrder{
		Symbol:   "HK.00700",
		Side:     types.SideTypeBuy,
		Type:     types.OrderTypeLimit,
		Price:    fixedpoint.NewFromFloat(320),
		Quantity: fixedpoint.NewFromFloat(100),
	}, BrokerPlaceOrderQuery{Session: &session}); err == nil || !strings.Contains(err.Error(), "session is supported for US orders only") {
		t.Fatalf("non-US session err = %v", err)
	}

	if _, err := placeOrderRequestFromSubmitOrder(hkAccount, types.SubmitOrder{
		Symbol:   "HK.00700",
		Side:     types.SideTypeBuy,
		Type:     types.OrderTypeLimit,
		Price:    fixedpoint.NewFromFloat(320),
		Quantity: fixedpoint.NewFromFloat(100),
	}, BrokerPlaceOrderQuery{FillOutsideRTH: &fillOutsideRTH}); err == nil || !strings.Contains(err.Error(), "fillOutsideRTH is supported for US orders only") {
		t.Fatalf("non-US fillOutsideRTH err = %v", err)
	}

	if _, err := placeOrderRequestFromSubmitOrder(usAccount, types.SubmitOrder{
		Symbol:      "US.AAPL",
		Side:        types.SideTypeBuy,
		Type:        types.OrderTypeLimit,
		Price:       fixedpoint.NewFromFloat(180),
		Quantity:    fixedpoint.NewFromFloat(10),
		TimeInForce: types.TimeInForceFOK,
	}, BrokerPlaceOrderQuery{}); err == nil || !strings.Contains(err.Error(), "unsupported timeInForce") {
		t.Fatalf("unsupported timeInForce err = %v", err)
	}

	invalidSession := "weekend"
	if _, err := placeOrderRequestFromSubmitOrder(usAccount, types.SubmitOrder{
		Symbol:   "US.AAPL",
		Side:     types.SideTypeBuy,
		Type:     types.OrderTypeLimit,
		Price:    fixedpoint.NewFromFloat(180),
		Quantity: fixedpoint.NewFromFloat(10),
	}, BrokerPlaceOrderQuery{Session: &invalidSession}); err == nil || !strings.Contains(err.Error(), "unsupported session") {
		t.Fatalf("unsupported session err = %v", err)
	}

	request, err := placeOrderRequestFromSubmitOrder(usAccount, types.SubmitOrder{
		ClientOrderID: "client-001",
		Tag:           "fallback-tag",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeBuy,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(10),
		TimeInForce:   types.TimeInForce("DAY"),
	}, BrokerPlaceOrderQuery{
		Session:        &session,
		FillOutsideRTH: &fillOutsideRTH,
	})
	if err != nil {
		t.Fatalf("placeOrderRequestFromSubmitOrder(valid US market order): %v", err)
	}
	if got := request.GetRemark(); got != "client-001" {
		t.Fatalf("Remark = %q, want client order id to win over tag", got)
	}
	if request.FillOutsideRTH != nil {
		t.Fatalf("FillOutsideRTH = %#v, want omitted for market order", request.FillOutsideRTH)
	}
	if got := request.GetSession(); got != 1 {
		t.Fatalf("Session = %d, want RTH(1)", got)
	}
	if got := request.GetTimeInForce(); got != int32(trdcommonpb.TimeInForce_TimeInForce_DAY) {
		t.Fatalf("TimeInForce = %d, want DAY", got)
	}
}
