package servercore

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

// seedOutOfOrderPlacedOrder 登记一笔已进入 SUBMITTED 状态的实盘订单，作为乱序回报的基线。
func seedOutOfOrderPlacedOrder(store *executionOrderStore, brokerOrderID string) executionOrderSummaryResponse {
	price := 100.0
	return store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID: "futu", BrokerOrderID: brokerOrderID, TradingEnvironment: "SIMULATE",
		AccountID: "SIM-1", Market: "US", Symbol: "US.AAPL", Side: "BUY",
		OrderType: "LIMIT", Status: "SUBMITTED", RequestedQuantity: 10,
		RequestedPrice: &price, SubmittedAt: "2026-07-10T01:00:00Z", EventType: "COMMAND_PLACE_ACCEPTED",
	})
}

func executionTestTimestampAfter(t *testing.T, base string, offset time.Duration) string {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, base)
	if err != nil {
		t.Fatalf("parse execution timestamp %q: %v", base, err)
	}
	return parsed.Add(offset).Format(time.RFC3339Nano)
}

func outOfOrderPushSnapshot(brokerOrderID, status string, filled float64, updatedAt string) broker.OrderSnapshot {
	price := 100.0
	return broker.OrderSnapshot{
		AccountID: "SIM-1", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: brokerOrderID, Symbol: "US.AAPL", Side: "BUY", OrderType: "LIMIT",
		Status: status, Quantity: 10, FilledQuantity: &filled,
		Price: &price, SubmittedAt: "2026-07-10T01:00:00Z", UpdatedAt: updatedAt,
	}
}

// 券商推送乱序到达（延迟的旧状态回报带着更新的时间戳晚到）时，
// 持久化层按状态机单调推进：终态不回退、已成交量不回退。
func TestExecutionOrderStoreIgnoresOutOfOrderRegressionPushAfterTerminalState(t *testing.T) {
	tests := []struct {
		name             string
		advanceStatus    string
		advanceFilled    float64
		regressionStatus string
		regressionFilled float64
		wantStatus       string
		wantFilled       float64
	}{
		{name: "delayed submitted after filled", advanceStatus: "FILLED_ALL", advanceFilled: 10, regressionStatus: "SUBMITTED", regressionFilled: 0, wantStatus: "FILLED", wantFilled: 10},
		{name: "delayed cancelled after filled", advanceStatus: "FILLED_ALL", advanceFilled: 10, regressionStatus: "CANCELLED_ALL", regressionFilled: 10, wantStatus: "FILLED", wantFilled: 10},
		{name: "delayed submitted after partial fill", advanceStatus: "FILLED_PART", advanceFilled: 4, regressionStatus: "SUBMITTED", regressionFilled: 1, wantStatus: "PARTIALLY_FILLED", wantFilled: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newExecutionOrderStore()
			seeded := seedOutOfOrderPlacedOrder(store, "ooo-1")
			advancedAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 2*time.Minute)
			regressedAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 3*time.Minute)

			advanced, _, changed := store.upsertBrokerOrderWithSource("futu",
				outOfOrderPushSnapshot("ooo-1", test.advanceStatus, test.advanceFilled, advancedAt),
				"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
			if !changed || advanced.Status != test.wantStatus {
				t.Fatalf("advance push status=%q changed=%v, want %q", advanced.Status, changed, test.wantStatus)
			}

			// 延迟回报带着更新的时间戳到达，但状态机拒绝回退。
			regressed, event, changed := store.upsertBrokerOrderWithSource("futu",
				outOfOrderPushSnapshot("ooo-1", test.regressionStatus, test.regressionFilled, regressedAt),
				"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
			if regressed.Status != test.wantStatus {
				t.Fatalf("regression push status=%q, want %q", regressed.Status, test.wantStatus)
			}
			if regressed.FilledQuantity == nil || *regressed.FilledQuantity != test.wantFilled {
				t.Fatalf("regression push filled=%v, want %v", regressed.FilledQuantity, test.wantFilled)
			}
			if changed {
				// 允许事件落账（审计券商推送），但前后状态必须一致，即不允许状态回退事件。
				if event == nil || event.PreviousStatus == nil || *event.PreviousStatus != test.wantStatus || event.NextStatus != test.wantStatus {
					t.Fatalf("regression event = %#v, want no-op status event on %q", event, test.wantStatus)
				}
			}
			persisted, ok := store.order(seeded.InternalOrderID)
			if !ok || persisted.Status != test.wantStatus || persisted.FilledQuantity == nil || *persisted.FilledQuantity != test.wantFilled {
				t.Fatalf("persisted order regressed: %#v", persisted)
			}
		})
	}
}

