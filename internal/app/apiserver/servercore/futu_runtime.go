package servercore

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
)

const liveQuoteTransportMode = "bbgo-opend-tcp-api"

func (s *Server) descriptor() map[string]any {
	return map[string]any{
		"id":           "futu",
		"displayName":  "Futu OpenAPI via OpenD",
		"environments": []string{"SIMULATE", "REAL"},
		"capabilities": []map[string]any{{
			"market":        "HK",
			"supportsQuote": true,
			"supportsTrade": true,
			"readFeatures": map[string]any{
				"funds":            map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}},
				"positions":        map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}},
				"orders":           map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "supportsHistory": true},
				"fills":            map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "supportsHistory": true},
				"cashFlows":        map[string]any{"supportedEnvironments": []string{"REAL"}, "requiresClearingDate": true},
				"orderFees":        map[string]any{"supportedEnvironments": []string{"REAL"}, "requiresOrderIdEx": true},
				"marginRatios":     map[string]any{"supportedEnvironments": []string{"REAL"}, "requiresSymbols": true},
				"maxTradeQuantity": map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "requiresPrice": true},
				"orderBook":        map[string]any{"defaultNum": 10, "minNum": 1, "maxNum": 50, "numPresets": []int32{5, 10, 20, 50}, "supportsRealTimePush": true},
			},
		}},
		"notes": []string{
			"Market data is exposed to the frontend through the bbgo exchange boundary.",
			"OpenD WebSocket settings are retained for compatibility and diagnostics; the current hot path uses the native API port.",
		},
	}
}

func (s *Server) brokerSettings() map[string]any {
	integration := s.store.SavedIntegration()
	defaults := defaultFutuConfig()
	if integration != nil {
		defaults = integration.Config
	}
	return map[string]any{
		"brokers": []any{map[string]any{
			"descriptor":  s.descriptor(),
			"integration": integration,
			"defaults":    defaults,
		}},
		"accounts": s.store.ManagedAccounts(),
	}
}

func (s *Server) onboardingState(ctx context.Context) map[string]any {
	return s.onboardingStateFromSettings(ctx, s.store.Onboarding())
}

func (s *Server) onboardingStateFromSettings(_ context.Context, onboarding OnboardingSettings) map[string]any {
	integration := s.store.SavedIntegration()
	accounts := s.store.ManagedAccounts()
	reasons := make([]map[string]any, 0, 4)

	enabledAccounts := 0
	for _, account := range accounts {
		if account.Enabled {
			enabledAccounts++
		}
	}
	if enabledAccounts == 0 {
		reasons = append(reasons, map[string]any{
			"code":     "NO_MANAGED_ACCOUNTS",
			"severity": "info",
			"message":  "No managed broker accounts have been configured.",
		})
	}

	return map[string]any{
		"state":               onboarding,
		"shouldShowOobe":      !onboarding.Completed && len(reasons) > 0,
		"reasons":             reasons,
		"recommendedBrokerId": "futu",
		"brokers": []any{map[string]any{
			"descriptor": s.descriptor(),
			"enabled":    integration != nil && integration.Enabled,
			"available":  true,
			"configured": integration != nil,
		}},
	}
}

func (s *Server) futuOpenDInstallGuide() map[string]any {
	config := defaultFutuConfig()
	if integration := s.store.SavedIntegration(); integration != nil {
		config = integration.Config
	}
	return map[string]any{
		"brokerId":    "futu",
		"title":       "Futu OpenD",
		"description": "Configure Futu OpenD. Current market data reaches OpenD through the bbgo exchange adapter and the native API port; WebSocket settings remain available for compatibility and future push-stream support.",
		"options":     []any{},
		"nextSteps": []string{
			"确认 OpenD 已登录，并先保证 API Port 可从本机访问。",
			"保存 Host 和 API Port；WebSocket Port / Key 目前主要用于兼容配置与诊断。",
			"保存后刷新 OpenD 健康状态，确认 API 侧连接正常。",
		},
		"settings": map[string]any{
			"host": config.Host, "apiPort": config.APIPort, "websocketPort": config.WebSocketPort,
			"maxWebSocketConnections": config.MaxWebSocketConnections, "useEncryption": config.UseEncryption,
			"websocketKeyRequired": strings.TrimSpace(config.WebSocketKey) != "",
			"marketDataTransport":  liveQuoteTransportMode,
		},
	}
}

