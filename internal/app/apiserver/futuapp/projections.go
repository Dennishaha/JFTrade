package futuapp

import (
	"context"
	"runtime"
	"strings"
	"time"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

// LiveQuoteTransportMode identifies the active market-data transport.
const LiveQuoteTransportMode = "bbgo-opend-tcp-api"

// Descriptor returns the public Futu broker descriptor.
func (c *Coordinator) Descriptor() map[string]any {
	descriptor := BrokerRuntimeDescriptor()
	return map[string]any{
		"id": descriptor.ID, "displayName": descriptor.DisplayName,
		"environments": descriptor.Environments, "capabilities": descriptor.Capabilities,
		"notes": descriptor.Notes,
	}
}

// BrokerRuntimeDescriptor returns the typed trading-service descriptor.
func BrokerRuntimeDescriptor() trdsrv.BrokerRuntimeDescriptor {
	environments := []string{"SIMULATE", "REAL"}
	return trdsrv.BrokerRuntimeDescriptor{
		ID: "futu", DisplayName: "Futu", Environments: environments,
		Capabilities: []trdsrv.BrokerMarketCapability{{
			Market: "HK", SupportsQuote: true, SupportsTrade: true,
			ReadFeatures: map[string]trdsrv.BrokerReadFeatureCapability{
				"funds":            {SupportedEnvironments: environments},
				"positions":        {SupportedEnvironments: environments},
				"orders":           {SupportedEnvironments: environments, SupportsHistory: true},
				"fills":            {SupportedEnvironments: environments, SupportsHistory: true},
				"cashFlows":        {SupportedEnvironments: []string{"REAL"}, RequiresClearingDate: true},
				"orderFees":        {SupportedEnvironments: []string{"REAL"}, RequiresOrderIDEx: true},
				"marginRatios":     {SupportedEnvironments: []string{"REAL"}, RequiresSymbols: true},
				"maxTradeQuantity": {SupportedEnvironments: environments, RequiresPrice: true},
				"orderBook": {
					SupportedEnvironments: environments,
					DefaultNum:            10, MinNum: 1, MaxNum: 50, NumPresets: []int32{5, 10, 20, 50},
					SupportsRealTimePush: true,
				},
			},
		}},
		Notes: []string{
			"Market data is exposed to the frontend through the bbgo exchange boundary.",
			"OpenD WebSocket settings are retained for compatibility and diagnostics; the current hot path uses the native API port.",
		},
	}
}

// BrokerSettings returns the settings projection consumed by the console.
func (c *Coordinator) BrokerSettings() map[string]any {
	if c == nil || c.settings == nil {
		return map[string]any{"brokers": []any{}, "accounts": []jfsettings.ManagedBrokerAccount{}}
	}
	savedIntegration := c.settings.SavedIntegration()
	defaults := c.settings.Integration().Config
	return map[string]any{
		"brokers": []any{map[string]any{
			"descriptor":  c.Descriptor(),
			"integration": savedIntegration,
			"defaults":    defaults,
		}},
		"accounts": c.settings.ManagedAccounts(),
	}
}

// OnboardingState returns onboarding readiness for the persisted state.
func (c *Coordinator) OnboardingState(ctx context.Context) map[string]any {
	onboarding := jfsettings.OnboardingSettings{}
	if c != nil && c.settings != nil {
		onboarding = c.settings.Onboarding()
	}
	return c.OnboardingStateFromSettings(ctx, onboarding)
}

// OnboardingStateFromSettings projects a supplied onboarding state.
func (c *Coordinator) OnboardingStateFromSettings(
	ctx context.Context,
	onboarding jfsettings.OnboardingSettings,
) map[string]any {
	var savedIntegration *jfsettings.BrokerIntegration
	effectiveIntegration := jfsettings.BrokerIntegration{}
	accounts := []jfsettings.ManagedBrokerAccount{}
	if c != nil && c.settings != nil {
		savedIntegration = c.settings.SavedIntegration()
		effectiveIntegration = c.settings.Integration()
		accounts = c.settings.ManagedAccounts()
	}
	reasons := make([]map[string]any, 0, 4)
	dependencyIssue := false
	dependencies := map[string]any{"allRequiredSatisfied": true}
	if c != nil && c.runtimeDependencies != nil {
		dependencies = c.runtimeDependencies(ctx)
	}
	if satisfied, _ := dependencies["allRequiredSatisfied"].(bool); !satisfied {
		dependencyIssue = true
		reasons = append(reasons, map[string]any{
			"code":     "RUNTIME_DEPENDENCY_UNSATISFIED",
			"severity": "warning",
			"message":  "Required runtime dependencies are missing or do not meet the minimum version.",
		})
	}

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
		"shouldShowOobe":      dependencyIssue || (!onboarding.Completed && len(reasons) > 0),
		"reasons":             reasons,
		"recommendedBrokerId": "futu",
		"brokers": []any{map[string]any{
			"descriptor": c.Descriptor(),
			"enabled":    effectiveIntegration.Enabled,
			"available":  true,
			"configured": savedIntegration != nil,
		}},
	}
}

