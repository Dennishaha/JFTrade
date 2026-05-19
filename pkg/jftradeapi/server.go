package jftradeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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

type settingsFile struct {
	Integration *BrokerIntegration `json:"integration,omitempty"`
}

type SettingsStore struct {
	path string
	mu   sync.RWMutex
	data settingsFile
}

type Server struct {
	store    *SettingsStore
	upgrader websocket.Upgrader
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
		store: store,
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
	case r.URL.Path == "/api/v1/market-data/instruments" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"query": r.URL.Query().Get("query"), "totalReturned": 0, "entries": []any{}})
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
		s.writeOK(w, marketSnapshotResponse(r.URL.Path))
	case strings.HasPrefix(r.URL.Path, "/api/v1/market-data/candles/") && r.Method == http.MethodGet:
		s.writeOK(w, marketCandlesResponse(r.URL.Path, r.URL.Query()))
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

func (s *Server) handleLiveWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	if err := writeHeartbeat(conn); err != nil {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := writeHeartbeat(conn); err != nil {
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
		"accounts": []any{},
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
			"pendingMigrations": []any{}, "tables": []string{"broker_integrations"}, "checkedAt": time.Now().UTC().Format(time.RFC3339Nano),
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

func marketSnapshotResponse(path string) map[string]any {
	market, symbol := pathTail(path, "/api/v1/market-data/snapshots/")
	instrumentID := market + "." + symbol
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID},
		"snapshot": nil,
		"meta":     map[string]any{"instrumentId": instrumentID, "source": nil, "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": false},
	}
}

func marketCandlesResponse(path string, query map[string][]string) map[string]any {
	market, symbol := pathTail(path, "/api/v1/market-data/candles/")
	instrumentID := market + "." + symbol
	period := firstQuery(query, "period", "1m")
	limit := intQuery(query, "limit", 200)
	return map[string]any{
		"request":       map[string]any{"instrument": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID}, "period": period, "limit": limit},
		"candles":       []any{},
		"totalReturned": 0,
		"meta":          map[string]any{"instrumentId": instrumentID, "source": nil, "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": false},
	}
}

func pathTail(path string, prefix string) (string, string) {
	tail := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(tail, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
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
