package jftradeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/gorilla/websocket"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

const (
	defaultAPIBind             = "127.0.0.1:3000"
	defaultSettingsPath        = "var/jftrade-api/settings.json"
	defaultFutuHost            = "127.0.0.1"
	defaultFutuAPIPort         = 11110
	defaultFutuWebSocketPort   = 11111
	defaultMaxWebSocketClients = 20
	maxTickCacheSamples        = 30000
	tickCacheRetention         = 30 * time.Minute
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

type FutuIntegrationConfig struct {
	Type                    string `json:"type"`
	Host                    string `json:"host"`
	APIPort                 int    `json:"apiPort"`
	WebSocketPort           int    `json:"websocketPort"`
	MaxWebSocketConnections int    `json:"maxWebSocketConnections"`
	UseEncryption           bool   `json:"useEncryption"`
	WebSocketKey            string `json:"websocketKey"`
	TradeMarket             string `json:"tradeMarket"`
	SecurityFirm            string `json:"securityFirm"`
}

type BrokerIntegration struct {
	BrokerID  string                `json:"brokerId"`
	Enabled   bool                  `json:"enabled"`
	Config    FutuIntegrationConfig `json:"config"`
	UpdatedAt string                `json:"updatedAt"`
	CreatedAt string                `json:"createdAt"`
}

type ManagedBrokerAccount struct {
	ID                 string  `json:"id"`
	BrokerID           string  `json:"brokerId"`
	AccountID          string  `json:"accountId"`
	DisplayName        string  `json:"displayName"`
	TradingEnvironment string  `json:"tradingEnvironment"`
	Market             string  `json:"market"`
	SecurityFirm       *string `json:"securityFirm"`
	Enabled            bool    `json:"enabled"`
	UpdatedAt          string  `json:"updatedAt"`
	CreatedAt          string  `json:"createdAt"`
}

type settingsFile struct {
	Integration *BrokerIntegration     `json:"integration,omitempty"`
	Accounts    []ManagedBrokerAccount `json:"accounts,omitempty"`
}

type SettingsStore struct {
	path string
	mu   sync.RWMutex
	data settingsFile
}

type Server struct {
	store                  *SettingsStore
	upgrader               websocket.Upgrader
	marketMu               sync.Mutex
	marketSubscriptions    map[string]*marketSubscription
	tickCacheMu            sync.Mutex
	tickCache              map[string][]marketTickSample
	exchangeMu             sync.Mutex
	exchange               *futu.Exchange
	exchangeConfigKey      string
	liveMu                 sync.Mutex
	liveWebSocketClients   int
	liveRefreshMu          sync.Mutex
	liveLastQuoteRefreshAt time.Time
	liveQuoteRetryAfter    time.Time
	liveQuoteFailureCount  int
	liveQuoteLastError     string
	liveStreamMu           sync.Mutex
	liveStream             bbgotypes.Stream
	liveStreamKey          string
	liveStreamRetryAfter   time.Time
	liveStreamFailureCount int
	liveStreamLastError    string
}

type marketSubscription struct {
	Key          string
	Channel      string
	Market       string
	Symbol       string
	InstrumentID string
	Interval     string
	Consumers    map[string]time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type marketTickSample struct {
	InstrumentID       string
	Market             string
	Symbol             string
	Price              float64
	Bid                float64
	Ask                float64
	OpenPrice          *float64
	HighPrice          *float64
	LowPrice           *float64
	PreviousClosePrice *float64
	Volume             float64
	Turnover           float64
	QuoteAt            string
	ObservedAt         string
	Source             string
	Session            string
	ExtendedHours      bool
	PreMarket          *futu.ExtendedMarketQuote
	AfterMarket        *futu.ExtendedMarketQuote
	Overnight          *futu.ExtendedMarketQuote
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

	settingsPath := envOrDefault("JFTRADE_SETTINGS_PATH", defaultSettingsPath)
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		return nil, err
	}
	store.applyRuntimeEnv()

	bind := envOrDefault("JFTRADE_API_BIND", defaultAPIBind)
	server := &http.Server{
		Addr:              bind,
		Handler:           NewServer(store),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("JFTrade API listening on http://%s", bind)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("JFTrade API server stopped: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	return server.Shutdown, nil
}

func NewSettingsStore(path string) (*SettingsStore, error) {
	store := &SettingsStore{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func NewServer(store *SettingsStore) *Server {
	return &Server{
		store:               store,
		marketSubscriptions: map[string]*marketSubscription{},
		tickCache:           map[string][]marketTickSample{},
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
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

func (s *SettingsStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = settingsFile{}
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		s.data = settingsFile{}
		return nil
	}
	return json.Unmarshal(data, &s.data)
}

func (s *SettingsStore) integration() BrokerIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Integration != nil {
		return *s.data.Integration
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return BrokerIntegration{
		BrokerID:  "futu",
		Enabled:   true,
		Config:    defaultFutuConfig(),
		UpdatedAt: now,
		CreatedAt: now,
	}
}

func (s *SettingsStore) saveIntegration(input BrokerIntegration) (BrokerIntegration, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.BrokerID = "futu"
	input.Config = normalizeFutuConfig(input.Config)
	input.UpdatedAt = now
	if input.CreatedAt == "" {
		existing := s.integration()
		input.CreatedAt = existing.CreatedAt
		if input.CreatedAt == "" {
			input.CreatedAt = now
		}
	}

	s.mu.Lock()
	s.data.Integration = &input
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		s.mu.Unlock()
		return input, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		s.mu.Unlock()
		return input, err
	}
	err = os.WriteFile(s.path, data, 0o600)
	s.mu.Unlock()
	if err != nil {
		return input, err
	}

	s.applyRuntimeEnv()
	return input, nil
}

func (s *SettingsStore) managedAccounts() []ManagedBrokerAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accounts := make([]ManagedBrokerAccount, len(s.data.Accounts))
	copy(accounts, s.data.Accounts)
	return accounts
}

func (s *SettingsStore) createManagedAccount(input ManagedBrokerAccount) (ManagedBrokerAccount, error) {
	input = normalizeManagedBrokerAccount(input)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		account := &s.data.Accounts[index]
		if sameManagedAccountScope(*account, input) {
			input.ID = account.ID
			input.CreatedAt = account.CreatedAt
			if input.CreatedAt == "" {
				input.CreatedAt = now
			}
			s.data.Accounts[index] = input
			if err := s.persistLocked(); err != nil {
				return input, err
			}
			return input, nil
		}
	}

	if input.ID == "" {
		input.ID = buildManagedAccountID(input)
	}
	if input.CreatedAt == "" {
		input.CreatedAt = now
	}
	s.data.Accounts = append(s.data.Accounts, input)
	if err := s.persistLocked(); err != nil {
		return input, err
	}
	return input, nil
}

func (s *SettingsStore) updateManagedAccount(id string, input ManagedBrokerAccount) (ManagedBrokerAccount, error) {
	input = normalizeManagedBrokerAccount(input)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		account := &s.data.Accounts[index]
		if account.ID != id {
			continue
		}
		input.ID = account.ID
		input.CreatedAt = account.CreatedAt
		input.UpdatedAt = now
		s.data.Accounts[index] = input
		if err := s.persistLocked(); err != nil {
			return input, err
		}
		return input, nil
	}

	return ManagedBrokerAccount{}, os.ErrNotExist
}

func (s *SettingsStore) deleteManagedAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		if s.data.Accounts[index].ID != id {
			continue
		}
		s.data.Accounts = append(s.data.Accounts[:index], s.data.Accounts[index+1:]...)
		return s.persistLocked()
	}
	return os.ErrNotExist
}

