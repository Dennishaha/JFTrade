package futu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
)

type BrokerPlaceOrderResult struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	Order              types.Order
	BrokerOrderIDEx    *string
}

type BrokerPlaceOrderQuery struct {
	BrokerReadQuery
	Session        *string
	FillOutsideRTH *bool
}

func (e *Exchange) submitOrder(ctx context.Context, order types.SubmitOrder) (*types.Order, error) {
	result, err := e.PlaceBrokerOrder(ctx, BrokerPlaceOrderQuery{
		BrokerReadQuery: BrokerReadQuery{Market: marketFromSymbol(order.Symbol, "")},
	}, order)
	if err != nil {
		return nil, err
	}
	return new(result.Order), nil
}

func (e *Exchange) cancelOrders(ctx context.Context, orders ...types.Order) error {
	return e.CancelBrokerOrders(ctx, BrokerReadQuery{}, orders...)
}

func (e *Exchange) PlaceBrokerOrder(ctx context.Context, query BrokerPlaceOrderQuery, submitOrder types.SubmitOrder) (*BrokerPlaceOrderResult, error) {
	if market, err := e.EnsureMarketWithContext(ctx, submitOrder.Symbol); err == nil {
		submitOrder.Market = market
		if err := validateSubmitOrderQuantityAgainstMarket(submitOrder, market); err != nil {
			return nil, err
		}
	}

	var result BrokerPlaceOrderResult
	if err := e.withClient(ctx, func(client *opend.Client) error {
		market := query.Market
		if market == "" {
			market = marketFromSymbol(submitOrder.Symbol, "")
		}
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             market,
		})
		if err != nil {
			return err
		}

		request, err := placeOrderRequestFromSubmitOrder(resolved, submitOrder, query)
		if err != nil {
			return err
		}
		placed, err := client.PlaceOrder(ctx, request)
		if err != nil {
			return err
		}

		result = BrokerPlaceOrderResult{
			AccountID:          resolved.AccountID,
			TradingEnvironment: resolved.TradingEnvironment,
			Market:             resolved.Market,
			Order:              placedOrderFromSubmitOrder(submitOrder, placed.OrderID),
			BrokerOrderIDEx:    optionalNonEmptyString(placed.OrderIDEx),
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &result, nil
}

func validateSubmitOrderQuantityAgainstMarket(order types.SubmitOrder, market types.Market) error {
	if order.Quantity.Sign() <= 0 {
		return fmt.Errorf("futu exchange: order quantity must be positive")
	}
	if market.MinQuantity.Sign() > 0 && order.Quantity.Compare(market.MinQuantity) < 0 {
		return fmt.Errorf("futu exchange: order quantity %s is less than market min quantity %s for %s", order.Quantity.String(), market.MinQuantity.String(), order.Symbol)
	}
	if !market.StepSize.IsZero() {
		normalized := market.TruncateQuantity(order.Quantity)
		if normalized.Compare(order.Quantity) != 0 {
			return fmt.Errorf("futu exchange: order quantity %s does not match market quantity step %s for %s", order.Quantity.String(), market.StepSize.String(), order.Symbol)
		}
	}
	return nil
}

func (e *Exchange) CancelBrokerOrders(ctx context.Context, query BrokerReadQuery, orders ...types.Order) error {
	if len(orders) == 0 {
		return nil
	}
	return e.withClient(ctx, func(client *opend.Client) error {
		for _, order := range orders {
			market := query.Market
			if market == "" {
				market = marketFromSymbol(order.Symbol, "")
			}
			resolved, err := e.resolveTradeAccountWithClient(ctx, client, BrokerReadQuery{
				AccountID:          query.AccountID,
				TradingEnvironment: query.TradingEnvironment,
				Market:             market,
			})
			if err != nil {
				return err
			}
			if order.OrderID == 0 {
				return fmt.Errorf("futu exchange: cancel requires broker order id")
			}
			request := &trdmodifyorderpb.C2S{
				Header:        resolved.header(),
				OrderID:       new(order.OrderID),
				ModifyOrderOp: new(int32(trdcommonpb.ModifyOrderOp_ModifyOrderOp_Cancel)),
			}
			if _, err := client.ModifyOrder(ctx, request); err != nil {
				return err
			}
		}
		return nil
	})
}

func placeOrderRequestFromSubmitOrder(account resolvedTradeAccount, submitOrder types.SubmitOrder, query BrokerPlaceOrderQuery) (*trdplaceorderpb.C2S, error) {
	code, secMarket, err := tradeSecurityInfoFromSymbol(submitOrder.Symbol)
	if err != nil {
		return nil, err
	}
	trdSide, err := trdSideFromBBGOSide(submitOrder.Side)
	if err != nil {
		return nil, err
	}
	orderType, err := trdOrderTypeFromBBGOOrderType(submitOrder.Type)
	if err != nil {
		return nil, err
	}

	request := &trdplaceorderpb.C2S{
		Header:    account.header(),
		TrdSide:   new(int32(trdSide)),
		OrderType: new(int32(orderType)),
		Code:      new(code),
		Qty:       new(submitOrder.Quantity.Float64()),
		SecMarket: new(int32(secMarket)),
	}
	normalizedPrice := normalizeSubmitOrderPrice(submitOrder.Symbol, submitOrder.Price)
	if normalizedPrice.Sign() > 0 {
		request.Price = new(normalizedPrice.Float64())
	}
	normalizedStopPrice := normalizeSubmitOrderPrice(submitOrder.Symbol, submitOrder.StopPrice)
	if normalizedStopPrice.Sign() > 0 {
		request.AuxPrice = new(normalizedStopPrice.Float64())
	}
	if timeInForce, ok := trdTimeInForceFromBBGO(submitOrder.TimeInForce); ok {
		request.TimeInForce = new(int32(timeInForce))
	} else if strings.TrimSpace(string(submitOrder.TimeInForce)) != "" {
		return nil, fmt.Errorf("futu exchange: unsupported timeInForce %q", submitOrder.TimeInForce)
	}
	remark := strings.TrimSpace(submitOrder.ClientOrderID)
	if remark == "" {
		remark = strings.TrimSpace(submitOrder.Tag)
	}
	if remark != "" {
		request.Remark = new(remark)
	}
	if query.Session != nil {
		if secMarket != trdcommonpb.TrdSecMarket_TrdSecMarket_US {
			return nil, fmt.Errorf("futu exchange: session is supported for US orders only")
		}
		session, ok := sessionValue(*query.Session)
		if !ok {
			return nil, fmt.Errorf("futu exchange: unsupported session %q", *query.Session)
		}
		request.Session = new(session)
	}
	if query.FillOutsideRTH != nil {
		if secMarket != trdcommonpb.TrdSecMarket_TrdSecMarket_US {
			return nil, fmt.Errorf("futu exchange: fillOutsideRTH is supported for US orders only")
		}
		if supportsFillOutsideRTH(submitOrder.Type) {
			request.FillOutsideRTH = new(*query.FillOutsideRTH)
		}
	}
	return request, nil
}

func placedOrderFromSubmitOrder(submitOrder types.SubmitOrder, orderID uint64) types.Order {
	now := time.Now().UTC()
	market := submitOrder.Market
	if market.Symbol == "" {
		market = inferMarket(submitOrder.Symbol)
	}
	if market.Exchange == "" {
		market.Exchange = Name
	}
	return types.Order{
		SubmitOrder: types.SubmitOrder{
			ClientOrderID:    submitOrder.ClientOrderID,
			Symbol:           submitOrder.Symbol,
			Side:             submitOrder.Side,
			Type:             submitOrder.Type,
			Price:            submitOrder.Price,
			Quantity:         submitOrder.Quantity,
			AveragePrice:     submitOrder.AveragePrice,
			StopPrice:        submitOrder.StopPrice,
			Market:           market,
			TimeInForce:      submitOrder.TimeInForce,
			GroupID:          submitOrder.GroupID,
			QuoteID:          submitOrder.QuoteID,
			MarginSideEffect: submitOrder.MarginSideEffect,
			ReduceOnly:       submitOrder.ReduceOnly,
			ClosePosition:    submitOrder.ClosePosition,
			Tag:              submitOrder.Tag,
		},
		Exchange:         Name,
		OrderID:          orderID,
		Status:           types.OrderStatusNew,
		OriginalStatus:   "SUBMITTED",
		ExecutedQuantity: fixedpoint.Zero,
		IsWorking:        true,
		CreationTime:     types.Time(now),
		UpdateTime:       types.Time(now),
	}
}

func tradeSecurityInfoFromSymbol(symbol string) (string, trdcommonpb.TrdSecMarket, error) {
	trimmed := strings.TrimSpace(strings.ToUpper(symbol))
	if trimmed == "" {
		return "", 0, fmt.Errorf("futu exchange: symbol is required")
	}
	separator := "."
	if strings.Contains(trimmed, ":") {
		separator = ":"
	}
	parts := strings.SplitN(trimmed, separator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", 0, fmt.Errorf("futu exchange: symbol %q must be in MARKET.CODE form", symbol)
	}
	market, err := trdSecMarketForCode(parts[0])
	if err != nil {
		return "", 0, err
	}
	return parts[1], market, nil
}

func trdSecMarketForCode(market string) (trdcommonpb.TrdSecMarket, error) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "HK":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_HK, nil
	case "US":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_US, nil
	case "SH":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_CN_SH, nil
	case "SZ":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_CN_SZ, nil
	case "SG":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_SG, nil
	case "JP":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_JP, nil
	case "AU":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_AU, nil
	case "MY":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_MY, nil
	case "CA":
		return trdcommonpb.TrdSecMarket_TrdSecMarket_CA, nil
	default:
		return 0, fmt.Errorf("futu exchange: unsupported security market %q", market)
	}
}

