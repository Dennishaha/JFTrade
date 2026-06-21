package servercore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/broker"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

type RuntimeDeps struct {
	RuntimeLimits func() jfadk.RuntimeLimits
	Tools         ToolDeps
}

type ToolDeps struct {
	SystemStatus               func() map[string]any
	ADKEnabled                 func() bool
	FutuOpenDHealth            func(context.Context) (any, error)
	PluginCatalog              func() any
	MarketSubscriptions        func(context.Context) (subscriptions any, activeInstruments any, err error)
	MarketSnapshot             func(context.Context, string, string) (any, error)
	MarketCandles              func(context.Context, string, string, string, int) (any, error)
	ManagedAccounts            func() any
	BrokerEnabled              func() bool
	DefaultTradeMarket         func() string
	BrokerFunds                func(context.Context, broker.ReadQuery, time.Duration) any
	BrokerPositions            func(context.Context, broker.ReadQuery, time.Duration) any
	ExecutionOrders            func() any
	ExecutionOrderEvents       func(string) any
	BrokerOrders               func(context.Context, BrokerReadInput) (any, error)
	BrokerFills                func(context.Context, BrokerReadInput) (any, error)
	BrokerCashFlows            func(context.Context, BrokerReadInput) (any, error)
	BrokerFees                 func(context.Context, BrokerReadInput) (any, error)
	BrokerMarginRatios         func(context.Context, BrokerReadInput) (any, error)
	MarketDepth                func(context.Context, string, string, int) (any, error)
	RiskState                  func() any
	RiskEvents                 func() any
	ListStrategyDefinitions    func() []StrategyDefinitionSummary
	ListStrategyInstances      func() []StrategyInstanceSummary
	SaveStrategyDraft          func(StrategyDraftInput) (any, error)
	SaveStrategyDefinition     func(StrategyDefinitionInput) (any, error)
	UpdateStrategyInstanceMode func(instanceID string, executionMode string) (any, error)
	ListBacktestRuns           func() []BacktestRunSummary
	EnqueueBacktest            func(BacktestStartInput) (BacktestRunRef, error)
	StartResearchBacktest      func(ResearchBacktestInput) (BacktestRunSummary, error)
	BacktestResultView         func(BacktestResultViewInput) (any, error)
	CancelBacktest             func(string)
	RecordAudit                func(context.Context, string, string, string, map[string]any)
}

type BrokerReadInput struct {
	TradingEnvironment string
	AccountID          string
	Market             string
	Scope              string
	Symbol             string
	Symbols            []string
	StartTime          string
	EndTime            string
	Status             []string
	Statuses           []string
	ClearingDate       string
	Direction          string
	OrderIDEx          []string
	OrderIDExList      []string
}

type StrategyDefinitionSummary struct {
	ID, Name, Version, Description, Runtime, SourceFormat, Symbol, Interval, Script, CreatedAt, UpdatedAt string
	VisualNodeCount, VisualEdgeCount                                                                      int
}

type StrategyInstanceSummary struct {
	ID, DefinitionID, DefinitionName, DefinitionVersion, Runtime, SourceFormat, Status, ActualStatus string
	Startable                                                                                        bool
	Symbols, ActiveSymbols                                                                           []string
	Interval, ExecutionMode, Market, AccountID, CreatedAt, LatestLog, LastError                      string
	LogCount                                                                                         int
}

type StrategyDraftInput struct {
	Name, Script string
	Validation   StrategyPineValidation
}

type StrategyDefinitionInput struct {
	DefinitionID, Name, Description, Symbol, Interval string
	VisualModel                                       any
	Validation                                        StrategyPineValidation
}

type BacktestStartInput struct {
	DefinitionID, Market, Symbol, Code, Interval, StartDate, EndDate, StartTime, EndTime, RehabType string
	InitialBalance                                                                                  float64
}

type ResearchBacktestInput struct {
	Script, Market, Symbol, Code, Interval, StartDate, EndDate, StartTime, EndTime, RehabType string
	InitialBalance                                                                            float64
	UseExtendedHours                                                                          *bool
}

type BacktestResultViewInput struct {
	RunID, View, Resolution, StartTime, EndTime, Cursor string
	Include                                             []string
	Limit                                               int
}

type BacktestRunRef struct {
	ID, Status string
}

type BacktestRunSummary struct {
	ID, Status, DefinitionID, DefinitionVersion, Market, Code, Symbol, Interval             string
	StartDate, EndDate, StartTime, EndTime, MarketTimezone, RehabType, CreatedAt, UpdatedAt string
	InitialBalance                                                                          float64
	UseExtendedHours                                                                        *bool
	Result                                                                                  *backtest.RunResult
}

func NewADKRuntime(settingsPath string, deps RuntimeDeps) *jfadk.Runtime {
	dbPath := apiruntime.DeriveADKDBPath(settingsPath)
	sessionDBPath := apiruntime.DeriveADKSessionDBPath(settingsPath)
	store, err := jfadk.NewStore(dbPath, apiruntime.DeriveADKSecretsPath(settingsPath), apiruntime.DeriveADKSkillsDir(settingsPath))
	if err != nil {
		log.Printf("JFTrade ADK runtime degraded: %v", err)
		return nil
	}
	registry := jfadk.NewToolRegistry()
	RegisterJFTradeADKTools(store, registry, deps.Tools)
	sessionService, err := jfadk.NewSQLiteSessionService(sessionDBPath)
	if err != nil {
		log.Printf("JFTrade ADK session store degraded: %v", err)
		_ = store.Close()
		return nil
	}
	runtime := jfadk.NewRuntimeWithSessionService(store, registry, sessionService)
	configureADKRuntime(runtime, deps)
	return runtime
}

func configureADKRuntime(runtime *jfadk.Runtime, deps RuntimeDeps) {
	if runtime != nil && deps.RuntimeLimits != nil {
		runtime.SetRuntimeLimitsProvider(deps.RuntimeLimits)
	}
}

