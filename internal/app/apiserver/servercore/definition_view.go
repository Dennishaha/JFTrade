package servercore

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
