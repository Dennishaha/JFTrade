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
	registerJFTradeADKTools(server, registry)
	sessionService, err := jfadk.NewSQLiteSessionService(sessionDBPath)
	if err != nil {
		log.Printf("JFTrade ADK session store degraded: %v", err)
		return jfadk.NewRuntime(store, registry)
	}
	if err := jfadk.MigrateSQLiteSessionService(sessionService); err != nil {
		log.Printf("JFTrade ADK session migration degraded: %v", err)
		_ = sessionService.Close()
		return jfadk.NewRuntime(store, registry)
	}
	return jfadk.NewRuntimeWithSessionService(store, registry, sessionService)
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

func registerJFTradeADKTools(server *Server, registry *jfadk.ToolRegistry) {
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
		return server.marketSnapshotResponse(ctx, "/api/v1/market-data/snapshots/"+market+"/"+symbol, map[string][]string{"refresh": {"false"}})
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
		return server.marketCandlesResponse(ctx, "/api/v1/market-data/candles/"+market+"/"+symbol, map[string][]string{"period": {period}, "limit": {strconv.Itoa(limit)}})
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
