package trading

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 推送订阅建立失败（如断线）后：下一次 Sync 必须重新发起订阅而不是放弃，
// 失败的半开订阅被清理一次，恢复后推送正常送达且不重复订阅。
func TestOrderUpdatesWorkerResubscribesAfterSubscribeFailureAndPushResumes(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	source := &fakeOrderUpdateSource{
		accounts:     []Account{{ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"}}},
		subscribeErr: errors.New("dial tcp 127.0.0.1:11111: connect: connection refused"),
	}
	execution := &fakeExecutionOrderUpdates{}
	worker := NewOrderUpdatesWorker(source, execution, OrderUpdatesConfig{Now: func() time.Time { return now }})

	worker.Sync(context.Background(), true, true)
	if source.subscribeCalls != 1 {
		t.Fatalf("subscribe calls after failure = %d, want 1", source.subscribeCalls)
	}
	// 失败的订阅尝试被记为 DISCONNECTED invalidation。
	invalidations := jftradeCheckedTypeAssertion[[]any](worker.SnapshotResponse()["recentInvalidations"])
	if len(invalidations) == 0 {
		t.Fatal("subscribe failure did not record an invalidation")
	}
	latest := jftradeCheckedTypeAssertion[map[string]any](invalidations[len(invalidations)-1])
	if latest["kind"] != "DISCONNECTED" {
		t.Fatalf("latest invalidation = %#v, want DISCONNECTED", latest)
	}
	// 半开订阅被清理一次，避免泄漏。
	if len(source.subscriptions) != 1 || source.subscriptions[0].stops != 1 {
		t.Fatalf("failed subscription cleanup = %#v", source.subscriptions)
	}

	// 连接恢复后，下一次 Sync 重新订阅（而不是复用失败状态）。
	source.subscribeErr = nil
	worker.Sync(context.Background(), true, true)
	if source.subscribeCalls != 2 {
		t.Fatalf("subscribe calls after recovery = %d, want 2", source.subscribeCalls)
	}
	if len(source.subscriptions) != 2 || source.subscriptions[1].stops != 0 {
		t.Fatalf("recovered subscription = %#v", source.subscriptions)
	}

	// 恢复后的推送正常送达执行账本。
	if source.handler == nil {
		t.Fatal("recovered subscription did not install a push handler")
	}
	source.handler.HandleOrderUpdate(Order{
		AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "HK",
		BrokerOrderID: "9001", Status: "SUBMITTED",
	})
	if len(execution.orders) == 0 {
		t.Fatal("push after recovery was not applied")
	}
	last := execution.orders[len(execution.orders)-1]
	if last.order.BrokerOrderID != "9001" || last.metadata.SourceDetail != "broker.push" || last.metadata.UpdatedEventType != "BROKER_PUSH_ORDER" {
		t.Fatalf("push after recovery = %#v metadata=%#v", last.order, last.metadata)
	}
}

// 重连通过既有订阅的 Refresh 完成：Refresh 失败（仍断线）时不拆除旧订阅、
// 不新建重复订阅；恢复后仍走 Refresh，推送不中断语义保持单订阅。
func TestOrderUpdatesWorkerKeepsPushSubscriptionWhenRefreshFails(t *testing.T) {
	now := time.Date(2026, 7, 18, 11, 0, 0, 0, time.UTC)
	source := &fakeRefreshOrderUpdateSource{fakeOrderUpdateSource: fakeOrderUpdateSource{
		accounts: []Account{{ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"}}},
	}}
	execution := &fakeExecutionOrderUpdates{}
	worker := NewOrderUpdatesWorker(source, execution, OrderUpdatesConfig{Now: func() time.Time { return now }})

	worker.Sync(context.Background(), true, true)
	if source.subscribeCalls != 1 || source.refreshSubscription == nil {
		t.Fatalf("initial subscription calls=%d subscription=%#v", source.subscribeCalls, source.refreshSubscription)
	}

	// 断线期间 Refresh 失败：旧订阅保留、不重复 Subscribe。
	source.refreshSubscription.refreshErr = errors.New("dial tcp 127.0.0.1:11111: i/o timeout")
	worker.Sync(context.Background(), true, true)
	if source.subscribeCalls != 1 {
		t.Fatalf("refresh failure triggered duplicate subscribe calls = %d, want 1", source.subscribeCalls)
	}
	if source.refreshSubscription.stops != 0 {
		t.Fatalf("refresh failure stopped the live subscription: stops=%d", source.refreshSubscription.stops)
	}
	if source.refreshSubscription.refreshCalls != 1 {
		t.Fatalf("refresh calls after failure = %d, want 1", source.refreshSubscription.refreshCalls)
	}
	invalidations := jftradeCheckedTypeAssertion[[]any](worker.SnapshotResponse()["recentInvalidations"])
	if len(invalidations) == 0 {
		t.Fatal("refresh failure did not record an invalidation")
	}

	// 连接恢复：继续 Refresh 同一订阅，全程只有一次 Subscribe、零次 Stop。
	source.refreshSubscription.refreshErr = nil
	worker.Sync(context.Background(), true, true)
	if source.subscribeCalls != 1 || source.refreshSubscription.refreshCalls != 2 || source.refreshSubscription.stops != 0 {
		t.Fatalf("recovered refresh state calls=%d refresh=%d stops=%d",
			source.subscribeCalls, source.refreshSubscription.refreshCalls, source.refreshSubscription.stops)
	}
	source.handler.HandleOrderUpdate(Order{
		AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "HK",
		BrokerOrderID: "9002", Status: "SUBMITTED",
	})
	if len(execution.orders) == 0 || execution.orders[len(execution.orders)-1].order.BrokerOrderID != "9002" {
		t.Fatalf("push after refresh recovery = %#v", execution.orders)
	}
}
