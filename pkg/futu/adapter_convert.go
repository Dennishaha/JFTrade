package futu

import (
	"strconv"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

// --- Futu → broker type converters ---

func convertFundsSnapshot(s *BrokerFundsSnapshot) *broker.FundsSnapshot {
	if s == nil {
		return nil
	}
	result := &broker.FundsSnapshot{
		AccountID:               s.AccountID,
		TradingEnvironment:      s.TradingEnvironment,
		Market:                  s.Market,
		AccountType:             s.AccountType,
		Currency:                s.Currency,
		TotalAssets:             s.TotalAssets,
		SecuritiesAssets:        s.SecuritiesAssets,
		FundAssets:              s.FundAssets,
		BondAssets:              s.BondAssets,
		Cash:                    s.Cash,
		MarketValue:             s.MarketValue,
		LongMarketValue:         s.LongMarketValue,
		ShortMarketValue:        s.ShortMarketValue,
		PurchasingPower:         s.PurchasingPower,
		ShortSellingPower:       s.ShortSellingPower,
		NetCashPower:            s.NetCashPower,
		AvailableWithdrawalCash: s.AvailableWithdrawalCash,
		MaxWithdrawal:           s.MaxWithdrawal,
		AvailableFunds:          s.AvailableFunds,
		FrozenCash:              s.FrozenCash,
		PendingAsset:            s.PendingAsset,
		UnrealizedPnl:           s.UnrealizedPnl,
		RealizedPnl:             s.RealizedPnl,
		InitialMargin:           s.InitialMargin,
		MaintenanceMargin:       s.MaintenanceMargin,
		MarginCallMargin:        s.MarginCallMargin,
		RiskStatus:              s.RiskStatus,
		// Margin & Financing 融资融券
		DebtCash:       s.DebtCash,
		IsPDT:          s.IsPDT,
		PDTSeq:         s.PDTSeq,
		BeginningDTBP:  s.BeginningDTBP,
		RemainingDTBP:  s.RemainingDTBP,
		DTCallAmount:   s.DTCallAmount,
		DTStatus:       s.DTStatus,
		ExposureLevel:  s.ExposureLevel,
		ExposureLimit:  s.ExposureLimit,
		UsedLimit:      s.UsedLimit,
		RemainingLimit: s.RemainingLimit,
	}
	if len(s.CurrencyBalances) > 0 {
		result.CurrencyBalances = make([]broker.CurrencyBalanceSnapshot, len(s.CurrencyBalances))
		for i, cb := range s.CurrencyBalances {
			result.CurrencyBalances[i] = broker.CurrencyBalanceSnapshot{
				AccountID:               cb.AccountID,
				TradingEnvironment:      cb.TradingEnvironment,
				Currency:                cb.Currency,
				Cash:                    cb.Cash,
				AvailableWithdrawalCash: cb.AvailableWithdrawalCash,
				NetCashPower:            cb.NetCashPower,
			}
		}
	}
	if len(s.MarketAssets) > 0 {
		result.MarketAssets = make([]broker.MarketAssetSnapshot, len(s.MarketAssets))
		for i, ma := range s.MarketAssets {
			result.MarketAssets[i] = broker.MarketAssetSnapshot{
				AccountID:          ma.AccountID,
				TradingEnvironment: ma.TradingEnvironment,
				Market:             ma.Market,
				Assets:             ma.Assets,
			}
		}
	}
	return result
}

func convertPositionSnapshot(s BrokerPositionSnapshot) broker.PositionSnapshot {
	return broker.PositionSnapshot{
		AccountID:          s.AccountID,
		TradingEnvironment: s.TradingEnvironment,
		Market:             s.Market,
		Symbol:             s.Symbol,
		SymbolName:         s.SymbolName,
		Quantity:           s.Quantity,
		SellableQuantity:   s.SellableQuantity,
		LastPrice:          s.LastPrice,
		CostPrice:          s.CostPrice,
		AverageCostPrice:   s.AverageCostPrice,
		MarketValue:        s.MarketValue,
		UnrealizedPnl:      s.UnrealizedPnl,
		RealizedPnl:        s.RealizedPnl,
		PnlRatio:           s.PnlRatio,
		Currency:           s.Currency,
	}
}

func convertOrderSnapshot(s BrokerOrderSnapshot) broker.OrderSnapshot {
	return broker.OrderSnapshot{
		AccountID:          s.AccountID,
		TradingEnvironment: s.TradingEnvironment,
		Market:             s.Market,
		BrokerOrderID:      s.BrokerOrderID,
		BrokerOrderIDEx:    s.BrokerOrderIDEx,
		Symbol:             s.Symbol,
		SymbolName:         s.SymbolName,
		Side:               s.Side,
		OrderType:          s.OrderType,
		Status:             s.Status,
		Quantity:           s.Quantity,
		FilledQuantity:     s.FilledQuantity,
		Price:              s.Price,
		FilledAveragePrice: s.FilledAveragePrice,
		SubmittedAt:        s.SubmittedAt,
		UpdatedAt:          s.UpdatedAt,
		Remark:             s.Remark,
		LastError:          s.LastError,
		TimeInForce:        s.TimeInForce,
		Currency:           s.Currency,
	}
}

func convertOrderFillSnapshot(s BrokerOrderFillSnapshot) broker.OrderFillSnapshot {
	return broker.OrderFillSnapshot{
		AccountID:          s.AccountID,
		TradingEnvironment: s.TradingEnvironment,
		Market:             s.Market,
		BrokerOrderID:      s.BrokerOrderID,
		BrokerOrderIDEx:    s.BrokerOrderIDEx,
		BrokerFillID:       s.BrokerFillID,
		BrokerFillIDEx:     s.BrokerFillIDEx,
		Symbol:             s.Symbol,
		SymbolName:         s.SymbolName,
		Side:               s.Side,
		FilledQuantity:     s.FilledQuantity,
		FillPrice:          s.FillPrice,
		FilledAt:           s.FilledAt,
		Status:             s.Status,
	}
}

func convertOrderFeeSnapshot(s BrokerOrderFeeSnapshot) broker.OrderFeeSnapshot {
	result := broker.OrderFeeSnapshot{
		AccountID:          s.AccountID,
		TradingEnvironment: s.TradingEnvironment,
		Market:             s.Market,
		BrokerOrderIDEx:    s.BrokerOrderIDEx,
		FeeAmount:          s.FeeAmount,
	}
	if len(s.FeeItems) > 0 {
		result.FeeItems = make([]broker.OrderFeeItemSnapshot, len(s.FeeItems))
		for i, fi := range s.FeeItems {
			result.FeeItems[i] = broker.OrderFeeItemSnapshot{
				Title: fi.Title,
				Value: fi.Value,
			}
		}
	}
	return result
}

func convertMarginRatioSnapshot(s BrokerMarginRatioSnapshot) broker.MarginRatioSnapshot {
	return broker.MarginRatioSnapshot{
		AccountID:               s.AccountID,
		TradingEnvironment:      s.TradingEnvironment,
		Market:                  s.Market,
		Symbol:                  s.Symbol,
		IsLongPermit:            s.IsLongPermit,
		IsShortPermit:           s.IsShortPermit,
		ShortPoolRemain:         s.ShortPoolRemain,
		ShortFeeRate:            s.ShortFeeRate,
		AlertLongRatio:          s.AlertLongRatio,
		AlertShortRatio:         s.AlertShortRatio,
		InitialMarginLongRatio:  s.InitialMarginLongRatio,
		InitialMarginShortRatio: s.InitialMarginShortRatio,
		MarginCallLongRatio:     s.MarginCallLongRatio,
		MarginCallShortRatio:    s.MarginCallShortRatio,
		MaintenanceLongRatio:    s.MaintenanceLongRatio,
		MaintenanceShortRatio:   s.MaintenanceShortRatio,
	}
}

func convertCashFlowSnapshot(s BrokerCashFlowSnapshot) broker.CashFlowSnapshot {
	return broker.CashFlowSnapshot{
		AccountID:          s.AccountID,
		TradingEnvironment: s.TradingEnvironment,
		Market:             s.Market,
		CashFlowID:         s.CashFlowID,
		ClearingDate:       s.ClearingDate,
		SettlementDate:     s.SettlementDate,
		Currency:           s.Currency,
		CashFlowType:       s.CashFlowType,
		CashFlowDirection:  s.CashFlowDirection,
		CashFlowAmount:     s.CashFlowAmount,
		CashFlowRemark:     s.CashFlowRemark,
	}
}

func convertMaxTradeQuantitySnapshot(s *BrokerMaxTradeQuantitySnapshot) broker.MaxTradeQuantitySnapshot {
	if s == nil {
		return broker.MaxTradeQuantitySnapshot{}
	}
	return broker.MaxTradeQuantitySnapshot{
		AccountID:           s.AccountID,
		TradingEnvironment:  s.TradingEnvironment,
		Market:              s.Market,
		Symbol:              s.Symbol,
		OrderType:           s.OrderType,
		Price:               s.Price,
		MaxCashBuy:          s.MaxCashBuy,
		MaxCashAndMarginBuy: s.MaxCashAndMarginBuy,
		MaxPositionSell:     s.MaxPositionSell,
		MaxSellShort:        s.MaxSellShort,
		MaxBuyBack:          s.MaxBuyBack,
		LongRequiredIM:      s.LongRequiredIM,
		ShortRequiredIM:     s.ShortRequiredIM,
		Session:             s.Session,
	}
}

// --- broker → Futu type converters (for trading) ---

func bbgoSubmitOrderFromBrokerPlaceOrder(q broker.PlaceOrderQuery) bbgotypes.SubmitOrder {
	submit := bbgotypes.SubmitOrder{
		Symbol:   q.Symbol,
		Side:     bbgotypes.SideType(q.Side),
		Type:     bbgotypes.OrderType(q.OrderType),
		Quantity: fixedpoint.NewFromFloat(q.Quantity),
	}
	if q.Price != nil {
		submit.Price = fixedpoint.NewFromFloat(*q.Price)
	}
	if q.TimeInForce != nil {
		submit.TimeInForce = bbgotypes.TimeInForce(*q.TimeInForce)
	}
	if q.ClientOrderID != "" {
		submit.ClientOrderID = q.ClientOrderID
	}
	return submit
}

func formatOrderID(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// formatBrokerOrderID converts a uint64 order ID to string.
// Kept as a separate function for clarity in the adapter layer.
func formatBrokerOrderIDPtr(id uint64) *string {
	s := formatOrderID(id)
	return &s
}

// convertFutuAccountsToBroker converts Futu RuntimeAccount slice to broker Account slice.
func convertFutuAccountsToBroker(accounts []RuntimeAccount) []broker.Account {
	result := make([]broker.Account, len(accounts))
	for i, a := range accounts {
		result[i] = broker.Account{
			ID:                   a.AccountID,
			BrokerID:             string(Name),
			TradingEnvironment:   a.TradingEnvironment,
			AccountType:          a.AccountType,
			AccountRole:          a.AccountRole,
			SecurityFirm:         a.SecurityFirm,
			MarketAuthorities:    a.MarketAuthorities,
			SimulatedAccountType: a.SimulatedAccountType,
		}
	}
	return result
}

// brokerReadQueryFromFutu converts a broker ReadQuery to a Futu BrokerReadQuery.
func brokerReadQueryFromFutu(q broker.ReadQuery) BrokerReadQuery {
	return BrokerReadQuery{
		AccountID:          q.AccountID,
		TradingEnvironment: q.TradingEnvironment,
		Market:             q.Market,
	}
}

// orderBookSnapshotFromOpendResult converts an opend.OrderBookResult into a broker.OrderBookSnapshot.
func orderBookSnapshotFromOpendResult(res *opend.OrderBookResult, query *broker.OrderBookQuery) *broker.OrderBookSnapshot {
	if res == nil {
		return nil
	}
	snapshot := &broker.OrderBookSnapshot{
		AccountID: query.AccountID,
		Symbol:    query.Symbol,
	}
	if sym, err := futuSymbolFromSecurity(res.Security); err == nil && sym != "" {
		snapshot.Symbol = sym
	}
	if res.Name != "" {
		snapshot.Name = &res.Name
	}
	if res.SvrRecvTimeBid != "" {
		snapshot.SvrRecvTimeBid = &res.SvrRecvTimeBid
	}
	if res.SvrRecvTimeAsk != "" {
		snapshot.SvrRecvTimeAsk = &res.SvrRecvTimeAsk
	}
	for _, level := range res.BidList {
		snapshot.Bids = append(snapshot.Bids, orderBookLevelFromPb(level))
	}
	for _, level := range res.AskList {
		snapshot.Asks = append(snapshot.Asks, orderBookLevelFromPb(level))
	}
	return snapshot
}

// orderBookLevelFromPb converts a protobuf OrderBook to a broker.OrderBookLevel.
func orderBookLevelFromPb(pb *qotcommonpb.OrderBook) broker.OrderBookLevel {
	if pb == nil {
		return broker.OrderBookLevel{}
	}
	level := broker.OrderBookLevel{
		Price:      pb.GetPrice(),
		Volume:     float64(pb.GetVolume()),
		OrderCount: pb.GetOrederCount(),
	}
	for _, detail := range pb.GetDetailList() {
		level.DetailList = append(level.DetailList, broker.OrderBookDetailItem{
			OrderID: detail.GetOrderID(),
			Volume:  float64(detail.GetVolume()),
		})
	}
	return level
}

// Verify interface compliance at compile time.
var (
	_ broker.Broker           = (*futuAdapter)(nil)
	_ broker.TradingService   = (*futuTradingService)(nil)
	_ broker.MarketDataReader = (*futuMarketDataReader)(nil)
)
