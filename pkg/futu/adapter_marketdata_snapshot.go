package futu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func (r *futuMarketDataReader) QuerySecuritySnapshot(ctx context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	if len(query.Symbols) == 0 {
		return nil, fmt.Errorf("futu: QuerySecuritySnapshot requires at least one symbol")
	}
	snapshotsBySymbol, err := r.exchange.querySecuritySnapshotList(ctx, query.Symbols)
	if err != nil {
		if errors.Is(err, errNoSecuritySnapshots) {
			return &broker.SecuritySnapshotResult{AccountID: query.AccountID}, nil
		}
		return nil, err
	}
	// querySecuritySnapshotList canonicalizes the same symbol set before it
	// reaches OpenD, so this second pass cannot fail after a successful read.
	canonical, _ := canonicalSecuritySnapshotSymbols(query.Symbols)
	snapshots := make([]*qotgetsecuritysnapshotpb.Snapshot, 0, len(canonical))
	for _, symbol := range canonical {
		if snapshot := snapshotsBySymbol[symbol]; snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
	}
	return securitySnapshotResultFromProtoList(query.AccountID, snapshots, time.Now().UTC()), nil
}

func securitySnapshotResultFromProtoList(accountID string, snapshots []*qotgetsecuritysnapshotpb.Snapshot, observedAt time.Time) *broker.SecuritySnapshotResult {
	result := &broker.SecuritySnapshotResult{AccountID: accountID}
	for _, snapshot := range snapshots {
		item, ok := securitySnapshotItemFromProto(snapshot, observedAt)
		if !ok {
			continue
		}
		result.Snapshots = append(result.Snapshots, item)
	}
	return result
}

func securitySnapshotItemFromProto(snap *qotgetsecuritysnapshotpb.Snapshot, observedAt time.Time) (broker.SecuritySnapshotItem, bool) {
	if snap == nil || snap.Basic == nil {
		return broker.SecuritySnapshotItem{}, false
	}
	basic := snap.Basic
	item := broker.SecuritySnapshotItem{
		Symbol:       securitySymbol(basic.GetSecurity()),
		Name:         cloneStringPtr(basic.Name),
		SecurityType: new(enumName(basic.GetType(), qotcommonpb.SecurityType_name)),
		ProductClass: productClassFromSecurityType(basic.GetType()),
		IsSuspended:  cloneBoolPtr(basic.IsSuspend),
		LastPrice:    cloneFloat64Ptr(basic.CurPrice),
		BidPrice:     cloneFloat64Ptr(basic.BidPrice),
		AskPrice:     cloneFloat64Ptr(basic.AskPrice),
		// Keep OpenD's raw LastClosePrice here. Watchlist regular-session
		// change is measured against that value even while the US market is closed.
		PreviousClose: cloneFloat64Ptr(basic.LastClosePrice),
		OpenPrice:     cloneFloat64Ptr(basic.OpenPrice),
		HighPrice:     cloneFloat64Ptr(basic.HighPrice),
		LowPrice:      cloneFloat64Ptr(basic.LowPrice),
		Volume:        int64AsFloat64Ptr(basic.Volume),
		Turnover:      cloneFloat64Ptr(basic.Turnover),
		LotSize:       cloneInt32Ptr(basic.LotSize),
		UpdateTime:    cloneStringPtr(basic.UpdateTime),
		ObservedAt:    observedAt,
		PreMarket:     extendedSessionSnapshotFromProto(basic.GetPreMarket()),
		AfterMarket:   extendedSessionSnapshotFromProto(basic.GetAfterMarket()),
		Overnight:     extendedSessionSnapshotFromProto(basic.GetOvernight()),
	}
	item.MarketSegment = marketSegmentFromProductClass(item.ProductClass)
	preQuote := extendedMarketQuoteFromProto(basic.GetPreMarket())
	afterQuote := extendedMarketQuoteFromProto(basic.GetAfterMarket())
	overnightQuote := extendedMarketQuoteFromProto(basic.GetOvernight())
	session := sessionFromExtendedBlocksAt(item.Symbol, preQuote, afterQuote, overnightQuote, observedAt)
	if session != market.SessionUnknown {
		item.Session = new(string(session))
	}
	applySecuritySnapshotExtensions(&item, snap)
	return item, true
}

