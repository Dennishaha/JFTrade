package jftradeapi

import (
	"encoding/json"
	"fmt"
	"strings"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
	strategydslspec "github.com/jftrade/jftrade-main/pkg/strategy/dslspec"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type strategyDSLValidation struct {
	NormalizedScript string
	Program          *strategyir.Program
	Requirements     strategyir.Requirements
}

func strategyDSLSpecToolPayload(input map[string]any) (map[string]any, error) {
	return strategydslspec.BuildToolPayload(
		stringValue(input, "section"),
		boolInputValue(input, "includeExamples"),
	)
}

func strategyValidateDSLToolPayload(input map[string]any) map[string]any {
	script := strings.TrimSpace(stringValue(input, "script"))
	includeRequirements := boolInputValueDefault(input, "includeRequirements", true)
	payload := map[string]any{
		"ok":               false,
		"sourceFormat":     strategydslspec.SourceFormat,
		"runtime":          strategydslspec.Runtime,
		"normalizedScript": script,
		"metadata":         strategyMetadataPayload(nil),
		"hooks":            []string{},
		"requirements":     nil,
		"errors":           []string{},
		"saveHint":         nil,
	}
	if script == "" {
		payload["errors"] = []string{"script 是必填项"}
		payload["saveHint"] = strategySaveHintPayload()
		return payload
	}
	validation, err := validateADKStrategyScript("strategy.validate_dsl", script)
	if err != nil {
		payload["errors"] = []string{err.Error()}
		payload["saveHint"] = strategySaveHintPayload()
		return payload
	}
	payload["ok"] = true
	payload["normalizedScript"] = validation.NormalizedScript
	payload["metadata"] = strategyMetadataPayload(validation.Program)
	payload["hooks"] = buildCompiledHookKinds(validation.Program)
	if includeRequirements {
		payload["requirements"] = buildCompiledRequirementsPayload(validation.Requirements)
	}
	return payload
}

func validateADKStrategyDraftScript(script string) error {
	if strings.TrimSpace(script) == "" {
		return nil
	}
	_, err := validateADKStrategyScript("strategy.save_draft", script)
	return err
}

func validateADKStrategyScript(toolName string, script string) (strategyDSLValidation, error) {
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return strategyDSLValidation{}, fmt.Errorf("%s 需要提供非空的 JFTrade DSL v1 脚本", strings.TrimSpace(toolName))
	}
	if looksLikeTradingViewPineScript(trimmed) {
		return strategyDSLValidation{}, fmt.Errorf("%s 只接受 JFTrade DSL v1，不支持 TradingView Pine Script。%s", strings.TrimSpace(toolName), strategydslspec.SaveDraftUsageHint())
	}
	if err := strategydefinition.ValidateScript(strategydefinition.SourceFormatDSLV1, trimmed); err != nil {
		return strategyDSLValidation{}, fmt.Errorf("%s 需要合法的 JFTrade DSL v1：%w\n\n%s", strings.TrimSpace(toolName), err, strategydslspec.SaveDraftUsageHint())
	}
	program, err := strategydsl.ParseScript(trimmed)
	if err != nil {
		return strategyDSLValidation{}, fmt.Errorf("%s 需要合法的 JFTrade DSL v1：%w\n\n%s", strings.TrimSpace(toolName), err, strategydslspec.SaveDraftUsageHint())
	}
	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		return strategyDSLValidation{}, fmt.Errorf("%s 需要可规划的 JFTrade DSL v1：%w\n\n%s", strings.TrimSpace(toolName), err, strategydslspec.SaveDraftUsageHint())
	}
	return strategyDSLValidation{
		NormalizedScript: trimmed,
		Program:          program,
		Requirements:     requirements,
	}, nil
}

