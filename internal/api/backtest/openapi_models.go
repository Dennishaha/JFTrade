package backtest

import srv "github.com/jftrade/jftrade-main/internal/backtest"

// BacktestRunsData documents the list response envelope payload.
type BacktestRunsData struct {
	Runs []*srv.RunState `json:"runs"`
}
