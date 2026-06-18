package servercore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	asst "github.com/jftrade/jftrade-main/internal/assistant"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/broker"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func newADKRuntime(server *Server, settingsPath string) *jfadk.Runtime {
	return NewADKRuntime(settingsPath, RuntimeDeps{
		RuntimeLimits: func() jfadk.RuntimeLimits {
			if server == nil || server.store == nil {
				return jfadk.RuntimeLimits{}
			}
			settings := server.store.ADKSettings()
			return jfadk.RuntimeLimits{RunTimeout: time.Duration(settings.RunTimeoutMs) * time.Millisecond}
		},
		Tools: server.adkToolDeps(),
	})
}

func (s *Server) adkToolDeps() ToolDeps {
	return ToolDeps{
		SystemStatus: func() map[string]any { return s.sysSvc.Status() },
		ADKEnabled:   func() bool { return s != nil && s.adkRuntime != nil },
		FutuOpenDHealth: func(ctx context.Context) (any, error) {
			return s.futuOpenDHealth(ctx), nil
		},
		PluginCatalog: func() any { return s.strategySvc.PluginCatalog() },
		MarketSubscriptions: func(ctx context.Context) (any, any, error) {
			subscriptions, err := s.marketdataSvc.GetSubscriptions(ctx)
			if err != nil {
				return nil, nil, err
			}
			activeInstruments, err := s.marketdataSvc.GetActiveInstruments(ctx)
			return subscriptions, activeInstruments, err
		},
		MarketSnapshot: func(ctx context.Context, market string, symbol string) (any, error) {
			return s.marketSnapshotResponseForInstrument(ctx, market, symbol, marketSnapshotQuery{Refresh: newOptionalBoolValue(false)})
		},
		MarketCandles: func(ctx context.Context, market string, symbol string, period string, limit int) (any, error) {
			return s.marketCandlesResponseForInstrument(ctx, market, symbol, marketCandlesQuery{Period: candlePeriodValue(period), Limit: newOptionalIntValue(limit)})
		},
		ManagedAccounts:    func() any { return s.store.ManagedAccounts() },
		BrokerEnabled:      func() bool { return s.futuIntegrationEnabled() },
		DefaultTradeMarket: func() string { return s.store.Integration().Config.TradeMarket },
		BrokerFunds: func(ctx context.Context, query broker.ReadQuery, timeout time.Duration) any {
			return s.tradingSvc.FundsWithTimeout(ctx, query, timeout)
		},
		BrokerPositions: func(ctx context.Context, query broker.ReadQuery, timeout time.Duration) any {
			return s.tradingSvc.PositionsWithTimeout(ctx, query, timeout)
		},
		ExecutionOrders: func() any {
			orders, err := s.tradingSvc.ExecutionOrdersSnapshot(context.Background())
			if err != nil {
				return []trdsrv.ExecutionOrder{}
			}
			return orders.Orders
		},
		ExecutionOrderEvents: func(internalOrderID string) any {
			events, err := s.tradingSvc.ExecutionOrderEvents(context.Background(), internalOrderID)
			if err != nil {
				return trdsrv.ExecutionOrderEvents{InternalOrderID: internalOrderID}
			}
			return events
		},
		BrokerOrders: func(ctx context.Context, input BrokerReadInput) (any, error) {
			scope, err := normalizeTradingBrokerScope(input.Scope)
			if err != nil {
				return nil, err
			}
			return s.tradingSvc.Orders(ctx, trdsrv.OrdersQuery{
				ReadQuery: brokerReadQueryFromADK(s.tradingSvc, input),
				Scope:     scope,
				Symbol:    strings.TrimSpace(input.Symbol),
				StartTime: strings.TrimSpace(input.StartTime),
				EndTime:   strings.TrimSpace(input.EndTime),
				Statuses:  mergeADKBrokerValues(input.Status, input.Statuses),
			})
		},
		BrokerFills: func(ctx context.Context, input BrokerReadInput) (any, error) {
			scope, err := normalizeTradingBrokerScope(input.Scope)
			if err != nil {
				return nil, err
			}
			return s.tradingSvc.Fills(ctx, trdsrv.FillsQuery{
				ReadQuery: brokerReadQueryFromADK(s.tradingSvc, input),
				Scope:     scope,
				Symbol:    strings.TrimSpace(input.Symbol),
				StartTime: strings.TrimSpace(input.StartTime),
				EndTime:   strings.TrimSpace(input.EndTime),
			})
		},
		BrokerCashFlows: func(ctx context.Context, input BrokerReadInput) (any, error) {
			clearingDate := strings.TrimSpace(input.ClearingDate)
			if clearingDate == "" {
				return nil, fmt.Errorf("query parameter clearingDate is required")
			}
			return s.tradingSvc.CashFlows(ctx, broker.CashFlowQuery{
				ReadQuery:    brokerReadQueryFromADK(s.tradingSvc, input),
				ClearingDate: clearingDate,
				Direction:    strings.TrimSpace(input.Direction),
			})
		},
		BrokerFees: func(ctx context.Context, input BrokerReadInput) (any, error) {
			orderIDs := mergeADKBrokerValues(input.OrderIDEx, input.OrderIDExList)
			if len(orderIDs) == 0 {
				return nil, fmt.Errorf("query parameter orderIdEx is required")
			}
			return s.tradingSvc.OrderFees(ctx, broker.OrderFeeQuery{
				ReadQuery:     brokerReadQueryFromADK(s.tradingSvc, input),
				OrderIDExList: orderIDs,
			})
		},
		BrokerMarginRatios: func(ctx context.Context, input BrokerReadInput) (any, error) {
			readQuery := brokerReadQueryFromADK(s.tradingSvc, input)
			symbols, err := trdsrv.NormalizeSymbols(readQuery.Market, input.Symbols)
			if err != nil {
				return nil, err
			}
			if len(symbols) == 0 {
				return nil, fmt.Errorf("query parameter symbol is required")
			}
			return s.tradingSvc.MarginRatios(ctx, broker.MarginRatioQuery{
				ReadQuery: readQuery,
				Symbols:   symbols,
			})
		},
		MarketDepth: func(ctx context.Context, market string, symbol string, num int) (any, error) {
			return s.marketDepthResponseForInstrument(ctx, market, symbol, marketDepthQuery{Num: newOptionalIntValue(num)})
		},
		RiskState: func() any {
			return map[string]any{
				"killSwitch": s.sysSvc.RealTradeKillSwitch(),
				"riskLimits": s.sysSvc.RealTradeRiskLimits(),
				"checkedAt":  time.Now().UTC().Format(time.RFC3339Nano),
			}
		},
		RiskEvents:              func() any { return s.sysSvc.RealTradeRiskEvents() },
		ListStrategyDefinitions: s.adkStrategyDefinitionSummaries,
		ListStrategyInstances:   s.adkStrategyInstanceSummaries,
		SaveStrategyDraft:       s.adkSaveStrategyDraft,
		SaveStrategyDefinition:  s.adkSaveStrategyDefinition,
		UpdateStrategyInstanceMode: func(instanceID string, executionMode string) (any, error) {
			current, ok := s.strategySvc.GetInstance(instanceID)
			if !ok {
				return nil, fmt.Errorf("策略实例 %q 不存在", instanceID)
			}
			binding := current.Binding
			binding.ExecutionMode = executionMode
			return s.strategySvc.UpdateInstance(instanceID, binding)
		},
		ListBacktestRuns: s.adkBacktestRunSummaries,
		EnqueueBacktest: func(input BacktestStartInput) (BacktestRunRef, error) {
			run, err := s.backtestSvc.Start(context.Background(), btsrv.StartRequest{
				DefinitionID:   input.DefinitionID,
				Market:         input.Market,
				Symbol:         input.Symbol,
				Code:           input.Code,
				Interval:       input.Interval,
				StartTime:      input.StartTime,
				EndTime:        input.EndTime,
				InitialBalance: input.InitialBalance,
				RehabType:      input.RehabType,
			})
			if err != nil {
				return BacktestRunRef{}, err
			}
			return BacktestRunRef{ID: run.ID, Status: run.Status}, nil
		},
		StartResearchBacktest: func(input ResearchBacktestInput) (BacktestRunSummary, error) {
			run, err := s.backtestSvc.StartScript(context.Background(), btsrv.ScriptStartRequest{
				Script:           input.Script,
				Market:           input.Market,
				Symbol:           input.Symbol,
				Code:             input.Code,
				Interval:         input.Interval,
				StartTime:        input.StartTime,
				EndTime:          input.EndTime,
				InitialBalance:   input.InitialBalance,
				RehabType:        input.RehabType,
				UseExtendedHours: input.UseExtendedHours,
			})
			if err != nil {
				return BacktestRunSummary{}, err
			}
			return backtestRunSummaryFromSrvRun(run), nil
		},
		BacktestResultView: func(input BacktestResultViewInput) (any, error) {
			return s.backtestSvc.ResultView(btsrv.ResultViewRequest{
				RunID:      input.RunID,
				View:       input.View,
				Resolution: input.Resolution,
				StartTime:  input.StartTime,
				EndTime:    input.EndTime,
				Include:    append([]string(nil), input.Include...),
				Limit:      input.Limit,
				Cursor:     input.Cursor,
			})
		},
		CancelBacktest: func(runID string) { s.backtestSvc.Cancel(runID) },
		RecordAudit: func(ctx context.Context, kind string, subjectID string, detail string, metadata map[string]any) {
			if s != nil && s.adkRuntime != nil {
				s.adkRuntime.RecordAudit(ctx, kind, subjectID, detail, metadata)
			}
		},
	}
}