// OpenDInstallGuide returns the local OpenD setup projection.
func (c *Coordinator) OpenDInstallGuide() map[string]any {
	config := jfsettings.FutuIntegrationConfig{}
	if c != nil && c.settings != nil {
		config = c.settings.Integration().Config
	}
	return map[string]any{
		"brokerId":    "futu",
		"title":       "Futu OpenD",
		"description": "Configure Futu OpenD. Current market data reaches OpenD through the bbgo exchange adapter and the native API port; WebSocket settings remain available for compatibility and future push-stream support.",
		"options":     []any{},
		"nextSteps": []string{
			"安装或升级至 Futu OpenD " + futuintegration.MinimumOpenDVersion + " 或更高版本。",
			"确认 OpenD 已登录，并先保证 API Port 可从本机访问。",
			"保存 Host 和 API Port；WebSocket Port / Key 目前主要用于兼容配置与诊断。",
			"保存后刷新 OpenD 健康状态，确认 API 侧连接正常。",
		},
		"settings": map[string]any{
			"host": config.Host, "apiPort": config.APIPort, "websocketPort": config.WebSocketPort,
			"maxWebSocketConnections": config.MaxWebSocketConnections, "useEncryption": config.UseEncryption,
			"websocketKeyRequired": strings.TrimSpace(config.WebSocketKey) != "",
			"marketDataTransport":  LiveQuoteTransportMode,
			"minimumVersion":       futuintegration.MinimumOpenDVersion,
		},
	}
}

