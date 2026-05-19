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

	bbgofixedpoint "github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
)

const (
	defaultAPIBind             = "127.0.0.1:3000"
	defaultSettingsPath        = "var/jftrade-api/settings.json"
	defaultFutuHost            = "127.0.0.1"
	defaultFutuAPIPort         = 11110
	defaultFutuWebSocketPort   = 11111
	defaultMaxWebSocketClients = 20
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
	store               *SettingsStore
	upgrader            websocket.Upgrader
	marketMu            sync.Mutex
	marketSubscriptions map[string]*marketSubscription
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
		if arg == "run" {
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

func (s *Server) handleLiveWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	if err := writeHeartbeat(conn); err != nil {
		return
	}
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()
	quoteTicker := time.NewTicker(3 * time.Second)
	defer quoteTicker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeatTicker.C:
			if err := writeHeartbeat(conn); err != nil {
				return
			}
		case <-quoteTicker.C:
			if err := s.writeLiveMarketTicks(r.Context(), conn); err != nil {
				return
			}
		}
	}
}

func writeHeartbeat(conn *websocket.Conn) error {
	return conn.WriteJSON(map[string]any{"type": "heartbeat", "at": time.Now().UTC().Format(time.RFC3339Nano)})
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

func (s *Server) descriptor() map[string]any {
	return map[string]any{
		"id":           "futu",
		"displayName":  "Futu OpenAPI via OpenD",
		"environments": []string{"SIMULATE", "REAL"},
		"capabilities": []map[string]any{{"market": "HK", "supportsQuote": true, "supportsTrade": true}},
		"notes":        []string{},
	}
}

func (s *Server) brokerSettings() map[string]any {
	integration := s.store.integration()
	return map[string]any{
		"brokers": []any{map[string]any{
			"descriptor":  s.descriptor(),
			"integration": integration,
			"defaults":    integration.Config,
		}},
		"accounts": s.store.managedAccounts(),
	}
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

func (s *Server) futuOpenDInstallGuide() map[string]any {
	config := s.store.integration().Config
	return map[string]any{
		"brokerId":    "futu",
		"title":       "Futu OpenD",
		"description": "Configure Futu OpenD and connect JFTrade through the WebSocket API port.",
		"options":     []any{},
		"nextSteps": []string{
			"确认 OpenD 已登录并启用 WebSocket。",
			"保存 Host、API Port、WebSocket Port 和 WebSocket Password / Key。",
			"保存后刷新 OpenD 连接状态。",
		},
		"settings": map[string]any{
			"host": config.Host, "apiPort": config.APIPort, "websocketPort": config.WebSocketPort,
			"maxWebSocketConnections": config.MaxWebSocketConnections, "useEncryption": config.UseEncryption,
			"websocketKeyRequired": strings.TrimSpace(config.WebSocketKey) != "",
		},
	}
}

func (s *Server) brokerRuntime(ctx context.Context) map[string]any {
	probe := s.probeOpenD(ctx)
	config := s.store.integration().Config
	globalState := any(nil)
	if probe.QuoteLoggedIn != nil || probe.TradeLoggedIn != nil || probe.ProgramStatus != nil {
		globalState = map[string]any{
			"quoteLoggedIn": boolValue(probe.QuoteLoggedIn),
			"tradeLoggedIn": boolValue(probe.TradeLoggedIn),
			"serverVersion": probe.ServerVersion,
			"programStatus": probe.ProgramStatus,
			"timestamp":     probe.ProgramTimestamp,
			"markets":       probe.Markets,
		}
	}
	return map[string]any{
		"descriptor": s.descriptor(),
		"session": map[string]any{
			"brokerId":           "futu",
			"displayName":        "Futu OpenAPI via OpenD",
			"connection":         map[string]any{"host": config.Host, "port": config.WebSocketPort, "useEncryption": config.UseEncryption},
			"connectivity":       probe.Connectivity,
			"checkedAt":          probe.CheckedAt,
			"lastError":          probe.LastError,
			"globalState":        globalState,
			"accountsDiscovered": 0,
		},
		"accounts": []any{},
	}
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func (s *Server) futuOpenDHealth(ctx context.Context) map[string]any {
	probe := s.probeOpenD(ctx)
	config := s.store.integration().Config
	summary := any(nil)
	code := "NONE"
	manualRetry := false
	if probe.LastError != nil {
		summary = *probe.LastError
		code = "WEBSOCKET_AUTH"
		manualRetry = true
	}
	return map[string]any{
		"checkedAt": probe.CheckedAt,
		"status":    probe.Status,
		"runtime": map[string]any{
			"connectivity":           probe.Connectivity,
			"host":                   config.Host,
			"port":                   config.WebSocketPort,
			"useEncryption":          config.UseEncryption,
			"websocketKeyConfigured": strings.TrimSpace(config.WebSocketKey) != "",
			"quoteLoggedIn":          probe.QuoteLoggedIn,
			"tradeLoggedIn":          probe.TradeLoggedIn,
			"programStatus":          probe.ProgramStatus,
			"serverVersion":          probe.ServerVersion,
			"lastError":              probe.LastError,
		},
		"diagnosis": map[string]any{
			"code": code, "summary": summary, "manualRetryRequired": manualRetry, "restartOpenDRecommended": false,
		},
		"localSocketDiagnostics": map[string]any{
			"websocketEstablishedConnections": 0,
			"likelyConnectionSaturation":      false,
			"topClientProcesses":              []any{},
		},
		"localInstallation": map[string]any{
			"platform": os.Getenv("GOOS"), "installed": false, "version": nil, "installPath": nil, "guiDetected": false,
			"process": map[string]any{"running": false, "pid": nil, "executablePath": nil},
		},
		"latestVersion":   map[string]any{"value": nil, "sourceUrl": nil, "checkedAt": nil, "status": "unknown", "error": nil},
		"recommendations": []any{},
	}
}

func (s *Server) probeOpenD(ctx context.Context) opendProbe {
	config := s.store.integration().Config
	checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client := opend.New(opend.Config{
		Addr:             net.JoinHostPort(config.Host, strconv.Itoa(config.APIPort)),
		TLS:              config.UseEncryption,
		WebSocketKey:     config.WebSocketKey,
		HandshakeTimeout: 2 * time.Second,
		RequestTimeout:   3 * time.Second,
	})
	if err := client.Connect(probeCtx); err != nil {
		message := err.Error()
		return opendProbe{CheckedAt: checkedAt, Connectivity: "disconnected", Status: "offline", LastError: &message}
	}
	defer client.Close()

	initReq := &initpb.Request{C2S: &initpb.C2S{
		ClientVer:           proto.Int32(101),
		ClientID:            proto.String("jftrade-api"),
		RecvNotify:          proto.Bool(false),
		ProgrammingLanguage: proto.String("Go"),
	}}
	var initResp initpb.Response
	if err := client.Call(probeCtx, opend.ProtoInitConnect, initReq, &initResp); err != nil {
		message := err.Error()
		return opendProbe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: &message}
	}
	if initResp.GetRetType() != int32(commonpb.RetType_RetType_Succeed) {
		message := initResp.GetRetMsg()
		if message == "" {
			message = fmt.Sprintf("InitConnect failed: retType=%d", initResp.GetRetType())
		}
		return opendProbe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: &message}
	}

	globalReq := &globalpb.Request{C2S: &globalpb.C2S{UserID: proto.Uint64(0)}}
	var globalResp globalpb.Response
	if err := client.Call(probeCtx, opend.ProtoGetGlobalState, globalReq, &globalResp); err != nil {
		message := err.Error()
		return opendProbe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: &message}
	}
	if globalResp.GetRetType() != int32(commonpb.RetType_RetType_Succeed) {
		message := globalResp.GetRetMsg()
		if message == "" {
			message = fmt.Sprintf("GetGlobalState failed: retType=%d", globalResp.GetRetType())
		}
		return opendProbe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: &message}
	}

	s2c := globalResp.GetS2C()
	quoteLoggedIn := s2c.GetQotLogined()
	tradeLoggedIn := s2c.GetTrdLogined()
	serverVersion := fmt.Sprintf("%d.%d", s2c.GetServerVer(), s2c.GetServerBuildNo())
	programStatus := programStatusString(s2c.GetProgramStatus())
	programTimestamp := time.Unix(s2c.GetTime(), 0).UTC().Format(time.RFC3339Nano)

	return opendProbe{
		CheckedAt:        checkedAt,
		Connectivity:     "connected",
		Status:           "healthy",
		QuoteLoggedIn:    &quoteLoggedIn,
		TradeLoggedIn:    &tradeLoggedIn,
		ServerVersion:    &serverVersion,
		ProgramStatus:    &programStatus,
		ProgramTimestamp: &programTimestamp,
		Markets: []map[string]any{
			{"market": "HK", "state": strconv.Itoa(int(s2c.GetMarketHK()))},
			{"market": "US", "state": strconv.Itoa(int(s2c.GetMarketUS()))},
			{"market": "SH", "state": strconv.Itoa(int(s2c.GetMarketSH()))},
			{"market": "SZ", "state": strconv.Itoa(int(s2c.GetMarketSZ()))},
		},
	}
}

