package futuapp

import (
	"context"
	"net"
	"strconv"
	"testing"

	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
)

func enabledFutuRuntimeBoundaryCoordinator(t *testing.T) *Coordinator {
	t.Helper()
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reservation.Addr().(*net.TCPAddr).Port
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}
	coordinator, _ := coordinatorForOpenD(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	return coordinator
}

func TestFutuRuntimeRemainingDisconnectedAndResetPaths(t *testing.T) {
	coordinator := enabledFutuRuntimeBoundaryCoordinator(t)
	probe := coordinator.Probe(t.Context())
	if probe.Connectivity != "disconnected" || probe.LastError == nil {
		t.Fatalf("closed-port probe = %#v", probe)
	}
	runtime := coordinator.BrokerRuntime(t.Context())
	if runtime.Session.Connectivity != "disconnected" {
		t.Fatalf("disconnected broker runtime = %#v", runtime)
	}
	health := coordinator.OpenDHealth(t.Context())
	diagnosis, ok := health["diagnosis"].(map[string]any)
	if !ok || diagnosis["code"] != "OPEND_API_CONNECTIVITY" || diagnosis["manualRetryRequired"] != true {
		t.Fatalf("disconnected health = %#v", health)
	}
	coordinator.Reset()
}

func TestFutuRuntimeRemainingDisabledProbeAndBoolValue(t *testing.T) {
	coordinator := New(Options{Settings: coordinatorTestSettings{}})
	if probe := coordinator.Probe(context.Background()); probe.Connectivity != "" || probe.LastError != nil || len(probe.Markets) != 0 {
		t.Fatalf("disabled probe = %#v", probe)
	}
}

func TestFutuRuntimeHealthyProbeAndGlobalStateBoundaries(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()
	coordinator, _ := coordinatorForOpenD(t, quoteServer.Addr())
	probe := coordinator.Probe(context.Background())
	if probe.Connectivity != "connected" || probe.ServerVersion == nil || len(probe.Markets) != 4 {
		t.Fatalf("healthy OpenD probe = %#v", probe)
	}
	if probe.ServerVersion == nil {
		t.Fatalf("healthy OpenD probe omitted server version: %#v", probe)
	}
}
