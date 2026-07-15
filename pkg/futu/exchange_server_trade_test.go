package futu

import (
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsubinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsubinfo"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetacclistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetacclist"
	trdgetfundspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetfunds"
	trdgethistoryorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderfilllist"
	trdgethistoryorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderlist"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
	trdgetorderfeepb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfee"
	trdgetorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfilllist"
	trdgetorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderlist"
	trdgetpositionlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetpositionlist"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
)

func (s *quoteOpenDServer) accountListResponse() *trdgetacclistpb.Response {
	s.accountMu.Lock()
	accounts := append([]*trdcommonpb.TrdAcc(nil), s.accounts...)
	s.accountMu.Unlock()
	return &trdgetacclistpb.Response{
		RetType: new(int32(0)),
		S2C: &trdgetacclistpb.S2C{
			AccList: accounts,
		},
	}
}

func (s *quoteOpenDServer) fundsResponse(body []byte) *trdgetfundspb.Response {
	request := &trdgetfundspb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetfundspb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	funds := s.funds
	s.tradeMu.Unlock()
	return &trdgetfundspb.Response{
		RetType: new(int32(0)),
		S2C: &trdgetfundspb.S2C{
			Header: normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			Funds:  normalizeTestFunds(funds),
		},
	}
}

func (s *quoteOpenDServer) positionListResponse(body []byte) *trdgetpositionlistpb.Response {
	request := &trdgetpositionlistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetpositionlistpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	positions := append([]*trdcommonpb.Position(nil), s.positions...)
	s.tradeMu.Unlock()
	normalized := make([]*trdcommonpb.Position, 0, len(positions))
	for _, position := range positions {
		normalized = append(normalized, normalizeTestPosition(position))
	}
	return &trdgetpositionlistpb.Response{
		RetType: new(int32(0)),
		S2C: &trdgetpositionlistpb.S2C{
			Header:       normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			PositionList: normalized,
		},
	}
}

func (s *quoteOpenDServer) orderListResponse(body []byte) *trdgetorderlistpb.Response {
	request := &trdgetorderlistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetorderlistpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	orders := append([]*trdcommonpb.Order(nil), s.orders...)
	s.tradeMu.Unlock()
	normalizedOrders := make([]*trdcommonpb.Order, 0, len(orders))
	for _, order := range orders {
		normalizedOrders = append(normalizedOrders, normalizeTestOrder(order))
	}
	orders = normalizedOrders
	if codes := request.GetC2S().GetFilterConditions().GetCodeList(); len(codes) > 0 {
		filtered := make([]*trdcommonpb.Order, 0, len(orders))
		for _, order := range orders {
			if order == nil {
				continue
			}
			for _, code := range codes {
				if strings.EqualFold(order.GetCode(), code) {
					filtered = append(filtered, order)
					break
				}
			}
		}
		orders = filtered
	}
	return &trdgetorderlistpb.Response{
		RetType: new(int32(0)),
		S2C: &trdgetorderlistpb.S2C{
			Header:    normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderList: orders,
		},
	}
}

func (s *quoteOpenDServer) historyOrderListResponse(body []byte) *trdgethistoryorderlistpb.Response {
	request := &trdgethistoryorderlistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgethistoryorderlistpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	orders := append([]*trdcommonpb.Order(nil), s.historyOrders...)
	s.tradeMu.Unlock()
	normalizedOrders := make([]*trdcommonpb.Order, 0, len(orders))
	for _, order := range orders {
		normalizedOrders = append(normalizedOrders, normalizeTestOrder(order))
	}
	orders = filterTestOrdersByConditions(normalizedOrders, request.GetC2S().GetFilterConditions(), request.GetC2S().GetFilterStatusList())
	return &trdgethistoryorderlistpb.Response{
		RetType: new(int32(0)),
		S2C: &trdgethistoryorderlistpb.S2C{
			Header:    normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderList: orders,
		},
	}
}

func (s *quoteOpenDServer) historyOrderFillListResponse(body []byte) *trdgethistoryorderfilllistpb.Response {
	request := &trdgethistoryorderfilllistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgethistoryorderfilllistpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	fills := append([]*trdcommonpb.OrderFill(nil), s.historyFills...)
	s.tradeMu.Unlock()
	filtered := filterTestFillsByConditions(fills, request.GetC2S().GetFilterConditions())
	return &trdgethistoryorderfilllistpb.Response{
		RetType: new(int32(0)),
		S2C: &trdgethistoryorderfilllistpb.S2C{
			Header:        normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderFillList: filtered,
		},
	}
}

