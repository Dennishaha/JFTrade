package futu

import (
	"strconv"
	"strings"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func brokerFundsSnapshotFromProto(account resolvedTradeAccount, funds *trdcommonpb.Funds) *BrokerFundsSnapshot {
	if funds == nil {
		funds = &trdcommonpb.Funds{}
	}

	snapshot := &BrokerFundsSnapshot{
		AccountID:               account.AccountID,
		TradingEnvironment:      account.TradingEnvironment,
		Market:                  account.Market,
		AccountType:             account.AccountType,
		Currency:                optionalEnumStringPtr(funds.Currency, trdcommonpb.Currency_name),
		TotalAssets:             cloneFloat64Ptr(funds.TotalAssets),
		SecuritiesAssets:        cloneFloat64Ptr(funds.SecuritiesAssets),
		FundAssets:              cloneFloat64Ptr(funds.FundAssets),
		BondAssets:              cloneFloat64Ptr(funds.BondAssets),
		Cash:                    cloneFloat64Ptr(funds.Cash),
		MarketValue:             cloneFloat64Ptr(funds.MarketVal),
		LongMarketValue:         cloneFloat64Ptr(funds.LongMv),
		ShortMarketValue:        cloneFloat64Ptr(funds.ShortMv),
		PurchasingPower:         cloneFloat64Ptr(funds.Power),
		ShortSellingPower:       cloneFloat64Ptr(funds.MaxPowerShort),
		NetCashPower:            cloneFloat64Ptr(funds.NetCashPower),
		AvailableWithdrawalCash: cloneFloat64Ptr(funds.AvlWithdrawalCash),
		MaxWithdrawal:           cloneFloat64Ptr(funds.MaxWithdrawal),
		AvailableFunds:          cloneFloat64Ptr(funds.AvailableFunds),
		FrozenCash:              cloneFloat64Ptr(funds.FrozenCash),
		PendingAsset:            cloneFloat64Ptr(funds.PendingAsset),
		UnrealizedPnl:           cloneFloat64Ptr(funds.UnrealizedPL),
		RealizedPnl:             cloneFloat64Ptr(funds.RealizedPL),
		InitialMargin:           cloneFloat64Ptr(funds.InitialMargin),
		MaintenanceMargin:       cloneFloat64Ptr(funds.MaintenanceMargin),
		MarginCallMargin:        cloneFloat64Ptr(funds.MarginCallMargin),
		RiskStatus:              optionalEnumStringPtr(funds.RiskStatus, trdcommonpb.CltRiskStatus_name),
		// Margin & Financing 融资融券
		DebtCash:       cloneFloat64Ptr(funds.DebtCash),
		IsPDT:          cloneBoolPtr(funds.IsPdt),
		PDTSeq:         optionalNonEmptyString(funds.GetPdtSeq()),
		BeginningDTBP:  cloneFloat64Ptr(funds.BeginningDTBP),
		RemainingDTBP:  cloneFloat64Ptr(funds.RemainingDTBP),
		DTCallAmount:   cloneFloat64Ptr(funds.DtCallAmount),
		DTStatus:       optionalEnumStringPtr(funds.DtStatus, trdcommonpb.DTStatus_name),
		ExposureLevel:  optionalEnumStringPtr(funds.ExposureLevel, trdcommonpb.ExposureLevel_name),
		ExposureLimit:  cloneFloat64Ptr(funds.ExposureLimit),
		UsedLimit:      cloneFloat64Ptr(funds.UsedLimit),
		RemainingLimit: cloneFloat64Ptr(funds.RemainingLimit),
	}

	snapshot.CurrencyBalances = make([]BrokerCurrencyBalanceSnapshot, 0, len(funds.GetCashInfoList()))
	for _, cashInfo := range funds.GetCashInfoList() {
		if cashInfo == nil {
			continue
		}
		currency := optionalEnumStringPtr(cashInfo.Currency, trdcommonpb.Currency_name)
		if currency == nil {
			continue
		}
		snapshot.CurrencyBalances = append(snapshot.CurrencyBalances, BrokerCurrencyBalanceSnapshot{
			AccountID:               account.AccountID,
			TradingEnvironment:      account.TradingEnvironment,
			Currency:                *currency,
			Cash:                    cloneFloat64Ptr(cashInfo.Cash),
			AvailableWithdrawalCash: cloneFloat64Ptr(cashInfo.AvailableBalance),
			NetCashPower:            cloneFloat64Ptr(cashInfo.NetCashPower),
		})
	}

	snapshot.MarketAssets = make([]BrokerMarketAssetSnapshot, 0, len(funds.GetMarketInfoList()))
	for _, marketInfo := range funds.GetMarketInfoList() {
		if marketInfo == nil {
			continue
		}
		market := runtimeMarketAuthority(marketInfo.GetTrdMarket())
		if market == "" {
			continue
		}
		snapshot.MarketAssets = append(snapshot.MarketAssets, BrokerMarketAssetSnapshot{
			AccountID:          account.AccountID,
			TradingEnvironment: account.TradingEnvironment,
			Market:             market,
			Assets:             cloneFloat64Ptr(marketInfo.Assets),
		})
	}

	return snapshot
}

func brokerPositionSnapshotFromProto(account resolvedTradeAccount, position *trdcommonpb.Position) BrokerPositionSnapshot {
	market := runtimeMarketAuthority(position.GetTrdMarket())
	if market == "" {
		market = marketFromSymbol(position.GetCode(), account.Market)
	}

	return BrokerPositionSnapshot{
		AccountID:          account.AccountID,
		TradingEnvironment: account.TradingEnvironment,
		Market:             market,
		Symbol:             strings.TrimSpace(strings.ToUpper(position.GetCode())),
		SymbolName:         optionalNonEmptyString(position.GetName()),
		Quantity:           position.GetQty(),
		SellableQuantity:   position.GetCanSellQty(),
		LastPrice:          position.GetPrice(),
		CostPrice:          preferredFloat64Ptr(position.DilutedCostPrice, position.CostPrice),
		AverageCostPrice:   cloneFloat64Ptr(position.AverageCostPrice),
		MarketValue:        position.GetVal(),
		UnrealizedPnl:      preferredFloat64Ptr(position.UnrealizedPL, position.PlVal),
		RealizedPnl:        cloneFloat64Ptr(position.RealizedPL),
		PnlRatio:           preferredFloat64Ptr(position.AveragePlRatio, position.PlRatio),
		Currency:           optionalEnumStringPtr(position.Currency, trdcommonpb.Currency_name),
	}
}

func brokerOrderSnapshotFromProto(account resolvedTradeAccount, order *trdcommonpb.Order) BrokerOrderSnapshot {
	market := resolveBrokerOrderMarket(order.GetTrdMarket(), order.GetCode(), account.Market)
	timeSymbol := brokerOrderTimeSymbol(market, order.GetCode())

	return BrokerOrderSnapshot{
		AccountID:          account.AccountID,
		TradingEnvironment: account.TradingEnvironment,
		Market:             market,
		BrokerOrderID:      strconv.FormatUint(order.GetOrderID(), 10),
		BrokerOrderIDEx:    optionalNonEmptyString(order.GetOrderIDEx()),
		Symbol:             strings.TrimSpace(strings.ToUpper(order.GetCode())),
		SymbolName:         optionalNonEmptyString(order.GetName()),
		Side:               normalizeRuntimeEnum(enumName(order.GetTrdSide(), trdcommonpb.TrdSide_name)),
		OrderType:          normalizeRuntimeEnum(enumName(order.GetOrderType(), trdcommonpb.OrderType_name)),
		Status:             normalizeRuntimeEnum(enumName(order.GetOrderStatus(), trdcommonpb.OrderStatus_name)),
		Quantity:           order.GetQty(),
		FilledQuantity:     cloneFloat64Ptr(order.FillQty),
		Price:              cloneFloat64Ptr(order.Price),
		FilledAveragePrice: cloneFloat64Ptr(order.FillAvgPrice),
		SubmittedAt:        formatBrokerOrderTime(order.CreateTimestamp, order.GetCreateTime(), timeSymbol),
		UpdatedAt:          formatBrokerOrderTime(order.UpdateTimestamp, order.GetUpdateTime(), timeSymbol),
		Remark:             optionalNonEmptyString(order.GetRemark()),
		LastError:          optionalNonEmptyString(order.GetLastErrMsg()),
		TimeInForce:        optionalEnumStringPtr(order.TimeInForce, trdcommonpb.TimeInForce_name),
		Currency:           optionalEnumStringPtr(order.Currency, trdcommonpb.Currency_name),
	}
}

func brokerOrderFillSnapshotFromProto(account resolvedTradeAccount, fill *trdcommonpb.OrderFill) BrokerOrderFillSnapshot {
	market := resolveBrokerOrderMarket(fill.GetTrdMarket(), fill.GetCode(), account.Market)
	timeSymbol := brokerOrderTimeSymbol(market, fill.GetCode())

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
		FilledAt:           formatBrokerOrderTime(fill.CreateTimestamp, fill.GetCreateTime(), timeSymbol),
		Status:             optionalEnumStringPtr(fill.Status, trdcommonpb.OrderFillStatus_name),
	}
}

