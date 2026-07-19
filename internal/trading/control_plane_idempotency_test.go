package trading

import (
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

// kill switch 的 release 是幂等的状态操作：未激活时 release 不报错、状态保持关闭；
// 每次 release 只追加一条审计事件（审计轨迹），不产生其他副作用。
func TestRealTradeControlPlaneKillSwitchReleaseIsIdempotentAndAudited(t *testing.T) {
	plane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}

	// 未激活时 release：成功、状态仍为关闭，事件 ActivatedAt 为空（没有可引用的激活记录）。
	snapshot, err := plane.ReleaseKillSwitch(t.Context(), RealTradeKillSwitchCommand{OperatorID: "tester", Reason: "precautionary release"})
	if err != nil {
		t.Fatalf("release without activation: %v", err)
	}
	if snapshot.KillSwitchActive {
		t.Fatalf("release without activation changed state: %#v", snapshot)
	}
	events := snapshot.KillSwitchEvents
	if len(events) != 1 || events[0].Action != "KILL_SWITCH_RELEASE" || events[0].ActivatedAt != nil {
		t.Fatalf("release-without-activation events = %#v", events)
	}

	if _, err := plane.ActivateKillSwitch(t.Context(), RealTradeKillSwitchCommand{OperatorID: "tester", Reason: "incident"}); err != nil {
		t.Fatalf("ActivateKillSwitch: %v", err)
	}
	activatedAt := plane.state.KillSwitch.ActivatedAt

	// 激活后重复 release：两次都成功，状态保持关闭，审计事件逐条追加。
	if _, err := plane.ReleaseKillSwitch(t.Context(), RealTradeKillSwitchCommand{OperatorID: "tester"}); err != nil {
		t.Fatalf("first release: %v", err)
	}
	snapshot, err = plane.ReleaseKillSwitch(t.Context(), RealTradeKillSwitchCommand{OperatorID: "tester"})
	if err != nil {
		t.Fatalf("repeated release: %v", err)
	}
	if snapshot.KillSwitchActive {
		t.Fatalf("repeated release changed state: %#v", snapshot)
	}
	events = snapshot.KillSwitchEvents
	if len(events) != 4 {
		t.Fatalf("kill-switch events = %d, want 4 (release/release/activate/release)", len(events))
	}
	// 事件按时间倒序：最新一条 release 无激活记录可引用；紧邻的前一条 release 记录激活时间。
	if events[0].Action != "KILL_SWITCH_RELEASE" || events[0].ActivatedAt != nil {
		t.Fatalf("repeated release event = %#v, want release with nil ActivatedAt", events[0])
	}
	if events[1].Action != "KILL_SWITCH_RELEASE" || events[1].ActivatedAt == nil || *events[1].ActivatedAt != activatedAt {
		t.Fatalf("first release event = %#v, want release referencing %q", events[1], activatedAt)
	}
	if events[2].Action != "KILL_SWITCH_ACTIVATE" {
		t.Fatalf("activate event = %#v", events[2])
	}
}

// hard stop 的 release 是单次操作：重复 release 同一 ID 返回 not found，
// 且不追加审计事件、不改变状态。
func TestRealTradeControlPlaneHardStopReleaseIsSingleShot(t *testing.T) {
	plane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	snapshot, err := plane.ActivateHardStop(t.Context(), RealTradeHardStopCommand{AccountID: "ACC-1"})
	if err != nil {
		t.Fatalf("ActivateHardStop: %v", err)
	}
	entries := snapshot.HardStopEntries
	if len(entries) != 1 {
		t.Fatalf("hard-stop entries = %#v", entries)
	}
	id := entries[0].ID

	released, err := plane.ReleaseHardStop(t.Context(), id, RealTradeHardStopCommand{OperatorID: "tester"})
	if err != nil {
		t.Fatalf("ReleaseHardStop: %v", err)
	}
	if got := released.HardStopEntries; len(got) != 0 {
		t.Fatalf("entries after release = %#v", got)
	}

	if _, err := plane.ReleaseHardStop(t.Context(), id, RealTradeHardStopCommand{OperatorID: "tester"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("repeated release error = %v, want not found", err)
	}
	events := plane.Snapshot().HardStopEvents
	if len(events) != 2 || events[0].Action != "HARD_STOP_RELEASE" || events[1].Action != "HARD_STOP_ACTIVATE" {
		t.Fatalf("repeated release appended events or lost audit: %#v", events)
	}
}

// hard stop 按条目累积生效：同范围重复 activate 生成独立条目，
// 只 release 其中一个不会恢复交易，全部 release 后才放行。
func TestRealTradeControlPlaneHardStopsBlockUntilEveryEntryReleased(t *testing.T) {
	plane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	maxQty := 25.0
	if _, err := plane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{
		RealTradingEnabled: true,
		MaxOrderQuantity:   &maxQty,
		OperatorID:         "tester",
	}); err != nil {
		t.Fatalf("UpdateRuntimeRiskConfig: %v", err)
	}

	first, err := plane.ActivateHardStop(t.Context(), RealTradeHardStopCommand{AccountID: "ACC-1", Market: "US", Reason: "first halt"})
	if err != nil {
		t.Fatalf("first ActivateHardStop: %v", err)
	}
	second, err := plane.ActivateHardStop(t.Context(), RealTradeHardStopCommand{AccountID: "ACC-1", Market: "US", Reason: "second halt"})
	if err != nil {
		t.Fatalf("second ActivateHardStop: %v", err)
	}
	firstID := first.HardStopEntries[0].ID
	entries := second.HardStopEntries
	if len(entries) != 2 || entries[0].ID == entries[1].ID {
		t.Fatalf("cumulative hard-stop entries = %#v, want two distinct entries", entries)
	}

	price := 10.0
	command := ExecutionOrderCommand{
		BrokerID: "futu",
		Symbol:   "AAPL",
		Query: broker.PlaceOrderQuery{
			ReadQuery: broker.ReadQuery{TradingEnvironment: "REAL", AccountID: "ACC-1", Market: "US"},
			Quantity:  1,
			Price:     &price,
		},
	}
	if decision := plane.EvaluatePlaceOrder(t.Context(), command); decision.Allows() || decision.ReasonCode != "REAL_TRADE_HARD_STOP_ACTIVE" {
		t.Fatalf("decision with two hard stops = %#v", decision)
	}

	// 只解除第一条：交易仍被第二条拦截，且拦截动作计入审计事件。
	if _, err := plane.ReleaseHardStop(t.Context(), firstID, RealTradeHardStopCommand{OperatorID: "tester"}); err != nil {
		t.Fatalf("release first hard stop: %v", err)
	}
	if decision := plane.EvaluatePlaceOrder(t.Context(), command); decision.Allows() || decision.ReasonCode != "REAL_TRADE_HARD_STOP_ACTIVE" {
		t.Fatalf("decision with one remaining hard stop = %#v", decision)
	}
	rejections := 0
	for _, event := range plane.Snapshot().HardStopEvents {
		if event.Action == "HARD_STOP_REJECT" {
			rejections++
		}
	}
	if rejections != 2 {
		t.Fatalf("hard-stop rejection audit events = %d, want 2", rejections)
	}

	// 全部解除后放行。
	if _, err := plane.ReleaseHardStop(t.Context(), entries[1].ID, RealTradeHardStopCommand{OperatorID: "tester"}); err != nil {
		t.Fatalf("release second hard stop: %v", err)
	}
	if decision := plane.EvaluatePlaceOrder(t.Context(), command); !decision.Allows() {
		t.Fatalf("decision after releasing all hard stops = %#v", decision)
	}
}
