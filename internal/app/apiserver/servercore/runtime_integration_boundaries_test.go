package servercore

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/settings"
	"github.com/jftrade/jftrade-main/pkg/broker"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestStartupIntegrationRemainsEffectiveWithoutPersistedBrokerSettings(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	integration := jfsettings.BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
			Type:    "futu",
			Host:    "127.0.0.8",
			APIPort: 21110,
		}),
	}
	wrapped := startupIntegrationSettingsStore{
		SidecarSettingsStore: store,
		startupIntegration:   integration,
	}
	server := &Server{serverApplication: serverApplication{store: wrapped}}
	server.runtimes.SetBrokerRegistry(broker.NewRegistry())
	server.initializeMarketdataRuntime()
	t.Cleanup(func() {
		if err := server.runtimes.MarketData().Close(); err != nil {
			t.Errorf("close market data runtime: %v", err)
		}
	})

	if !server.futuIntegrationEnabled() {
		t.Fatal("startup integration was treated as disabled")
	}
	if exchange := server.runtimes.MarketData().Ensure(); exchange == nil {
		t.Fatal("startup integration did not configure the Futu runtime")
	}
	brokers := server.futuCoordinator().BrokerSettings()["brokers"].([]any)
	brokerSettings := brokers[0].(map[string]any)
	if persisted, ok := brokerSettings["integration"].(*jfsettings.BrokerIntegration); !ok || persisted != nil {
		t.Fatalf("startup integration was reported as persisted: %#v", brokerSettings["integration"])
	}
	defaults := brokerSettings["defaults"].(jfsettings.FutuIntegrationConfig)
	if defaults.Host != integration.Config.Host || defaults.APIPort != integration.Config.APIPort {
		t.Fatalf("broker defaults = %#v, want startup config %#v", defaults, integration.Config)
	}
	guideSettings := server.futuCoordinator().OpenDInstallGuide()["settings"].(map[string]any)
	if guideSettings["host"] != integration.Config.Host || guideSettings["apiPort"] != integration.Config.APIPort {
		t.Fatalf("OpenD guide settings = %#v, want startup config %#v", guideSettings, integration.Config)
	}
}

func TestPersistenceStoreUnwrapsStartupIntegrationForMCPSettings(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	valueWrapper := startupIntegrationSettingsStore{
		SidecarSettingsStore: store,
	}
	testCases := map[string]SidecarSettingsStore{
		"value":   valueWrapper,
		"pointer": &valueWrapper,
		"nested": startupIntegrationSettingsStore{
			SidecarSettingsStore: &valueWrapper,
		},
	}
	for name, wrapped := range testCases {
		t.Run(name, func(t *testing.T) {
			persistence := persistenceOnlySettingsStore(wrapped)
			if _, ok := persistence.(settings.MCPServerStore); !ok {
				t.Fatalf("persistence store type %T hides MCP settings", persistence)
			}
		})
	}

	service := settings.NewService(persistenceOnlySettingsStore(testCases["nested"]))
	saved, err := service.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{
		Port:     17888,
		AuthMode: "none",
	})
	if err != nil || saved.Port != 17888 || saved.AuthMode != "none" {
		t.Fatalf("save MCP settings through unwrapped store = %#v, %v", saved, err)
	}
	reset, token, err := service.ResetMCPServerToken()
	if err != nil || token == "" || !reset.TokenConfigured {
		t.Fatalf("reset MCP token through unwrapped store = %#v, token=%q, err=%v", reset, token, err)
	}
	if current := service.GetMCPServerSettings(); !current.TokenConfigured || current.Port != 17888 {
		t.Fatalf("read MCP settings through unwrapped store = %#v", current)
	}
}

func TestFutuBrokerRefreshesAfterRuntimeResetAndStaysHiddenWhenDisabled(t *testing.T) {
	server := enabledFutuRuntimeBoundaryServer(t)
	first := server.activeBroker()
	if first == nil {
		t.Fatal("initial Futu broker is unavailable")
	}

	server.futuCoordinator().Reset()
	second := server.activeBroker()
	if second == nil {
		t.Fatal("Futu broker is unavailable after runtime reset")
	}
	if second == first {
		t.Fatal("runtime reset retained the broker adapter for the closed exchange")
	}

	integration := server.store.Integration()
	integration.Enabled = false
	if _, err := server.store.SaveIntegration(integration); err != nil {
		t.Fatalf("disable Futu integration: %v", err)
	}
	server.futuCoordinator().Reset()
	if active := server.activeBroker(); active != nil {
		t.Fatalf("disabled integration exposed active broker %T", active)
	}
	if selected := server.resolveBroker("futu"); selected != nil {
		t.Fatalf("disabled integration resolved Futu broker %T", selected)
	}
}