func brokerOrderFeeSnapshotFromProto(account resolvedTradeAccount, fee *trdcommonpb.OrderFee) BrokerOrderFeeSnapshot {
	snapshot := BrokerOrderFeeSnapshot{
		AccountID:          account.AccountID,
		TradingEnvironment: account.TradingEnvironment,
		Market:             account.Market,
		BrokerOrderIDEx:    strings.TrimSpace(fee.GetOrderIDEx()),
		FeeAmount:          cloneFloat64Ptr(fee.FeeAmount),
		FeeItems:           make([]BrokerOrderFeeItemSnapshot, 0, len(fee.GetFeeList())),
	}
	for _, item := range fee.GetFeeList() {
		if item == nil {
			continue
		}
		snapshot.FeeItems = append(snapshot.FeeItems, BrokerOrderFeeItemSnapshot{
			Title: strings.TrimSpace(item.GetTitle()),
			Value: item.GetValue(),
		})
	}
	return snapshot
}

func brokerMarginRatioSnapshotFromProto(account resolvedTradeAccount, info *trdgetmarginratiopb.MarginRatioInfo) BrokerMarginRatioSnapshot {
	symbol, err := futuSymbolFromSecurity(info.GetSecurity())
	if err != nil {
		symbol = ""
	}
	market := marketFromSymbol(symbol, account.Market)
	return BrokerMarginRatioSnapshot{
		AccountID:               account.AccountID,
		TradingEnvironment:      "REAL",
		Market:                  market,
		Symbol:                  symbol,
		IsLongPermit:            cloneBoolPtr(info.IsLongPermit),
		IsShortPermit:           cloneBoolPtr(info.IsShortPermit),
		ShortPoolRemain:         cloneFloat64Ptr(info.ShortPoolRemain),
		ShortFeeRate:            cloneFloat64Ptr(info.ShortFeeRate),
		AlertLongRatio:          cloneFloat64Ptr(info.AlertLongRatio),
		AlertShortRatio:         cloneFloat64Ptr(info.AlertShortRatio),
		InitialMarginLongRatio:  cloneFloat64Ptr(info.ImLongRatio),
		InitialMarginShortRatio: cloneFloat64Ptr(info.ImShortRatio),
		MarginCallLongRatio:     cloneFloat64Ptr(info.McmLongRatio),
		MarginCallShortRatio:    cloneFloat64Ptr(info.McmShortRatio),
		MaintenanceLongRatio:    cloneFloat64Ptr(info.MmLongRatio),
		MaintenanceShortRatio:   cloneFloat64Ptr(info.MmShortRatio),
	}
}

