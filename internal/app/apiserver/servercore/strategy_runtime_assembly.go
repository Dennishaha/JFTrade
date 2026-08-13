package servercore

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/strategyapp"

	"github.com/jftrade/jftrade-main/internal/live"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

func newStrategyRuntimeDependencies(server *Server) liveruntime.Dependencies {
	dependencies := liveruntime.Dependencies{
		MarketDataProvider: func() liveruntime.MarketDataSource {
			exchange := server.futuCoordinator().Exchange()
			return exchange
		},
		AccountResolver:        strategyapp.AccountResolver(server.futuCoordinator().ResolveBroker),
		MarketDataCapabilities: strategyapp.MarketDataCapabilities(server.marketdataSvc),
		PineWorker: func() pineWorkerRunner {
			_, runner := server.runtimes.PineWorkerRunners()
			return runner
		}(),
		PineWorkerLimit: func() int {
			return settingsfile.NormalizePineWorkerSettings(server.pineWorkerSettings()).InstanceWorkerLimit
		},
		WakeMarketDataCollector: func() {
			if server.marketdataSvc != nil {
				server.marketdataSvc.WakeCollector()
			}
		},
		CurrentInstance: func(instanceID string) (stratsrv.ManagedInstance, bool) {
			if server.stores.StrategyCatalog == nil {
				return stratsrv.ManagedInstance{}, false
			}
			return server.stores.StrategyCatalog.GetInstance(instanceID)
		},
		AppendRuntimeEvent: func(instanceID string, logMessage string, kind string, detail string) error {
			if server.stores.StrategyCatalog == nil {
				return nil
			}
			return server.stores.StrategyCatalog.AppendRuntimeEvent(instanceID, logMessage, kind, detail)
		},
		TransitionInstance: func(instanceID string, nextStatus string, kind string, detail string) error {
			if server.stores.StrategyCatalog == nil {
				return nil
			}
			_, err := server.stores.StrategyCatalog.TransitionRuntime(instanceID, nextStatus, kind, detail)
			return err
		},
		ReconcileRuntimeFailure: func(instanceID string, detail string) error {
			if server.stores.StrategyCatalog == nil {
				return nil
			}
			return server.stores.StrategyCatalog.ReconcileRuntimeFailure(instanceID, detail)
		},
		RecordNotification: server.recordStrategyRuntimeNotification,
		TradeCommands:      strategyapp.TradeCommands(server.tradingSvc),
	}
	configureStrategyRuntimeStorageDependencies(&dependencies, server)
	return dependencies
}

func configureStrategyRuntimeStorageDependencies(dependencies *liveruntime.Dependencies, server *Server) {
	dependencies.CountRuntimeAudit = func(ctx context.Context, query runtimeactivity.AuditQuery) (int, error) {
		if server.stores.StrategyCatalog == nil {
			return 0, nil
		}
		return server.stores.StrategyCatalog.CountAudit(ctx, query)
	}
	dependencies.UpsertObservation = func(ctx context.Context, snapshot runtimeactivity.ObservationSnapshot) error {
		if server.stores.StrategyCatalog == nil {
			return nil
		}
		return server.stores.StrategyCatalog.UpsertObservation(ctx, snapshot)
	}
	dependencies.AcquireMarketDataLease = func(
		ctx context.Context,
		consumerID string,
		refs []mdsrv.InstrumentRef,
	) (liveruntime.SubscriptionLease, error) {
		if server.marketdataSvc == nil {
			return nil, fmt.Errorf("market-data service is unavailable")
		}
		return server.marketdataSvc.AcquireManagedSubscription(ctx, consumerID, refs)
	}
}
func (s *serverApplication) recordStrategyRuntimeNotification(note liveruntime.Notification) {
	s.recordLiveNotification(live.Notification{
		At:       note.At,
		Level:    note.Level,
		Title:    note.Title,
		Message:  note.Message,
		Source:   note.Source,
		BrokerID: note.BrokerID,
		Category: note.Category,
	})
}
