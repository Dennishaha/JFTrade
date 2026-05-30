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

type RuntimeOptions struct {
	IncludeExtendedHours bool
}

func NewIndicatorEngineForPlan(plan strategyir.Requirements, interval types.Interval, symbol string) (*IndicatorEngine, error) {
	return NewIndicatorEngineForPlanWithOptions(plan, interval, symbol, RuntimeOptions{})
}

func NewIndicatorEngineForPlanWithOptions(plan strategyir.Requirements, interval types.Interval, symbol string, options RuntimeOptions) (*IndicatorEngine, error) {
	runtime, err := newIndicatorRuntimeFromPlanWithOptions(plan, interval, symbol, options)
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
	clone := make(map[string]any, len(snapshot))
	for key, value := range snapshot {
		if scalar, ok := value.(interface{ ScalarValue() (float64, bool) }); ok {
			if current, currentOK := scalar.ScalarValue(); currentOK {
				clone[key] = current
			} else {
				clone[key] = nil
			}
			continue
		}
		clone[key] = value
	}
	return clone
}

// SnapshotBorrowed returns a snapshot map borrowed from the engine runtime.
// The returned map is reused by the next SnapshotBorrowed call and must only
// be consumed synchronously within the current execution tick.
func (e *IndicatorEngine) SnapshotBorrowed() map[string]any {
	if e == nil || e.runtime == nil {
		return map[string]any{}
	}
	snapshot := e.runtime.snapshot()
	if snapshot == nil {
		return map[string]any{}
	}
	return snapshot
}