func strategySaveDefinitionToolPayload(server *Server, input map[string]any) (map[string]any, error) {
	if server == nil || server.designStore == nil {
		return nil, fmt.Errorf("策略定义存储不可用")
	}
	name := strings.TrimSpace(stringValue(input, "name"))
	if name == "" {
		return nil, fmt.Errorf("name 是必填项")
	}
	validation, err := validateADKStrategyScript("strategy.save_definition", stringValue(input, "script"))
	if err != nil {
		return nil, err
	}
	definitionID := strings.TrimSpace(stringValue(input, "definitionId"))
	operation := "created"
	if definitionID != "" {
		if _, ok := server.designStore.definition(definitionID); !ok {
			return nil, fmt.Errorf("策略定义 %q 不存在", definitionID)
		}
		operation = "updated"
	}
	visualModel, err := strategyVisualModelFromInput(input["visualModel"])
	if err != nil {
		return nil, err
	}
	definition := strategyDesignDefinition{
		ID:           definitionID,
		Name:         name,
		Description:  strings.TrimSpace(stringValue(input, "description")),
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Symbol:       defaultStringLocal(strings.TrimSpace(stringValue(input, "symbol")), strings.TrimSpace(validation.Program.Metadata.Symbol)),
		Interval:     defaultStringLocal(strings.TrimSpace(stringValue(input, "interval")), strings.TrimSpace(validation.Program.Metadata.Interval)),
		Script:       validation.NormalizedScript,
		VisualModel:  visualModel,
	}
	saved, err := server.designStore.saveDefinition(definition)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"operation":  operation,
		"definition": saved,
	}, nil
}

func strategySaveDraftToolPayload(server *Server, input map[string]any) (strategyDesignDefinition, error) {
	if server == nil || server.designStore == nil {
		return strategyDesignDefinition{}, fmt.Errorf("策略定义存储不可用")
	}
	script := strings.TrimSpace(stringValue(input, "script"))
	if script == "" {
		script = "# ADK strategy draft\n"
	}
	validation, err := validateADKStrategyScript("strategy.save_draft", script)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	definition := strategyDesignDefinition{
		Name:         defaultStringLocal(stringValue(input, "name"), "ADK 策略草稿"),
		Description:  "由 ADK agent 生成的策略草稿。",
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Runtime:      strategyRuntimeDSLPlan,
		Version:      defaultStrategyVersion,
		Symbol:       strings.TrimSpace(validation.Program.Metadata.Symbol),
		Interval:     strings.TrimSpace(validation.Program.Metadata.Interval),
		Script:       validation.NormalizedScript,
	}
	return server.designStore.saveDefinition(definition)
}

func strategyUpdateInstanceModeToolPayload(server *Server, input map[string]any) (map[string]any, error) {
	if server == nil || server.strategyStore == nil {
		return nil, fmt.Errorf("策略实例存储不可用")
	}
	instanceID := strings.TrimSpace(stringValue(input, "instanceId"))
	if instanceID == "" {
		return nil, fmt.Errorf("instanceId 是必填项")
	}
	executionMode := strings.ToLower(strings.TrimSpace(stringValue(input, "executionMode")))
	switch executionMode {
	case strategyExecutionModeLive, strategyExecutionModeNotifyOnly:
	default:
		return nil, fmt.Errorf("executionMode 必须是以下值之一：%s、%s", strategyExecutionModeLive, strategyExecutionModeNotifyOnly)
	}
	current, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		return nil, fmt.Errorf("策略实例 %q 不存在", instanceID)
	}
	binding := current.Binding
	binding.ExecutionMode = executionMode
	updated, err := server.strategyStore.updateStrategyBinding(instanceID, binding)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"instance":      server.enrichStrategyItem(updated),
		"updatedFields": []string{"executionMode"},
	}, nil
}

func strategySaveHintPayload() map[string]any {
	return map[string]any{
		"message":       strategydslspec.SaveDraftUsageHint(),
		"specTool":      strategydslspec.ToolName,
		"resourceFiles": []string{"references/dsl-v1-spec.md", "references/dsl-v1-examples.md"},
		"skeleton":      strategydslspec.Skeleton(),
	}
}

func strategyMetadataPayload(program *strategyir.Program) map[string]any {
	metadata := map[string]any{
		"name":     "",
		"version":  "",
		"symbol":   "",
		"interval": "",
	}
	if program == nil {
		return metadata
	}
	metadata["name"] = strings.TrimSpace(program.Metadata.Name)
	metadata["version"] = strings.TrimSpace(program.Metadata.Version)
	metadata["symbol"] = strings.TrimSpace(program.Metadata.Symbol)
	metadata["interval"] = strings.TrimSpace(program.Metadata.Interval)
	return metadata
}

func strategyVisualModelFromInput(value any) (*strategyVisualModel, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("visualModel must be a valid object: %w", err)
	}
	var model strategyVisualModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("visualModel must be a valid object: %w", err)
	}
	return normalizeStrategyVisualModel(&model), nil
}

func boolInputValue(input map[string]any, key string) bool {
	return boolInputValueDefault(input, key, false)
}

func boolInputValueDefault(input map[string]any, key string, fallback bool) bool {
	value, ok := input[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return fallback
	}
}