func (s *SettingsStore) persistLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func normalizeManagedBrokerAccount(input ManagedBrokerAccount) ManagedBrokerAccount {
	input.BrokerID = strings.TrimSpace(strings.ToLower(input.BrokerID))
	if input.BrokerID == "" {
		input.BrokerID = "futu"
	}
	input.AccountID = strings.TrimSpace(input.AccountID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		input.DisplayName = input.AccountID
	}
	input.TradingEnvironment = strings.ToUpper(strings.TrimSpace(input.TradingEnvironment))
	if input.TradingEnvironment == "" {
		input.TradingEnvironment = "SIMULATE"
	}
	input.Market = strings.ToUpper(strings.TrimSpace(input.Market))
	if input.Market == "" {
		input.Market = "HK"
	}
	if input.SecurityFirm != nil {
		value := strings.TrimSpace(*input.SecurityFirm)
		if value == "" {
			input.SecurityFirm = nil
		} else {
			input.SecurityFirm = &value
		}
	}
	return input
}

func sameManagedAccountScope(left ManagedBrokerAccount, right ManagedBrokerAccount) bool {
	return left.BrokerID == right.BrokerID &&
		left.AccountID == right.AccountID &&
		left.TradingEnvironment == right.TradingEnvironment &&
		left.Market == right.Market
}

