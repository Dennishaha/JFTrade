package servercore

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestLiveStreamDiagnosticsUseConfiguredLimit(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"interfaces":{"liveWebSocketConnectionLimit":2}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewSettingsStore(settingsPath)
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
			Host:                    "127.0.0.1",
			APIPort:                 11110,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 2,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()

	server := newTestServer(t, store)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	first := dialLiveWebSocket(t, httpServer.URL)
	defer func() { jftradeCheckTestError(t, first.Close()) }()
	second := dialLiveWebSocket(t, httpServer.URL)
	defer func() { jftradeCheckTestError(t, second.Close()) }()

	deadline := time.Now().Add(time.Second)
	for {
		count, _, _ := server.liveStreamStats()
		if count == 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	diagnostics := server.futuCoordinator().SocketDiagnostics(store.Integration().Config)
	if got := diagnostics["configuredOpenDWebSocketLimit"]; got != 2 {
		t.Fatalf("configuredOpenDWebSocketLimit = %#v", got)
	}
	if got := diagnostics["jftradeLiveWebSocketLimit"]; got != 2 {
		t.Fatalf("jftradeLiveWebSocketLimit = %#v", got)
	}
	if got := diagnostics["configuredOpenDWebSocketLimitActive"]; got != false {
		t.Fatalf("configuredOpenDWebSocketLimitActive = %#v", got)
	}
	if got := diagnostics["likelyConnectionSaturation"]; got != true {
		t.Fatalf("likelyConnectionSaturation = %#v", got)
	}
}
