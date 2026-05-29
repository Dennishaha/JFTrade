package jftradeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/internal/buildinfo"
	"github.com/jftrade/jftrade-main/pkg/futu"
)

const (
	defaultFutuHost            = "127.0.0.1"
	defaultFutuAPIPort         = 11110
	defaultFutuWebSocketPort   = 11111
	defaultMaxWebSocketClients = 20
	strategyListLogsTailSize   = 20
)

type envelope struct {
	OK        bool      `json:"ok"`
	Data      any       `json:"data,omitempty"`
	Error     *apiError `json:"error,omitempty"`
	Timestamp string    `json:"timestamp"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Server struct {
	store                  *SettingsStore
	strategyStore          *strategyCatalogStore
	strategyRuntimeStore   *strategyRuntimeStore
	strategyRuntimeManager *strategyRuntimeManager
	designStore            *strategyDesignStore
	backtestRuns           *backtestRunStore
	backtestSyncTasks      *backtestSyncTaskStore
	executionOrders        *executionOrderStore
	brokerOrderUpdates     *brokerOrderUpdateWorker
	upgrader               websocket.Upgrader
	marketSubscriptions    marketSubscriptionManager
	tickCache              tickSampleCacheManager
	liveSockets            liveSocketPool
	liveNotifications      liveNotificationCache
	liveQuoteState         liveQuoteRefreshState
	liveStreamState        liveMarketStreamState
	exchangeMu             sync.Mutex
	exchange               *futu.Exchange
	exchangeConfigKey      string
	frontend               *frontendServer
	apiPort                int
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

func StartForRunArgs(ctx context.Context, args []string) (func(context.Context) error, error) {
	if !shouldStartForArgs(args) {
		return func(context.Context) error { return nil }, nil
	}

	frontendFS := loadFrontendFS()
	defaults := resolveLaunchDefaults(frontendFS != nil)
	settingsPath := envOrDefault("JFTRADE_SETTINGS_PATH", defaults.settingsPath)
	backtestDBPath := envOrDefault("JFTRADE_BACKTEST_DB", defaults.backtestDBPath)
	if err := ensureRuntimeLayout(settingsPath, backtestDBPath); err != nil {
		return nil, err
	}
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		return nil, err
	}
	if err := store.ensureBootstrapFile(defaults); err != nil {
		return nil, err
	}
	store.applyRuntimeEnv()
	interfaceSettings := store.interfaceSettings(defaults)

	apiBind := envOrDefault("JFTRADE_API_BIND", interfaceSettings.APIBind)
	apiHandler := newServerWithFrontend(store, nil)
	apiHandler.apiPort = portFromBind(apiBind, portFromBind(defaults.apiBind, 3000))
	apiServer := &http.Server{
		Addr:              apiBind,
		Handler:           apiHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	servers := []*http.Server{apiServer}

	go func() {
		log.Printf("JFTrade API listening on http://%s", apiBind)
		if err := apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("JFTrade API server stopped: %v", err)
		}
	}()

	if frontendFS != nil {
		guiBind := envOrDefault("JFTRADE_GUI_BIND", interfaceSettings.GUIBind)
		guiAPIBaseURL := resolveGUIAPIBaseURL(interfaceSettings, apiBind)
		guiServer := &http.Server{
			Addr:              guiBind,
			Handler:           newFrontendServerWithRuntimeConfig(frontendFS, guiAPIBaseURL),
			ReadHeaderTimeout: 5 * time.Second,
		}
		servers = append(servers, guiServer)

		go func() {
			fmt.Printf("JFTrade 交互界面已启动，请访问 http://%s\n\n", guiBind)
			log.Printf("JFTrade GUI listening on http://%s", guiBind)
			if err := guiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("JFTrade GUI server stopped: %v", err)
			}
		}()
	}

	var shutdownOnce sync.Once
	shutdownAll := func(shutdownCtx context.Context) error {
		var shutdownErr error
		shutdownOnce.Do(func() {
			for _, server := range servers {
				if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && shutdownErr == nil {
					shutdownErr = err
				}
			}
		})
		return shutdownErr
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownAll(shutdownCtx)
	}()

	return shutdownAll, nil
}

func resolveGUIAPIBaseURL(interfaceSettings InterfaceSettings, apiBind string) string {
	envValue := strings.TrimSpace(os.Getenv("JFTRADE_GUI_API_BASE_URL"))
	if envValue != "" {
		return envValue
	}

	configuredValue := strings.TrimSpace(interfaceSettings.GUIAPIBaseURL)
	defaultConfiguredValue := apiBaseURLForBind(interfaceSettings.APIBind)
	if configuredValue == "" || configuredValue == defaultConfiguredValue {
		return apiBaseURLForBind(apiBind)
	}
	return configuredValue
}

func NewServer(store *SettingsStore) *Server {
	return newServerWithFrontend(store, newFrontendServer(loadFrontendFS()))
}

func newServerWithFrontend(store *SettingsStore, frontend *frontendServer) *Server {
	strategyStore, err := NewStrategyCatalogStore(deriveStrategyCatalogPath(store.path), deriveStrategyPluginTargetDir(store.path))
	if err != nil {
		log.Printf("JFTrade strategy catalog store degraded: %v", err)
		fallbackSettingsPath := filepath.Join(os.TempDir(), "jftrade-strategy-catalog-fallback", "settings.json")
		strategyStore, err = NewStrategyCatalogStore(deriveStrategyCatalogPath(fallbackSettingsPath), deriveStrategyPluginTargetDir(fallbackSettingsPath))
		if err != nil {
			log.Printf("JFTrade strategy catalog fallback sqlite store degraded: %v", err)
		}
	}
	var runtimeStore *strategyRuntimeStore
	if strategyStore != nil {
		runtimeStore = strategyStore.runtimeStore
	}
	if strategyStore == nil {
		var runtimeDB *sqlx.DB
		runtimeStore, err = NewStrategyRuntimeStore(deriveStrategyRuntimeDBPath(store.path))
		if err != nil {
			log.Printf("JFTrade strategy runtime sqlite store degraded: %v", err)
			runtimeStore = nil
		}
		if runtimeStore != nil {
			runtimeDB = runtimeStore.DB()
		}
		strategyStore = &strategyCatalogStore{path: deriveStrategyCatalogPath(store.path), dbPath: deriveStrategyCatalogDBPath(deriveStrategyCatalogPath(store.path)), db: runtimeDB, targetDir: deriveStrategyPluginTargetDir(store.path), runtimeStore: runtimeStore, data: strategyCatalogFile{TargetDir: deriveStrategyPluginTargetDir(store.path)}}
		if strategyStore.db != nil {
			if migrateErr := strategyStore.migrateLocked(); migrateErr != nil {
				log.Printf("JFTrade strategy catalog fallback sqlite store degraded: %v", migrateErr)
			}
		}
	}
	designStore, err := NewStrategyDesignStore(deriveStrategyDesignPath(store.path))
	if err != nil {
		log.Printf("JFTrade strategy design store degraded: %v", err)
		fallbackSettingsPath := filepath.Join(os.TempDir(), "jftrade-strategy-design-fallback", "settings.json")
		designStore, err = NewStrategyDesignStore(deriveStrategyDesignPath(fallbackSettingsPath))
		if err != nil {
			log.Printf("JFTrade strategy design fallback sqlite store degraded: %v", err)
			designStore = nil
		}
	}
	server := &Server{
		store:                store,
		strategyStore:        strategyStore,
		strategyRuntimeStore: runtimeStore,
		designStore:          designStore,
		backtestRuns:         newBacktestRunStore(),
		backtestSyncTasks:    newBacktestSyncTaskStore(),
		executionOrders:      newExecutionOrderStore(),
		brokerOrderUpdates:   newBrokerOrderUpdateWorker(),
		marketSubscriptions:  newMarketSubscriptionManager(),
		tickCache:            newTickSampleCacheManager(),
		apiPort:              portFromBind(defaultDevelopmentAPIBind, 3000),
		frontend:             frontend,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
	server.strategyRuntimeManager = newStrategyRuntimeManager(server)
	if reconciled, err := server.strategyStore.reconcileRuntimeStatesOnStartup(); err != nil {
		log.Printf("JFTrade strategy runtime state reconciliation failed: %v", err)
	} else if reconciled > 0 {
		log.Printf("JFTrade reconciled %d stale strategy runtime state(s) to STOPPED during startup", reconciled)
	}
	registerBBGONotificationSink(server.recordLiveNotification)
	return server
}

func shouldStartForArgs(args []string) bool {
	if strings.EqualFold(os.Getenv("JFTRADE_API_DISABLED"), "1") || strings.EqualFold(os.Getenv("JFTRADE_API_DISABLED"), "true") {
		return false
	}
	for _, arg := range args {
		if arg == "run" || arg == "api" || arg == "serve-api" {
			return true
		}
		if arg == "help" || arg == "--help" || arg == "-h" {
			return false
		}
	}
	return false
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func (s *Server) liveMarketExchange() bbgotypes.Exchange {
	if s.strategyRuntimeManager != nil && s.strategyRuntimeManager.exchangeProvider != nil {
		if exchange := s.strategyRuntimeManager.exchangeProvider(); exchange != nil {
			return exchange
		}
	}
	return s.futuExchange()
}

func (s *Server) brokerExecutionExchange() strategyRuntimeExchange {
	if s.strategyRuntimeManager != nil && s.strategyRuntimeManager.exchangeProvider != nil {
		if exchange := s.strategyRuntimeManager.exchangeProvider(); exchange != nil {
			return exchange
		}
	}
	return s.futuExchange()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.writeCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch {
	case r.URL.Path == "/swagger" || strings.HasPrefix(r.URL.Path, "/swagger/"):
		s.handleSwaggerUI(w, r)
	case r.URL.Path == "/openapi.json":
		s.handleOpenAPISpec(w, r)
	case s.serveMarketRoutes(w, r):
	case s.serveSettingsRoutes(w, r):
	case s.serveSystemRoutes(w, r):
	case s.servePluginRoutes(w, r):
	case s.serveStrategyRoutes(w, r):
	case s.serveBacktestRoutes(w, r):
	case s.serveBrokerRoutes(w, r):
	case s.servePortfolioRoutes(w, r):
	case s.serveExecutionRoutes(w, r):
	case s.frontend != nil && s.frontend.serveRequest(w, r):
	default:
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("unknown endpoint %s", r.URL.Path))
	}
}

func (s *Server) writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
}

func (s *Server) writeOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(envelope{OK: true, Data: data, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
}

func (s *Server) writeError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{OK: false, Error: &apiError{Code: code, Message: message}, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
}

func (s *Server) systemStatus() map[string]any {
	return map[string]any{
		"name":                      "JFTrade",
		"apiPort":                   s.apiPort,
		"defaultBroker":             "futu",
		"defaultTradingEnvironment": "SIMULATE",
		"realTradingEnabled":        false,
		"realTradingKillSwitch": map[string]any{
			"active": false, "envConfiguredActive": false, "controlPlaneActive": false,
			"blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true,
		},
		"realTradingRisk": map[string]any{
			"enabled": false, "maxOrderQuantity": nil, "maxOrderNotional": nil,
			"envConfiguredMaxOrderQuantity": nil, "envConfiguredMaxOrderNotional": nil,
			"controlPlaneActive": false, "controlPlaneMaxOrderQuantity": nil, "controlPlaneMaxOrderNotional": nil,
			"riskConfigSource": nil,
		},
		"realTradeAccess": map[string]any{"approverAllowlistEnabled": false, "approverCount": 0, "adminAllowlistEnabled": false, "adminCount": 0},
		"broker":          s.descriptor(),
		"build":           buildinfo.Snapshot(),
		"persistence": map[string]any{
			"engine": "json", "databasePath": s.store.path, "status": "ok", "migrated": true,
			"pendingMigrations": []any{}, "tables": []string{"broker_integrations", "broker_accounts"}, "checkedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
		"strategyRuntime": s.strategyRuntimeSummary(),
		"message":         "JFTrade API adapter is running.",
	}
}

func (s *Server) strategyRuntimeSummary() map[string]any {
	if s.strategyRuntimeManager == nil {
		return map[string]any{
			"status":                 "idle",
			"activeStrategies":       0,
			"supportsBacktestParity": true,
			"activeInstances":        []strategyRuntimeActiveInstanceSummary{},
		}
	}
	return s.strategyRuntimeManager.runtimeSummary()
}

func (s *Server) enrichStrategyItem(item strategyListItem) strategyListItem {
	if sync := s.buildStrategyDefinitionSyncStatus(item); sync != nil {
		item.DefinitionSync = sync
	}
	if s.strategyRuntimeStore != nil {
		persistedLogs, err := s.strategyRuntimeStore.ListRecentLogsTail(context.Background(), item.ID, strategyListLogsTailSize)
		if err != nil {
			log.Printf("JFTrade load persisted strategy list logs degraded: %v", err)
		} else if len(persistedLogs) > 0 {
			logs := make([]string, 0, len(persistedLogs))
			for _, event := range persistedLogs {
				logs = append(logs, event.Raw)
			}
			item.Logs = logs
		}
	}
	if s.strategyRuntimeManager != nil {
		if observation, ok := s.strategyRuntimeManager.runtimeObservation(item.ID); ok {
			item.RuntimeObservation = &observation
			return item
		}
	}
	if s.strategyRuntimeStore != nil {
		snapshot, ok, err := s.strategyRuntimeStore.GetObservation(context.Background(), item.ID)
		if err != nil {
			log.Printf("JFTrade load persisted strategy runtime observation degraded: %v", err)
			return item
		}
		if ok {
			observation := strategyRuntimeObservationFromSnapshot(snapshot, item.Status)
			item.RuntimeObservation = &observation
		}
	}
	return item
}

func (s *Server) buildStrategyDefinitionSyncStatus(item strategyListItem) *strategyDefinitionSyncStatus {
	definitionID := strings.TrimSpace(item.Definition.StrategyID)
	if definitionID == "" {
		definitionID = strategyDefinitionIDFromParams(item.Params)
	}
	if definitionID == "" {
		return nil
	}
	appliedVersion := strings.TrimSpace(item.Definition.Version)
	status := &strategyDefinitionSyncStatus{
		DefinitionID:   definitionID,
		AppliedVersion: appliedVersion,
		LatestVersion:  appliedVersion,
		IsLatest:       true,
	}
	if s == nil || s.designStore == nil {
		return status
	}
	definition, ok := s.designStore.definition(definitionID)
	if !ok {
		return status
	}
	status.LatestVersion = strings.TrimSpace(definition.Version)
	status.IsLatest = status.AppliedVersion == status.LatestVersion
	if status.IsLatest {
		return status
	}
	status.CanApplyLatest = item.Status == strategyStatusStopped
	if !status.CanApplyLatest {
		reason := "当前实例不是 STOPPED，先停止后才能刷新到最新策略。"
		status.BlockedReason = &reason
	}
	return status
}

func (s *Server) enrichStrategyItems(items []strategyListItem) []strategyListItem {
	if len(items) == 0 {
		return items
	}
	enriched := make([]strategyListItem, len(items))
	for index := range items {
		enriched[index] = s.enrichStrategyItem(items[index])
	}
	return enriched
}

func (s *Server) emptyConnectivityList(key string, value any, extraKeys ...string) map[string]any {
	result := map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "disconnected",
		"lastError":    nil,
		key:            value,
	}
	for _, extraKey := range extraKeys {
		result[extraKey] = []any{}
	}
	return result
}

func (s *Server) realTradeApprovals() map[string]any {
	return map[string]any{
		"realTradingEnabled":       false,
		"requiredConfirmationText": "ENABLE_REAL_TRADING",
		"maxApprovalAgeMs":         5 * 60 * 1000,
		"approvalPolicy":           map[string]any{"approverAllowlistEnabled": false, "approverCount": 0},
		"entries":                  []any{},
	}
}

func (s *Server) realTradeKillSwitch() map[string]any {
	return map[string]any{
		"realTradingEnabled": false, "envConfiguredActive": false, "controlPlaneActive": false,
		"killSwitchActive": false, "killSwitchSource": nil, "blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entry": nil,
	}
}

func (s *Server) realTradeRiskState() map[string]any {
	return map[string]any{
		"realTradingEnabled": false, "riskEnabled": false, "riskConfigSource": nil,
		"envConfiguredMaxOrderQuantity": nil, "envConfiguredMaxOrderNotional": nil,
		"controlPlaneActive": false, "controlPlaneMaxOrderQuantity": nil, "controlPlaneMaxOrderNotional": nil,
		"effectiveMaxOrderQuantity": nil, "effectiveMaxOrderNotional": nil, "entry": nil,
	}
}

func (s *Server) realTradeRiskEvents() map[string]any {
	result := s.realTradeRiskState()
	result["maxOrderQuantity"] = nil
	result["maxOrderNotional"] = nil
	result["entries"] = []any{}
	delete(result, "entry")
	return result
}

func stringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func decodePathSegment(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", err
	}
	return decoded, nil
}

func pathMiddle(path string, prefix string, suffix string) string {
	tail := strings.TrimPrefix(path, prefix)
	return strings.TrimSuffix(tail, suffix)
}