func (s *Server) brokerRuntime(ctx context.Context) map[string]any {
	integration := s.store.SavedIntegration()
	config := defaultFutuConfig()
	if integration != nil {
		config = integration.Config
	}
	if integration == nil || !integration.Enabled {
		return s.emptyBrokerRuntime(config)
	}

	probe := s.probeOpenD(ctx)
	accounts := []any{}
	if probe.Connectivity != "disconnected" {
		exchange := s.futuExchange()
		if exchange == nil {
			return s.emptyBrokerRuntime(config)
		}
		discoveredAccounts, err := exchange.DiscoverAccounts(ctx)
		if err != nil {
			if probe.LastError == nil {
				probe.LastError = new(err.Error())
			}
			if probe.Connectivity == "connected" {
				probe.Connectivity = "degraded"
				probe.Status = "degraded"
			}
		} else {
			accounts = make([]any, 0, len(discoveredAccounts))
			for _, account := range discoveredAccounts {
				accounts = append(accounts, account)
			}
		}
	}
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
	count, limit, atLimit := s.liveStreamStats()
	return map[string]any{
		"descriptor": s.descriptor(),
		"session": map[string]any{
			"brokerId":           "futu",
			"displayName":        "Futu OpenAPI via OpenD",
			"connection":         map[string]any{"host": config.Host, "apiPort": config.APIPort, "websocketPort": config.WebSocketPort, "port": config.APIPort, "useEncryption": config.UseEncryption, "marketDataTransport": liveQuoteTransportMode},
			"connectivity":       probe.Connectivity,
			"checkedAt":          probe.CheckedAt,
			"lastError":          probe.LastError,
			"globalState":        globalState,
			"accountsDiscovered": len(accounts),
			"liveWebSocketClients": map[string]any{
				"connected": count,
				"limit":     limit,
				"atLimit":   atLimit,
			},
		},
		"accounts": accounts,
	}
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func (s *Server) futuOpenDHealth(ctx context.Context) map[string]any {
	integration := s.store.SavedIntegration()
	config := defaultFutuConfig()
	if integration != nil {
		config = integration.Config
	}
	if integration == nil || !integration.Enabled {
		return s.emptyFutuOpenDHealth(config)
	}

	probe := s.probeOpenD(ctx)
	summary := any(nil)
	code := "NONE"
	manualRetry := false
	restartOpenDRecommended := false
	if probe.LastError != nil {
		summary = *probe.LastError
		code = "OPEND_API_CONNECTIVITY"
		manualRetry = true
		lower := strings.ToLower(*probe.LastError)
		restartOpenDRecommended = strings.Contains(lower, "dial") || strings.Contains(lower, "connection refused")
	}
	return map[string]any{
		"checkedAt": probe.CheckedAt,
		"status":    probe.Status,
		"runtime": map[string]any{
			"connectivity":           probe.Connectivity,
			"host":                   config.Host,
			"apiPort":                config.APIPort,
			"websocketPort":          config.WebSocketPort,
			"useEncryption":          config.UseEncryption,
			"websocketKeyConfigured": strings.TrimSpace(config.WebSocketKey) != "",
			"marketDataTransport":    liveQuoteTransportMode,
			"quoteLoggedIn":          probe.QuoteLoggedIn,
			"tradeLoggedIn":          probe.TradeLoggedIn,
			"programStatus":          probe.ProgramStatus,
			"serverVersion":          probe.ServerVersion,
			"lastError":              probe.LastError,
		},
		"diagnosis": map[string]any{
			"code": code, "summary": summary, "manualRetryRequired": manualRetry, "restartOpenDRecommended": restartOpenDRecommended,
		},
		"localSocketDiagnostics": s.liveSocketDiagnostics(config),
		"localInstallation": map[string]any{
			"platform": os.Getenv("GOOS"), "installed": false, "version": nil, "installPath": nil, "guiDetected": false,
			"process": map[string]any{"running": false, "pid": nil, "executablePath": nil},
		},
		"latestVersion":   map[string]any{"value": nil, "sourceUrl": nil, "checkedAt": nil, "status": "unknown", "error": nil},
		"recommendations": []any{},
	}
}

func (s *Server) liveSocketDiagnostics(config FutuIntegrationConfig) map[string]any {
	count, limit, atLimit := s.liveStreamStats()
	state := s.marketdataSvc.RuntimeState()
	quoteRetryAfter := state.QuoteRetryAt
	quoteFailureCount := state.QuoteFailures
	quoteLastError := state.QuoteLastError
	retryAfter := state.StreamRetryAt
	failureCount := state.StreamFailures
	lastError := state.StreamLastError
	quoteRetryAfterText, quoteBackoffActive := retryState(quoteRetryAfter)
	streamRetryAfterText, streamBackoffActive := retryState(retryAfter)
	return map[string]any{
		"transportMode":                       liveQuoteTransportMode,
		"configuredOpenDWebSocketLimit":       config.MaxWebSocketConnections,
		"configuredOpenDWebSocketLimitActive": false,
		"configuredOpenDWebSocketLimitScope":  "stored for FTWebSocket compatibility; current market-data path uses the OpenD native API via bbgo",
		"websocketEstablishedConnections":     count,
		"jftradeLiveWebSocketLimit":           limit,
		"jftradeLiveWebSocketAtLimit":         atLimit,
		"likelyConnectionSaturation":          atLimit,
		"openDWebSocketPoolLikelySaturation":  false,
		"liveQuoteBackoffActive":              quoteBackoffActive,
		"liveQuoteRetryAfter":                 quoteRetryAfterText,
		"liveQuoteFailureCount":               quoteFailureCount,
		"liveQuoteLastError":                  quoteLastError,
		"liveStreamBackoffActive":             streamBackoffActive,
		"liveStreamRetryAfter":                streamRetryAfterText,
		"liveStreamFailureCount":              failureCount,
		"liveStreamLastError":                 lastError,
		"topClientProcesses":                  []any{},
	}
}

func retryState(retryAfter time.Time) (any, bool) {
	retryAfterText := any(nil)
	backoffActive := false
	if !retryAfter.IsZero() {
		retryAfterText = retryAfter.UTC().Format(time.RFC3339Nano)
		backoffActive = time.Now().UTC().Before(retryAfter)
	}
	return retryAfterText, backoffActive
}

func (s *Server) resetFutuRuntime() {
	if s.tradingSvc != nil {
		jftradeErr1 := s.tradingSvc.StopOrderUpdates()
		jftradeLogError(jftradeErr1)
	}
	if s.marketdataSvc != nil {
		s.marketdataSvc.ResetCollector()
	}
	if s.marketdataRuntime != nil {
		s.marketdataRuntime.Reset()
	}
	if s.marketdataSvc != nil {
		s.marketdataSvc.ResumeCollector()
	}
}

func (s *Server) probeOpenD(ctx context.Context) opendProbe {
	if !s.futuIntegrationEnabled() {
		return opendProbe{}
	}
	config := s.store.Integration().Config
	checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client := opend.New(opend.Config{
		Addr:             net.JoinHostPort(config.Host, strconv.Itoa(config.APIPort)),
		TLS:              false,
		WebSocketKey:     config.WebSocketKey,
		HandshakeTimeout: 2 * time.Second,
		RequestTimeout:   3 * time.Second,
	})
	defer func() { jftradeLogError(client.Close()) }()
	if err := client.Connect(probeCtx); err != nil {
		return opendProbe{CheckedAt: checkedAt, Connectivity: "disconnected", Status: "offline", LastError: new(err.Error())}
	}

	initReq := &initpb.Request{C2S: &initpb.C2S{
		ClientVer:           new(int32(101)),
		ClientID:            new("jftrade-api"),
		RecvNotify:          new(false),
		ProgrammingLanguage: new("Go"),
	}}
	var initResp initpb.Response
	if err := client.Call(probeCtx, opend.ProtoInitConnect, initReq, &initResp); err != nil {
		return opendProbe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: new(err.Error())}
	}
	if initResp.GetRetType() != int32(commonpb.RetType_RetType_Succeed) {
		message := initResp.GetRetMsg()
		if message == "" {
			message = fmt.Sprintf("InitConnect failed: retType=%d", initResp.GetRetType())
		}
		return opendProbe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: &message}
	}

	globalReq := &globalpb.Request{C2S: &globalpb.C2S{UserID: new(uint64(0))}}
	var globalResp globalpb.Response
	if err := client.Call(probeCtx, opend.ProtoGetGlobalState, globalReq, &globalResp); err != nil {
		return opendProbe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: new(err.Error())}
	}
	if globalResp.GetRetType() != int32(commonpb.RetType_RetType_Succeed) {
		message := globalResp.GetRetMsg()
		if message == "" {
			message = fmt.Sprintf("GetGlobalState failed: retType=%d", globalResp.GetRetType())
		}
		return opendProbe{CheckedAt: checkedAt, Connectivity: "degraded", Status: "degraded", LastError: &message}
	}

	s2c := globalResp.GetS2C()
	return opendProbe{
		CheckedAt:        checkedAt,
		Connectivity:     "connected",
		Status:           "healthy",
		QuoteLoggedIn:    new(s2c.GetQotLogined()),
		TradeLoggedIn:    new(s2c.GetTrdLogined()),
		ServerVersion:    new(fmt.Sprintf("%d.%d", s2c.GetServerVer(), s2c.GetServerBuildNo())),
		ProgramStatus:    new(programStatusString(s2c.GetProgramStatus())),
		ProgramTimestamp: new(time.Unix(s2c.GetTime(), 0).UTC().Format(time.RFC3339Nano)),
		Markets: []map[string]any{
			{"market": "HK", "state": strconv.Itoa(int(s2c.GetMarketHK()))},
			{"market": "US", "state": strconv.Itoa(int(s2c.GetMarketUS()))},
			{"market": "SH", "state": strconv.Itoa(int(s2c.GetMarketSH()))},
			{"market": "SZ", "state": strconv.Itoa(int(s2c.GetMarketSZ()))},
		},
	}
}

