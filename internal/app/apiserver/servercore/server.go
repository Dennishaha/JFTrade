package servercore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	asst "github.com/jftrade/jftrade-main/internal/assistant"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/live"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/settings"
	exchangecalendarstore "github.com/jftrade/jftrade-main/internal/store/exchangecalendar"
	watchliststore "github.com/jftrade/jftrade-main/internal/store/watchlist"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/internal/watchlist"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
	"github.com/jftrade/jftrade-main/pkg/observability"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineengine"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

const (
	defaultFutuHost                  = "127.0.0.1"
	defaultFutuAPIPort               = 11110
	defaultFutuWebSocketPort         = 11111
	defaultMaxWebSocketClients       = 20
	strategyListLogsTailSize         = 20
	exchangeCalendarOperationTimeout = 75 * time.Second
	observabilityMinImportanceEnv    = "JFTRADE_OBSERVABILITY_MIN_IMPORTANCE"
)

type envelope = httpserver.Envelope
type apiError = httpserver.APIError

var errFutuIntegrationNotEnabled = errors.New("futu integration is not enabled")

type Server struct {
	store                    SidecarSettingsStore
	strategyStore            *strategyCatalogStore
	strategyRuntimeStore     *strategyRuntimeStore
	strategyRuntimeManager   *strategyRuntimeManager
	designStore              *strategyDesignStore
	backtestRuns             *backtestRunStore
	backtestSyncTasks        *backtestSyncTaskStore
	executionOrders          *executionOrderStore
	watchlistStore           *watchliststore.Store
	liveWebSocket            *apilive.Handler
	liveNotifications        *live.ReplayPublisher
	liveNotificationSink     func(live.Event) live.NotificationDelivery
	closeOnce                sync.Once
	closeErr                 error
	marketdataRuntime        *futuintegration.MarketDataRuntime
	brokers                  *broker.Registry // Unified broker registry for multi-broker support
	adkRuntime               *jfadk.Runtime
	assistantSvc             *asst.Service
	frontend                 *frontendServer
	apiPort                  int
	auth                     *webAuth
	router                   *gin.Engine
	desktopMode              bool
	exchangeCalendars        *exchangecalendar.Manager
	previousCalendarResolver marketcalendar.Resolver
	sysSvc                   *system.Service
	settingsSvc              *settings.Service
	dataManagementSvc        *dmsrv.Service
	dataMigration            *datamigration.Manager
	unavailableDatabases     map[string]error
	backtestSvc              *btsrv.Service
	strategySvc              *stratsrv.Service
	marketdataSvc            *mdsrv.Service
	watchlistSvc             *watchlist.Service
	tradingSvc               *trdsrv.Service
	preTradeRiskGateway      trdsrv.PreTradeRiskGateway
	realTradeControlPlane    *trdsrv.RealTradeControlPlane
	backtestPineWorkerRunner pineWorkerRunner
	instancePineWorkerRunner pineWorkerRunner
	observability            *observability.Recorder
	desktopAPIToken          string
	webAccessReconfigure     func(SecuritySettings) error
}

// SidecarHandler is the minimal server surface required by API sidecar assembly.
type SidecarHandler interface {
	http.Handler
	WebAccessHandler() http.Handler
	Close() error
	SetAPIPort(int)
	ConfigureAuthOrigins(...string)
	SetFrontendFS(fs.FS, string)
	ApplySecuritySettings(SecuritySettings)
	SetWebAccessReconfigure(func(SecuritySettings) error)
}

// SidecarOptions customizes API sidecar assembly for embedded hosts.
type SidecarOptions struct {
	FrontendFS        fs.FS
	FrontendDevURL    string
	RuntimeAPIBaseURL string
	NotificationSink  func(live.Event) live.NotificationDelivery
	DesktopMode       bool
	DesktopAPIToken   string
}

// SidecarSettingsStore is the settings surface required by the legacy HTTP server.
type SidecarSettingsStore interface {
	settings.Store
}