func brokerCashFlowSnapshotFromProto(account resolvedTradeAccount, flow *trdflowsummarypb.FlowSummaryInfo) BrokerCashFlowSnapshot {
	return BrokerCashFlowSnapshot{
		AccountID:          account.AccountID,
		TradingEnvironment: account.TradingEnvironment,
		Market:             account.Market,
		CashFlowID:         optionalUint64StringPtr(flow.CashFlowID),
		ClearingDate:       optionalNonEmptyString(flow.GetClearingDate()),
		SettlementDate:     optionalNonEmptyString(flow.GetSettlementDate()),
		Currency:           optionalEnumStringPtr(flow.Currency, trdcommonpb.Currency_name),
		CashFlowType:       optionalNonEmptyString(flow.GetCashFlowType()),
		CashFlowDirection:  optionalEnumStringPtr(flow.CashFlowDirection, trdflowsummarypb.TrdCashFlowDirection_name),
		CashFlowAmount:     cloneFloat64Ptr(flow.CashFlowAmount),
		CashFlowRemark:     optionalNonEmptyString(flow.GetCashFlowRemark()),
	}
}

func brokerMaxTradeQuantitySnapshotFromProto(account resolvedTradeAccount, symbol string, orderType string, price float64, maxQtys *trdcommonpb.MaxTrdQtys) *BrokerMaxTradeQuantitySnapshot {
	if maxQtys == nil {
		maxQtys = &trdcommonpb.MaxTrdQtys{}
	}
	return &BrokerMaxTradeQuantitySnapshot{
		AccountID:           account.AccountID,
		TradingEnvironment:  account.TradingEnvironment,
		Market:              account.Market,
		Symbol:              symbol,
		OrderType:           orderType,
		Price:               price,
		MaxCashBuy:          maxQtys.GetMaxCashBuy(),
		MaxCashAndMarginBuy: cloneFloat64Ptr(maxQtys.MaxCashAndMarginBuy),
		MaxPositionSell:     maxQtys.GetMaxPositionSell(),
		MaxSellShort:        cloneFloat64Ptr(maxQtys.MaxSellShort),
		MaxBuyBack:          cloneFloat64Ptr(maxQtys.MaxBuyBack),
		LongRequiredIM:      cloneFloat64Ptr(maxQtys.LongRequiredIM),
		ShortRequiredIM:     cloneFloat64Ptr(maxQtys.ShortRequiredIM),
		Session:             optionalEnumStringPtr(maxQtys.Session, commonpb.Session_name),
	}
}
