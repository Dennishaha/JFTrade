package jftradeapi

import (
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func normalizeBrokerRouteHeader(header *trdcommonpb.TrdHeader) *trdcommonpb.TrdHeader {
	if header == nil {
		header = &trdcommonpb.TrdHeader{}
	}
	clone := proto.Clone(header).(*trdcommonpb.TrdHeader)
	if clone.TrdEnv == nil {
		clone.TrdEnv = proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate))
	}
	if clone.AccID == nil {
		clone.AccID = proto.Uint64(1001)
	}
	if clone.TrdMarket == nil {
		clone.TrdMarket = proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK))
	}
	return clone
}

func normalizeBrokerRouteFunds(funds *trdcommonpb.Funds) *trdcommonpb.Funds {
	if funds == nil {
		funds = &trdcommonpb.Funds{}
	}
	clone := proto.Clone(funds).(*trdcommonpb.Funds)
	if clone.Power == nil {
		clone.Power = proto.Float64(0)
	}
	if clone.TotalAssets == nil {
		clone.TotalAssets = proto.Float64(0)
	}
	if clone.Cash == nil {
		clone.Cash = proto.Float64(0)
	}
	if clone.MarketVal == nil {
		clone.MarketVal = proto.Float64(0)
	}
	if clone.FrozenCash == nil {
		clone.FrozenCash = proto.Float64(0)
	}
	if clone.DebtCash == nil {
		clone.DebtCash = proto.Float64(0)
	}
	if clone.AvlWithdrawalCash == nil {
		clone.AvlWithdrawalCash = proto.Float64(0)
	}
	return clone
}

func normalizeBrokerRoutePosition(position *trdcommonpb.Position) *trdcommonpb.Position {
	if position == nil {
		position = &trdcommonpb.Position{}
	}
	clone := proto.Clone(position).(*trdcommonpb.Position)
	if clone.PositionID == nil {
		clone.PositionID = proto.Uint64(1)
	}
	if clone.PositionSide == nil {
		clone.PositionSide = proto.Int32(1)
	}
	if clone.Code == nil {
		clone.Code = proto.String("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = proto.String(clone.GetCode())
	}
	if clone.Qty == nil {
		clone.Qty = proto.Float64(0)
	}
	if clone.CanSellQty == nil {
		clone.CanSellQty = proto.Float64(0)
	}
	if clone.Price == nil {
		clone.Price = proto.Float64(0)
	}
	if clone.Val == nil {
		clone.Val = proto.Float64(0)
	}
	if clone.PlVal == nil {
		clone.PlVal = proto.Float64(0)
	}
	return clone
}

func normalizeBrokerRouteOrder(order *trdcommonpb.Order) *trdcommonpb.Order {
	if order == nil {
		order = &trdcommonpb.Order{}
	}
	clone := proto.Clone(order).(*trdcommonpb.Order)
	if clone.TrdSide == nil {
		clone.TrdSide = proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy))
	}
	if clone.OrderType == nil {
		clone.OrderType = proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal))
	}
	if clone.OrderStatus == nil {
		clone.OrderStatus = proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted))
	}
	if clone.OrderID == nil {
		clone.OrderID = proto.Uint64(1)
	}
	if clone.OrderIDEx == nil {
		clone.OrderIDEx = proto.String(strconv.FormatUint(clone.GetOrderID(), 10))
	}
	if clone.Code == nil {
		clone.Code = proto.String("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = proto.String(clone.GetCode())
	}
	if clone.Qty == nil {
		clone.Qty = proto.Float64(0)
	}
	if clone.CreateTime == nil {
		clone.CreateTime = proto.String("2026-05-20 09:30:00")
	}
	if clone.UpdateTime == nil {
		clone.UpdateTime = proto.String(clone.GetCreateTime())
	}
	return clone
}

func normalizeBrokerRouteOrderFill(fill *trdcommonpb.OrderFill) *trdcommonpb.OrderFill {
	if fill == nil {
		fill = &trdcommonpb.OrderFill{}
	}
	clone := proto.Clone(fill).(*trdcommonpb.OrderFill)
	if clone.OrderID == nil {
		clone.OrderID = proto.Uint64(1)
	}
	if clone.OrderIDEx == nil {
		clone.OrderIDEx = proto.String(strconv.FormatUint(clone.GetOrderID(), 10))
	}
	if clone.FillID == nil {
		clone.FillID = proto.Uint64(1)
	}
	if clone.FillIDEx == nil {
		clone.FillIDEx = proto.String(strconv.FormatUint(clone.GetFillID(), 10))
	}
	if clone.Code == nil {
		clone.Code = proto.String("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = proto.String(clone.GetCode())
	}
	if clone.TrdSide == nil {
		clone.TrdSide = proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy))
	}
	if clone.Qty == nil {
		clone.Qty = proto.Float64(0)
	}
	if clone.CreateTime == nil {
		clone.CreateTime = proto.String("2026-05-20 09:30:00")
	}
	return clone
}