type opendProbe struct {
	CheckedAt        string
	Connectivity     string
	Status           string
	LastError        *string
	QuoteLoggedIn    *bool
	TradeLoggedIn    *bool
	ServerVersion    *string
	ProgramStatus    *string
	ProgramTimestamp *string
	Markets          []map[string]any
}

// StartForRunArgs,
// shouldStartForArgs, and envOrDefault are defined in server_startup.go.

func NewServer(store *SettingsStore) *Server {
	return newServerWithFrontend(store, newFrontendServer(loadFrontendFS()))
}

// NewSidecarHandler creates the HTTP handler used by API sidecar assembly.
func NewSidecarHandler(store *SettingsStore, frontendFS fs.FS, runtimeAPIBaseURL string) SidecarHandler {
	return NewSidecarHandlerWithStore(store, frontendFS, runtimeAPIBaseURL)
}

// NewSidecarHandlerWithStore creates the HTTP handler from an abstract settings store.
func NewSidecarHandlerWithStore(store SidecarSettingsStore, frontendFS fs.FS, runtimeAPIBaseURL string) SidecarHandler {
	return NewSidecarHandlerWithOptions(store, SidecarOptions{
		FrontendFS:        frontendFS,
		RuntimeAPIBaseURL: runtimeAPIBaseURL,
	})
}

// NewSidecarHandlerWithOptions creates the HTTP handler from an abstract settings store.
func NewSidecarHandlerWithOptions(store SidecarSettingsStore, options SidecarOptions) SidecarHandler {
	server := newServerWithFrontend(store, newFrontendServerWithOptions(options.FrontendFS, options.RuntimeAPIBaseURL, options.FrontendDevURL))
	server.liveNotificationSink = options.NotificationSink
	server.desktopMode = options.DesktopMode
	server.desktopAPIToken = strings.TrimSpace(options.DesktopAPIToken)
	if server.auth != nil {
		server.auth.enforceAccess = !options.DesktopMode || server.desktopAPIToken != ""
	}
	server.applySecuritySettings(store.SecuritySettings())
	return server
}

// SetAPIPort updates the API port exposed by system status responses.
func (s *Server) SetAPIPort(port int) {
	if s != nil {
		s.apiPort = port
	}
}

// ConfigureAuthOrigins allows API sidecar assembly to add trusted origins.
func (s *Server) ConfigureAuthOrigins(origins ...string) {
	if s != nil && s.auth != nil {
		s.auth.configureOrigins(origins...)
	}
}

// SetFrontendFS mounts frontend assets with the runtime API base URL.
func (s *Server) SetFrontendFS(frontendFS fs.FS, runtimeAPIBaseURL string) {
	if s != nil {
		s.frontend = newFrontendServerWithRuntimeConfig(frontendFS, runtimeAPIBaseURL)
		if s.frontend != nil {
			s.frontend.setDesktopMode(s.desktopMode)
		}
	}
}

// ApplySecuritySettings applies optional Web access settings to API and frontend.
func (s *Server) ApplySecuritySettings(settings SecuritySettings) {
	if s != nil {
		s.applySecuritySettings(settings)
	}
}

// SetWebAccessReconfigure installs the desktop lifecycle callback that owns
// the optional browser listener. Non-desktop servers keep applying settings
// directly without a separate listener.
func (s *Server) SetWebAccessReconfigure(reconfigure func(SecuritySettings) error) {
	if s != nil {
		s.webAccessReconfigure = reconfigure
	}
}

type serverBootstrap struct {
	settingsPath         string
	backtestDBPath       string
	dataMigration        *datamigration.Manager
	unavailableDatabases map[string]error
}

type serverPersistentState struct {
	strategyStore       *strategyCatalogStore
	runtimeStore        *strategyRuntimeStore
	designStore         *strategyDesignStore
	backtestRunStore    *backtestRunStore
	executionOrderStore *executionOrderStore
	watchlistStore      *watchliststore.Store
	auth                *webAuth
}

func newServerWithFrontend(store SidecarSettingsStore, frontend *frontendServer) *Server {
	bootstrap := newServerBootstrap(store)
	state := bootstrap.loadPersistentState(store)
	server := newBootstrapServer(store, frontend, bootstrap, state)
	server.initializeBootstrapState(store, bootstrap, state)
	server.router = server.buildRouter()
	return server
}

