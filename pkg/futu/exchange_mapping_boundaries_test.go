package futu

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/broker"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestFutuKLineIntervalMappingsCoverSupportedAndUnsupportedValues(t *testing.T) {
	intervalCases := []struct {
		interval types.Interval
		klType   qotcommonpb.KLType
		subType  qotcommonpb.SubType
	}{
		{types.Interval1m, qotcommonpb.KLType_KLType_1Min, qotcommonpb.SubType_SubType_KL_1Min},
		{types.Interval3m, qotcommonpb.KLType_KLType_3Min, qotcommonpb.SubType_SubType_KL_3Min},
		{types.Interval5m, qotcommonpb.KLType_KLType_5Min, qotcommonpb.SubType_SubType_KL_5Min},
		{types.Interval15m, qotcommonpb.KLType_KLType_15Min, qotcommonpb.SubType_SubType_KL_15Min},
		{types.Interval30m, qotcommonpb.KLType_KLType_30Min, qotcommonpb.SubType_SubType_KL_30Min},
		{types.Interval1h, qotcommonpb.KLType_KLType_60Min, qotcommonpb.SubType_SubType_KL_60Min},
		{types.Interval1d, qotcommonpb.KLType_KLType_Day, qotcommonpb.SubType_SubType_KL_Day},
		{types.Interval1w, qotcommonpb.KLType_KLType_Week, qotcommonpb.SubType_SubType_KL_Week},
		{types.Interval1mo, qotcommonpb.KLType_KLType_Month, qotcommonpb.SubType_SubType_KL_Month},
	}
	for _, tt := range intervalCases {
		if got, err := futuKLTypeFromInterval(tt.interval); err != nil || got != tt.klType {
			t.Fatalf("futuKLTypeFromInterval(%s) = %v, err=%v, want %v", tt.interval, got, err, tt.klType)
		}
		if got, err := futuSubTypeFromInterval(tt.interval); err != nil || got != tt.subType {
			t.Fatalf("futuSubTypeFromInterval(%s) = %v, err=%v, want %v", tt.interval, got, err, tt.subType)
		}
	}
	if _, err := futuKLTypeFromInterval(types.Interval("2m")); err == nil {
		t.Fatal("unsupported KL interval error = nil")
	}
	if _, err := futuSubTypeFromInterval(types.Interval("2m")); err == nil {
		t.Fatal("unsupported subscription interval error = nil")
	}

	stringCases := map[string]qotcommonpb.KLType{
		"1m":      qotcommonpb.KLType_KLType_1Min,
		"10min":   qotcommonpb.KLType_KLType_10Min,
		"2h":      qotcommonpb.KLType_KLType_120Min,
		"3h":      qotcommonpb.KLType_KLType_180Min,
		"240m":    qotcommonpb.KLType_KLType_240Min,
		"daily":   qotcommonpb.KLType_KLType_Day,
		"weekly":  qotcommonpb.KLType_KLType_Week,
		"1M":      qotcommonpb.KLType_KLType_Month,
		"quarter": qotcommonpb.KLType_KLType_Quarter,
		"yearly":  qotcommonpb.KLType_KLType_Year,
	}
	for raw, want := range stringCases {
		if got, err := futuKLTypeFromIntervalString(raw); err != nil || got != want {
			t.Fatalf("futuKLTypeFromIntervalString(%q) = %v, err=%v, want %v", raw, got, err, want)
		}
	}
	if _, err := futuKLTypeFromIntervalString("13m"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported string interval err = %v", err)
	}

	if got := resolveHistoricalKLinePageSize(0); got != 0 {
		t.Fatalf("resolveHistoricalKLinePageSize(0) = %d", got)
	}
	if got := resolveHistoricalKLinePageSize(50); got != 200 {
		t.Fatalf("resolveHistoricalKLinePageSize(50) = %d", got)
	}
	if got := resolveHistoricalKLinePageSize(500); got != 500 {
		t.Fatalf("resolveHistoricalKLinePageSize(500) = %d", got)
	}
	if got := resolveHistoricalKLinePageSize(5000); got != 1000 {
		t.Fatalf("resolveHistoricalKLinePageSize(5000) = %d", got)
	}
}

