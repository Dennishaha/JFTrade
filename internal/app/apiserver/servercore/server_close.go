package servercore

import (
	"reflect"
)

// Close idempotently releases the application graph in reverse dependency order.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	registerOwnedResources(s)
	return s.lifecycle.Close()
}

func registerOwnedResources(s *Server) {
	s.lifecycle.EnsureOwnedResources(
		func() {
			s.registerPersistentResources()
			s.registerResource("web authentication", func() error {
				if s.auth != nil {
					s.auth.Close()
				}
				return nil
			})
		},
		func() {
			s.registerRuntimeResources()
		},
	)
}

func (a *serverApplication) registerPersistentResources() {
	a.registerResource("persistent stores", a.stores.Close)
}

func (a *serverApplication) registerRuntimeResources() {
	a.registerResource("runtime providers", a.runtimes.CloseProviders)
	a.registerServiceResources()
	a.registerResource("runtime consumers", a.runtimes.CloseConsumers)
	if !a.ownsAssistantRuntimeComponents() {
		a.registerResource("assistant service", func() error { return closeApplicationResource(a.assistantSvc) })
	}
}

func (a *serverApplication) registerServiceResources() {
	a.registerResource("backtest service", func() error { return closeApplicationResource(a.backtestSvc) })
	a.registerResource("market data service", func() error { return closeApplicationResource(a.marketdataSvc) })
	a.registerResource("trading order updates", a.stopTradingOrderUpdates)
}

func (a *serverApplication) registerResource(name string, closeFn func() error) {
	_ = ownResource(a, name, closeFn)
}

// ownResource registers cleanup before the constructed dependency is
// published from its installer.
func ownResource(a *serverApplication, name string, closeFn func() error) error {
	if a == nil || closeFn == nil {
		return nil
	}
	err := a.lifecycle.Resources().Register(name, closeFn)
	if err != nil {
		a.lifecycle.AddSetupError(err)
	}
	return err
}

func (a *serverApplication) ownsAssistantRuntimeComponents() bool {
	assistantRuntime := a.runtimes.Assistant()
	return assistantRuntime != nil &&
		a.assistantSvc == assistantRuntime.Service()
}

func (a *serverApplication) stopTradingOrderUpdates() error {
	if a.tradingSvc == nil {
		return nil
	}
	return a.tradingSvc.StopOrderUpdates()
}

func closeApplicationResource(closer interface{ Close() error }) error {
	if closer == nil {
		return nil
	}
	value := reflect.ValueOf(closer)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil
	}
	return closer.Close()
}