func RegisterJFTradeADKTools(store *jfadk.Store, registry *jfadk.ToolRegistry, deps ToolDeps) {
	registry.Register(jfadk.ToolDescriptor{Name: "system.status", DisplayName: "系统状态", Description: "读取 JFTrade API、持久层、broker、策略运行时和 ADK 状态摘要。", Category: "system", Permission: "read_internal", OutputSummary: "系统健康、持久化、broker、策略运行时与 ADK 状态。"}, func(context.Context, map[string]any) (any, error) {
		status := callMap(deps.SystemStatus)
		if callBool(deps.ADKEnabled) {
			status["adk"] = map[string]any{"module": jfadk.GoogleADKModule, "enabled": true}
		}
		return status, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "system.futu_opend", DisplayName: "OpenD 健康", Description: "读取 Futu OpenD 连通性、登录态与诊断。", Category: "system", Permission: "read_internal", OutputSummary: "OpenD 连接、登录态、配置和诊断信息。"}, func(ctx context.Context, _ map[string]any) (any, error) {
		return deps.FutuOpenDHealth(ctx)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "plugins.catalog", DisplayName: "策略插件目录", Description: "读取现有策略插件安装状态。", Category: "system", Permission: "read_internal", OutputSummary: "策略插件目录与安装状态。"}, func(context.Context, map[string]any) (any, error) {
		return deps.PluginCatalog(), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "market.subscriptions", DisplayName: "行情订阅", Description: "读取当前行情订阅和配额摘要。", Category: "market", Permission: "read_internal", OutputSummary: "当前订阅、活跃标的和检查时间。"}, func(ctx context.Context, _ map[string]any) (any, error) {
		subscriptions, activeInstruments, err := deps.MarketSubscriptions(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"subscriptions": subscriptions, "activeInstruments": activeInstruments, "checkedAt": nowStringRFC3339Nano()}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "market.snapshot", DisplayName: "行情快照", Description: "读取当前工作问题中指定标的的行情快照；未指定时返回可用说明。", Category: "market", Permission: "read_internal", OutputSummary: "单个标的的行情快照或缺少标的提示。"}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		return deps.MarketSnapshot(ctx, market, symbol)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "market.candles", DisplayName: "K 线查询", Description: "读取指定标的近期 K 线；未指定时返回使用说明。", Category: "market", Permission: "read_internal", OutputSummary: "近期 1m K 线，默认最多 50 根。"}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		period := stringOrDefault(stringValue(input, "period"), "1m")
		limit := intValue(input, "limit", 50)
		if limit < 1 || limit > 500 {
			return nil, fmt.Errorf("limit must be between 1 and 500")
		}
		normalizedPeriod, err := httpserver.NormalizeCandlePeriod(period)
		if err != nil {
			return nil, err
		}
		return deps.MarketCandles(ctx, market, symbol, normalizedPeriod, limit)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "portfolio.summary", DisplayName: "组合摘要", Description: "读取托管账户、资金、订单和持仓的控制台摘要。", Category: "portfolio", Permission: "read_internal", OutputSummary: "托管账户、broker 状态、执行订单摘要和当前检查时间。"}, func(ctx context.Context, input map[string]any) (any, error) {
		query := broker.ReadQuery{
			BrokerID:           "futu",
			AccountID:          strings.TrimSpace(stringValue(input, "accountId")),
			TradingEnvironment: strings.ToUpper(strings.TrimSpace(stringValue(input, "tradingEnvironment"))),
			Market:             strings.ToUpper(stringOrDefault(stringValue(input, "market"), deps.DefaultTradeMarket())),
		}
		orders := deps.ExecutionOrders()
		return map[string]any{"accounts": deps.ManagedAccounts(), "brokerEnabled": deps.BrokerEnabled(), "orders": orders, "orderCount": collectionLen(orders), "funds": deps.BrokerFunds(ctx, query, 8*time.Second), "positions": deps.BrokerPositions(ctx, query, 8*time.Second), "checkedAt": nowStringRFC3339Nano()}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "account.orders", DisplayName: "订单摘要", Description: "读取执行订单视图摘要。", Category: "portfolio", Permission: "read_internal", OutputSummary: "执行订单列表和数量。"}, func(context.Context, map[string]any) (any, error) {
		orders := deps.ExecutionOrders()
		return map[string]any{"orders": orders, "count": collectionLen(orders), "checkedAt": nowStringRFC3339Nano()}, nil
	})
	registerJFTradeADKWorkflowTools(store, registry, deps)
	registerJFTradeADKReadTools(registry, deps)
	registerJFTradeADKStrategyTools(store, registry, deps)
}