// BrokerRuntime returns the trading service's runtime projection.
func (c *Coordinator) BrokerRuntime(ctx context.Context) *trdsrv.BrokerRuntimeResponse {
	integration := jfsettings.BrokerIntegration{}
	if c != nil && c.settings != nil {
		integration = c.settings.Integration()
	}
	config := integration.Config
	if !integration.Enabled {
		return c.emptyBrokerRuntime(config)
	}

	probe := c.Probe(ctx)
	accounts := []trdsrv.BrokerRuntimeAccount{}
	if probe.Connectivity != "disconnected" {
		active := c.Broker()
		if active == nil {
			return c.emptyBrokerRuntime(config)
		}
		discoveredAccounts, err := active.DiscoverAccounts(ctx)
		if err != nil {
			if probe.LastError == nil {
				probe.LastError = new(err.Error())
			}
			if probe.Connectivity == "connected" {
				probe.Connectivity = "degraded"
				probe.Status = "degraded"
			}
		} else {
			accounts = make([]trdsrv.BrokerRuntimeAccount, 0, len(discoveredAccounts))
			for _, account := range discoveredAccounts {
				accounts = append(accounts, trdsrv.BrokerRuntimeAccount{
					AccountID: account.ID, TradingEnvironment: account.TradingEnvironment,
					AccountType: account.AccountType, AccountRole: account.AccountRole,
					SecurityFirm: account.SecurityFirm, MarketAuthorities: account.MarketAuthorities,
					SimulatedAccountType: account.SimulatedAccountType,
				})
			}
		}
	}
	var globalState *trdsrv.BrokerRuntimeGlobalState
	if probe.QuoteLoggedIn != nil || probe.TradeLoggedIn != nil || probe.ProgramStatus != nil || probe.ServerVersion != nil {
		globalState = &trdsrv.BrokerRuntimeGlobalState{
			QuoteLoggedIn: boolValue(probe.QuoteLoggedIn), TradeLoggedIn: boolValue(probe.TradeLoggedIn),
			ServerVersion: probe.ServerVersion, ProgramStatus: probe.ProgramStatus,
			Timestamp: probe.ProgramTimestamp, Markets: probe.Markets,
		}
	}
	count, limit, atLimit := c.streamStats()
	return &trdsrv.BrokerRuntimeResponse{
		Descriptor: BrokerRuntimeDescriptor(),
		Session: trdsrv.BrokerRuntimeSession{
			BrokerID: "futu", DisplayName: "Futu",
			Connection: trdsrv.BrokerRuntimeConnection{
				Host: config.Host, APIPort: config.APIPort, WebSocketPort: config.WebSocketPort,
				Port: config.APIPort, UseEncryption: config.UseEncryption, MarketDataTransport: LiveQuoteTransportMode,
			},
			Connectivity: probe.Connectivity, CheckedAt: probe.CheckedAt, LastError: probe.LastError,
			GlobalState: globalState, AccountsDiscovered: len(accounts),
			LiveWebSocketClients: trdsrv.BrokerRuntimeLiveClients{Connected: count, Limit: limit, AtLimit: atLimit},
		},
		Accounts: accounts,
	}
}

// OpenDHealth returns the system health projection for OpenD.
func (c *Coordinator) OpenDHealth(ctx context.Context) map[string]any {
	integration := jfsettings.BrokerIntegration{}
	if c != nil && c.settings != nil {
		integration = c.settings.Integration()
	}
	config := integration.Config
	if !integration.Enabled {
		return c.emptyOpenDHealth(config)
	}

	probe := c.Probe(ctx)
	summary := any(nil)
	code := "NONE"
	manualRetry := false
	restartOpenDRecommended := false
	if probe.LastError != nil {
		summary = *probe.LastError
		code = "OPEND_API_CONNECTIVITY"
		manualRetry = true
		lower := strings.ToLower(*probe.LastError)
		if probe.IssueCode != "" {
			code = probe.IssueCode
		} else {
			restartOpenDRecommended = strings.Contains(lower, "dial") || strings.Contains(lower, "connection refused")
		}
	}
	return c.healthPayload(config, probe, code, summary, manualRetry, restartOpenDRecommended)
}

// MarketDataHealth projects the same OpenD probe used by system status into
// the broker-neutral market-data health contract.
func (c *Coordinator) MarketDataHealth(ctx context.Context) (mdsrv.HealthStatus, error) {
	enabled := c != nil && c.Enabled()
	probe := futuintegration.Probe{}
	if enabled {
		probe = c.Probe(ctx)
	}
	return marketDataHealthFromProbe(enabled, probe), nil
}

func marketDataHealthFromProbe(enabled bool, probe futuintegration.Probe) mdsrv.HealthStatus {
	health := mdsrv.HealthStatus{}
	switch {
	case !enabled:
		health.LastError = "Futu OpenD integration is disabled"
	case probe.LastError != nil:
		health.LastError = strings.TrimSpace(*probe.LastError)
	case probe.Connectivity != "connected" || probe.Status != "healthy":
		health.LastError = "Futu OpenD is not connected"
	case probe.QuoteLoggedIn == nil:
		health.LastError = "Futu OpenD quote session status is unavailable"
	case !*probe.QuoteLoggedIn:
		health.LastError = "Futu OpenD quote session is not logged in"
	default:
		health.Connected = true
	}
	return health
}

