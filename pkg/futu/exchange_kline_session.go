package futu

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
)

// --- US session-aware historical K-line request planning ---
// When RequestHistoryKL does not natively support session-routed queries
// (RTH/ETH/OVERNIGHT), the exchange fans out requests across sessions,
// merges the returned candles, and falls back to a single Session_ALL
// request when OpenD explicitly rejects a session-routed call.

var usEasternLocation = func() *time.Location {
	profile, ok := market.ProfileForSymbol("US.AAPL")
	if !ok {
		return time.UTC
	}
	return profile.Location
}()

type historicalKLineRequestPlan struct {
	extendedTime bool
	session      *commonpb.Session
	keepSessions []market.Session
}

type historicalKLineRequestError struct {
	session *commonpb.Session
	retType int32
	errCode int32
	retMsg  string
}

func (err *historicalKLineRequestError) Error() string {
	return fmt.Sprintf("opend RequestHistoryKL retType=%d errCode=%d retMsg=%s", err.retType, err.errCode, err.retMsg)
}

// --- Session planning ---

func buildHistoricalKLineRequestPlans(symbol string, interval types.Interval) []historicalKLineRequestPlan {
	if shouldSplitHistoricalKLineRequestsBySession(symbol, interval) {
		return []historicalKLineRequestPlan{
			{extendedTime: true, session: new(commonpb.Session_Session_RTH), keepSessions: []market.Session{market.SessionRegular}},
			{extendedTime: true, session: new(commonpb.Session_Session_ETH), keepSessions: []market.Session{market.SessionPre, market.SessionAfter}},
			{extendedTime: true, session: new(commonpb.Session_Session_ALL), keepSessions: []market.Session{market.SessionOvernight}},
		}
	}
	if shouldRequestExtendedKLines(symbol, interval) {
		return []historicalKLineRequestPlan{{extendedTime: true, session: new(commonpb.Session_Session_ALL)}}
	}
	return []historicalKLineRequestPlan{{}}
}

func historicalKLineRequestPlanAll() historicalKLineRequestPlan {
	return historicalKLineRequestPlan{extendedTime: true, session: new(commonpb.Session_Session_ALL)}
}

func shouldSplitHistoricalKLineRequestsBySession(symbol string, interval types.Interval) bool {
	return shouldRequestExtendedKLines(symbol, interval)
}

// --- Fallback strategy ---

func shouldFallbackHistoricalKLineSplit(err error, plan historicalKLineRequestPlan) bool {
	if plan.session == nil {
		return false
	}
	var routeErr *historicalKLineRequestError
	if !errors.As(err, &routeErr) || routeErr.session == nil {
		return false
	}
	if *routeErr.session != *plan.session {
		return false
	}
	message := strings.ToUpper(strings.TrimSpace(routeErr.retMsg))
	if message == "" {
		return false
	}
	hasSessionMarker := strings.Contains(message, "OVERNIGHT") || strings.Contains(message, "SESSION") || strings.Contains(message, "时段") || (strings.Contains(message, "RTH") && strings.Contains(message, "ETH") && strings.Contains(message, "ALL"))
	if hasSessionMarker {
		if strings.Contains(message, "NOT SUPPORT") || strings.Contains(message, "UNSUPPORTED") || strings.Contains(message, "INVALID") || strings.Contains(message, "ONLY SUPPORT") || strings.Contains(message, "SUPPORT ONLY") || strings.Contains(message, "不支持") || strings.Contains(message, "无效") || strings.Contains(message, "仅支持") {
			return true
		}
	}
	return false
}

// --- Session resolution ---

func (plan historicalKLineRequestPlan) resolveMarketSession(symbol string, kline types.KLine) market.Session {
	if plan.session == nil {
		return resolveKLineSessionByClock(symbol, kline)
	}
	return resolveHistoricalMarketSession(*plan.session, symbol, kline)
}

