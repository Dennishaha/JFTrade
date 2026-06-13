package jftradeapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/broker"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func deriveADKDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_ADK_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return "adk.db"
	}
	return filepath.Join(directory, "adk.db")
}

func deriveADKSecretsPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_ADK_SECRETS")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return filepath.Join("secrets", "adk-secrets.json")
	}
	return filepath.Join(directory, "secrets", "adk-secrets.json")
}

func deriveADKSkillsDir(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_ADK_SKILLS_DIR")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return filepath.Join("adk", "skills")
	}
	return filepath.Join(directory, "adk", "skills")
}

func deriveADKSessionDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_ADK_SESSION_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return "adk-session.db"
	}
	return filepath.Join(directory, "adk-session.db")
}

func newADKRuntime(server *Server, settingsPath string) *jfadk.Runtime {
	dbPath := deriveADKDBPath(settingsPath)
	sessionDBPath := deriveADKSessionDBPath(settingsPath)
	if err := backupSQLiteFiles(dbPath, sessionDBPath); err != nil {
		log.Printf("JFTrade ADK backup degraded: %v", err)
	}
	store, err := jfadk.NewStore(dbPath, deriveADKSecretsPath(settingsPath), deriveADKSkillsDir(settingsPath))
	if err != nil {
		log.Printf("JFTrade ADK runtime degraded: %v", err)
		return nil
	}
	registry := jfadk.NewToolRegistry()
	registerJFTradeADKTools(server, store, registry)
	sessionService, err := jfadk.NewSQLiteSessionService(sessionDBPath)
	if err != nil {
		log.Printf("JFTrade ADK session store degraded: %v", err)
		runtime := jfadk.NewRuntime(store, registry)
		configureADKRuntime(server, runtime)
		return runtime
	}
	if err := jfadk.MigrateSQLiteSessionService(sessionService); err != nil {
		log.Printf("JFTrade ADK session migration degraded: %v", err)
		_ = sessionService.Close()
		runtime := jfadk.NewRuntime(store, registry)
		configureADKRuntime(server, runtime)
		return runtime
	}
	runtime := jfadk.NewRuntimeWithSessionService(store, registry, sessionService)
	configureADKRuntime(server, runtime)
	return runtime
}

func configureADKRuntime(server *Server, runtime *jfadk.Runtime) {
	if server == nil || runtime == nil || server.store == nil {
		return
	}
	runtime.SetRuntimeLimitsProvider(func() jfadk.RuntimeLimits {
		settings := server.store.adkSettings()
		return jfadk.RuntimeLimits{
			RunTimeout: time.Duration(settings.RunTimeoutMs) * time.Millisecond,
		}
	})
}