func (s *quoteOpenDServer) orderFillListResponse(body []byte) *trdgetorderfilllistpb.Response {
	request := &trdgetorderfilllistpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetorderfilllistpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	fills := append([]*trdcommonpb.OrderFill(nil), s.orderFills...)
	s.tradeMu.Unlock()
	filtered := filterTestFillsByConditions(fills, request.GetC2S().GetFilterConditions())
	return &trdgetorderfilllistpb.Response{
		RetType: new(int32(0)),
		S2C: &trdgetorderfilllistpb.S2C{
			Header:        normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderFillList: filtered,
		},
	}
}

func (s *quoteOpenDServer) orderFeeResponse(body []byte) *trdgetorderfeepb.Response {
	request := &trdgetorderfeepb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetorderfeepb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	fees := append([]*trdcommonpb.OrderFee(nil), s.orderFees...)
	s.tradeMu.Unlock()
	if ids := request.GetC2S().GetOrderIdExList(); len(ids) > 0 {
		filtered := make([]*trdcommonpb.OrderFee, 0, len(fees))
		for _, fee := range fees {
			if fee == nil {
				continue
			}
			for _, id := range ids {
				if strings.EqualFold(strings.TrimSpace(fee.GetOrderIDEx()), strings.TrimSpace(id)) {
					filtered = append(filtered, fee)
					break
				}
			}
		}
		fees = filtered
	}
	return &trdgetorderfeepb.Response{
		RetType: new(int32(0)),
		S2C: &trdgetorderfeepb.S2C{
			Header:       normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderFeeList: fees,
		},
	}
}

func (s *quoteOpenDServer) marginRatioResponse(body []byte) *trdgetmarginratiopb.Response {
	request := &trdgetmarginratiopb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetmarginratiopb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	if s.marginRatioError != nil {
		response := jftradeCheckedTypeAssertion[*trdgetmarginratiopb.Response](proto.Clone(s.marginRatioError))
		s.tradeMu.Unlock()
		return response
	}
	ratios := append([]*trdgetmarginratiopb.MarginRatioInfo(nil), s.marginRatios...)
	strict := s.strictMarginRatios
	s.tradeMu.Unlock()
	if strict {
		available := make(map[string]struct{}, len(ratios))
		for _, ratio := range ratios {
			if ratio == nil || ratio.GetSecurity() == nil {
				continue
			}
			available[strings.ToUpper(strings.TrimSpace(ratio.GetSecurity().GetCode()))] = struct{}{}
		}
		for _, security := range request.GetC2S().GetSecurityList() {
			code := strings.ToUpper(strings.TrimSpace(security.GetCode()))
			if code == "" {
				continue
			}
			if _, ok := available[code]; !ok {
				return &trdgetmarginratiopb.Response{RetType: new(int32(-1)), ErrCode: new(int32(0)), RetMsg: new("未知股票 " + code)}
			}
		}
	}
	return &trdgetmarginratiopb.Response{
		RetType: new(int32(0)),
		S2C: &trdgetmarginratiopb.S2C{
			Header:              normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			MarginRatioInfoList: ratios,
		},
	}
}

func (s *quoteOpenDServer) flowSummaryResponse(body []byte) *trdflowsummarypb.Response {
	request := &trdflowsummarypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdflowsummarypb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	flows := append([]*trdflowsummarypb.FlowSummaryInfo(nil), s.cashFlows...)
	s.tradeMu.Unlock()
	if direction := request.GetC2S().GetCashFlowDirection(); direction != 0 {
		filtered := make([]*trdflowsummarypb.FlowSummaryInfo, 0, len(flows))
		for _, flow := range flows {
			if flow != nil && flow.GetCashFlowDirection() == direction {
				filtered = append(filtered, flow)
			}
		}
		flows = filtered
	}
	return &trdflowsummarypb.Response{
		RetType: new(int32(0)),
		S2C: &trdflowsummarypb.S2C{
			Header:              normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			FlowSummaryInfoList: flows,
		},
	}
}

func (s *quoteOpenDServer) maxTrdQtysResponse(body []byte) *trdgetmaxtrdqtyspb.Response {
	request := &trdgetmaxtrdqtyspb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdgetmaxtrdqtyspb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.tradeMu.Lock()
	if request.GetC2S() != nil {
		s.lastMaxTrdQtys = jftradeCheckedTypeAssertion[*trdgetmaxtrdqtyspb.C2S](proto.Clone(request.GetC2S()))
	}
	maxQtys := s.maxTrdQtys
	s.tradeMu.Unlock()
	return &trdgetmaxtrdqtyspb.Response{
		RetType: new(int32(0)),
		S2C: &trdgetmaxtrdqtyspb.S2C{
			Header:     normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			MaxTrdQtys: maxQtys,
		},
	}
}

