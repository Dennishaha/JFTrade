package futu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
)

type SecurityRef struct {
	InstrumentID string
	Market       string
	Symbol       string
}

type EquitySecurityDetails struct {
	IssuedShares         int64
	IssuedMarketValue    decimal.Decimal
	NetAsset             decimal.Decimal
	NetProfit            decimal.Decimal
	EarningsPerShare     decimal.Decimal
	OutstandingShares    int64
	OutstandingMarketVal decimal.Decimal
	NetAssetPerShare     decimal.Decimal
	EarningsYieldRate    decimal.Decimal
	PERate               decimal.Decimal
	PBRate               decimal.Decimal
	PETTMRate            decimal.Decimal
	DividendTTM          *decimal.Decimal
	DividendRatioTTM     *decimal.Decimal
	DividendLFY          *decimal.Decimal
	DividendLFYRatio     *decimal.Decimal
}

type WarrantSecurityDetails struct {
	ConversionRate     decimal.Decimal
	WarrantType        string
	StrikePrice        decimal.Decimal
	MaturityTime       string
	EndTradeTime       string
	Owner              *SecurityRef
	RecoveryPrice      decimal.Decimal
	StreetVolume       int64
	IssueVolume        int64
	StreetRate         decimal.Decimal
	Delta              decimal.Decimal
	ImpliedVolatility  decimal.Decimal
	Premium            decimal.Decimal
	MaturityTimestamp  *float64
	EndTradeTimestamp  *float64
	Leverage           *decimal.Decimal
	InOutPriceRatio    *decimal.Decimal
	BreakEvenPoint     *decimal.Decimal
	ConversionPrice    *decimal.Decimal
	PriceRecoveryRatio *decimal.Decimal
	Score              *decimal.Decimal
	UpperStrikePrice   *decimal.Decimal
	LowerStrikePrice   *decimal.Decimal
	InLinePriceStatus  string
	IssuerCode         *string
}

type OptionSecurityDetails struct {
	OptionType           string
	Owner                *SecurityRef
	StrikeTime           string
	StrikePrice          decimal.Decimal
	ContractSize         int32
	ContractSizeFloat    *decimal.Decimal
	OpenInterest         int32
	ImpliedVolatility    decimal.Decimal
	Premium              decimal.Decimal
	Delta                decimal.Decimal
	Gamma                decimal.Decimal
	Vega                 decimal.Decimal
	Theta                decimal.Decimal
	Rho                  decimal.Decimal
	StrikeTimestamp      *float64
	IndexOptionType      string
	NetOpenInterest      *int32
	ExpiryDateDistance   *int32
	ContractNominalValue *decimal.Decimal
	OwnerLotMultiplier   *decimal.Decimal
	OptionAreaType       string
	ContractMultiplier   *decimal.Decimal
}

type IndexSecurityDetails struct {
	RaiseCount int32
	FallCount  int32
	EqualCount int32
}

type PlateSecurityDetails struct {
	RaiseCount int32
	FallCount  int32
	EqualCount int32
}

type FutureSecurityDetails struct {
	LastSettlePrice    decimal.Decimal
	Position           int32
	PositionChange     int32
	LastTradeTime      string
	LastTradeTimestamp *float64
	IsMainContract     bool
}

type TrustSecurityDetails struct {
	DividendYield   decimal.Decimal
	AUM             decimal.Decimal
	OutstandingUnit int64
	NetAssetValue   decimal.Decimal
	Premium         decimal.Decimal
	AssetClass      string
}

