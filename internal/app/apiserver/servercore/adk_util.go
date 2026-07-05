package servercore

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/backtest"
)

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

func waitForADKKLineSyncProgress(ctx context.Context, deps ToolDeps, taskID string, waitMs int) (*backtest.SyncProgress, bool) {
	progress, ok := deps.BacktestKLineSyncProgress(taskID)
	if !ok || waitMs <= 0 || isTerminalKLineSyncStatus(progress.Status) {
		return progress, ok
	}
	deadline := time.NewTimer(time.Duration(waitMs) * time.Millisecond)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return progress, true
		case <-deadline.C:
			return progress, true
		case <-ticker.C:
			next, found := deps.BacktestKLineSyncProgress(taskID)
			if !found {
				return nil, false
			}
			progress = next
			if isTerminalKLineSyncStatus(progress.Status) {
				return progress, true
			}
		}
	}
}

func isTerminalKLineSyncStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func klineSyncProgressPayload(progress *backtest.SyncProgress) map[string]any {
	if progress == nil {
		return map[string]any{"status": "unknown", "readyToRetry": false}
	}
	return map[string]any{
		"taskId": progress.TaskID, "status": progress.Status, "symbol": progress.Symbol,
		"currentInterval": progress.CurrentInterval, "totalIntervals": progress.TotalIntervals,
		"completedIntervals": progress.CompletedIntervals, "totalBatches": progress.TotalBatches,
		"completedBatches": progress.CompletedBatches, "retries": progress.Retries,
		"error": progress.Error, "startedAt": progress.StartedAt, "updatedAt": progress.UpdatedAt,
		"readyToRetry": strings.EqualFold(progress.Status, "completed"),
	}
}

func backtestDataReadinessPayload(readiness BacktestDataReadiness) map[string]any {
	payload := map[string]any{
		"ok":         readiness.Status == "syncing_data",
		"status":     readiness.Status,
		"nextAction": "等待 K 线同步完成后，使用完全相同的参数重试原回测工具。",
	}
	if readiness.Error != "" {
		payload["error"] = readiness.Error
	}
	if readiness.DataSync != nil {
		payload["dataSync"] = map[string]any{
			"taskId": readiness.DataSync.TaskID, "symbol": readiness.DataSync.Symbol,
			"intervals": readiness.DataSync.Intervals, "since": readiness.DataSync.Since,
			"until": readiness.DataSync.Until, "sessionScope": readiness.DataSync.SessionScope,
			"status": readiness.DataSync.Status,
		}
		payload["nextTool"] = map[string]any{"name": "backtest.kline_sync_status", "input": map[string]any{"taskId": readiness.DataSync.TaskID, "waitForCompletionMs": 25000}}
	}
	if readiness.Progress != nil {
		payload["progress"] = klineSyncProgressPayload(readiness.Progress)
	}
	if readiness.Status != "syncing_data" {
		payload["nextAction"] = "停止自动重试并向用户说明 K 线同步失败或同步后覆盖仍不足。"
	}
	return payload
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
