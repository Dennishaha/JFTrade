package pineruntime

import (
	"strings"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func (s *evaluationScope) variable(name string) (any, bool) {
	for current := s; current != nil; current = current.parent {
		if current.variables == nil {
			if value, ok := current.reservedVariable(name); ok {
				return value, true
			}
			continue
		}
		value, ok := current.variables[name]
		if ok {
			return value, true
		}
		if value, ok := current.reservedVariable(name); ok {
			return value, true
		}
	}
	return nil, false
}

func (s *evaluationScope) reservedVariable(name string) (any, bool) {
	if s == nil {
		return nil, false
	}
	switch name {
	case "indicators":
		if s.indicators == nil {
			return nil, false
		}
		return s.indicators, true
	case "kline":
		if s.currentKline == nil {
			return nil, false
		}
		return &s.klinePayload, true
	case "close":
		if !s.hasBarData {
			return nil, false
		}
		return &s.closeSeries, true
	case "open":
		if !s.hasBarData {
			return nil, false
		}
		return &s.openSeries, true
	case "high":
		if !s.hasBarData {
			return nil, false
		}
		return &s.highSeries, true
	case "low":
		if !s.hasBarData {
			return nil, false
		}
		return &s.lowSeries, true
	case "volume":
		if !s.hasBarData {
			return nil, false
		}
		return &s.volumeSeries, true
	case "hl2":
		if !s.hasBarData {
			return nil, false
		}
		return &s.hl2Series, true
	case "hlc3":
		if !s.hasBarData {
			return nil, false
		}
		return &s.hlc3Series, true
	case "ohlc4":
		if !s.hasBarData {
			return nil, false
		}
		return &s.ohlc4Series, true
	case "position_size":
		position := s.currentPosition()
		if position == nil {
			return 0.0, true
		}
		return position.Quantity, true
	case "position_avg_price":
		position := s.currentPosition()
		if position == nil || position.Quantity == 0 || position.AveragePrice <= 0 {
			return nil, true
		}
		return position.AveragePrice, true
	case "equity":
		if s.runtime == nil {
			return 0.0, true
		}
		return s.runtime.getTotalAccountValue(), true
	case "bar_index":
		return float64(s.barIndex), true
	case "time":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).UnixMilli()), true
	case "time_close":
		if s.currentKline == nil {
			return nil, false
		}
		closeTime, ok := pineBarCloseTime(s.currentKline, s.runtimeInterval())
		if !ok {
			return nil, true
		}
		return float64(closeTime.UnixMilli()), true
	case "hour":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Hour()), true
	case "minute":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Minute()), true
	case "dayofweek":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineDayOfWeek(pineBarTime(s.currentKline))), true
	case "dayofmonth":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Day()), true
	case "month":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Month()), true
	case "year":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Year()), true
	case "syminfo_tickerid":
		if s.runtime == nil {
			return "", true
		}
		return s.runtime.symbol, true
	case "syminfo_prefix":
		if s.runtime == nil {
			return "", true
		}
		return pineSymbolPrefix(s.runtime.symbol), true
	case "timeframe_period":
		if s.runtime == nil {
			return "", true
		}
		return string(s.runtime.interval), true
	case "timeframe_multiplier":
		return float64(pineTimeframeMultiplier(s.runtimeInterval())), true
	case "timeframe_isintraday":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeIsIntraday(s.runtime.interval), true
	case "timeframe_isseconds":
		return pineTimeframeUnit(s.runtimeInterval()) == "second", true
	case "timeframe_isminutes":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeIsMinutes(s.runtime.interval), true
	case "timeframe_isdaily":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeUnit(s.runtime.interval) == "day", true
	case "timeframe_isweekly":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeUnit(s.runtime.interval) == "week", true
	case "timeframe_ismonthly":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeUnit(s.runtime.interval) == "month", true
	case "barstate_isfirst":
		return s.barIndex == 0, true
	case "barstate_isnew":
		return s.hasBarData, true
	case "barstate_isconfirmed":
		return s.hasBarData, true
	case "barstate_ishistory":
		return bbgo2.IsBackTesting, true
	case "barstate_isrealtime":
		return !bbgo2.IsBackTesting, true
	case "barstate_islast":
		return s.hasBarData, true
	case "session_ismarket":
		return s.currentSession == market.SessionRegular, true
	case "session_ispremarket":
		return s.currentSession == market.SessionPre, true
	case "session_ispostmarket":
		return s.currentSession == market.SessionAfter, true
	default:
		return nil, false
	}
}

