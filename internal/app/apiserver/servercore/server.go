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
	"time"

	"github.com/gin-gonic/gin"

	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/live"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/internal/settings"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
	exchangecalendarstore "github.com/jftrade/jftrade-main/internal/store/exchangecalendar"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategycatalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
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
	exchangeCalendarOperationTimeout = 75 * time.Second
	observabilityMinImportanceEnv    = "JFTRADE_OBSERVABILITY_MIN_IMPORTANCE"
)

var errFutuIntegrationNotEnabled = errors.New("futu integration is not enabled")

// Server is the API sidecar's HTTP and security shell. Domain dependencies and
// their lifecycle are exposed through the single embedded serverApplication
// entry; the remaining fields only cover transport, frontend and access
// control concerns.
type Server struct {
	serverApplication

	frontend             *frontendServer
	apiPort              int
	auth                 *webAuth
	router               *gin.Engine
	desktopMode          bool
	desktopAPIToken      string
	webAccessReconfigure func(jfsettings.SecuritySettings) error
}

// SidecarHandler is the minimal server surface required by API sidecar assembly.
type SidecarHandler interface {
	http.Handler
	WebAccessHandler() http.Handler
	Close() error
	SetAPIPort(int)
	ConfigureAuthOrigins(...string)
	SetFrontendFS(fs.FS, string)
	ApplySecuritySettings(jfsettings.SecuritySettings)
	SetWebAccessReconfigure(func(jfsettings.SecuritySettings) error)
}

// SidecarOptions customizes API sidecar assembly for embedded hosts.
type SidecarOptions struct {
	FrontendFS         fs.FS
	FrontendDevURL     string
	RuntimeAPIBaseURL  string
	StartupIntegration *jfsettings.BrokerIntegration
	NotificationSink   func(live.Event) live.NotificationDelivery
	DesktopMode        bool
	DesktopAPIToken    string
}

// SidecarSettingsStore is the settings surface required by the legacy HTTP server.
type SidecarSettingsStore interface {
	settings.Store
}

type opendProbe = futuintegration.Probe

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
	if options.StartupIntegration != nil {
		store = startupIntegrationSettingsStore{SidecarSettingsStore: store, startupIntegration: *options.StartupIntegration}
	}
	server := newServerWithFrontend(store, newFrontendServerWithOptions(options.FrontendFS, options.RuntimeAPIBaseURL, options.FrontendDevURL))
	server.runtimes.SetLiveNotificationSink(options.NotificationSink)
	server.desktopMode = options.DesktopMode
	server.desktopAPIToken = strings.TrimSpace(options.DesktopAPIToken)
	if server.auth != nil {
		server.auth.enforceAccess = !options.DesktopMode || server.desktopAPIToken != ""
	}
	server.applySecuritySettings(store.SecuritySettings())
	return server
}

type startupIntegrationSettingsStore struct {
	SidecarSettingsStore
	startupIntegration jfsettings.BrokerIntegration
}

func (s startupIntegrationSettingsStore) Integration() jfsettings.BrokerIntegration {
	if saved := s.SavedIntegration(); saved != nil {
		return *saved
	}
	return s.startupIntegration
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
func (s *Server) ApplySecuritySettings(settings jfsettings.SecuritySettings) {
	if s != nil {
		s.applySecuritySettings(settings)
	}
}

// SetWebAccessReconfigure installs the desktop lifecycle callback that owns
// the optional browser listener. Non-desktop servers keep applying settings
// directly without a separate listener.
func (s *Server) SetWebAccessReconfigure(reconfigure func(jfsettings.SecuritySettings) error) {
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

func newServerWithFrontend(store SidecarSettingsStore, frontend *frontendServer) *Server {
	bootstrap := newServerBootstrap(store)
	state := bootstrap.loadPersistentState(store)
	server := newBootstrapServer(store, frontend, bootstrap, state)
	server.initializeBootstrapState(store, bootstrap, state)
	server.registerResource("runtime consumers", server.runtimes.CloseConsumers)
	server.registerOwnedResources()
	server.router = server.buildRouter()
	return server
}

func newServerBootstrap(store SidecarSettingsStore) serverBootstrap {
	bootstrap := serverBootstrap{
		settingsPath:         store.Path(),
		backtestDBPath:       deriveBacktestDBPath(),
		unavailableDatabases: make(map[string]error),
	}
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
	backtestStore, err := backteststore.OpenKLineDatabase(b.backtestDBPath)
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseBacktest, err)
		return
	}
	if err := backtestStore.Close(); err != nil {
		log.Printf("JFTrade backtest database close failed: %v", err)
	}
}