func (s *quoteOpenDServer) placeOrderResponse(body []byte) *trdplaceorderpb.Response {
	request := &trdplaceorderpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdplaceorderpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	if request.GetC2S() == nil {
		return &trdplaceorderpb.Response{RetType: new(int32(1)), RetMsg: new("missing place order payload")}
	}
	s.tradeMu.Lock()
	s.lastPlaceOrder = jftradeCheckedTypeAssertion[*trdplaceorderpb.C2S](proto.Clone(request.GetC2S()))
	orderID := s.placedOrderID
	orderIDEx := s.placedOrderIDEx
	s.tradeMu.Unlock()
	if orderID == 0 {
		orderID = 9001
	}
	if orderIDEx == "" {
		orderIDEx = strconv.FormatUint(orderID, 10)
	}
	if request.GetC2S().GetPacketID().GetConnID() == 0 {
		return &trdplaceorderpb.Response{RetType: new(int32(1)), RetMsg: new("missing packet id connID")}
	}
	return &trdplaceorderpb.Response{
		RetType: new(int32(0)),
		S2C: &trdplaceorderpb.S2C{
			Header:    normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderID:   new(orderID),
			OrderIDEx: new(orderIDEx),
		},
	}
}

func (s *quoteOpenDServer) modifyOrderResponse(body []byte) *trdmodifyorderpb.Response {
	request := &trdmodifyorderpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &trdmodifyorderpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	if request.GetC2S() == nil {
		return &trdmodifyorderpb.Response{RetType: new(int32(1)), RetMsg: new("missing modify order payload")}
	}
	s.tradeMu.Lock()
	s.lastModifyOrder = jftradeCheckedTypeAssertion[*trdmodifyorderpb.C2S](proto.Clone(request.GetC2S()))
	s.tradeMu.Unlock()
	if request.GetC2S().GetPacketID().GetConnID() == 0 {
		return &trdmodifyorderpb.Response{RetType: new(int32(1)), RetMsg: new("missing packet id connID")}
	}
	return &trdmodifyorderpb.Response{
		RetType: new(int32(0)),
		S2C: &trdmodifyorderpb.S2C{
			Header:    normalizeTestTrdHeader(request.GetC2S().GetHeader()),
			OrderID:   new(request.GetC2S().GetOrderID()),
			OrderIDEx: new(strconv.FormatUint(request.GetC2S().GetOrderID(), 10)),
		},
	}
}

func basicQotListForSecurities(securities []*qotcommonpb.Security) []*qotcommonpb.BasicQot {
	quotes := make([]*qotcommonpb.BasicQot, 0, len(securities))
	baseQuoteTime := time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC)
	for index, security := range securities {
		price := 700.0 + float64(index)
		quotes = append(quotes, &qotcommonpb.BasicQot{
			Security:        security,
			IsSuspended:     new(false),
			ListTime:        new("2020-01-01"),
			PriceSpread:     new(0.01),
			UpdateTime:      new(baseQuoteTime.Format("2006-01-02 15:04:05")),
			HighPrice:       new(price + 1),
			OpenPrice:       new(price - 1),
			LowPrice:        new(price - 2),
			CurPrice:        new(price),
			LastClosePrice:  new(price - 0.5),
			Volume:          new(1000 + int64(index)*10),
			Turnover:        new(price * 1000),
			TurnoverRate:    new(1.25),
			Amplitude:       new(2.5),
			UpdateTimestamp: new(float64(baseQuoteTime.Unix())),
		})
	}
	return quotes
}

func (s *quoteOpenDServer) acceptCount() int {
	return int(s.accepts.Load())
}

func (s *quoteOpenDServer) lastInitRecvNotify() bool {
	return s.initRecvNotify.Load()
}

func (s *quoteOpenDServer) subCallCount() int {
	return int(s.qotSubCalls.Load())
}

func (s *quoteOpenDServer) capturedQotSubRequests() []*qotsubpb.C2S {
	s.qotSubMu.Lock()
	defer s.qotSubMu.Unlock()
	requests := make([]*qotsubpb.C2S, 0, len(s.qotSubRequests))
	for _, request := range s.qotSubRequests {
		requests = append(requests, proto.Clone(request).(*qotsubpb.C2S))
	}
	return requests
}

func (s *quoteOpenDServer) setQotSubResponses(responses ...*qotsubpb.Response) {
	s.qotSubMu.Lock()
	s.qotSubResponses = append([]*qotsubpb.Response(nil), responses...)
	s.qotSubMu.Unlock()
}

