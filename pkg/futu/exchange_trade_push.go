package futu

import (
	"strconv"
	"strings"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

type BrokerOrderFillSnapshot struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	BrokerOrderID      string
	BrokerOrderIDEx    *string
	BrokerFillID       string
	BrokerFillIDEx     *string
	Symbol             string
	SymbolName         *string
	Side               string
	FilledQuantity     float64
	FillPrice          *float64
	FilledAt           string
	Status             *string
}

func BrokerOrderSnapshotFromPush(header *trdcommonpb.TrdHeader, order *trdcommonpb.Order) BrokerOrderSnapshot {
	account := resolvedTradeAccountFromHeader(header, marketFromSymbol(order.GetCode(), ""))
	return brokerOrderSnapshotFromProto(account, order)
}

func BrokerOrderFillSnapshotFromPush(header *trdcommonpb.TrdHeader, fill *trdcommonpb.OrderFill) BrokerOrderFillSnapshot {
	account := resolvedTradeAccountFromHeader(header, marketFromSymbol(fill.GetCode(), ""))
	market := runtimeMarketAuthority(fill.GetTrdMarket())
	if market == "" {
		market = account.Market
	}
	return BrokerOrderFillSnapshot{
		AccountID:          account.AccountID,
		TradingEnvironment: account.TradingEnvironment,
		Market:             market,
		BrokerOrderID:      strconv.FormatUint(fill.GetOrderID(), 10),
		BrokerOrderIDEx:    optionalNonEmptyString(fill.GetOrderIDEx()),
		BrokerFillID:       strconv.FormatUint(fill.GetFillID(), 10),
		BrokerFillIDEx:     optionalNonEmptyString(fill.GetFillIDEx()),
		Symbol:             strings.TrimSpace(strings.ToUpper(fill.GetCode())),
		SymbolName:         optionalNonEmptyString(fill.GetName()),
		Side:               normalizeRuntimeEnum(enumName(fill.GetTrdSide(), trdcommonpb.TrdSide_name)),
		FilledQuantity:     fill.GetQty(),
		FillPrice:          cloneFloat64Ptr(fill.Price),
		FilledAt:           formatBrokerOrderTime(fill.CreateTimestamp, fill.GetCreateTime()),
		Status:             optionalEnumStringPtr(fill.Status, trdcommonpb.OrderFillStatus_name),
	}
}

func resolvedTradeAccountFromHeader(header *trdcommonpb.TrdHeader, fallbackMarket string) resolvedTradeAccount {
	if header == nil {
		return resolvedTradeAccount{Market: fallbackMarket}
	}
	market := runtimeMarketAuthority(header.GetTrdMarket())
	if market == "" {
		market = fallbackMarket
	}
	return resolvedTradeAccount{
		AccountID:          strconv.FormatUint(header.GetAccID(), 10),
		TradingEnvironment: runtimeTradingEnvironment(header.GetTrdEnv()),
		Market:             market,
		protoAccountID:     header.GetAccID(),
		protoTrdEnv:        header.GetTrdEnv(),
		protoTrdMarket:     header.GetTrdMarket(),
	}
}