func newBootstrapServer(store SidecarSettingsStore, frontend *frontendServer, bootstrap serverBootstrap, state serverPersistentState) *Server {
	minimumImportance := observability.NormalizeMinimumImportance(os.Getenv(observabilityMinImportanceEnv))
	observability.SetMinimumImportance(minimumImportance)
	ownedResources := state.resources
	if ownedResources == nil {
		ownedResources = &appcomposition.Resources{}
	}
	server := &Server{
		serverApplication: serverApplication{
			store:                store,
			stores:               state.stores,
			dataMigration:        bootstrap.dataMigration,
			unavailableDatabases: bootstrap.unavailableDatabases,
			lifecycle: appcomposition.NewLifecycle(
				ownedResources,
				state.resourceSetupErr,
				true,
				true,
			),
			observability: observability.NewRecorderWithConfig(observability.RecorderConfig{
				EventLimit:        20,
				SlowThreshold:     750 * time.Millisecond,
				MinimumImportance: minimumImportance,
			}),
		},
		apiPort:  portFromBind(defaultDevelopmentAPIBind, 3000),
		frontend: frontend,
		auth:     state.auth,
	}
	server.runtimes.SetLiveNotifications(live.NewReplayPublisher(), nil)
	server.runtimes.SetBrokerRegistry(broker.NewRegistry())
	server.runtimes.SetFutuCoordinator(newFutuRuntimeCoordinator(&server.serverApplication))
	server.registerResource("runtime providers", server.runtimes.CloseProviders)
	server.productFeaturesSvc = productsrv.NewService(server.runtimes.Brokers(), futuintegration.BrokerID, nil, func() {
		_ = server.activeBroker()
	})
	server.productFeaturesSvc.SetPredictionQuoteStore(
		server.stores.ExecutionOrders,
	)
	return server
}

func (s *Server) initializeSecurityAndCalendars(store SidecarSettingsStore, settingsPath string) {
	s.applySecuritySettings(store.SecuritySettings())
	manager := exchangecalendar.NewManager(
		exchangecalendarstore.New(apiruntime.DeriveExchangeCalendarDir(settingsPath)),
		func() jfsettings.ExchangeCalendarSettings {
			return persistenceOnlySettingsStore(store).ExchangeCalendarSettings()
		},
		exchangecalendar.WithAlertSink(func(alert exchangecalendar.SourceAlert) {
			s.recordExchangeCalendarAlert(alert)
		}),
	)
	previousResolver := marketpkg.SwapCalendarResolver(manager)
	s.runtimes.SetExchangeCalendars(manager, previousResolver)
	manager.Start()
}

func (s *Server) initializeADKRuntime(bootstrap serverBootstrap) {
	bootstrap.probeADKDatabase()
	bootstrap.probeADKSessionDatabase()
	if bootstrap.unavailableDatabases[datamigration.DatabaseADK] == nil &&
		bootstrap.unavailableDatabases[datamigration.DatabaseADKSession] == nil {
		assembly, err := appcomposition.OpenAssistant(appcomposition.AssistantOptions{
			SettingsPath:    bootstrap.settingsPath,
			Settings:        s.store,
			Health:          s.futuCoordinator(),
			System:          s.sysSvc,
			MarketData:      s.marketdataSvc,
			Strategy:        s.strategySvc,
			Trading:         s.tradingSvc,
			Backtest:        s.backtestSvc,
			ProductFeatures: s.productFeaturesSvc,
			Watchlist:       s.watchlistSvc,
		})
		if err != nil {
			log.Printf("JFTrade assistant runtime degraded: %v", err)
		} else {
			s.runtimes.SetAssistant(assembly)
			s.assistantSvc = assembly.Service()
		}
	}
	s.refreshUnavailableDatabaseStatuses()
}

func (b *serverBootstrap) probeADKDatabase() {
	probe := appcomposition.InspectAssistantRuntimeDatabase(b.settingsPath)
	if probe.OpenError != nil {
		b.recordUnavailable(datamigration.DatabaseADK, probe.OpenError)
		return
	}
	if probe.CloseError != nil {
		log.Printf("JFTrade ADK database close failed: %v", probe.CloseError)
	}
}