func applySecuritySnapshotExtensions(
	item *broker.SecuritySnapshotItem,
	snap *qotgetsecuritysnapshotpb.Snapshot,
) {
	if snap.EquityExData != nil {
		item.PERate = cloneFloat64Ptr(snap.EquityExData.PeRate)
		item.PBRate = cloneFloat64Ptr(snap.EquityExData.PbRate)
	}
	applyOptionSnapshotData(item, snap.GetOptionExData())
	applyWarrantSnapshotData(item, snap.GetWarrantExData())
	applyFutureSnapshotData(item, snap.GetFutureExData())
	applyFundSnapshotData(item, snap.GetTrustExData())
}

func applyOptionSnapshotData(item *broker.SecuritySnapshotItem, option *qotgetsecuritysnapshotpb.OptionSnapshotExData) {
	if option == nil {
		return
	}
	contractSize := float64(option.GetContractSize())
	if option.ContractSizeFloat != nil {
		contractSize = option.GetContractSizeFloat()
	}
	item.Option = &broker.OptionSnapshotData{
		OptionType:           enumName(option.GetType(), qotcommonpb.OptionType_name),
		UnderlyingCode:       securitySymbol(option.GetOwner()),
		ExpiryDate:           option.GetStrikeTime(),
		StrikePrice:          option.GetStrikePrice(),
		ContractSize:         contractSize,
		ContractMultiplier:   cloneFloat64Ptr(option.ContractMultiplier),
		OpenInterest:         option.GetOpenInterest(),
		NetOpenInterest:      cloneInt32Ptr(option.NetOpenInterest),
		ImpliedVolatility:    option.GetImpliedVolatility(),
		Premium:              option.GetPremium(),
		Delta:                option.GetDelta(),
		Gamma:                option.GetGamma(),
		Vega:                 option.GetVega(),
		Theta:                option.GetTheta(),
		Rho:                  option.GetRho(),
		DaysToExpiry:         cloneInt32Ptr(option.ExpiryDateDistance),
		ContractNominalValue: cloneFloat64Ptr(option.ContractNominalValue),
	}
	item.ProductClass = broker.ProductClassOption
	item.MarketSegment = broker.MarketSegmentDerivatives
}

func applyWarrantSnapshotData(item *broker.SecuritySnapshotItem, warrant *qotgetsecuritysnapshotpb.WarrantSnapshotExData) {
	if warrant == nil {
		return
	}
	item.Warrant = &broker.WarrantSnapshotData{
		WarrantType:        enumName(warrant.GetWarrantType(), qotcommonpb.WarrantType_name),
		UnderlyingCode:     securitySymbol(warrant.GetOwner()),
		IssuerCode:         cloneStringPtr(warrant.IssuerCode),
		MaturityDate:       warrant.GetMaturityTime(),
		LastTradeDate:      warrant.GetEndTradeTime(),
		StrikePrice:        warrant.GetStrikePrice(),
		RecoveryPrice:      warrant.GetRecoveryPrice(),
		ConversionRate:     warrant.GetConversionRate(),
		StreetVolume:       warrant.GetStreetVolumn(),
		IssueVolume:        warrant.GetIssueVolumn(),
		StreetRate:         warrant.GetStreetRate(),
		ImpliedVolatility:  warrant.GetImpliedVolatility(),
		Premium:            warrant.GetPremium(),
		Delta:              warrant.GetDelta(),
		Leverage:           cloneFloat64Ptr(warrant.Leverage),
		BreakEvenPoint:     cloneFloat64Ptr(warrant.BreakEvenPoint),
		PriceRecoveryRatio: cloneFloat64Ptr(warrant.PriceRecoveryRatio),
	}
	item.ProductClass = broker.ProductClassWarrant
	if warrant.GetWarrantType() == int32(qotcommonpb.WarrantType_WarrantType_Bull) ||
		warrant.GetWarrantType() == int32(qotcommonpb.WarrantType_WarrantType_Bear) {
		item.ProductClass = broker.ProductClassCBBC
	}
	item.MarketSegment = broker.MarketSegmentDerivatives
}