func trdSideFromBBGOSide(side types.SideType) (trdcommonpb.TrdSide, error) {
	switch side {
	case types.SideTypeBuy:
		return trdcommonpb.TrdSide_TrdSide_Buy, nil
	case types.SideTypeSell:
		return trdcommonpb.TrdSide_TrdSide_Sell, nil
	default:
		return 0, fmt.Errorf("futu exchange: unsupported side %q", side)
	}
}

func trdOrderTypeFromBBGOOrderType(orderType types.OrderType) (trdcommonpb.OrderType, error) {
	switch orderType {
	case types.OrderTypeLimit, types.OrderTypeLimitMaker:
		return trdcommonpb.OrderType_OrderType_Normal, nil
	case types.OrderTypeMarket:
		return trdcommonpb.OrderType_OrderType_Market, nil
	case types.OrderTypeStopMarket:
		return trdcommonpb.OrderType_OrderType_Stop, nil
	case types.OrderTypeStopLimit:
		return trdcommonpb.OrderType_OrderType_StopLimit, nil
	case types.OrderTypeTakeProfitMarket:
		return trdcommonpb.OrderType_OrderType_MarketifTouched, nil
	case types.OrderTypeTakeProfit:
		return trdcommonpb.OrderType_OrderType_LimitifTouched, nil
	default:
		return 0, fmt.Errorf("futu exchange: unsupported order type %q", orderType)
	}
}

func trdTimeInForceFromBBGO(timeInForce types.TimeInForce) (trdcommonpb.TimeInForce, bool) {
	switch strings.ToUpper(strings.TrimSpace(string(timeInForce))) {
	case "", "GTC":
		return trdcommonpb.TimeInForce_TimeInForce_GTC, true
	case "DAY":
		return trdcommonpb.TimeInForce_TimeInForce_DAY, true
	case "IOC":
		return trdcommonpb.TimeInForce_TimeInForce_IOC, true
	default:
		return 0, false
	}
}

func supportsFillOutsideRTH(orderType types.OrderType) bool {
	switch orderType {
	case types.OrderTypeLimit, types.OrderTypeLimitMaker, types.OrderTypeStopLimit:
		return true
	default:
		return false
	}
}