func backupSQLiteFiles(paths ...string) error {
	var errs []error
	for _, path := range paths {
		if err := backupSQLiteFile(path, 3); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func backupSQLiteFile(path string, retain int) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("ADK database is not a regular file: %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backupDir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, filepath.Base(path)+"."+time.Now().UTC().Format("20060102T150405.000000000Z")+".bak")
	if err := os.WriteFile(backupPath, raw, 0o600); err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(backupDir, filepath.Base(path)+".*.bak"))
	if err != nil {
		return err
	}
	sort.Strings(matches)
	for len(matches) > retain {
		if err := os.Remove(matches[0]); err != nil {
			return err
		}
		matches = matches[1:]
	}
	return nil
}

func registerJFTradeADKTools(server *Server, store *jfadk.Store, registry *jfadk.ToolRegistry) {
	registry.Register(jfadk.ToolDescriptor{Name: "system.status", DisplayName: "系统状态", Description: "读取 JFTrade API、持久层、broker、策略运行时和 ADK 状态摘要。", Category: "system", Permission: "read_internal", OutputSummary: "系统健康、持久化、broker、策略运行时与 ADK 状态。"}, func(context.Context, map[string]any) (any, error) {
		status := server.systemStatus()
		if server.adkRuntime != nil {
			status["adk"] = map[string]any{"module": jfadk.GoogleADKModule, "enabled": true}
		}
		return status, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "system.futu_opend", DisplayName: "OpenD 健康", Description: "读取 Futu OpenD 连通性、登录态与诊断。", Category: "system", Permission: "read_internal", OutputSummary: "OpenD 连接、登录态、配置和诊断信息。"}, func(ctx context.Context, _ map[string]any) (any, error) {
		return server.futuOpenDHealth(ctx), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "plugins.catalog", DisplayName: "策略插件目录", Description: "读取现有策略插件安装状态。", Category: "system", Permission: "read_internal", OutputSummary: "策略插件目录与安装状态。"}, func(context.Context, map[string]any) (any, error) {
		return server.strategyStore.pluginCatalog(), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "market.subscriptions", DisplayName: "行情订阅", Description: "读取当前行情订阅和配额摘要。", Category: "market", Permission: "read_internal", OutputSummary: "当前订阅、活跃标的和检查时间。"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{
			"subscriptions":     server.marketSubscriptionsResponse(),
			"activeInstruments": server.marketSubscriptions.activeInstrumentIDs(),
			"checkedAt":         nowStringRFC3339Nano(),
		}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "market.snapshot", DisplayName: "行情快照", Description: "读取当前工作问题中指定标的的行情快照；未指定时返回可用说明。", Category: "market", Permission: "read_internal", OutputSummary: "单个标的的行情快照或缺少标的提示。"}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		return server.marketSnapshotResponseForInstrument(ctx, market, symbol, marketSnapshotQuery{Refresh: newOptionalBoolValue(false)})
	})
	registry.Register(jfadk.ToolDescriptor{Name: "market.candles", DisplayName: "K 线查询", Description: "读取指定标的近期 K 线；未指定时返回使用说明。", Category: "market", Permission: "read_internal", OutputSummary: "近期 1m K 线，默认最多 50 根。"}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		period := defaultStringLocal(stringValue(input, "period"), "1m")
		limit := intValue(input, "limit", 50)
		if limit < 1 || limit > 500 {
			return nil, fmt.Errorf("limit must be between 1 and 500")
		}
		normalizedPeriod, err := normalizeCandlePeriod(period)
		if err != nil {
			return nil, err
		}
		return server.marketCandlesResponseForInstrument(ctx, market, symbol, marketCandlesQuery{
			Period: candlePeriodValue(normalizedPeriod),
			Limit:  newOptionalIntValue(limit),
		})
	})
	registry.Register(jfadk.ToolDescriptor{Name: "portfolio.summary", DisplayName: "组合摘要", Description: "读取托管账户、资金、订单和持仓的控制台摘要。", Category: "portfolio", Permission: "read_internal", OutputSummary: "托管账户、broker 状态、本地订单摘要和当前检查时间。"}, func(ctx context.Context, input map[string]any) (any, error) {
		orders := []executionOrderSummaryResponse{}
		if server.executionOrders != nil {
			orders = server.executionOrders.listOrders().Orders
		}
		query := broker.ReadQuery{
			BrokerID:           "futu",
			AccountID:          strings.TrimSpace(stringValue(input, "accountId")),
			TradingEnvironment: strings.ToUpper(strings.TrimSpace(stringValue(input, "tradingEnvironment"))),
			Market:             strings.ToUpper(defaultStringLocal(stringValue(input, "market"), server.store.integration().Config.TradeMarket)),
		}
		funds := server.brokerFundsResponseWithTimeout(ctx, query, 8*time.Second)
		positions := server.brokerPositionsResponseWithTimeout(ctx, query, 8*time.Second)
		return map[string]any{
			"accounts":      server.store.managedAccounts(),
			"brokerEnabled": server.futuIntegrationEnabled(),
			"orders":        orders,
			"orderCount":    len(orders),
			"funds":         funds,
			"positions":     positions,
			"checkedAt":     nowStringRFC3339Nano(),
		}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "account.orders", DisplayName: "订单摘要", Description: "读取本地执行订单视图摘要。", Category: "portfolio", Permission: "read_internal", OutputSummary: "本地执行订单列表和数量。"}, func(_ context.Context, _ map[string]any) (any, error) {
		orders := []executionOrderSummaryResponse{}
		if server.executionOrders != nil {
			orders = server.executionOrders.listOrders().Orders
		}
		return map[string]any{"orders": orders, "count": len(orders), "checkedAt": nowStringRFC3339Nano()}, nil
	})
	registerJFTradeADKWorkflowTools(server, store, registry)
	registerJFTradeADKReadTools(server, registry)
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.definitions", DisplayName: "策略定义", Description: "读取当前策略定义和策略实例摘要。", Category: "strategy", Permission: "read_internal", OutputSummary: "策略定义、运行实例和数量摘要。"}, func(context.Context, map[string]any) (any, error) {
		definitions := server.designStore.listDefinitions()
		instances := server.enrichStrategyItems(server.strategyStore.strategies())
		return summarizeADKStrategyDefinitions(definitions, instances), nil
	})
	registry.Register(jfadk.ToolDescriptor{
		Name:          strategypinespec.ToolName,
		DisplayName:   "Pine 定义",
		Description:   "读取当前 JFTrade Pine Script v6 的结构化定义、最小骨架、支持清单和示例。",
		Category:      "strategy",
		Permission:    "read_internal",
		OutputSummary: "JFTrade Pine Script v6 的章节摘要、支持语法与可选示例。",
	}, func(_ context.Context, input map[string]any) (any, error) {
		return strategyPineSpecToolPayload(input)
	})
	registry.Register(jfadk.ToolDescriptor{
		Name:          "strategy.validate_pine",
		DisplayName:   "校验 Pine",
		Description:   "校验 Pine Script v6 是否可被当前 parser、lowerer、planner 和 runtime 接受，并返回结构化元数据、warnings 与 requirements。",
		Category:      "strategy",
		Permission:    "read_internal",
		OutputSummary: "校验结果、元数据、hooks、warnings、编译后的 requirements，以及失败时的保存提示。",
	}, func(_ context.Context, input map[string]any) (any, error) {
		return strategyValidatePineToolPayload(input), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.save_draft", DisplayName: "保存策略草稿", Description: "把 agent 生成的 Pine Script v6 策略脚本保存为策略定义草稿。", Category: "strategy", Permission: "write_strategy", RequiresApprovalIn: []string{jfadk.PermissionModeApproval}, OutputSummary: "保存后的策略定义。"}, func(_ context.Context, input map[string]any) (any, error) {
		return strategySaveDraftToolPayload(server, input)
	})
	registry.Register(jfadk.ToolDescriptor{
		Name:               "strategy.save_definition",
		DisplayName:        "保存策略定义",
		Description:        "新建或更新 Pine Script v6 策略定义；保存前会强制校验 Pine 并拒绝 JFTrade 暂不支持的执行语义。",
		Category:           "strategy",
		Permission:         "write_strategy",
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
		OutputSummary:      "保存后的策略定义，以及本次是创建还是更新。",
	}, func(_ context.Context, input map[string]any) (any, error) {
		return strategySaveDefinitionToolPayload(server, input)
	})
	registry.Register(jfadk.ToolDescriptor{
		Name:               "strategy.update_instance_mode",
		DisplayName:        "修改实例模式",
		Description:        "按 strategy instanceId 修改单个实例的 executionMode，仅允许在实例处于 STOPPED 时执行。",
		Category:           "strategy",
		Permission:         "write_strategy",
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
		OutputSummary:      "更新后的策略实例，以及本次实际修改的字段。",
	}, func(_ context.Context, input map[string]any) (any, error) {
		return strategyUpdateInstanceModeToolPayload(server, input)
	})
	registry.Register(jfadk.ToolDescriptor{Name: "backtest.runs", DisplayName: "回测结果", Description: "读取最近回测运行结果。", Category: "strategy", Permission: "read_internal", OutputSummary: "最近回测运行和数量。"}, func(context.Context, map[string]any) (any, error) {
		return summarizeADKBacktestRuns(server.backtestRuns.list()), nil
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
			run, err := server.enqueueBacktest(backtestStartRequest{
				DefinitionID:   definitionID,
				Market:         stringValue(input, "market"),
				Symbol:         stringValue(input, "symbol"),
				Code:           stringValue(input, "code"),
				Interval:       defaultStringLocal(stringValue(input, "interval"), "1m"),
				StartTime:      stringValue(input, "startTime"),
				EndTime:        stringValue(input, "endTime"),
				InitialBalance: floatValue(input, "initialBalance", 0),
				RehabType:      defaultStringLocal(stringValue(input, "rehabType"), "forward"),
			})
			if err != nil {
				for _, queued := range runRefs {
					server.backtestRuns.cancel(queued.RunID)
				}
				return nil, fmt.Errorf("queue candidate %q: %w", definitionID, err)
			}
			runs = append(runs, map[string]any{"definitionId": definitionID, "runId": run.ID, "status": run.Status})
			runRefs = append(runRefs, jfadk.OptimizationRunRef{DefinitionID: definitionID, RunID: run.ID})
		}
		task, err := server.adkRuntime.Store().SaveOptimizationTask(context.Background(), jfadk.OptimizationTask{
			ID:        taskID,
			Status:    "queued",
			Objective: defaultStringLocal(stringValue(input, "objective"), "return"),
			Runs:      runRefs,
		})
		if err != nil {
			for _, queued := range runRefs {
				server.backtestRuns.cancel(queued.RunID)
			}
			return nil, fmt.Errorf("persist optimization task: %w", err)
		}
		return map[string]any{
			"taskId":    task.ID,
			"status":    task.Status,
			"objective": task.Objective,
			"runs":      runs,
			"message":   "候选策略已进入真实回测队列；使用 backtest.runs 查询进度和结果。",
		}, nil
	})
}

