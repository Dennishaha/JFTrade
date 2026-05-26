package futu

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

const maxHistoryKLinePages = 8
const maxSyncKLinePages = 200 // unlimited: loop until OpenD nextReqKey is empty

type historicalKLineRequestPlan struct {
	extendedTime bool
	session      *commonpb.Session
	keepSessions []MarketSession
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

type klineSubscriptionRequest struct {
	canonical    string
	security     *qotcommonpb.Security
	subType      qotcommonpb.SubType
	extendedTime bool
	session      commonpb.Session
}

func (request klineSubscriptionRequest) cacheKey() string {
	return fmt.Sprintf("%s:%d:%t:%d", request.canonical, request.subType, request.extendedTime, request.session)
}

func (e *Exchange) QueryKLines(ctx context.Context, symbol string, interval types.Interval, options types.KLineQueryOptions) ([]types.KLine, error) {
	security, canonicalSymbol, err := futuSecurityFromSymbol(symbol)
	if err != nil {
		return nil, err
	}
	klType, err := futuKLTypeFromInterval(interval)
	if err != nil {
		return nil, err
	}
	beginAt, endAt, limit := futuKLineQueryWindow(interval, options)
	klines, err := e.queryHistoricalKLines(ctx, security, canonicalSymbol, interval, klType, beginAt, endAt, limit)
	if err != nil {
		return nil, err
	}
	if shouldQueryCurrentKLine(interval, endAt) {
		currentKLines, err := e.queryCurrentKLines(ctx, security, canonicalSymbol, interval, klType)
		if err == nil {
			klines = mergeKLinesByStartTime(klines, filterKLinesByWindow(currentKLines, beginAt, endAt))
		}
	}
	sort.Slice(klines, func(i, j int) bool {
		return klines[i].StartTime.Time().Before(klines[j].StartTime.Time())
	})
	if len(klines) > limit {
		klines = klines[len(klines)-limit:]
	}
	return klines, nil
}

// QueryAllKLines is a sync-optimized variant that does not set MaxAckKLNum,
// allowing OpenD to return larger pages. It follows nextReqKey until empty
// (capped at maxSyncKLinePages) and returns ALL klines without trimming.
// It does not query the current unfinished bucket (not needed for sync).
// rehabType controls price adjustment: None(0)=不复权, Forward(1)=前复权, Backward(2)=后复权.
func (e *Exchange) QueryAllKLines(ctx context.Context, symbol string, interval types.Interval, beginAt, endAt time.Time, rehabType qotcommonpb.RehabType) ([]types.KLine, error) {
	security, canonicalSymbol, err := futuSecurityFromSymbol(symbol)
	if err != nil {
		return nil, err
	}
	klType, err := futuKLTypeFromInterval(interval)
	if err != nil {
		return nil, err
	}

	plans := buildHistoricalKLineRequestPlans(canonicalSymbol, interval)
	klines, err := e.queryHistoricalKLinesAcrossPlans(ctx, security, canonicalSymbol, interval, klType, beginAt, endAt, rehabType, 0, maxSyncKLinePages, plans)
	if err != nil {
		return nil, err
	}
	sort.Slice(klines, func(i, j int) bool {
		return klines[i].StartTime.Time().Before(klines[j].StartTime.Time())
	})
	return klines, nil
}

func (e *Exchange) queryHistoricalKLines(ctx context.Context, security *qotcommonpb.Security, canonicalSymbol string, interval types.Interval, klType qotcommonpb.KLType, beginAt time.Time, endAt time.Time, limit int) ([]types.KLine, error) {
	plans := buildHistoricalKLineRequestPlans(canonicalSymbol, interval)
	return e.queryHistoricalKLinesAcrossPlans(ctx, security, canonicalSymbol, interval, klType, beginAt, endAt, qotcommonpb.RehabType_RehabType_None, limit, maxHistoryKLinePages, plans)
}

func (e *Exchange) queryHistoricalKLinesAcrossPlans(ctx context.Context, security *qotcommonpb.Security, canonicalSymbol string, interval types.Interval, klType qotcommonpb.KLType, beginAt time.Time, endAt time.Time, rehabType qotcommonpb.RehabType, limit int, maxPages int, plans []historicalKLineRequestPlan) ([]types.KLine, error) {
	klines := make([]types.KLine, 0, max(limit, 1))
	for _, plan := range plans {
		routeKLines, err := e.queryHistoricalKLinesForPlan(ctx, security, canonicalSymbol, interval, klType, beginAt, endAt, rehabType, limit, maxPages, plan)
		if err != nil {
			if shouldFallbackHistoricalKLineSplit(err, plan) {
				return e.queryHistoricalKLinesForPlan(ctx, security, canonicalSymbol, interval, klType, beginAt, endAt, rehabType, limit, maxPages, historicalKLineRequestPlanAll())
			}
			return nil, err
		}
		klines = mergeKLinesByStartTime(klines, routeKLines)
	}
	return klines, nil
}

func (e *Exchange) queryHistoricalKLinesForPlan(ctx context.Context, security *qotcommonpb.Security, canonicalSymbol string, interval types.Interval, klType qotcommonpb.KLType, beginAt time.Time, endAt time.Time, rehabType qotcommonpb.RehabType, limit int, maxPages int, plan historicalKLineRequestPlan) ([]types.KLine, error) {
	klines := make([]types.KLine, 0, max(limit, 1))
	nextReqKey := []byte(nil)
	for page := 0; page < maxPages; page++ {
		request := &historypb.Request{C2S: &historypb.C2S{
			RehabType: proto.Int32(int32(rehabType)),
			KlType:    proto.Int32(int32(klType)),
			Security:  security,
			BeginTime: proto.String(beginAt.Format("2006-01-02 15:04:05")),
			EndTime:   proto.String(endAt.Format("2006-01-02 15:04:05")),
		}}
		if limit > 0 {
			request.C2S.MaxAckKLNum = proto.Int32(int32(limit))
		}
		if len(nextReqKey) > 0 {
			request.C2S.NextReqKey = nextReqKey
		}
		if plan.extendedTime {
			request.C2S.ExtendedTime = proto.Bool(true)
			if plan.session != nil {
				request.C2S.Session = proto.Int32(int32(*plan.session))
			}
		}

		var response historypb.Response
		if err := e.callProto(ctx, opend.ProtoRequestHistoryKL, request, &response); err != nil {
			return nil, err
		}
		if response.GetRetType() != 0 {
			return nil, &historicalKLineRequestError{
				session: plan.session,
				retType: response.GetRetType(),
				errCode: response.GetErrCode(),
				retMsg:  response.GetRetMsg(),
			}
		}

		for _, candle := range response.GetS2C().GetKlList() {
			if candle.GetIsBlank() {
				continue
			}
			kline := futuKLineFromProto(candle, canonicalSymbol, interval)
			session := plan.resolveMarketSession(canonicalSymbol, kline)
			if !plan.shouldKeepMarketSession(session) {
				continue
			}
			e.RegisterKLineSession(kline, session)
			klines = append(klines, kline)
		}

		nextReqKey = response.GetS2C().GetNextReqKey()
		if len(nextReqKey) == 0 {
			break
		}
		if page == maxPages-1 {
			return nil, fmt.Errorf("opend RequestHistoryKL pagination exceeded %d pages", maxPages)
		}
	}
	return klines, nil
}

func (e *Exchange) queryCurrentKLines(ctx context.Context, security *qotcommonpb.Security, canonicalSymbol string, interval types.Interval, klType qotcommonpb.KLType) ([]types.KLine, error) {
	subType, err := futuSubTypeFromInterval(interval)
	if err != nil {
		return nil, err
	}

	subscription := klineSubscriptionRequest{
		canonical: canonicalSymbol,
		security:  security,
		subType:   subType,
	}
	if shouldRequestExtendedKLines(canonicalSymbol, interval) {
		subscription.extendedTime = true
		subscription.session = commonpb.Session_Session_ALL
	}

	request := &qotgetklpb.Request{C2S: &qotgetklpb.C2S{
		RehabType: proto.Int32(int32(qotcommonpb.RehabType_RehabType_None)),
		KlType:    proto.Int32(int32(klType)),
		Security:  security,
		ReqNum:    proto.Int32(2),
	}}

	var response qotgetklpb.Response
	if err := e.withClient(ctx, func(client *opend.Client) error {
		if err := e.ensureKLineSubscription(ctx, client, subscription); err != nil {
			return err
		}
		return client.Call(ctx, opend.ProtoGetKL, request, &response)
	}); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend GetKL retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}

	klines := make([]types.KLine, 0, len(response.GetS2C().GetKlList()))
	for _, candle := range response.GetS2C().GetKlList() {
		if candle.GetIsBlank() {
			continue
		}
		kline := futuKLineFromProto(candle, canonicalSymbol, interval)
		e.RegisterKLineSession(kline, resolveKLineSessionByClock(canonicalSymbol, kline))
		klines = append(klines, kline)
	}
	return klines, nil
}

