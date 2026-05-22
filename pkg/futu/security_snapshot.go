package futu

import (
	"context"
	"fmt"
	"strings"

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
	IssuedMarketValue    float64
	NetAsset             float64
	NetProfit            float64
	EarningsPerShare     float64
	OutstandingShares    int64
	OutstandingMarketVal float64
	NetAssetPerShare     float64
	EarningsYieldRate    float64
	PERate               float64
	PBRate               float64
	PETTMRate            float64
	DividendTTM          *float64
	DividendRatioTTM     *float64
	DividendLFY          *float64
	DividendLFYRatio     *float64
}

type WarrantSecurityDetails struct {
	ConversionRate     float64
	WarrantType        string
	StrikePrice        float64
	MaturityTime       string
	EndTradeTime       string
	Owner              *SecurityRef
	RecoveryPrice      float64
	StreetVolume       int64
	IssueVolume        int64
	StreetRate         float64
	Delta              float64
	ImpliedVolatility  float64
	Premium            float64
	MaturityTimestamp  *float64
	EndTradeTimestamp  *float64
	Leverage           *float64
	InOutPriceRatio    *float64
	BreakEvenPoint     *float64
	ConversionPrice    *float64
	PriceRecoveryRatio *float64
	Score              *float64
	UpperStrikePrice   *float64
	LowerStrikePrice   *float64
	InLinePriceStatus  string
	IssuerCode         *string
}

type OptionSecurityDetails struct {
	OptionType           string
	Owner                *SecurityRef
	StrikeTime           string
	StrikePrice          float64
	ContractSize         int32
	ContractSizeFloat    *float64
	OpenInterest         int32
	ImpliedVolatility    float64
	Premium              float64
	Delta                float64
	Gamma                float64
	Vega                 float64
	Theta                float64
	Rho                  float64
	StrikeTimestamp      *float64
	IndexOptionType      string
	NetOpenInterest      *int32
	ExpiryDateDistance   *int32
	ContractNominalValue *float64
	OwnerLotMultiplier   *float64
	OptionAreaType       string
	ContractMultiplier   *float64
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
	LastSettlePrice    float64
	Position           int32
	PositionChange     int32
	LastTradeTime      string
	LastTradeTimestamp *float64
	IsMainContract     bool
}

