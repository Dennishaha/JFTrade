package servercore

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
)

func enabledFutuRuntimeCoverageServer(t *testing.T) *Server {
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
	if _, err := settings.SaveIntegration(BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type: "futu", Host: "127.0.0.1", APIPort: port,
	})}); err != nil {
		t.Fatal(err)
	}
	return newTestServer(t, settings)
}

func TestFutuRuntimeRemainingDisconnectedAndResetPaths(t *testing.T) {
	server := enabledFutuRuntimeCoverageServer(t)
	probe := server.probeOpenD(t.Context())
	if probe.Connectivity != "disconnected" || probe.LastError == nil {
		t.Fatalf("closed-port probe = %#v", probe)
	}
	runtime := server.brokerRuntime(t.Context())
	session, ok := runtime["session"].(map[string]any)
	if !ok || session["connectivity"] != "disconnected" {
		t.Fatalf("disconnected broker runtime = %#v", runtime)
	}
	health := server.futuOpenDHealth(t.Context())
	diagnosis, ok := health["diagnosis"].(map[string]any)
	if !ok || diagnosis["code"] != "OPEND_API_CONNECTIVITY" || diagnosis["manualRetryRequired"] != true {
		t.Fatalf("disconnected health = %#v", health)
	}
	server.resetFutuRuntime()
}

func TestFutuRuntimeRemainingRetryAndProgramStatusBoundaries(t *testing.T) {
	future := time.Now().UTC().Add(time.Minute)
	text, active := retryState(future)
	if text == nil || !active {
		t.Fatalf("future retry state = %#v, %v", text, active)
	}
	past := time.Now().UTC().Add(-time.Minute)
	text, active = retryState(past)
	if text == nil || active {
		t.Fatalf("past retry state = %#v, %v", text, active)
	}
	status := &commonpb.ProgramStatus{Type: new(commonpb.ProgramStatusType_ProgramStatusType_None)}
	if got := programStatusString(status); strings.Contains(got, ":") || got == "" {
		t.Fatalf("plain program status = %q", got)
	}
}

func TestFutuRuntimeRemainingDisabledProbeAndBoolValue(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	if probe := server.probeOpenD(context.Background()); probe.Connectivity != "" || probe.LastError != nil || len(probe.Markets) != 0 {
		t.Fatalf("disabled probe = %#v", probe)
	}
	if boolValue(nil) {
		t.Fatal("nil bool value was true")
	}
}

func TestFutuRuntimeHealthyProbeAndGlobalStateBoundaries(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()
	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	probe := server.probeOpenD(context.Background())
	if probe.Connectivity != "connected" || probe.ServerVersion == nil || len(probe.Markets) != 4 {
		t.Fatalf("healthy OpenD probe = %#v", probe)
	}
	runtime := server.brokerRuntime(context.Background())
	if runtime["session"] == nil {
		t.Fatalf("broker runtime = %#v", runtime)
	}

	if nilState := openDProbeFromGlobalState("checked", nil); nilState.LastError == nil || nilState.Connectivity != "degraded" {
		t.Fatalf("nil global state = %#v", nilState)
	}
	oldVersion := openDProbeFromGlobalState("checked", &globalpb.S2C{ServerVer: new(int32(1007)), ServerBuildNo: new(int32(1))})
	if oldVersion.IssueCode != "OPEND_VERSION_UNSUPPORTED" || oldVersion.LastError == nil {
		t.Fatalf("old OpenD global state = %#v", oldVersion)
	}
}
