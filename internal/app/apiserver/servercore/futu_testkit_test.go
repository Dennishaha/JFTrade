package servercore

import (
	"testing"
	"time"

	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
)

type brokerRouteOpenDServer struct {
	fixture *fututestkit.BrokerServer
	addr    string
}

func startBrokerRouteOpenDServer(t *testing.T) *brokerRouteOpenDServer {
	t.Helper()
	fixture := fututestkit.StartBrokerServer(t)
	return &brokerRouteOpenDServer{fixture: fixture, addr: fixture.Addr()}
}

func (s *brokerRouteOpenDServer) stop() { s.fixture.Close() }

func (s *brokerRouteOpenDServer) setServerVersion(version, build int32) {
	s.fixture.SetServerVersion(version, build)
}

func (s *brokerRouteOpenDServer) setAccounts(accounts []fututestkit.Account) {
	s.fixture.SetAccounts(accounts)
}

func (s *brokerRouteOpenDServer) setFunds(funds fututestkit.Funds) {
	s.fixture.SetFunds(funds)
}

func (s *brokerRouteOpenDServer) setPositions(positions []fututestkit.Position) {
	s.fixture.SetPositions(positions)
}

func (s *brokerRouteOpenDServer) setOrders(orders []fututestkit.Order) {
	s.fixture.SetOrders(orders)
}

func (s *brokerRouteOpenDServer) setHistoryOrders(orders []fututestkit.Order) {
	s.fixture.SetHistoryOrders(orders)
}

func (s *brokerRouteOpenDServer) setOrderFills(fills []fututestkit.OrderFill) {
	s.fixture.SetOrderFills(fills)
}

func (s *brokerRouteOpenDServer) setHistoryFills(fills []fututestkit.OrderFill) {
	s.fixture.SetHistoryFills(fills)
}

func (s *brokerRouteOpenDServer) setOrderFees(fees []fututestkit.OrderFee) {
	s.fixture.SetOrderFees(fees)
}

func (s *brokerRouteOpenDServer) setCashFlows(flows []fututestkit.CashFlow) {
	s.fixture.SetCashFlows(flows)
}

func (s *brokerRouteOpenDServer) setMarginRatios(ratios []fututestkit.MarginRatio) {
	s.fixture.SetMarginRatios(ratios)
}

func (s *brokerRouteOpenDServer) setMaxTrdQtys(value fututestkit.MaxTradeQuantities) {
	s.fixture.SetMaxTradeQuantities(value)
}

func (s *brokerRouteOpenDServer) setPlacedOrderResponse(id uint64, externalID string) {
	s.fixture.SetPlacedOrderResponse(id, externalID)
}

func (s *brokerRouteOpenDServer) placeOrderCallCount() int {
	return s.fixture.PlaceOrderCallCount()
}

func (s *brokerRouteOpenDServer) modifyOrderCallCount() int {
	return s.fixture.ModifyOrderCallCount()
}

func (s *brokerRouteOpenDServer) subAccPushCallCount() int {
	return s.fixture.SubAccountPushCallCount()
}

func (s *brokerRouteOpenDServer) lastPlaceOrderRequest() *fututestkit.PlaceOrderRequest {
	return s.fixture.LastPlaceOrderRequest()
}

