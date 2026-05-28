package jftradeapi

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
	Symbol                string               `json:"symbol"`
	Interval              string               `json:"interval"`
	Script                string               `json:"script"`
	VisualModel           *strategyVisualModel `json:"visualModel,omitempty"`
	CreatedAt             string               `json:"createdAt"`
	UpdatedAt             string               `json:"updatedAt"`
	DerivedWarmupBars     int                  `json:"derivedWarmupBars"`
	DerivedWarmupInterval string               `json:"derivedWarmupInterval"`
}

func buildStrategyDefinitionResponse(definition strategyDesignDefinition, interval string) strategyDefinitionResponse {
	previewInterval := strings.TrimSpace(interval)
	if previewInterval == "" {
		previewInterval = strings.TrimSpace(definition.Interval)
	}
	if previewInterval == "" {
		previewInterval = "1m"
	}

	derivedWarmupBars := 0
	if warmupBars, err := indicatorruntime.WarmupBarsFromScript(definition.Script, types.Interval(previewInterval)); err == nil {
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