func newServerBootstrap(store SidecarSettingsStore) serverBootstrap {
	bootstrap := serverBootstrap{
		settingsPath:         store.Path(),
		backtestDBPath:       deriveBacktestDBPath(),
		unavailableDatabases: make(map[string]error),
	}
	removeLegacyAdminKey(bootstrap.settingsPath)
	bootstrap.dataMigration = datamigration.NewManager(bootstrap.settingsPath, bootstrap.backtestDBPath)
	if err := ensureRuntimeLayout(bootstrap.settingsPath, bootstrap.backtestDBPath); err != nil {
		log.Printf("JFTrade runtime layout unavailable: %v", err)
	}
	bootstrap.probeBacktestDatabase()
	return bootstrap
}

func (b *serverBootstrap) recordUnavailable(id string, err error) {
	if err == nil {
		return
	}
	b.unavailableDatabases[id] = err
	b.dataMigration.SetUnavailable(id, err)
	log.Printf("JFTrade %s database unavailable: %v", id, err)
}

func (b *serverBootstrap) probeBacktestDatabase() {
	backtestStore, err := bt.NewFutuKLineStore(b.backtestDBPath)
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseBacktest, err)
		return
	}
	if err := backtestStore.Close(); err != nil {
		log.Printf("JFTrade backtest database close failed: %v", err)
	}
}

func (b serverBootstrap) loadPersistentState(store SidecarSettingsStore) serverPersistentState {
	state := serverPersistentState{
		strategyStore:       b.loadStrategyStore(),
		designStore:         b.loadDesignStore(),
		backtestRunStore:    b.loadBacktestRunStore(),
		executionOrderStore: b.loadExecutionOrderStore(store.ExecutionSettings()),
		watchlistStore:      b.loadWatchlistStore(),
		auth:                newWebAuth(store.SecuritySettings()),
	}
	if state.strategyStore != nil {
		state.runtimeStore = state.strategyStore.runtimeStore
	} else {
		state.strategyStore = b.newFallbackStrategyStore()
	}
	return state
}

func (b *serverBootstrap) loadStrategyStore() *strategyCatalogStore {
	store, err := NewStrategyCatalogStore(deriveStrategyCatalogPath(b.settingsPath), deriveStrategyPluginTargetDir(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseStrategy, err)
	}
	return store
}

func (b serverBootstrap) newFallbackStrategyStore() *strategyCatalogStore {
	path := deriveStrategyCatalogPath(b.settingsPath)
	return &strategyCatalogStore{
		path:      path,
		dbPath:    deriveStrategyCatalogDBPath(path),
		targetDir: deriveStrategyPluginTargetDir(b.settingsPath),
		data:      strategyCatalogFile{TargetDir: deriveStrategyPluginTargetDir(b.settingsPath)},
	}
}

func (b *serverBootstrap) loadDesignStore() *strategyDesignStore {
	path := deriveStrategyDesignPath(b.settingsPath)
	store, err := NewStrategyDesignStore(path)
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseStrategy, err)
		return &strategyDesignStore{path: path, dbPath: deriveStrategyDesignDBPath(path)}
	}
	return store
}

func (b *serverBootstrap) loadBacktestRunStore() *backtestRunStore {
	store, err := newBacktestRunStoreWithDB(deriveBacktestRunDBPath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseBacktestRuns, err)
		return newBacktestRunStore()
	}
	return store
}

func (b *serverBootstrap) loadExecutionOrderStore(settings ExecutionSettings) *executionOrderStore {
	store, err := newExecutionOrderStoreWithDB(deriveExecutionOrderDBPath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseExecution, err)
		store = newExecutionOrderStore()
	}
	store.configureSeenFillRetention(settings.SeenFillRetentionDays)
	return store
}

