package jftradeapi

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/broker"
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

var errFutuIntegrationNotEnabled = errors.New("futu integration is not enabled")

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
	liveStreams            liveStreamPool
	liveSocketClients      liveWebSocketClientRegistry
	liveNotifications      liveNotificationCache
	liveQuoteState         liveQuoteRefreshState
	liveStreamState        liveMarketStreamState
	exchangeMu             sync.Mutex
	exchange               *futu.Exchange
	exchangeConfigKey      string
	brokers                *broker.Registry // Unified broker registry for multi-broker support
	adkRuntime             *jfadk.Runtime
	frontend               *frontendServer
	apiPort                int
	auth                   *adminAuth
	router                 *gin.Engine
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
	backtestRunStore, err := newBacktestRunStoreWithDB(deriveBacktestRunDBPath(store.path))
	if err != nil {
		log.Printf("JFTrade backtest run store degraded: %v", err)
		fallbackSettingsPath := filepath.Join(os.TempDir(), "jftrade-backtest-runs-fallback", "settings.json")
		backtestRunStore, err = newBacktestRunStoreWithDB(deriveBacktestRunDBPath(fallbackSettingsPath))
		if err != nil {
			log.Printf("JFTrade backtest run fallback sqlite store degraded: %v", err)
			backtestRunStore = newBacktestRunStore()
		}
	}
	executionOrderStore, err := newExecutionOrderStoreWithDB(deriveExecutionOrderDBPath(store.path))
	if err != nil {
		log.Printf("JFTrade execution order sqlite store degraded: %v", err)
		executionOrderStore = newExecutionOrderStore()
	}
	executionOrderStore.configureSeenFillRetention(store.executionSettings().SeenFillRetentionDays)
	auth, authErr := newAdminAuth(store.path)
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
		brokerOrderUpdates:   newBrokerOrderUpdateWorker(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
		marketSubscriptions: newMarketSubscriptionManager(),
		tickCache:           newTickSampleCacheManager(),
		brokers:             broker.NewRegistry(),
		apiPort:             portFromBind(defaultDevelopmentAPIBind, 3000),
		frontend:            frontend,
		auth:                auth,
	}
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
	server.applySecuritySettings(store.securitySettings())
	server.adkRuntime = newADKRuntime(server, store.path)
	server.strategyRuntimeManager = newStrategyRuntimeManager(server)
	if reconciled, err := server.strategyStore.reconcileRuntimeStatesOnStartup(); err != nil {
		log.Printf("JFTrade strategy runtime state reconciliation failed: %v", err)
	} else if reconciled > 0 {
		log.Printf("JFTrade reconciled %d stale strategy runtime state(s) to STOPPED during startup", reconciled)
	}
	registerBBGONotificationSink(server.recordLiveNotification)
	server.router = server.buildRouter()
	return server
}

// --- Exchange / broker accessors (see also futu_runtime.go for futuExchange) ---

func (s *Server) liveMarketExchange() bbgotypes.Exchange {
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
	integration := s.store.savedIntegration()
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

func (s *Server) isWriteMethod(r *http.Request) bool {
	return r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete
}

func (s *Server) writeOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{OK: true, Data: data, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
}

func (s *Server) writeError(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, envelope{OK: false, Error: &apiError{Code: code, Message: message}, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
}

// Close releases all resources held by the server, including database connections.
// It is safe to call Close multiple times. After Close, the server should not be used.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
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
	if s.adkRuntime != nil && s.adkRuntime.Store() != nil {
		if err := s.adkRuntime.Close(); err != nil {
			errs = append(errs, fmt.Errorf("adkRuntime close: %w", err))
		}
	}
	return errors.Join(errs...)
}