func (s *evaluationScope) currentPosition() *positionSnapshot {
	if s == nil || s.runtime == nil {
		return nil
	}
	symbol := strings.TrimSpace(s.currentKlineSymbol)
	if symbol == "" {
		symbol = s.runtime.symbol
	}
	if symbol == "" {
		return nil
	}
	return s.runtime.getPosition(symbol, s.currentKlineTime)
}

func (s *evaluationScope) setVariable(name string, value any) {
	if s == nil {
		return
	}
	if s.variables == nil {
		s.variables = map[string]any{}
	}
	s.variables[name] = value
}

func (s *evaluationScope) assignVariable(name string, value any) {
	if s == nil {
		return
	}
	for current := s; current != nil; current = current.parent {
		if current.variables == nil {
			continue
		}
		if _, ok := current.variables[name]; ok {
			current.variables[name] = value
			return
		}
	}
	s.setVariable(name, value)
}

func (s *evaluationScope) binding(name string) (indicatorBinding, bool) {
	for current := s; current != nil; current = current.parent {
		if current.bindings == nil {
			continue
		}
		value, ok := current.bindings[name]
		if ok {
			return value, true
		}
	}
	return indicatorBinding{}, false
}

func (s *evaluationScope) setBinding(name string, binding indicatorBinding) {
	if s == nil {
		return
	}
	if s.bindings == nil {
		s.bindings = map[string]indicatorBinding{}
	}
	s.bindings[name] = binding
}

func (r *strategyRuntime) newScope(kline *types.KLine, session market.Session) *evaluationScope {
	var indicators map[string]any
	if r.engine != nil {
		indicators = r.engine.SnapshotBorrowed()
	}
	scope := r.reusableScope
	if scope == nil {
		scope = &evaluationScope{
			runtime:   r,
			parent:    r.baseScope,
			variables: make(map[string]any, r.variableCapacity),
		}
		if r.bindingCapacity > 0 {
			scope.bindings = make(map[string]indicatorBinding, r.bindingCapacity)
		}
		r.reusableScope = scope
	}
	scope.parent = r.baseScope
	clear(scope.variables)
	if scope.bindings != nil {
		clear(scope.bindings)
	}
	scope.indicators = indicators
	scope.currentKline = kline
	scope.currentSession = session
	scope.currentKlineTime = time.Time{}
	scope.currentKlineSymbol = ""
	scope.klinePayload = klinePayloadView{}
	scope.closeSeries = seriesNumber{}
	scope.openSeries = seriesNumber{}
	scope.highSeries = seriesNumber{}
	scope.lowSeries = seriesNumber{}
	scope.volumeSeries = seriesNumber{}
	scope.hl2Series = seriesNumber{}
	scope.hlc3Series = seriesNumber{}
	scope.ohlc4Series = seriesNumber{}
	scope.hasBarData = false
	scope.barIndex = r.barIndex
	if kline != nil {
		scope.currentKlineTime = kline.EndTime.Time()
		scope.currentKlineSymbol = kline.Symbol
		scope.klinePayload = klinePayloadView{kline: kline, session: session}
		scope.closeSeries = seriesNumber{Current: kline.Close.Float64(), Previous: r.previousClose, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.openSeries = seriesNumber{Current: kline.Open.Float64(), Previous: r.previousOpen, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.highSeries = seriesNumber{Current: kline.High.Float64(), Previous: r.previousHigh, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.lowSeries = seriesNumber{Current: kline.Low.Float64(), Previous: r.previousLow, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.volumeSeries = seriesNumber{Current: kline.Volume.Float64(), Previous: r.previousVolume, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		currentHL2 := (scope.highSeries.Current + scope.lowSeries.Current) / 2
		currentHLC3 := (scope.highSeries.Current + scope.lowSeries.Current + scope.closeSeries.Current) / 3
		currentOHLC4 := (scope.openSeries.Current + scope.highSeries.Current + scope.lowSeries.Current + scope.closeSeries.Current) / 4
		previousHL2 := (r.previousHigh + r.previousLow) / 2
		previousHLC3 := (r.previousHigh + r.previousLow + r.previousClose) / 3
		previousOHLC4 := (r.previousOpen + r.previousHigh + r.previousLow + r.previousClose) / 4
		scope.hl2Series = seriesNumber{Current: currentHL2, Previous: previousHL2, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.hlc3Series = seriesNumber{Current: currentHLC3, Previous: previousHLC3, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.ohlc4Series = seriesNumber{Current: currentOHLC4, Previous: previousOHLC4, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.hasBarData = true
	}
	return scope
}