func newBootstrapServer(store SidecarSettingsStore, frontend *frontendServer, bootstrap serverBootstrap, state serverPersistentState) *Server {
	minimumImportance := observability.NormalizeMinimumImportance(os.Getenv(observabilityMinImportanceEnv))
	observability.SetMinimumImportance(minimumImportance)
	server := &Server{
		store:                store,
		strategyStore:        state.strategyStore,
		strategyRuntimeStore: state.runtimeStore,
		designStore:          state.designStore,
		backtestRuns:         state.backtestRunStore,
		backtestSyncTasks:    newBacktestSyncTaskStore(),
		executionOrders:      state.executionOrderStore,
		watchlistStore:       state.watchlistStore,
		liveNotifications:    live.NewReplayPublisher(),
		brokers:              broker.NewRegistry(),
		apiPort:              portFromBind(defaultDevelopmentAPIBind, 3000),
		frontend:             frontend,
		auth:                 state.auth,
		dataMigration:        bootstrap.dataMigration,
		unavailableDatabases: bootstrap.unavailableDatabases,
		observability: observability.NewRecorderWithConfig(observability.RecorderConfig{
			EventLimit:        20,
			SlowThreshold:     750 * time.Millisecond,
			MinimumImportance: minimumImportance,
		}),
	}
	server.liveWebSocket = apilive.NewHandler(liveWebSocketBackend{server: server}, apilive.Options{
		DataInterval:            liveTickDispatchInterval,
		SecurityDetailsInterval: marketSecurityDetailsStreamInterval,
		DepthRefreshInterval:    marketDepthStreamRefreshInterval,
	})
	return server
}

func (s *Server) initializeSecurityAndCalendars(store SidecarSettingsStore, settingsPath string) {
	s.applySecuritySettings(store.SecuritySettings())
	s.exchangeCalendars = exchangecalendar.NewManager(
		exchangecalendarstore.New(apiruntime.DeriveExchangeCalendarDir(settingsPath)),
		func() ExchangeCalendarSettings {
			return persistenceOnlySettingsStore(store).ExchangeCalendarSettings()
		},
		exchangecalendar.WithAlertSink(func(alert exchangecalendar.SourceAlert) {
			s.recordExchangeCalendarAlert(alert)
		}),
	)
	s.previousCalendarResolver = marketpkg.SwapCalendarResolver(s.exchangeCalendars)
	s.exchangeCalendars.Start()
}

func (s *Server) initializeADKRuntime(bootstrap serverBootstrap) {
	bootstrap.probeADKDatabase()
	bootstrap.probeADKSessionDatabase()
	if bootstrap.unavailableDatabases[datamigration.DatabaseADK] == nil &&
		bootstrap.unavailableDatabases[datamigration.DatabaseADKSession] == nil {
		s.adkRuntime = newADKRuntime(s, bootstrap.settingsPath)
	}
	s.refreshUnavailableDatabaseStatuses()
}

func (b *serverBootstrap) probeADKDatabase() {
	adkStore, err := jfadk.NewStore(
		apiruntime.DeriveADKDBPath(b.settingsPath),
		apiruntime.DeriveADKSecretsPath(b.settingsPath),
		apiruntime.DeriveADKSkillsDir(b.settingsPath),
	)
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseADK, err)
		return
	}
	if err := adkStore.Close(); err != nil {
		log.Printf("JFTrade ADK database close failed: %v", err)
	}
}

func (b *serverBootstrap) probeADKSessionDatabase() {
	sessionService, err := jfadk.NewSQLiteSessionService(apiruntime.DeriveADKSessionDBPath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseADKSession, err)
		return
	}
	if err := jfadk.CloseSessionService(sessionService); err != nil {
		log.Printf("JFTrade ADK session database close failed: %v", err)
	}
}

func (s *Server) refreshUnavailableDatabaseStatuses() {
	statuses, err := s.dataMigration.Statuses(context.Background())
	if err != nil {
		log.Printf("JFTrade database status inspection failed: %v", err)
		return
	}
	for _, status := range statuses {
		if status.Status == "ready" {
			continue
		}
		reason := status.Error
		if strings.TrimSpace(reason) == "" {
			reason = "database was not initialized"
		}
		s.unavailableDatabases[status.ID] = fmt.Errorf("%s", reason)
	}
}