func (s *quoteOpenDServer) setSubInfoResponse(response *qotgetsubinfopb.Response) {
	s.qotSubMu.Lock()
	s.subInfoResponse = response
	s.qotSubMu.Unlock()
}

func (s *quoteOpenDServer) accountListCallCount() int {
	return int(s.accountListCalls.Load())
}

func (s *quoteOpenDServer) fundsCallCount() int {
	return int(s.fundsCalls.Load())
}

func (s *quoteOpenDServer) orderListCallCount() int {
	return int(s.orderListCalls.Load())
}

func (s *quoteOpenDServer) placeOrderCallCount() int {
	return int(s.placeOrderCalls.Load())
}

func (s *quoteOpenDServer) modifyOrderCallCount() int {
	return int(s.modifyOrderCalls.Load())
}

func (s *quoteOpenDServer) tradeAccountPushCallCount() int {
	return int(s.tradeAccPushCalls.Load())
}

func (s *quoteOpenDServer) lastPlaceOrderRequest() *trdplaceorderpb.C2S {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	if s.lastPlaceOrder == nil {
		return nil
	}
	return jftradeCheckedTypeAssertion[*trdplaceorderpb.C2S](proto.Clone(s.lastPlaceOrder))
}

func (s *quoteOpenDServer) lastModifyOrderRequest() *trdmodifyorderpb.C2S {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	if s.lastModifyOrder == nil {
		return nil
	}
	return jftradeCheckedTypeAssertion[*trdmodifyorderpb.C2S](proto.Clone(s.lastModifyOrder))
}

func (s *quoteOpenDServer) lastTradeAccountPushIDs() []uint64 {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	return append([]uint64(nil), s.lastTradeAccPushIDs...)
}

func (s *quoteOpenDServer) pushSubCallCount() int {
	return int(s.pushSubCalls.Load())
}

func (s *quoteOpenDServer) basicQotCallCount() int {
	return int(s.basicQotCalls.Load())
}

func (s *quoteOpenDServer) historyKLCallCount() int {
	return int(s.historyKLCalls.Load())
}

func (s *quoteOpenDServer) currentKLCallCount() int {
	return int(s.currentKLCalls.Load())
}

func (s *quoteOpenDServer) lastHistoryExtendedTime() bool {
	return s.historyExtended.Load()
}

func (s *quoteOpenDServer) historySessionCalls() []int32 {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	return append([]int32(nil), s.historySessionCallLog...)
}

func (s *quoteOpenDServer) historyOrderListCallCount() int {
	return int(s.historyOrderListCalls.Load())
}

func (s *quoteOpenDServer) historyOrderFillListCallCount() int {
	return int(s.historyFillListCalls.Load())
}

func (s *quoteOpenDServer) orderFeeCallCount() int {
	return int(s.orderFeeCalls.Load())
}

func (s *quoteOpenDServer) marginRatioCallCount() int {
	return int(s.marginRatioCalls.Load())
}

func (s *quoteOpenDServer) flowSummaryCallCount() int {
	return int(s.flowSummaryCalls.Load())
}

func (s *quoteOpenDServer) maxTrdQtysCallCount() int {
	return int(s.maxTrdQtysCalls.Load())
}

func (s *quoteOpenDServer) lastMaxTrdQtysRequest() *trdgetmaxtrdqtyspb.C2S {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	if s.lastMaxTrdQtys == nil {
		return nil
	}
	return jftradeCheckedTypeAssertion[*trdgetmaxtrdqtyspb.C2S](proto.Clone(s.lastMaxTrdQtys))
}

func normalizeTestFunds(funds *trdcommonpb.Funds) *trdcommonpb.Funds {
	if funds == nil {
		funds = &trdcommonpb.Funds{}
	}
	clone := jftradeCheckedTypeAssertion[*trdcommonpb.Funds](proto.Clone(funds))
	if clone.Power == nil {
		clone.Power = new(float64(0))
	}
	if clone.TotalAssets == nil {
		clone.TotalAssets = new(float64(0))
	}
	if clone.Cash == nil {
		clone.Cash = new(float64(0))
	}
	if clone.MarketVal == nil {
		clone.MarketVal = new(float64(0))
	}
	if clone.FrozenCash == nil {
		clone.FrozenCash = new(float64(0))
	}
	if clone.DebtCash == nil {
		clone.DebtCash = new(float64(0))
	}
	if clone.AvlWithdrawalCash == nil {
		clone.AvlWithdrawalCash = new(float64(0))
	}
	return clone
}

