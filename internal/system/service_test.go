package system

import (
	"context"
	"errors"
	"testing"
)

func TestStatusDefaultsAndInjectedSummaries(t *testing.T) {
	svc := NewService(
		WithAPIPort(3900),
		WithSettingsPath("/tmp/jftrade/settings.json"),
		WithDefaultTradingEnvironment("SIMULATE"),
		WithBrokerDescriptor(func() map[string]any {
			return map[string]any{"id": "futu", "status": "ready"}
		}),
		WithStrategyRuntimeSummary(func() map[string]any {
			return map[string]any{"status": "idle", "activeStrategies": 0}
		}),
		WithRuntimeResources(func() map[string]any {
			return map[string]any{"checkedAt": "2026-07-04T00:00:00Z", "count": 1, "items": []any{map[string]any{"id": "settings-file", "owner": "settings"}}}
		}),
	)

	status := svc.Status()
	if status["name"] != "JFTrade" {
		t.Fatalf("name = %v, want JFTrade", status["name"])
	}
	if status["apiPort"] != 3900 {
		t.Fatalf("apiPort = %v, want 3900", status["apiPort"])
	}
	if status["defaultTradingEnvironment"] != "SIMULATE" {
		t.Fatalf("defaultTradingEnvironment = %v", status["defaultTradingEnvironment"])
	}

	persistence, ok := status["persistence"].(map[string]any)
	if !ok {
		t.Fatalf("persistence = %#v, want map", status["persistence"])
	}
	if persistence["databasePath"] != "/tmp/jftrade/settings.json" {
		t.Fatalf("databasePath = %v", persistence["databasePath"])
	}
	if persistence["checkedAt"] == "" {
		t.Fatal("checkedAt is empty")
	}

	broker, ok := status["broker"].(map[string]any)
	if !ok || broker["id"] != "futu" || broker["status"] != "ready" {
		t.Fatalf("broker = %#v", status["broker"])
	}
	runtime, ok := status["strategyRuntime"].(map[string]any)
	if !ok || runtime["status"] != "idle" || runtime["activeStrategies"] != 0 {
		t.Fatalf("strategyRuntime = %#v", status["strategyRuntime"])
	}
	resources, ok := status["runtimeResources"].(map[string]any)
	if !ok || resources["count"] != 1 {
		t.Fatalf("runtimeResources = %#v", status["runtimeResources"])
	}
	items, ok := resources["items"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["owner"] != "settings" {
		t.Fatalf("runtimeResources items = %#v", resources["items"])
	}
}

func TestStatusUsesDynamicPortAndTradingEnvironmentProviders(t *testing.T) {
	apiPort := 3000
	tradingEnvironment := "SIMULATE"
	svc := NewService(
		WithAPIPortFunc(func() int { return apiPort }),
		WithDefaultTradingEnvironmentFunc(func() string { return tradingEnvironment }),
	)

	status := svc.Status()
	if status["apiPort"] != 3000 || status["defaultTradingEnvironment"] != "SIMULATE" {
		t.Fatalf("initial status = %#v", status)
	}

	apiPort = 38401
	tradingEnvironment = "REAL"
	status = svc.Status()
	if status["apiPort"] != 38401 {
		t.Fatalf("apiPort = %v, want 38401", status["apiPort"])
	}
	if status["defaultTradingEnvironment"] != "REAL" {
		t.Fatalf("defaultTradingEnvironment = %v, want REAL", status["defaultTradingEnvironment"])
	}
}

func TestRealTradeDefaultsMatchFrontendContract(t *testing.T) {
	svc := NewService()

	approvals := svc.RealTradeApprovals()
	assertSystemMapKeys(t, approvals, "realTradingEnabled", "requiredConfirmationText", "maxApprovalAgeMs", "approvalWorkflowAvailable", "approvalWorkflowStatus", "approvalWorkflowMessage", "approvalPolicy", "entries")
	assertSystemMapMissingKeys(t, approvals, "enabled", "approvals", "pendingCount")
	if approvals["realTradingEnabled"] != false {
		t.Fatalf("approvals realTradingEnabled = %v, want false", approvals["realTradingEnabled"])
	}
	if approvals["requiredConfirmationText"] != "ENABLE_REAL_TRADING" {
		t.Fatalf("requiredConfirmationText = %v", approvals["requiredConfirmationText"])
	}
	if approvals["maxApprovalAgeMs"] != 5*60*1000 {
		t.Fatalf("maxApprovalAgeMs = %v", approvals["maxApprovalAgeMs"])
	}
	if approvals["approvalWorkflowAvailable"] != false || approvals["approvalWorkflowStatus"] != "not_configured" {
		t.Fatalf("approval workflow = %#v", approvals)
	}
	if message, ok := approvals["approvalWorkflowMessage"].(string); !ok || message == "" {
		t.Fatalf("approvalWorkflowMessage = %#v, want non-empty string", approvals["approvalWorkflowMessage"])
	}
	assertSystemEmptyAnySlice(t, approvals, "entries")
	policy, ok := approvals["approvalPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("approvalPolicy = %#v, want map", approvals["approvalPolicy"])
	}
	if policy["approverAllowlistEnabled"] != false || policy["approverCount"] != 0 {
		t.Fatalf("approvalPolicy = %#v", policy)
	}
	if policy["approvalWorkflowAvailable"] != false || policy["approvalMode"] != "none" || policy["largeOrderNotional"] != nil {
		t.Fatalf("approvalPolicy workflow fields = %#v", policy)
	}

	killSwitch := svc.RealTradeKillSwitch()
	assertSystemMapKeys(t, killSwitch, "realTradingEnabled", "killSwitchActive", "killSwitchSource", "runtimeActive", "blockedOperations", "allowsCancel", "entry")
	assertSystemMapMissingKeys(t, killSwitch, "active")
	if killSwitch["killSwitchActive"] != false || killSwitch["allowsCancel"] != true {
		t.Fatalf("killSwitch = %#v", killSwitch)
	}
	if killSwitch["killSwitchSource"] != nil || killSwitch["entry"] != nil {
		t.Fatalf("killSwitch nullable fields = %#v", killSwitch)
	}

	riskLimits := svc.RealTradeRiskLimits()
	assertSystemMapKeys(t, riskLimits, "realTradingEnabled", "riskEnabled", "runtimeRiskConfigured", "runtimeConfiguredMaxOrderQuantity", "runtimeConfiguredMaxOrderNotional", "effectiveMaxOrderQuantity", "effectiveMaxOrderNotional", "entry")
	assertSystemMapMissingKeys(t, riskLimits, "enabled")
	if riskLimits["riskEnabled"] != false || riskLimits["entry"] != nil {
		t.Fatalf("riskLimits = %#v", riskLimits)
	}

	riskEvents := svc.RealTradeRiskEvents()
	assertSystemMapKeys(t, riskEvents, "realTradingEnabled", "riskEnabled", "runtimeRiskConfigured", "runtimeConfiguredMaxOrderQuantity", "runtimeConfiguredMaxOrderNotional", "effectiveMaxOrderQuantity", "effectiveMaxOrderNotional", "maxOrderQuantity", "maxOrderNotional", "entries")
	assertSystemMapMissingKeys(t, riskEvents, "events")
	if riskEvents["riskEnabled"] != false {
		t.Fatalf("riskEvents riskEnabled = %v, want false", riskEvents["riskEnabled"])
	}
	assertSystemEmptyAnySlice(t, riskEvents, "entries")
}

func TestRealTradeStateUsesInjectedRiskGatewaySnapshot(t *testing.T) {
	maxQty := 12.5
	maxNotional := 2500.0
	svc := NewService(WithRealTradeRiskState(func() map[string]any {
		return map[string]any{
			"realTradingEnabled":                true,
			"killSwitchActive":                  true,
			"killSwitchSource":                  "RUNTIME",
			"runtimeKillSwitchActive":           true,
			"riskEnabled":                       true,
			"runtimeRiskConfigured":             true,
			"runtimeConfiguredMaxOrderQuantity": &maxQty,
			"runtimeConfiguredMaxOrderNotional": &maxNotional,
			"effectiveMaxOrderQuantity":         &maxQty,
			"effectiveMaxOrderNotional":         &maxNotional,
			"riskEvents":                        []any{map[string]any{"id": "risk-event-1"}},
		}
	}))

	status := svc.Status()
	if status["realTradingEnabled"] != true {
		t.Fatalf("status realTradingEnabled = %v, want true", status["realTradingEnabled"])
	}
	killSwitch := status["realTradingKillSwitch"].(map[string]any)
	if killSwitch["active"] != true || killSwitch["runtimeActive"] != true || killSwitch["allowsCancel"] != true {
		t.Fatalf("status killSwitch = %#v", killSwitch)
	}
	risk := status["realTradingRisk"].(map[string]any)
	if risk["enabled"] != true || risk["runtimeRiskConfigured"] != true || risk["maxOrderQuantity"] != &maxQty || risk["maxOrderNotional"] != &maxNotional {
		t.Fatalf("status risk = %#v", risk)
	}

	state := svc.RealTradeKillSwitch()
	if state["realTradingEnabled"] != true || state["killSwitchActive"] != true || state["killSwitchSource"] != "RUNTIME" {
		t.Fatalf("kill switch state = %#v", state)
	}
	limits := svc.RealTradeRiskLimits()
	if limits["riskEnabled"] != true || limits["effectiveMaxOrderQuantity"] != &maxQty || limits["effectiveMaxOrderNotional"] != &maxNotional {
		t.Fatalf("risk limits = %#v", limits)
	}
	events := svc.RealTradeRiskEvents()
	if events["maxOrderQuantity"] != &maxQty || events["maxOrderNotional"] != &maxNotional {
		t.Fatalf("risk events = %#v", events)
	}
	if entries := events["entries"].([]any); len(entries) != 1 {
		t.Fatalf("risk event entries = %#v", events)
	}
	approvals := svc.RealTradeApprovals()
	if approvals["approvalWorkflowAvailable"] != false || approvals["approvalWorkflowStatus"] != "not_configured" {
		t.Fatalf("approval workflow = %#v", approvals)
	}
	policy := approvals["approvalPolicy"].(map[string]any)
	if policy["largeOrderNotional"] != nil || policy["approvalMode"] != "none" || policy["approvalWorkflowAvailable"] != false {
		t.Fatalf("approval policy = %#v", policy)
	}
}

func TestRealTradeStateNormalizesTypedNilSlices(t *testing.T) {
	var hardStops []map[string]any
	var hardStopEvents []map[string]any
	var killSwitchEvents []map[string]any
	svc := NewService(WithRealTradeRiskState(func() map[string]any {
		return map[string]any{
			"hardStopEntries":  hardStops,
			"hardStopEvents":   hardStopEvents,
			"killSwitchEvents": killSwitchEvents,
		}
	}))

	assertSystemEmptyAnySlice(t, svc.RealTradeHardStops(), "entries")
	assertSystemEmptyAnySlice(t, svc.RealTradeHardStopEvents(), "entries")
	assertSystemEmptyAnySlice(t, svc.RealTradeKillSwitchEvents(), "entries")
}

func TestRealTradeControlDelegatesAndUnavailableBoundaries(t *testing.T) {
	ctx := t.Context()
	empty := NewService()
	for name, run := range map[string]func() error{
		"update runtime risk": func() error {
			_, err := empty.UpdateRealTradeRuntimeRisk(ctx, RealTradeRuntimeRiskCommand{})
			return err
		},
		"disable runtime risk": func() error {
			_, err := empty.DisableRealTradeRuntimeRisk(ctx, RealTradeRuntimeRiskCommand{})
			return err
		},
		"activate kill switch": func() error {
			_, err := empty.ActivateRealTradeKillSwitch(ctx, RealTradeKillSwitchCommand{})
			return err
		},
		"release kill switch": func() error {
			_, err := empty.ReleaseRealTradeKillSwitch(ctx, RealTradeKillSwitchCommand{})
			return err
		},
		"activate hard stop": func() error { _, err := empty.ActivateRealTradeHardStop(ctx, RealTradeHardStopCommand{}); return err },
		"release hard stop": func() error {
			_, err := empty.ReleaseRealTradeHardStop(ctx, "hs-1", RealTradeHardStopCommand{})
			return err
		},
	} {
		if err := run(); !errors.Is(err, errRealTradeControlUnavailable) {
			t.Fatalf("%s error = %v", name, err)
		}
	}

	calls := map[string]bool{}
	svc := NewService(
		WithRealTradeRuntimeRiskControls(
			func(_ context.Context, command RealTradeRuntimeRiskCommand) (map[string]any, error) {
				calls["update-risk"] = command.OperatorID == "operator" && command.RealTradingEnabled
				return map[string]any{"realTradingEnabled": true}, nil
			},
			func(_ context.Context, command RealTradeRuntimeRiskCommand) (map[string]any, error) {
				calls["disable-risk"] = command.Reason == "off"
				return map[string]any{"realTradingEnabled": false}, nil
			},
		),
		WithRealTradeKillSwitchControls(
			func(_ context.Context, command RealTradeKillSwitchCommand) (map[string]any, error) {
				calls["activate-kill"] = command.OperatorID == "operator"
				return map[string]any{"active": true}, nil
			},
			func(_ context.Context, command RealTradeKillSwitchCommand) (map[string]any, error) {
				calls["release-kill"] = command.Reason == "resolved"
				return map[string]any{"active": false}, nil
			},
		),
		WithRealTradeHardStopControls(
			func(_ context.Context, command RealTradeHardStopCommand) (map[string]any, error) {
				calls["activate-stop"] = command.AccountID == "ACC-1"
				return map[string]any{"id": "hs-1"}, nil
			},
			func(_ context.Context, id string, command RealTradeHardStopCommand) (map[string]any, error) {
				calls["release-stop"] = id == "hs-1" && command.OperatorID == "operator"
				return map[string]any{"released": true}, nil
			},
		),
	)
	if result, err := svc.UpdateRealTradeRuntimeRisk(ctx, RealTradeRuntimeRiskCommand{RealTradingEnabled: true, OperatorID: "operator"}); err != nil || result["realTradingEnabled"] != true {
		t.Fatalf("UpdateRealTradeRuntimeRisk = %#v, %v", result, err)
	}
	if result, err := svc.DisableRealTradeRuntimeRisk(ctx, RealTradeRuntimeRiskCommand{Reason: "off"}); err != nil || result["realTradingEnabled"] != false {
		t.Fatalf("DisableRealTradeRuntimeRisk = %#v, %v", result, err)
	}
	if result, err := svc.ActivateRealTradeKillSwitch(ctx, RealTradeKillSwitchCommand{OperatorID: "operator"}); err != nil || result["active"] != true {
		t.Fatalf("ActivateRealTradeKillSwitch = %#v, %v", result, err)
	}
	if result, err := svc.ReleaseRealTradeKillSwitch(ctx, RealTradeKillSwitchCommand{Reason: "resolved"}); err != nil || result["active"] != false {
		t.Fatalf("ReleaseRealTradeKillSwitch = %#v, %v", result, err)
	}
	if result, err := svc.ActivateRealTradeHardStop(ctx, RealTradeHardStopCommand{AccountID: "ACC-1"}); err != nil || result["id"] != "hs-1" {
		t.Fatalf("ActivateRealTradeHardStop = %#v, %v", result, err)
	}
	if result, err := svc.ReleaseRealTradeHardStop(ctx, "hs-1", RealTradeHardStopCommand{OperatorID: "operator"}); err != nil || result["released"] != true {
		t.Fatalf("ReleaseRealTradeHardStop = %#v, %v", result, err)
	}
	for name, called := range calls {
		if !called {
			t.Fatalf("%s delegate did not receive its command", name)
		}
	}

	nilState := NewService(WithRealTradeRiskState(func() map[string]any { return nil }))
	if nilState.RealTradeApprovals()["realTradingEnabled"] != false {
		t.Fatal("nil risk state should remain disabled")
	}
	if boolValue(map[string]any{"enabled": "true"}, "enabled") || boolValue(map[string]any{}, "missing") {
		t.Fatal("boolValue accepted missing or non-boolean values")
	}
	entries := []string{"event"}
	if got := anySliceValue(map[string]any{"entries": entries}, "entries"); len(got.([]string)) != 1 {
		t.Fatalf("anySliceValue = %#v", got)
	}
}

func TestFutuHealthAndResetDelegates(t *testing.T) {
	resetCalled := false
	svc := NewService(
		WithFutuOpenDHealth(func(ctx context.Context) map[string]any {
			return map[string]any{"status": "ok", "ctx": ctx != nil}
		}),
		WithFutuOpenDInstallGuide(func() map[string]any {
			return map[string]any{"available": true}
		}),
		WithResetFutuRuntime(func() {
			resetCalled = true
		}),
		WithBrokerOrderSnapshot(func() map[string]any {
			return map[string]any{"running": true}
		}),
	)

	health := svc.FutuOpenDHealth(context.Background())
	if health["status"] != "ok" || health["ctx"] != true {
		t.Fatalf("health = %#v", health)
	}
	guide := svc.FutuOpenDInstallGuide()
	if guide["available"] != true {
		t.Fatalf("guide = %#v", guide)
	}
	snapshot := svc.BrokerOrderUpdatesSnapshot()
	if snapshot["running"] != true {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	svc.ResetFutuRuntime()
	if !resetCalled {
		t.Fatal("reset callback was not called")
	}
}

func TestFutuHealthDefaultsUnavailable(t *testing.T) {
	health := NewService().FutuOpenDHealth(context.Background())
	if health["status"] != "unavailable" {
		t.Fatalf("status = %v, want unavailable", health["status"])
	}
	if health["reason"] != "futu integration not enabled" {
		t.Fatalf("reason = %v", health["reason"])
	}
}

func assertSystemMapKeys(t *testing.T, values map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := values[key]; !ok {
			t.Fatalf("map missing %q: %#v", key, values)
		}
	}
}

func assertSystemMapMissingKeys(t *testing.T, values map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := values[key]; ok {
			t.Fatalf("map unexpectedly includes %q: %#v", key, values)
		}
	}
}

func assertSystemEmptyAnySlice(t *testing.T, values map[string]any, key string) {
	t.Helper()
	entries, ok := values[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want []any", key, values[key])
	}
	if len(entries) != 0 {
		t.Fatalf("%s length = %d, want 0", key, len(entries))
	}
}