func brokerReadQueryFromADK(service *trdsrv.Service, input BrokerReadInput) broker.ReadQuery {
	return service.ReadQuery("futu", input.TradingEnvironment, input.AccountID, input.Market)
}

func normalizeTradingBrokerScope(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "CURRENT":
		return "CURRENT", nil
	case "HISTORY":
		return "HISTORY", nil
	default:
		return "", fmt.Errorf("query parameter scope is invalid")
	}
}

func mergeADKBrokerValues(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var values []string
	for _, group := range groups {
		for _, raw := range group {
			for _, part := range strings.Split(raw, ",") {
				value := strings.TrimSpace(part)
				key := strings.ToUpper(value)
				if value == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				values = append(values, value)
			}
		}
	}
	return values
}

func (s *Server) adkStrategyDefinitionSummaries() []StrategyDefinitionSummary {
	definitions := s.strategySvc.ListDefinitions()
	out := make([]StrategyDefinitionSummary, 0, len(definitions))
	for _, definition := range definitions {
		summary := StrategyDefinitionSummary{
			ID: definition.ID, Name: definition.Name, Version: definition.Version, Description: definition.Description,
			Runtime: definition.Runtime, SourceFormat: definition.SourceFormat, Symbol: definition.Symbol, Interval: definition.Interval,
			Script: definition.Script, CreatedAt: definition.CreatedAt, UpdatedAt: definition.UpdatedAt,
		}
		if definition.VisualModel != nil {
			summary.VisualNodeCount = len(definition.VisualModel.Nodes)
			summary.VisualEdgeCount = len(definition.VisualModel.Edges)
		}
		out = append(out, summary)
	}
	return out
}

