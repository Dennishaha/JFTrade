package servercore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	asst "github.com/jftrade/jftrade-main/internal/assistant"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/live"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/settings"
	exchangecalendarstore "github.com/jftrade/jftrade-main/internal/store/exchangecalendar"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
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
)

type envelope = httpserver.Envelope
type apiError = httpserver.APIError

var errFutuIntegrationNotEnabled = errors.New("futu integration is not enabled")

func exchangeCalendarOperationContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), exchangeCalendarOperationTimeout)
}

type Server struct {
	store                    SidecarSettingsStore
	strategyStore            *strategyCatalogStore
	strategyRuntimeStore     *strategyRuntimeStore
	strategyRuntimeManager   *strategyRuntimeManager
	designStore              *strategyDesignStore
	backtestRuns             *backtestRunStore
	backtestSyncTasks        *backtestSyncTaskStore
	executionOrders          *executionOrderStore
	liveWebSocket            *apilive.Handler
	liveNotifications        *live.ReplayPublisher
	closeOnce                sync.Once
	closeErr                 error
	marketdataRuntime        *futuintegration.MarketDataRuntime
	brokers                  *broker.Registry // Unified broker registry for multi-broker support
	adkRuntime               *jfadk.Runtime
	assistantSvc             *asst.Service
	frontend                 *frontendServer
	apiPort                  int
	auth                     *adminAuth
	router                   *gin.Engine
	exchangeCalendars        *exchangecalendar.Manager
	previousCalendarResolver marketcalendar.Resolver
	sysSvc                   *system.Service
	settingsSvc              *settings.Service
	dataMigration            *datamigration.Manager
	unavailableDatabases     map[string]error
	backtestSvc              *btsrv.Service
	strategySvc              *stratsrv.Service
	marketdataSvc            *mdsrv.Service
	tradingSvc               *trdsrv.Service
	backtestPineWorkerRunner pineWorkerRunner
	instancePineWorkerRunner pineWorkerRunner
}

// SidecarHandler is the minimal server surface required by API sidecar assembly.
type SidecarHandler interface {
	http.Handler
	Close() error
	SetAPIPort(int)
	ConfigureAuthOrigins(...string)
	SetFrontendFS(fs.FS, string)
	ApplySecuritySettings(SecuritySettings)
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

// StartForRunArgs, resolveGUIAPIBaseURL, resolveGUIRuntimeAPIBaseURL,
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
	return newServerWithFrontend(store, newFrontendServerWithRuntimeConfig(frontendFS, runtimeAPIBaseURL))
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
	}
}

// ApplySecuritySettings applies administrator auth settings to API and frontend.
func (s *Server) ApplySecuritySettings(settings SecuritySettings) {
	if s != nil {
		s.applySecuritySettings(settings)
	}
}