func registerJFTradeADKStrategyTools(store *jfadk.Store, registry *jfadk.ToolRegistry, deps ToolDeps) {
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.definitions", DisplayName: "策略定义", Description: "读取当前策略定义和策略实例摘要。", Category: "strategy", Permission: "read_internal", OutputSummary: "策略定义、运行实例和数量摘要。"}, func(context.Context, map[string]any) (any, error) {
		return SummarizeADKStrategyDefinitions(deps.ListStrategyDefinitions(), deps.ListStrategyInstances()), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: strategypinespec.ToolName, DisplayName: "Pine 定义", Description: "读取当前 JFTrade Pine Script v6 的结构化定义、最小骨架、支持清单和示例。", Category: "strategy", Permission: "read_internal", OutputSummary: "JFTrade Pine Script v6 的章节摘要、支持语法与可选示例。"}, func(_ context.Context, input map[string]any) (any, error) {
		return StrategyPineSpecToolPayload(input)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.validate_pine", DisplayName: "校验 Pine", Description: "校验 Pine Script v6 是否可被当前 parser、lowerer、planner 和 runtime 接受，并返回结构化元数据、warnings 与 requirements。", Category: "strategy", Permission: "read_internal", OutputSummary: "校验结果、元数据、hooks、warnings、编译后的 requirements，以及失败时的保存提示。"}, func(_ context.Context, input map[string]any) (any, error) {
		return StrategyValidatePineToolPayload(input), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.research_backtest", DisplayName: "策略研究回测", Description: "用临时 Pine Script v6 脚本进行研究回测；会先校验脚本并启动临时回测，但不会保存策略草稿或定义。回测运行和结果会保留供后续查询。", Category: "strategy", Permission: "optimize_strategy", RiskLevel: "low", OutputSummary: "临时回测 runId、状态、脚本 hash、校验摘要和可选结果视图。"}, func(ctx context.Context, input map[string]any) (any, error) {
		if deps.StartResearchBacktest == nil {
			return nil, fmt.Errorf("research backtest is unavailable")
		}
		script := strings.TrimSpace(stringValue(input, "script"))
		validation, err := ValidateADKStrategyScript("strategy.research_backtest", script)
		if err != nil {
			return nil, err
		}
		run, err := deps.StartResearchBacktest(ResearchBacktestInput{
			Script:           validation.NormalizedScript,
			Market:           stringValue(input, "market"),
			Symbol:           stringValue(input, "symbol"),
			Code:             stringValue(input, "code"),
			Interval:         stringOrDefault(stringValue(input, "interval"), "1m"),
			StartDate:        stringValue(input, "startDate"),
			EndDate:          stringValue(input, "endDate"),
			StartTime:        stringValue(input, "startTime"),
			EndTime:          stringValue(input, "endTime"),
			InitialBalance:   floatValue(input, "initialBalance", 0),
			RehabType:        stringOrDefault(stringValue(input, "rehabType"), "forward"),
			UseExtendedHours: optionalBoolInput(input, "useExtendedHours"),
		})
		if err != nil {
			return nil, err
		}
		waitMs := intValue(input, "waitForCompletionMs", 0)
		if waitMs > 25000 {
			waitMs = 25000
		}
		if waitMs > 0 && deps.BacktestResultView != nil {
			run.Status = waitForADKBacktestStatus(ctx, deps, run.ID, waitMs, run.Status)
		}
		viewInput := backtestResultViewInputFromNested(input["resultView"])
		if viewInput.View == "" {
			viewInput.View = "summary"
		}
		viewInput.RunID = run.ID
		var viewOutput any
		var viewErr error
		if deps.BacktestResultView != nil {
			viewOutput, viewErr = deps.BacktestResultView(viewInput)
			if status := statusFromBacktestResultView(viewOutput); status != "" {
				run.Status = status
			}
		}
		payload := map[string]any{
			"ok":         true,
			"status":     run.Status,
			"runId":      run.ID,
			"scriptHash": researchScriptHash(validation.NormalizedScript),
			"validation": map[string]any{
				"metadata": strategyMetadataPayload(validation.Program),
				"hooks":    BuildCompiledHookKinds(validation.Program),
				"warnings": validation.Warnings,
			},
			"saveRecommendation": "仅当用户明确要求保存/发布/更新策略定义时，再调用 strategy.save_definition。",
		}
		if viewErr != nil {
			payload["resultViewError"] = viewErr.Error()
		} else if viewOutput != nil {
			payload["resultView"] = viewOutput
		}
		return payload, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.save_draft", DisplayName: "保存策略草稿", Description: "把 agent 生成的 Pine Script v6 策略脚本保存为策略定义草稿。", Category: "strategy", Permission: "write_strategy", RequiresApprovalIn: []string{jfadk.PermissionModeApproval}, OutputSummary: "保存后的策略定义。"}, func(_ context.Context, input map[string]any) (any, error) {
		script := strings.TrimSpace(stringValue(input, "script"))
		if script == "" {
			script = strategypinespec.Skeleton()
		}
		validation, err := ValidateADKStrategyScript("strategy.save_draft", script)
		if err != nil {
			return nil, err
		}
		return deps.SaveStrategyDraft(StrategyDraftInput{Name: stringValue(input, "name"), Script: script, Validation: validation})
	})
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.save_definition", DisplayName: "保存策略定义", Description: "新建或更新 Pine Script v6 策略定义；保存前会强制校验 Pine 并拒绝 JFTrade 暂不支持的执行语义。", Category: "strategy", Permission: "write_strategy", RequiresApprovalIn: []string{jfadk.PermissionModeApproval}, OutputSummary: "保存后的策略定义，以及本次是创建还是更新。"}, func(_ context.Context, input map[string]any) (any, error) {
		name := strings.TrimSpace(stringValue(input, "name"))
		if name == "" {
			return nil, fmt.Errorf("name 是必填项")
		}
		validation, err := ValidateADKStrategyScript("strategy.save_definition", stringValue(input, "script"))
		if err != nil {
			return nil, err
		}
		saved, err := deps.SaveStrategyDefinition(StrategyDefinitionInput{DefinitionID: stringValue(input, "definitionId"), Name: name, Description: stringValue(input, "description"), Symbol: stringValue(input, "symbol"), Interval: stringValue(input, "interval"), VisualModel: input["visualModel"], Validation: validation})
		if err != nil {
			return nil, err
		}
		operation := "created"
		if strings.TrimSpace(stringValue(input, "definitionId")) != "" {
			operation = "updated"
		}
		return map[string]any{"operation": operation, "definition": saved}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.update_instance_mode", DisplayName: "修改实例模式", Description: "按 strategy instanceId 修改单个实例的 executionMode，仅允许在实例处于 STOPPED 时执行。", Category: "strategy", Permission: "write_strategy", RequiresApprovalIn: []string{jfadk.PermissionModeApproval}, OutputSummary: "更新后的策略实例，以及本次实际修改的字段。"}, func(_ context.Context, input map[string]any) (any, error) {
		instanceID := strings.TrimSpace(stringValue(input, "instanceId"))
		if instanceID == "" {
			return nil, fmt.Errorf("instanceId 是必填项")
		}
		executionMode := strings.ToLower(strings.TrimSpace(stringValue(input, "executionMode")))
		switch executionMode {
		case "live", "notify_only":
		default:
			return nil, fmt.Errorf("executionMode 必须是以下值之一：%s、%s", "live", "notify_only")
		}
		updated, err := deps.UpdateStrategyInstanceMode(instanceID, executionMode)
		if err != nil {
			return nil, err
		}
		return map[string]any{"instance": updated, "updatedFields": []string{"executionMode"}}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "backtest.runs", DisplayName: "回测结果", Description: "读取最近回测运行结果。", Category: "strategy", Permission: "read_internal", OutputSummary: "最近回测运行和数量。"}, func(context.Context, map[string]any) (any, error) {
		return SummarizeADKBacktestRuns(deps.ListBacktestRuns()), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "backtest.result_view", DisplayName: "回测结果视图", Description: "按 runId 同步读取回测摘要、图表窗口、订单、日志或错误；支持按时间范围、精度和 limit 多次查询。", Category: "strategy", Permission: "read_internal", OutputSummary: "指定回测 run 的轻量摘要或窗口化结果序列。"}, func(_ context.Context, input map[string]any) (any, error) {
		if deps.BacktestResultView == nil {
			return nil, fmt.Errorf("backtest result view is unavailable")
		}
		viewInput := backtestResultViewInputFromMap(input)
		if strings.TrimSpace(viewInput.RunID) == "" {
			return nil, fmt.Errorf("runId is required")
		}
		return deps.BacktestResultView(viewInput)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.optimize", DisplayName: "策略优化", Description: "为多个候选策略定义创建真实异步回测任务，并返回任务引用。", Category: "strategy", Permission: "optimize_strategy", RequiresApprovalIn: []string{jfadk.PermissionModeApproval}, OutputSummary: "优化任务 ID 与候选回测 Run。"}, func(_ context.Context, input map[string]any) (any, error) {
		definitionIDs := stringSliceValue(input, "definitionIds")
		if len(definitionIDs) == 0 {
			if definitionID := strings.TrimSpace(stringValue(input, "definitionId")); definitionID != "" {
				definitionIDs = []string{definitionID}
			}
		}
		if len(definitionIDs) == 0 {
			return nil, fmt.Errorf("definitionIds is required")
		}
		if len(definitionIDs) > 12 {
			return nil, fmt.Errorf("at most 12 optimization candidates are allowed")
		}
		taskID := "opt-" + time.Now().UTC().Format("20060102T150405.000000000")
		runs := make([]map[string]any, 0, len(definitionIDs))
		runRefs := make([]jfadk.OptimizationRunRef, 0, len(definitionIDs))
		for _, definitionID := range definitionIDs {
			run, err := deps.EnqueueBacktest(BacktestStartInput{DefinitionID: definitionID, Market: stringValue(input, "market"), Symbol: stringValue(input, "symbol"), Code: stringValue(input, "code"), Interval: stringOrDefault(stringValue(input, "interval"), "1m"), StartDate: stringValue(input, "startDate"), EndDate: stringValue(input, "endDate"), StartTime: stringValue(input, "startTime"), EndTime: stringValue(input, "endTime"), InitialBalance: floatValue(input, "initialBalance", 0), RehabType: stringOrDefault(stringValue(input, "rehabType"), "forward")})
			if err != nil {
				for _, queued := range runRefs {
					deps.CancelBacktest(queued.RunID)
				}
				return nil, fmt.Errorf("queue candidate %q: %w", definitionID, err)
			}
			runs = append(runs, map[string]any{"definitionId": definitionID, "runId": run.ID, "status": run.Status})
			runRefs = append(runRefs, jfadk.OptimizationRunRef{DefinitionID: definitionID, RunID: run.ID})
		}
		task, err := store.SaveOptimizationTask(context.Background(), jfadk.OptimizationTask{ID: taskID, Status: "queued", Objective: stringOrDefault(stringValue(input, "objective"), "return"), Runs: runRefs})
		if err != nil {
			for _, queued := range runRefs {
				deps.CancelBacktest(queued.RunID)
			}
			return nil, fmt.Errorf("persist optimization task: %w", err)
		}
		return map[string]any{"taskId": task.ID, "status": task.Status, "objective": task.Objective, "runs": runs, "message": "候选策略已进入真实回测队列；使用 backtest.runs 查询进度和结果。"}, nil
	})
}

