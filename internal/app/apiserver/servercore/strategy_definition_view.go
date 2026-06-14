package servercore

import (
	"strings"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
)

type strategyDefinitionResponse struct {
	ID                    string               `json:"id"`
	Name                  string               `json:"name"`
	Version               string               `json:"version"`
	Description           string               `json:"description"`
	Runtime               string               `json:"runtime"`
	SourceFormat          string               `json:"sourceFormat"`
	Symbol                string               `json:"symbol,omitempty"`
	Interval              string               `json:"interval,omitempty"`
	Script                string               `json:"script"`
	VisualModel           *strategyVisualModel `json:"visualModel,omitempty"`
	CreatedAt             string               `json:"createdAt"`
	UpdatedAt             string               `json:"updatedAt"`
	DerivedWarmupBars     int                  `json:"derivedWarmupBars"`
	DerivedWarmupInterval string               `json:"derivedWarmupInterval"`
}

func buildStrategyDefinitionResponse(definition strategyDesignDefinition, interval string, symbol string, useExtendedHours bool) strategyDefinitionResponse {
	previewInterval := strings.TrimSpace(interval)
	if previewInterval == "" {
		previewInterval = "5m"
	}
	previewSymbol := strings.TrimSpace(symbol)
	if previewSymbol == "" {
		previewSymbol = strings.TrimSpace(definition.Symbol)
	}

	derivedWarmupBars := 0
	if warmupBars, err := indicatorruntime.WarmupBarsFromScriptForSymbolWithOptions(
		definition.Script,
		types.Interval(previewInterval),
		previewSymbol,
		indicatorruntime.RuntimeOptions{IncludeExtendedHours: useExtendedHours},
	); err == nil {
		derivedWarmupBars = warmupBars
	}

	return strategyDefinitionResponse{
		ID:                    definition.ID,
		Name:                  definition.Name,
		Version:               definition.Version,
		Description:           definition.Description,
		Runtime:               definition.Runtime,
		SourceFormat:          definition.SourceFormat,
		Symbol:                definition.Symbol,
		Interval:              definition.Interval,
		Script:                definition.Script,
		VisualModel:           definition.VisualModel,
		CreatedAt:             definition.CreatedAt,
		UpdatedAt:             definition.UpdatedAt,
		DerivedWarmupBars:     derivedWarmupBars,
		DerivedWarmupInterval: previewInterval,
	}
}
