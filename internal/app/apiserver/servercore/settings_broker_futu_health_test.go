package servercore

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestFutuRuntimeAndHealthDiagnoseEnabledButUnreachableOpenD(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(jfsettings.BrokerIntegration{
		Enabled: true,
		Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
			Host:                    "127.0.0.1",
			APIPort:                 1,
			WebSocketPort:           2,
			WebSocketKey:            "diagnostic-key",
			MaxWebSocketConnections: 3,
			TradeMarket:             "HK",
		}),
	})
	if err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	api := newTestServer(t, store)

	guide := api.futuCoordinator().OpenDInstallGuide()
	settings := guide["settings"].(map[string]any)
	if settings["apiPort"] != 1 || settings["websocketKeyRequired"] != true || settings["marketDataTransport"] != liveQuoteTransportMode {
		t.Fatalf("install guide settings = %#v", settings)
	}

	runtime := api.futuCoordinator().BrokerRuntime(context.Background())
	session := runtime.Session
	if session.Connectivity != "disconnected" || session.AccountsDiscovered != 0 || session.Connection.APIPort != 1 {
		t.Fatalf("runtime session = %#v", session)
	}
	if session.GlobalState != nil {
		t.Fatalf("unreachable runtime globalState = %#v", session.GlobalState)
	}

	health := api.futuCoordinator().OpenDHealth(context.Background())
	diagnosis := health["diagnosis"].(map[string]any)
	healthRuntime := health["runtime"].(map[string]any)
	if health["status"] != "offline" || healthRuntime["connectivity"] != "disconnected" {
		t.Fatalf("health = %#v", health)
	}
	if diagnosis["code"] != "OPEND_API_CONNECTIVITY" || diagnosis["manualRetryRequired"] != true || diagnosis["restartOpenDRecommended"] != true {
		t.Fatalf("diagnosis = %#v", diagnosis)
	}
	if healthRuntime["websocketKeyConfigured"] != true || healthRuntime["marketDataTransport"] != liveQuoteTransportMode {
		t.Fatalf("health runtime = %#v", healthRuntime)
	}
}

func TestFutuOpenDHealthRejectsOldBuildAndGuidesUpgrade(t *testing.T) {
	opendServer := fututestkit.StartBrokerServer(t)
	opendServer.SetServerVersion(1008, 6708)
	defer opendServer.Close()
	host, portText, err := net.SplitHostPort(opendServer.Addr())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi port: %v", err)
	}

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := store.SaveIntegration(jfsettings.BrokerIntegration{
		Enabled: true,
		Config:  normalizeFutuConfig(jfsettings.FutuIntegrationConfig{Host: host, APIPort: port}),
	}); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	api := newTestServer(t, store)

	health := api.futuCoordinator().OpenDHealth(t.Context())
	diagnosis := health["diagnosis"].(map[string]any)
	runtime := health["runtime"].(map[string]any)
	if health["status"] != "degraded" || diagnosis["code"] != "OPEND_VERSION_UNSUPPORTED" {
		t.Fatalf("health = %#v", health)
	}
	if diagnosis["manualRetryRequired"] != true || diagnosis["restartOpenDRecommended"] != false {
		t.Fatalf("diagnosis = %#v", diagnosis)
	}
	serverVersion, ok := runtime["serverVersion"].(*string)
	if !ok || serverVersion == nil || *serverVersion != "10.8.6708" || runtime["minimumVersion"] != futuintegration.MinimumOpenDVersion {
		t.Fatalf("runtime = %#v", runtime)
	}
	if summary, _ := diagnosis["summary"].(string); !strings.Contains(summary, futuintegration.MinimumOpenDVersion) {
		t.Fatalf("summary = %q", summary)
	}
}
