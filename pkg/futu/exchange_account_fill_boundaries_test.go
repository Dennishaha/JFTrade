package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestQueryAccountReturnsBBGOAccountSnapshot(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{
		{
			TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
			AccID:             new(uint64(1001)),
			TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
			AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
		},
	})
	server.setFunds(&trdcommonpb.Funds{
		TotalAssets:       new(float64(25000)),
		Cash:              new(float64(12000)),
		FrozenCash:        new(float64(1500)),
		AvlWithdrawalCash: new(float64(10500)),
		MarketVal:         new(float64(13000)),
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	account, err := ex.QueryAccount(t.Context())
	if err != nil {
		t.Fatalf("QueryAccount: %v", err)
	}
	if account == nil {
		t.Fatal("expected account snapshot")
	}
	if _, ok := account.RawAccount.(*trdcommonpb.Funds); !ok {
		t.Fatalf("RawAccount type = %T, want *trdcommonpb.Funds", account.RawAccount)
	}
	if account.AccountType != types.AccountTypeMargin {
		t.Fatalf("AccountType = %q, want margin", account.AccountType)
	}
	if !account.CanTrade || !account.CanDeposit || !account.CanWithdraw {
		t.Fatalf("account capability flags = trade:%v deposit:%v withdraw:%v", account.CanTrade, account.CanDeposit, account.CanWithdraw)
	}
	if got := account.TotalAccountValue.Float64(); got != 25000 {
		t.Fatalf("TotalAccountValue = %v, want 25000", got)
	}
	balance, ok := account.Balance("HKD")
	if !ok {
		t.Fatalf("expected HKD balance, got %#v", account.Balances())
	}
	if got := balance.Available.Float64(); got != 10500 {
		t.Fatalf("Available = %v, want withdrawal cash 10500", got)
	}
	if got := balance.Locked.Float64(); got != 1500 {
		t.Fatalf("Locked = %v, want frozen cash 1500", got)
	}
	if got := server.fundsCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetFunds call, got %d", got)
	}
}

func TestQueryBrokerOrderFillsFiltersAndSortsCurrentSessionFills(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	server.tradeMu.Lock()
	server.orderFills = []*trdcommonpb.OrderFill{
		{
			FillID:     new(uint64(3001)),
			FillIDEx:   new("FILL-OLDER"),
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
		},
		{
			FillID:     new(uint64(3002)),
			FillIDEx:   new("FILL-LATEST"),
			OrderID:    new(uint64(2002)),
			OrderIDEx:  new("EXT-2002"),
			Code:       new("HK.00700"),
			Name:       new("Tencent"),
			TrdSide:    new(int32(trdcommonpb.TrdSide_TrdSide_Sell)),
			Qty:        new(float64(50)),
			Price:      new(321.2),
			CreateTime: new("2026-05-20 10:05:00"),
			TrdMarket:  new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
			Status:     new(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
		},
		{
			FillID:     new(uint64(3003)),
			FillIDEx:   new("FILL-OTHER-SYMBOL"),
			OrderID:    new(uint64(2003)),
			OrderIDEx:  new("EXT-2003"),
			Code:       new("HK.00005"),
			Name:       new("HSBC"),
			TrdSide:    new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
			Qty:        new(float64(10)),
			Price:      new(float64(60)),
			CreateTime: new("2026-05-20 10:10:00"),
			TrdMarket:  new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
			Status:     new(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
		},
	}
	server.tradeMu.Unlock()
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	fills, err := ex.QueryBrokerOrderFills(t.Context(), BrokerOrderFillQuery{
		BrokerReadQuery: BrokerReadQuery{TradingEnvironment: "SIMULATE", AccountID: "1001", Market: "HK"},
		Symbol:          "hk.00700",
		StartTime:       "2026-05-20 09:00:00",
		EndTime:         "2026-05-20 16:00:00",
	})
	if err != nil {
		t.Fatalf("QueryBrokerOrderFills: %v", err)
	}
	if len(fills) != 2 {
		t.Fatalf("expected two current fills after symbol filter, got %#v", fills)
	}
	if fills[0].BrokerFillID != "3002" || fills[1].BrokerFillID != "3001" {
		t.Fatalf("fills not sorted newest first: %#v", fills)
	}
	if fills[0].Symbol != "HK.00700" || fills[0].Side != "SELL" || fills[0].FilledQuantity != 50 || fills[0].FillPrice == nil || *fills[0].FillPrice != 321.2 {
		t.Fatalf("latest fill normalization = %#v", fills[0])
	}
	if got := server.orderListCallCount(); got != 1 {
		t.Fatalf("expected one Trd_GetOrderFillList call, got %d", got)
	}
}

func TestQueryBrokerMaxTradeQuantityRejectsInvalidBusinessInputs(t *testing.T) {
	ex := NewExchange("127.0.0.1:11110")
	cases := []struct {
		name  string
		query BrokerMaxTradeQuantityQuery
		want  string
	}{
		{
			name:  "missing symbol",
			query: BrokerMaxTradeQuantityQuery{OrderType: "LIMIT", Price: 10},
			want:  "symbol is required",
		},
		{
			name:  "non positive price",
			query: BrokerMaxTradeQuantityQuery{Symbol: "HK.00700", OrderType: "LIMIT", Price: 0},
			want:  "price must be positive",
		},
		{
			name:  "unsupported order type",
			query: BrokerMaxTradeQuantityQuery{Symbol: "HK.00700", OrderType: "TRAILING", Price: 320},
			want:  "unsupported orderType",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.QueryBrokerMaxTradeQuantity(t.Context(), tc.query)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("QueryBrokerMaxTradeQuantity err=%v, want containing %q", err, tc.want)
			}
		})
	}
}
