package servercore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	appstores "github.com/jftrade/jftrade-main/internal/app/apiserver/stores"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/webaccess"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type closeTrackingPineWorkerRunner struct {
	closed int
}

func (runner *closeTrackingPineWorkerRunner) RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	return pineworker.RunScriptResponse{}, nil
}

func (runner *closeTrackingPineWorkerRunner) Close(context.Context) error {
	runner.closed++
	return nil
}

func restorePineWorkerAssetSelector(t *testing.T, asset pineworkerassets.Asset, ok bool, err error) {
	t.Helper()
	previous := selectPineWorkerAsset
	selectPineWorkerAsset = func() (pineworkerassets.Asset, bool, error) {
		return asset, ok, err
	}
	t.Cleanup(func() { selectPineWorkerAsset = previous })
}

func TestServerRuntimeRiskControlsDelegateToControlPlane(t *testing.T) {
	plane, err := trdsrv.NewRealTradeControlPlane(filepath.Join(t.TempDir(), "real-trade-control.json"))
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	server := &Server{}
	server.runtimes.SetRealTradeControl(plane, plane)
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
	if got := riskSnapshot.RealTradingEnabled; got != true {
		t.Fatalf("realTradingEnabled = %#v, want true", got)
	}
	riskEntry := riskSnapshot.RiskEntry
	if riskEntry == nil || riskEntry.OperatorID != "risk-operator" || riskEntry.MaxOrderQuantity == nil || *riskEntry.MaxOrderQuantity != maxQty {
		t.Fatalf("riskEntry = %#v", riskSnapshot.RiskEntry)
	}

	killSnapshot, err := server.activateKillSwitch(t.Context(), system.RealTradeKillSwitchCommand{
		TradingEnvironment: "real",
		OperatorID:         "kill-operator",
		Reason:             "maintenance",
	})
	if err != nil {
		t.Fatalf("activateKillSwitch: %v", err)
	}
	if got := killSnapshot.KillSwitchActive; got != true {
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
	entries := hardStopSnapshot.HardStopEntries
	if len(entries) != 1 || entries[0].BrokerID != "futu" || entries[0].AccountID != "acct-1" {
		t.Fatalf("hardStopEntries = %#v", hardStopSnapshot.HardStopEntries)
	}

	releasedHardStop, err := server.releaseHardStop(t.Context(), entries[0].ID, system.RealTradeHardStopCommand{
		OperatorID: "hard-stop-operator",
		Reason:     "resume symbol",
	})
	if err != nil {
		t.Fatalf("releaseHardStop: %v", err)
	}
	if releasedEntries := releasedHardStop.HardStopEntries; len(releasedEntries) != 0 {
		t.Fatalf("released hardStopEntries = %#v, want empty", releasedHardStop.HardStopEntries)
	}

	releasedKill, err := server.releaseKillSwitch(t.Context(), system.RealTradeKillSwitchCommand{
		OperatorID: "kill-operator",
		Reason:     "resume trading",
	})
	if err != nil {
		t.Fatalf("releaseKillSwitch: %v", err)
	}
	if got := releasedKill.KillSwitchActive; got != false {
		t.Fatalf("released killSwitchActive = %#v, want false", got)
	}

	disabledRisk, err := server.disableRuntimeRiskConfig(t.Context(), system.RealTradeRuntimeRiskCommand{
		OperatorID: "risk-operator",
		Reason:     "disable limits",
	})
	if err != nil {
		t.Fatalf("disableRuntimeRiskConfig: %v", err)
	}
	if got := disabledRisk.RuntimeRiskConfigured; got != false {
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
		auth:     webaccess.NewAuth(jfsettings.SecuritySettings{}),
		frontend: newFrontendServerWithRuntimeConfig(os.DirFS(frontendDir), "http://127.0.0.1:3000"),
		serverApplication: serverApplication{
			stores: appstores.Handle{ExecutionOrders: newExecutionOrderStore()},
		},
	}
	server.runtimes.SetPineWorkerRunners(backtestRunner, instanceRunner)
	runtime := liveruntime.NewManager(liveruntime.Dependencies{})
	server.runtimes.SetStrategyRuntime(runtime, runtime)

	sideEffects := settingsSideEffects(server)
	integration := jfsettings.BrokerIntegration{
		Enabled: true,
		Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
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

	sideEffects.OnExecutionChanged(jfsettings.ExecutionSettings{SeenFillRetentionDays: 12})
	if got := server.stores.ExecutionOrders.SeenFillRetentionDays(); got != 12 {
		t.Fatalf("seenFillRetentionDays = %d, want 12", got)
	}

	if err := sideEffects.OnSecurityChanged(webSecuritySettings(t, false)); err != nil {
		t.Fatalf("OnSecurityChanged enable: %v", err)
	}
	if server.auth == nil || !server.auth.WebAccessEnabled() {
		t.Fatal("OnSecurityChanged should enable Web password auth")
	}
	recorder := httptest.NewRecorder()
	server.frontend.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil))
	if body := recorder.Body.String(); !strings.Contains(body, `"authRequired":true`) {
		t.Fatalf("runtime config body = %q, want authRequired true", body)
	}

	if err := sideEffects.OnSecurityChanged(jfsettings.SecuritySettings{}); err != nil {
		t.Fatalf("OnSecurityChanged disable: %v", err)
	}
	if server.auth.WebAccessEnabled() {
		t.Fatal("OnSecurityChanged should disable Web access")
	}

	sideEffects.OnPineWorkerChanged(jfsettings.PineWorkerSettings{})
	if backtestRunner.closed != 1 || instanceRunner.closed != 1 {
		t.Fatalf("runner close counts = %d/%d, want 1/1", backtestRunner.closed, instanceRunner.closed)
	}
	currentBacktestRunner, currentInstanceRunner := server.runtimes.PineWorkerRunners()
	if currentBacktestRunner != nil || currentInstanceRunner != nil {
		t.Fatalf("pine worker runners = %#v/%#v, want nil when disabled", currentBacktestRunner, currentInstanceRunner)
	}
	server.runtimes.StrategyRuntime().SetExchangeProvider(func() liveruntime.Exchange {
		return newStrategyRuntimeStubExchange()
	})
	err := server.runtimes.StrategyRuntime().Start(t.Context(), stratsrv.ManagedInstance{
		ID:         "disabled-pine-worker",
		Definition: stratsrv.DefinitionSummary{Name: "Disabled Pine Worker"},
		Binding: stratsrv.InstanceBinding{
			Symbols:  []string{"US.AAPL"},
			Interval: "1m",
		},
		Params: map[string]any{
			"script": "//@version=6\nstrategy(\"Disabled Pine Worker\")",
		},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "pine worker") {
		t.Fatalf("strategy runtime start without Pine worker error = %v", err)
	}
}

func TestDataManagementBackendNilManagerBoundaries(t *testing.T) {
	if service := datamigration.NewService(nil); service == nil {
		t.Fatal("datamigration.NewService(nil) = nil")
	}

	backend := datamigration.NewBackend(nil)
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