func (plan historicalKLineRequestPlan) shouldKeepMarketSession(session market.Session) bool {
	if len(plan.keepSessions) == 0 {
		return true
	}
	for _, candidate := range plan.keepSessions {
		if candidate == session {
			return true
		}
	}
	return false
}

func resolveHistoricalMarketSession(requestSession commonpb.Session, symbol string, kline types.KLine) market.Session {
	switch requestSession {
	case commonpb.Session_Session_RTH:
		return market.SessionRegular
	case commonpb.Session_Session_OVERNIGHT:
		return market.SessionOvernight
	case commonpb.Session_Session_ETH:
		return resolveETHHistoricalKLineSession(symbol, kline)
	default:
		return resolveKLineSessionByClock(symbol, kline)
	}
}

func resolveETHHistoricalKLineSession(symbol string, kline types.KLine) market.Session {
	clockSession := resolveKLineSessionByClock(symbol, kline)
	if clockSession == market.SessionPre || clockSession == market.SessionAfter {
		return clockSession
	}
	if !market.IsUSSymbol(symbol) {
		return clockSession
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if observedAt.IsZero() {
		return market.SessionUnknown
	}
	local := observedAt.In(usEasternLocation)
	minutes := local.Hour()*60 + local.Minute()
	if minutes < 12*60 {
		return market.SessionPre
	}
	return market.SessionAfter
}

// --- K-line merge & filter utilities ---

func filterKLinesByWindow(klines []types.KLine, beginAt time.Time, endAt time.Time) []types.KLine {
	filtered := make([]types.KLine, 0, len(klines))
	for _, kline := range klines {
		startAt := kline.StartTime.Time().UTC()
		finishAt := kline.EndTime.Time().UTC()
		if finishAt.Before(beginAt) || startAt.After(endAt) {
			continue
		}
		filtered = append(filtered, kline)
	}
	return filtered
}

func mergeKLinesByStartTime(slices ...[]types.KLine) []types.KLine {
	byStartTime := make(map[int64]types.KLine)
	for _, slice := range slices {
		for _, kline := range slice {
			byStartTime[kline.StartTime.Time().UTC().UnixNano()] = kline
		}
	}
	merged := make([]types.KLine, 0, len(byStartTime))
	for _, kline := range byStartTime {
		merged = append(merged, kline)
	}
	return merged
}

func futuKLineFromProto(candle *qotcommonpb.KLine, symbol string, interval types.Interval) types.KLine {
	labelAt := futuQuoteTime(candle.GetTimestamp(), candle.GetTime(), symbol)
	startAt := futuHistoryKLineStartTime(labelAt, interval)
	endAt := startAt.Add(interval.Duration()).Add(-time.Millisecond)
	if endAt.Before(startAt) {
		endAt = startAt
	}
	return types.KLine{
		Exchange:    Name,
		Symbol:      symbol,
		StartTime:   types.Time(startAt),
		EndTime:     types.Time(endAt),
		Interval:    interval,
		Open:        fixedpointFromFloat64(candle.GetOpenPrice()),
		Close:       fixedpointFromFloat64(candle.GetClosePrice()),
		High:        fixedpointFromFloat64(candle.GetHighPrice()),
		Low:         fixedpointFromFloat64(candle.GetLowPrice()),
		Volume:      fixedpoint.NewFromInt(candle.GetVolume()),
		QuoteVolume: fixedpointFromFloat64(candle.GetTurnover()),
		Closed:      !endAt.After(time.Now().UTC()),
	}
}

func futuHistoryKLineStartTime(labelAt time.Time, interval types.Interval) time.Time {
	duration := interval.Duration()
	if duration <= 0 || duration >= 24*time.Hour {
		return labelAt
	}

	return labelAt.Add(-duration)
}

func shouldRequestExtendedKLines(symbol string, interval types.Interval) bool {
	return market.IsUSSymbol(symbol) && interval.Duration() <= time.Hour
}
