package futu

import (
	"context"
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

func (e *Exchange) queryHistoricalKLines(ctx context.Context, security *qotcommonpb.Security, canonicalSymbol string, interval types.Interval, klType qotcommonpb.KLType, beginAt time.Time, endAt time.Time, limit int) ([]types.KLine, error) {
	klines := make([]types.KLine, 0, limit)
	nextReqKey := []byte(nil)
	for page := 0; page < maxHistoryKLinePages; page++ {
		request := &historypb.Request{C2S: &historypb.C2S{
			RehabType:   proto.Int32(int32(qotcommonpb.RehabType_RehabType_None)),
			KlType:      proto.Int32(int32(klType)),
			Security:    security,
			BeginTime:   proto.String(beginAt.Format("2006-01-02 15:04:05")),
			EndTime:     proto.String(endAt.Format("2006-01-02 15:04:05")),
			MaxAckKLNum: proto.Int32(int32(limit)),
		}}
		if len(nextReqKey) > 0 {
			request.C2S.NextReqKey = nextReqKey
		}
		if shouldRequestExtendedKLines(canonicalSymbol, interval) {
			request.C2S.ExtendedTime = proto.Bool(true)
			request.C2S.Session = proto.Int32(int32(commonpb.Session_Session_ALL))
		}

		var response historypb.Response
		if err := e.callProto(ctx, opend.ProtoRequestHistoryKL, request, &response); err != nil {
			return nil, err
		}
		if response.GetRetType() != 0 {
			return nil, fmt.Errorf("opend RequestHistoryKL retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
		}

		for _, candle := range response.GetS2C().GetKlList() {
			if candle.GetIsBlank() {
				continue
			}
			klines = append(klines, futuKLineFromProto(candle, canonicalSymbol, interval))
		}

		nextReqKey = response.GetS2C().GetNextReqKey()
		if len(nextReqKey) == 0 {
			break
		}
		if page == maxHistoryKLinePages-1 {
			return nil, fmt.Errorf("opend RequestHistoryKL pagination exceeded %d pages", maxHistoryKLinePages)
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
		klines = append(klines, futuKLineFromProto(candle, canonicalSymbol, interval))
	}
	return klines, nil
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