func TestFutuKLineQueryWindowAndPreflightValidation(t *testing.T) {
	endAt := time.Date(2026, time.July, 1, 20, 0, 0, 0, time.UTC)
	startAt := endAt.Add(-2 * time.Hour)
	begin, end, limit := futuKLineQueryWindow(types.Interval1m, types.KLineQueryOptions{
		StartTime: &startAt,
		EndTime:   &endAt,
		Limit:     5,
	})
	if !begin.Equal(startAt) || !end.Equal(endAt) || limit != 5 {
		t.Fatalf("explicit intraday window begin=%s end=%s limit=%d", begin, end, limit)
	}

	begin, end, limit = futuKLineQueryWindow(types.Interval1d, types.KLineQueryOptions{
		StartTime: new(endAt.Add(time.Hour)),
		EndTime:   &endAt,
		Limit:     2000,
	})
	if !end.Equal(endAt) || limit != 1000 || !begin.Before(endAt) {
		t.Fatalf("daily window fallback begin=%s end=%s limit=%d", begin, end, limit)
	}
	if shouldQueryCurrentKLine(types.Interval1m, time.Now().UTC().Add(-2*time.Minute)) {
		t.Fatal("old end time should not request current kline")
	}
	if !shouldQueryCurrentKLine(types.Interval1m, time.Now().UTC()) {
		t.Fatal("fresh end time should request current kline")
	}

	exchange := NewExchange("127.0.0.1:1")
	if _, err := exchange.QueryKLines(t.Context(), "BAD", types.Interval1m, types.KLineQueryOptions{}); err == nil {
		t.Fatal("QueryKLines accepted invalid symbol")
	}
	if _, err := exchange.QueryKLines(t.Context(), "US.AAPL", types.Interval("2m"), types.KLineQueryOptions{}); err == nil {
		t.Fatal("QueryKLines accepted unsupported interval")
	}
	if _, err := exchange.QueryAllKLines(t.Context(), "BAD", types.Interval1d, startAt, endAt, qotcommonpb.RehabType_RehabType_None); err == nil {
		t.Fatal("QueryAllKLines accepted invalid symbol")
	}
	if _, err := exchange.QueryAllKLines(t.Context(), "US.AAPL", types.Interval("2m"), startAt, endAt, qotcommonpb.RehabType_RehabType_None); err == nil {
		t.Fatal("QueryAllKLines accepted unsupported interval")
	}
}

func TestFutuHistoricalKLineSessionFallbacksAndETHClassification(t *testing.T) {
	session := commonpb.Session_Session_ETH
	plan := historicalKLineRequestPlan{session: &session}
	if !shouldFallbackHistoricalKLineSplit(&historicalKLineRequestError{session: &session, retMsg: "RTH ETH ALL session not support"}, plan) {
		t.Fatal("session unsupported error should trigger ALL fallback")
	}
	if shouldFallbackHistoricalKLineSplit(errors.New("RTH ETH ALL session not support"), plan) {
		t.Fatal("plain error should not trigger session fallback")
	}
	otherSession := commonpb.Session_Session_RTH
	if shouldFallbackHistoricalKLineSplit(&historicalKLineRequestError{session: &otherSession, retMsg: "ETH session not support"}, plan) {
		t.Fatal("different rejected session should not trigger fallback")
	}
	if shouldFallbackHistoricalKLineSplit(&historicalKLineRequestError{session: &session, retMsg: "temporary timeout"}, plan) {
		t.Fatal("non-support error should not trigger session fallback")
	}

	preMarket := types.KLine{Symbol: "US.AAPL", StartTime: types.Time(time.Date(2026, time.June, 22, 12, 0, 0, 0, time.UTC))}
	if got := resolveETHHistoricalKLineSession("US.AAPL", preMarket); got != market.SessionPre {
		t.Fatalf("pre-market ETH session = %s", got)
	}
	afterMarket := types.KLine{Symbol: "US.AAPL", StartTime: types.Time(time.Date(2026, time.June, 22, 22, 0, 0, 0, time.UTC))}
	if got := resolveETHHistoricalKLineSession("US.AAPL", afterMarket); got != market.SessionAfter {
		t.Fatalf("after-market ETH session = %s", got)
	}
	zeroTime := types.KLine{Symbol: "US.AAPL"}
	if got := resolveETHHistoricalKLineSession("US.AAPL", zeroTime); got != market.SessionUnknown {
		t.Fatalf("zero-time ETH session = %s", got)
	}
}

