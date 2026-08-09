package servercore

import (
	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", portText, err)
	}
	return host, port
}

func newMarketDataTestServerWithQuoteRuntime(t *testing.T, addr string) *Server {
	t.Helper()
	host, port := splitHostPort(t, addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	store.data.Integration = &jfsettings.BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    host,
			APIPort:                 port,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()
	return newTestServer(t, store)
}

func testMarketDataProtoKLine(at time.Time, open, high, low, close float64, volume int64) fututestkit.KLine {
	return fututestkit.KLine{At: at, Open: open, High: high, Low: low, Close: close, Volume: volume}
}

func seedCachedTickSample(server *Server, sample mdsrv.Tick) {
	server.marketdataSvc.Seed(sample)
}
