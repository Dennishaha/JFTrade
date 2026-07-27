package futu

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
	"github.com/shopspring/decimal"
)

func TestBrokerAdapterForwardsUnavailableOpenDErrors(t *testing.T) {
	exchange := NewExchangeWithConfig(opend.Config{Addr: "127.0.0.1:1", RequestTimeout: 50 * time.Millisecond})
	t.Cleanup(func() { jftradeCheckTestError(t, exchange.Close()) })
	adapter := NewBrokerAdapter(exchange)

	if _, err := adapter.DiscoverAccounts(t.Context()); err == nil {
		t.Fatal("DiscoverAccounts(unavailable OpenD) error = nil")
	}
	trading := adapter.Trading()
	if _, err := trading.PlaceOrder(t.Context(), broker.PlaceOrderQuery{Symbol: "HK.00700", Quantity: 100}); err == nil {
		t.Fatal("PlaceOrder(unavailable OpenD) error = nil")
	}
	reader := adapter.MarketData()
	if _, err := reader.QueryFunds(t.Context(), broker.ReadQuery{}); err == nil {
		t.Fatal("QueryFunds(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryPositions(t.Context(), broker.ReadQuery{}); err == nil {
		t.Fatal("QueryPositions(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryOrders(t.Context(), broker.ReadQuery{}, ""); err == nil {
		t.Fatal("QueryOrders(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryHistoryOrders(t.Context(), broker.OrderHistoryQuery{}); err == nil {
		t.Fatal("QueryHistoryOrders(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryOrderFills(t.Context(), broker.OrderFillQuery{}); err == nil {
		t.Fatal("QueryOrderFills(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryHistoryOrderFills(t.Context(), broker.OrderFillHistoryQuery{}); err == nil {
		t.Fatal("QueryHistoryOrderFills(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryOrderFees(t.Context(), broker.OrderFeeQuery{}); err == nil {
		t.Fatal("QueryOrderFees(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryMarginRatios(t.Context(), broker.MarginRatioQuery{Symbols: []string{"HK.00700"}}); err == nil {
		t.Fatal("QueryMarginRatios(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryCashFlows(t.Context(), broker.CashFlowQuery{}); err == nil {
		t.Fatal("QueryCashFlows(unavailable OpenD) error = nil")
	}
	if _, err := reader.QueryMaxTradeQuantity(t.Context(), broker.MaxTradeQuantityQuery{Symbol: "HK.00700"}); err == nil {
		t.Fatal("QueryMaxTradeQuantity(unavailable OpenD) error = nil")
	}
}

func TestAdapterSubscriptionParsingErrorsAreReturned(t *testing.T) {
	_, exchange := coverageMarginExchange(t)
	adapter := NewBrokerAdapter(exchange)
	quoteSubscriber := adapter.(broker.QuoteSubscriber)
	if err := quoteSubscriber.SubscribeQuotes(t.Context(), broker.QuoteSubscribeRequest{Symbols: []string{"BAD"}}); err == nil {
		t.Fatal("SubscribeQuotes(invalid symbol) error = nil")
	}
	orderBookSubscriber := adapter.(broker.OrderBookSubscriber)
	if err := orderBookSubscriber.SubscribeOrderBook(t.Context(), broker.OrderBookSubscribeRequest{Symbols: []string{"BAD"}}); err == nil {
		t.Fatal("SubscribeOrderBook(invalid symbol) error = nil")
	}
}

func TestAdapterAndDecimalConversionBoundaries(t *testing.T) {
	invalid := &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_Unknown)), Code: new("AAPL")}
	if got := securitySymbol(invalid); got != "" {
		t.Fatalf("securitySymbol(invalid) = %q", got)
	}
	if got := fixedpointFromDecimalPtr(nil); got != fixedpoint.Zero {
		t.Fatalf("fixedpointFromDecimalPtr(nil) = %s", got)
	}
	value := decimal.NewFromInt(7)
	if got := fixedpointFromDecimalPtr(&value); got != fixedpoint.NewFromInt(7) {
		t.Fatalf("fixedpointFromDecimalPtr(7) = %s", got)
	}
	stopPrice := 12.5
	converted := bbgoSubmitOrderFromBrokerPlaceOrder(broker.PlaceOrderQuery{StopPrice: &stopPrice, ReduceOnly: true})
	if converted.StopPrice != fixedpoint.NewFromFloat(stopPrice) {
		t.Fatalf("converted stop price = %s", converted.StopPrice)
	}
	if !converted.ReduceOnly {
		t.Fatal("converted reduce-only flag = false")
	}
	if tickerFromBasicQot(nil) != nil {
		t.Fatal("tickerFromBasicQot(nil) != nil")
	}
	ticker := tickerFromBasicQot(&qotcommonpb.BasicQot{Security: invalid, CurPrice: new(1.0)})
	if ticker == nil || ticker.Last != fixedpoint.One {
		t.Fatalf("tickerFromBasicQot(invalid security) = %#v", ticker)
	}
	if got := brokerOrderStatus(bbgotypes.Order{OriginalStatus: " submitted "}); got != "submitted" {
		t.Fatalf("original broker status = %q", got)
	}
	if got := brokerOrderStatus(bbgotypes.Order{Status: bbgotypes.OrderStatusFilled}); got != string(bbgotypes.OrderStatusFilled) {
		t.Fatalf("fallback broker status = %q", got)
	}
	if got, err := futuKLTypeFromIntervalString("3m"); err != nil || got != qotcommonpb.KLType_KLType_3Min {
		t.Fatalf("3m K-line type = %v, %v", got, err)
	}
}

func TestKLineSessionAndPriceHelperBoundaries(t *testing.T) {
	plan := historicalKLineRequestPlan{}
	if shouldFallbackHistoricalKLineSplit(nil, plan) {
		t.Fatal("nil-session plan unexpectedly falls back")
	}
	session := commonpb.Session_Session_ETH
	plan.session = &session
	if shouldFallbackHistoricalKLineSplit(&historicalKLineRequestError{session: &session}, plan) {
		t.Fatal("empty route error unexpectedly falls back")
	}
	start := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	if got := resolveHistoricalMarketSession(commonpb.Session_Session_OVERNIGHT, "US.AAPL", bbgotypes.KLine{}); got != market.SessionOvernight {
		t.Fatalf("overnight session = %s", got)
	}
	if got := resolveETHHistoricalKLineSession("HK.00700", bbgotypes.KLine{}); got != market.SessionUnknown {
		t.Fatalf("non-US zero session = %s", got)
	}
	endOnly := bbgotypes.KLine{EndTime: bbgotypes.Time(start)}
	if got := resolveETHHistoricalKLineSession("US.AAPL", endOnly); got != market.SessionPre {
		t.Fatalf("end-time ETH session = %s", got)
	}
	after := bbgotypes.KLine{StartTime: bbgotypes.Time(start.Add(6 * time.Hour))}
	if got := resolveETHHistoricalKLineSession("US.AAPL", after); got != market.SessionAfter {
		t.Fatalf("afternoon ETH session = %s", got)
	}
	window := filterKLinesByWindow([]bbgotypes.KLine{
		{StartTime: bbgotypes.Time(start.Add(-2 * time.Hour)), EndTime: bbgotypes.Time(start.Add(-time.Hour))},
		{StartTime: bbgotypes.Time(start), EndTime: bbgotypes.Time(start.Add(time.Minute))},
		{StartTime: bbgotypes.Time(start.Add(2 * time.Hour)), EndTime: bbgotypes.Time(start.Add(3 * time.Hour))},
	}, start.Add(-time.Minute), start.Add(time.Hour))
	if len(window) != 1 {
		t.Fatalf("filtered K-lines = %d", len(window))
	}
	if got := futuHistoryKLineStartTime(start, bbgotypes.Interval1d); !got.Equal(start) {
		t.Fatalf("daily interval start = %s", got)
	}
	if got := normalizeSubmitOrderPrice("US.AAPL", fixedpoint.Zero); got != fixedpoint.Zero {
		t.Fatalf("zero normalized price = %s", got)
	}
	if got := roundPriceToStep(math.NaN(), 0.01); !math.IsNaN(got) {
		t.Fatalf("NaN round result = %v", got)
	}
	if got := roundPriceToStep(1, 0); got != 1 {
		t.Fatalf("zero-step round result = %v", got)
	}
	if got := stepRoundedUnit(-1000); got != 1 {
		t.Fatalf("underflowed step unit = %v", got)
	}
}

func TestCanceledContextIsPreservedByUnavailableAdapter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exchange := NewExchange("")
	t.Cleanup(func() { jftradeCheckTestError(t, exchange.Close()) })
	if _, err := exchange.DiscoverAccounts(ctx); err == nil {
		t.Fatal("DiscoverAccounts(canceled) error = nil")
	}
}
