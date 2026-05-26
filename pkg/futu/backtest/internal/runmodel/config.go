package runmodel

import "time"

// RunConfig describes a single backtest run.
type RunConfig struct {
	DBPath         string    `json:"dbPath"`
	Symbol         string    `json:"symbol"`
	Interval       string    `json:"interval"`
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime"`
	StrategyScript string    `json:"strategyScript"`
	InitialBalance float64   `json:"initialBalance"`
	WarmupCandles  int       `json:"warmupCandles"`
	QuoteCurrency  string    `json:"quoteCurrency"`
	RehabType      string    `json:"rehabType"`
}