// SocketDiagnostics returns local live-stream and collector state.
func (c *Coordinator) SocketDiagnostics(config jfsettings.FutuIntegrationConfig) map[string]any {
	count, limit, atLimit := c.streamStats()
	state := c.collectorState()
	quoteRetryAfterText, quoteBackoffActive := retryState(state.QuoteRetryAt)
	streamRetryAfterText, streamBackoffActive := retryState(state.StreamRetryAt)
	return map[string]any{
		"transportMode":                       LiveQuoteTransportMode,
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
		"liveQuoteFailureCount":               state.QuoteFailures,
		"liveQuoteLastError":                  state.QuoteLastError,
		"liveStreamBackoffActive":             streamBackoffActive,
		"liveStreamRetryAfter":                streamRetryAfterText,
		"liveStreamFailureCount":              state.StreamFailures,
		"liveStreamLastError":                 state.StreamLastError,
		"topClientProcesses":                  []any{},
	}
}

func (c *Coordinator) emptyBrokerRuntime(config jfsettings.FutuIntegrationConfig) *trdsrv.BrokerRuntimeResponse {
	count, limit, atLimit := c.streamStats()
	return &trdsrv.BrokerRuntimeResponse{
		Descriptor: BrokerRuntimeDescriptor(),
		Session: trdsrv.BrokerRuntimeSession{
			BrokerID: "futu", DisplayName: "Futu",
			Connection: trdsrv.BrokerRuntimeConnection{
				Host: config.Host, APIPort: config.APIPort, WebSocketPort: config.WebSocketPort,
				Port: config.APIPort, UseEncryption: config.UseEncryption, MarketDataTransport: LiveQuoteTransportMode,
			},
			Connectivity: "disconnected", CheckedAt: "", AccountsDiscovered: 0,
			LiveWebSocketClients: trdsrv.BrokerRuntimeLiveClients{Connected: count, Limit: limit, AtLimit: atLimit},
		},
		Accounts: []trdsrv.BrokerRuntimeAccount{},
	}
}

func (c *Coordinator) emptyOpenDHealth(config jfsettings.FutuIntegrationConfig) map[string]any {
	return c.healthPayload(config, futuintegration.Probe{
		Status: "offline", Connectivity: "disconnected",
	}, "NONE", nil, false, false)
}

func (c *Coordinator) healthPayload(
	config jfsettings.FutuIntegrationConfig,
	probe futuintegration.Probe,
	code string,
	summary any,
	manualRetry bool,
	restartOpenDRecommended bool,
) map[string]any {
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
			"marketDataTransport":    LiveQuoteTransportMode,
			"quoteLoggedIn":          probe.QuoteLoggedIn,
			"tradeLoggedIn":          probe.TradeLoggedIn,
			"programStatus":          probe.ProgramStatus,
			"serverVersion":          probe.ServerVersion,
			"minimumVersion":         futuintegration.MinimumOpenDVersion,
			"lastError":              probe.LastError,
		},
		"diagnosis": map[string]any{
			"code": code, "summary": summary, "manualRetryRequired": manualRetry, "restartOpenDRecommended": restartOpenDRecommended,
		},
		"localSocketDiagnostics": c.SocketDiagnostics(config),
		"localInstallation": map[string]any{
			"platform": runtime.GOOS, "installed": false, "version": nil, "installPath": nil, "guiDetected": false,
			"process": map[string]any{"running": false, "pid": nil, "executablePath": nil},
		},
		"latestVersion":   map[string]any{"value": nil, "sourceUrl": nil, "checkedAt": nil, "status": "unknown", "error": nil},
		"recommendations": []any{},
	}
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func retryState(retryAfter time.Time) (any, bool) {
	if retryAfter.IsZero() {
		return nil, false
	}
	return retryAfter.UTC().Format(time.RFC3339Nano), time.Now().UTC().Before(retryAfter)
}
