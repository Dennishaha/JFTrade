package servercore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineengine"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

type StrategyPineValidation struct {
	NormalizedScript string
	Program          *strategyir.Program
	Requirements     strategyir.Requirements
	Warnings         []string
}

func recordADKWorkflowAudit(ctx context.Context, deps ToolDeps, kind string, subjectID string, detail string, metadata map[string]any) {
	if deps.RecordAudit != nil {
		deps.RecordAudit(ctx, kind, subjectID, detail, metadata)
	}
}

func StrategyPineSpecToolPayload(input map[string]any) (map[string]any, error) {
	return strategypinespec.BuildToolPayload(stringValue(input, "section"), boolInputValue(input, "includeExamples"))
}

func StrategyValidatePineToolPayload(input map[string]any) map[string]any {
	script := strings.TrimSpace(stringValue(input, "script"))
	includeRequirements := boolInputValueDefault(input, "includeRequirements", true)
	payload := map[string]any{"ok": false, "sourceFormat": strategypinespec.SourceFormat, "runtime": strategypinespec.Runtime, "externalEngine": pineengine.PayloadMap(pineengine.DisabledPayload()), "normalizedScript": script, "metadata": strategyMetadataPayload(nil), "hooks": []string{}, "requirements": nil, "warnings": []string{}, "errors": []string{}, "saveHint": nil}
	if script == "" {
		payload["errors"] = []string{"script 是必填项"}
		payload["saveHint"] = StrategySaveHintPayload()
		return payload
	}
	payload["externalEngine"] = pineengine.PayloadMap(pineengine.ShadowPayloadForScript(script))
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
	metadata := map[string]any{
		"name":            "",
		"version":         "",
		"symbol":          "",
		"interval":        "",
		"defaultQtyMode":  "fixed",
		"defaultQtyValue": "1",
		"pyramiding":      1,
		"risk":            map[string]any{},
	}
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
	risk := map[string]any{}
	if program.Metadata.AllowedEntryDirection != "" && program.Metadata.AllowedEntryDirection != "all" {
		risk["allowedEntryDirection"] = strings.TrimSpace(program.Metadata.AllowedEntryDirection)
	}
	if program.Metadata.MaxDrawdownValue > 0 {
		risk["maxDrawdown"] = map[string]any{
			"value":        program.Metadata.MaxDrawdownValue,
			"type":         strings.TrimSpace(program.Metadata.MaxDrawdownType),
			"alertMessage": strings.TrimSpace(program.Metadata.MaxDrawdownAlert),
		}
	}
	if program.Metadata.MaxIntradayLossValue > 0 {
		risk["maxIntradayLoss"] = map[string]any{
			"value":        program.Metadata.MaxIntradayLossValue,
			"type":         strings.TrimSpace(program.Metadata.MaxIntradayLossType),
			"alertMessage": strings.TrimSpace(program.Metadata.MaxIntradayLossAlert),
		}
	}
	if program.Metadata.MaxIntradayFilledOrders > 0 {
		risk["maxIntradayFilledOrders"] = map[string]any{
			"count":        program.Metadata.MaxIntradayFilledOrders,
			"alertMessage": strings.TrimSpace(program.Metadata.MaxIntradayFilledOrdersAlert),
		}
	}
	if program.Metadata.MaxPositionSize > 0 {
		risk["maxPositionSize"] = program.Metadata.MaxPositionSize
	}
	if program.Metadata.MaxConsLossDays > 0 {
		risk["maxConsLossDays"] = map[string]any{
			"count":        program.Metadata.MaxConsLossDays,
			"alertMessage": strings.TrimSpace(program.Metadata.MaxConsLossDaysAlert),
		}
	}
	metadata["risk"] = risk
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

func SourceFormatPineV6() string { return strategydefinition.SourceFormatPineV6 }