type SecurityDetails struct {
	InstrumentID        string
	Market              string
	Symbol              string
	SecurityID          *int64
	Name                string
	SecurityType        string
	ExchangeType        string
	ListTime            string
	ListTimestamp       *float64
	Delisting           *bool
	LotSize             int32
	IsSuspend           bool
	PriceSpread         decimal.Decimal
	UpdateTime          string
	UpdateTimestamp     *float64
	HighPrice           decimal.Decimal
	OpenPrice           decimal.Decimal
	LowPrice            decimal.Decimal
	LastClosePrice      decimal.Decimal
	CurrentPrice        decimal.Decimal
	Volume              int64
	Turnover            decimal.Decimal
	TurnoverRate        decimal.Decimal
	AskPrice            *decimal.Decimal
	BidPrice            *decimal.Decimal
	AskVolume           *int64
	BidVolume           *int64
	Amplitude           *decimal.Decimal
	AveragePrice        *decimal.Decimal
	BidAskRatio         *decimal.Decimal
	VolumeRatio         *decimal.Decimal
	Highest52WeeksPrice *decimal.Decimal
	Lowest52WeeksPrice  *decimal.Decimal
	HighestHistoryPrice *decimal.Decimal
	LowestHistoryPrice  *decimal.Decimal
	SessionStatus       string
	ClosePrice5Minute   *decimal.Decimal
	HighPrecisionVolume *decimal.Decimal
	HighPrecisionAskVol *decimal.Decimal
	HighPrecisionBidVol *decimal.Decimal
	PreMarket           *ExtendedMarketQuote
	AfterMarket         *ExtendedMarketQuote
	Overnight           *ExtendedMarketQuote
	Equity              *EquitySecurityDetails
	Warrant             *WarrantSecurityDetails
	Option              *OptionSecurityDetails
	Index               *IndexSecurityDetails
	Plate               *PlateSecurityDetails
	Future              *FutureSecurityDetails
	Trust               *TrustSecurityDetails
}

// QuerySecuritySnapshot returns a broker-agnostic rich security snapshot.
// It is backed by Futu OpenD 3203 and is optionally enriched with 3007 static
// fields when available.
func (e *Exchange) QuerySecuritySnapshot(ctx context.Context, symbol string) (*SecurityDetails, error) {
	snapshot, err := e.querySecuritySnapshot(ctx, symbol)
	if err != nil {
		return nil, err
	}
	canonical, _ := futuSymbolFromSecurity(snapshot.GetBasic().GetSecurity())
	details := securityDetailsFromSnapshot(snapshot, canonical)
	staticInfo, err := e.queryStaticInfo(ctx, symbol)
	if err == nil {
		mergeStaticInfoIntoSecurityDetails(details, staticInfo)
	}
	return details, nil

}

// QuerySecurityDetails is an alias kept for project-side broker abstractions.
func (e *Exchange) QuerySecurityDetails(ctx context.Context, symbol string) (*SecurityDetails, error) {
	return e.QuerySecuritySnapshot(ctx, symbol)
}

func (e *Exchange) querySecuritySnapshot(ctx context.Context, symbol string) (*qotgetsecuritysnapshotpb.Snapshot, error) {
	snapshots, err := e.querySecuritySnapshotList(ctx, []string{symbol})
	if err != nil {
		return nil, err
	}
	canonical := strings.TrimSpace(strings.ToUpper(symbol))
	snapshot := snapshots[canonical]
	if snapshot == nil {
		return nil, fmt.Errorf("opend GetSecuritySnapshot returned no snapshot for %s", symbol)
	}
	return snapshot, nil
}

func (e *Exchange) querySecuritySnapshotList(ctx context.Context, symbols []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
	securityList := make([]*qotcommonpb.Security, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		security, canonical, err := futuSecurityFromSymbol(symbol)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		securityList = append(securityList, security)
	}
	if len(securityList) == 0 {
		return map[string]*qotgetsecuritysnapshotpb.Snapshot{}, nil
	}

	request := &qotgetsecuritysnapshotpb.Request{C2S: &qotgetsecuritysnapshotpb.C2S{SecurityList: securityList}}
	var response qotgetsecuritysnapshotpb.Response
	if err := e.callProto(ctx, opend.ProtoGetSecuritySnapshot, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend GetSecuritySnapshot retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}

	snapshots := make(map[string]*qotgetsecuritysnapshotpb.Snapshot, len(response.GetS2C().GetSnapshotList()))
	for _, snapshot := range response.GetS2C().GetSnapshotList() {
		canonical, err := futuSymbolFromSecurity(snapshot.GetBasic().GetSecurity())
		if err != nil {
			continue
		}
		snapshots[canonical] = snapshot
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("opend GetSecuritySnapshot returned no snapshots")
	}
	return snapshots, nil
}

func (e *Exchange) queryStaticInfo(ctx context.Context, symbol string) (*qotcommonpb.SecurityStaticInfo, error) {
	infos, err := e.queryStaticInfoList(ctx, []string{symbol})
	if err != nil {
		return nil, err
	}
	canonical := strings.TrimSpace(strings.ToUpper(symbol))
	info := infos[canonical]
	if info == nil {
		return nil, fmt.Errorf("opend GetStaticInfo returned no static info for %s", symbol)
	}
	return info, nil
}