func (s *Server) initializeAssistantService() {
	s.assistantSvc = asst.NewService(
		s.adkRuntime,
		asst.WithRuntimeSettings(func() any { return s.store.ADKSettings() }),
		asst.WithStreamIdleTimeout(func() int { return s.store.ADKSettings().StreamIdleTimeoutMs }),
		asst.WithOptimizationRuns(assistantOptimizationRuns{server: s}),
		asst.WithWorkflowMarketSnapshot(func(ctx context.Context, instrumentID string) (map[string]any, error) {
			return s.workflowMarketSnapshot(ctx, instrumentID)
		}),
	)
}

func (s *Server) initializeMarketdataRuntime() {
	s.marketdataRuntime = futuintegration.NewMarketDataRuntime(futuintegration.MarketDataRuntimeOptions{
		ConfigSource: func() futuintegration.MarketDataConfig {
			integration := s.store.SavedIntegration()
			if integration == nil {
				return futuintegration.MarketDataConfig{}
			}
			return futuintegration.MarketDataConfig{
				Enabled:      integration.Enabled,
				Host:         integration.Config.Host,
				APIPort:      integration.Config.APIPort,
				WebSocketKey: integration.Config.WebSocketKey,
			}
		},
		OnExchange: func(exchange *futu.Exchange) {
			exchange.OnSystemNotify(s.handleFutuSystemNotify)
			if s.brokers != nil {
				s.brokers.Replace(futu.NewBrokerAdapter(exchange))
			}
		},
	})
}

func (s *Server) reconcileStrategyRuntimeStates() {
	if _, unavailable := s.unavailableDatabases[datamigration.DatabaseStrategy]; unavailable {
		return
	}
	reconciled, err := s.strategyStore.reconcileRuntimeStatesOnStartup()
	if err != nil {
		log.Printf("JFTrade strategy runtime state reconciliation failed: %v", err)
		return
	}
	if reconciled > 0 {
		log.Printf("JFTrade reconciled %d stale strategy runtime state(s) to STOPPED during startup", reconciled)
	}
}

func (s *Server) startLiveNotifications() {
	if err := s.liveNotifications.Start(bbgoNotificationSource{}); err != nil {
		log.Printf("JFTrade BBGO notification source unavailable: %v", err)
	}
}

func (s *Server) initializeRealTradeControl(bootstrap serverBootstrap) {
	controlPlane, err := trdsrv.NewRealTradeControlPlane(deriveRealTradeControlPath(bootstrap.settingsPath))
	if err != nil {
		bootstrap.recordUnavailable("real-trade-control", err)
	}
	s.realTradeControlPlane = controlPlane
	s.preTradeRiskGateway = controlPlane
}

func (s *Server) initializeSystemService(bootstrap serverBootstrap) {
	opts := append(s.systemCoreOptions(bootstrap.settingsPath, bootstrap.backtestDBPath), s.systemCalendarOptions()...)
	opts = append(opts, s.systemRuntimeOptions()...)
	opts = append(opts, s.systemRiskOptions()...)
	s.sysSvc = system.NewService(opts...)
}

func (s *Server) systemCoreOptions(settingsPath string, backtestDBPath string) []system.Option {
	return []system.Option{
		system.WithAPIPortFunc(func() int { return s.apiPort }),
		system.WithSettingsPath(settingsPath),
		system.WithDefaultTradingEnvironmentFunc(func() string { return s.defaultTradingEnvironment() }),
		system.WithBrokerDescriptor(func() map[string]any { return s.descriptor() }),
		system.WithStrategyRuntimeSummary(func() map[string]any { return s.strategyRuntimeSummary() }),
		system.WithLiveStats(func() map[string]any { return s.liveStatsSummary() }),
		system.WithMarketdataRuntimeSummary(func() map[string]any { return s.marketdataRuntimeSummary() }),
		system.WithRuntimeResources(func() map[string]any {
			return apiruntime.RuntimeResourceSummary(settingsPath, backtestDBPath)
		}),
		system.WithBrokerOrderSnapshot(func() map[string]any {
			if s.tradingSvc == nil {
				return map[string]any{}
			}
			return s.tradingSvc.OrderUpdatesSnapshot()
		}),
	}
}