func (b *serverBootstrap) probeADKSessionDatabase() {
	probe := appcomposition.InspectAssistantSessionDatabase(b.settingsPath)
	if probe.OpenError != nil {
		b.recordUnavailable(datamigration.DatabaseADKSession, probe.OpenError)
		return
	}
	if probe.CloseError != nil {
		log.Printf("JFTrade ADK session database close failed: %v", probe.CloseError)
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

func (s *Server) initializeMarketdataRuntime() {
	coordinator := s.runtimes.FutuCoordinator()
	if coordinator == nil {
		coordinator = newFutuRuntimeCoordinator(&s.serverApplication)
		s.runtimes.SetFutuCoordinator(coordinator)
	}
	runtime := futuintegration.NewMarketDataRuntime(futuintegration.MarketDataRuntimeOptions{
		ConfigSource: func() futuintegration.MarketDataConfig {
			integration := s.store.Integration()
			return futuintegration.MarketDataConfig{
				Enabled:      integration.Enabled,
				Host:         integration.Config.Host,
				APIPort:      integration.Config.APIPort,
				WebSocketKey: integration.Config.WebSocketKey,
			}
		},
		OnBroker: func(active broker.Broker) {
			coordinator.AcceptRuntimeBroker(active)
		},
		OnSystemNotification: func(note live.Notification) {
			s.handleFutuSystemNotification(note)
		},
	})
	s.runtimes.SetMarketData(runtime)
}

func (s *Server) reconcileStrategyRuntimeStates() {
	if _, unavailable := s.unavailableDatabases[datamigration.DatabaseStrategy]; unavailable {
		return
	}
	reconciled, err := s.stores.StrategyCatalog.ReconcileOnStartup()
	if err != nil {
		log.Printf("JFTrade strategy runtime state reconciliation failed: %v", err)
		return
	}
	if reconciled > 0 {
		log.Printf("JFTrade reconciled %d stale strategy runtime state(s) to STOPPED during startup", reconciled)
	}
}

func (s *Server) startLiveNotifications() {
	if err := s.runtimes.LiveNotifications().Start(bbgoNotificationSource{}); err != nil {
		log.Printf("JFTrade BBGO notification source unavailable: %v", err)
	}
}

func (s *Server) initializeRealTradeControl(bootstrap serverBootstrap) {
	controlPlane, err := trdsrv.NewRealTradeControlPlane(deriveRealTradeControlPath(bootstrap.settingsPath))
	if err != nil {
		bootstrap.recordUnavailable("real-trade-control", err)
	}
	s.runtimes.SetRealTradeControl(controlPlane, controlPlane)
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
		system.WithBrokerDescriptor(func() map[string]any { return s.futuCoordinator().Descriptor() }),
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
		system.WithFutuOpenDHealth(func(ctx context.Context) map[string]any { return s.futuCoordinator().OpenDHealth(ctx) }),
		system.WithFutuOpenDInstallGuide(func() map[string]any { return s.futuCoordinator().OpenDInstallGuide() }),
		system.WithResetFutuRuntime(func() { s.futuCoordinator().Reset() }),
		system.WithRuntimeDependencies(func(ctx context.Context) map[string]any { return s.runtimeDependencies(ctx) }),
		system.WithRequestObservability(func() any { return s.observability.Snapshot() }),
		system.WithRealTradeRiskState(func() *trdsrv.RealTradeRiskSnapshot {
			riskGateway := s.runtimes.PreTradeRisk()
			if riskGateway == nil {
				return nil
			}
			snapshot := riskGateway.Snapshot()
			return &snapshot
		}),
	}
}

func (s *Server) initializeBacktestService(state serverPersistentState) {
	backtestRunner, instanceRunner := s.startPineWorkerManagers()
	s.runtimes.SetPineWorkerRunners(backtestRunner, instanceRunner)
	s.backtestSvc = btsrv.NewService(s.backtestServiceOptions(state, backtestRunner)...)
	s.registerResource("backtest service", func() error {
		return closeApplicationResource(s.backtestSvc)
	})
}

func (s *Server) backtestServiceOptions(state serverPersistentState, runner pineWorkerRunner) []btsrv.Option {
	opts := []btsrv.Option{
		btsrv.WithRunStore(state.stores.BacktestRuns),
		btsrv.WithSyncTaskStore(s.stores.BacktestTasks),
		btsrv.WithStrategyProvider(&strategyProviderAdapter{store: state.stores.Design}),
		btsrv.WithDBPathFn(func() string { return deriveBacktestDBPath() }),
		btsrv.WithNewKLineSyncerFn(futuintegration.NewKLineSyncer),
	}
	if runner != nil {
		opts = append(opts, btsrv.WithPineWorkerRunner(runner))
	}
	return opts
}

func (s *Server) initializeStrategyService(state serverPersistentState) {
	state.stores.StrategyCatalog.SetDefinitionStore(state.stores.Design)
	strategyRuntime := s.runtimes.StrategyRuntime()
	if strategyRuntime != nil {
		state.stores.StrategyCatalog.SetObservationSource(
			strategycatalog.ObservationSourceFunc(strategyRuntime.GetObservation),
		)
	}
	s.strategySvc = stratsrv.NewService(
		state.stores.Design,
		state.stores.StrategyCatalog,
		strategyRuntime,
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
		"metadata":         assistantassembly.StrategyMetadataPayload(analysis.Program),
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
	s.registerResource("market data service", func() error {
		return closeApplicationResource(s.marketdataSvc)
	})
	s.marketdataSvc.SetSubscriptionReconciler(s.runtimes.MarketData())
}

func (s *Server) liveWebSocketDemand() []string {
	liveWebSocket := s.runtimes.LiveWebSocket()
	if liveWebSocket == nil {
		return nil
	}
	return liveWebSocket.ActiveInstrumentIDs()
}

func (s *Server) strategyRuntimeDemand() []string {
	strategyRuntime := s.runtimes.StrategyRuntime()
	if strategyRuntime == nil {
		return nil
	}
	return strategyRuntime.ActiveInstrumentIDs()
}

func (s *Server) startAssistantWorkflowScheduler() {
	if assistantRuntime := s.runtimes.Assistant(); assistantRuntime != nil {
		assistantRuntime.StartWorkflowScheduler(context.Background())
	}
}

func (s *Server) initializeRuntimeServices(store SidecarSettingsStore) {
	s.configureDataManagement()
	s.dataManagementSvc = s.newDataManagementService()
	persistenceStore := persistenceOnlySettingsStore(store)
	s.settingsSvc = settings.NewService(persistenceStore, s.settingsServiceOptions()...)
	if mcpStore, ok := persistenceStore.(settings.MCPServerStore); ok {
		assistantRuntime := s.runtimes.Assistant()
		if assistantRuntime == nil {
			log.Printf("JFTrade local MCP server unavailable: ADK runtime is unavailable")
		} else if err := assistantRuntime.ReconfigureMCP(mcpStore.MCPServerSettings()); err != nil {
			log.Printf("JFTrade local MCP server unavailable: %v", err)
		}
	} else {
		log.Printf("JFTrade local MCP server settings unavailable")
	}
	marketDataRuntime := s.runtimes.MarketData()
	s.marketdataSvc.StartCollector(
		marketDataRuntime,
		marketDataRuntime,
		s.handlePushMarketdataTick,
		mdsrv.DemandSourceFunc(s.liveWebSocketDemand),
		mdsrv.DemandSourceFunc(func() []string { return s.workflowWatchedInstruments() }),
	)
}

func (s *Server) settingsServiceOptions() []settings.Option {
	return []settings.Option{
		settings.WithSideEffects(s.settingsSideEffects()),
		settings.WithBrokerDescriptor(func() map[string]any { return s.futuCoordinator().Descriptor() }),
		settings.WithBrokerSettings(func() map[string]any { return s.futuCoordinator().BrokerSettings() }),
		settings.WithOnboardingState(func(ctx context.Context) map[string]any { return s.futuCoordinator().OnboardingState(ctx) }),
		settings.WithDefaultTradingEnvironment(s.defaultTradingEnvironment()),
		settings.WithMCPServerStatus(func() jfsettings.MCPServerStatus {
			assistantRuntime := s.runtimes.Assistant()
			if assistantRuntime == nil {
				return jfsettings.MCPServerStatus{}
			}
			return assistantRuntime.MCPStatus()
		}),
		settings.WithSystemNotificationTester(func() (*live.Event, live.NotificationDelivery) {
			return s.recordLiveNotificationWithDelivery(live.Notification{
				Level:    "warn",
				Title:    "JFTrade 系统通知测试",
				Message:  "系统通知通道已连接。",
				Source:   "desktop",
				Category: "system.notification.test",
			})
		}),
	}
}

func (s *Server) settingsSideEffects() settings.SideEffects {
	return settings.SideEffects{
		OnIntegrationChanged: func(_ jfsettings.BrokerIntegration) {
			s.futuCoordinator().Reset()
		},
		OnExecutionChanged: func(exec jfsettings.ExecutionSettings) {
			if s.stores.ExecutionOrders != nil {
				s.stores.ExecutionOrders.ConfigureSeenFillRetention(exec.SeenFillRetentionDays)
			}
		},
		OnSecurityChanged: func(sec jfsettings.SecuritySettings) error {
			if s.webAccessReconfigure != nil {
				return s.webAccessReconfigure(sec)
			}
			s.applySecuritySettings(sec)
			return nil
		},
		OnExchangeCalendarsChanged: func(settings jfsettings.ExchangeCalendarSettings) {
			if calendars := s.runtimes.ExchangeCalendars(); calendars != nil {
				calendars.NotifySettingsChanged()
			}
		},
		OnPineWorkerChanged: func(settings jfsettings.PineWorkerSettings) {
			s.applyPineWorkerSettings(settings)
		},
		OnMCPServerChanged: func(settings jfsettings.MCPServerSettings) error {
			assistantRuntime := s.runtimes.Assistant()
			if assistantRuntime == nil {
				return errors.New("MCP server manager is unavailable")
			}
			return assistantRuntime.ReconfigureMCP(settings)
		},
	}
}