func (e *Exchange) queryStaticInfoList(ctx context.Context, symbols []string) (map[string]*qotcommonpb.SecurityStaticInfo, error) {
	securityList := make([]*qotcommonpb.Security, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		security, canonical, err := futuSecurityFromSymbol(symbol)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		securityList = append(securityList, security)
	}
	if len(securityList) == 0 {
		return map[string]*qotcommonpb.SecurityStaticInfo{}, nil
	}

	request := &qotgetstaticinfopb.Request{C2S: &qotgetstaticinfopb.C2S{SecurityList: securityList}}
	var response qotgetstaticinfopb.Response
	if err := e.callProto(ctx, opend.ProtoGetStaticInfo, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend GetStaticInfo retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}

	infos := make(map[string]*qotcommonpb.SecurityStaticInfo, len(response.GetS2C().GetStaticInfoList()))
	for _, info := range response.GetS2C().GetStaticInfoList() {
		canonical, err := futuSymbolFromSecurity(info.GetBasic().GetSecurity())
		if err != nil {
			continue
		}
		infos[canonical] = info
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("opend GetStaticInfo returned no static info")
	}
	return infos, nil
}

//nolint:funlen
func securityDetailsFromSnapshot(snapshot *qotgetsecuritysnapshotpb.Snapshot, canonical string) *SecurityDetails {
	if snapshot == nil || snapshot.GetBasic() == nil {
		return nil
	}
	details := baseSecurityDetailsFromSnapshot(snapshot.GetBasic(), canonical)
	applyEquitySnapshotDetails(details, snapshot.GetEquityExData())
	applyWarrantSnapshotDetails(details, snapshot.GetWarrantExData())
	applyOptionSnapshotDetails(details, snapshot.GetOptionExData())
	applyIndexSnapshotDetails(details, snapshot.GetIndexExData())
	applyPlateSnapshotDetails(details, snapshot.GetPlateExData())
	applyFutureSnapshotDetails(details, snapshot.GetFutureExData())
	applyTrustSnapshotDetails(details, snapshot.GetTrustExData())
	return details
}

func baseSecurityDetailsFromSnapshot(basic *qotgetsecuritysnapshotpb.SnapshotBasicData, canonical string) *SecurityDetails {
	ref := securityRefFromProto(basic.GetSecurity())
	if ref == nil {
		ref = securityRefFromCanonical(canonical)
	}
	quoteTime := futuQuoteTime(basic.GetUpdateTimestamp(), basic.GetUpdateTime(), canonical).Format(time.RFC3339Nano)
	return &SecurityDetails{
		InstrumentID:        ref.InstrumentID,
		Market:              ref.Market,
		Symbol:              ref.Symbol,
		Name:                basic.GetName(),
		SecurityType:        enumName(basic.GetType(), qotcommonpb.SecurityType_name),
		ListTime:            basic.GetListTime(),
		ListTimestamp:       cloneFloat64(basic.ListTimestamp),
		LotSize:             basic.GetLotSize(),
		IsSuspend:           basic.GetIsSuspend(),
		PriceSpread:         decimalFromFloat64(basic.GetPriceSpread()),
		UpdateTime:          basic.GetUpdateTime(),
		UpdateTimestamp:     cloneFloat64(basic.UpdateTimestamp),
		HighPrice:           decimalFromFloat64(basic.GetHighPrice()),
		OpenPrice:           decimalFromFloat64(basic.GetOpenPrice()),
		LowPrice:            decimalFromFloat64(basic.GetLowPrice()),
		LastClosePrice:      decimalFromFloat64(basic.GetLastClosePrice()),
		CurrentPrice:        decimalFromFloat64(basic.GetCurPrice()),
		Volume:              basic.GetVolume(),
		Turnover:            decimalFromFloat64(basic.GetTurnover()),
		TurnoverRate:        decimalFromFloat64(basic.GetTurnoverRate()),
		AskPrice:            decimalPtrFromFloat64(basic.AskPrice),
		BidPrice:            decimalPtrFromFloat64(basic.BidPrice),
		AskVolume:           cloneInt64Ptr(basic.AskVol),
		BidVolume:           cloneInt64Ptr(basic.BidVol),
		Amplitude:           decimalPtrFromFloat64(basic.Amplitude),
		AveragePrice:        decimalPtrFromFloat64(basic.AvgPrice),
		BidAskRatio:         decimalPtrFromFloat64(basic.BidAskRatio),
		VolumeRatio:         decimalPtrFromFloat64(basic.VolumeRatio),
		Highest52WeeksPrice: decimalPtrFromFloat64(basic.Highest52WeeksPrice),
		Lowest52WeeksPrice:  decimalPtrFromFloat64(basic.Lowest52WeeksPrice),
		HighestHistoryPrice: decimalPtrFromFloat64(basic.HighestHistoryPrice),
		LowestHistoryPrice:  decimalPtrFromFloat64(basic.LowestHistoryPrice),
		SessionStatus:       enumName(basic.GetSecStatus(), qotcommonpb.SecurityStatus_name),
		ClosePrice5Minute:   decimalPtrFromFloat64(basic.ClosePrice5Minute),
		HighPrecisionVolume: decimalPtrFromFloat64(basic.HpVolume),
		HighPrecisionAskVol: decimalPtrFromFloat64(basic.HpAskVol),
		HighPrecisionBidVol: decimalPtrFromFloat64(basic.HpBidVol),
		PreMarket:           extendedMarketQuoteFromProto(basic.GetPreMarket(), quoteTime),
		AfterMarket:         extendedMarketQuoteFromProto(basic.GetAfterMarket(), quoteTime),
		Overnight:           extendedMarketQuoteFromProto(basic.GetOvernight(), quoteTime),
	}
}

