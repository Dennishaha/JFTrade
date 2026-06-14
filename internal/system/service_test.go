package system

import (
	"context"
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
	assertSystemMapKeys(t, approvals, "realTradingEnabled", "requiredConfirmationText", "maxApprovalAgeMs", "approvalPolicy", "entries")
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
	assertSystemEmptyAnySlice(t, approvals, "entries")
	policy, ok := approvals["approvalPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("approvalPolicy = %#v, want map", approvals["approvalPolicy"])
	}
	if policy["approverAllowlistEnabled"] != false || policy["approverCount"] != 0 {
		t.Fatalf("approvalPolicy = %#v", policy)
	}

	killSwitch := svc.RealTradeKillSwitch()
	assertSystemMapKeys(t, killSwitch, "realTradingEnabled", "killSwitchActive", "killSwitchSource", "envConfiguredActive", "controlPlaneActive", "blockedOperations", "allowsCancel", "entry")
	assertSystemMapMissingKeys(t, killSwitch, "active")
	if killSwitch["killSwitchActive"] != false || killSwitch["allowsCancel"] != true {
		t.Fatalf("killSwitch = %#v", killSwitch)
	}
	if killSwitch["killSwitchSource"] != nil || killSwitch["entry"] != nil {
		t.Fatalf("killSwitch nullable fields = %#v", killSwitch)
	}

	riskLimits := svc.RealTradeRiskLimits()
	assertSystemMapKeys(t, riskLimits, "realTradingEnabled", "riskEnabled", "riskConfigSource", "envConfiguredMaxOrderQuantity", "envConfiguredMaxOrderNotional", "controlPlaneActive", "controlPlaneMaxOrderQuantity", "controlPlaneMaxOrderNotional", "effectiveMaxOrderQuantity", "effectiveMaxOrderNotional", "entry")
	assertSystemMapMissingKeys(t, riskLimits, "enabled")
	if riskLimits["riskEnabled"] != false || riskLimits["entry"] != nil {
		t.Fatalf("riskLimits = %#v", riskLimits)
	}

	riskEvents := svc.RealTradeRiskEvents()
	assertSystemMapKeys(t, riskEvents, "realTradingEnabled", "riskEnabled", "riskConfigSource", "envConfiguredMaxOrderQuantity", "envConfiguredMaxOrderNotional", "controlPlaneActive", "controlPlaneMaxOrderQuantity", "controlPlaneMaxOrderNotional", "effectiveMaxOrderQuantity", "effectiveMaxOrderNotional", "maxOrderQuantity", "maxOrderNotional", "entries")
	assertSystemMapMissingKeys(t, riskEvents, "events")
	if riskEvents["riskEnabled"] != false {
		t.Fatalf("riskEvents riskEnabled = %v, want false", riskEvents["riskEnabled"])
	}
	assertSystemEmptyAnySlice(t, riskEvents, "entries")
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
