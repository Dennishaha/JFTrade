package servercore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestServerRuntimeRiskControlsDelegateToControlPlane(t *testing.T) {
	plane, err := trdsrv.NewRealTradeControlPlane(filepath.Join(t.TempDir(), "real-trade-control.json"))
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	server := &Server{realTradeControlPlane: plane}
	if got := len(server.systemRiskOptions()); got != 3 {
		t.Fatalf("systemRiskOptions len = %d, want 3", got)
	}

	maxQty := 10.0
	maxNotional := 2500.0
	riskSnapshot, err := server.updateRuntimeRiskConfig(t.Context(), system.RealTradeRuntimeRiskCommand{
		TradingEnvironment: "real",
		RealTradingEnabled: true,
		MaxOrderQuantity:   &maxQty,
		MaxOrderNotional:   &maxNotional,
		OperatorID:         "risk-operator",
		Reason:             "enable limits",
	})
	if err != nil {
		t.Fatalf("updateRuntimeRiskConfig: %v", err)
	}
	if got := riskSnapshot["realTradingEnabled"]; got != true {
		t.Fatalf("realTradingEnabled = %#v, want true", got)
	}
	riskEntry, ok := riskSnapshot["riskEntry"].(*trdsrv.RealTradeRuntimeRiskEntry)
	if !ok || riskEntry == nil || riskEntry.OperatorID != "risk-operator" || riskEntry.MaxOrderQuantity == nil || *riskEntry.MaxOrderQuantity != maxQty {
		t.Fatalf("riskEntry = %#v", riskSnapshot["riskEntry"])
	}

	killSnapshot, err := server.activateKillSwitch(t.Context(), system.RealTradeKillSwitchCommand{
		TradingEnvironment: "real",
		OperatorID:         "kill-operator",
		Reason:             "maintenance",
	})
	if err != nil {
		t.Fatalf("activateKillSwitch: %v", err)
	}
	if got := killSnapshot["killSwitchActive"]; got != true {
		t.Fatalf("killSwitchActive = %#v, want true", got)
	}

	hardStopSnapshot, err := server.activateHardStop(t.Context(), system.RealTradeHardStopCommand{
		BrokerID:           "futu",
		TradingEnvironment: "REAL",
		AccountID:          "acct-1",
		Market:             "US",
		Symbol:             "AAPL",
		HardStopScope:      "symbol",
		OperatorID:         "hard-stop-operator",
		Reason:             "halt symbol",
	})
	if err != nil {
		t.Fatalf("activateHardStop: %v", err)
	}
	entries, ok := hardStopSnapshot["hardStopEntries"].([]trdsrv.RealTradeHardStopEntry)
	if !ok || len(entries) != 1 || entries[0].BrokerID != "futu" || entries[0].AccountID != "acct-1" {
		t.Fatalf("hardStopEntries = %#v", hardStopSnapshot["hardStopEntries"])
	}

	releasedHardStop, err := server.releaseHardStop(t.Context(), entries[0].ID, system.RealTradeHardStopCommand{
		OperatorID: "hard-stop-operator",
		Reason:     "resume symbol",
	})
	if err != nil {
		t.Fatalf("releaseHardStop: %v", err)
	}
	if releasedEntries, ok := releasedHardStop["hardStopEntries"].([]trdsrv.RealTradeHardStopEntry); !ok || len(releasedEntries) != 0 {
		t.Fatalf("released hardStopEntries = %#v, want empty", releasedHardStop["hardStopEntries"])
	}

	releasedKill, err := server.releaseKillSwitch(t.Context(), system.RealTradeKillSwitchCommand{
		OperatorID: "kill-operator",
		Reason:     "resume trading",
	})
	if err != nil {
		t.Fatalf("releaseKillSwitch: %v", err)
	}
	if got := releasedKill["killSwitchActive"]; got != false {
		t.Fatalf("released killSwitchActive = %#v, want false", got)
	}

	disabledRisk, err := server.disableRuntimeRiskConfig(t.Context(), system.RealTradeRuntimeRiskCommand{
		OperatorID: "risk-operator",
		Reason:     "disable limits",
	})
	if err != nil {
		t.Fatalf("disableRuntimeRiskConfig: %v", err)
	}
	if got := disabledRisk["runtimeRiskConfigured"]; got != false {
		t.Fatalf("runtimeRiskConfigured after disable = %#v, want false", got)
	}
}

