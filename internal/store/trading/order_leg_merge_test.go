package trading

import (
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestExecutionLegSnapshotsMergeAppendAndNormalizeLifecycle(t *testing.T) {
	now := "2026-07-17T12:00:00Z"
	summary := &trdsrv.ExecutionOrder{
		InternalOrderID: "order-1",
		Legs: []trdsrv.ExecutionOrderLeg{{
			ID: "order-1-leg-001", InternalOrderID: "order-1", Index: 0,
			InstrumentID: "US.ONE", ProductClass: broker.ProductClassOption,
			Side: "BUY", Ratio: 1, Status: trdsrv.OrderStatusSubmitted,
		}},
	}
	applyExecutionLegSnapshots(nil, []broker.OrderLegSnapshot{{InstrumentID: "US.ONE"}}, now)
	applyExecutionLegSnapshots(summary, nil, now)
	applyExecutionLegSnapshots(summary, []broker.OrderLegSnapshot{
		{
			BrokerLegID: "broker-leg-1", InstrumentID: "us.one",
			ProductClass: broker.ProductClassEventContract, Side: " sell ", Ratio: 2,
			PredictionSide: " yes ", RequestedQuantity: 3, RequestedAmount: 30,
			RequestedPrice: 0.6, Status: "FILLED_PART", FilledQuantity: 1,
			FilledAmount: 10, AveragePrice: 0.55, Fees: 0.1, Payout: 5,
		},
		{
			InstrumentID: "US.TWO", ProductClass: broker.ProductClassOption,
			Side: "BUY", Ratio: 0, Status: "CUSTOM_PENDING",
		},
		{
			InstrumentID: "US.THREE", ProductClass: broker.ProductClassFuture,
			Side: "SELL", Ratio: 3, Status: "FILLED_ALL",
		},
	}, now)
	if len(summary.Legs) != 3 {
		t.Fatalf("merged legs = %#v", summary.Legs)
	}
	first := summary.Legs[0]
	if first.BrokerLegID == nil || *first.BrokerLegID != "broker-leg-1" ||
		first.ProductClass != broker.ProductClassEventContract || first.Side != "SELL" ||
		first.Ratio != 2 || first.PredictionSide != "YES" ||
		first.RequestedQuantity == nil || *first.RequestedQuantity != 3 ||
		first.RequestedAmount == nil || *first.RequestedAmount != 30 ||
		first.RequestedPrice == nil || *first.RequestedPrice != 0.6 ||
		first.FilledQuantity == nil || *first.FilledQuantity != 1 ||
		first.FilledAmount == nil || *first.FilledAmount != 10 ||
		first.AveragePrice == nil || *first.AveragePrice != 0.55 ||
		first.Fees == nil || *first.Fees != 0.1 ||
		first.Payout == nil || *first.Payout != 5 {
		t.Fatalf("first merged leg = %#v", first)
	}
	if summary.Legs[1].Ratio != 1 || summary.Legs[1].Status != trdsrv.OrderStatusUnknown {
		t.Fatalf("fallback-index leg = %#v", summary.Legs[1])
	}
	if summary.Legs[2].Ratio != 3 || summary.Legs[2].Status != trdsrv.OrderStatusFilled {
		t.Fatalf("appended leg = %#v", summary.Legs[2])
	}
}