func buildManagedAccountID(input ManagedBrokerAccount) string {
	return strings.Join([]string{input.BrokerID, input.TradingEnvironment, input.AccountID, input.Market}, "|")
}

func (s *SettingsStore) applyRuntimeEnv() {
	integration := s.integration()
	config := integration.Config
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.APIPort))
	_ = os.Setenv(futu.EnvOpenDAddr, addr)
	_ = os.Setenv("FUTU_OPEND_WEBSOCKET_KEY", config.WebSocketKey)
	_ = os.Setenv("JFTRADE_FUTU_WEBSOCKET_KEY", config.WebSocketKey)
	_ = os.Setenv("JFTRADE_FUTU_API_PORT", strconv.Itoa(config.APIPort))
	_ = os.Setenv("JFTRADE_FUTU_WEBSOCKET_PORT", strconv.Itoa(config.WebSocketPort))
}

func defaultFutuConfig() FutuIntegrationConfig {
	host := defaultFutuHost
	apiPort := defaultFutuAPIPort
	webSocketPort := defaultFutuWebSocketPort
	if rawAddr := strings.TrimSpace(os.Getenv(futu.EnvOpenDAddr)); rawAddr != "" {
		if parsedHost, parsedPort, err := net.SplitHostPort(rawAddr); err == nil {
			host = parsedHost
			if portValue, convErr := strconv.Atoi(parsedPort); convErr == nil && portValue > 0 {
				apiPort = portValue
			}
		}
	}

	return normalizeFutuConfig(FutuIntegrationConfig{
		Type:                    "futu",
		Host:                    envOrDefault("JFTRADE_FUTU_HOST", host),
		APIPort:                 intEnv("JFTRADE_FUTU_API_PORT", apiPort),
		WebSocketPort:           intEnv("JFTRADE_FUTU_WEBSOCKET_PORT", webSocketPort),
		MaxWebSocketConnections: intEnv("JFTRADE_FUTU_MAX_WEBSOCKET_CONNECTIONS", defaultMaxWebSocketClients),
		UseEncryption:           boolEnv("JFTRADE_FUTU_USE_ENCRYPTION", false),
		WebSocketKey:            firstNonEmpty(os.Getenv("JFTRADE_FUTU_WEBSOCKET_KEY"), os.Getenv("FUTU_OPEND_WEBSOCKET_KEY")),
		TradeMarket:             envOrDefault("JFTRADE_FUTU_TRADE_MARKET", "HK"),
		SecurityFirm:            envOrDefault("JFTRADE_FUTU_SECURITY_FIRM", "FUTUSECURITIES"),
	})
}

func normalizeFutuConfig(config FutuIntegrationConfig) FutuIntegrationConfig {
	if config.Type == "" {
		config.Type = "futu"
	}
	if strings.TrimSpace(config.Host) == "" {
		config.Host = defaultFutuHost
	}
	if config.APIPort <= 0 {
		config.APIPort = defaultFutuAPIPort
	}
	if config.WebSocketPort <= 0 {
		config.WebSocketPort = defaultFutuWebSocketPort
	}
	if config.MaxWebSocketConnections <= 0 {
		config.MaxWebSocketConnections = defaultMaxWebSocketClients
	}
	if strings.TrimSpace(config.TradeMarket) == "" {
		config.TradeMarket = "HK"
	}
	if strings.TrimSpace(config.SecurityFirm) == "" {
		config.SecurityFirm = "FUTUSECURITIES"
	}
	return config
}