func programStatusString(status *commonpb.ProgramStatus) string {
	if status == nil {
		return "Unavailable"
	}
	value := status.GetType().String()
	if desc := status.GetStrExtDesc(); desc != "" {
		return value + ": " + desc
	}
	return value
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

func (s *Server) handleMarketSnapshot(w http.ResponseWriter, r *http.Request) {
	response, err := s.marketSnapshotResponse(r.Context(), r.URL.Path)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "MARKET_SNAPSHOT_FAILED", err.Error())
		return
	}
	s.writeOK(w, response)
}

func (s *Server) marketSnapshotResponse(ctx context.Context, path string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/snapshots/")
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	ticker, err := s.futuExchange().QueryTicker(ctx, instrumentID)
	if err != nil {
		return nil, err
	}
	resolvedAt := tickerTimestamp(ticker)
	price := ticker.Last.Float64()
	if ticker.Last.IsZero() {
		price = ticker.GetValidPrice().Float64()
	}
	bid := price
	if !ticker.Buy.IsZero() {
		bid = ticker.Buy.Float64()
	}
	ask := price
	if !ticker.Sell.IsZero() {
		ask = ticker.Sell.Float64()
	}
	return map[string]any{
		"request": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID},
		"snapshot": map[string]any{
			"price":              price,
			"bid":                bid,
			"ask":                ask,
			"openPrice":          tickerOptionalValue(ticker.Open),
			"highPrice":          tickerOptionalValue(ticker.High),
			"lowPrice":           tickerOptionalValue(ticker.Low),
			"previousClosePrice": nil,
			"volume":             ticker.Volume.Float64(),
			"turnover":           0,
			"at":                 resolvedAt,
		},
		"meta": map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": resolvedAt, "fromCache": false},
	}, nil
}

