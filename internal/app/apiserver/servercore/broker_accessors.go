package servercore

import (
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func persistenceOnlySettingsStore(store SidecarSettingsStore) SidecarSettingsStore {
	switch current := store.(type) {
	case startupIntegrationSettingsStore:
		return persistenceOnlySettingsStore(current.SidecarSettingsStore)
	case *startupIntegrationSettingsStore:
		if current == nil {
			return nil
		}
		return persistenceOnlySettingsStore(current.SidecarSettingsStore)
	case *SettingsStore:
		if current != nil && current.Store != nil {
			return persistenceOnlySettingsStore(current.Store)
		}
	}
	return store
}

func (s *serverApplication) brokerExecutionExchange() liveruntime.Exchange {
	if strategyRuntime := s.runtimes.StrategyRuntime(); strategyRuntime != nil {
		if exchange := strategyRuntime.CurrentExchange(); exchange != nil {
			return exchange
		}
	}
	if !s.futuIntegrationEnabled() {
		return nil
	}
	return &strategyRuntimeBrokerBridge{
		RuntimeExchange: s.futuExchange(),
		broker:          s.activeBroker(),
	}
}

func (s *serverApplication) futuIntegrationEnabled() bool {
	return s.futuCoordinator().Enabled()
}

func (s *serverApplication) futuExchangeOrError() (futuintegration.RuntimeExchange, error) {
	exchange := s.futuExchange()
	if exchange == nil {
		return nil, errFutuIntegrationNotEnabled
	}
	return exchange, nil
}

func (s *serverApplication) futuBrokerOrError() (broker.Broker, error) {
	b := s.futuBroker()
	if b == nil {
		return nil, errFutuIntegrationNotEnabled
	}
	return b, nil
}

// activeBroker returns the currently active broker.Broker from the registry.
// If no broker is registered yet, it restores the adapter from the Futu
// runtime, creating the exchange lazily when needed.
// This is the recommended entry point for all new broker-facing code.
func (s *serverApplication) activeBroker() broker.Broker {
	return s.futuCoordinator().ActiveBroker()
}

// resolveBroker resolves an explicitly selected broker without falling back to
// another provider. Futu is restored lazily by ID even when other providers
// are already registered.
func (s *serverApplication) resolveBroker(id string) broker.Broker {
	return s.futuCoordinator().ResolveBroker(id)
}