func (s *Server) systemRuntimeOptions() []system.Option {
	return []system.Option{
		system.WithFutuOpenDHealth(func(ctx context.Context) map[string]any { return s.futuOpenDHealth(ctx) }),
		system.WithFutuOpenDInstallGuide(func() map[string]any { return s.futuOpenDInstallGuide() }),
		system.WithResetFutuRuntime(func() { s.resetFutuRuntime() }),
		system.WithRuntimeDependencies(func(ctx context.Context) map[string]any { return s.runtimeDependencies(ctx) }),
		system.WithRequestObservability(func() any { return s.observability.Snapshot() }),
		system.WithRealTradeRiskState(func() map[string]any {
			if s.preTradeRiskGateway == nil {
				return nil
			}
			return s.preTradeRiskGateway.Snapshot()
		}),
	}
}

func (s *Server) initializeBacktestService(state serverPersistentState) {
	backtestRunner, instanceRunner := s.startPineWorkerManagers()
	s.backtestPineWorkerRunner = backtestRunner
	s.instancePineWorkerRunner = instanceRunner
	if instanceRunner != nil && s.strategyRuntimeManager != nil {
		s.strategyRuntimeManager.pineWorkerRunner = instanceRunner
	}
	s.backtestSvc = btsrv.NewService(s.backtestServiceOptions(state, backtestRunner)...)
}

func (s *Server) backtestServiceOptions(state serverPersistentState, runner pineWorkerRunner) []btsrv.Option {
	opts := []btsrv.Option{
		btsrv.WithRunStore(&backtestRunStoreAdapter{store: state.backtestRunStore}),
		btsrv.WithSyncTaskStore(&backtestSyncTaskStoreAdapter{store: s.backtestSyncTasks}),
		btsrv.WithStrategyProvider(&strategyProviderAdapter{store: state.designStore}),
		btsrv.WithDBPathFn(func() string { return deriveBacktestDBPath() }),
		btsrv.WithNewKLineSyncerFn(futuintegration.NewKLineSyncer),
	}
	if runner != nil {
		opts = append(opts, btsrv.WithPineWorkerRunner(runner))
	}
	return opts
}

func (s *Server) initializeStrategyService(state serverPersistentState) {
	s.strategySvc = stratsrv.NewService(
		&strategyDesignStoreAdapter{store: state.designStore},
		&strategyCatalogStoreAdapter{store: state.strategyStore, designStore: state.designStore, runtimeMgr: s.strategyRuntimeManager},
		&strategyRuntimeManagerAdapter{mgr: s.strategyRuntimeManager},
		stratsrv.WithPineAnalyzer(s.analyzePineScript),
		stratsrv.WithLiveMarketStreamRefresher(func(ctx context.Context) {
			s.ensureLiveMarketStream(ctx, s.activeLiveStreamInstrumentIDs(nil))
		}),
	)
}

func (s *Server) analyzePineScript(input stratsrv.PineAnalyzeInput) (stratsrv.PineAnalysisResult, error) {
	analysis := strategypine.AnalyzeScript(input.Script, strategypine.AnalysisOptions{IncludeAST: input.IncludeAST})
	response := map[string]any{
		"ok":               analysis.OK,
		"sourceFormat":     strategypinespec.SourceFormat,
		"runtime":          strategypinespec.Runtime,
		"normalizedScript": analysis.NormalizedScript,
		"diagnostics":      analysis.Diagnostics,
		"warnings":         analysis.Warnings,
		"externalEngine":   pineengine.PayloadMap(pineengine.ShadowPayloadForScript(input.Script)),
		"metadata":         strategyMetadataPayload(analysis.Program),
		"hooks":            buildCompiledHookKinds(analysis.Program),
		"requirements":     buildCompiledRequirementsPayload(analysis.Requirements),
		"features":         analysis.Features,
	}
	if len(analysis.Visuals) > 0 {
		response["visuals"] = analysis.Visuals
	}
	if len(analysis.Declarations) > 0 {
		response["declarations"] = analysis.Declarations
	}
	if len(analysis.CollectionOperations) > 0 {
		response["collectionOperations"] = analysis.CollectionOperations
	}
	if len(analysis.ObjectOperations) > 0 {
		response["objectOperations"] = analysis.ObjectOperations
	}
	if input.IncludeAST {
		response["ast"] = analysis.AST
		response["semantic"] = analysis.Semantic
	}
	return response, nil
}