func (s *Server) adkStrategyInstanceSummaries() []StrategyInstanceSummary {
	items := s.strategySvc.ListInstances()
	out := make([]StrategyInstanceSummary, 0, len(items))
	for _, item := range items {
		definitionID := strings.TrimSpace(item.Definition.StrategyID)
		if definitionID == "" {
			definitionID = strategyDefinitionIDFromParams(item.Params)
		}
		activeSymbols := []string{}
		actualStatus := ""
		lastError := ""
		if item.RuntimeObservation != nil {
			activeSymbols = append(activeSymbols, item.RuntimeObservation.ActiveSymbols...)
			actualStatus = strings.TrimSpace(item.RuntimeObservation.ActualStatus)
			if item.RuntimeObservation.LastError != nil {
				lastError = strings.TrimSpace(*item.RuntimeObservation.LastError)
			}
		}
		lastLog := ""
		if len(item.Logs) > 0 {
			lastLog = strings.TrimSpace(item.Logs[len(item.Logs)-1])
		}
		out = append(out, StrategyInstanceSummary{
			ID: item.ID, DefinitionID: definitionID, DefinitionName: item.Definition.Name, DefinitionVersion: item.Definition.Version,
			Runtime: item.Runtime, SourceFormat: item.SourceFormat, Status: item.Status, ActualStatus: actualStatus, Startable: item.Startable,
			Symbols: append([]string(nil), item.Binding.Symbols...), ActiveSymbols: activeSymbols, Interval: item.Binding.Interval,
			ExecutionMode: item.Binding.ExecutionMode, Market: brokerBindingMarket(item.Binding.BrokerAccount), AccountID: brokerBindingAccountID(item.Binding.BrokerAccount),
			CreatedAt: item.CreatedAt, LogCount: len(item.Logs), LatestLog: lastLog, LastError: lastError,
		})
	}
	return out
}

