package servercore

import (
	"github.com/jftrade/jftrade-main/internal/app/apiserver/status"
	"github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
)

func liveStatsSummary(s *serverApplication) *system.LiveStats {
	if s == nil {
		return status.LiveStats(0, 0, false, nil)
	}
	liveWebSocket := s.runtimes.LiveWebSocket()
	if liveWebSocket == nil {
		return status.LiveStats(0, 0, false, nil)
	}
	stats := liveWebSocket.Stats()
	activeInstruments := liveWebSocket.ActiveInstrumentIDs()
	return status.LiveStats(stats.Connected, stats.Limit, stats.AtLimit, activeInstruments)
}

func marketdataRuntimeSummary(s *serverApplication) *system.MarketDataRuntime {
	return status.MarketDataRuntimeSummary(s.marketdataSvc)
}

func strategyRuntimeSummary(s *serverApplication) *strategy.RuntimeSummary {
	if s == nil {
		return status.StrategyRuntimeSummary(nil)
	}
	return status.StrategyRuntimeSummary(s.runtimes.StrategyRuntime())
}