func registerJFTradeADKReadTools(registry *jfadk.ToolRegistry, deps ToolDeps) {
	registry.Register(jfadk.ToolDescriptor{Name: "broker.orders", DisplayName: "经纪商订单", Description: "读取所选账户范围下经纪商当前或历史订单。", Category: "portfolio", Permission: "read_internal", OutputSummary: "经纪商订单列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		return deps.BrokerOrders(ctx, brokerReadInput(input, deps, "CURRENT"))
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.fills", DisplayName: "经纪商成交", Description: "读取所选账户范围下经纪商当前或历史成交记录。", Category: "portfolio", Permission: "read_internal", OutputSummary: "经纪商成交列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		return deps.BrokerFills(ctx, brokerReadInput(input, deps, "CURRENT"))
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.cash_flows", DisplayName: "资金流水", Description: "按清算日期读取经纪商资金流水记录。", Category: "portfolio", Permission: "read_internal", OutputSummary: "资金流水列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		read := brokerReadInput(input, deps, "")
		read.ClearingDate = stringValue(input, "clearingDate")
		read.Direction = stringValue(input, "direction")
		return deps.BrokerCashFlows(ctx, read)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.fees", DisplayName: "订单费用", Description: "按一个或多个外部订单号读取经纪商费用明细。", Category: "portfolio", Permission: "read_internal", OutputSummary: "订单费用列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		read := brokerReadInput(input, deps, "")
		read.OrderIDEx = stringSliceValue(input, "orderIdEx")
		read.OrderIDExList = stringSliceValue(input, "orderIdExList")
		return deps.BrokerFees(ctx, read)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.margin_ratios", DisplayName: "融资融券比率", Description: "读取一个或多个标的的融资与融券保证金比率。", Category: "portfolio", Permission: "read_internal", OutputSummary: "融资融券比率列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		read := brokerReadInput(input, deps, "")
		read.Symbols = stringSliceValue(input, "symbols")
		if symbol := strings.TrimSpace(stringValue(input, "symbol")); symbol != "" {
			read.Symbols = append(read.Symbols, symbol)
		}
		return deps.BrokerMarginRatios(ctx, read)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "market.depth", DisplayName: "盘口深度", Description: "读取指定标的的买卖盘深度。", Category: "market", Permission: "read_internal", OutputSummary: "买卖盘档位数据。"}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		return deps.MarketDepth(ctx, market, symbol, intValue(input, "num", 10))
	})
	registry.Register(jfadk.ToolDescriptor{Name: "risk.state", DisplayName: "风险状态", Description: "读取实盘 kill switch 与风险限制状态。", Category: "risk", Permission: "read_internal", OutputSummary: "当前 kill switch 与实盘风险状态。"}, func(context.Context, map[string]any) (any, error) {
		return deps.RiskState(), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "risk.events", DisplayName: "风险事件", Description: "读取近期实盘风险事件状态。", Category: "risk", Permission: "read_internal", OutputSummary: "风险事件摘要。"}, func(context.Context, map[string]any) (any, error) {
		return deps.RiskEvents(), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "execution.order_events", DisplayName: "执行订单事件", Description: "按内部订单 ID 读取执行订单事件历史；未提供 ID 时返回订单列表。", Category: "portfolio", Permission: "read_internal", OutputSummary: "执行订单事件时间线。"}, func(_ context.Context, input map[string]any) (any, error) {
		internalOrderID := strings.TrimSpace(stringValue(input, "internalOrderId"))
		if internalOrderID == "" {
			return deps.ExecutionOrders(), nil
		}
		return deps.ExecutionOrderEvents(internalOrderID), nil
	})
}

