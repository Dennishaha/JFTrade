package pineruntime

import (
	"strings"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) resolveKLineSession(kline types.KLine) market.Session {
	if r.session != nil && r.session.Exchange != nil {
		if resolver, ok := r.session.Exchange.(klineSessionResolver); ok {
			if session, ok := resolver.ResolveKLineSession(kline); ok {
				return session
			}
		}
	}
	strategySymbol := ""
	if r.strategy != nil {
		strategySymbol = r.strategy.Symbol
	}
	resolvedSymbol := strings.ToUpper(strings.TrimSpace(strategySymbol))
	if resolvedSymbol == "" {
		resolvedSymbol = strings.ToUpper(strings.TrimSpace(kline.Symbol))
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if resolvedSymbol == "" || observedAt.IsZero() {
		return market.SessionUnknown
	}
	return market.ClassifySession(resolvedSymbol, observedAt)
}

func (r *strategyRuntime) isPlaceBlockedDuringWarmup(currentKlineTime time.Time) bool {
	return !r.strategy.WarmupUntil.IsZero() && !currentKlineTime.IsZero() && currentKlineTime.Before(r.strategy.WarmupUntil)
}

func (r *strategyRuntime) log(message string) {
	bbgo2.Notify("pine strategy %s: %s", r.displayName, strings.TrimSpace(message))
}

func (r *strategyRuntime) internalLog(message string) {
	if bbgo2.IsBackTesting {
		return
	}
	r.log(message)
}

func (r *strategyRuntime) notify(message string) {
	bbgo2.Notify("pine strategy %s: %s", r.displayName, strings.TrimSpace(message))
}

func (r *strategyRuntime) cachedPosition(symbol string, barTime time.Time) (*positionSnapshot, bool) {
	if r == nil || !r.positionCache.valid {
		return nil, false
	}
	if r.positionCache.symbol != symbol || !r.positionCache.barTime.Equal(barTime) {
		return nil, false
	}
	return r.positionCache.value, true
}

func (r *strategyRuntime) storeCachedPosition(symbol string, barTime time.Time, snapshot *positionSnapshot) {
	if r == nil {
		return
	}
	r.positionCache = cachedPositionSnapshot{
		barTime: barTime,
		symbol:  symbol,
		value:   snapshot,
		valid:   true,
	}
}

func (r *strategyRuntime) clearPositionCache() {
	if r == nil {
		return
	}
	r.positionCache = cachedPositionSnapshot{}
}

func normalizeRuntimePyramiding(program *strategyir.Program) int {
	if program == nil || program.Metadata.Pyramiding <= 0 {
		return 1
	}
	return program.Metadata.Pyramiding
}