func intEnv(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.writeCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch {
	case r.URL.Path == "/api/v1/ws/live":
		s.handleLiveWebSocket(w, r)
	case r.URL.Path == "/api/v1/stream/live" || r.URL.Path == "/api/v1/streams/console":
		s.handleEventStream(w, r)
	case r.URL.Path == "/api/v1/settings/brokers" && r.Method == http.MethodGet:
		s.writeOK(w, s.brokerSettings())
	case strings.HasPrefix(r.URL.Path, "/api/v1/settings/brokers/") && strings.HasSuffix(r.URL.Path, "/integration") && r.Method == http.MethodPut:
		s.handleSaveBrokerIntegration(w, r)
	case r.URL.Path == "/api/v1/settings/broker-accounts" && r.Method == http.MethodPost:
		s.handleCreateManagedBrokerAccount(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/settings/broker-accounts/") && r.Method == http.MethodPut:
		s.handleUpdateManagedBrokerAccount(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/settings/broker-accounts/") && r.Method == http.MethodDelete:
		s.handleDeleteManagedBrokerAccount(w, r)
	case r.URL.Path == "/api/v1/system/futu-opend" && r.Method == http.MethodGet:
		s.writeOK(w, s.futuOpenDHealth(r.Context()))
	case r.URL.Path == "/api/v1/system/futu-opend/manual-retry" && r.Method == http.MethodPost:
		s.resetFutuRuntime()
		s.writeOK(w, map[string]any{"accepted": true})
	case r.URL.Path == "/api/v1/system/futu-opend/install-guide" && r.Method == http.MethodGet:
		s.writeOK(w, s.futuOpenDInstallGuide())
	case r.URL.Path == "/api/v1/system/status" && r.Method == http.MethodGet:
		s.writeOK(w, s.systemStatus())
	case r.URL.Path == "/api/v1/system/storage/overview" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"pendingOutbox": []any{}, "recentJobs": []any{}, "recentAuditLogs": []any{}, "recentExecutionCommands": []any{}})
	case r.URL.Path == "/api/v1/system/real-trade-approvals" && r.Method == http.MethodGet:
		s.writeOK(w, s.realTradeApprovals())
	case r.URL.Path == "/api/v1/system/real-trade-hard-stops" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
	case r.URL.Path == "/api/v1/system/real-trade-hard-stop-events" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"realTradingEnabled": false, "blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
	case r.URL.Path == "/api/v1/system/real-trade-kill-switch" && r.Method == http.MethodGet:
		s.writeOK(w, s.realTradeKillSwitch())
	case r.URL.Path == "/api/v1/system/real-trade-kill-switch-events" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"realTradingEnabled": false, "killSwitchActive": false, "envConfiguredActive": false, "controlPlaneActive": false, "blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
	case r.URL.Path == "/api/v1/system/real-trade-risk-limits" && r.Method == http.MethodGet:
		s.writeOK(w, s.realTradeRiskState())
	case r.URL.Path == "/api/v1/system/real-trade-risk-events" && r.Method == http.MethodGet:
		s.writeOK(w, s.realTradeRiskEvents())
	case r.URL.Path == "/api/v1/system/worker/broker-order-updates" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"subscriptions": []any{}, "recentInvalidations": []any{}, "brokers": []any{}, "runtime": map[string]any{"lastStoppedAt": nil, "stoppedSubscriptions": nil}})
	case r.URL.Path == "/api/v1/plugins" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"targetDir": "", "plugins": []any{}})
	case r.URL.Path == "/api/v1/strategies" && r.Method == http.MethodGet:
		s.writeOK(w, []any{})
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/logs") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"instanceId": pathMiddle(r.URL.Path, "/api/v1/strategies/", "/logs"), "logs": []string{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/audit") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"instanceId": pathMiddle(r.URL.Path, "/api/v1/strategies/", "/audit"), "entries": []any{}})
	case r.URL.Path == "/api/v1/market-data/instruments" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"query": r.URL.Query().Get("query"), "totalReturned": 0, "entries": []any{}})
	case r.URL.Path == "/api/v1/market-data/subscriptions" && r.Method == http.MethodGet:
		s.writeOK(w, s.marketSubscriptionsResponse())
	case r.URL.Path == "/api/v1/market-data/subscriptions" && r.Method == http.MethodPost:
		s.handleAcquireMarketSubscription(w, r)
	case r.URL.Path == "/api/v1/market-data/subscriptions" && r.Method == http.MethodDelete:
		s.handleClearMarketSubscriptions(w, r)
	case r.URL.Path == "/api/v1/market-data/subscriptions/release" && r.Method == http.MethodPost:
		s.handleReleaseMarketSubscription(w, r)
	case r.URL.Path == "/api/v1/market-data/subscriptions/heartbeat" && r.Method == http.MethodPost:
		s.handleHeartbeatMarketSubscription(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/runtime") && r.Method == http.MethodGet:
		s.writeOK(w, s.brokerRuntime(r.Context()))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/funds") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("summary", nil, "currencyBalances", "marketAssets"))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/positions") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("positions", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/orders") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("orders", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/cash-flows") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("cashFlows", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/order-fees") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("fees", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/portfolio/") && strings.Contains(r.URL.Path, "/cash-balances") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"balances": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/portfolio/") && strings.Contains(r.URL.Path, "/positions") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"positions": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/portfolio/") && strings.Contains(r.URL.Path, "/cash-reconciliation") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("balances", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/portfolio/") && strings.Contains(r.URL.Path, "/reconciliation") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("positions", []any{}))
	case r.URL.Path == "/api/v1/execution/orders" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"orders": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/execution/orders/") && strings.HasSuffix(r.URL.Path, "/events") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"internalOrderId": "", "events": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/market-data/snapshots/") && r.Method == http.MethodGet:
		s.handleMarketSnapshot(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/market-data/candles/") && r.Method == http.MethodGet:
		s.handleMarketCandles(w, r)
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

func (s *Server) handleAcquireMarketSubscription(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Channel    string `json:"channel"`
		Market     string `json:"market"`
		Symbol     string `json:"symbol"`
		Interval   string `json:"interval"`
		ConsumerID string `json:"consumerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	market, symbol, channel := normalizeMarketDataSubscription(payload.Market, payload.Symbol, payload.Channel)
	if market == "" || symbol == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "market and symbol are required")
		return
	}
	consumerID := normalizeConsumerID(payload.ConsumerID)
	interval := strings.TrimSpace(strings.ToLower(payload.Interval))
	now := time.Now().UTC()
	key := marketSubscriptionKey(channel, market, symbol, interval)

	s.marketMu.Lock()
	entry := s.marketSubscriptions[key]
	if entry == nil {
		entry = &marketSubscription{
			Key:          key,
			Channel:      channel,
			Market:       market,
			Symbol:       symbol,
			InstrumentID: market + "." + symbol,
			Interval:     interval,
			Consumers:    map[string]time.Time{},
			CreatedAt:    now,
		}
		s.marketSubscriptions[key] = entry
	}
	entry.Consumers[consumerID] = now
	entry.UpdatedAt = now
	response := s.marketSubscriptionsResponseLocked()
	s.marketMu.Unlock()

	s.writeOK(w, response)
}

func (s *Server) handleReleaseMarketSubscription(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Channel    string `json:"channel"`
		Market     string `json:"market"`
		Symbol     string `json:"symbol"`
		Interval   string `json:"interval"`
		ConsumerID string `json:"consumerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	market, symbol, channel := normalizeMarketDataSubscription(payload.Market, payload.Symbol, payload.Channel)
	consumerID := normalizeConsumerID(payload.ConsumerID)
	key := marketSubscriptionKey(channel, market, symbol, strings.TrimSpace(strings.ToLower(payload.Interval)))
	now := time.Now().UTC()

	s.marketMu.Lock()
	if entry := s.marketSubscriptions[key]; entry != nil {
		delete(entry.Consumers, consumerID)
		entry.UpdatedAt = now
		if len(entry.Consumers) == 0 {
			delete(s.marketSubscriptions, key)
		}
	}
	response := s.marketSubscriptionsResponseLocked()
	s.marketMu.Unlock()

	s.writeOK(w, response)
}

func (s *Server) handleHeartbeatMarketSubscription(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ConsumerID string `json:"consumerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	consumerID := normalizeConsumerID(payload.ConsumerID)
	now := time.Now().UTC()

	s.marketMu.Lock()
	for _, entry := range s.marketSubscriptions {
		if _, exists := entry.Consumers[consumerID]; exists {
			entry.Consumers[consumerID] = now
			entry.UpdatedAt = now
		}
	}
	response := s.marketSubscriptionsResponseLocked()
	s.marketMu.Unlock()

	s.writeOK(w, response)
}

func (s *Server) handleClearMarketSubscriptions(w http.ResponseWriter, r *http.Request) {
	consumerID := normalizeConsumerID(r.URL.Query().Get("consumerId"))

	s.marketMu.Lock()
	if consumerID == "web" && r.URL.Query().Get("consumerId") == "" {
		s.marketSubscriptions = map[string]*marketSubscription{}
	} else {
		for key, entry := range s.marketSubscriptions {
			delete(entry.Consumers, consumerID)
			entry.UpdatedAt = time.Now().UTC()
			if len(entry.Consumers) == 0 {
				delete(s.marketSubscriptions, key)
			}
		}
	}
	response := s.marketSubscriptionsResponseLocked()
	s.marketMu.Unlock()

	s.writeOK(w, response)
}

func (s *Server) marketSubscriptionsResponse() map[string]any {
	s.marketMu.Lock()
	defer s.marketMu.Unlock()
	return s.marketSubscriptionsResponseLocked()
}

func (s *Server) marketSubscriptionsResponseLocked() map[string]any {
	entries := make([]map[string]any, 0, len(s.marketSubscriptions))
	byMarket := map[string]int{}
	for _, entry := range s.marketSubscriptions {
		consumers := make([]string, 0, len(entry.Consumers))
		for consumerID := range entry.Consumers {
			consumers = append(consumers, consumerID)
		}
		byMarket[entry.Market]++
		var interval any
		if entry.Interval != "" {
			interval = entry.Interval
		}
		entries = append(entries, map[string]any{
			"key":          entry.Key,
			"channel":      entry.Channel,
			"market":       entry.Market,
			"symbol":       entry.Symbol,
			"instrumentId": entry.InstrumentID,
			"interval":     interval,
			"depthLevel":   nil,
			"consumers":    consumers,
			"refCount":     len(consumers),
			"createdAt":    entry.CreatedAt.Format(time.RFC3339Nano),
			"updatedAt":    entry.UpdatedAt.Format(time.RFC3339Nano),
		})
	}

	quotaBuckets := make([]map[string]any, 0, len(byMarket))
	for market, used := range byMarket {
		quotaBuckets = append(quotaBuckets, map[string]any{"market": market, "used": used, "limit": nil, "remaining": nil})
	}

	return map[string]any{
		"totalActiveSubscriptions": len(entries),
		"quota": map[string]any{
			"totalUsed":      len(entries),
			"totalLimit":     nil,
			"totalRemaining": nil,
			"byMarket":       quotaBuckets,
		},
		"entries": entries,
	}
}

func normalizeMarketDataSubscription(market string, symbol string, channel string) (string, string, string) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	channel = strings.ToUpper(strings.TrimSpace(channel))
	if channel == "" {
		channel = "SNAPSHOT"
	}
	if strings.Contains(symbol, ".") {
		parts := strings.SplitN(symbol, ".", 2)
		if market == "" {
			market = strings.ToUpper(parts[0])
		}
		symbol = strings.ToUpper(parts[1])
	}
	return market, symbol, channel
}

func normalizeConsumerID(consumerID string) string {
	consumerID = strings.TrimSpace(consumerID)
	if consumerID == "" {
		return "web"
	}
	return consumerID
}

func marketSubscriptionKey(channel string, market string, symbol string, interval string) string {
	if interval == "" {
		return channel + ":" + market + ":" + symbol
	}
	return channel + ":" + market + ":" + symbol + ":" + interval
}

func (s *Server) handleSaveBrokerIntegration(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Enabled bool                  `json:"enabled"`
		Config  FutuIntegrationConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	integration, err := s.store.saveIntegration(BrokerIntegration{BrokerID: "futu", Enabled: payload.Enabled, Config: payload.Config})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, integration)
}

func (s *Server) handleCreateManagedBrokerAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		BrokerID           string `json:"brokerId"`
		AccountID          string `json:"accountId"`
		DisplayName        string `json:"displayName"`
		TradingEnvironment string `json:"tradingEnvironment"`
		Market             string `json:"market"`
		SecurityFirm       string `json:"securityFirm"`
		Enabled            bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(payload.AccountID) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "accountId is required")
		return
	}
	account, err := s.store.createManagedAccount(ManagedBrokerAccount{
		BrokerID:           payload.BrokerID,
		AccountID:          payload.AccountID,
		DisplayName:        payload.DisplayName,
		TradingEnvironment: payload.TradingEnvironment,
		Market:             payload.Market,
		SecurityFirm:       stringPointerOrNil(payload.SecurityFirm),
		Enabled:            payload.Enabled,
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, account)
}

func (s *Server) handleUpdateManagedBrokerAccount(w http.ResponseWriter, r *http.Request) {
	accountID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/broker-accounts/"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if accountID == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "accountRecordId is required")
		return
	}
	var payload struct {
		BrokerID           string `json:"brokerId"`
		AccountID          string `json:"accountId"`
		DisplayName        string `json:"displayName"`
		TradingEnvironment string `json:"tradingEnvironment"`
		Market             string `json:"market"`
		SecurityFirm       string `json:"securityFirm"`
		Enabled            bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	account, err := s.store.updateManagedAccount(accountID, ManagedBrokerAccount{
		BrokerID:           payload.BrokerID,
		AccountID:          payload.AccountID,
		DisplayName:        payload.DisplayName,
		TradingEnvironment: payload.TradingEnvironment,
		Market:             payload.Market,
		SecurityFirm:       stringPointerOrNil(payload.SecurityFirm),
		Enabled:            payload.Enabled,
	})
	if errors.Is(err, os.ErrNotExist) {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "managed broker account not found")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, account)
}

func (s *Server) handleDeleteManagedBrokerAccount(w http.ResponseWriter, r *http.Request) {
	accountID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/broker-accounts/"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if accountID == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "accountRecordId is required")
		return
	}
	if err := s.store.deleteManagedAccount(accountID); errors.Is(err, os.ErrNotExist) {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "managed broker account not found")
		return
	} else if err != nil {
		s.writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, map[string]any{"deleted": true, "id": accountID})
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	write := func() bool {
		_, err := fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"type": "heartbeat", "at": time.Now().UTC().Format(time.RFC3339Nano)}))
		if flusher != nil {
			flusher.Flush()
		}
		return err == nil
	}
	if !write() {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !write() {
				return
			}
		}
	}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func (s *Server) systemStatus() map[string]any {
	return map[string]any{
		"name":                      "JFTrade",
		"apiPort":                   3000,
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
		"persistence": map[string]any{
			"engine": "json", "databasePath": s.store.path, "status": "ok", "migrated": true,
			"pendingMigrations": []any{}, "tables": []string{"broker_integrations", "broker_accounts"}, "checkedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
		"strategyRuntime": map[string]any{"status": "idle", "activeStrategies": 0, "supportsBacktestParity": true},
		"message":         "JFTrade API adapter is running.",
	}
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
