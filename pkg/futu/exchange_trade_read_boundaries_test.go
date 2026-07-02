package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestBrokerReadQueriesSurfaceAccountResolutionErrors(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	readQuery := BrokerReadQuery{TradingEnvironment: "SIMULATE", AccountID: "missing", Market: "HK"}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "queryAccount", run: func() error {
			_, err := ex.queryAccount(t.Context())
			return err
		}},
		{name: "queryAccountBalances", run: func() error {
			_, err := ex.queryAccountBalances(t.Context())
			return err
		}},
		{name: "queryOpenOrders", run: func() error {
			_, err := ex.queryOpenOrders(t.Context(), "HK.00700")
			return err
		}},
		{name: "QueryBrokerFunds", run: func() error {
			_, err := ex.QueryBrokerFunds(t.Context(), readQuery)
			return err
		}},
		{name: "QueryBrokerPositions", run: func() error {
			_, err := ex.QueryBrokerPositions(t.Context(), readQuery)
			return err
		}},
		{name: "QueryBrokerOrders", run: func() error {
			_, err := ex.QueryBrokerOrders(t.Context(), readQuery, "HK.00700")
			return err
		}},
		{name: "QueryBrokerHistoryOrders", run: func() error {
			_, err := ex.QueryBrokerHistoryOrders(t.Context(), BrokerOrderHistoryQuery{BrokerReadQuery: readQuery, Symbol: "HK.00700"})
			return err
		}},
		{name: "QueryBrokerHistoryOrderFills", run: func() error {
			_, err := ex.QueryBrokerHistoryOrderFills(t.Context(), BrokerOrderFillHistoryQuery{BrokerReadQuery: readQuery, Symbol: "HK.00700"})
			return err
		}},
		{name: "QueryBrokerOrderFills", run: func() error {
			_, err := ex.QueryBrokerOrderFills(t.Context(), BrokerOrderFillQuery{BrokerReadQuery: readQuery, Symbol: "HK.00700"})
			return err
		}},
		{name: "QueryBrokerOrderFees", run: func() error {
			_, err := ex.QueryBrokerOrderFees(t.Context(), BrokerOrderFeeQuery{BrokerReadQuery: readQuery, OrderIDExList: []string{"EXT-1"}})
			return err
		}},
		{name: "QueryBrokerCashFlows", run: func() error {
			_, err := ex.QueryBrokerCashFlows(t.Context(), BrokerCashFlowQuery{BrokerReadQuery: readQuery, ClearingDate: "2026-05-20"})
			return err
		}},
		{name: "QueryBrokerMaxTradeQuantity", run: func() error {
			_, err := ex.QueryBrokerMaxTradeQuantity(t.Context(), BrokerMaxTradeQuantityQuery{
				BrokerReadQuery: readQuery,
				Symbol:          "HK.00700",
				OrderType:       "LIMIT",
				Price:           320,
			})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil {
				t.Fatal("error=nil, want account resolution failure")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "account") {
				t.Fatalf("error = %v, want account context", err)
			}
		})
	}
}

func TestQueryBrokerMaxTradeQuantityOptionalRequestFieldsAndSessionValidation(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	server.setMaxTrdQtys(&trdcommonpb.MaxTrdQtys{
		MaxCashBuy:      new(float64(100)),
		MaxPositionSell: new(float64(50)),
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	adjust := 2.0
	positionID := uint64(88001)
	session := "ETH"
	snapshot, err := ex.QueryBrokerMaxTradeQuantity(t.Context(), BrokerMaxTradeQuantityQuery{
		BrokerReadQuery:    BrokerReadQuery{TradingEnvironment: "REAL", AccountID: "1001"},
		Symbol:             "HK.00700",
		OrderType:          "LIMIT",
		Price:              320,
		OrderIDEx:          "EXT-2001",
		AdjustSideAndLimit: &adjust,
		Session:            &session,
		PositionID:         &positionID,
	})
	if err != nil {
		t.Fatalf("QueryBrokerMaxTradeQuantity optional fields: %v", err)
	}
	if snapshot == nil || snapshot.MaxCashBuy != 100 || snapshot.MaxPositionSell != 50 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	request := server.lastMaxTrdQtysRequest()
	if request == nil {
		t.Fatal("expected max trade quantity request")
	}
	if request.GetOrderIDEx() != "EXT-2001" || !request.GetAdjustPrice() || request.GetAdjustSideAndLimit() != adjust || request.GetPositionID() != positionID {
		t.Fatalf("optional request fields = %#v", request)
	}
	if request.GetSession() == 0 {
		t.Fatalf("session was not forwarded: %#v", request)
	}

	badSession := "LUNCH"
	if _, err := ex.QueryBrokerMaxTradeQuantity(t.Context(), BrokerMaxTradeQuantityQuery{
		BrokerReadQuery: BrokerReadQuery{TradingEnvironment: "REAL", AccountID: "1001", Market: "HK"},
		Symbol:          "HK.00700",
		OrderType:       "LIMIT",
		Price:           320,
		Session:         &badSession,
	}); err == nil || !strings.Contains(err.Error(), "unsupported session") {
		t.Fatalf("bad session err = %v, want unsupported session", err)
	}
}