func (s *Server) adkSaveStrategyDraft(input StrategyDraftInput) (any, error) {
	if s == nil || s.strategySvc == nil {
		return nil, fmt.Errorf("策略定义存储不可用")
	}
	definition := strategyDesignDefinition{
		Name:         stringOrDefault(input.Name, "ADK 策略草稿"),
		Description:  "由 ADK agent 生成的策略草稿。",
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Runtime:      strategyRuntimePinePlan,
		Version:      defaultStrategyVersion,
		Symbol:       strings.TrimSpace(input.Validation.Program.Metadata.Symbol),
		Interval:     strings.TrimSpace(input.Validation.Program.Metadata.Interval),
		Script:       input.Validation.NormalizedScript,
	}
	return s.strategySvc.SaveDefinition(definition)
}

func (s *Server) adkSaveStrategyDefinition(input StrategyDefinitionInput) (any, error) {
	if s == nil || s.strategySvc == nil {
		return nil, fmt.Errorf("策略定义存储不可用")
	}
	definitionID := strings.TrimSpace(input.DefinitionID)
	if definitionID != "" {
		if _, ok, err := s.strategySvc.GetDefinition(definitionID); err != nil {
			return nil, err
		} else if !ok {
			return nil, fmt.Errorf("策略定义 %q 不存在", definitionID)
		}
	}
	visualModel, err := strategyVisualModelFromInput(input.VisualModel)
	if err != nil {
		return nil, err
	}
	definition := strategyDesignDefinition{
		ID:           definitionID,
		Name:         strings.TrimSpace(input.Name),
		Description:  strings.TrimSpace(input.Description),
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       stringOrDefault(strings.TrimSpace(input.Symbol), strings.TrimSpace(input.Validation.Program.Metadata.Symbol)),
		Interval:     stringOrDefault(strings.TrimSpace(input.Interval), strings.TrimSpace(input.Validation.Program.Metadata.Interval)),
		Script:       input.Validation.NormalizedScript,
		VisualModel:  visualModel,
	}
	return s.strategySvc.SaveDefinition(definition)
}

func (s *Server) adkBacktestRunSummaries() []BacktestRunSummary {
	runs := s.backtestSvc.ListFull()
	out := make([]BacktestRunSummary, 0, len(runs))
	for _, run := range runs {
		if run == nil {
			continue
		}
		out = append(out, backtestRunSummaryFromSrvRun(run))
	}
	return out
}

func backtestRunSummaryFromSrvRun(run *btsrv.RunState) BacktestRunSummary {
	if run == nil {
		return BacktestRunSummary{}
	}
	return BacktestRunSummary{
		ID:                run.ID,
		Status:            run.Status,
		DefinitionID:      run.Request.DefinitionID,
		DefinitionVersion: run.Request.DefinitionVersion,
		Market:            run.Request.Market,
		Code:              run.Request.Code,
		Symbol:            run.Request.Symbol,
		Interval:          run.Request.Interval,
		StartTime:         run.Request.StartTime,
		EndTime:           run.Request.EndTime,
		InitialBalance:    run.Request.InitialBalance,
		RehabType:         run.Request.RehabType,
		UseExtendedHours:  run.Request.UseExtendedHours,
		Result:            run.Result,
		CreatedAt:         run.CreatedAt,
		UpdatedAt:         run.UpdatedAt,
	}
}

func strategyVisualModelFromInput(value any) (*strategyVisualModel, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("visualModel must be a valid object: %w", err)
	}
	var model strategyVisualModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("visualModel must be a valid object: %w", err)
	}
	return normalizeStrategyVisualModel(&model)
}

func validateADKStrategyDraftScript(script string) error {
	return ValidateADKStrategyDraftScript(script)
}

func brokerBindingMarket(binding *strategyBrokerAccountBinding) string {
	if binding == nil {
		return ""
	}
	return binding.Market
}

func brokerBindingAccountID(binding *strategyBrokerAccountBinding) string {
	if binding == nil {
		return ""
	}
	return binding.AccountID
}

type assistantOptimizationRuns struct {
	server *Server
}

func (a assistantOptimizationRuns) Get(runID string) (asst.OptimizationRun, bool) {
	if a.server == nil || a.server.backtestSvc == nil {
		return asst.OptimizationRun{}, false
	}
	run, ok, err := a.server.backtestSvc.GetResult(runID)
	if err != nil || !ok {
		return asst.OptimizationRun{}, false
	}
	return asst.OptimizationRun{Status: run.Status, Result: run.Result}, true
}

func (a assistantOptimizationRuns) Cancel(runID string) {
	if a.server != nil && a.server.backtestSvc != nil {
		a.server.backtestSvc.Cancel(runID)
	}
}