func applyFutureSnapshotData(item *broker.SecuritySnapshotItem, future *qotgetsecuritysnapshotpb.FutureSnapshotExData) {
	if future == nil {
		return
	}
	item.Future = &broker.FutureSnapshotData{
		LastSettlementPrice: future.GetLastSettlePrice(),
		OpenInterest:        future.GetPosition(),
		OpenInterestChange:  future.GetPositionChange(),
		LastTradeDate:       future.GetLastTradeTime(),
		LastTradeTimestamp:  cloneFloat64Ptr(future.LastTradeTimestamp),
		IsMainContract:      future.GetIsMainContract(),
	}
	item.ProductClass = broker.ProductClassFuture
	item.MarketSegment = broker.MarketSegmentDerivatives
}

func applyFundSnapshotData(item *broker.SecuritySnapshotItem, fund *qotgetsecuritysnapshotpb.TrustSnapshotExData) {
	if fund == nil {
		return
	}
	item.Fund = &broker.FundSnapshotData{
		DividendYield:         fund.GetDividendYield(),
		AssetsUnderManagement: fund.GetAum(),
		OutstandingUnits:      fund.GetOutstandingUnits(),
		NetAssetValue:         fund.GetNetAssetValue(),
		Premium:               fund.GetPremium(),
		AssetClass:            enumName(fund.GetAssetClass(), qotcommonpb.AssetClass_name),
	}
	item.ProductClass = broker.ProductClassFund
	item.MarketSegment = broker.MarketSegmentSecurities
}

func productClassFromSecurityType(value int32) broker.ProductClass {
	switch qotcommonpb.SecurityType(value) {
	case qotcommonpb.SecurityType_SecurityType_Bond:
		return broker.ProductClassBond
	case qotcommonpb.SecurityType_SecurityType_Eqty:
		return broker.ProductClassEquity
	case qotcommonpb.SecurityType_SecurityType_Trust:
		return broker.ProductClassFund
	case qotcommonpb.SecurityType_SecurityType_Warrant:
		return broker.ProductClassWarrant
	case qotcommonpb.SecurityType_SecurityType_Index:
		return broker.ProductClassIndex
	case qotcommonpb.SecurityType_SecurityType_Plate,
		qotcommonpb.SecurityType_SecurityType_PlateSet:
		return broker.ProductClassPlate
	case qotcommonpb.SecurityType_SecurityType_Drvt:
		return broker.ProductClassOption
	case qotcommonpb.SecurityType_SecurityType_Future:
		return broker.ProductClassFuture
	default:
		return broker.ProductClassUnknown
	}
}

func marketSegmentFromProductClass(productClass broker.ProductClass) broker.MarketSegment {
	switch productClass {
	case broker.ProductClassOption,
		broker.ProductClassWarrant,
		broker.ProductClassCBBC,
		broker.ProductClassFuture:
		return broker.MarketSegmentDerivatives
	case broker.ProductClassEventContract:
		return broker.MarketSegmentPrediction
	default:
		return broker.MarketSegmentSecurities
	}
}

func extendedSessionSnapshotFromProto(data *qotcommonpb.PreAfterMarketData) *broker.ExtendedSessionSnapshot {
	if data == nil {
		return nil
	}
	return &broker.ExtendedSessionSnapshot{
		Price:      cloneFloat64Ptr(data.Price),
		HighPrice:  cloneFloat64Ptr(data.HighPrice),
		LowPrice:   cloneFloat64Ptr(data.LowPrice),
		Volume:     int64AsFloat64Ptr(data.Volume),
		Turnover:   cloneFloat64Ptr(data.Turnover),
		Change:     cloneFloat64Ptr(data.ChangeVal),
		ChangeRate: cloneFloat64Ptr(data.ChangeRate),
		Amplitude:  cloneFloat64Ptr(data.Amplitude),
	}
}

func (r *futuMarketDataReader) QueryOrderBook(ctx context.Context, query broker.OrderBookQuery) (*broker.OrderBookSnapshot, error) {
	if query.Symbol == "" {
		return nil, fmt.Errorf("futu: QueryOrderBook requires a symbol")
	}
	num := query.Num
	if num <= 0 {
		num = 10 // default depth levels
	}
	var result *broker.OrderBookSnapshot
	if err := r.exchange.withRetryingClient(ctx, func(client *opend.Client) error {
		res, err := r.exchange.QueryOrderBook(ctx, query.Symbol, num)
		if err != nil {
			return err
		}
		snapshot := orderBookSnapshotFromOpendResult(res, &query)
		result = snapshot
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}
