package servercore

import (
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	instancebinding "github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
)

func normalizeStrategyRuntimeRiskSettings(input stratsrv.RuntimeRiskSettings) stratsrv.RuntimeRiskSettings {
	return instancebinding.NormalizeRiskSettings(input)
}

func normalizeStrategyInstanceBinding(input stratsrv.InstanceBinding, params map[string]any) stratsrv.InstanceBinding {
	return instancebinding.NormalizeBinding(input, params)
}

func applyStrategyBindingParams(input *stratsrv.ManagedInstance) {
	instancebinding.ApplyParams((*stratsrv.ManagedInstance)(input))
}

func strategyRuntimeRiskAuditDetail(input stratsrv.RuntimeRiskSettings) string {
	return instancebinding.RiskAuditDetail(input)
}

func strategyBindingAuditDetail(definitionID string, binding stratsrv.InstanceBinding) string {
	return instancebinding.BindingAuditDetail(definitionID, binding)
}
