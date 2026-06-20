package pineruntime

import (
	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) runInit() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runHookLocked(strategyir.HookInit, nil, market.SessionUnknown)
}

func (r *strategyRuntime) handleKLineClosed(kline types.KLine) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if symbol := r.symbol; symbol != "" && kline.Symbol != symbol {
		return
	}
	if interval := r.interval; interval != "" && kline.Interval != interval {
		return
	}
	r.barIndex++

	resolvedSession := r.resolveKLineSession(kline)
	if r.engine != nil {
		r.engine.Push(kline, resolvedSession)
	}
	pendingScope := r.newScope(&kline, resolvedSession)
	if err := r.triggerPendingOrders(&kline, pendingScope); err != nil {
		errMsg := err.Error()
		bbgo.Notify("pine strategy %s pending order error: %s", r.displayName, errMsg)
		if r.strategy.OnError != nil {
			r.strategy.OnError(errMsg)
		}
	}
	if err := r.runHookLocked(strategyir.HookKLineClose, &kline, resolvedSession); err != nil {
		errMsg := err.Error()
		bbgo.Notify("pine strategy %s onKLineClosed error: %s", r.displayName, errMsg)
		if r.strategy.OnError != nil {
			r.strategy.OnError(errMsg)
		}
	}
	r.previousClose = kline.Close.Float64()
	r.previousOpen = kline.Open.Float64()
	r.previousHigh = kline.High.Float64()
	r.previousLow = kline.Low.Float64()
	r.previousVolume = kline.Volume.Float64()
	r.previousBarTime = pineBarTime(&kline)
	r.hasPreviousClose = true
	r.hasPreviousBarTime = true
}

func (r *strategyRuntime) runHookLocked(kind strategyir.HookKind, kline *types.KLine, session market.Session) error {
	hook := r.hooks[kind]
	if hook == nil {
		return nil
	}
	scope := r.newScope(kline, session)
	_, err := r.executeStatements(hook.Statements, scope)
	if err != nil {
		return err
	}
	r.recordHistorySnapshots(scope)
	return nil
}
