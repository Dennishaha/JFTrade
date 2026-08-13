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

type Services struct {
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
}

type RuntimeControllers struct {
	runtimes appruntimes.Handle
}

type RouteDependencies struct {
	store                SidecarSettingsStore
	stores               appstores.Handle
	dataMigration        *datamigration.Manager
	unavailableDatabases dmsrv.AvailabilitySnapshot
	observability        *observability.Recorder
}

type serverApplication struct {
	Services
	RuntimeControllers
	RouteDependencies
	lifecycle appcomposition.Lifecycle
}