func (s *Server) emptyBrokerRuntime(config FutuIntegrationConfig) map[string]any {
	count, limit, atLimit := s.liveStreamStats()
	return map[string]any{
		"descriptor": s.descriptor(),
		"session": map[string]any{
			"brokerId":           "futu",
			"displayName":        "Futu OpenAPI via OpenD",
			"connection":         map[string]any{"host": config.Host, "apiPort": config.APIPort, "websocketPort": config.WebSocketPort, "port": config.APIPort, "useEncryption": config.UseEncryption, "marketDataTransport": liveQuoteTransportMode},
			"connectivity":       "disconnected",
			"checkedAt":          "",
			"lastError":          nil,
			"globalState":        nil,
			"accountsDiscovered": 0,
			"liveWebSocketClients": map[string]any{
				"connected": count,
				"limit":     limit,
				"atLimit":   atLimit,
			},
		},
		"accounts": []any{},
	}
}

func (s *Server) emptyFutuOpenDHealth(config FutuIntegrationConfig) map[string]any {
	return map[string]any{
		"checkedAt": "",
		"status":    "offline",
		"runtime": map[string]any{
			"connectivity":           "disconnected",
			"host":                   config.Host,
			"apiPort":                config.APIPort,
			"websocketPort":          config.WebSocketPort,
			"useEncryption":          config.UseEncryption,
			"websocketKeyConfigured": strings.TrimSpace(config.WebSocketKey) != "",
			"marketDataTransport":    liveQuoteTransportMode,
			"quoteLoggedIn":          nil,
			"tradeLoggedIn":          nil,
			"programStatus":          nil,
			"serverVersion":          nil,
			"lastError":              nil,
		},
		"diagnosis": map[string]any{
			"code":                    "NONE",
			"summary":                 nil,
			"manualRetryRequired":     false,
			"restartOpenDRecommended": false,
		},
		"localSocketDiagnostics": s.liveSocketDiagnostics(config),
		"localInstallation": map[string]any{
			"platform": os.Getenv("GOOS"), "installed": false, "version": nil, "installPath": nil, "guiDetected": false,
			"process": map[string]any{"running": false, "pid": nil, "executablePath": nil},
		},
		"latestVersion":   map[string]any{"value": nil, "sourceUrl": nil, "checkedAt": nil, "status": "unknown", "error": nil},
		"recommendations": []any{},
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
