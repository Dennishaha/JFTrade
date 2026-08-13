package servercore

import (
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
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

func brokerExecutionExchangeFor(s *serverApplication) liveruntime.Exchange {
	if !s.futuCoordinator().Enabled() {
		return nil
	}
	return &strategyRuntimeBrokerBridge{
		RuntimeExchange: s.futuCoordinator().Exchange(),
		broker:          s.futuCoordinator().ActiveBroker(),
	}
}