// 时间戳更旧但携带真实成交进度的快照不是脏数据：进度推进优先于时间戳先后。
func TestExecutionOrderStoreAppliesFillProgressFromOlderSnapshot(t *testing.T) {
	store := newExecutionOrderStore()
	seeded := seedOutOfOrderPlacedOrder(store, "ooo-2")
	partialAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 5*time.Minute)
	olderProgressAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 3*time.Minute)

	// 先在 01:05 确认部分成交 2 股。
	partial, _, changed := store.upsertBrokerOrderWithSource("futu",
		outOfOrderPushSnapshot("ooo-2", "FILLED_PART", 2, partialAt),
		"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
	if !changed || partial.Status != "PARTIALLY_FILLED" {
		t.Fatalf("partial push status=%q changed=%v", partial.Status, changed)
	}

	// 01:03 生成的快照晚到（时间戳落后），但成交量从 2 推进到 4，必须被采纳。
	progressed, event, changed := store.upsertBrokerOrderWithSource("futu",
		outOfOrderPushSnapshot("ooo-2", "FILLED_PART", 4, olderProgressAt),
		"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
	if !changed || progressed.Status != "PARTIALLY_FILLED" {
		t.Fatalf("progressed push status=%q changed=%v, want PARTIALLY_FILLED", progressed.Status, changed)
	}
	if progressed.FilledQuantity == nil || *progressed.FilledQuantity != 4 {
		t.Fatalf("progressed filled=%v, want 4", progressed.FilledQuantity)
	}
	if progressed.UpdatedAt != partialAt {
		t.Fatalf("progressed updatedAt=%q, want latest accepted timestamp", progressed.UpdatedAt)
	}
	if event == nil || event.PreviousStatus == nil || *event.PreviousStatus != "PARTIALLY_FILLED" || event.NextStatus != "PARTIALLY_FILLED" {
		t.Fatalf("progress event = %#v", event)
	}
	if event.CreatedAt != progressed.UpdatedAt {
		t.Fatalf("progress event createdAt=%q, want monotonic order timestamp %q", event.CreatedAt, progressed.UpdatedAt)
	}
	persisted, ok := store.order(seeded.InternalOrderID)
	if !ok || persisted.Status != "PARTIALLY_FILLED" || persisted.FilledQuantity == nil || *persisted.FilledQuantity != 4 {
		t.Fatalf("persisted order = %#v", persisted)
	}
	if persisted.UpdatedAt != progressed.UpdatedAt {
		t.Fatalf("persisted updatedAt=%q, want %q", persisted.UpdatedAt, progressed.UpdatedAt)
	}
}

// 独立成交回报可能比累计订单快照更晚到，但事件本身的发生时间更早。
// 该成交应留痕但不能重复累加，订单的最后更新时间也必须保持单调。
func TestExecutionOrderStoreKeepsUpdatedAtMonotonicForOlderFill(t *testing.T) {
	store := newExecutionOrderStore()
	seeded := seedOutOfOrderPlacedOrder(store, "ooo-fill-time")
	partialAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 5*time.Minute)
	olderFillAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 3*time.Minute)

	partial, _, changed := store.upsertBrokerOrderWithSource("futu",
		outOfOrderPushSnapshot("ooo-fill-time", "FILLED_PART", 2, partialAt),
		"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
	if !changed || partial.UpdatedAt != partialAt {
		t.Fatalf("partial order = %#v", partial)
	}

	status := "FILLED_PART"
	fillPrice := 101.0
	filled, event, changed := store.recordBrokerOrderFill("futu", broker.OrderFillSnapshot{
		AccountID: "SIM-1", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "ooo-fill-time", BrokerFillID: "fill-older-time",
		Symbol: "US.AAPL", Side: "BUY", Status: &status,
		FilledQuantity: 1, FillPrice: &fillPrice, FilledAt: olderFillAt,
	})
	if changed || filled.FilledQuantity == nil || *filled.FilledQuantity != 2 {
		t.Fatalf("filled order = %#v changed=%v", filled, changed)
	}
	if filled.UpdatedAt != partial.UpdatedAt {
		t.Fatalf("filled updatedAt=%q, want monotonic %q", filled.UpdatedAt, partial.UpdatedAt)
	}
	if event == nil || event.CreatedAt != partial.UpdatedAt {
		t.Fatalf("fill event = %#v, want monotonic timestamp %q", event, partial.UpdatedAt)
	}
	persisted, ok := store.order(seeded.InternalOrderID)
	if !ok || persisted.UpdatedAt != partial.UpdatedAt {
		t.Fatalf("persisted order = %#v", persisted)
	}
}