func (plan historicalKLineRequestPlan) resolveMarketSession(symbol string, kline types.KLine) MarketSession {
	if plan.session == nil {
		return resolveKLineSessionByClock(symbol, kline)
	}
	return resolveHistoricalMarketSession(*plan.session, symbol, kline)
}

func (plan historicalKLineRequestPlan) shouldKeepMarketSession(session MarketSession) bool {
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

func buildHistoricalKLineRequestPlans(symbol string, interval types.Interval) []historicalKLineRequestPlan {
	if shouldSplitHistoricalKLineRequestsBySession(symbol, interval) {
		rth := commonpb.Session_Session_RTH
		eth := commonpb.Session_Session_ETH
		all := commonpb.Session_Session_ALL
		return []historicalKLineRequestPlan{
			{extendedTime: true, session: &rth, keepSessions: []MarketSession{MarketSessionRegular}},
			{extendedTime: true, session: &eth, keepSessions: []MarketSession{MarketSessionPre, MarketSessionAfter}},
			{extendedTime: true, session: &all, keepSessions: []MarketSession{MarketSessionOvernight}},
		}
	}
	if shouldRequestExtendedKLines(symbol, interval) {
		all := commonpb.Session_Session_ALL
		return []historicalKLineRequestPlan{{extendedTime: true, session: &all}}
	}
	return []historicalKLineRequestPlan{{}}
}

func historicalKLineRequestPlanAll() historicalKLineRequestPlan {
	all := commonpb.Session_Session_ALL
	return historicalKLineRequestPlan{extendedTime: true, session: &all}
}

func shouldSplitHistoricalKLineRequestsBySession(symbol string, interval types.Interval) bool {
	return shouldRequestExtendedKLines(symbol, interval)
}

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

func resolveHistoricalMarketSession(requestSession commonpb.Session, symbol string, kline types.KLine) MarketSession {
	switch requestSession {
	case commonpb.Session_Session_RTH:
		return MarketSessionRegular
	case commonpb.Session_Session_OVERNIGHT:
		return MarketSessionOvernight
	case commonpb.Session_Session_ETH:
		return resolveETHHistoricalKLineSession(symbol, kline)
	default:
		return resolveKLineSessionByClock(symbol, kline)
	}
}

func resolveETHHistoricalKLineSession(symbol string, kline types.KLine) MarketSession {
	clockSession := resolveKLineSessionByClock(symbol, kline)
	if clockSession == MarketSessionPre || clockSession == MarketSessionAfter {
		return clockSession
	}
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") {
		return clockSession
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if observedAt.IsZero() {
		return MarketSessionUnknown
	}
	local := observedAt.In(usEasternLocation)
	minutes := local.Hour()*60 + local.Minute()
	if minutes < 12*60 {
		return MarketSessionPre
	}
	return MarketSessionAfter
}

func (e *Exchange) ensureKLineSubscription(ctx context.Context, client *opend.Client, request klineSubscriptionRequest) error {
	cacheKey := request.cacheKey()

	e.mu.Lock()
	exists := e.subscriptions.hasKLine(cacheKey)
	e.mu.Unlock()
	if exists {
		return nil
	}

	if err := subscribeKLine(ctx, client, request); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.subscriptions.markKLine(cacheKey)
	return nil
}

func subscribeKLine(ctx context.Context, client *opend.Client, request klineSubscriptionRequest) error {
	subscription := &qotsubpb.Request{C2S: &qotsubpb.C2S{
		SecurityList:     []*qotcommonpb.Security{request.security},
		SubTypeList:      []int32{int32(request.subType)},
		IsSubOrUnSub:     proto.Bool(true),
		IsRegOrUnRegPush: proto.Bool(false),
	}}
	if request.extendedTime {
		subscription.C2S.ExtendedTime = proto.Bool(true)
		subscription.C2S.Session = proto.Int32(int32(request.session))
	}

	var response qotsubpb.Response
	if err := client.Call(ctx, opend.ProtoQotSub, subscription, &response); err != nil {
		return err
	}
	if response.GetRetType() != 0 {
		return fmt.Errorf("opend Qot_Sub retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	return nil
}

func futuKLTypeFromInterval(interval types.Interval) (qotcommonpb.KLType, error) {
	switch interval {
	case types.Interval1m:
		return qotcommonpb.KLType_KLType_1Min, nil
	case types.Interval3m:
		return qotcommonpb.KLType_KLType_3Min, nil
	case types.Interval5m:
		return qotcommonpb.KLType_KLType_5Min, nil
	case types.Interval15m:
		return qotcommonpb.KLType_KLType_15Min, nil
	case types.Interval30m:
		return qotcommonpb.KLType_KLType_30Min, nil
	case types.Interval1h:
		return qotcommonpb.KLType_KLType_60Min, nil
	case types.Interval1d:
		return qotcommonpb.KLType_KLType_Day, nil
	case types.Interval1w:
		return qotcommonpb.KLType_KLType_Week, nil
	case types.Interval1mo:
		return qotcommonpb.KLType_KLType_Month, nil
	default:
		return qotcommonpb.KLType_KLType_Unknown, fmt.Errorf("futu exchange: unsupported interval %q", interval)
	}
}

func futuSubTypeFromInterval(interval types.Interval) (qotcommonpb.SubType, error) {
	switch interval {
	case types.Interval1m:
		return qotcommonpb.SubType_SubType_KL_1Min, nil
	case types.Interval3m:
		return qotcommonpb.SubType_SubType_KL_3Min, nil
	case types.Interval5m:
		return qotcommonpb.SubType_SubType_KL_5Min, nil
	case types.Interval15m:
		return qotcommonpb.SubType_SubType_KL_15Min, nil
	case types.Interval30m:
		return qotcommonpb.SubType_SubType_KL_30Min, nil
	case types.Interval1h:
		return qotcommonpb.SubType_SubType_KL_60Min, nil
	case types.Interval1d:
		return qotcommonpb.SubType_SubType_KL_Day, nil
	case types.Interval1w:
		return qotcommonpb.SubType_SubType_KL_Week, nil
	case types.Interval1mo:
		return qotcommonpb.SubType_SubType_KL_Month, nil
	default:
		return qotcommonpb.SubType_SubType_None, fmt.Errorf("futu exchange: unsupported interval %q", interval)
	}
}

func futuKLineQueryWindow(interval types.Interval, options types.KLineQueryOptions) (time.Time, time.Time, int) {
	limit := options.Limit
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	endAt := time.Now()
	if options.EndTime != nil {
		endAt = *options.EndTime
	}
	lookback := interval.Duration() * time.Duration(limit) * 4
	minimumLookback := 36 * time.Hour
	if interval.Duration() >= 24*time.Hour {
		minimumLookback = 45 * 24 * time.Hour
	}
	if lookback < minimumLookback {
		lookback = minimumLookback
	}
	beginAt := endAt.Add(-lookback)
	if options.StartTime != nil {
		beginAt = *options.StartTime
	}
	if !beginAt.Before(endAt) {
		beginAt = endAt.Add(-lookback)
	}
	return beginAt, endAt, limit
}

func shouldQueryCurrentKLine(interval types.Interval, endAt time.Time) bool {
	duration := interval.Duration()
	if duration <= 0 {
		return false
	}
	return !endAt.Before(time.Now().UTC().Add(-duration))
}

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
	labelAt := futuQuoteTime(candle.GetTimestamp(), candle.GetTime()).UTC()
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
		Open:        fixedpoint.NewFromFloat(candle.GetOpenPrice()),
		Close:       fixedpoint.NewFromFloat(candle.GetClosePrice()),
		High:        fixedpoint.NewFromFloat(candle.GetHighPrice()),
		Low:         fixedpoint.NewFromFloat(candle.GetLowPrice()),
		Volume:      fixedpoint.NewFromFloat(float64(candle.GetVolume())),
		QuoteVolume: fixedpoint.NewFromFloat(candle.GetTurnover()),
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
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") && interval.Duration() <= time.Hour
}