func registerJFTradeADKWorkflowTools(store *jfadk.Store, registry *jfadk.ToolRegistry, deps ToolDeps) {
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.list", DisplayName: "ADK 任务列表", Description: "列出用于跟踪 agent 工作的 ADK 任务记录。", Category: "workflow", Permission: "read_internal", OutputSummary: "任务分页结果。"}, func(ctx context.Context, input map[string]any) (any, error) {
		limit, offset := httpserver.NormalizeBoundPage(intValue(input, "limit", 20), intValue(input, "offset", 0), 20, 100)
		tasks, total, err := store.ListTasksPage(ctx, stringValue(input, "status"), stringValue(input, "agentId"), stringValue(input, "runId"), limit, offset)
		if err != nil {
			return nil, err
		}
		return map[string]any{"tasks": tasks, "page": pageEnvelope(limit, offset, total, len(tasks))}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.create", DisplayName: "创建 ADK 任务", Description: "创建一个用于后续跟进的轻量 ADK 任务。", Category: "workflow", Permission: "write_task", RiskLevel: "low", OutputSummary: "已创建的任务。"}, func(ctx context.Context, input map[string]any) (any, error) {
		task, err := store.SaveTask(ctx, jfadk.TaskWriteRequest{
			Title:           stringValue(input, "title"),
			Description:     stringValue(input, "description"),
			Status:          stringValue(input, "status"),
			AgentID:         stringValue(input, "agentId"),
			RunID:           stringValue(input, "runId"),
			DependsOn:       stringSliceValue(input, "dependsOn"),
			Order:           intValue(input, "order", 0),
			ModeHint:        stringValue(input, "modeHint"),
			AgentRole:       stringValue(input, "agentRole"),
			PlannerStepID:   stringValue(input, "plannerStepId"),
			PlanSource:      stringValue(input, "planSource"),
			WorkflowMode:    stringValue(input, "workflowMode"),
			Objective:       stringValue(input, "objective"),
			Message:         stringValue(input, "message"),
			Executor:        stringValue(input, "executor"),
			ResultSummary:   stringValue(input, "resultSummary"),
			PlannerWarnings: stringSliceValue(input, "plannerWarnings"),
		})
		if err == nil {
			recordADKWorkflowAudit(ctx, deps, "task.saved", task.ID, "ADK task saved.", map[string]any{"status": task.Status})
		}
		return task, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.update", DisplayName: "更新 ADK 任务", Description: "更新轻量 ADK 任务的状态或详情。", Category: "workflow", Permission: "write_task", OutputSummary: "已更新的任务。"}, func(ctx context.Context, input map[string]any) (any, error) {
		task, err := store.UpdateTask(ctx, stringValue(input, "id"), taskPatchFromInput(input))
		if err == nil {
			recordADKWorkflowAudit(ctx, deps, "task.updated", task.ID, "ADK task updated.", map[string]any{"status": task.Status})
		}
		return task, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.delete", DisplayName: "删除 ADK 任务", Description: "删除轻量 ADK 任务记录。", Category: "workflow", Permission: "write_task", OutputSummary: "删除结果。"}, func(ctx context.Context, input map[string]any) (any, error) {
		id := stringValue(input, "id")
		if err := store.DeleteTask(ctx, id); err != nil {
			return nil, err
		}
		recordADKWorkflowAudit(ctx, deps, "task.deleted", id, "ADK task deleted.", nil)
		return map[string]any{"deleted": true, "id": id}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "memory.list", DisplayName: "ADK 记忆列表", Description: "列出 ADK 数据库中的工作区和 agent 记忆条目。", Category: "workflow", Permission: "read_internal", OutputSummary: "记忆条目列表。"}, func(ctx context.Context, input map[string]any) (any, error) {
		entries, err := store.ListMemoryFiltered(ctx, stringValue(input, "scope"), stringValue(input, "agentId"), stringValue(input, "key"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"entries": entries, "totalReturned": len(entries)}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "memory.remember", DisplayName: "写入 ADK 记忆", Description: "将简短的工作区或 agent 记忆条目保存到 ADK 数据库。", Category: "workflow", Permission: "write_memory", OutputSummary: "已保存的记忆条目。"}, func(ctx context.Context, input map[string]any) (any, error) {
		entry, err := store.SaveMemory(ctx, jfadk.MemoryWriteRequest{AgentID: stringValue(input, "agentId"), Key: stringValue(input, "key"), Value: stringValue(input, "value"), Scope: stringValue(input, "scope")})
		if err == nil {
			recordADKWorkflowAudit(ctx, deps, "memory.saved", entry.ID, "ADK memory saved.", map[string]any{"scope": entry.Scope, "key": entry.Key})
		}
		return entry, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "memory.forget", DisplayName: "删除 ADK 记忆", Description: "从 ADK 数据库删除工作区或 agent 记忆条目。", Category: "workflow", Permission: "write_memory", OutputSummary: "删除结果。"}, func(ctx context.Context, input map[string]any) (any, error) {
		id := stringValue(input, "id")
		if err := store.DeleteMemory(ctx, id); err != nil {
			return nil, err
		}
		recordADKWorkflowAudit(ctx, deps, "memory.deleted", id, "ADK memory deleted.", nil)
		return map[string]any{"deleted": true, "id": id}, nil
	})
}

func recordADKWorkflowAudit(ctx context.Context, deps ToolDeps, kind string, subjectID string, detail string, metadata map[string]any) {
	if deps.RecordAudit != nil {
		deps.RecordAudit(ctx, kind, subjectID, detail, metadata)
	}
}

type StrategyPineValidation struct {
	NormalizedScript string
	Program          *strategyir.Program
	Requirements     strategyir.Requirements
	Warnings         []string
}

