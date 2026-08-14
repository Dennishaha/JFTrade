package skillsruntime

func strategyToolInputSchema(name string) (map[string]any, bool) {
	switch name {
	case "strategy.optimize":
		return strategyOptimizeInputSchema(), true
	case "strategy.research_backtest":
		return strategyResearchBacktestInputSchema(), true
	case "strategy.instantiate":
		return strategyInstantiateInputSchema(), true
	case "strategy.instance_start":
		return strategyInstanceIDInputSchema(), true
	case "strategy.instance_stop":
		return strategyInstanceStopInputSchema(), true
	case "strategy.instance_refresh_definition":
		return strategyInstanceIDInputSchema(), true
	case "strategy.instance_risk.update":
		return strategyInstanceRiskInputSchema(), true
	case "strategy.instance_activity":
		return strategyInstanceActivityInputSchema(), true
	case "backtest.cancel":
		return map[string]any{"type": "object", "properties": map[string]any{"runId": map[string]any{"type": "string", "minLength": 1}}, "required": []string{"runId"}, "additionalProperties": false}, true
	case "strategy.definition_versions.list":
		return strategyDefinitionVersionsListInputSchema(), true
	case "strategy.definition_versions.get":
		return strategyDefinitionVersionsGetInputSchema(), true
	case "backtest.runs":
		return backtestRunsInputSchema(), true
	case "backtest.result_view":
		return backtestResultViewInputSchema(), true
	case "backtest.kline_sync_status":
		return backtestKLineSyncStatusInputSchema(), true
	case "strategy.pine_spec":
		return strategyPineSpecInputSchema(), true
	case "strategy.validate_pine":
		return strategyValidatePineInputSchema(), true
	case "strategy.save_draft":
		return strategySaveDraftInputSchema(), true
	case "strategy.save_definition":
		return strategySaveDefinitionInputSchema(), true
	case "strategy.update_instance_mode":
		return strategyUpdateInstanceModeInputSchema(), true
	default:
		return nil, false
	}
}
