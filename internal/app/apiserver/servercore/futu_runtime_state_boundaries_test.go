package servercore

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func enabledFutuRuntimeBoundaryServer(t *testing.T) *Server {
	t.Helper()
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reservation.Addr().(*net.TCPAddr).Port
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
		Type: "futu", Host: "127.0.0.1", APIPort: port,
	})}); err != nil {
		t.Fatal(err)
	}
	return newTestServer(t, settings)
}

func TestFutuRuntimeRemainingDisconnectedAndResetPaths(t *testing.T) {
	server := enabledFutuRuntimeBoundaryServer(t)
	probe := server.futuCoordinator().Probe(t.Context())
	if probe.Connectivity != "disconnected" || probe.LastError == nil {
		t.Fatalf("closed-port probe = %#v", probe)
	}
	runtime := server.futuCoordinator().BrokerRuntime(t.Context())
	if runtime.Session.Connectivity != "disconnected" {
		t.Fatalf("disconnected broker runtime = %#v", runtime)
	}
	health := server.futuCoordinator().OpenDHealth(t.Context())
	diagnosis, ok := health["diagnosis"].(map[string]any)
	if !ok || diagnosis["code"] != "OPEND_API_CONNECTIVITY" || diagnosis["manualRetryRequired"] != true {
		t.Fatalf("disconnected health = %#v", health)
	}
	server.futuCoordinator().Reset()
}

func TestFutuRuntimeRemainingDisabledProbeAndBoolValue(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	if probe := server.futuCoordinator().Probe(context.Background()); probe.Connectivity != "" || probe.LastError != nil || len(probe.Markets) != 0 {
		t.Fatalf("disabled probe = %#v", probe)
	}
}

func TestFutuRuntimeHealthyProbeAndGlobalStateBoundaries(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()
	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	probe := server.futuCoordinator().Probe(context.Background())
	if probe.Connectivity != "connected" || probe.ServerVersion == nil || len(probe.Markets) != 4 {
		t.Fatalf("healthy OpenD probe = %#v", probe)
	}
	runtime := server.futuCoordinator().BrokerRuntime(context.Background())
	if runtime.Session.BrokerID == "" || runtime.Session.GlobalState == nil {
		t.Fatalf("broker runtime = %#v", runtime)
	}

}
