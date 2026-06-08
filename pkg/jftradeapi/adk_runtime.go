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
	"github.com/jftrade/jftrade-main/pkg/broker"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
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
		return map[string]any{"definitions": definitions, "definitionCount": len(definitions), "instances": instances, "instanceCount": len(instances)}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "strategy.save_draft", DisplayName: "保存策略草稿", Description: "把 agent 生成的 DSL 保存为策略定义草稿。", Category: "strategy", Permission: "write_strategy", RequiresApprovalIn: []string{jfadk.PermissionModeApproval}, OutputSummary: "保存后的策略定义。"}, func(_ context.Context, input map[string]any) (any, error) {
		script := strings.TrimSpace(stringValue(input, "script"))
		if script == "" {
			script = "# ADK strategy draft\n"
		}
		definition := strategyDesignDefinition{
			Name:         defaultStringLocal(stringValue(input, "name"), "ADK 策略草稿"),
			Description:  "由 ADK agent 生成的策略草稿。",
			SourceFormat: strategydefinition.SourceFormatDSLV1,
			Runtime:      strategyRuntimeDSLPlan,
			Version:      defaultStrategyVersion,
			Script:       script,
		}
		if err := strategydefinition.ValidateScript(definition.SourceFormat, definition.Script); err != nil {
			return nil, err
		}
		saved, err := server.designStore.saveDefinition(definition)
		if err != nil {
			return nil, err
		}
		return saved, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "backtest.runs", DisplayName: "回测结果", Description: "读取最近回测运行结果。", Category: "strategy", Permission: "read_internal", OutputSummary: "最近回测运行和数量。"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"runs": server.backtestRuns.list()}, nil
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
				InitialBalance: floatValue(input, "initialBalance", 100000),
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

func nowStringRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func registerJFTradeADKReadTools(server *Server, registry *jfadk.ToolRegistry) {
	registry.Register(jfadk.ToolDescriptor{Name: "broker.orders", DisplayName: "Broker orders", Description: "Read broker current or historical orders for the selected account scope.", Category: "portfolio", Permission: "read_internal", OutputSummary: "Broker order list and connectivity status."}, func(ctx context.Context, input map[string]any) (any, error) {
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
	registry.Register(jfadk.ToolDescriptor{Name: "broker.fills", DisplayName: "Broker fills", Description: "Read broker current or historical order fills for the selected account scope.", Category: "portfolio", Permission: "read_internal", OutputSummary: "Broker fill list and connectivity status."}, func(ctx context.Context, input map[string]any) (any, error) {
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
	registry.Register(jfadk.ToolDescriptor{Name: "broker.cash_flows", DisplayName: "Broker cash flows", Description: "Read broker cash flow records by clearing date.", Category: "portfolio", Permission: "read_internal", OutputSummary: "Cash flow list and connectivity status."}, func(ctx context.Context, input map[string]any) (any, error) {
		request, err := server.brokerCashFlowsRequest(brokerCashFlowsReadQuery{brokerBaseReadQuery: adkBrokerBaseReadQuery(server, input), ClearingDate: stringValue(input, "clearingDate"), Direction: stringValue(input, "direction")})
		if err != nil {
			return nil, err
		}
		return server.brokerCashFlowsResponse(ctx, request), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.fees", DisplayName: "Broker order fees", Description: "Read broker fee details for one or more external order IDs.", Category: "portfolio", Permission: "read_internal", OutputSummary: "Order fee list and connectivity status."}, func(ctx context.Context, input map[string]any) (any, error) {
		request, err := server.brokerOrderFeesRequest(brokerOrderFeesReadQuery{brokerBaseReadQuery: adkBrokerBaseReadQuery(server, input), OrderIDEx: stringSliceValue(input, "orderIdEx"), OrderIDExList: stringSliceValue(input, "orderIdExList")})
		if err != nil {
			return nil, err
		}
		return server.brokerOrderFeesResponse(ctx, request), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "broker.margin_ratios", DisplayName: "Broker margin ratios", Description: "Read financing and short-selling margin ratios for one or more symbols.", Category: "portfolio", Permission: "read_internal", OutputSummary: "Margin ratio list and connectivity status."}, func(ctx context.Context, input map[string]any) (any, error) {
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
	registry.Register(jfadk.ToolDescriptor{Name: "market.depth", DisplayName: "Market depth", Description: "Read order book depth for a concrete instrument.", Category: "market", Permission: "read_internal", OutputSummary: "Bid and ask order book levels."}, func(ctx context.Context, input map[string]any) (any, error) {
		market, symbol := inferMarketSymbol(input)
		if market == "" || symbol == "" {
			return nil, fmt.Errorf("market and symbol are required")
		}
		return server.marketDepthResponseForInstrument(ctx, market, symbol, marketDepthQuery{Num: newOptionalIntValue(intValue(input, "num", 10))})
	})
	registry.Register(jfadk.ToolDescriptor{Name: "risk.state", DisplayName: "Risk state", Description: "Read real-trade kill switch and risk limit state.", Category: "risk", Permission: "read_internal", OutputSummary: "Current kill switch and real-trade risk state."}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"killSwitch": server.realTradeKillSwitch(), "riskLimits": server.realTradeRiskState(), "checkedAt": nowStringRFC3339Nano()}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "risk.events", DisplayName: "Risk events", Description: "Read recent real-trade risk event state.", Category: "risk", Permission: "read_internal", OutputSummary: "Risk event summary."}, func(context.Context, map[string]any) (any, error) {
		return server.realTradeRiskEvents(), nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "execution.order_events", DisplayName: "Execution order events", Description: "Read local execution order event history by internal order ID, or return the order list when no ID is provided.", Category: "portfolio", Permission: "read_internal", OutputSummary: "Execution order event timeline."}, func(_ context.Context, input map[string]any) (any, error) {
		internalOrderID := strings.TrimSpace(stringValue(input, "internalOrderId"))
		if internalOrderID == "" {
			return server.executionOrders.listOrders(), nil
		}
		return server.executionOrders.orderEvents(internalOrderID), nil
	})
}

func registerJFTradeADKWorkflowTools(server *Server, store *jfadk.Store, registry *jfadk.ToolRegistry) {
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.list", DisplayName: "List ADK tasks", Description: "List ADK task records used to track agent work.", Category: "workflow", Permission: "read_internal", OutputSummary: "Task page."}, func(ctx context.Context, input map[string]any) (any, error) {
		limit, offset := normalizeBoundPage(intValue(input, "limit", 20), intValue(input, "offset", 0), 20, 100)
		tasks, total, err := store.ListTasksPage(ctx, stringValue(input, "status"), stringValue(input, "agentId"), stringValue(input, "runId"), limit, offset)
		if err != nil {
			return nil, err
		}
		return map[string]any{"tasks": tasks, "page": pageEnvelope(limit, offset, total, len(tasks))}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.create", DisplayName: "Create ADK task", Description: "Create a lightweight ADK task for follow-up work.", Category: "workflow", Permission: "write_task", OutputSummary: "Created task."}, func(ctx context.Context, input map[string]any) (any, error) {
		task, err := store.SaveTask(ctx, jfadk.TaskWriteRequest{Title: stringValue(input, "title"), Description: stringValue(input, "description"), Status: stringValue(input, "status"), AgentID: stringValue(input, "agentId"), RunID: stringValue(input, "runId"), DependsOn: stringSliceValue(input, "dependsOn")})
		if err == nil {
			recordADKWorkflowAudit(ctx, server, "task.saved", task.ID, "ADK task saved.", map[string]any{"status": task.Status})
		}
		return task, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.update", DisplayName: "Update ADK task", Description: "Update a lightweight ADK task status or details.", Category: "workflow", Permission: "write_task", OutputSummary: "Updated task."}, func(ctx context.Context, input map[string]any) (any, error) {
		task, err := store.UpdateTask(ctx, stringValue(input, "id"), taskPatchFromInput(input))
		if err == nil {
			recordADKWorkflowAudit(ctx, server, "task.updated", task.ID, "ADK task updated.", map[string]any{"status": task.Status})
		}
		return task, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "tasks.delete", DisplayName: "Delete ADK task", Description: "Delete a lightweight ADK task record.", Category: "workflow", Permission: "write_task", OutputSummary: "Deleted task."}, func(ctx context.Context, input map[string]any) (any, error) {
		id := stringValue(input, "id")
		if err := store.DeleteTask(ctx, id); err != nil {
			return nil, err
		}
		recordADKWorkflowAudit(ctx, server, "task.deleted", id, "ADK task deleted.", nil)
		return map[string]any{"deleted": true, "id": id}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "memory.list", DisplayName: "List ADK memory", Description: "List workspace and agent memory entries from the ADK database.", Category: "workflow", Permission: "read_internal", OutputSummary: "Memory entries."}, func(ctx context.Context, input map[string]any) (any, error) {
		entries, err := store.ListMemoryFiltered(ctx, stringValue(input, "scope"), stringValue(input, "agentId"), stringValue(input, "key"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"entries": entries, "totalReturned": len(entries)}, nil
	})
	registry.Register(jfadk.ToolDescriptor{Name: "memory.remember", DisplayName: "Remember ADK preference", Description: "Store a short workspace or agent memory entry in the ADK database.", Category: "workflow", Permission: "write_memory", OutputSummary: "Saved memory entry."}, func(ctx context.Context, input map[string]any) (any, error) {
		entry, err := store.SaveMemory(ctx, jfadk.MemoryWriteRequest{AgentID: stringValue(input, "agentId"), Key: stringValue(input, "key"), Value: stringValue(input, "value"), Scope: stringValue(input, "scope")})
		if err == nil {
			recordADKWorkflowAudit(ctx, server, "memory.saved", entry.ID, "ADK memory saved.", map[string]any{"scope": entry.Scope, "key": entry.Key})
		}
		return entry, err
	})
	registry.Register(jfadk.ToolDescriptor{Name: "memory.forget", DisplayName: "Forget ADK memory", Description: "Delete a workspace or agent memory entry from the ADK database.", Category: "workflow", Permission: "write_memory", OutputSummary: "Deleted memory entry."}, func(ctx context.Context, input map[string]any) (any, error) {
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