func summarizeADKStrategyDefinitions(definitions []strategyDesignDefinition, instances []strategyListItem) map[string]any {
	linkedInstanceCountByDefinition := make(map[string]int, len(instances))
	instanceSummaries := make([]map[string]any, 0, len(instances))
	for _, item := range instances {
		definitionID := strings.TrimSpace(item.Definition.StrategyID)
		if definitionID == "" {
			definitionID = strategyDefinitionIDFromParams(item.Params)
		}
		if definitionID != "" {
			linkedInstanceCountByDefinition[definitionID]++
		}
		instanceSummaries = append(instanceSummaries, summarizeADKStrategyInstance(item, definitionID))
	}

	definitionSummaries := make([]map[string]any, 0, len(definitions))
	for _, definition := range definitions {
		definitionSummaries = append(definitionSummaries, summarizeADKStrategyDefinition(definition, linkedInstanceCountByDefinition[definition.ID]))
	}

	return map[string]any{
		"definitions":     definitionSummaries,
		"definitionCount": len(definitionSummaries),
		"instances":       instanceSummaries,
		"instanceCount":   len(instanceSummaries),
	}
}

func summarizeADKStrategyDefinition(definition strategyDesignDefinition, linkedInstanceCount int) map[string]any {
	summary := map[string]any{
		"id":                  definition.ID,
		"name":                definition.Name,
		"version":             definition.Version,
		"description":         definition.Description,
		"runtime":             definition.Runtime,
		"sourceFormat":        definition.SourceFormat,
		"symbol":              definition.Symbol,
		"interval":            definition.Interval,
		"createdAt":           definition.CreatedAt,
		"updatedAt":           definition.UpdatedAt,
		"scriptPreview":       summarizeADKText(definition.Script, 280),
		"scriptBytes":         len([]byte(definition.Script)),
		"linkedInstanceCount": linkedInstanceCount,
	}
	if definition.VisualModel != nil {
		summary["visualNodeCount"] = len(definition.VisualModel.Nodes)
		summary["visualEdgeCount"] = len(definition.VisualModel.Edges)
	}
	return summary
}