func StrategyPineSpecToolPayload(input map[string]any) (map[string]any, error) {
	return strategypinespec.BuildToolPayload(stringValue(input, "section"), boolInputValue(input, "includeExamples"))
}

func StrategyValidatePineToolPayload(input map[string]any) map[string]any {
	script := strings.TrimSpace(stringValue(input, "script"))
	includeRequirements := boolInputValueDefault(input, "includeRequirements", true)
	payload := map[string]any{"ok": false, "sourceFormat": strategypinespec.SourceFormat, "runtime": strategypinespec.Runtime, "normalizedScript": script, "metadata": strategyMetadataPayload(nil), "hooks": []string{}, "requirements": nil, "warnings": []string{}, "errors": []string{}, "saveHint": nil}
	if script == "" {
		payload["errors"] = []string{"script 是必填项"}
		payload["saveHint"] = StrategySaveHintPayload()
		return payload
	}
	validation, err := ValidateADKStrategyScript("strategy.validate_pine", script)
	if err != nil {
		payload["errors"] = []string{err.Error()}
		payload["saveHint"] = StrategySaveHintPayload()
		return payload
	}
	payload["ok"] = true
	payload["normalizedScript"] = validation.NormalizedScript
	payload["metadata"] = strategyMetadataPayload(validation.Program)
	payload["hooks"] = BuildCompiledHookKinds(validation.Program)
	payload["warnings"] = validation.Warnings
	if includeRequirements {
		payload["requirements"] = BuildCompiledRequirementsPayload(validation.Requirements)
	}
	return payload
}

func ValidateADKStrategyDraftScript(script string) error {
	if strings.TrimSpace(script) == "" {
		return nil
	}
	_, err := ValidateADKStrategyScript("strategy.save_draft", script)
	return err
}

func ValidateADKStrategyScript(toolName string, script string) (StrategyPineValidation, error) {
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return StrategyPineValidation{}, fmt.Errorf("%s 需要提供非空的 Pine Script v6 策略脚本", strings.TrimSpace(toolName))
	}
	compilation, err := strategypine.Compile(trimmed)
	if err != nil {
		return StrategyPineValidation{}, fmt.Errorf("%s 需要合法的 Pine Script v6 策略脚本：%w\n\n%s", strings.TrimSpace(toolName), err, strategypinespec.SaveDraftUsageHint())
	}
	return StrategyPineValidation{NormalizedScript: trimmed, Program: compilation.Program, Requirements: compilation.Requirements, Warnings: compilation.Warnings}, nil
}

func StrategySaveHintPayload() map[string]any {
	return map[string]any{"message": strategypinespec.SaveDraftUsageHint(), "specTool": strategypinespec.ToolName, "resourceFiles": []string{"references/pine-v6-spec.md", "references/pine-v6-examples.md"}, "skeleton": strategypinespec.Skeleton()}
}

func strategyMetadataPayload(program *strategyir.Program) map[string]any {
	metadata := map[string]any{"name": "", "version": "", "symbol": "", "interval": "", "defaultQtyMode": "fixed", "defaultQtyValue": "1", "pyramiding": 1}
	if program == nil {
		return metadata
	}
	metadata["name"] = strings.TrimSpace(program.Metadata.Name)
	metadata["version"] = strings.TrimSpace(program.Metadata.Version)
	metadata["symbol"] = strings.TrimSpace(program.Metadata.Symbol)
	metadata["interval"] = strings.TrimSpace(program.Metadata.Interval)
	metadata["defaultQtyMode"] = strings.TrimSpace(program.Metadata.DefaultQtyMode)
	if metadata["defaultQtyMode"] == "" {
		metadata["defaultQtyMode"] = "fixed"
	}
	metadata["defaultQtyValue"] = strings.TrimSpace(program.Metadata.DefaultQtyValue)
	if metadata["defaultQtyValue"] == "" {
		metadata["defaultQtyValue"] = "1"
	}
	metadata["pyramiding"] = program.Metadata.Pyramiding
	if program.Metadata.Pyramiding <= 0 {
		metadata["pyramiding"] = 1
	}
	return metadata
}

func BuildCompiledHookKinds(program *strategyir.Program) []string {
	if program == nil {
		return []string{}
	}
	kinds := make([]string, 0, len(program.Hooks))
	for _, hook := range program.Hooks {
		kinds = append(kinds, string(hook.Kind))
	}
	return kinds
}

func BuildCompiledRequirementsPayload(requirements strategyir.Requirements) map[string]any {
	indicators := make([]map[string]any, 0, len(requirements.Indicators))
	for _, indicator := range requirements.Indicators {
		indicators = append(indicators, map[string]any{
			"alias": indicator.Alias,
			"kind":  indicator.Kind,
			"key":   indicator.Key,
		})
	}
	return map[string]any{
		"indicators":                indicators,
		"requiresPosition":          requirements.RequiresPosition,
		"requiresTotalAccountValue": requirements.RequiresTotalAccountValue,
	}
}

func StrategyMetadataPayload(program *strategyir.Program) map[string]any {
	return strategyMetadataPayload(program)
}

func SummarizeADKStrategyDefinitions(definitions []StrategyDefinitionSummary, instances []StrategyInstanceSummary) map[string]any {
	linkedInstanceCountByDefinition := make(map[string]int, len(instances))
	instanceSummaries := make([]map[string]any, 0, len(instances))
	for _, item := range instances {
		definitionID := strings.TrimSpace(item.DefinitionID)
		if definitionID != "" {
			linkedInstanceCountByDefinition[definitionID]++
		}
		instanceSummaries = append(instanceSummaries, summarizeADKStrategyInstance(item))
	}
	definitionSummaries := make([]map[string]any, 0, len(definitions))
	for _, definition := range definitions {
		definitionSummaries = append(definitionSummaries, summarizeADKStrategyDefinition(definition, linkedInstanceCountByDefinition[definition.ID]))
	}
	return map[string]any{"definitions": definitionSummaries, "definitionCount": len(definitionSummaries), "instances": instanceSummaries, "instanceCount": len(instanceSummaries)}
}