func (s *Server) handleMarketCandles(w http.ResponseWriter, r *http.Request) {
	response, err := s.marketCandlesResponse(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "OPEND_CANDLES_FAILED", err.Error())
		return
	}
	s.writeOK(w, response)
}

func (s *Server) marketCandlesResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/candles/")
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	period, err := normalizeCandlePeriod(firstQuery(query, "period", "1m"))
	if err != nil {
		return nil, err
	}
	limit := intQuery(query, "limit", 200)
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	if period == "tick" {
		ticker, err := s.futuExchange().QueryTicker(ctx, instrumentID)
		if err != nil {
			return nil, err
		}
		candles := []map[string]any{}
		if tickCandle := tickCandleFromTicker(ticker); tickCandle != nil {
			candles = append(candles, map[string]any{
				"period": period,
				"open":   tickCandle["open"],
				"high":   tickCandle["high"],
				"low":    tickCandle["low"],
				"close":  tickCandle["close"],
				"volume": tickCandle["volume"],
				"at":     tickCandle["at"],
			})
		}
		return map[string]any{
			"request":       map[string]any{"instrument": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID}, "period": period, "limit": limit},
			"candles":       candles,
			"totalReturned": len(candles),
			"meta":          map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": tickerTimestamp(ticker), "fromCache": false},
		}, nil
	}

	interval := bbgotypes.Interval(period)
	beginAt, endAt := kLineQueryWindow(query, interval.Duration(), limit)
	klines, err := s.futuExchange().QueryKLines(ctx, instrumentID, interval, bbgotypes.KLineQueryOptions{Limit: limit, StartTime: &beginAt, EndTime: &endAt})
	if err != nil {
		return nil, err
	}
	candles := make([]map[string]any, 0, len(klines))
	for _, kline := range klines {
		candles = append(candles, map[string]any{
			"period": period,
			"open":   kline.Open.Float64(),
			"high":   kline.High.Float64(),
			"low":    kline.Low.Float64(),
			"close":  kline.Close.Float64(),
			"volume": kline.Volume.Float64(),
			"at":     kline.StartTime.Time().UTC().Format(time.RFC3339Nano),
		})
	}

	return map[string]any{
		"request":       map[string]any{"instrument": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID}, "period": period, "limit": limit},
		"candles":       candles,
		"totalReturned": len(candles),
		"meta":          map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": false},
	}, nil
}

