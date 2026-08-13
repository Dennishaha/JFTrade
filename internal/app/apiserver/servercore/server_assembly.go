package servercore

import (
	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	appruntimes "github.com/jftrade/jftrade-main/internal/app/apiserver/runtimes"
	appstores "github.com/jftrade/jftrade-main/internal/app/apiserver/stores"
	asst "github.com/jftrade/jftrade-main/internal/assistant"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/internal/research"
	"github.com/jftrade/jftrade-main/internal/settings"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jftrade/jftrade-main/pkg/observability"
)

// serverApplication is the single application dependency entry owned by
// Server. It holds domain stores, runtimes, facades, maintenance state and the
// resource lifecycle; HTTP, security and frontend plumbing remain on Server.
// The zero value is intentionally usable so narrow Server literals in tests
// retain their nil-safe behavior.
type serverApplication struct {
	store    SidecarSettingsStore
	stores   appstores.Handle
	runtimes appruntimes.Handle

	assistantSvc       *asst.Service
	sysSvc             *system.Service
	settingsSvc        *settings.Service
	dataManagementSvc  *dmsrv.Service
	backtestSvc        *btsrv.Service
	strategySvc        *stratsrv.Service
	marketdataSvc      *mdsrv.Service
	productFeaturesSvc *productsrv.Service
	watchlistSvc       *watchlist.Service
	researchSvc        *research.Service
	tradingSvc         *trdsrv.Service

	dataMigration        *datamigration.Manager
	unavailableDatabases dmsrv.AvailabilitySnapshot
	observability        *observability.Recorder
	lifecycle            appcomposition.Lifecycle
}
