package indicatorruntime

import (
	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/futu"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

// IndicatorEngine exposes the pure-Go indicator runtime so strategy
// executors can reuse the exact same indicator and risk snapshots.
type IndicatorEngine struct {
	runtime *indicatorRuntime
}

func NewIndicatorEngineForPlan(plan strategyir.Requirements, interval types.Interval, symbol string) (*IndicatorEngine, error) {
	runtime, err := newIndicatorRuntimeFromPlan(plan, interval, symbol)
	if err != nil {
		return nil, err
	}
	return &IndicatorEngine{runtime: runtime}, nil
}

func (e *IndicatorEngine) Push(kline types.KLine, session futu.MarketSession) {
	if e == nil || e.runtime == nil {
		return
	}
	e.runtime.push(kline, session)
}

func (e *IndicatorEngine) Snapshot() map[string]any {
	if e == nil || e.runtime == nil {
		return map[string]any{}
	}
	snapshot := e.runtime.snapshot()
	if snapshot == nil {
		return map[string]any{}
	}
	return snapshot
}
