package servercore

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func assistantRuntime(server *Server) assistantassembly.Runtime {
	if server == nil {
		return nil
	}
	return server.runtimes.Assistant()
}

func (s *serverApplication) workflowMarketSnapshot(ctx context.Context, instrumentID string) (map[string]any, error) {
	if s == nil {
		return assistantassembly.NewApplicationAdapter(assistantassembly.ApplicationPorts{}).WorkflowMarketSnapshot(ctx, instrumentID)
	}
	return assistantassembly.NewApplicationAdapter(assistantassembly.ApplicationPorts{
		MarketData: func() *mdsrv.Service { return s.marketdataSvc },
	}).WorkflowMarketSnapshot(ctx, instrumentID)
}

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
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
		Type: "futu", Host: "127.0.0.1", APIPort: port,
	})}); err != nil {
		t.Fatal(err)
	}
	return newTestServer(t, store)
}