func (s *Server) initializeMarketdataService() {
	s.marketdataSvc = mdsrv.NewService(newMarketdataProvider(s))
	s.marketdataSvc.StartCollector(
		s.marketdataRuntime,
		s.marketdataRuntime,
		s.handlePushMarketdataTick,
		mdsrv.DemandSourceFunc(s.liveWebSocketDemand),
		mdsrv.DemandSourceFunc(s.strategyRuntimeDemand),
		mdsrv.DemandSourceFunc(func() []string { return s.workflowWatchedInstruments() }),
	)
}

func (s *Server) liveWebSocketDemand() []string {
	if s.liveWebSocket == nil {
		return nil
	}
	return s.liveWebSocket.ActiveInstrumentIDs()
}

func (s *Server) strategyRuntimeDemand() []string {
	if s.strategyRuntimeManager == nil {
		return nil
	}
	return s.strategyRuntimeManager.activeInstrumentIDs()
}

func (s *Server) startAssistantWorkflowScheduler() {
	if s.assistantSvc != nil {
		s.assistantSvc.StartWorkflowScheduler(context.Background())
	}
}

func (s *Server) initializeRuntimeServices(store SidecarSettingsStore) {
	s.tradingSvc = s.newTradingService()
	s.configureDataManagement()
	s.dataManagementSvc = s.newDataManagementService()
	s.settingsSvc = settings.NewService(persistenceOnlySettingsStore(store), s.settingsServiceOptions()...)
}

func (s *Server) settingsServiceOptions() []settings.Option {
	return []settings.Option{
		settings.WithSideEffects(s.settingsSideEffects()),
		settings.WithBrokerDescriptor(func() map[string]any { return s.descriptor() }),
		settings.WithBrokerSettings(func() map[string]any { return s.brokerSettings() }),
		settings.WithOnboardingState(func(ctx context.Context) map[string]any { return s.onboardingState(ctx) }),
		settings.WithDefaultTradingEnvironment(s.defaultTradingEnvironment()),
	}
}

func (s *Server) settingsSideEffects() settings.SideEffects {
	return settings.SideEffects{
		OnIntegrationChanged: func(integration BrokerIntegration) {
			apiruntime.ApplyIntegrationEnv(integration)
			s.resetFutuRuntime()
		},
		OnExecutionChanged: func(exec ExecutionSettings) {
			if s.executionOrders != nil {
				s.executionOrders.configureSeenFillRetention(exec.SeenFillRetentionDays)
			}
		},
		OnSecurityChanged: func(sec SecuritySettings) error {
			if s.webAccessReconfigure != nil {
				return s.webAccessReconfigure(sec)
			}
			s.applySecuritySettings(sec)
			return nil
		},
		OnExchangeCalendarsChanged: func(settings ExchangeCalendarSettings) {
			if s.exchangeCalendars != nil {
				s.exchangeCalendars.NotifySettingsChanged()
			}
		},
		OnPineWorkerChanged: func(settings PineWorkerSettings) {
			s.applyPineWorkerSettings(settings)
		},
	}
}

func persistenceOnlySettingsStore(store SidecarSettingsStore) SidecarSettingsStore {
	if compatibilityStore, ok := store.(*SettingsStore); ok && compatibilityStore.Store != nil {
		return compatibilityStore.Store
	}
	return store
}

// --- Exchange / broker accessors (see also futu_runtime.go for futuExchange) ---

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
	// Ensure the Futu exchange is initialized and registered.
	s.futuExchange()
	return s.brokers.ActiveBroker()
}
