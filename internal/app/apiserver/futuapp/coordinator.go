// Package futuapp coordinates the application-owned Futu runtime lifecycle.
//
// Protocol conversion and OpenD connectivity remain in internal/integration/futu;
// this package owns broker selection, reset ordering, and application projections.
package futuapp

import (
	"context"
	"strings"
	"sync"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// Settings is the persisted settings surface used by Futu application
// coordination. It is defined at the consuming boundary.
type Settings interface {
	Integration() jfsettings.BrokerIntegration
	SavedIntegration() *jfsettings.BrokerIntegration
	ManagedAccounts() []jfsettings.ManagedBrokerAccount
	Onboarding() jfsettings.OnboardingSettings
}

// MarketDataRuntime is the narrow lifecycle and broker surface coordinated by
// the application. The concrete Futu exchange remains in the integration layer.
type MarketDataRuntime interface {
	Reset()
	BBGOExchange() futuintegration.RuntimeExchange
	Broker() broker.Broker
	OwnsBroker(broker.Broker) bool
}

// Options provides runtime dependencies without transferring their ownership
// to the coordinator.
type Options struct {
	Settings            Settings
	Registry            *broker.Registry
	MarketDataRuntime   func() MarketDataRuntime
	RuntimeDependencies func(context.Context) map[string]any
	LiveStreamStats     func() (count int, limit int, atLimit bool)
	MarketDataState     func() mdsrv.RuntimeState
	StopOrderUpdates    func() error
	ResetCollector      func()
	ResumeCollector     func()
}

// Coordinator owns the application-level synchronization and projections for
// the Futu runtime. External runtimes and services remain owned by apiserver.
type Coordinator struct {
	mu sync.RWMutex

	settings            Settings
	registry            *broker.Registry
	marketDataRuntime   func() MarketDataRuntime
	runtimeDependencies func(context.Context) map[string]any
	liveStreamStats     func() (count int, limit int, atLimit bool)
	marketDataState     func() mdsrv.RuntimeState
	stopOrderUpdates    func() error
	resetCollector      func()
	resumeCollector     func()
}

// New creates a Futu application coordinator.
func New(options Options) *Coordinator {
	return &Coordinator{
		settings:            options.Settings,
		registry:            options.Registry,
		marketDataRuntime:   options.MarketDataRuntime,
		runtimeDependencies: options.RuntimeDependencies,
		liveStreamStats:     options.LiveStreamStats,
		marketDataState:     options.MarketDataState,
		stopOrderUpdates:    options.StopOrderUpdates,
		resetCollector:      options.ResetCollector,
		resumeCollector:     options.ResumeCollector,
	}
}

// Enabled reports whether the effective Futu integration is enabled.
func (c *Coordinator) Enabled() bool {
	return c != nil && c.settings != nil && c.settings.Integration().Enabled
}

// Exchange returns the lazily-created stable exchange boundary.
func (c *Coordinator) Exchange() futuintegration.RuntimeExchange {
	runtime := c.runtime()
	if runtime == nil {
		return nil
	}
	return runtime.BBGOExchange()
}

// Broker resolves the Futu broker while excluding stale runtime-owned adapters.
func (c *Coordinator) Broker() broker.Broker {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.futuBrokerLocked()
}

// ActiveBroker returns the active broker, restoring Futu lazily when needed.
func (c *Coordinator) ActiveBroker() broker.Broker {
	if c == nil || c.registry == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activeBrokerLocked()
}

// ResolveBroker resolves an explicit broker without falling back to another
// provider. Futu is restored lazily even when another provider is active.
func (c *Coordinator) ResolveBroker(id string) broker.Broker {
	if c == nil || c.registry == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return c.activeBrokerLocked()
	}
	if selected := c.registry.Lookup(id); selected != nil {
		if id == futuintegration.BrokerID {
			return c.futuBrokerLocked()
		}
		return selected
	}
	if id == futuintegration.BrokerID {
		return c.futuBrokerLocked()
	}
	return nil
}

// AcceptRuntimeBroker publishes an adapter created by MarketDataRuntime. The
// integration invokes this callback while holding its generation lock, so the
// registry's own concurrency control is used to avoid lock inversion.
func (c *Coordinator) AcceptRuntimeBroker(active broker.Broker) {
	if c != nil && c.registry != nil && active != nil {
		c.registry.Replace(active)
	}
}

// Reset stops consumers, invalidates the runtime-owned broker generation, and
// then resumes collection. Broker reads cannot observe the invalidation window.
func (c *Coordinator) Reset() {
	if c == nil {
		return
	}
	if c.stopOrderUpdates != nil {
		besteffort.LogError(c.stopOrderUpdates())
	}
	if c.resetCollector != nil {
		c.resetCollector()
	}
	c.mu.Lock()
	if runtime := c.runtime(); runtime != nil {
		runtime.Reset()
	}
	if c.registry != nil {
		c.registry.Remove(futuintegration.BrokerID)
	}
	c.mu.Unlock()
	if c.resumeCollector != nil {
		c.resumeCollector()
	}
}

// Probe inspects the effective OpenD connection.
func (c *Coordinator) Probe(ctx context.Context) futuintegration.Probe {
	if !c.Enabled() {
		return futuintegration.Probe{}
	}
	config := c.settings.Integration().Config
	return futuintegration.ProbeOpenD(ctx, futuintegration.ProbeConfig{
		Host: config.Host, APIPort: config.APIPort, WebSocketKey: config.WebSocketKey,
	})
}

func (c *Coordinator) activeBrokerLocked() broker.Broker {
	if active := c.registry.ActiveBroker(); active != nil {
		if active.ID() == futuintegration.BrokerID {
			return c.futuBrokerLocked()
		}
		return active
	}
	return c.futuBrokerLocked()
}

func (c *Coordinator) futuBrokerLocked() broker.Broker {
	runtime := c.runtime()
	if existing := c.lookupFutuBroker(); existing != nil {
		if runtime != nil && runtime.OwnsBroker(existing) {
			if !c.Enabled() {
				c.registry.Remove(futuintegration.BrokerID)
				return nil
			}
			active := runtime.Broker()
			if active == nil {
				c.registry.Remove(futuintegration.BrokerID)
				return nil
			}
			c.registry.Replace(active)
			return active
		}
		// Injected adapters have a lifecycle independent of the local runtime.
		return existing
	}
	if !c.Enabled() || runtime == nil {
		return nil
	}
	active := runtime.Broker()
	if active != nil && c.registry != nil {
		c.registry.Replace(active)
	}
	return active
}

func (c *Coordinator) lookupFutuBroker() broker.Broker {
	if c.registry == nil {
		return nil
	}
	return c.registry.Lookup(futuintegration.BrokerID)
}

func (c *Coordinator) runtime() MarketDataRuntime {
	if c == nil || c.marketDataRuntime == nil {
		return nil
	}
	return c.marketDataRuntime()
}

func (c *Coordinator) streamStats() (count int, limit int, atLimit bool) {
	if c == nil || c.liveStreamStats == nil {
		return 0, jfsettings.DefaultLiveWebSocketConnectionLimit, false
	}
	return c.liveStreamStats()
}

func (c *Coordinator) collectorState() mdsrv.RuntimeState {
	if c == nil || c.marketDataState == nil {
		return mdsrv.RuntimeState{}
	}
	return c.marketDataState()
}