func newServerWithFrontend(store SidecarSettingsStore, frontend *frontendServer) *Server {
	settingsPath := store.Path()
	backtestDBPath := deriveBacktestDBPath()
	dataMigration := datamigration.NewManager(settingsPath, backtestDBPath)
	unavailableDatabases := make(map[string]error)
	recordUnavailable := func(id string, err error) {
		if err == nil {
			return
		}
		unavailableDatabases[id] = err
		dataMigration.SetUnavailable(id, err)
		log.Printf("JFTrade %s database unavailable: %v", id, err)
	}
	if err := ensureRuntimeLayout(settingsPath, backtestDBPath); err != nil {
		log.Printf("JFTrade runtime layout unavailable: %v", err)
	}
	if backtestStore, err := bt.NewFutuKLineStore(backtestDBPath); err != nil {
		recordUnavailable(datamigration.DatabaseBacktest, err)
	} else if err := backtestStore.Close(); err != nil {
		log.Printf("JFTrade backtest database close failed: %v", err)
	}
	strategyStore, err := NewStrategyCatalogStore(deriveStrategyCatalogPath(settingsPath), deriveStrategyPluginTargetDir(settingsPath))
	if err != nil {
		recordUnavailable(datamigration.DatabaseStrategy, err)
	}
	var runtimeStore *strategyRuntimeStore
	if strategyStore != nil {
		runtimeStore = strategyStore.runtimeStore
	}
	if strategyStore == nil {
		strategyStore = &strategyCatalogStore{path: deriveStrategyCatalogPath(settingsPath), dbPath: deriveStrategyCatalogDBPath(deriveStrategyCatalogPath(settingsPath)), targetDir: deriveStrategyPluginTargetDir(settingsPath), data: strategyCatalogFile{TargetDir: deriveStrategyPluginTargetDir(settingsPath)}}
	}
	designStore, err := NewStrategyDesignStore(deriveStrategyDesignPath(settingsPath))
	if err != nil {
		recordUnavailable(datamigration.DatabaseStrategy, err)
		designStore = &strategyDesignStore{path: deriveStrategyDesignPath(settingsPath), dbPath: deriveStrategyDesignDBPath(deriveStrategyDesignPath(settingsPath))}
	}
	backtestRunStore, err := newBacktestRunStoreWithDB(deriveBacktestRunDBPath(settingsPath))
	if err != nil {
		recordUnavailable(datamigration.DatabaseBacktestRuns, err)
		backtestRunStore = newBacktestRunStore()
	}
	executionOrderStore, err := newExecutionOrderStoreWithDB(deriveExecutionOrderDBPath(settingsPath))
	if err != nil {
		recordUnavailable(datamigration.DatabaseExecution, err)
		executionOrderStore = newExecutionOrderStore()
	}
	executionOrderStore.configureSeenFillRetention(store.ExecutionSettings().SeenFillRetentionDays)
	auth, authErr := newAdminAuth(settingsPath)
	if authErr != nil {
		log.Printf("JFTrade administrator authentication unavailable: %v", authErr)
		auth = &adminAuth{
			enabled:        true,
			unavailable:    true,
			allowedOrigins: map[string]struct{}{},
			sessions:       map[string]adminSession{},
			attempts:       map[string]loginAttempt{},
			now:            time.Now,
		}
	}
	server := &Server{
		store:                store,
		strategyStore:        strategyStore,
		strategyRuntimeStore: runtimeStore,
		designStore:          designStore,
		backtestRuns:         backtestRunStore,
		backtestSyncTasks:    newBacktestSyncTaskStore(),
		executionOrders:      executionOrderStore,
		liveNotifications:    live.NewReplayPublisher(),
		brokers:              broker.NewRegistry(),
		apiPort:              portFromBind(defaultDevelopmentAPIBind, 3000),
		frontend:             frontend,
		auth:                 auth,
		unavailableDatabases: unavailableDatabases,
	}
	server.dataMigration = dataMigration
	server.liveWebSocket = apilive.NewHandler(liveWebSocketBackend{server: server}, apilive.Options{
		DataInterval:            liveTickDispatchInterval,
		SecurityDetailsInterval: marketSecurityDetailsStreamInterval,
		DepthRefreshInterval:    marketDepthStreamRefreshInterval,
	})
	if server.auth != nil {
		server.auth.configureOrigins(
			apiBaseURLForBind(defaultDevelopmentAPIBind),
			apiBaseURLForBind(defaultReleaseAPIBind),
			"http://"+defaultReleaseGUIBind,
			"http://127.0.0.1:5173",
			"http://127.0.0.1:5174",
			"http://localhost:5173",
			"http://localhost:5174",
		)
		log.Printf("JFTrade administrator key file: %s", server.auth.keyPath)
	}
	server.applySecuritySettings(store.SecuritySettings())
	server.exchangeCalendars = exchangecalendar.NewManager(
		exchangecalendarstore.New(apiruntime.DeriveExchangeCalendarDir(settingsPath)),
		func() ExchangeCalendarSettings {
			return persistenceOnlySettingsStore(store).ExchangeCalendarSettings()
		},
		exchangecalendar.WithAlertSink(func(alert exchangecalendar.SourceAlert) {
			server.recordExchangeCalendarAlert(alert)
		}),
	)
	server.previousCalendarResolver = marketpkg.SwapCalendarResolver(server.exchangeCalendars)
	server.exchangeCalendars.Start()
	if adkStore, err := jfadk.NewStore(
		apiruntime.DeriveADKDBPath(settingsPath),
		apiruntime.DeriveADKSecretsPath(settingsPath),
		apiruntime.DeriveADKSkillsDir(settingsPath),
	); err != nil {
		recordUnavailable(datamigration.DatabaseADK, err)
	} else if err := adkStore.Close(); err != nil {
		log.Printf("JFTrade ADK database close failed: %v", err)
	}
	if sessionService, err := jfadk.NewSQLiteSessionService(apiruntime.DeriveADKSessionDBPath(settingsPath)); err != nil {
		recordUnavailable(datamigration.DatabaseADKSession, err)
	} else if err := jfadk.CloseSessionService(sessionService); err != nil {
		log.Printf("JFTrade ADK session database close failed: %v", err)
	}
	if unavailableDatabases[datamigration.DatabaseADK] == nil &&
		unavailableDatabases[datamigration.DatabaseADKSession] == nil {
		server.adkRuntime = newADKRuntime(server, settingsPath)
	}
	if statuses, statusErr := server.dataMigration.Statuses(context.Background()); statusErr != nil {
		log.Printf("JFTrade database status inspection failed: %v", statusErr)
	} else {
		for _, status := range statuses {
			if status.Status == "ready" {
				continue
			}
			reason := status.Error
			if strings.TrimSpace(reason) == "" {
				reason = "database was not initialized"
			}
			err := fmt.Errorf("%s", reason)
			server.unavailableDatabases[status.ID] = err
		}
	}
	server.assistantSvc = asst.NewService(
		server.adkRuntime,
		asst.WithRuntimeSettings(func() any {
			return server.store.ADKSettings()
		}),
		asst.WithStreamIdleTimeout(func() int {
			return server.store.ADKSettings().StreamIdleTimeoutMs
		}),
		asst.WithOptimizationRuns(assistantOptimizationRuns{server: server}),
	)
	server.strategyRuntimeManager = newStrategyRuntimeManager(server)
	server.marketdataRuntime = futuintegration.NewMarketDataRuntime(futuintegration.MarketDataRuntimeOptions{
		ConfigSource: func() futuintegration.MarketDataConfig {
			integration := server.store.SavedIntegration()
			if integration == nil {
				return futuintegration.MarketDataConfig{}
			}
			return futuintegration.MarketDataConfig{
				Enabled: integration.Enabled, Host: integration.Config.Host,
				APIPort: integration.Config.APIPort, WebSocketKey: integration.Config.WebSocketKey,
			}
		},
		OnExchange: func(exchange *futu.Exchange) {
			exchange.OnSystemNotify(server.handleFutuSystemNotify)
			if server.brokers != nil {
				server.brokers.Replace(futu.NewBrokerAdapter(exchange))
			}
		},
	})
	if _, unavailable := unavailableDatabases[datamigration.DatabaseStrategy]; unavailable {
		// The settings and migration endpoints remain available while strategy data is incompatible.
	} else if reconciled, err := server.strategyStore.reconcileRuntimeStatesOnStartup(); err != nil {
		log.Printf("JFTrade strategy runtime state reconciliation failed: %v", err)
	} else if reconciled > 0 {
		log.Printf("JFTrade reconciled %d stale strategy runtime state(s) to STOPPED during startup", reconciled)
	}
	if err := server.liveNotifications.Start(bbgoNotificationSource{}); err != nil {
		log.Printf("JFTrade BBGO notification source unavailable: %v", err)
	}

	// Wire system service — delegates to Server methods via closures.
	server.sysSvc = system.NewService(
		system.WithAPIPortFunc(func() int { return server.apiPort }),
		system.WithSettingsPath(settingsPath),
		system.WithDefaultTradingEnvironmentFunc(func() string { return server.defaultTradingEnvironment() }),
		system.WithBrokerDescriptor(func() map[string]any { return server.descriptor() }),
		system.WithStrategyRuntimeSummary(func() map[string]any { return server.strategyRuntimeSummary() }),
		system.WithLiveStats(func() map[string]any { return server.liveStatsSummary() }),
		system.WithMarketdataRuntimeSummary(func() map[string]any { return server.marketdataRuntimeSummary() }),
		system.WithBrokerOrderSnapshot(func() map[string]any {
			if server.tradingSvc == nil {
				return map[string]any{}
			}
			return server.tradingSvc.OrderUpdatesSnapshot()
		}),
		system.WithExchangeCalendarStatus(func() map[string]any {
			if server.exchangeCalendars == nil {
				return map[string]any{}
			}
			return server.exchangeCalendars.Status()
		}),
		system.WithExchangeCalendarSources(func() []map[string]any {
			if server.exchangeCalendars == nil {
				return nil
			}
			return server.exchangeCalendars.Sources()
		}),
		system.WithRefreshExchangeCalendars(func(ctx context.Context, market string) map[string]any {
			if server.exchangeCalendars == nil {
				return map[string]any{"accepted": false}
			}
			operationCtx, cancel := exchangeCalendarOperationContext(ctx)
			defer cancel()
			if strings.TrimSpace(market) == "" {
				return server.exchangeCalendars.RefreshAll(operationCtx)
			}
			return server.exchangeCalendars.RefreshMarket(operationCtx, market)
		}),
		system.WithProbeExchangeCalendars(func(ctx context.Context, market string) map[string]any {
			if server.exchangeCalendars == nil {
				return map[string]any{"accepted": false}
			}
			operationCtx, cancel := exchangeCalendarOperationContext(ctx)
			defer cancel()
			if strings.TrimSpace(market) == "" {
				return server.exchangeCalendars.ProbeAll(operationCtx)
			}
			return server.exchangeCalendars.ProbeMarket(operationCtx, market)
		}),
		system.WithFutuOpenDHealth(func(ctx context.Context) map[string]any { return server.futuOpenDHealth(ctx) }),
		system.WithFutuOpenDInstallGuide(func() map[string]any { return server.futuOpenDInstallGuide() }),
		system.WithResetFutuRuntime(func() { server.resetFutuRuntime() }),
	)

	// Wire backtest service — RunStore / SyncTaskStore / StrategyProvider
	// are implemented by the same backing stores already held by Server.
	backtestPineWorkerRunner, instancePineWorkerRunner := server.startPineWorkerManagers()
	server.backtestPineWorkerRunner = backtestPineWorkerRunner
	server.instancePineWorkerRunner = instancePineWorkerRunner
	if instancePineWorkerRunner != nil && server.strategyRuntimeManager != nil {
		server.strategyRuntimeManager.pineWorkerRunner = instancePineWorkerRunner
	}
	backtestOptions := []btsrv.Option{
		btsrv.WithRunStore(&backtestRunStoreAdapter{store: backtestRunStore}),
		btsrv.WithSyncTaskStore(&backtestSyncTaskStoreAdapter{store: server.backtestSyncTasks}),
		btsrv.WithStrategyProvider(&strategyProviderAdapter{store: designStore}),
		btsrv.WithDBPathFn(func() string { return deriveBacktestDBPath() }),
		btsrv.WithNewKLineSyncerFn(futuintegration.NewKLineSyncer),
	}
	if backtestPineWorkerRunner != nil {
		backtestOptions = append(backtestOptions, btsrv.WithPineWorkerRunner(backtestPineWorkerRunner))
	}
	server.backtestSvc = btsrv.NewService(backtestOptions...)

	// Wire strategy service
	server.strategySvc = stratsrv.NewService(
		&strategyDesignStoreAdapter{store: designStore},
		&strategyCatalogStoreAdapter{store: strategyStore, designStore: designStore, runtimeMgr: server.strategyRuntimeManager},
		&strategyRuntimeManagerAdapter{mgr: server.strategyRuntimeManager},
		stratsrv.WithPineAnalyzer(func(input stratsrv.PineAnalyzeInput) (stratsrv.PineAnalysisResult, error) {
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
		}),
		stratsrv.WithLiveMarketStreamRefresher(func(ctx context.Context) {
			server.ensureLiveMarketStream(ctx, server.activeLiveStreamInstrumentIDs(nil))
		}),
	)

	// Wire marketdata service — delegates to Server methods via closure-based Provider.
	server.marketdataSvc = mdsrv.NewService(newMarketdataProvider(server))
	server.marketdataSvc.StartCollector(
		server.marketdataRuntime,
		server.marketdataRuntime,
		server.handlePushMarketdataTick,
		mdsrv.DemandSourceFunc(func() []string {
			if server.liveWebSocket == nil {
				return nil
			}
			return server.liveWebSocket.ActiveInstrumentIDs()
		}),
		mdsrv.DemandSourceFunc(func() []string {
			if server.strategyRuntimeManager == nil {
				return nil
			}
			return server.strategyRuntimeManager.activeInstrumentIDs()
		}),
	)

	// Wire trading service to broker capabilities and the application-owned execution store.
	server.tradingSvc = server.newTradingService()

	// Wire settings service — delegates to SettingsStore with side-effect orchestration.
	server.settingsSvc = settings.NewService(persistenceOnlySettingsStore(store),
		settings.WithSideEffects(settings.SideEffects{
			OnIntegrationChanged: func(integration BrokerIntegration) {
				apiruntime.ApplyIntegrationEnv(integration)
				server.resetFutuRuntime()
			},
			OnExecutionChanged: func(exec ExecutionSettings) {
				if server.executionOrders != nil {
					server.executionOrders.configureSeenFillRetention(exec.SeenFillRetentionDays)
				}
			},
			OnSecurityChanged: func(sec SecuritySettings) {
				server.applySecuritySettings(sec)
			},
			OnExchangeCalendarsChanged: func(settings ExchangeCalendarSettings) {
				if server.exchangeCalendars != nil {
					server.exchangeCalendars.NotifySettingsChanged()
				}
			},
			OnPineWorkerChanged: func(settings PineWorkerSettings) {
				server.applyPineWorkerSettings(settings)
			},
		}),
		settings.WithBrokerDescriptor(func() map[string]any { return server.descriptor() }),
		settings.WithBrokerSettings(func() map[string]any { return server.brokerSettings() }),
		settings.WithOnboardingState(func(ctx context.Context) map[string]any { return server.onboardingState(ctx) }),
		settings.WithDefaultTradingEnvironment(server.defaultTradingEnvironment()),
		settings.WithDataMigration(
			func(ctx context.Context) (any, error) {
				statuses, err := server.dataMigration.Statuses(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"databases": statuses}, nil
			},
			func(ctx context.Context, raw any) (any, error) {
				payload, ok := raw.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid database rebuild payload")
				}
				request := datamigration.RebuildRequest{
					Mode:         strings.TrimSpace(fmt.Sprint(payload["mode"])),
					Confirmation: strings.TrimSpace(fmt.Sprint(payload["confirmation"])),
				}
				if values, ok := payload["databaseIds"].([]any); ok {
					for _, value := range values {
						request.DatabaseIDs = append(request.DatabaseIDs, strings.TrimSpace(fmt.Sprint(value)))
					}
				}
				if value, ok := payload["databaseId"].(string); ok && strings.TrimSpace(value) != "" {
					request.DatabaseIDs = append(request.DatabaseIDs, strings.TrimSpace(value))
				}
				return server.dataMigration.ScheduleRebuild(ctx, request)
			},
		),
	)

	server.router = server.buildRouter()
	return server
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

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.router == nil {
		http.NotFound(w, r)
		return
	}
	s.router.ServeHTTP(w, r)
}