func summarizeADKStrategyInstance(item strategyListItem, definitionID string) map[string]any {
	symbols := append([]string(nil), item.Binding.Symbols...)
	activeSymbols := []string{}
	actualStatus := ""
	lastError := ""
	lastLog := ""
	if item.RuntimeObservation != nil {
		activeSymbols = append(activeSymbols, item.RuntimeObservation.ActiveSymbols...)
		actualStatus = strings.TrimSpace(item.RuntimeObservation.ActualStatus)
		if item.RuntimeObservation.LastError != nil {
			lastError = strings.TrimSpace(*item.RuntimeObservation.LastError)
		}
	}
	if len(item.Logs) > 0 {
		lastLog = strings.TrimSpace(item.Logs[len(item.Logs)-1])
	}
	return map[string]any{
		"id":                item.ID,
		"definitionId":      definitionID,
		"definitionName":    item.Definition.Name,
		"definitionVersion": item.Definition.Version,
		"runtime":           item.Runtime,
		"sourceFormat":      item.SourceFormat,
		"status":            item.Status,
		"actualStatus":      actualStatus,
		"startable":         item.Startable,
		"symbols":           symbols,
		"symbolCount":       len(symbols),
		"activeSymbols":     activeSymbols,
		"activeSymbolCount": len(activeSymbols),
		"interval":          item.Binding.Interval,
		"executionMode":     item.Binding.ExecutionMode,
		"market":            brokerBindingMarket(item.Binding.BrokerAccount),
		"accountId":         brokerBindingAccountID(item.Binding.BrokerAccount),
		"createdAt":         item.CreatedAt,
		"logCount":          len(item.Logs),
		"latestLog":         summarizeADKText(lastLog, 220),
		"lastError":         summarizeADKText(lastError, 220),
	}
}