func TestExplicitFutuResolutionRestoresRuntimeBrokerAlongsideOtherBrokers(t *testing.T) {
	server := enabledFutuRuntimeBoundaryServer(t)
	server.runtimes.Brokers().Remove(futuintegration.BrokerID)
	other := &runtimeBoundaryBroker{id: "other"}
	server.runtimes.Brokers().Replace(other)

	selected := server.resolveBroker(futuintegration.BrokerID)
	if selected == nil || selected.ID() != futuintegration.BrokerID {
		t.Fatalf("resolve Futu with another broker registered = %T", selected)
	}
	if retained := server.runtimes.Brokers().Lookup(other.ID()); retained != other {
		t.Fatalf("Futu resolution replaced unrelated broker %T", retained)
	}
}

func TestActiveBrokerWaitsForFutuRuntimeResetInvalidation(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := store.SaveIntegration(jfsettings.BrokerIntegration{
		Enabled: true,
		Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
			Type: "futu", Host: "127.0.0.1", APIPort: 11110,
		}),
	}); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}

	server := &Server{serverApplication: serverApplication{store: store}}
	registry := broker.NewRegistry()
	server.runtimes.SetBrokerRegistry(registry)
	closeStarted := make(chan struct{}, 1)
	releaseClose := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseClose) }) }
	t.Cleanup(release)
	runtime := &runtimeBoundaryMarketData{
		active:       &runtimeBoundaryBroker{id: futuintegration.BrokerID},
		resetStarted: closeStarted,
		releaseReset: releaseClose,
	}
	server.runtimes.SetFutuCoordinator(futuapp.New(futuapp.Options{
		Settings: store,
		Registry: registry,
		MarketDataRuntime: func() futuapp.MarketDataRuntime {
			return runtime
		},
	}))

	first := server.activeBroker()
	if first == nil {
		t.Fatal("initial Futu broker is unavailable")
	}
	resetDone := make(chan struct{})
	go func() {
		server.futuCoordinator().Reset()
		close(resetDone)
	}()
	<-closeStarted

	activeResult := make(chan broker.Broker, 1)
	go func() { activeResult <- server.activeBroker() }()
	select {
	case active := <-activeResult:
		release()
		<-resetDone
		t.Fatalf("activeBroker returned %T before reset invalidated the old adapter", active)
	case <-time.After(50 * time.Millisecond):
	}

	release()
	<-resetDone
	second := <-activeResult
	if second == nil || second == first {
		t.Fatalf("active broker after reset = %T, want a fresh Futu adapter", second)
	}
}

type runtimeBoundaryBroker struct {
	id string
}

type runtimeBoundaryMarketData struct {
	mu           sync.Mutex
	active       broker.Broker
	resetStarted chan<- struct{}
	releaseReset <-chan struct{}
}

func (r *runtimeBoundaryMarketData) Reset() {
	r.resetStarted <- struct{}{}
	<-r.releaseReset
	r.mu.Lock()
	r.active = &runtimeBoundaryBroker{id: futuintegration.BrokerID}
	r.mu.Unlock()
}

func (r *runtimeBoundaryMarketData) BBGOExchange() futuintegration.RuntimeExchange { return nil }

func (r *runtimeBoundaryMarketData) Broker() broker.Broker {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

func (r *runtimeBoundaryMarketData) OwnsBroker(candidate broker.Broker) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return candidate != nil && candidate == r.active
}

func (b *runtimeBoundaryBroker) ID() string { return b.id }

func (b *runtimeBoundaryBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: b.id}
}

func (b *runtimeBoundaryBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (b *runtimeBoundaryBroker) Trading() broker.TradingService { return nil }

func (b *runtimeBoundaryBroker) MarketData() broker.MarketDataReader { return nil }