var _ middleware.WriteMethodDetector = (*Server)(nil)

func (s *Server) IsWriteMethod(r *http.Request) bool {
	return r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete
}

func (s *Server) writeError(c *gin.Context, status int, code string, message string) {
	httpserver.WriteError(c, status, code, message)
}

// Close releases all resources held by the server, including database connections.
// It is safe to call Close multiple times. After Close, the server should not be used.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		var errs []error
		if s.tradingSvc != nil {
			if err := s.tradingSvc.StopOrderUpdates(); err != nil {
				errs = append(errs, fmt.Errorf("trading order updates close: %w", err))
			}
		}
		if s.liveWebSocket != nil {
			if err := s.liveWebSocket.Close(); err != nil {
				errs = append(errs, fmt.Errorf("liveWebSocket close: %w", err))
			}
		}
		if s.marketdataSvc != nil {
			if err := s.marketdataSvc.Close(); err != nil {
				errs = append(errs, fmt.Errorf("marketdata close: %w", err))
			}
		}
		if s.liveNotifications != nil {
			if err := s.liveNotifications.Close(); err != nil {
				errs = append(errs, fmt.Errorf("liveNotifications close: %w", err))
			}
		}
		if s.backtestSvc != nil {
			if err := s.backtestSvc.Close(); err != nil {
				errs = append(errs, fmt.Errorf("backtestSvc close: %w", err))
			}
		}
		closePineWorkerRunner := func(name string, runner pineWorkerRunner) {
			if runner == nil {
				return
			}
			if closer, ok := runner.(interface{ Close(context.Context) error }); ok {
				if err := closer.Close(context.Background()); err != nil {
					errs = append(errs, fmt.Errorf("%s close: %w", name, err))
				}
			}
		}
		closePineWorkerRunner("backtestPineWorkerRunner", s.backtestPineWorkerRunner)
		closePineWorkerRunner("instancePineWorkerRunner", s.instancePineWorkerRunner)
		if s.backtestRuns != nil {
			if err := s.backtestRuns.Close(); err != nil {
				errs = append(errs, fmt.Errorf("backtestRuns close: %w", err))
			}
		}
		if s.executionOrders != nil {
			if err := s.executionOrders.Close(); err != nil {
				errs = append(errs, fmt.Errorf("executionOrders close: %w", err))
			}
		}
		if s.strategyStore != nil {
			if err := s.strategyStore.Close(); err != nil {
				errs = append(errs, fmt.Errorf("strategyStore close: %w", err))
			}
		}
		if s.designStore != nil {
			if err := s.designStore.Close(); err != nil {
				errs = append(errs, fmt.Errorf("designStore close: %w", err))
			}
		}
		if s.assistantSvc != nil {
			if err := s.assistantSvc.Close(); err != nil {
				errs = append(errs, fmt.Errorf("assistantSvc close: %w", err))
			}
		}
		if s.marketdataRuntime != nil {
			if err := s.marketdataRuntime.Close(); err != nil {
				errs = append(errs, fmt.Errorf("futu marketdata runtime close: %w", err))
			}
		}
		if s.exchangeCalendars != nil {
			if err := s.exchangeCalendars.Close(); err != nil {
				errs = append(errs, fmt.Errorf("exchange calendar manager close: %w", err))
			}
		}
		if s.previousCalendarResolver != nil {
			marketpkg.SetCalendarResolver(s.previousCalendarResolver)
		} else {
			marketpkg.ResetCalendarResolver()
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}