func summarizeADKBacktestRuns(runs []*backtestRunState) map[string]any {
	items := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		if run == nil {
			continue
		}
		items = append(items, summarizeADKBacktestRun(run))
	}
	return map[string]any{"runs": items, "runCount": len(items)}
}

func summarizeADKBacktestRun(run *backtestRunState) map[string]any {
	summary := map[string]any{
		"id":                run.ID,
		"status":            run.Status,
		"definitionId":      run.Request.DefinitionID,
		"definitionVersion": run.Request.DefinitionVersion,
		"market":            run.Request.Market,
		"code":              run.Request.Code,
		"symbol":            run.Request.Symbol,
		"interval":          run.Request.Interval,
		"startTime":         run.Request.StartTime,
		"endTime":           run.Request.EndTime,
		"initialBalance":    run.Request.InitialBalance,
		"rehabType":         run.Request.RehabType,
		"createdAt":         run.CreatedAt,
		"updatedAt":         run.UpdatedAt,
	}
	if run.Request.UseExtendedHours != nil {
		summary["useExtendedHours"] = *run.Request.UseExtendedHours
	}
	if run.Result == nil {
		return summary
	}

	tradeCount := run.Result.TotalTrades
	totalReturn := 0.0
	if run.Request.InitialBalance > 0 {
		totalReturn = run.Result.PnL / run.Request.InitialBalance
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

func looksLikeTradingViewPineScript(script string) bool {
	for _, rawLine := range strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "//@version") ||
			strings.HasPrefix(lower, "strategy(") ||
			strings.HasPrefix(lower, "indicator(") ||
			strings.HasPrefix(lower, "study(") ||
			strings.Contains(lower, "ta.") ||
			strings.Contains(lower, "request.security(") {
			return true
		}
		if strings.HasPrefix(line, "//") {
			continue
		}
		break
	}
	return false
}

func nowStringRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func registerJFTradeADKReadTools(server *Server, registry *jfadk.ToolRegistry) {
	registry.Register(jfadk.ToolDescriptor{Name: "broker.orders", DisplayName: "经纪商订单", Description: "读取所选账户范围下经纪商当前或历史订单。", Category: "portfolio", Permission: "read_internal", OutputSummary: "经纪商订单列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		request, err := server.brokerOrdersRequest(brokerOrdersReadQuery{
			brokerBaseReadQuery: adkBrokerBaseReadQuery(server, input),
			Scope:               defaultStringLocal(stringValue(input, "scope"), "CURRENT"),
			Symbol:              stringValue(input, "symbol"),
			StartTime:           stringValue(input, "startTime"),
			EndTime:             stringValue(input, "endTime"),
			Status:              stringSliceValue(input, "status"),
			Statuses:            stringSliceValue(input, "statuses"),
		})
		if err != nil {
			return nil, err
		}
		return server.brokerOrdersResponse(ctx, request), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.fills", DisplayName: "经纪商成交", Description: "读取所选账户范围下经纪商当前或历史成交记录。", Category: "portfolio", Permission: "read_internal", OutputSummary: "经纪商成交列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		request, err := server.brokerFillsRequest(brokerFillsReadQuery{
			brokerBaseReadQuery: adkBrokerBaseReadQuery(server, input),
			Scope:               defaultStringLocal(stringValue(input, "scope"), "CURRENT"),
			Symbol:              stringValue(input, "symbol"),
			StartTime:           stringValue(input, "startTime"),
			EndTime:             stringValue(input, "endTime"),
		})
		if err != nil {
			return nil, err
		}
		return server.brokerFillsResponse(ctx, request), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.cash_flows", DisplayName: "资金流水", Description: "按清算日期读取经纪商资金流水记录。", Category: "portfolio", Permission: "read_internal", OutputSummary: "资金流水列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		request, err := server.brokerCashFlowsRequest(brokerCashFlowsReadQuery{brokerBaseReadQuery: adkBrokerBaseReadQuery(server, input), ClearingDate: stringValue(input, "clearingDate"), Direction: stringValue(input, "direction")})
		if err != nil {
			return nil, err
		}
		return server.brokerCashFlowsResponse(ctx, request), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.fees", DisplayName: "订单费用", Description: "按一个或多个外部订单号读取经纪商费用明细。", Category: "portfolio", Permission: "read_internal", OutputSummary: "订单费用列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		request, err := server.brokerOrderFeesRequest(brokerOrderFeesReadQuery{brokerBaseReadQuery: adkBrokerBaseReadQuery(server, input), OrderIDEx: stringSliceValue(input, "orderIdEx"), OrderIDExList: stringSliceValue(input, "orderIdExList")})
		if err != nil {
			return nil, err
		}
		return server.brokerOrderFeesResponse(ctx, request), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.margin_ratios", DisplayName: "融资融券比率", Description: "读取一个或多个标的的融资与融券保证金比率。", Category: "portfolio", Permission: "read_internal", OutputSummary: "融资融券比率列表与连接状态。"}, func(ctx context.Context, input map[string]any) (any, error) {
		symbols := stringSliceValue(input, "symbols")
		if symbol := strings.TrimSpace(stringValue(input, "symbol")); symbol != "" {
			symbols = append(symbols, symbol)
		}
		request, err := server.brokerMarginRatiosRequest(brokerMarginRatiosReadQuery{brokerBaseReadQuery: adkBrokerBaseReadQuery(server, input), Symbol: symbols})
		if err != nil {
			return nil, err
		}
		return server.brokerMarginRatiosResponse(ctx, request), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "market.depth", DisplayName: "盘口深度", Description: "读取指定标的的买卖盘深度。", Category: "market", Permission: "read_internal", OutputSummary: "买卖盘档位数据。"}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		return server.marketDepthResponseForInstrument(ctx, market, symbol, marketDepthQuery{Num: newOptionalIntValue(intValue(input, "num", 10))})
	})
	registry.Register(jfadk.ToolDescriptor{Name: "risk.state", DisplayName: "风险状态", Description: "读取实盘 kill switch 与风险限制状态。", Category: "risk", Permission: "read_internal", OutputSummary: "当前 kill switch 与实盘风险状态。"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"killSwitch": server.realTradeKillSwitch(), "riskLimits": server.realTradeRiskState(), "checkedAt": nowStringRFC3339Nano()}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "risk.events", DisplayName: "风险事件", Description: "读取近期实盘风险事件状态。", Category: "risk", Permission: "read_internal", OutputSummary: "风险事件摘要。"}, func(context.Context, map[string]any) (any, error) {
		return server.realTradeRiskEvents(), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "execution.order_events", DisplayName: "执行订单事件", Description: "按内部订单 ID 读取本地执行订单事件历史；未提供 ID 时返回订单列表。", Category: "portfolio", Permission: "read_internal", OutputSummary: "执行订单事件时间线。"}, func(_ context.Context, input map[string]any) (any, error) {
		internalOrderID := strings.TrimSpace(stringValue(input, "internalOrderId"))
		if internalOrderID == "" {
			return server.executionOrders.listOrders(), nil
		}
		return server.executionOrders.orderEvents(internalOrderID), nil
	})
}

