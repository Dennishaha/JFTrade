package backtest

import (
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

// Resource is the application-owned run-store resource. Backtest services
// consume RunStore, while bootstrap and data maintenance use only the extra
// lifecycle, availability and maintenance ports composed here.
type Resource interface {
	btsrv.RunStore
	dmsrv.BusyChecker
	dmsrv.CandidatePurger
	dmsrv.Compactor
	Available() bool
}

// SyncTaskResource composes the backtest service's task port with the busy
// signal consumed by database maintenance.
type SyncTaskResource interface {
	btsrv.SyncTaskStore
	dmsrv.BusyChecker
}

var (
	_ btsrv.RunStore        = (*Store)(nil)
	_ dmsrv.BusyChecker     = (*Store)(nil)
	_ dmsrv.CandidatePurger = (*Store)(nil)
	_ dmsrv.Compactor       = (*Store)(nil)
	_ Resource              = (*Store)(nil)
	_ btsrv.SyncTaskStore   = (*SyncTaskStore)(nil)
	_ dmsrv.BusyChecker     = (*SyncTaskStore)(nil)
	_ SyncTaskResource      = (*SyncTaskStore)(nil)
)