func (s *brokerRouteOpenDServer) seedReadEndpointData() {
	s.setAccounts([]fututestkit.Account{
		{Environment: "SIMULATE", ID: 1001, Markets: []string{"HK"}, Type: "CASH"},
		{Environment: "REAL", ID: 2001, Markets: []string{"HK"}, Type: "MARGIN"},
	})
	s.setFunds(fututestkit.Funds{
		Power: 120000, TotalAssets: 100000, Cash: 40000, MarketValue: 60000,
		FrozenCash: 500, AvailableToWithdraw: 39500, Currency: "HKD",
		CashEntries: []fututestkit.CashEntry{{
			Currency: "HKD", Cash: 40000, AvailableBalance: 39500, NetCashPower: 120000,
		}},
		MarketEntries: []fututestkit.MarketEntry{{Market: "HK", Assets: 100000}},
	})
	s.setPositions([]fututestkit.Position{{
		ID: 1, Side: 1, Code: "HK.00700", Name: "Tencent", Quantity: 200,
		SellableQuantity: 180, Price: 320.5, CostPrice: 300, AverageCostPrice: 301,
		Value: 64100, ProfitLoss: 3900, ProfitLossRatio: 13, Market: "HK", Currency: "HKD",
	}})
	s.setOrders([]fututestkit.Order{{
		Side: "BUY", Type: "NORMAL", Status: "SUBMITTED", ID: 2001, ExternalID: "EXT-2001",
		Code: "HK.00700", Name: "Tencent", Quantity: 100, Price: 319.8,
		CreatedAt: "2026-05-20 09:30:00", UpdatedAt: "2026-05-20 09:31:00",
		FilledQuantity: 20, AverageFillPrice: 319.5, TimeInForce: "GTC", Currency: "HKD", Market: "HK",
	}})
	s.setHistoryOrders([]fututestkit.Order{{
		Side: "BUY", Type: "NORMAL", Status: "FILLED", ID: 2101, ExternalID: "EXT-2101",
		Code: "HK.00700", Name: "Tencent", Quantity: 50, Price: 321.2,
		CreatedAt: "2026-05-19 09:30:00", UpdatedAt: "2026-05-19 09:45:00",
		FilledQuantity: 50, AverageFillPrice: 321.1, TimeInForce: "GTC", Currency: "HKD", Market: "HK",
	}})
	s.setOrderFills([]fututestkit.OrderFill{{
		OrderID: 2001, OrderIDEx: "EXT-2001", FillID: 3001, FillIDEx: "FILL-3001",
		Code: "HK.00700", Name: "Tencent", Side: "BUY", Quantity: 20, Price: 319.5,
		CreatedAt: "2026-05-20 09:31:30", Status: "OK", Market: "HK",
	}})
	s.setHistoryFills([]fututestkit.OrderFill{{
		OrderID: 2101, OrderIDEx: "EXT-2101", FillID: 3101, FillIDEx: "FILL-3101",
		Code: "HK.00700", Name: "Tencent", Side: "BUY", Quantity: 50, Price: 321.1,
		CreatedAt: "2026-05-19 09:40:00", Status: "OK", Market: "HK",
	}})
	s.setOrderFees([]fututestkit.OrderFee{{
		OrderIDEx: "EXT-2001", Amount: 12.5,
		Items: []fututestkit.FeeItem{{Title: "Commission", Value: 10}},
	}})
	s.setCashFlows([]fututestkit.CashFlow{{
		ID: 5001, ClearingDate: "2026-05-20", SettlementDate: "2026-05-21", Currency: "HKD",
		Type: "DIVIDEND", Direction: "IN", Amount: 88.8, Remark: "cash-flow-test",
	}})
	s.setMarginRatios([]fututestkit.MarginRatio{
		{Market: "HK", Code: "00700", LongPermitted: true, ShortFeeRate: 1.25, AlertLongRatio: 0.3},
		{Market: "HK", Code: "07226", LongPermitted: true, ShortPermitted: true},
	})
	s.setMaxTrdQtys(fututestkit.MaxTradeQuantities{
		CashBuy: 1000, CashAndMarginBuy: 2000, PositionSell: 500, SellShort: 300,
		BuyBack: 150, LongRequiredIM: 10, ShortRequiredIM: 12, Session: "RTH",
	})
}

type marketDataQuoteOpenDServer struct {
	fixture *fututestkit.QuoteServer
	addr    string
}

func startMarketDataQuoteOpenDServer(t *testing.T) *marketDataQuoteOpenDServer {
	t.Helper()
	fixture := fututestkit.StartQuoteServer(t)
	return &marketDataQuoteOpenDServer{fixture: fixture, addr: fixture.Addr()}
}

func (s *marketDataQuoteOpenDServer) stop()                  { s.fixture.Close() }
func (s *marketDataQuoteOpenDServer) basicQotCallCount() int { return s.fixture.BasicQuoteCallCount() }
func (s *marketDataQuoteOpenDServer) securitySnapshotCallCount() int {
	return s.fixture.SecuritySnapshotCallCount()
}
func (s *marketDataQuoteOpenDServer) staticInfoCallCount() int {
	return s.fixture.StaticInfoCallCount()
}
func (s *marketDataQuoteOpenDServer) searchQuoteCallCount() int {
	return s.fixture.SearchQuoteCallCount()
}
func (s *marketDataQuoteOpenDServer) qotSubCallCount() int    { return s.fixture.SubscribeCallCount() }
func (s *marketDataQuoteOpenDServer) orderBookCallCount() int { return s.fixture.OrderBookCallCount() }
func (s *marketDataQuoteOpenDServer) orderBookLastNum() int32 { return s.fixture.OrderBookLastNum() }
func (s *marketDataQuoteOpenDServer) currentKLCallCount() int {
	return s.fixture.CurrentKLineCallCount()
}

func (s *marketDataQuoteOpenDServer) setOrderBook(bids, asks []fututestkit.OrderBookEntry) {
	s.fixture.SetOrderBook(bids, asks)
}

func (s *marketDataQuoteOpenDServer) setOrderBookErr(err error) {
	s.fixture.SetOrderBookError(err)
}

func (s *marketDataQuoteOpenDServer) setHistoryPages(pages [][]fututestkit.KLine) {
	s.fixture.SetHistoryPages(pages)
}

func (s *marketDataQuoteOpenDServer) setHistoryPagesBySession(pages map[string][][]fututestkit.KLine) {
	s.fixture.SetHistoryPagesBySession(pages)
}

func (s *marketDataQuoteOpenDServer) setCurrentKLines(lines []fututestkit.KLine) {
	s.fixture.SetCurrentKLines(lines)
}

func testMarketDataProtoKLine(at time.Time, open, high, low, close float64, volume int64) fututestkit.KLine {
	return fututestkit.KLine{At: at, Open: open, High: high, Low: low, Close: close, Volume: volume}
}

func marketDataDepthOrderBookFixture(price float64, volume int64, orderCount int32) fututestkit.OrderBookEntry {
	return fututestkit.OrderBookEntry{Price: price, Volume: volume, OrderCount: orderCount}
}