func applyEquitySnapshotDetails(details *SecurityDetails, equity *qotgetsecuritysnapshotpb.EquitySnapshotExData) {
	if equity == nil {
		return
	}
	details.Equity = &EquitySecurityDetails{
		IssuedShares:         equity.GetIssuedShares(),
		IssuedMarketValue:    decimalFromFloat64(equity.GetIssuedMarketVal()),
		NetAsset:             decimalFromFloat64(equity.GetNetAsset()),
		NetProfit:            decimalFromFloat64(equity.GetNetProfit()),
		EarningsPerShare:     decimalFromFloat64(equity.GetEarningsPershare()),
		OutstandingShares:    equity.GetOutstandingShares(),
		OutstandingMarketVal: decimalFromFloat64(equity.GetOutstandingMarketVal()),
		NetAssetPerShare:     decimalFromFloat64(equity.GetNetAssetPershare()),
		EarningsYieldRate:    decimalFromFloat64(equity.GetEyRate()),
		PERate:               decimalFromFloat64(equity.GetPeRate()),
		PBRate:               decimalFromFloat64(equity.GetPbRate()),
		PETTMRate:            decimalFromFloat64(equity.GetPeTTMRate()),
		DividendTTM:          decimalPtrFromFloat64(equity.DividendTTM),
		DividendRatioTTM:     decimalPtrFromFloat64(equity.DividendRatioTTM),
		DividendLFY:          decimalPtrFromFloat64(equity.DividendLFY),
		DividendLFYRatio:     decimalPtrFromFloat64(equity.DividendLFYRatio),
	}
}

func applyWarrantSnapshotDetails(details *SecurityDetails, warrant *qotgetsecuritysnapshotpb.WarrantSnapshotExData) {
	if warrant == nil {
		return
	}
	details.Warrant = &WarrantSecurityDetails{
		ConversionRate:     decimalFromFloat64(warrant.GetConversionRate()),
		WarrantType:        enumName(warrant.GetWarrantType(), qotcommonpb.WarrantType_name),
		StrikePrice:        decimalFromFloat64(warrant.GetStrikePrice()),
		MaturityTime:       warrant.GetMaturityTime(),
		EndTradeTime:       warrant.GetEndTradeTime(),
		Owner:              securityRefFromProto(warrant.GetOwner()),
		RecoveryPrice:      decimalFromFloat64(warrant.GetRecoveryPrice()),
		StreetVolume:       warrant.GetStreetVolumn(),
		IssueVolume:        warrant.GetIssueVolumn(),
		StreetRate:         decimalFromFloat64(warrant.GetStreetRate()),
		Delta:              decimalFromFloat64(warrant.GetDelta()),
		ImpliedVolatility:  decimalFromFloat64(warrant.GetImpliedVolatility()),
		Premium:            decimalFromFloat64(warrant.GetPremium()),
		MaturityTimestamp:  cloneFloat64(warrant.MaturityTimestamp),
		EndTradeTimestamp:  cloneFloat64(warrant.EndTradeTimestamp),
		Leverage:           decimalPtrFromFloat64(warrant.Leverage),
		InOutPriceRatio:    decimalPtrFromFloat64(warrant.Ipop),
		BreakEvenPoint:     decimalPtrFromFloat64(warrant.BreakEvenPoint),
		ConversionPrice:    decimalPtrFromFloat64(warrant.ConversionPrice),
		PriceRecoveryRatio: decimalPtrFromFloat64(warrant.PriceRecoveryRatio),
		Score:              decimalPtrFromFloat64(warrant.Score),
		UpperStrikePrice:   decimalPtrFromFloat64(warrant.UpperStrikePrice),
		LowerStrikePrice:   decimalPtrFromFloat64(warrant.LowerStrikePrice),
		InLinePriceStatus:  enumName(warrant.GetInLinePriceStatus(), qotcommonpb.PriceType_name),
		IssuerCode:         cloneStringPtr(warrant.IssuerCode),
	}
}