func kLineQueryWindow(query map[string][]string, periodDuration time.Duration, limit int) (time.Time, time.Time) {
	endAt := parseQueryTime(firstQuery(query, "toTime", ""), time.Now())
	if queryEnd := firstQuery(query, "to", ""); queryEnd != "" {
		endAt = parseQueryTime(queryEnd, endAt)
	}
	lookback := periodDuration * time.Duration(limit) * 4
	minimumLookback := 36 * time.Hour
	if periodDuration >= 24*time.Hour {
		minimumLookback = 45 * 24 * time.Hour
	}
	if lookback < minimumLookback {
		lookback = minimumLookback
	}
	defaultBegin := endAt.Add(-lookback)
	beginAt := parseQueryTime(firstQuery(query, "fromTime", ""), defaultBegin)
	if queryBegin := firstQuery(query, "from", ""); queryBegin != "" {
		beginAt = parseQueryTime(queryBegin, beginAt)
	}
	if !beginAt.Before(endAt) {
		beginAt = defaultBegin
	}
	return beginAt, endAt
}

func parseQueryTime(value string, fallback time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func (s *Server) futuExchange() *futu.Exchange {
	integration := s.store.integration()
	return futu.NewExchangeWithConfig(opend.Config{
		Addr:             net.JoinHostPort(integration.Config.Host, strconv.Itoa(integration.Config.APIPort)),
		WebSocketKey:     integration.Config.WebSocketKey,
		HandshakeTimeout: 3 * time.Second,
		RequestTimeout:   8 * time.Second,
	})
}

func (s *Server) writeLiveMarketTicks(ctx context.Context, conn *websocket.Conn) error {
	for _, instrumentID := range s.activeMarketInstrumentIDs() {
		ticker, err := s.futuExchange().QueryTicker(ctx, instrumentID)
		if err != nil {
			continue
		}
		event := liveTickEventFromTicker(instrumentID, ticker)
		if event == nil {
			continue
		}
		if err := conn.WriteJSON(event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) activeMarketInstrumentIDs() []string {
	s.marketMu.Lock()
	defer s.marketMu.Unlock()
	ids := make([]string, 0, len(s.marketSubscriptions))
	seen := make(map[string]struct{}, len(s.marketSubscriptions))
	for _, entry := range s.marketSubscriptions {
		if entry.Market == "" || entry.Symbol == "" {
			continue
		}
		instrumentID := entry.Market + "." + entry.Symbol
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		ids = append(ids, instrumentID)
	}
	return ids
}

func normalizeCandlePeriod(period string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "tick", "ticker", "k_tick":
		return "tick", nil
	case "1m", "1min", "k_1m":
		return "1m", nil
	case "3m", "3min", "k_3m":
		return "3m", nil
	case "5m", "5min", "k_5m":
		return "5m", nil
	case "10m", "10min", "k_10m":
		return "10m", nil
	case "15m", "15min", "k_15m":
		return "15m", nil
	case "30m", "30min", "k_30m":
		return "30m", nil
	case "60m", "60min", "1h", "k_60m":
		return "1h", nil
	case "1d", "day", "d", "k_day":
		return "1d", nil
	case "1w", "week", "w", "k_week":
		return "1w", nil
	case "1mo", "month", "mth", "k_month":
		return "1mo", nil
	default:
		return "", fmt.Errorf("unsupported period %q", period)
	}
}

func tickerTimestamp(ticker *bbgotypes.Ticker) string {
	resolvedAt := time.Now().UTC()
	if ticker != nil && !ticker.Time.IsZero() {
		resolvedAt = ticker.Time.UTC()
	}
	return resolvedAt.Format(time.RFC3339Nano)
}

func tickerOptionalValue(value bbgofixedpoint.Value) any {
	if value.IsZero() {
		return nil
	}
	return value.Float64()
}

func tickCandleFromTicker(ticker *bbgotypes.Ticker) map[string]any {
	if ticker == nil {
		return nil
	}
	price := ticker.Last.Float64()
	if ticker.Last.IsZero() {
		price = ticker.GetValidPrice().Float64()
	}
	if price == 0 {
		return nil
	}
	return map[string]any{
		"open":   price,
		"high":   price,
		"low":    price,
		"close":  price,
		"volume": ticker.Volume.Float64(),
		"at":     tickerTimestamp(ticker),
	}
}

func liveTickEventFromTicker(instrumentID string, ticker *bbgotypes.Ticker) map[string]any {
	tickCandle := tickCandleFromTicker(ticker)
	if tickCandle == nil {
		return nil
	}
	market, symbol := pathTail(instrumentID, "")
	if market == "" || symbol == "" {
		parts := strings.SplitN(instrumentID, ".", 2)
		if len(parts) != 2 {
			return nil
		}
		market = parts[0]
		symbol = parts[1]
	}
	price := tickCandle["close"]
	return map[string]any{
		"type":     "market-data.tick",
		"at":       tickCandle["at"],
		"brokerId": "futu",
		"instrument": map[string]any{
			"market":       market,
			"symbol":       symbol,
			"instrumentId": instrumentID,
		},
		"snapshot": map[string]any{
			"price":              price,
			"bid":                price,
			"ask":                price,
			"openPrice":          tickerOptionalValue(ticker.Open),
			"highPrice":          tickerOptionalValue(ticker.High),
			"lowPrice":           tickerOptionalValue(ticker.Low),
			"previousClosePrice": nil,
			"volume":             ticker.Volume.Float64(),
			"turnover":           0,
			"at":                 tickCandle["at"],
		},
		"source": "bbgo:futu",
	}
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

func pathTail(path string, prefix string) (string, string) {
	tail := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(tail, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func pathMiddle(path string, prefix string, suffix string) string {
	tail := strings.TrimPrefix(path, prefix)
	return strings.TrimSuffix(tail, suffix)
}

func firstQuery(query map[string][]string, key string, fallback string) string {
	values := query[key]
	if len(values) == 0 || values[0] == "" {
		return fallback
	}
	return values[0]
}

func intQuery(query map[string][]string, key string, fallback int) int {
	value, err := strconv.Atoi(firstQuery(query, key, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return value
}