func normalizeBrokerRouteOrderFee(fee *trdcommonpb.OrderFee) *trdcommonpb.OrderFee {
	if fee == nil {
		fee = &trdcommonpb.OrderFee{}
	}
	clone := proto.Clone(fee).(*trdcommonpb.OrderFee)
	if clone.OrderIDEx == nil {
		clone.OrderIDEx = proto.String("")
	}
	if clone.FeeAmount == nil {
		clone.FeeAmount = proto.Float64(0)
	}
	return clone
}

func normalizeBrokerRouteMarginRatio(ratio *trdgetmarginratiopb.MarginRatioInfo) *trdgetmarginratiopb.MarginRatioInfo {
	if ratio == nil {
		ratio = &trdgetmarginratiopb.MarginRatioInfo{}
	}
	clone := proto.Clone(ratio).(*trdgetmarginratiopb.MarginRatioInfo)
	if clone.Security == nil {
		clone.Security = &qotcommonpb.Security{Market: proto.Int32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: proto.String("00700")}
	}
	return clone
}

func normalizeBrokerRouteCashFlow(flow *trdflowsummarypb.FlowSummaryInfo) *trdflowsummarypb.FlowSummaryInfo {
	if flow == nil {
		flow = &trdflowsummarypb.FlowSummaryInfo{}
	}
	clone := proto.Clone(flow).(*trdflowsummarypb.FlowSummaryInfo)
	if clone.CashFlowID == nil {
		clone.CashFlowID = proto.Uint64(1)
	}
	if clone.ClearingDate == nil {
		clone.ClearingDate = proto.String("2026-05-20")
	}
	return clone
}

func normalizeBrokerRouteMaxTrdQtys(maxQtys *trdcommonpb.MaxTrdQtys) *trdcommonpb.MaxTrdQtys {
	if maxQtys == nil {
		maxQtys = &trdcommonpb.MaxTrdQtys{}
	}
	return proto.Clone(maxQtys).(*trdcommonpb.MaxTrdQtys)
}

func filterBrokerRouteOrders(input []*trdcommonpb.Order, filter *trdcommonpb.TrdFilterConditions, statuses []int32) []*trdcommonpb.Order {
	orders := make([]*trdcommonpb.Order, 0, len(input))
	codeSet := make(map[string]struct{}, len(filter.GetCodeList()))
	for _, code := range filter.GetCodeList() {
		codeSet[strings.ToUpper(strings.TrimSpace(code))] = struct{}{}
	}
	statusSet := make(map[int32]struct{}, len(statuses))
	for _, status := range statuses {
		statusSet[status] = struct{}{}
	}
	for _, order := range input {
		normalized := normalizeBrokerRouteOrder(order)
		if len(codeSet) > 0 {
			if _, ok := codeSet[strings.ToUpper(strings.TrimSpace(normalized.GetCode()))]; !ok {
				continue
			}
		}
		if len(statusSet) > 0 {
			if _, ok := statusSet[normalized.GetOrderStatus()]; !ok {
				continue
			}
		}
		orders = append(orders, normalized)
	}
	return orders
}

func filterBrokerRouteFills(input []*trdcommonpb.OrderFill, filter *trdcommonpb.TrdFilterConditions) []*trdcommonpb.OrderFill {
	fills := make([]*trdcommonpb.OrderFill, 0, len(input))
	codeSet := make(map[string]struct{}, len(filter.GetCodeList()))
	for _, code := range filter.GetCodeList() {
		codeSet[strings.ToUpper(strings.TrimSpace(code))] = struct{}{}
	}
	for _, fill := range input {
		normalized := normalizeBrokerRouteOrderFill(fill)
		if len(codeSet) > 0 {
			if _, ok := codeSet[strings.ToUpper(strings.TrimSpace(normalized.GetCode()))]; !ok {
				continue
			}
		}
		fills = append(fills, normalized)
	}
	return fills
}