type TrustSecurityDetails struct {
	DividendYield   float64
	AUM             float64
	OutstandingUnit int64
	NetAssetValue   float64
	Premium         float64
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
	PriceSpread         float64
	UpdateTime          string
	UpdateTimestamp     *float64
	HighPrice           float64
	OpenPrice           float64
	LowPrice            float64
	LastClosePrice      float64
	CurrentPrice        float64
	Volume              int64
	Turnover            float64
	TurnoverRate        float64
	AskPrice            *float64
	BidPrice            *float64
	AskVolume           *int64
	BidVolume           *int64
	Amplitude           *float64
	AveragePrice        *float64
	BidAskRatio         *float64
	VolumeRatio         *float64
	Highest52WeeksPrice *float64
	Lowest52WeeksPrice  *float64
	HighestHistoryPrice *float64
	LowestHistoryPrice  *float64
	SessionStatus       string
	ClosePrice5Minute   *float64
	HighPrecisionVolume *float64
	HighPrecisionAskVol *float64
	HighPrecisionBidVol *float64
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
	canonical, err := futuSymbolFromSecurity(snapshot.GetBasic().GetSecurity())
	if err != nil {
		canonical = strings.TrimSpace(strings.ToUpper(symbol))
	}
	details := securityDetailsFromSnapshot(snapshot, canonical)
	if details == nil {
		return nil, fmt.Errorf("opend GetSecuritySnapshot returned no usable snapshot for %s", symbol)
	}
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

func securityDetailsFromSnapshot(snapshot *qotgetsecuritysnapshotpb.Snapshot, canonical string) *SecurityDetails {
	if snapshot == nil || snapshot.GetBasic() == nil {
		return nil
	}
	basic := snapshot.GetBasic()
	ref := securityRefFromProto(basic.GetSecurity())
	if ref == nil {
		ref = securityRefFromCanonical(canonical)
	}
	details := &SecurityDetails{
		InstrumentID:        ref.InstrumentID,
		Market:              ref.Market,
		Symbol:              ref.Symbol,
		Name:                basic.GetName(),
		SecurityType:        enumName(basic.GetType(), qotcommonpb.SecurityType_name),
		ListTime:            basic.GetListTime(),
		ListTimestamp:       cloneFloat64(basic.ListTimestamp),
		LotSize:             basic.GetLotSize(),
		IsSuspend:           basic.GetIsSuspend(),
		PriceSpread:         basic.GetPriceSpread(),
		UpdateTime:          basic.GetUpdateTime(),
		UpdateTimestamp:     cloneFloat64(basic.UpdateTimestamp),
		HighPrice:           basic.GetHighPrice(),
		OpenPrice:           basic.GetOpenPrice(),
		LowPrice:            basic.GetLowPrice(),
		LastClosePrice:      basic.GetLastClosePrice(),
		CurrentPrice:        basic.GetCurPrice(),
		Volume:              basic.GetVolume(),
		Turnover:            basic.GetTurnover(),
		TurnoverRate:        basic.GetTurnoverRate(),
		AskPrice:            cloneFloat64(basic.AskPrice),
		BidPrice:            cloneFloat64(basic.BidPrice),
		AskVolume:           cloneInt64Ptr(basic.AskVol),
		BidVolume:           cloneInt64Ptr(basic.BidVol),
		Amplitude:           cloneFloat64(basic.Amplitude),
		AveragePrice:        cloneFloat64(basic.AvgPrice),
		BidAskRatio:         cloneFloat64(basic.BidAskRatio),
		VolumeRatio:         cloneFloat64(basic.VolumeRatio),
		Highest52WeeksPrice: cloneFloat64(basic.Highest52WeeksPrice),
		Lowest52WeeksPrice:  cloneFloat64(basic.Lowest52WeeksPrice),
		HighestHistoryPrice: cloneFloat64(basic.HighestHistoryPrice),
		LowestHistoryPrice:  cloneFloat64(basic.LowestHistoryPrice),
		SessionStatus:       enumName(basic.GetSecStatus(), qotcommonpb.SecurityStatus_name),
		ClosePrice5Minute:   cloneFloat64(basic.ClosePrice5Minute),
		HighPrecisionVolume: cloneFloat64(basic.HpVolume),
		HighPrecisionAskVol: cloneFloat64(basic.HpAskVol),
		HighPrecisionBidVol: cloneFloat64(basic.HpBidVol),
		PreMarket:           extendedMarketQuoteFromProto(basic.GetPreMarket()),
		AfterMarket:         extendedMarketQuoteFromProto(basic.GetAfterMarket()),
		Overnight:           extendedMarketQuoteFromProto(basic.GetOvernight()),
	}
	if equity := snapshot.GetEquityExData(); equity != nil {
		details.Equity = &EquitySecurityDetails{
			IssuedShares:         equity.GetIssuedShares(),
			IssuedMarketValue:    equity.GetIssuedMarketVal(),
			NetAsset:             equity.GetNetAsset(),
			NetProfit:            equity.GetNetProfit(),
			EarningsPerShare:     equity.GetEarningsPershare(),
			OutstandingShares:    equity.GetOutstandingShares(),
			OutstandingMarketVal: equity.GetOutstandingMarketVal(),
			NetAssetPerShare:     equity.GetNetAssetPershare(),
			EarningsYieldRate:    equity.GetEyRate(),
			PERate:               equity.GetPeRate(),
			PBRate:               equity.GetPbRate(),
			PETTMRate:            equity.GetPeTTMRate(),
			DividendTTM:          cloneFloat64(equity.DividendTTM),
			DividendRatioTTM:     cloneFloat64(equity.DividendRatioTTM),
			DividendLFY:          cloneFloat64(equity.DividendLFY),
			DividendLFYRatio:     cloneFloat64(equity.DividendLFYRatio),
		}
	}
	if warrant := snapshot.GetWarrantExData(); warrant != nil {
		details.Warrant = &WarrantSecurityDetails{
			ConversionRate:     warrant.GetConversionRate(),
			WarrantType:        enumName(warrant.GetWarrantType(), qotcommonpb.WarrantType_name),
			StrikePrice:        warrant.GetStrikePrice(),
			MaturityTime:       warrant.GetMaturityTime(),
			EndTradeTime:       warrant.GetEndTradeTime(),
			Owner:              securityRefFromProto(warrant.GetOwner()),
			RecoveryPrice:      warrant.GetRecoveryPrice(),
			StreetVolume:       warrant.GetStreetVolumn(),
			IssueVolume:        warrant.GetIssueVolumn(),
			StreetRate:         warrant.GetStreetRate(),
			Delta:              warrant.GetDelta(),
			ImpliedVolatility:  warrant.GetImpliedVolatility(),
			Premium:            warrant.GetPremium(),
			MaturityTimestamp:  cloneFloat64(warrant.MaturityTimestamp),
			EndTradeTimestamp:  cloneFloat64(warrant.EndTradeTimestamp),
			Leverage:           cloneFloat64(warrant.Leverage),
			InOutPriceRatio:    cloneFloat64(warrant.Ipop),
			BreakEvenPoint:     cloneFloat64(warrant.BreakEvenPoint),
			ConversionPrice:    cloneFloat64(warrant.ConversionPrice),
			PriceRecoveryRatio: cloneFloat64(warrant.PriceRecoveryRatio),
			Score:              cloneFloat64(warrant.Score),
			UpperStrikePrice:   cloneFloat64(warrant.UpperStrikePrice),
			LowerStrikePrice:   cloneFloat64(warrant.LowerStrikePrice),
			InLinePriceStatus:  enumName(warrant.GetInLinePriceStatus(), qotcommonpb.PriceType_name),
			IssuerCode:         cloneStringPtr(warrant.IssuerCode),
		}
	}
	if option := snapshot.GetOptionExData(); option != nil {
		details.Option = &OptionSecurityDetails{
			OptionType:           enumName(option.GetType(), qotcommonpb.OptionType_name),
			Owner:                securityRefFromProto(option.GetOwner()),
			StrikeTime:           option.GetStrikeTime(),
			StrikePrice:          option.GetStrikePrice(),
			ContractSize:         option.GetContractSize(),
			ContractSizeFloat:    cloneFloat64(option.ContractSizeFloat),
			OpenInterest:         option.GetOpenInterest(),
			ImpliedVolatility:    option.GetImpliedVolatility(),
			Premium:              option.GetPremium(),
			Delta:                option.GetDelta(),
			Gamma:                option.GetGamma(),
			Vega:                 option.GetVega(),
			Theta:                option.GetTheta(),
			Rho:                  option.GetRho(),
			StrikeTimestamp:      cloneFloat64(option.StrikeTimestamp),
			IndexOptionType:      enumName(option.GetIndexOptionType(), qotcommonpb.IndexOptionType_name),
			NetOpenInterest:      cloneInt32Ptr(option.NetOpenInterest),
			ExpiryDateDistance:   cloneInt32Ptr(option.ExpiryDateDistance),
			ContractNominalValue: cloneFloat64(option.ContractNominalValue),
			OwnerLotMultiplier:   cloneFloat64(option.OwnerLotMultiplier),
			OptionAreaType:       enumName(option.GetOptionAreaType(), qotcommonpb.OptionAreaType_name),
			ContractMultiplier:   cloneFloat64(option.ContractMultiplier),
		}
	}
	if index := snapshot.GetIndexExData(); index != nil {
		details.Index = &IndexSecurityDetails{
			RaiseCount: index.GetRaiseCount(),
			FallCount:  index.GetFallCount(),
			EqualCount: index.GetEqualCount(),
		}
	}
	if plate := snapshot.GetPlateExData(); plate != nil {
		details.Plate = &PlateSecurityDetails{
			RaiseCount: plate.GetRaiseCount(),
			FallCount:  plate.GetFallCount(),
			EqualCount: plate.GetEqualCount(),
		}
	}
	if future := snapshot.GetFutureExData(); future != nil {
		details.Future = &FutureSecurityDetails{
			LastSettlePrice:    future.GetLastSettlePrice(),
			Position:           future.GetPosition(),
			PositionChange:     future.GetPositionChange(),
			LastTradeTime:      future.GetLastTradeTime(),
			LastTradeTimestamp: cloneFloat64(future.LastTradeTimestamp),
			IsMainContract:     future.GetIsMainContract(),
		}
	}
	if trust := snapshot.GetTrustExData(); trust != nil {
		details.Trust = &TrustSecurityDetails{
			DividendYield:   trust.GetDividendYield(),
			AUM:             trust.GetAum(),
			OutstandingUnit: trust.GetOutstandingUnits(),
			NetAssetValue:   trust.GetNetAssetValue(),
			Premium:         trust.GetPremium(),
			AssetClass:      enumName(trust.GetAssetClass(), qotcommonpb.AssetClass_name),
		}
	}
	return details
}

func mergeStaticInfoIntoSecurityDetails(details *SecurityDetails, info *qotcommonpb.SecurityStaticInfo) {
	if details == nil || info == nil || info.GetBasic() == nil {
		return
	}
	basic := info.GetBasic()
	securityID := basic.GetId()
	details.SecurityID = &securityID
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
		if details.Option.StrikePrice == 0 {
			details.Option.StrikePrice = option.GetStrikePrice()
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
	cloned := *value
	return &cloned
}

func cloneInt32Ptr(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