func summarizeADKStrategyDefinition(definition StrategyDefinitionSummary, linkedInstanceCount int) map[string]any {
	return map[string]any{"id": definition.ID, "name": definition.Name, "version": definition.Version, "description": definition.Description, "runtime": definition.Runtime, "sourceFormat": definition.SourceFormat, "symbol": definition.Symbol, "interval": definition.Interval, "createdAt": definition.CreatedAt, "updatedAt": definition.UpdatedAt, "scriptPreview": summarizeADKText(definition.Script, 280), "scriptBytes": len([]byte(definition.Script)), "linkedInstanceCount": linkedInstanceCount, "visualNodeCount": definition.VisualNodeCount, "visualEdgeCount": definition.VisualEdgeCount}
}

func summarizeADKStrategyInstance(item StrategyInstanceSummary) map[string]any {
	return map[string]any{"id": item.ID, "definitionId": item.DefinitionID, "definitionName": item.DefinitionName, "definitionVersion": item.DefinitionVersion, "runtime": item.Runtime, "sourceFormat": item.SourceFormat, "status": item.Status, "actualStatus": item.ActualStatus, "startable": item.Startable, "symbols": append([]string(nil), item.Symbols...), "symbolCount": len(item.Symbols), "activeSymbols": append([]string(nil), item.ActiveSymbols...), "activeSymbolCount": len(item.ActiveSymbols), "interval": item.Interval, "executionMode": item.ExecutionMode, "market": item.Market, "accountId": item.AccountID, "createdAt": item.CreatedAt, "logCount": item.LogCount, "latestLog": summarizeADKText(item.LatestLog, 220), "lastError": summarizeADKText(item.LastError, 220)}
}

func SummarizeADKBacktestRuns(runs []BacktestRunSummary) map[string]any {
	items := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		items = append(items, summarizeADKBacktestRun(run))
	}
	return map[string]any{"runs": items, "runCount": len(items)}
}

func summarizeADKBacktestRun(run BacktestRunSummary) map[string]any {
	summary := map[string]any{"id": run.ID, "status": run.Status, "definitionId": run.DefinitionID, "definitionVersion": run.DefinitionVersion, "market": run.Market, "code": run.Code, "symbol": run.Symbol, "interval": run.Interval, "startDate": run.StartDate, "endDate": run.EndDate, "startTime": run.StartTime, "endTime": run.EndTime, "marketTimezone": run.MarketTimezone, "initialBalance": run.InitialBalance, "rehabType": run.RehabType, "createdAt": run.CreatedAt, "updatedAt": run.UpdatedAt}
	if run.UseExtendedHours != nil {
		summary["useExtendedHours"] = *run.UseExtendedHours
	}
	if run.Result == nil {
		return summary
	}
	tradeCount := run.Result.TotalTrades
	totalReturn := 0.0
	if run.InitialBalance > 0 {
		totalReturn = run.Result.PnL / run.InitialBalance
	}
	summary["quoteCurrency"] = run.Result.QuoteCurrency
	summary["finalBalance"] = run.Result.FinalBalance
	summary["pnl"] = run.Result.PnL
	summary["totalReturn"] = totalReturn
	summary["maxDrawdown"] = run.Result.MaxDrawdown
	summary["currentDrawdown"] = run.Result.CurrentDrawdown
	summary["totalTrades"] = run.Result.TotalTrades
	summary["tradeCount"] = tradeCount
	summary["winRate"] = run.Result.WinRate
	summary["orderBookCount"] = len(run.Result.OrderBook)
	summary["tradesCount"] = len(run.Result.Trades)
	summary["candlesCount"] = len(run.Result.Candles)
	summary["pnlCurveCount"] = len(run.Result.PnLCurve)
	summary["drawdownCurveCount"] = len(run.Result.DrawdownCurve)
	summary["logsCount"] = len(run.Result.Logs)
	summary["runtimeErrorCount"] = len(run.Result.RuntimeErrors)
	summary["error"] = summarizeADKText(run.Result.Error, 220)
	summary["latestLog"] = summarizeADKText(lastString(run.Result.Logs), 220)
	summary["latestRuntimeError"] = summarizeADKText(lastString(run.Result.RuntimeErrors), 220)
	if latestTrade := lastBacktestTrade(run.Result.Trades); latestTrade != nil {
		summary["latestTradeAt"] = latestTrade.Time
		summary["latestTradeSide"] = latestTrade.Side
		summary["latestTradePrice"] = latestTrade.Price
	}
	if latestCandle := lastBacktestCandle(run.Result.Candles); latestCandle != nil {
		summary["latestCandleAt"] = latestCandle.Time
		summary["latestClose"] = latestCandle.Close
	}
	return summary
}

func researchScriptHash(script string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(script)))
	return hex.EncodeToString(hash[:])[:16]
}

func optionalBoolInput(input map[string]any, key string) *bool {
	value, ok := input[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case bool:
		return &typed
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		parsed := strings.EqualFold(trimmed, "true") || strings.EqualFold(trimmed, "yes") || trimmed == "1"
		return &parsed
	case float64:
		parsed := typed != 0
		return &parsed
	case int:
		parsed := typed != 0
		return &parsed
	default:
		return nil
	}
}

func backtestResultViewInputFromNested(value any) BacktestResultViewInput {
	if value == nil {
		return BacktestResultViewInput{}
	}
	if typed, ok := value.(map[string]any); ok {
		return backtestResultViewInputFromMap(typed)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return BacktestResultViewInput{}
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return BacktestResultViewInput{}
	}
	return backtestResultViewInputFromMap(raw)
}

func backtestResultViewInputFromMap(input map[string]any) BacktestResultViewInput {
	return BacktestResultViewInput{
		RunID:      stringValue(input, "runId"),
		View:       stringValue(input, "view"),
		Resolution: stringValue(input, "resolution"),
		StartTime:  stringValue(input, "startTime"),
		EndTime:    stringValue(input, "endTime"),
		Include:    stringSliceOrCSVValue(input, "include"),
		Limit:      intValue(input, "limit", 0),
		Cursor:     stringValue(input, "cursor"),
	}
}

func waitForADKBacktestStatus(ctx context.Context, deps ToolDeps, runID string, waitMs int, initialStatus string) string {
	if deps.BacktestResultView == nil || strings.TrimSpace(runID) == "" || waitMs <= 0 {
		return initialStatus
	}
	status := initialStatus
	deadline := time.NewTimer(time.Duration(waitMs) * time.Millisecond)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if view, err := deps.BacktestResultView(BacktestResultViewInput{RunID: runID, View: "summary", Limit: 1}); err == nil {
			if next := statusFromBacktestResultView(view); next != "" {
				status = next
				if isTerminalBacktestStatus(next) {
					return status
				}
			}
		}
		select {
		case <-ctx.Done():
			return status
		case <-deadline.C:
			return status
		case <-ticker.C:
		}
	}
}