func TestFutuTradeEnumMappingsCoverBrokerAndBBGOBoundaries(t *testing.T) {
	readOrderTypes := map[string]trdcommonpb.OrderType{
		"limit":                trdcommonpb.OrderType_OrderType_Normal,
		"LIMIT_MAKER":          trdcommonpb.OrderType_OrderType_Normal,
		"market":               trdcommonpb.OrderType_OrderType_Market,
		"stop":                 trdcommonpb.OrderType_OrderType_Stop,
		"stop_limit":           trdcommonpb.OrderType_OrderType_StopLimit,
		"TAKE_PROFIT_MARKET":   trdcommonpb.OrderType_OrderType_MarketifTouched,
		"marketIfTouched":      trdcommonpb.OrderType_OrderType_MarketifTouched,
		"TAKE_PROFIT":          trdcommonpb.OrderType_OrderType_LimitifTouched,
		"limitIfTouched":       trdcommonpb.OrderType_OrderType_LimitifTouched,
		"definitely_not_valid": 0,
	}
	for raw, want := range readOrderTypes {
		got, normalized, ok := trdOrderTypeFromBrokerOrderType(raw)
		if raw == "definitely_not_valid" {
			if ok || got != 0 || normalized != "" {
				t.Fatalf("invalid broker order type = %v/%q/%v", got, normalized, ok)
			}
			continue
		}
		if !ok || got != want || normalized == "" {
			t.Fatalf("trdOrderTypeFromBrokerOrderType(%q) = %v/%q/%v, want %v", raw, got, normalized, ok, want)
		}
	}

	for _, tt := range []struct {
		side types.SideType
		want trdcommonpb.TrdSide
	}{
		{types.SideTypeBuy, trdcommonpb.TrdSide_TrdSide_Buy},
		{types.SideTypeSell, trdcommonpb.TrdSide_TrdSide_Sell},
	} {
		if got, err := trdSideFromBBGOSide(tt.side); err != nil || got != tt.want {
			t.Fatalf("trdSideFromBBGOSide(%s) = %v, err=%v", tt.side, got, err)
		}
	}
	if _, err := trdSideFromBBGOSide(types.SideType("HOLD")); err == nil {
		t.Fatal("unsupported side error = nil")
	}

	for _, tt := range []struct {
		orderType types.OrderType
		want      trdcommonpb.OrderType
	}{
		{types.OrderTypeLimit, trdcommonpb.OrderType_OrderType_Normal},
		{types.OrderTypeLimitMaker, trdcommonpb.OrderType_OrderType_Normal},
		{types.OrderTypeMarket, trdcommonpb.OrderType_OrderType_Market},
		{types.OrderTypeStopMarket, trdcommonpb.OrderType_OrderType_Stop},
		{types.OrderTypeStopLimit, trdcommonpb.OrderType_OrderType_StopLimit},
		{types.OrderTypeTakeProfitMarket, trdcommonpb.OrderType_OrderType_MarketifTouched},
		{types.OrderTypeTakeProfit, trdcommonpb.OrderType_OrderType_LimitifTouched},
	} {
		if got, err := trdOrderTypeFromBBGOOrderType(tt.orderType); err != nil || got != tt.want {
			t.Fatalf("trdOrderTypeFromBBGOOrderType(%s) = %v, err=%v", tt.orderType, got, err)
		}
	}
	if _, err := trdOrderTypeFromBBGOOrderType(types.OrderType("ICEBERG")); err == nil {
		t.Fatal("unsupported BBGO order type error = nil")
	}

	for _, tt := range []struct {
		tif  types.TimeInForce
		want trdcommonpb.TimeInForce
		ok   bool
	}{
		{"", trdcommonpb.TimeInForce_TimeInForce_GTC, true},
		{types.TimeInForceGTC, trdcommonpb.TimeInForce_TimeInForce_GTC, true},
		{types.TimeInForce("DAY"), trdcommonpb.TimeInForce_TimeInForce_DAY, true},
		{types.TimeInForceIOC, trdcommonpb.TimeInForce_TimeInForce_IOC, true},
		{types.TimeInForceFOK, 0, false},
	} {
		got, ok := trdTimeInForceFromBBGO(tt.tif)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("trdTimeInForceFromBBGO(%q) = %v/%v, want %v/%v", tt.tif, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFutuReadHelperMappingsAndPushSnapshots(t *testing.T) {
	value := "raw"
	if optionalStringValue(nil) != "" || optionalStringValue(&value) != "raw" {
		t.Fatal("optionalStringValue did not preserve nil/value semantics")
	}
	if optionalUint64StringPtr(nil) != nil {
		t.Fatal("optionalUint64StringPtr(nil) should be nil")
	}
	if got := optionalUint64StringPtr(new(uint64(9001))); got == nil || *got != "9001" {
		t.Fatalf("optionalUint64StringPtr = %#v", got)
	}
	unknown := int32(trdcommonpb.OrderType_OrderType_Unknown)
	if optionalEnumStringPtr(&unknown, trdcommonpb.OrderType_name) != nil {
		t.Fatal("unknown enum should not be serialized")
	}
	normal := int32(trdcommonpb.OrderType_OrderType_Normal)
	if got := optionalEnumStringPtr(&normal, trdcommonpb.OrderType_name); got == nil || *got != "NORMAL" {
		t.Fatalf("normal enum = %#v", got)
	}

	if value, ok := sessionValue("RTH"); !ok || value != int32(commonpb.Session_Session_RTH) {
		t.Fatalf("sessionValue(RTH) = %d/%v", value, ok)
	}
	if value, ok := sessionValue("bad-session"); ok || value != 0 {
		t.Fatalf("sessionValue(bad) = %d/%v", value, ok)
	}
	if got := cashFlowDirectionValue("in"); got == nil || *got != int32(trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_In) {
		t.Fatalf("cashFlowDirectionValue(in) = %#v", got)
	}
	if got := cashFlowDirectionValue(""); got != nil {
		t.Fatalf("cashFlowDirectionValue(blank) = %#v", got)
	}
	if got := cashFlowDirectionValue("sideways"); got != nil {
		t.Fatalf("cashFlowDirectionValue(sideways) = %#v", got)
	}

	if !isMarginRatioRateLimitedError(errors.New("每30秒最多10次")) ||
		!isMarginRatioRateLimitedError(errors.New("request frequency too high")) ||
		!isMarginRatioRateLimitedError(errors.New("rate limit exceeded")) ||
		isMarginRatioRateLimitedError(errors.New("unknown stock")) ||
		isMarginRatioRateLimitedError(nil) {
		t.Fatal("isMarginRatioRateLimitedError classification mismatch")
	}

	header := &trdcommonpb.TrdHeader{
		TrdEnv:    futuTestPtr(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:     new(uint64(9001)),
		TrdMarket: futuTestPtr(int32(trdcommonpb.TrdMarket_TrdMarket_US)),
	}
	order := &trdcommonpb.Order{
		OrderID:     new(uint64(123)),
		OrderIDEx:   new("EX-123"),
		Code:        new("AAPL"),
		Name:        new("Apple"),
		TrdSide:     futuTestPtr(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:   futuTestPtr(int32(trdcommonpb.OrderType_OrderType_Market)),
		OrderStatus: futuTestPtr(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		Qty:         new(10.0),
		Price:       new(188.5),
		TimeInForce: futuTestPtr(int32(trdcommonpb.TimeInForce_TimeInForce_DAY)),
	}
	snapshot := BrokerOrderSnapshotFromPush(header, order)
	if snapshot.AccountID != "9001" || snapshot.TradingEnvironment != "SIMULATE" || snapshot.Market != "US" ||
		snapshot.Symbol != "AAPL" || snapshot.BrokerOrderID != "123" || snapshot.OrderType != "MARKET" {
		t.Fatalf("BrokerOrderSnapshotFromPush = %#v", snapshot)
	}

	maxQty := convertMaxTradeQuantitySnapshot(&BrokerMaxTradeQuantitySnapshot{
		AccountID: "9001", TradingEnvironment: "SIMULATE", Market: "US", Symbol: "US.AAPL",
		OrderType: "LIMIT", Price: 188.5, MaxCashBuy: 10, MaxPositionSell: 5,
	})
	if maxQty.AccountID != "9001" || maxQty.Symbol != "US.AAPL" || maxQty.MaxCashBuy != 10 {
		t.Fatalf("convertMaxTradeQuantitySnapshot = %#v", maxQty)
	}
	if empty := convertMaxTradeQuantitySnapshot(nil); empty != (broker.MaxTradeQuantitySnapshot{}) {
		t.Fatalf("nil max trade quantity snapshot = %#v", empty)
	}
}

func TestFutuMarketAndSecuritySymbolBoundaries(t *testing.T) {
	for _, tt := range []struct {
		market qotcommonpb.QotMarket
		want   string
	}{
		{qotcommonpb.QotMarket_QotMarket_HK_Security, "HK"},
		{qotcommonpb.QotMarket_QotMarket_US_Security, "US"},
		{qotcommonpb.QotMarket_QotMarket_CNSH_Security, "SH"},
		{qotcommonpb.QotMarket_QotMarket_CNSZ_Security, "SZ"},
	} {
		if got, err := futuMarketCodeFromQotMarket(tt.market); err != nil || got != tt.want {
			t.Fatalf("futuMarketCodeFromQotMarket(%s) = %q err=%v", tt.market, got, err)
		}
	}
	if _, err := futuMarketCodeFromQotMarket(qotcommonpb.QotMarket_QotMarket_Unknown); err == nil {
		t.Fatal("unknown qot market error = nil")
	}
	if _, err := futuSymbolFromSecurity(nil); err == nil {
		t.Fatal("nil security error = nil")
	}
	if _, err := futuSymbolFromSecurity(&qotcommonpb.Security{Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_US_Security))}); err == nil {
		t.Fatal("empty security code error = nil")
	}
	if got, err := futuSymbolFromSecurity(&qotcommonpb.Security{
		Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
		Code:   new(" aapl "),
	}); err != nil || got != "US.AAPL" {
		t.Fatalf("futuSymbolFromSecurity = %q err=%v", got, err)
	}
	if market := inferMarket("us.aapl"); market.Symbol != "US.AAPL" || market.QuoteCurrency != "USD" || market.PricePrecision == 0 {
		t.Fatalf("inferMarket US = %+v", market)
	}
	if market := inferMarket("unknown.asset"); market.Symbol != "UNKNOWN.ASSET" || market.QuoteCurrency != "HKD" || market.TickSize.Sign() <= 0 {
		t.Fatalf("inferMarket fallback = %+v", market)
	}
}

func TestFutuOrderBookSubscriptionRequestExtraction(t *testing.T) {
	requests, err := orderBookRequestsFromSubscriptions([]types.Subscription{
		{Channel: types.KLineChannel, Symbol: "US.IGNORED"},
		{Channel: types.BookTickerChannel, Symbol: "us.aapl"},
		{Channel: types.BookTickerChannel, Symbol: "US.AAPL"},
		{Channel: types.BookTickerChannel, Symbol: "HK.00700"},
	})
	if err != nil {
		t.Fatalf("orderBookRequestsFromSubscriptions: %v", err)
	}
	if len(requests) != 2 || requests[0].canonical != "US.AAPL" || requests[1].canonical != "HK.00700" {
		t.Fatalf("order book requests = %#v", requests)
	}
	if _, err := orderBookRequestsFromSubscriptions([]types.Subscription{{Channel: types.BookTickerChannel, Symbol: "BAD"}}); err == nil {
		t.Fatal("invalid order book symbol error = nil")
	}
}
