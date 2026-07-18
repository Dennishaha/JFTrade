package servercore

import (
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
)

func persistenceOnlySettingsStore(store SidecarSettingsStore) SidecarSettingsStore {
	if compatibilityStore, ok := store.(*SettingsStore); ok && compatibilityStore.Store != nil {
		return compatibilityStore.Store
	}
	return store
}

func (s *Server) brokerExecutionExchange() strategyRuntimeExchange {
	if s.strategyRuntimeManager != nil && s.strategyRuntimeManager.exchangeProvider != nil {
		if exchange := s.strategyRuntimeManager.exchangeProvider(); exchange != nil {
			return exchange
		}
	}
	if !s.futuIntegrationEnabled() {
		return nil
	}
	return &strategyRuntimeBrokerBridge{
		Exchange: s.futuExchange(),
		broker:   s.activeBroker(),
	}
}

func (s *Server) futuIntegrationEnabled() bool {
	integration := s.store.SavedIntegration()
	return integration != nil && integration.Enabled
}

func (s *Server) futuExchangeOrError() (*futu.Exchange, error) {
	exchange := s.futuExchange()
	if exchange == nil {
		return nil, errFutuIntegrationNotEnabled
	}
	return exchange, nil
}

func (s *Server) futuBrokerOrError() (broker.Broker, error) {
	b := s.futuBroker()
	if b == nil {
		return nil, errFutuIntegrationNotEnabled
	}
	return b, nil
}

// activeBroker returns the currently active broker.Broker from the registry.
// If no broker is registered yet, it triggers futuExchange() which lazily
// creates and registers the default Futu broker.
// This is the recommended entry point for all new broker-facing code.
func (s *Server) activeBroker() broker.Broker {
	if b := s.brokers.ActiveBroker(); b != nil {
		return b
	}
	if !s.futuIntegrationEnabled() {
		return nil
	}
	s.futuExchange()
	return s.brokers.ActiveBroker()
}

// resolveBroker resolves an explicitly selected broker without falling back to
// another provider. Calling activeBroker once preserves lazy registration of
// the configured default integration, but the final lookup always uses id.
func (s *Server) resolveBroker(id string) broker.Broker {
	id = strings.TrimSpace(id)
	if id == "" {
		return s.activeBroker()
	}
	if selected := s.brokers.Lookup(id); selected != nil {
		return selected
	}
	_ = s.activeBroker()
	return s.brokers.Lookup(id)
}
