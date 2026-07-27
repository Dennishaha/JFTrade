package servercore

import (
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
)

type ToolDeps = assistantassembly.ToolDeps
type WatchlistListInput = assistantassembly.WatchlistListInput
type BrokerReadInput = assistantassembly.BrokerReadInput
type StrategyPineValidation = assistantassembly.StrategyPineValidation
type StrategyDefinitionSummary = assistantassembly.StrategyDefinitionSummary
type StrategyInstanceSummary = assistantassembly.StrategyInstanceSummary
type StrategyDraftInput = assistantassembly.StrategyDraftInput
type StrategyDefinitionInput = assistantassembly.StrategyDefinitionInput
type BacktestStartInput = assistantassembly.BacktestStartInput
type ResearchBacktestInput = assistantassembly.ResearchBacktestInput
type BacktestResultViewInput = assistantassembly.BacktestResultViewInput
type BacktestRunRef = assistantassembly.BacktestRunRef
type BacktestDataReadiness = assistantassembly.BacktestDataReadiness
type BacktestDataSync = assistantassembly.BacktestDataSync
type BacktestRunSummary = assistantassembly.BacktestRunSummary
type WorkflowToolPage[T any] = assistantassembly.WorkflowToolPage[T]
type WorkflowToolStartResult = assistantassembly.WorkflowToolStartResult
type WorkflowToolManager = assistantassembly.WorkflowToolManager
