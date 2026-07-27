package assembly

import (
	"encoding/json"
	"fmt"
	"strings"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/instanceview"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func (a *ApplicationAdapter) strategyDefinitionSummaries() []StrategyDefinitionSummary {
	service := a.strategy()
	if service == nil {
		return nil
	}
	definitions := service.ListDefinitions()
	out := make([]StrategyDefinitionSummary, 0, len(definitions))
	for _, definition := range definitions {
		summary := StrategyDefinitionSummary{
			ID: definition.ID, Name: definition.Name, Version: definition.Version, Description: definition.Description,
			Runtime: definition.Runtime, SourceFormat: definition.SourceFormat, Symbol: definition.Symbol, Interval: definition.Interval,
			Script: definition.Script, CreatedAt: definition.CreatedAt, UpdatedAt: definition.UpdatedAt,
		}
		if definition.VisualModel != nil {
			summary.VisualNodeCount = len(definition.VisualModel.Nodes)
			summary.VisualEdgeCount = len(definition.VisualModel.Edges)
		}
		out = append(out, summary)
	}
	return out
}

func (a *ApplicationAdapter) listStrategyDefinitionVersions(
	definitionID string,
) ([]stratsrv.DefinitionVersionSummary, bool, error) {
	service := a.strategy()
	if service == nil {
		return nil, false, fmt.Errorf("strategy definition service is unavailable")
	}
	return service.ListDefinitionVersions(definitionID)
}

func (a *ApplicationAdapter) getStrategyDefinitionVersion(
	definitionID string,
	version string,
) (stratsrv.DefinitionVersion, bool, error) {
	service := a.strategy()
	if service == nil {
		return stratsrv.DefinitionVersion{}, false, fmt.Errorf("strategy definition service is unavailable")
	}
	return service.GetDefinitionVersion(definitionID, version)
}

func (a *ApplicationAdapter) strategyInstanceSummaries() []StrategyInstanceSummary {
	service := a.strategy()
	if service == nil {
		return nil
	}
	items := service.ListInstances()
	out := make([]StrategyInstanceSummary, 0, len(items))
	for _, item := range items {
		out = append(out, strategyInstanceSummary(item))
	}
	return out
}

func strategyInstanceSummary(item stratsrv.InstanceView) StrategyInstanceSummary {
	activeSymbols := []string{}
	actualStatus := ""
	lastError := ""
	if item.RuntimeObservation != nil {
		activeSymbols = append(activeSymbols, item.RuntimeObservation.ActiveSymbols...)
		actualStatus = strings.TrimSpace(item.RuntimeObservation.ActualStatus)
		if item.RuntimeObservation.LastError != nil {
			lastError = strings.TrimSpace(*item.RuntimeObservation.LastError)
		}
	}
	lastLog := ""
	if len(item.Logs) > 0 {
		lastLog = strings.TrimSpace(item.Logs[len(item.Logs)-1])
	}
	return StrategyInstanceSummary{
		ID: item.ID, DefinitionID: strategySummaryDefinitionID(item), DefinitionName: item.Definition.Name,
		DefinitionVersion: item.Definition.Version, Runtime: item.Runtime, SourceFormat: item.SourceFormat,
		Status: item.Status, ActualStatus: actualStatus, Startable: item.Startable,
		Symbols: append([]string(nil), item.Binding.Symbols...), ActiveSymbols: activeSymbols,
		Interval: item.Binding.Interval, ExecutionMode: item.Binding.ExecutionMode,
		Market: brokerBindingMarket(item.Binding.BrokerAccount), AccountID: brokerBindingAccountID(item.Binding.BrokerAccount),
		CreatedAt: item.CreatedAt, LogCount: len(item.Logs), LatestLog: lastLog, LastError: lastError,
	}
}

func strategySummaryDefinitionID(item stratsrv.InstanceView) string {
	if definitionID := strings.TrimSpace(item.Definition.StrategyID); definitionID != "" {
		return definitionID
	}
	return instanceview.DefinitionIDFromParams(item.Params)
}

func (a *ApplicationAdapter) saveStrategyDraft(input StrategyDraftInput) (any, error) {
	service := a.strategy()
	if service == nil {
		return nil, fmt.Errorf("strategy definition service is unavailable")
	}
	symbol, interval := validationInstrument(input.Validation)
	return service.SaveDefinition(stratsrv.Definition{
		Name:         StringOrDefault(input.Name, "ADK 策略草稿"),
		Description:  "由 ADK agent 生成的策略草稿。",
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Runtime:      stratsrv.RuntimePinePlan,
		Version:      stratsrv.DefaultVersion,
		Symbol:       symbol,
		Interval:     interval,
		Script:       input.Validation.NormalizedScript,
	})
}

func (a *ApplicationAdapter) saveStrategyDefinition(input StrategyDefinitionInput) (any, error) {
	service := a.strategy()
	if service == nil {
		return nil, fmt.Errorf("strategy definition service is unavailable")
	}
	definitionID := strings.TrimSpace(input.DefinitionID)
	if err := ensureStrategyDefinitionExists(service, definitionID); err != nil {
		return nil, err
	}
	visualModel, err := strategyVisualModelFromInput(input.VisualModel)
	if err != nil {
		return nil, err
	}
	symbol, interval := validationInstrument(input.Validation)
	return service.SaveDefinition(stratsrv.Definition{
		ID: definitionID, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description),
		Runtime: stratsrv.RuntimePinePlan, SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:   StringOrDefault(strings.TrimSpace(input.Symbol), symbol),
		Interval: StringOrDefault(strings.TrimSpace(input.Interval), interval),
		Script:   input.Validation.NormalizedScript, VisualModel: visualModel,
	})
}

func ensureStrategyDefinitionExists(service *stratsrv.Service, definitionID string) error {
	if definitionID == "" {
		return nil
	}
	_, ok, err := service.GetDefinition(definitionID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("策略定义 %q 不存在", definitionID)
	}
	return nil
}

func (a *ApplicationAdapter) updateStrategyInstanceMode(instanceID string, executionMode string) (any, error) {
	service := a.strategy()
	if service == nil {
		return nil, fmt.Errorf("strategy service is unavailable")
	}
	current, ok := service.GetInstance(instanceID)
	if !ok {
		return nil, fmt.Errorf("策略实例 %q 不存在", instanceID)
	}
	binding := current.Binding
	binding.ExecutionMode = executionMode
	return service.UpdateInstance(instanceID, binding)
}

func strategyVisualModelFromInput(value any) (*stratsrv.VisualModel, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("visualModel must be a valid object: %w", err)
	}
	var model stratsrv.VisualModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("visualModel must be a valid object: %w", err)
	}
	return stratsrv.NormalizeVisualModel(&model)
}

func validationInstrument(validation StrategyPineValidation) (string, string) {
	if validation.Program == nil {
		return "", ""
	}
	return strings.TrimSpace(validation.Program.Metadata.Symbol), strings.TrimSpace(validation.Program.Metadata.Interval)
}

func brokerBindingMarket(binding *stratsrv.BrokerAccountBinding) string {
	if binding == nil {
		return ""
	}
	return binding.Market
}

func brokerBindingAccountID(binding *stratsrv.BrokerAccountBinding) string {
	if binding == nil {
		return ""
	}
	return binding.AccountID
}
