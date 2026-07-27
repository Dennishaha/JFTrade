package servercore

import (
	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
)

func (s *serverApplication) newMaintenanceRegistry() *dmsrv.MaintenanceRegistry {
	if s == nil {
		return dmsrv.NewMaintenanceRegistry(nil)
	}

	backtestBusy := dmsrv.BusyCheckers{s.stores.BacktestRuns, s.stores.BacktestTasks}
	strategyRuntime := s.runtimes.StrategyRuntimeMaintenance()
	if strategyRuntime == nil {
		strategyRuntime = s.runtimes.StrategyRuntime()
	}
	var adkRuntime *assistantassembly.DatabaseMaintenance
	var adkSession *assistantassembly.DatabaseMaintenance
	var adkArtifact *assistantassembly.DatabaseMaintenance
	if assistantRuntime := s.runtimes.Assistant(); assistantRuntime != nil {
		adkRuntime = assistantRuntime.DatabaseMaintenance(assistantassembly.MaintenanceRuntimeDatabase)
		adkSession = assistantRuntime.DatabaseMaintenance(assistantassembly.MaintenanceSessionDatabase)
		adkArtifact = assistantRuntime.DatabaseMaintenance(assistantassembly.MaintenanceArtifactDatabase)
	}

	targets := map[string]dmsrv.Target{
		datamigration.DatabaseBacktest: {
			Busy:      backtestBusy,
			Compactor: backteststore.NewKLineMaintenance(s.dataMigrationPath(datamigration.DatabaseBacktest)),
		},
		datamigration.DatabaseBacktestRuns: {
			Busy:      backtestBusy,
			Purger:    s.stores.BacktestRuns,
			Compactor: s.stores.BacktestRuns,
		},
		datamigration.DatabaseStrategy: {
			Busy:      strategyRuntime,
			Purger:    s.stores.Design,
			Compactor: s.stores.Design,
		},
		datamigration.DatabaseExecution: {
			Busy:      s.stores.ExecutionOrders,
			Compactor: s.stores.ExecutionOrders,
		},
		datamigration.DatabaseADK: {
			Busy:      adkRuntime,
			Purger:    adkRuntime,
			Compactor: adkRuntime,
		},
		datamigration.DatabaseADKSession: {
			Busy:      adkSession,
			Compactor: adkSession,
		},
		datamigration.DatabaseADKArtifact: {
			Busy:      adkArtifact,
			Compactor: adkArtifact,
		},
		datamigration.DatabaseWatchlist: {
			Compactor: s.stores.Watchlist,
		},
		datamigration.DatabaseResearch: {
			Compactor: s.stores.Research,
		},
	}
	return dmsrv.NewMaintenanceRegistry(targets)
}

func maintenanceCandidates(candidates []datamigration.CleanupCandidate) []dmsrv.CleanupCandidate {
	converted := make([]dmsrv.CleanupCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		converted = append(converted, dmsrv.CleanupCandidate{
			ID:       candidate.ID,
			Category: candidate.Category,
		})
	}
	return converted
}
