package assembly

import (
	"context"
	"fmt"
	"time"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// ApplicationPorts are the domain services and application projections used
// by assistant tools. Providers are resolved when a tool runs, so assembly can
// be created before every facade has finished bootstrapping without retaining
// the API server composition root.
type ApplicationPorts struct {
	Runtime         func() Runtime
	System          func() *system.Service
	MarketData      func() *mdsrv.Service
	Strategy        func() *stratsrv.Service
	Trading         func() *trdsrv.Service
	Backtest        func() *btsrv.Service
	ProductFeatures func() *productsrv.Service
	Watchlist       func() *watchlist.Service

	RuntimeSettings   func() jfsettings.ADKRuntimeSettings
	ManagedAccounts   func() []jfsettings.ManagedBrokerAccount
	BrokerIntegration func() jfsettings.BrokerIntegration
	FutuOpenDHealth   func(context.Context) (any, error)
}

// ApplicationOptions combines persistent paths with the application ports
// needed to build the assistant runtime.
type ApplicationOptions struct {
	Paths Paths
	Ports ApplicationPorts
}

// OpenApplication constructs the complete assistant runtime from domain
// services. The application layer supplies dependencies but owns no ADK tool
// conversion or cross-domain query orchestration.
func OpenApplication(options ApplicationOptions) (Runtime, error) {
	var opened Runtime
	externalRuntime := options.Ports.Runtime
	options.Ports.Runtime = func() Runtime {
		if externalRuntime != nil {
			if runtime := externalRuntime(); runtime != nil {
				return runtime
			}
		}
		return opened
	}
	adapter := NewApplicationAdapter(options.Ports)
	tools := adapter.ToolDeps()
	handle, err := Open(Options{
		Paths:         options.Paths,
		RuntimeLimits: adapter.runtimeLimits,
		Tools:         &tools,
		ServiceOptions: []assistant.Option{
			assistant.WithRuntimeSettings(adapter.runtimeSettings),
			assistant.WithStreamIdleTimeout(adapter.streamIdleTimeout),
			assistant.WithOptimizationRuns(adapter.optimizationRuns()),
			assistant.WithWorkflowMarketSnapshot(adapter.WorkflowMarketSnapshot),
		},
	})
	if err != nil {
		return nil, err
	}
	opened = handle
	return handle, nil
}

// ApplicationAdapter owns the assistant-facing projections of application
// domain services.
type ApplicationAdapter struct {
	ports ApplicationPorts
}

func NewApplicationAdapter(ports ApplicationPorts) *ApplicationAdapter {
	return &ApplicationAdapter{ports: ports}
}

func (a *ApplicationAdapter) ToolDeps() ToolDeps {
	productTool := func(ctx context.Context, name string, input map[string]any) (any, error) {
		adapter := NewProductExecutionAdapter(a.productFeatures(), a.trading())
		return adapter.InvokeProductTool(ctx, name, input)
	}
	executionTool := func(ctx context.Context, name string, input map[string]any) (any, error) {
		adapter := NewProductExecutionAdapter(a.productFeatures(), a.trading())
		return adapter.InvokeExecutionTool(ctx, name, input)
	}
	watchlistList := func(ctx context.Context, input WatchlistListInput) (any, error) {
		return NewWatchlistToolAdapter(a.watchlist()).List(ctx, input)
	}
	return ToolDeps{
		Workflows:                      NewWorkflowToolManager(a.assistantService),
		SystemStatus:                   a.systemStatus,
		ADKEnabled:                     a.assistantEnabled,
		FutuOpenDHealth:                a.futuOpenDHealth,
		PluginCatalog:                  a.pluginCatalog,
		MarketSubscriptions:            a.marketSubscriptions,
		MarketSnapshot:                 a.marketSnapshot,
		MarketCandles:                  a.marketCandles,
		WatchlistList:                  watchlistList,
		ManagedAccounts:                a.managedAccounts,
		BrokerEnabled:                  a.brokerEnabled,
		DefaultTradeMarket:             a.defaultTradeMarket,
		BrokerFunds:                    a.brokerFunds,
		BrokerPositions:                a.brokerPositions,
		ExecutionOrders:                a.executionOrders,
		ExecutionOrderEvents:           a.executionOrderEvents,
		BrokerOrders:                   a.brokerOrders,
		BrokerFills:                    a.brokerFills,
		BrokerCashFlows:                a.brokerCashFlows,
		BrokerFees:                     a.brokerFees,
		BrokerMarginRatios:             a.brokerMarginRatios,
		MarketDepth:                    a.marketDepth,
		RiskState:                      a.riskState,
		RiskEvents:                     a.riskEvents,
		ListStrategyDefinitions:        a.strategyDefinitionSummaries,
		ListStrategyDefinitionVersions: a.listStrategyDefinitionVersions,
		GetStrategyDefinitionVersion:   a.getStrategyDefinitionVersion,
		ListStrategyInstances:          a.strategyInstanceSummaries,
		SaveStrategyDraft:              a.saveStrategyDraft,
		SaveStrategyDefinition:         a.saveStrategyDefinition,
		UpdateStrategyInstanceMode:     a.updateStrategyInstanceMode,
		ListBacktestRuns:               a.backtestRunSummaries,
		EnsureBacktestData:             a.ensureBacktestData,
		EnsureResearchBacktestData:     a.ensureResearchBacktestData,
		BacktestKLineSyncProgress:      a.backtestKLineSyncProgress,
		EnqueueBacktest:                a.enqueueBacktest,
		StartResearchBacktest:          a.startResearchBacktest,
		BacktestResultView:             a.backtestResultView,
		CancelBacktest:                 a.cancelBacktest,
		RecordAudit:                    a.recordAudit,
		ProductTool:                    productTool,
		ExecutionTool:                  executionTool,
	}
}

func (a *ApplicationAdapter) systemStatus() map[string]any {
	if service := a.system(); service != nil {
		return service.Status()
	}
	return map[string]any{}
}

func (a *ApplicationAdapter) assistantEnabled() bool {
	runtime := a.runtime()
	return runtime != nil && runtime.Available()
}

func (a *ApplicationAdapter) futuOpenDHealth(ctx context.Context) (any, error) {
	if a != nil && a.ports.FutuOpenDHealth != nil {
		return a.ports.FutuOpenDHealth(ctx)
	}
	return map[string]any{"status": "unavailable"}, nil
}

func (a *ApplicationAdapter) pluginCatalog() any {
	if service := a.strategy(); service != nil {
		return service.PluginCatalog()
	}
	return stratsrv.PluginCatalog{}
}

func (a *ApplicationAdapter) marketSubscriptions(ctx context.Context) (any, any, error) {
	service := a.marketData()
	if service == nil {
		return nil, nil, fmt.Errorf("market data service is unavailable")
	}
	subscriptions, err := service.GetSubscriptions(ctx)
	if err != nil {
		return nil, nil, err
	}
	active, err := service.GetActiveInstruments(ctx)
	return subscriptions, active, err
}

func (a *ApplicationAdapter) marketSnapshot(ctx context.Context, market string, symbol string) (any, error) {
	service := a.marketData()
	if service == nil {
		return nil, fmt.Errorf("market data service is unavailable")
	}
	response, err := service.GetSnapshot(ctx, market, symbol, false)
	return map[string]any(response), err
}

func (a *ApplicationAdapter) marketCandles(
	ctx context.Context,
	market string,
	symbol string,
	period string,
	limit int,
) (any, error) {
	service := a.marketData()
	if service == nil {
		return nil, fmt.Errorf("market data service is unavailable")
	}
	response, err := service.GetCandles(ctx, mdsrv.HistoricalCandlesQuery{
		Market: market, Symbol: symbol, Period: period, Limit: limit,
	})
	return map[string]any(response), err
}

func (a *ApplicationAdapter) marketDepth(ctx context.Context, market string, symbol string, num int) (any, error) {
	service := a.marketData()
	if service == nil {
		return nil, fmt.Errorf("market data service is unavailable")
	}
	response, err := service.GetDepth(ctx, market, symbol, num)
	return map[string]any(response), err
}

func (a *ApplicationAdapter) managedAccounts() any {
	if a != nil && a.ports.ManagedAccounts != nil {
		return a.ports.ManagedAccounts()
	}
	return []jfsettings.ManagedBrokerAccount{}
}

func (a *ApplicationAdapter) brokerEnabled() bool {
	if a == nil || a.ports.BrokerIntegration == nil {
		return false
	}
	return a.ports.BrokerIntegration().Enabled
}

func (a *ApplicationAdapter) defaultTradeMarket() string {
	if a == nil || a.ports.BrokerIntegration == nil {
		return ""
	}
	return a.ports.BrokerIntegration().Config.TradeMarket
}

func (a *ApplicationAdapter) brokerFunds(ctx context.Context, query broker.ReadQuery, timeout time.Duration) any {
	if service := a.trading(); service != nil {
		return service.FundsWithTimeout(ctx, query, timeout)
	}
	return nil
}

func (a *ApplicationAdapter) brokerPositions(ctx context.Context, query broker.ReadQuery, timeout time.Duration) any {
	if service := a.trading(); service != nil {
		return service.PositionsWithTimeout(ctx, query, timeout)
	}
	return nil
}

func (a *ApplicationAdapter) riskState() any {
	if service := a.system(); service != nil {
		return map[string]any{
			"killSwitch": service.RealTradeKillSwitch(),
			"riskLimits": service.RealTradeRiskLimits(),
			"checkedAt":  time.Now().UTC().Format(time.RFC3339Nano),
		}
	}
	return map[string]any{}
}

func (a *ApplicationAdapter) riskEvents() any {
	if service := a.system(); service != nil {
		return service.RealTradeRiskEvents()
	}
	return nil
}

func (a *ApplicationAdapter) recordAudit(
	ctx context.Context,
	kind string,
	subjectID string,
	detail string,
	metadata map[string]any,
) {
	if runtime := a.runtime(); runtime != nil {
		runtime.RecordAudit(ctx, kind, subjectID, detail, metadata)
	}
}

func (a *ApplicationAdapter) WorkflowMarketSnapshot(ctx context.Context, instrumentID string) (map[string]any, error) {
	service := a.marketData()
	if service == nil {
		return nil, fmt.Errorf("market data service is unavailable")
	}
	market, symbol, ok := splitWorkflowInstrumentID(instrumentID)
	if !ok {
		return nil, fmt.Errorf("invalid instrumentId %q", instrumentID)
	}
	snapshot, err := service.GetSnapshot(ctx, market, symbol, false)
	return map[string]any(snapshot), err
}

func (a *ApplicationAdapter) runtimeSettings() any {
	if a != nil && a.ports.RuntimeSettings != nil {
		return a.ports.RuntimeSettings()
	}
	return jfsettings.ADKRuntimeSettings{}
}

func (a *ApplicationAdapter) streamIdleTimeout() int {
	if a != nil && a.ports.RuntimeSettings != nil {
		return a.ports.RuntimeSettings().StreamIdleTimeoutMs
	}
	return 0
}

func (a *ApplicationAdapter) runtimeLimits() jfadkmodel.RuntimeLimits {
	if a == nil || a.ports.RuntimeSettings == nil {
		return jfadkmodel.RuntimeLimits{}
	}
	return jfadkmodel.RuntimeLimits{RunTimeout: time.Duration(a.ports.RuntimeSettings().RunTimeoutMs) * time.Millisecond}
}

func (a *ApplicationAdapter) assistantService() *assistant.Service {
	if runtime := a.runtime(); runtime != nil {
		return runtime.Service()
	}
	return nil
}

func (a *ApplicationAdapter) runtime() Runtime {
	if a != nil && a.ports.Runtime != nil {
		return a.ports.Runtime()
	}
	return nil
}

func (a *ApplicationAdapter) system() *system.Service {
	if a != nil && a.ports.System != nil {
		return a.ports.System()
	}
	return nil
}

func (a *ApplicationAdapter) marketData() *mdsrv.Service {
	if a != nil && a.ports.MarketData != nil {
		return a.ports.MarketData()
	}
	return nil
}

func (a *ApplicationAdapter) strategy() *stratsrv.Service {
	if a != nil && a.ports.Strategy != nil {
		return a.ports.Strategy()
	}
	return nil
}

func (a *ApplicationAdapter) trading() *trdsrv.Service {
	if a != nil && a.ports.Trading != nil {
		return a.ports.Trading()
	}
	return nil
}

func (a *ApplicationAdapter) backtest() *btsrv.Service {
	if a != nil && a.ports.Backtest != nil {
		return a.ports.Backtest()
	}
	return nil
}

func (a *ApplicationAdapter) productFeatures() *productsrv.Service {
	if a != nil && a.ports.ProductFeatures != nil {
		return a.ports.ProductFeatures()
	}
	return nil
}

func (a *ApplicationAdapter) watchlist() *watchlist.Service {
	if a != nil && a.ports.Watchlist != nil {
		return a.ports.Watchlist()
	}
	return nil
}
