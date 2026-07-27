package liveruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	runtimecontrol "github.com/jftrade/jftrade-main/internal/strategy/runtimecontrol"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestRuntimeRiskRejectionRecordsAuditAndPauseTransition(t *testing.T) {
	var nilExecutor *strategyLiveOrderExecutor
	if got := nilExecutor.currentRuntimeRiskSettings(); got != instancebinding.NormalizeRiskSettings(stratsrv.RuntimeRiskSettings{}) {
		t.Fatalf("nil executor risk settings = %#v", got)
	}
	if got := ((*symbolRuntime)(nil)).sellableQuantity("US.AAPL"); got != 0 {
		t.Fatalf("nil runner sellable quantity = %v", got)
	}

	var eventKinds []string
	var transitioned []string
	manager := NewManager(Dependencies{
		CurrentInstance: func(string) (stratsrv.ManagedInstance, bool) {
			return stratsrv.ManagedInstance{
				Binding: stratsrv.InstanceBinding{
					RuntimeRisk: stratsrv.RuntimeRiskSettings{Mode: "monitor"},
				},
			}, true
		},
		AppendRuntimeEvent: func(_ string, _ string, kind string, _ string) error {
			eventKinds = append(eventKinds, kind)
			return nil
		},
		TransitionInstance: func(_ string, status string, kind string, _ string) error {
			transitioned = append(transitioned, status+":"+kind)
			return nil
		},
		CountRuntimeAudit: func(context.Context, runtimeactivity.AuditQuery) (int, error) {
			return 0, errors.New("audit store unavailable")
		},
	})
	executor := &strategyLiveOrderExecutor{
		manager: manager,
		instance: stratsrv.ManagedInstance{
			ID:      "runtime-risk-instance",
			Binding: stratsrv.InstanceBinding{RuntimeRisk: stratsrv.RuntimeRiskSettings{Mode: "enforce"}},
		},
	}
	if got := executor.currentRuntimeRiskSettings().Mode; got != "monitor" {
		t.Fatalf("persisted runtime risk settings were not preferred: %q", got)
	}
	if got := manager.todaySubmittedOrderCount("runtime-risk-instance", "US.AAPL", time.Now().UTC()); got != 0 {
		t.Fatalf("audit-store failure count = %d", got)
	}

	command := trdsrv.ExecutionOrderCommand{
		Symbol: "US.AAPL",
		Side:   "BUY",
		Query:  broker.PlaceOrderQuery{Symbol: "US.AAPL", Side: "BUY", Quantity: 2},
	}
	executor.recordRuntimeRiskDecision(runtimecontrol.RiskDecision{
		Matched: true, Rejected: true, PauseOnReject: true, Reason: "close_only", Detail: "rule=close_only",
	}, command)
	executor.recordRuntimeRiskDecision(runtimecontrol.RiskDecision{
		Matched: true, Reason: "daily_max_orders", Detail: "rule=daily_max_orders",
	}, command)
	if strings.Join(eventKinds, ",") != "risk_rejected,risk_monitor" {
		t.Fatalf("runtime risk event kinds = %#v", eventKinds)
	}
	if strings.Join(transitioned, ",") != "PAUSED:paused" {
		t.Fatalf("runtime risk pause transition = %#v", transitioned)
	}
}
