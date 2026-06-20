package futu

import (
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func balanceMapFromBrokerFunds(snapshot *BrokerFundsSnapshot) types.BalanceMap {
	balances := types.BalanceMap{}
	if snapshot == nil {
		return balances
	}
	for _, balance := range snapshot.CurrencyBalances {
		balances[balance.Currency] = types.Balance{
			Currency:          balance.Currency,
			Available:         fixedpointFromPtr(balance.AvailableWithdrawalCash, balance.Cash),
			Locked:            fixedpointFromPtr(nil, nil),
			NetAsset:          fixedpointFromPtr(balance.Cash, nil),
			MaxWithdrawAmount: fixedpointFromPtr(balance.AvailableWithdrawalCash, nil),
		}
	}
	if len(balances) > 0 {
		return balances
	}

	currency := defaultCurrencyForMarket(snapshot.Market)
	if snapshot.Currency != nil && *snapshot.Currency != "" {
		currency = *snapshot.Currency
	}
	if currency == "" {
		currency = "HKD"
	}
	balances[currency] = types.Balance{
		Currency:          currency,
		Available:         fixedpointFromPtr(snapshot.AvailableWithdrawalCash, snapshot.Cash),
		Locked:            fixedpointFromDifference(snapshot.Cash, snapshot.AvailableWithdrawalCash, snapshot.FrozenCash),
		NetAsset:          fixedpointFromPtr(snapshot.Cash, nil),
		MaxWithdrawAmount: fixedpointFromPtr(snapshot.MaxWithdrawal, snapshot.AvailableWithdrawalCash),
	}
	return balances
}

func balanceMapFromFunds(funds *trdcommonpb.Funds, market string) types.BalanceMap {
	return balanceMapFromBrokerFunds(brokerFundsSnapshotFromProto(resolvedTradeAccount{Market: market}, funds))
}

func bbgoAccountTypeFromRuntimeAccountType(accountType string) types.AccountType {
	switch strings.ToUpper(strings.TrimSpace(accountType)) {
	case "MARGIN":
		return types.AccountTypeMargin
	case "DERIVATIVES":
		return types.AccountTypeFutures
	default:
		return types.AccountTypeSpot
	}
}

func bbgoOrderFromBrokerOrder(order BrokerOrderSnapshot) types.Order {
	createdAt := parseBrokerOrderTime(order.SubmittedAt)
	updatedAt := parseBrokerOrderTime(order.UpdatedAt)
	market := inferMarket(order.Symbol)

	return types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:      order.Symbol,
			Side:        bbgoSideFromBrokerOrderSide(order.Side),
			Type:        bbgoOrderTypeFromBrokerOrderType(order.OrderType),
			Price:       fixedpoint.NewFromFloat(optionalFloat64Value(order.Price)),
			Quantity:    fixedpoint.NewFromFloat(order.Quantity),
			TimeInForce: bbgoTimeInForceFromBrokerOrder(order.TimeInForce),
			Market:      market,
		},
		Exchange:         Name,
		OrderID:          parseUint64(order.BrokerOrderID),
		Status:           bbgoOrderStatusFromBrokerOrderStatus(order.Status),
		OriginalStatus:   order.Status,
		ExecutedQuantity: fixedpoint.NewFromFloat(optionalFloat64Value(order.FilledQuantity)),
		IsWorking:        !bbgoOrderStatusFromBrokerOrderStatus(order.Status).Closed(),
		CreationTime:     types.Time(createdAt),
		UpdateTime:       types.Time(updatedAt),
	}
}

func bbgoSideFromBrokerOrderSide(side string) types.SideType {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "SELL", "SELLSHORT":
		return types.SideTypeSell
	default:
		return types.SideTypeBuy
	}
}

func bbgoOrderTypeFromBrokerOrderType(orderType string) types.OrderType {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "MARKET", "TWAP_MARKET", "VWAP_MARKET":
		return types.OrderTypeMarket
	case "STOP", "TRAILINGSTOP":
		return types.OrderTypeStopMarket
	case "STOPLIMIT", "TRAILINGSTOPLIMIT":
		return types.OrderTypeStopLimit
	case "MARKETIFTOUCHED":
		return types.OrderTypeTakeProfitMarket
	case "LIMITIFTOUCHED":
		return types.OrderTypeTakeProfit
	default:
		return types.OrderTypeLimit
	}
}

