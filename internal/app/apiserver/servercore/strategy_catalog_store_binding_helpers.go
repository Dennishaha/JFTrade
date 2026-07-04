package servercore

import (
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	instancebinding "github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
)

func normalizeStrategyRuntimeRiskSettings(input strategyRuntimeRiskSettings) strategyRuntimeRiskSettings {
	return instancebinding.NormalizeRiskSettings(input)
}

func normalizeStrategyInstanceBinding(input strategyInstanceBinding, params map[string]any) strategyInstanceBinding {
	return instancebinding.NormalizeBinding(input, params)
}

func applyStrategyBindingParams(input *managedStrategyInstance) {
	instancebinding.ApplyParams((*stratsrv.ManagedInstance)(input))
}

func strategyRuntimeRiskAuditDetail(input strategyRuntimeRiskSettings) string {
	return instancebinding.RiskAuditDetail(input)
}

func strategyBindingAuditDetail(definitionID string, binding strategyInstanceBinding) string {
	return instancebinding.BindingAuditDetail(definitionID, binding)
}