func registerJFTradeADKWorkflowTools(server *Server, store *jfadk.Store, registry *jfadk.ToolRegistry) {
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.list", DisplayName: "ADK 任务列表", Description: "列出用于跟踪 agent 工作的 ADK 任务记录。", Category: "workflow", Permission: "read_internal", OutputSummary: "任务分页结果。"}, func(ctx context.Context, input map[string]any) (any, error) {
		limit, offset := normalizeBoundPage(intValue(input, "limit", 20), intValue(input, "offset", 0), 20, 100)
		tasks, total, err := store.ListTasksPage(ctx, stringValue(input, "status"), stringValue(input, "agentId"), stringValue(input, "runId"), limit, offset)
		if err != nil {
			return nil, err
		}
		return map[string]any{"tasks": tasks, "page": pageEnvelope(limit, offset, total, len(tasks))}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.create", DisplayName: "创建 ADK 任务", Description: "创建一个用于后续跟进的轻量 ADK 任务。", Category: "workflow", Permission: "write_task", OutputSummary: "已创建的任务。"}, func(ctx context.Context, input map[string]any) (any, error) {
		task, err := store.SaveTask(ctx, jfadk.TaskWriteRequest{Title: stringValue(input, "title"), Description: stringValue(input, "description"), Status: stringValue(input, "status"), AgentID: stringValue(input, "agentId"), RunID: stringValue(input, "runId"), DependsOn: stringSliceValue(input, "dependsOn")})
		if err == nil {
			recordADKWorkflowAudit(ctx, server, "task.saved", task.ID, "ADK task saved.", map[string]any{"status": task.Status})
		}
		return task, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.update", DisplayName: "更新 ADK 任务", Description: "更新轻量 ADK 任务的状态或详情。", Category: "workflow", Permission: "write_task", OutputSummary: "已更新的任务。"}, func(ctx context.Context, input map[string]any) (any, error) {
		task, err := store.UpdateTask(ctx, stringValue(input, "id"), taskPatchFromInput(input))
		if err == nil {
			recordADKWorkflowAudit(ctx, server, "task.updated", task.ID, "ADK task updated.", map[string]any{"status": task.Status})
		}
		return task, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.delete", DisplayName: "删除 ADK 任务", Description: "删除轻量 ADK 任务记录。", Category: "workflow", Permission: "write_task", OutputSummary: "删除结果。"}, func(ctx context.Context, input map[string]any) (any, error) {
		id := stringValue(input, "id")
		if err := store.DeleteTask(ctx, id); err != nil {
			return nil, err
		}
		recordADKWorkflowAudit(ctx, server, "task.deleted", id, "ADK task deleted.", nil)
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
			recordADKWorkflowAudit(ctx, server, "memory.saved", entry.ID, "ADK memory saved.", map[string]any{"scope": entry.Scope, "key": entry.Key})
		}
		return entry, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "memory.forget", DisplayName: "删除 ADK 记忆", Description: "从 ADK 数据库删除工作区或 agent 记忆条目。", Category: "workflow", Permission: "write_memory", OutputSummary: "删除结果。"}, func(ctx context.Context, input map[string]any) (any, error) {
		id := stringValue(input, "id")
		if err := store.DeleteMemory(ctx, id); err != nil {
			return nil, err
		}
		recordADKWorkflowAudit(ctx, server, "memory.deleted", id, "ADK memory deleted.", nil)
		return map[string]any{"deleted": true, "id": id}, nil
	})
}

func recordADKWorkflowAudit(ctx context.Context, server *Server, kind string, subjectID string, detail string, metadata map[string]any) {
	if server == nil || server.adkRuntime == nil {
		return
	}
	server.adkRuntime.RecordAudit(ctx, kind, subjectID, detail, metadata)
}

func taskPatchFromInput(input map[string]any) jfadk.TaskPatchRequest {
	return jfadk.TaskPatchRequest{
		Title:       stringPtrFromInput(input, "title"),
		Description: stringPtrFromInput(input, "description"),
		Status:      stringPtrFromInput(input, "status"),
		AgentID:     stringPtrFromInput(input, "agentId"),
		RunID:       stringPtrFromInput(input, "runId"),
		DependsOn:   stringSliceFromPresentInput(input, "dependsOn"),
	}
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
		empty := ""
		return &empty
	default:
		text := fmt.Sprint(typed)
		return &text
	}
}

func stringSliceFromPresentInput(input map[string]any, key string) []string {
	if _, ok := input[key]; !ok {
		return nil
	}
	return stringSliceValue(input, key)
}

func adkBrokerBaseReadQuery(server *Server, input map[string]any) brokerBaseReadQuery {
	market := stringValue(input, "market")
	if market == "" && server != nil && server.store != nil {
		market = server.store.integration().Config.TradeMarket
	}
	return brokerBaseReadQuery{TradingEnvironment: stringValue(input, "tradingEnvironment"), AccountID: stringValue(input, "accountId"), Market: market}
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
	value, _ := input[key].(string)
	return value
}

func intValue(input map[string]any, key string, fallback int) int {
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
	return fallback
}

func floatValue(input map[string]any, key string, fallback float64) float64 {
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
	return fallback
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

func defaultStringLocal(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
