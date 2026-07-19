package servercore

import (
	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	asst "github.com/jftrade/jftrade-main/internal/assistant"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/live"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/internal/settings"
	watchliststore "github.com/jftrade/jftrade-main/internal/store/watchlist"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/internal/watchlist"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/broker"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

// serverStores groups the persistent stores owned by Server. Every field is
// a database- or file-backed store opened during bootstrap from paths derived
// from the sidecar settings file; stores outlive individual requests and are
// closed with the server.
type serverStores struct {
	store                SidecarSettingsStore
	strategyStore        *strategyCatalogStore
	strategyRuntimeStore *strategyRuntimeStore
	designStore          *strategyDesignStore
	backtestRuns         *backtestRunStore
	backtestSyncTasks    *backtestSyncTaskStore
	executionOrders      *executionOrderStore
	watchlistStore       *watchliststore.Store
}

// serverRuntimes groups the in-process runtimes and integrations owned by
// Server: the broker registry, market data runtime, live event fan-out,
// ADK/MCP runtimes, exchange calendars, pine worker runners and the
// real-trade control plane. Unlike serverStores these hold live connections
// and background goroutines rather than persisted state.
type serverRuntimes struct {
	strategyRuntimeManager   *strategyRuntimeManager
	liveWebSocket            *apilive.Handler
	liveNotifications        *live.ReplayPublisher
	liveNotificationSink     func(live.Event) live.NotificationDelivery
	marketdataRuntime        *futuintegration.MarketDataRuntime
	brokers                  *broker.Registry // Unified broker registry for multi-broker support
	adkRuntime               *jfadk.Runtime
	mcpServer                *mcpServerManager
	exchangeCalendars        *exchangecalendar.Manager
	previousCalendarResolver marketcalendar.Resolver
	backtestPineWorkerRunner pineWorkerRunner
	instancePineWorkerRunner pineWorkerRunner
	realTradeControlPlane    *trdsrv.RealTradeControlPlane
	preTradeRiskGateway      trdsrv.PreTradeRiskGateway
}

// serverFacades groups the business facade services shared by the HTTP, ADK
// and MCP surfaces. Facades are assembled after the stores and runtimes and
// expose the stable service API that handlers delegate to.
type serverFacades struct {
	assistantSvc       *asst.Service
	sysSvc             *system.Service
	settingsSvc        *settings.Service
	dataManagementSvc  *dmsrv.Service
	backtestSvc        *btsrv.Service
	strategySvc        *stratsrv.Service
	marketdataSvc      *mdsrv.Service
	productFeaturesSvc *productsrv.Service
	watchlistSvc       *watchlist.Service
	tradingSvc         *trdsrv.Service
}