func TestServerSettingsSideEffectsPropagateRuntimeChanges(t *testing.T) {
	restorePineWorkerAssetSelector(t, pineworkerassets.Asset{}, false, nil)
	t.Setenv(envPineWorkerDisabled, "true")

	frontendDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("WriteFile index.html: %v", err)
	}
	backtestRunner := &closeTrackingPineWorkerRunner{}
	instanceRunner := &closeTrackingPineWorkerRunner{}
	server := &Server{
		auth:                     newWebAuth(SecuritySettings{}),
		frontend:                 newFrontendServerWithRuntimeConfig(os.DirFS(frontendDir), "http://127.0.0.1:3000"),
		executionOrders:          newExecutionOrderStore(),
		backtestPineWorkerRunner: backtestRunner,
		instancePineWorkerRunner: instanceRunner,
		strategyRuntimeManager:   &strategyRuntimeManager{},
	}

	sideEffects := server.settingsSideEffects()
	integration := BrokerIntegration{
		Enabled: true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type:          "futu",
			Host:          "127.0.0.9",
			APIPort:       22222,
			WebSocketKey:  "secret-key",
			TradeMarket:   "US",
			SecurityFirm:  "FUTUSECURITIES",
			WebSocketPort: 11111,
			UseEncryption: false,
		}),
	}
	sideEffects.OnIntegrationChanged(integration)
	if got := os.Getenv("JFTRADE_FUTU_API_PORT"); got != "22222" {
		t.Fatalf("JFTRADE_FUTU_API_PORT = %q, want 22222", got)
	}
	if got := os.Getenv("FUTU_OPEND_WEBSOCKET_KEY"); got != "secret-key" {
		t.Fatalf("FUTU_OPEND_WEBSOCKET_KEY = %q, want secret-key", got)
	}

	sideEffects.OnExecutionChanged(ExecutionSettings{SeenFillRetentionDays: 12})
	if got := server.executionOrders.seenFillRetentionDays; got != 12 {
		t.Fatalf("seenFillRetentionDays = %d, want 12", got)
	}

	if err := sideEffects.OnSecurityChanged(webSecuritySettings(t, false)); err != nil {
		t.Fatalf("OnSecurityChanged enable: %v", err)
	}
	if server.auth == nil || !server.auth.enabled {
		t.Fatal("OnSecurityChanged should enable Web password auth")
	}
	recorder := httptest.NewRecorder()
	server.frontend.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil))
	if body := recorder.Body.String(); !strings.Contains(body, `"authRequired":true`) {
		t.Fatalf("runtime config body = %q, want authRequired true", body)
	}

	if err := sideEffects.OnSecurityChanged(SecuritySettings{}); err != nil {
		t.Fatalf("OnSecurityChanged disable: %v", err)
	}
	if server.auth.enabled {
		t.Fatal("OnSecurityChanged should disable Web access")
	}

	sideEffects.OnPineWorkerChanged(PineWorkerSettings{})
	if backtestRunner.closed != 1 || instanceRunner.closed != 1 {
		t.Fatalf("runner close counts = %d/%d, want 1/1", backtestRunner.closed, instanceRunner.closed)
	}
	if server.backtestPineWorkerRunner != nil || server.instancePineWorkerRunner != nil {
		t.Fatalf("pine worker runners = %#v/%#v, want nil when disabled", server.backtestPineWorkerRunner, server.instancePineWorkerRunner)
	}
	if server.strategyRuntimeManager.pineWorkerRunner != nil {
		t.Fatalf("strategy runtime manager runner = %#v, want nil", server.strategyRuntimeManager.pineWorkerRunner)
	}
}

func TestDataManagementBackendNilManagerBoundaries(t *testing.T) {
	var nilServer *Server
	if service := nilServer.newDataManagementService(); service == nil {
		t.Fatal("newDataManagementService() = nil")
	}

	backend := dataManagementBackend{}
	overview, err := backend.Overview(context.Background(), dmsrv.OverviewRequest{})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	payload, ok := overview.(map[string]any)
	if !ok {
		t.Fatalf("Overview type = %T, want map[string]any", overview)
	}
	if databases, ok := payload["databases"].([]any); !ok || len(databases) != 0 {
		t.Fatalf("Overview databases = %#v, want empty slice", payload["databases"])
	}

	if _, err := backend.PreviewCleanup(context.Background(), dmsrv.CleanupPreviewRequest{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("PreviewCleanup err = %v, want unavailable", err)
	}
	if _, err := backend.ExecuteCleanup(context.Background(), dmsrv.CleanupExecuteRequest{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("ExecuteCleanup err = %v, want unavailable", err)
	}
	if _, err := backend.Compact(context.Background(), "adk", dmsrv.CompactRequest{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Compact err = %v, want unavailable", err)
	}
	if _, err := backend.Rebuild(context.Background(), dmsrv.RebuildRequest{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Rebuild err = %v, want unavailable", err)
	}
}