func statusFromBacktestResultView(value any) string {
	payload, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	runValue, ok := payload["run"].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(runValue["status"]))
}

func isTerminalBacktestStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func brokerReadInput(input map[string]any, deps ToolDeps, defaultScope string) BrokerReadInput {
	market := stringValue(input, "market")
	if market == "" && deps.DefaultTradeMarket != nil {
		market = deps.DefaultTradeMarket()
	}
	return BrokerReadInput{TradingEnvironment: stringValue(input, "tradingEnvironment"), AccountID: stringValue(input, "accountId"), Market: market, Scope: stringOrDefault(stringValue(input, "scope"), defaultScope), Symbol: stringValue(input, "symbol"), StartTime: stringValue(input, "startTime"), EndTime: stringValue(input, "endTime"), Status: stringSliceValue(input, "status"), Statuses: stringSliceValue(input, "statuses")}
}

func taskPatchFromInput(input map[string]any) jfadk.TaskPatchRequest {
	return jfadk.TaskPatchRequest{
		Title:           stringPtrFromInput(input, "title"),
		Description:     stringPtrFromInput(input, "description"),
		Status:          stringPtrFromInput(input, "status"),
		AgentID:         stringPtrFromInput(input, "agentId"),
		RunID:           stringPtrFromInput(input, "runId"),
		DependsOn:       stringSliceFromPresentInput(input, "dependsOn"),
		Order:           intPtrFromInput(input, "order"),
		ModeHint:        stringPtrFromInput(input, "modeHint"),
		AgentRole:       stringPtrFromInput(input, "agentRole"),
		PlannerStepID:   stringPtrFromInput(input, "plannerStepId"),
		PlanSource:      stringPtrFromInput(input, "planSource"),
		WorkflowMode:    stringPtrFromInput(input, "workflowMode"),
		Objective:       stringPtrFromInput(input, "objective"),
		Message:         stringPtrFromInput(input, "message"),
		Executor:        stringPtrFromInput(input, "executor"),
		ResultSummary:   stringPtrFromInput(input, "resultSummary"),
		PlannerWarnings: stringSliceFromPresentInput(input, "plannerWarnings"),
	}
}

func intPtrFromInput(input map[string]any, key string) *int {
	if _, ok := input[key]; !ok {
		return nil
	}
	return new(intValue(input, key, 0))
}

func stringPtrFromInput(input map[string]any, key string) *string {
	value, ok := input[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return &typed
	case nil:
		return new("")
	default:
		return new(fmt.Sprint(typed))
	}
}

func stringSliceFromPresentInput(input map[string]any, key string) []string {
	if _, ok := input[key]; !ok {
		return nil
	}
	return stringSliceValue(input, key)
}

func pageEnvelope(limit int, offset int, total int, returned int) map[string]any {
	return map[string]any{"limit": limit, "offset": offset, "total": total, "returned": returned, "hasMore": offset+returned < total}
}

var adkInstrumentPattern = regexp.MustCompile(`(?i)\b(HK|US|SH|SZ|CN|JP|SG)\.([A-Z0-9._-]+)\b`)

func inferMarketSymbol(input map[string]any) (string, string) {
	market := strings.ToUpper(strings.TrimSpace(stringValue(input, "market")))
	symbol := strings.ToUpper(strings.TrimSpace(stringValue(input, "symbol")))
	if market != "" && symbol != "" {
		return market, symbol
	}
	query := strings.ToUpper(strings.TrimSpace(stringValue(input, "query")))
	if match := adkInstrumentPattern.FindStringSubmatch(query); len(match) == 3 {
		return strings.ToUpper(match[1]), strings.Trim(strings.ToUpper(match[2]), "。.!?()[]{}")
	}
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\t'
	})
	for _, field := range fields {
		field = strings.Trim(field, "。.!?()[]{}")
		if strings.Contains(field, ".") {
			parts := strings.SplitN(field, ".", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				return parts[0], parts[1]
			}
		}
	}
	return "", ""
}

func stringValue(input map[string]any, key string) string {
	value := jftradeOptionalTypeAssertion[string](input[key])
	return value
}

func intValue(input map[string]any, key string, defaultValue int) int {
	switch value := input[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	}
	return defaultValue
}

func floatValue(input map[string]any, key string, defaultValue float64) float64 {
	switch value := input[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil {
			return parsed
		}
	}
	return defaultValue
}

func stringSliceValue(input map[string]any, key string) []string {
	values, ok := input[key].([]any)
	if !ok {
		if typed, typedOK := input[key].([]string); typedOK {
			return typed
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

func stringSliceOrCSVValue(input map[string]any, key string) []string {
	if values := stringSliceValue(input, key); len(values) > 0 {
		return values
	}
	raw := strings.TrimSpace(stringValue(input, key))
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func boolInputValue(input map[string]any, key string) bool {
	return boolInputValueDefault(input, key, false)
}

func boolInputValueDefault(input map[string]any, key string, defaultValue bool) bool {
	value, ok := input[key]
	if !ok {
		return defaultValue
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return defaultValue
	}
}

func stringOrDefault(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}

func summarizeADKText(text string, limit int) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if trimmed == "" || limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "..."
}

func lastString(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1]
}

func lastBacktestTrade(items []backtest.TradeEvent) *backtest.TradeEvent {
	if len(items) == 0 {
		return nil
	}
	return &items[len(items)-1]
}

func lastBacktestCandle(items []backtest.Candle) *backtest.Candle {
	if len(items) == 0 {
		return nil
	}
	return &items[len(items)-1]
}

func nowStringRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func collectionLen(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	case interface{ Len() int }:
		return typed.Len()
	default:
		return 0
	}
}

func callMap(fn func() map[string]any) map[string]any {
	if fn == nil {
		return map[string]any{}
	}
	return fn()
}

func callBool(fn func() bool) bool {
	return fn != nil && fn()
}

func SourceFormatPineV6() string { return strategydefinition.SourceFormatPineV6 }