// 撤单请求与成交回报竞态：CANCEL_REQUESTED 之后 FILLED 仍然成立（成交优先），
// 没有成交时 CANCELLED 正常确认撤单。
func TestExecutionOrderStoreResolvesCancelRequestRaceAgainstBrokerPush(t *testing.T) {
	t.Run("fill wins over in-flight cancel", func(t *testing.T) {
		store := newExecutionOrderStore()
		seeded := seedOutOfOrderPlacedOrder(store, "ooo-3")
		cancelAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 2*time.Minute)
		filledAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 3*time.Minute)

		cancelRequested, _, changed := store.upsertBrokerOrderWithSource("futu",
			outOfOrderPushSnapshot("ooo-3", "CANCELLING_ALL", 0, cancelAt),
			"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
		if !changed || cancelRequested.Status != "CANCEL_REQUESTED" {
			t.Fatalf("cancel-requested push status=%q changed=%v", cancelRequested.Status, changed)
		}

		filled, event, changed := store.upsertBrokerOrderWithSource("futu",
			outOfOrderPushSnapshot("ooo-3", "FILLED_ALL", 10, filledAt),
			"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
		if !changed || filled.Status != "FILLED" {
			t.Fatalf("fill-during-cancel status=%q changed=%v, want FILLED", filled.Status, changed)
		}
		if event == nil || event.PreviousStatus == nil || *event.PreviousStatus != "CANCEL_REQUESTED" || event.NextStatus != "FILLED" {
			t.Fatalf("fill-during-cancel event = %#v", event)
		}
	})

	t.Run("cancel confirmed when no fill arrives", func(t *testing.T) {
		store := newExecutionOrderStore()
		seeded := seedOutOfOrderPlacedOrder(store, "ooo-4")
		cancelRequestAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 2*time.Minute)
		cancelledAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 3*time.Minute)

		if _, _, changed := store.upsertBrokerOrderWithSource("futu",
			outOfOrderPushSnapshot("ooo-4", "CANCELLING_ALL", 0, cancelRequestAt),
			"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push"); !changed {
			t.Fatal("cancel-requested push was not applied")
		}
		cancelled, event, changed := store.upsertBrokerOrderWithSource("futu",
			outOfOrderPushSnapshot("ooo-4", "CANCELLED_ALL", 0, cancelledAt),
			"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
		if !changed || cancelled.Status != "CANCELLED" {
			t.Fatalf("cancel-confirm status=%q changed=%v, want CANCELLED", cancelled.Status, changed)
		}
		if event == nil || event.PreviousStatus == nil || *event.PreviousStatus != "CANCEL_REQUESTED" || event.NextStatus != "CANCELLED" {
			t.Fatalf("cancel-confirm event = %#v", event)
		}
	})
}

// 同一终态回报重复推送是幂等 no-op：不重复落事件、不改变已持久化状态。
func TestExecutionOrderStoreDuplicateTerminalPushIsNoOp(t *testing.T) {
	store := newExecutionOrderStore()
	seeded := seedOutOfOrderPlacedOrder(store, "ooo-5")
	filledAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 2*time.Minute)

	filled, _, changed := store.upsertBrokerOrderWithSource("futu",
		outOfOrderPushSnapshot("ooo-5", "FILLED_ALL", 10, filledAt),
		"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
	if !changed || filled.Status != "FILLED" {
		t.Fatalf("fill push status=%q changed=%v", filled.Status, changed)
	}
	eventsBefore := len(store.orderEvents(seeded.InternalOrderID).Events)

	_, duplicateEvent, duplicateChanged := store.upsertBrokerOrderWithSource("futu",
		outOfOrderPushSnapshot("ooo-5", "FILLED_ALL", 10, filledAt),
		"BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
	if duplicateChanged || duplicateEvent != nil {
		t.Fatalf("duplicate terminal push changed=%v event=%#v, want no-op", duplicateChanged, duplicateEvent)
	}
	if got := len(store.orderEvents(seeded.InternalOrderID).Events); got != eventsBefore {
		t.Fatalf("duplicate terminal push appended events: before=%d after=%d", eventsBefore, got)
	}
}