func normalizeTestTrdHeader(header *trdcommonpb.TrdHeader) *trdcommonpb.TrdHeader {
	if header == nil {
		header = &trdcommonpb.TrdHeader{}
	}
	clone := jftradeCheckedTypeAssertion[*trdcommonpb.TrdHeader](proto.Clone(header))
	if clone.TrdEnv == nil {
		clone.TrdEnv = new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate))
	}
	if clone.AccID == nil {
		clone.AccID = new(uint64(1001))
	}
	if clone.TrdMarket == nil {
		clone.TrdMarket = new(int32(trdcommonpb.TrdMarket_TrdMarket_HK))
	}
	return clone
}

func normalizeTestPosition(position *trdcommonpb.Position) *trdcommonpb.Position {
	if position == nil {
		position = &trdcommonpb.Position{}
	}
	clone := jftradeCheckedTypeAssertion[*trdcommonpb.Position](proto.Clone(position))
	if clone.PositionID == nil {
		clone.PositionID = new(uint64(1))
	}
	if clone.PositionSide == nil {
		clone.PositionSide = new(int32(1))
	}
	if clone.Code == nil {
		clone.Code = new("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = new(clone.GetCode())
	}
	if clone.Qty == nil {
		clone.Qty = new(float64(0))
	}
	if clone.CanSellQty == nil {
		clone.CanSellQty = new(float64(0))
	}
	if clone.Price == nil {
		clone.Price = new(float64(0))
	}
	if clone.Val == nil {
		clone.Val = new(float64(0))
	}
	if clone.PlVal == nil {
		clone.PlVal = new(float64(0))
	}
	return clone
}

func normalizeTestOrder(order *trdcommonpb.Order) *trdcommonpb.Order {
	if order == nil {
		order = &trdcommonpb.Order{}
	}
	clone := jftradeCheckedTypeAssertion[*trdcommonpb.Order](proto.Clone(order))
	if clone.TrdSide == nil {
		clone.TrdSide = new(int32(trdcommonpb.TrdSide_TrdSide_Buy))
	}
	if clone.OrderType == nil {
		clone.OrderType = new(int32(trdcommonpb.OrderType_OrderType_Normal))
	}
	if clone.OrderStatus == nil {
		clone.OrderStatus = new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted))
	}
	if clone.OrderID == nil {
		clone.OrderID = new(uint64(1))
	}
	if clone.OrderIDEx == nil {
		clone.OrderIDEx = new(strconv.FormatUint(clone.GetOrderID(), 10))
	}
	if clone.Code == nil {
		clone.Code = new("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = new(clone.GetCode())
	}
	if clone.Qty == nil {
		clone.Qty = new(float64(0))
	}
	if clone.CreateTime == nil {
		clone.CreateTime = new("2026-05-20 09:30:00")
	}
	if clone.UpdateTime == nil {
		clone.UpdateTime = new(clone.GetCreateTime())
	}
	return clone
}

func filterTestOrdersByConditions(orders []*trdcommonpb.Order, filter *trdcommonpb.TrdFilterConditions, statuses []int32) []*trdcommonpb.Order {
	filtered := make([]*trdcommonpb.Order, 0, len(orders))
	for _, order := range orders {
		if order == nil {
			continue
		}
		if filter != nil && len(filter.GetCodeList()) > 0 {
			matchedCode := false
			for _, code := range filter.GetCodeList() {
				if strings.EqualFold(strings.TrimSpace(order.GetCode()), strings.TrimSpace(code)) {
					matchedCode = true
					break
				}
			}
			if !matchedCode {
				continue
			}
		}
		if len(statuses) > 0 {
			matchedStatus := false
			for _, status := range statuses {
				if order.GetOrderStatus() == status {
					matchedStatus = true
					break
				}
			}
			if !matchedStatus {
				continue
			}
		}
		filtered = append(filtered, order)
	}
	return filtered
}

func filterTestFillsByConditions(fills []*trdcommonpb.OrderFill, filter *trdcommonpb.TrdFilterConditions) []*trdcommonpb.OrderFill {
	filtered := make([]*trdcommonpb.OrderFill, 0, len(fills))
	for _, fill := range fills {
		if fill == nil {
			continue
		}
		if filter != nil && len(filter.GetCodeList()) > 0 {
			matchedCode := false
			for _, code := range filter.GetCodeList() {
				if strings.EqualFold(strings.TrimSpace(fill.GetCode()), strings.TrimSpace(code)) {
					matchedCode = true
					break
				}
			}
			if !matchedCode {
				continue
			}
		}
		filtered = append(filtered, fill)
	}
	return filtered
}
