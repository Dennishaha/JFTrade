package servercore

import (
	"github.com/jftrade/jftrade-main/pkg/backtest"
)

type backtestStartRequest struct {
	DefinitionID      string  `json:"definitionId"`
	DefinitionVersion string  `json:"definitionVersion,omitempty"`
	Market            string  `json:"market"`
	Code              string  `json:"code"`
	Symbol            string  `json:"symbol"`
	Interval          string  `json:"interval"`
	StartDate         string  `json:"startDate,omitempty"`
	EndDate           string  `json:"endDate,omitempty"`
	StartTime         string  `json:"startTime,omitempty"`
	EndTime           string  `json:"endTime,omitempty"`
	MarketTimezone    string  `json:"marketTimezone,omitempty"`
	InitialBalance    float64 `json:"initialBalance"`
	RehabType         string  `json:"rehabType"` // "forward" | "backward" | "none"
	UseExtendedHours  *bool   `json:"useExtendedHours,omitempty"`
}

type backtestRunState struct {
	ID        string               `json:"id"`
	Status    string               `json:"status"` // "queued", "running", "completed", "failed"
	Request   backtestStartRequest `json:"request"`
	Result    *backtest.RunResult  `json:"result,omitempty"`
	CreatedAt string               `json:"createdAt"`
	UpdatedAt string               `json:"updatedAt"`
}
