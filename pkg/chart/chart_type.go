// Package chart contains shared chart-domain contracts used by backend
// services. It intentionally has no rendering or market-data dependencies.
package chart

import "strings"

// ChartType selects the candle representation used for charting and Pine
// signal evaluation. It never changes the standard OHLC execution source.
type ChartType string

const (
	ChartTypeStandard   ChartType = "standard"
	ChartTypeHeikinAshi ChartType = "heikinashi"
)

// NormalizeChartType returns the supported chart type for value. Empty,
// legacy, and unknown values retain the historical standard-candle behavior.
func NormalizeChartType(value string) ChartType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ChartTypeHeikinAshi):
		return ChartTypeHeikinAshi
	default:
		return ChartTypeStandard
	}
}
