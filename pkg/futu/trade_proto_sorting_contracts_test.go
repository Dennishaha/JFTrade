package futu

import (
	"testing"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
)

func TestBrokerTradeProtoListsFilterAndSortEveryTieBreaker(t *testing.T) {
	account := resolvedTradeAccount{Market: "HK"}
	positions := brokerPositionSnapshotsFromProto(account, []*trdcommonpb.Position{
		nil,
		{Code: new("US.MSFT")},
		{Code: new("HK.00700")},
		{Code: new("HK.00005")},
	})
	if len(positions) != 3 || positions[0].Symbol != "HK.00005" || positions[2].Market != "US" {
		t.Fatalf("sorted positions = %#v", positions)
	}

	orders := []*trdcommonpb.Order{
		nil,
		{Code: new("AAPL"), OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_All))},
		{Code: new("MSFT"), OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted))},
		{Code: new("AAPL"), OrderID: new(uint64(1)), OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)), UpdateTime: new("2026-07-15 10:00:00")},
		{Code: new("AAPL"), OrderID: new(uint64(3)), OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)), UpdateTime: new("2026-07-15 10:00:00")},
		{Code: new("AAPL"), OrderID: new(uint64(2)), OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)), UpdateTime: new("2026-07-15 11:00:00")},
	}
	working := brokerOrderSnapshotsFromProto(account, orders, "AAPL", true)
	if len(working) != 3 || working[0].BrokerOrderID != "2" || working[1].BrokerOrderID != "3" {
		t.Fatalf("sorted working orders = %#v", working)
	}
	history := brokerOrderSnapshotsFromProto(account, orders[1:2], "", false)
	if len(history) != 1 {
		t.Fatalf("historical closed orders = %#v", history)
	}

	fills := brokerOrderFillSnapshotsFromProto(account, []*trdcommonpb.OrderFill{
		nil,
		{Code: new("MSFT"), FillID: new(uint64(9))},
		{Code: new("AAPL"), FillID: new(uint64(1)), CreateTime: new("2026-07-15 10:00:00")},
		{Code: new("AAPL"), FillID: new(uint64(3)), CreateTime: new("2026-07-15 10:00:00")},
		{Code: new("AAPL"), FillID: new(uint64(2)), CreateTime: new("2026-07-15 11:00:00")},
	}, "AAPL")
	if len(fills) != 3 || fills[0].BrokerFillID != "2" || fills[1].BrokerFillID != "3" {
		t.Fatalf("sorted fills = %#v", fills)
	}

	fees := brokerOrderFeeSnapshotsFromProto(account, []*trdcommonpb.OrderFee{
		nil, {OrderIDEx: new("B")}, {OrderIDEx: new("A")},
	})
	if len(fees) != 2 || fees[0].BrokerOrderIDEx != "A" {
		t.Fatalf("sorted fees = %#v", fees)
	}

	flows := brokerCashFlowSnapshotsFromProto(account, []*trdflowsummarypb.FlowSummaryInfo{
		nil,
		{CashFlowID: new(uint64(1)), ClearingDate: new("2026-07-14")},
		{CashFlowID: new(uint64(2)), ClearingDate: new("2026-07-15")},
		{CashFlowID: new(uint64(3)), ClearingDate: new("2026-07-15")},
	})
	if len(flows) != 3 || *flows[0].CashFlowID != "3" || *flows[2].ClearingDate != "2026-07-14" {
		t.Fatalf("sorted cash flows = %#v", flows)
	}
}
