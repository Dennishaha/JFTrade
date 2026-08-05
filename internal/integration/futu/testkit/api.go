// Package testkit provides semantic OpenD fixtures for tests outside the Futu
// integration boundary. Protocol frames and generated protobuf types remain
// private to this package.
package testkit

import (
	"strings"
	"testing"
	"time"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

type Account struct {
	Environment string
	ID          uint64
	Markets     []string
	Type        string
}

type Funds struct {
	Power               float64
	TotalAssets         float64
	Cash                float64
	MarketValue         float64
	FrozenCash          float64
	DebtCash            float64
	AvailableToWithdraw float64
	Currency            string
	CashEntries         []CashEntry
	MarketEntries       []MarketEntry
}

type CashEntry struct {
	Currency         string
	Cash             float64
	AvailableBalance float64
	NetCashPower     float64
}

type MarketEntry struct {
	Market string
	Assets float64
}

type Position struct {
	ID               uint64
	Side             int32
	Code             string
	Name             string
	Quantity         float64
	SellableQuantity float64
	Price            float64
	CostPrice        float64
	AverageCostPrice float64
	Value            float64
	ProfitLoss       float64
	ProfitLossRatio  float64
	Market           string
	Currency         string
}

type Order struct {
	Side             string
	Type             string
	Status           string
	ID               uint64
	ExternalID       string
	Code             string
	Name             string
	Quantity         float64
	Price            float64
	CreatedAt        string
	UpdatedAt        string
	FilledQuantity   float64
	AverageFillPrice float64
	TimeInForce      string
	Currency         string
	Market           string
}

type OrderFill struct {
	OrderID   uint64
	OrderIDEx string
	FillID    uint64
	FillIDEx  string
	Code      string
	Name      string
	Side      string
	Quantity  float64
	Price     float64
	CreatedAt string
	Status    string
	Market    string
}

type FeeItem struct {
	Title string
	Value float64
}

type OrderFee struct {
	OrderIDEx string
	Amount    float64
	Items     []FeeItem
}

type CashFlow struct {
	ID             uint64
	ClearingDate   string
	SettlementDate string
	Currency       string
	Type           string
	Direction      string
	Amount         float64
	Remark         string
}

type MarginRatio struct {
	Market         string
	Code           string
	LongPermitted  bool
	ShortPermitted bool
	ShortFeeRate   float64
	AlertLongRatio float64
}

type MaxTradeQuantities struct {
	CashBuy          float64
	CashAndMarginBuy float64
	PositionSell     float64
	SellShort        float64
	BuyBack          float64
	LongRequiredIM   float64
	ShortRequiredIM  float64
	Session          string
}

type PlaceOrderRequest struct {
	Price          float64
	Code           string
	Session        string
	FillOutsideRTH *bool
}

type BrokerServer struct {
	inner *brokerRouteOpenDServer
}

func StartBrokerServer(t *testing.T) *BrokerServer {
	t.Helper()
	return &BrokerServer{inner: startBrokerRouteOpenDServer(t)}
}

func (s *BrokerServer) Addr() string { return s.inner.addr }
func (s *BrokerServer) Close()       { s.inner.stop() }

func (s *BrokerServer) SetServerVersion(version, build int32) {
	s.inner.setServerVersion(version, build)
}

func (s *BrokerServer) SetAccounts(accounts []Account) {
	values := make([]*trdcommonpb.TrdAcc, 0, len(accounts))
	for _, account := range accounts {
		markets := make([]int32, 0, len(account.Markets))
		for _, market := range account.Markets {
			markets = append(markets, int32(tradingMarket(market)))
		}
		values = append(values, &trdcommonpb.TrdAcc{
			TrdEnv: new(int32(tradingEnvironment(account.Environment))), AccID: new(account.ID),
			TrdMarketAuthList: markets, AccType: new(int32(accountType(account.Type))),
		})
	}
	s.inner.setAccounts(values)
}

func (s *BrokerServer) SetFunds(funds Funds) {
	cashEntries := make([]*trdcommonpb.AccCashInfo, 0, len(funds.CashEntries))
	for _, entry := range funds.CashEntries {
		cashEntries = append(cashEntries, &trdcommonpb.AccCashInfo{
			Currency: new(int32(currency(entry.Currency))), Cash: new(entry.Cash),
			AvailableBalance: new(entry.AvailableBalance), NetCashPower: new(entry.NetCashPower),
		})
	}
	marketEntries := make([]*trdcommonpb.AccMarketInfo, 0, len(funds.MarketEntries))
	for _, entry := range funds.MarketEntries {
		marketEntries = append(marketEntries, &trdcommonpb.AccMarketInfo{
			TrdMarket: new(int32(tradingMarket(entry.Market))), Assets: new(entry.Assets),
		})
	}
	s.inner.setFunds(&trdcommonpb.Funds{
		Power: new(funds.Power), TotalAssets: new(funds.TotalAssets), Cash: new(funds.Cash),
		MarketVal: new(funds.MarketValue), FrozenCash: new(funds.FrozenCash), DebtCash: new(funds.DebtCash),
		AvlWithdrawalCash: new(funds.AvailableToWithdraw), Currency: new(int32(currency(funds.Currency))),
		CashInfoList: cashEntries, MarketInfoList: marketEntries,
	})
}

func (s *BrokerServer) SetPositions(positions []Position) {
	values := make([]*trdcommonpb.Position, 0, len(positions))
	for _, position := range positions {
		values = append(values, &trdcommonpb.Position{
			PositionID: new(position.ID), PositionSide: new(position.Side), Code: new(position.Code), Name: new(position.Name),
			Qty: new(position.Quantity), CanSellQty: new(position.SellableQuantity), Price: new(position.Price),
			CostPrice: new(position.CostPrice), AverageCostPrice: new(position.AverageCostPrice), Val: new(position.Value),
			PlVal: new(position.ProfitLoss), PlRatio: new(position.ProfitLossRatio),
			TrdMarket: new(int32(tradingMarket(position.Market))), Currency: new(int32(currency(position.Currency))),
		})
	}
	s.inner.setPositions(values)
}

func (s *BrokerServer) SetOrders(orders []Order) { s.inner.setOrders(protocolOrders(orders)) }
func (s *BrokerServer) SetHistoryOrders(orders []Order) {
	s.inner.setHistoryOrders(protocolOrders(orders))
}

func protocolOrders(orders []Order) []*trdcommonpb.Order {
	values := make([]*trdcommonpb.Order, 0, len(orders))
	for _, order := range orders {
		values = append(values, &trdcommonpb.Order{
			TrdSide: new(int32(tradingSide(order.Side))), OrderType: new(int32(orderType(order.Type))),
			OrderStatus: new(int32(orderStatus(order.Status))), OrderID: new(order.ID), OrderIDEx: new(order.ExternalID),
			Code: new(order.Code), Name: new(order.Name), Qty: new(order.Quantity), Price: new(order.Price),
			CreateTime: new(order.CreatedAt), UpdateTime: new(order.UpdatedAt), FillQty: new(order.FilledQuantity),
			FillAvgPrice: new(order.AverageFillPrice), TimeInForce: new(int32(timeInForce(order.TimeInForce))),
			Currency: new(int32(currency(order.Currency))), TrdMarket: new(int32(tradingMarket(order.Market))),
		})
	}
	return values
}

func (s *BrokerServer) SetOrderFills(fills []OrderFill) {
	s.inner.setOrderFills(protocolOrderFills(fills))
}
func (s *BrokerServer) SetHistoryFills(fills []OrderFill) {
	s.inner.setHistoryFills(protocolOrderFills(fills))
}

func protocolOrderFills(fills []OrderFill) []*trdcommonpb.OrderFill {
	values := make([]*trdcommonpb.OrderFill, 0, len(fills))
	for _, fill := range fills {
		values = append(values, &trdcommonpb.OrderFill{
			OrderID: new(fill.OrderID), OrderIDEx: new(fill.OrderIDEx), FillID: new(fill.FillID), FillIDEx: new(fill.FillIDEx),
			Code: new(fill.Code), Name: new(fill.Name), TrdSide: new(int32(tradingSide(fill.Side))), Qty: new(fill.Quantity),
			Price: new(fill.Price), CreateTime: new(fill.CreatedAt), Status: new(int32(orderFillStatus(fill.Status))),
			TrdMarket: new(int32(tradingMarket(fill.Market))),
		})
	}
	return values
}

func (s *BrokerServer) SetOrderFees(fees []OrderFee) {
	values := make([]*trdcommonpb.OrderFee, 0, len(fees))
	for _, fee := range fees {
		items := make([]*trdcommonpb.OrderFeeItem, 0, len(fee.Items))
		for _, item := range fee.Items {
			items = append(items, &trdcommonpb.OrderFeeItem{Title: new(item.Title), Value: new(item.Value)})
		}
		values = append(values, &trdcommonpb.OrderFee{OrderIDEx: new(fee.OrderIDEx), FeeAmount: new(fee.Amount), FeeList: items})
	}
	s.inner.setOrderFees(values)
}

func (s *BrokerServer) SetCashFlows(flows []CashFlow) {
	values := make([]*trdflowsummarypb.FlowSummaryInfo, 0, len(flows))
	for _, flow := range flows {
		values = append(values, &trdflowsummarypb.FlowSummaryInfo{
			CashFlowID: new(flow.ID), ClearingDate: new(flow.ClearingDate), SettlementDate: new(flow.SettlementDate),
			Currency: new(int32(currency(flow.Currency))), CashFlowType: new(flow.Type),
			CashFlowDirection: new(int32(cashFlowDirection(flow.Direction))), CashFlowAmount: new(flow.Amount),
			CashFlowRemark: new(flow.Remark),
		})
	}
	s.inner.setCashFlows(values)
}

func (s *BrokerServer) SetMarginRatios(ratios []MarginRatio) {
	values := make([]*trdgetmarginratiopb.MarginRatioInfo, 0, len(ratios))
	for _, ratio := range ratios {
		values = append(values, &trdgetmarginratiopb.MarginRatioInfo{
			Security:     &qotcommonpb.Security{Market: new(int32(quoteMarket(ratio.Market))), Code: new(ratio.Code)},
			IsLongPermit: new(ratio.LongPermitted), IsShortPermit: new(ratio.ShortPermitted),
			ShortFeeRate: new(ratio.ShortFeeRate), AlertLongRatio: new(ratio.AlertLongRatio),
		})
	}
	s.inner.setMarginRatios(values)
}

func (s *BrokerServer) SetMaxTradeQuantities(value MaxTradeQuantities) {
	s.inner.setMaxTrdQtys(&trdcommonpb.MaxTrdQtys{
		MaxCashBuy: new(value.CashBuy), MaxCashAndMarginBuy: new(value.CashAndMarginBuy),
		MaxPositionSell: new(value.PositionSell), MaxSellShort: new(value.SellShort), MaxBuyBack: new(value.BuyBack),
		LongRequiredIM: new(value.LongRequiredIM), ShortRequiredIM: new(value.ShortRequiredIM),
		Session: new(int32(session(value.Session))),
	})
}

func (s *BrokerServer) SetPlacedOrderResponse(id uint64, externalID string) {
	s.inner.setPlacedOrderResponse(id, externalID)
}

func (s *BrokerServer) PlaceOrderCallCount() int     { return s.inner.placeOrderCallCount() }
func (s *BrokerServer) ModifyOrderCallCount() int    { return s.inner.modifyOrderCallCount() }
func (s *BrokerServer) SubAccountPushCallCount() int { return s.inner.subAccPushCallCount() }

func (s *BrokerServer) LastPlaceOrderRequest() *PlaceOrderRequest {
	request := s.inner.lastPlaceOrderRequest()
	if request == nil {
		return nil
	}
	return &PlaceOrderRequest{
		Price: request.GetPrice(), Code: request.GetCode(), Session: sessionLabel(request.GetSession()),
		FillOutsideRTH: request.FillOutsideRTH,
	}
}

type KLine struct {
	At                     time.Time
	Open, High, Low, Close float64
	Volume                 int64
}

type OrderBookEntry struct {
	Price      float64
	Volume     int64
	OrderCount int32
}

type QuoteServer struct {
	inner *marketDataQuoteOpenDServer
}

func StartQuoteServer(t *testing.T) *QuoteServer {
	t.Helper()
	return &QuoteServer{inner: startMarketDataQuoteOpenDServer(t)}
}

func (s *QuoteServer) Addr() string                   { return s.inner.addr }
func (s *QuoteServer) Close()                         { s.inner.stop() }
func (s *QuoteServer) BasicQuoteCallCount() int       { return s.inner.basicQotCallCount() }
func (s *QuoteServer) SecuritySnapshotCallCount() int { return s.inner.securitySnapshotCallCount() }
func (s *QuoteServer) StaticInfoCallCount() int       { return s.inner.staticInfoCallCount() }
func (s *QuoteServer) SearchQuoteCallCount() int      { return s.inner.searchQuoteCallCount() }
func (s *QuoteServer) SubscribeCallCount() int        { return s.inner.qotSubCallCount() }
func (s *QuoteServer) OrderBookCallCount() int        { return s.inner.orderBookCallCount() }
func (s *QuoteServer) OrderBookLastNum() int32        { return s.inner.orderBookLastNum() }
func (s *QuoteServer) CurrentKLineCallCount() int     { return s.inner.currentKLCallCount() }

func (s *QuoteServer) SetOrderBook(bids, asks []OrderBookEntry) {
	s.inner.setOrderBook(protocolOrderBook(bids), protocolOrderBook(asks))
}

func protocolOrderBook(entries []OrderBookEntry) []*qotcommonpb.OrderBook {
	result := make([]*qotcommonpb.OrderBook, 0, len(entries))
	for _, entry := range entries {
		result = append(result, marketDataDepthOrderBookFixture(entry.Price, entry.Volume, entry.OrderCount))
	}
	return result
}

func (s *QuoteServer) SetOrderBookError(err error) { s.inner.setOrderBookErr(err) }

func (s *QuoteServer) SetHistoryPages(pages [][]KLine) {
	s.inner.setHistoryPages(protocolKLinePages(pages))
}

func (s *QuoteServer) SetHistoryPagesBySession(pages map[string][][]KLine) {
	values := make(map[int32][][]*qotcommonpb.KLine, len(pages))
	for name, sessionPages := range pages {
		values[int32(session(name))] = protocolKLinePages(sessionPages)
	}
	s.inner.setHistoryPagesBySession(values)
}

func (s *QuoteServer) SetCurrentKLines(lines []KLine) {
	s.inner.setCurrentKLines(protocolKLines(lines))
}

func protocolKLinePages(pages [][]KLine) [][]*qotcommonpb.KLine {
	values := make([][]*qotcommonpb.KLine, 0, len(pages))
	for _, page := range pages {
		values = append(values, protocolKLines(page))
	}
	return values
}

func protocolKLines(lines []KLine) []*qotcommonpb.KLine {
	values := make([]*qotcommonpb.KLine, 0, len(lines))
	for _, line := range lines {
		values = append(values, testMarketDataProtoKLine(line.At, line.Open, line.High, line.Low, line.Close, line.Volume))
	}
	return values
}

func tradingEnvironment(value string) trdcommonpb.TrdEnv {
	if strings.EqualFold(value, "REAL") {
		return trdcommonpb.TrdEnv_TrdEnv_Real
	}
	return trdcommonpb.TrdEnv_TrdEnv_Simulate
}

func accountType(value string) trdcommonpb.TrdAccType {
	if strings.EqualFold(value, "MARGIN") {
		return trdcommonpb.TrdAccType_TrdAccType_Margin
	}
	return trdcommonpb.TrdAccType_TrdAccType_Cash
}

func tradingMarket(value string) trdcommonpb.TrdMarket {
	switch strings.ToUpper(value) {
	case "US":
		return trdcommonpb.TrdMarket_TrdMarket_US
	case "CN":
		return trdcommonpb.TrdMarket_TrdMarket_CN
	case "HKCC":
		return trdcommonpb.TrdMarket_TrdMarket_HKCC
	default:
		return trdcommonpb.TrdMarket_TrdMarket_HK
	}
}

func quoteMarket(value string) qotcommonpb.QotMarket {
	switch strings.ToUpper(value) {
	case "US":
		return qotcommonpb.QotMarket_QotMarket_US_Security
	case "SH":
		return qotcommonpb.QotMarket_QotMarket_CNSH_Security
	case "SZ":
		return qotcommonpb.QotMarket_QotMarket_CNSZ_Security
	case "JP":
		return qotcommonpb.QotMarket_QotMarket_JP_Security
	default:
		return qotcommonpb.QotMarket_QotMarket_HK_Security
	}
}

func currency(value string) trdcommonpb.Currency {
	switch strings.ToUpper(value) {
	case "USD":
		return trdcommonpb.Currency_Currency_USD
	case "CNH":
		return trdcommonpb.Currency_Currency_CNH
	default:
		return trdcommonpb.Currency_Currency_HKD
	}
}

func tradingSide(value string) trdcommonpb.TrdSide {
	if strings.EqualFold(value, "SELL") {
		return trdcommonpb.TrdSide_TrdSide_Sell
	}
	return trdcommonpb.TrdSide_TrdSide_Buy
}

func orderType(string) trdcommonpb.OrderType { return trdcommonpb.OrderType_OrderType_Normal }

func orderStatus(value string) trdcommonpb.OrderStatus {
	if strings.EqualFold(value, "FILLED") {
		return trdcommonpb.OrderStatus_OrderStatus_Filled_All
	}
	return trdcommonpb.OrderStatus_OrderStatus_Submitted
}

func orderFillStatus(string) trdcommonpb.OrderFillStatus {
	return trdcommonpb.OrderFillStatus_OrderFillStatus_OK
}

func timeInForce(value string) trdcommonpb.TimeInForce {
	if strings.EqualFold(value, "GTC") {
		return trdcommonpb.TimeInForce_TimeInForce_GTC
	}
	return trdcommonpb.TimeInForce_TimeInForce_DAY
}

func cashFlowDirection(value string) trdflowsummarypb.TrdCashFlowDirection {
	if strings.EqualFold(value, "OUT") {
		return trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_Out
	}
	return trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_In
}

func session(value string) commonpb.Session {
	switch strings.ToUpper(value) {
	case "ETH":
		return commonpb.Session_Session_ETH
	case "ALL":
		return commonpb.Session_Session_ALL
	default:
		return commonpb.Session_Session_RTH
	}
}

func sessionLabel(value int32) string {
	switch commonpb.Session(value) {
	case commonpb.Session_Session_ETH:
		return "ETH"
	case commonpb.Session_Session_ALL:
		return "ALL"
	default:
		return "RTH"
	}
}