func bbgoTimeInForceFromBrokerOrder(timeInForce *string) types.TimeInForce {
	if timeInForce == nil {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(*timeInForce)) {
	case "IOC":
		return types.TimeInForceIOC
	case "FOK":
		return types.TimeInForceFOK
	case "GTT":
		return types.TimeInForceGTT
	case "GTC":
		return types.TimeInForceGTC
	default:
		return ""
	}
}

func bbgoOrderStatusFromBrokerOrderStatus(status string) types.OrderStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FILLED_ALL":
		return types.OrderStatusFilled
	case "FILLED_PART", "CANCELLING_PART", "CANCELLED_PART":
		return types.OrderStatusPartiallyFilled
	case "CANCELLED_ALL":
		return types.OrderStatusCanceled
	case "SUBMITFAILED", "FAILED", "DISABLED", "DELETED", "FILLCANCELLED":
		return types.OrderStatusRejected
	case "TIMEOUT":
		return types.OrderStatusNew
	default:
		return types.OrderStatusNew
	}
}

func brokerOrderIsWorking(status int32) bool {
	switch trdcommonpb.OrderStatus(status) {
	case trdcommonpb.OrderStatus_OrderStatus_Filled_All,
		trdcommonpb.OrderStatus_OrderStatus_Cancelled_Part,
		trdcommonpb.OrderStatus_OrderStatus_Cancelled_All,
		trdcommonpb.OrderStatus_OrderStatus_SubmitFailed,
		trdcommonpb.OrderStatus_OrderStatus_Failed,
		trdcommonpb.OrderStatus_OrderStatus_Disabled,
		trdcommonpb.OrderStatus_OrderStatus_Deleted,
		trdcommonpb.OrderStatus_OrderStatus_FillCancelled:
		return false
	default:
		return true
	}
}

func brokerOrderSortKey(order BrokerOrderSnapshot) time.Time {
	updatedAt := parseBrokerOrderTime(order.UpdatedAt)
	if !updatedAt.IsZero() {
		return updatedAt
	}
	return parseBrokerOrderTime(order.SubmittedAt)
}

func brokerOrderFillSortKey(fill BrokerOrderFillSnapshot) time.Time {
	return parseBrokerOrderTime(fill.FilledAt)
}

func parseBrokerOrderTime(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed
	}
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func formatBrokerOrderTime(timestamp *float64, fallback string) string {
	if timestamp != nil && *timestamp > 0 {
		seconds := int64(*timestamp)
		nanos := int64((*timestamp - float64(seconds)) * float64(time.Second))
		return time.Unix(seconds, nanos).UTC().Format(time.RFC3339Nano)
	}
	parsed := parseBrokerOrderTime(fallback)
	if parsed.IsZero() {
		return strings.TrimSpace(fallback)
	}
	return parsed.Format(time.RFC3339Nano)
}

func marketFromSymbol(symbol string, fallback string) string {
	trimmed := strings.TrimSpace(strings.ToUpper(symbol))
	if strings.HasPrefix(trimmed, "HK.") {
		return "HK"
	}
	if strings.HasPrefix(trimmed, "US.") {
		return "US"
	}
	if strings.HasPrefix(trimmed, "SH.") || strings.HasPrefix(trimmed, "SZ.") {
		return "CN"
	}
	if strings.HasPrefix(trimmed, "CN.") {
		return "CN"
	}
	if strings.HasPrefix(trimmed, "SG.") {
		return "SG"
	}
	if strings.HasPrefix(trimmed, "JP.") {
		return "JP"
	}
	if strings.HasPrefix(trimmed, "AU.") {
		return "AU"
	}
	if strings.HasPrefix(trimmed, "MY.") {
		return "MY"
	}
	if strings.HasPrefix(trimmed, "CA.") {
		return "CA"
	}
	if fallback != "" {
		return fallback
	}
	return "HK"
}

func defaultCurrencyForMarket(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "US":
		return "USD"
	case "CN":
		return "CNY"
	case "SG":
		return "SGD"
	case "JP":
		return "JPY"
	case "CA":
		return "CAD"
	case "AU":
		return "AUD"
	default:
		return "HKD"
	}
}