func applyOptionSnapshotDetails(details *SecurityDetails, option *qotgetsecuritysnapshotpb.OptionSnapshotExData) {
	if option == nil {
		return
	}
	details.Option = &OptionSecurityDetails{
		OptionType:           enumName(option.GetType(), qotcommonpb.OptionType_name),
		Owner:                securityRefFromProto(option.GetOwner()),
		StrikeTime:           option.GetStrikeTime(),
		StrikePrice:          decimalFromFloat64(option.GetStrikePrice()),
		ContractSize:         option.GetContractSize(),
		ContractSizeFloat:    decimalPtrFromFloat64(option.ContractSizeFloat),
		OpenInterest:         option.GetOpenInterest(),
		ImpliedVolatility:    decimalFromFloat64(option.GetImpliedVolatility()),
		Premium:              decimalFromFloat64(option.GetPremium()),
		Delta:                decimalFromFloat64(option.GetDelta()),
		Gamma:                decimalFromFloat64(option.GetGamma()),
		Vega:                 decimalFromFloat64(option.GetVega()),
		Theta:                decimalFromFloat64(option.GetTheta()),
		Rho:                  decimalFromFloat64(option.GetRho()),
		StrikeTimestamp:      cloneFloat64(option.StrikeTimestamp),
		IndexOptionType:      enumName(option.GetIndexOptionType(), qotcommonpb.IndexOptionType_name),
		NetOpenInterest:      cloneInt32Ptr(option.NetOpenInterest),
		ExpiryDateDistance:   cloneInt32Ptr(option.ExpiryDateDistance),
		ContractNominalValue: decimalPtrFromFloat64(option.ContractNominalValue),
		OwnerLotMultiplier:   decimalPtrFromFloat64(option.OwnerLotMultiplier),
		OptionAreaType:       enumName(option.GetOptionAreaType(), qotcommonpb.OptionAreaType_name),
		ContractMultiplier:   decimalPtrFromFloat64(option.ContractMultiplier),
	}
}

func applyIndexSnapshotDetails(details *SecurityDetails, index *qotgetsecuritysnapshotpb.IndexSnapshotExData) {
	if index == nil {
		return
	}
	details.Index = &IndexSecurityDetails{
		RaiseCount: index.GetRaiseCount(),
		FallCount:  index.GetFallCount(),
		EqualCount: index.GetEqualCount(),
	}
}

func applyPlateSnapshotDetails(details *SecurityDetails, plate *qotgetsecuritysnapshotpb.PlateSnapshotExData) {
	if plate == nil {
		return
	}
	details.Plate = &PlateSecurityDetails{
		RaiseCount: plate.GetRaiseCount(),
		FallCount:  plate.GetFallCount(),
		EqualCount: plate.GetEqualCount(),
	}
}

func applyFutureSnapshotDetails(details *SecurityDetails, future *qotgetsecuritysnapshotpb.FutureSnapshotExData) {
	if future == nil {
		return
	}
	details.Future = &FutureSecurityDetails{
		LastSettlePrice:    decimalFromFloat64(future.GetLastSettlePrice()),
		Position:           future.GetPosition(),
		PositionChange:     future.GetPositionChange(),
		LastTradeTime:      future.GetLastTradeTime(),
		LastTradeTimestamp: cloneFloat64(future.LastTradeTimestamp),
		IsMainContract:     future.GetIsMainContract(),
	}
}

func applyTrustSnapshotDetails(details *SecurityDetails, trust *qotgetsecuritysnapshotpb.TrustSnapshotExData) {
	if trust == nil {
		return
	}
	details.Trust = &TrustSecurityDetails{
		DividendYield:   decimalFromFloat64(trust.GetDividendYield()),
		AUM:             decimalFromFloat64(trust.GetAum()),
		OutstandingUnit: trust.GetOutstandingUnits(),
		NetAssetValue:   decimalFromFloat64(trust.GetNetAssetValue()),
		Premium:         decimalFromFloat64(trust.GetPremium()),
		AssetClass:      enumName(trust.GetAssetClass(), qotcommonpb.AssetClass_name),
	}
}

func mergeStaticInfoIntoSecurityDetails(details *SecurityDetails, info *qotcommonpb.SecurityStaticInfo) {
	if details == nil || info == nil || info.GetBasic() == nil {
		return
	}
	basic := info.GetBasic()
	details.SecurityID = new(basic.GetId())
	if details.Name == "" {
		details.Name = basic.GetName()
	}
	if details.SecurityType == "" {
		details.SecurityType = enumName(basic.GetSecType(), qotcommonpb.SecurityType_name)
	}
	details.ExchangeType = enumName(basic.GetExchType(), qotcommonpb.ExchType_name)
	if details.ListTime == "" {
		details.ListTime = basic.GetListTime()
	}
	if details.ListTimestamp == nil {
		details.ListTimestamp = cloneFloat64(basic.ListTimestamp)
	}
	details.Delisting = cloneBoolPtr(basic.Delisting)
	if details.LotSize == 0 {
		details.LotSize = basic.GetLotSize()
	}
	if warrant := info.GetWarrantExData(); warrant != nil {
		if details.Warrant == nil {
			details.Warrant = &WarrantSecurityDetails{}
		}
		if details.Warrant.WarrantType == "" {
			details.Warrant.WarrantType = enumName(warrant.GetType(), qotcommonpb.WarrantType_name)
		}
		if details.Warrant.Owner == nil {
			details.Warrant.Owner = securityRefFromProto(warrant.GetOwner())
		}
	}
	if option := info.GetOptionExData(); option != nil {
		if details.Option == nil {
			details.Option = &OptionSecurityDetails{}
		}
		if details.Option.OptionType == "" {
			details.Option.OptionType = enumName(option.GetType(), qotcommonpb.OptionType_name)
		}
		if details.Option.Owner == nil {
			details.Option.Owner = securityRefFromProto(option.GetOwner())
		}
		if details.Option.StrikeTime == "" {
			details.Option.StrikeTime = option.GetStrikeTime()
		}
		if details.Option.StrikePrice.IsZero() {
			details.Option.StrikePrice = decimalFromFloat64(option.GetStrikePrice())
		}
		if details.Option.StrikeTimestamp == nil {
			details.Option.StrikeTimestamp = cloneFloat64(option.StrikeTimestamp)
		}
		if details.Option.IndexOptionType == "" {
			details.Option.IndexOptionType = enumName(option.GetIndexOptionType(), qotcommonpb.IndexOptionType_name)
		}
	}
	if future := info.GetFutureExData(); future != nil {
		if details.Future == nil {
			details.Future = &FutureSecurityDetails{}
		}
		if details.Future.LastTradeTime == "" {
			details.Future.LastTradeTime = future.GetLastTradeTime()
		}
		if details.Future.LastTradeTimestamp == nil {
			details.Future.LastTradeTimestamp = cloneFloat64(future.LastTradeTimestamp)
		}
		details.Future.IsMainContract = future.GetIsMainContract()
	}
}

func securityRefFromCanonical(canonical string) *SecurityRef {
	parts := strings.SplitN(strings.TrimSpace(strings.ToUpper(canonical)), ".", 2)
	if len(parts) != 2 {
		return &SecurityRef{InstrumentID: canonical}
	}
	return &SecurityRef{InstrumentID: parts[0] + "." + parts[1], Market: parts[0], Symbol: parts[1]}
}

func securityRefFromProto(security *qotcommonpb.Security) *SecurityRef {
	canonical, err := futuSymbolFromSecurity(security)
	if err != nil {
		return nil
	}
	return securityRefFromCanonical(canonical)
}

func enumName(value int32, names map[int32]string) string {
	raw := names[value]
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, "_", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return raw
}

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneInt32Ptr(value *int32) *int32 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	return new(*value)
}
