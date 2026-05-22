package jftradeapi

import (
	"encoding/json"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

func float64JSON(v float64) json.Number {
	return json.Number(decimal.NewFromFloat(v).String())
}

func optionalFloat64JSON(v *float64) any {
	if v == nil {
		return nil
	}
	return float64JSON(*v)
}

func optionalInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func optionalInt32(v *int32) any {
	if v == nil {
		return nil
	}
	return *v
}

func optionalBool(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func optionalString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func securityRefMap(ref *futu.SecurityRef) map[string]any {
	if ref == nil {
		return nil
	}
	return map[string]any{
		"instrumentId": ref.InstrumentID,
		"market":       ref.Market,
		"symbol":       ref.Symbol,
	}
}

func securityDetailsMap(details *futu.SecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"instrumentId":        details.InstrumentID,
		"market":              details.Market,
		"symbol":              details.Symbol,
		"securityId":          optionalInt64(details.SecurityID),
		"name":                details.Name,
		"securityType":        details.SecurityType,
		"exchangeType":        details.ExchangeType,
		"listTime":            details.ListTime,
		"listTimestamp":       optionalFloat64JSON(details.ListTimestamp),
		"delisting":           optionalBool(details.Delisting),
		"lotSize":             details.LotSize,
		"isSuspend":           details.IsSuspend,
		"priceSpread":         float64JSON(details.PriceSpread),
		"updateTime":          details.UpdateTime,
		"updateTimestamp":     optionalFloat64JSON(details.UpdateTimestamp),
		"highPrice":           float64JSON(details.HighPrice),
		"openPrice":           float64JSON(details.OpenPrice),
		"lowPrice":            float64JSON(details.LowPrice),
		"lastClosePrice":      float64JSON(details.LastClosePrice),
		"currentPrice":        float64JSON(details.CurrentPrice),
		"volume":              details.Volume,
		"turnover":            float64JSON(details.Turnover),
		"turnoverRate":        float64JSON(details.TurnoverRate),
		"askPrice":            optionalFloat64JSON(details.AskPrice),
		"bidPrice":            optionalFloat64JSON(details.BidPrice),
		"askVolume":           optionalInt64(details.AskVolume),
		"bidVolume":           optionalInt64(details.BidVolume),
		"amplitude":           optionalFloat64JSON(details.Amplitude),
		"averagePrice":        optionalFloat64JSON(details.AveragePrice),
		"bidAskRatio":         optionalFloat64JSON(details.BidAskRatio),
		"volumeRatio":         optionalFloat64JSON(details.VolumeRatio),
		"highest52WeeksPrice": optionalFloat64JSON(details.Highest52WeeksPrice),
		"lowest52WeeksPrice":  optionalFloat64JSON(details.Lowest52WeeksPrice),
		"highestHistoryPrice": optionalFloat64JSON(details.HighestHistoryPrice),
		"lowestHistoryPrice":  optionalFloat64JSON(details.LowestHistoryPrice),
		"sessionStatus":       details.SessionStatus,
		"closePrice5Minute":   optionalFloat64JSON(details.ClosePrice5Minute),
		"highPrecisionVolume": optionalFloat64JSON(details.HighPrecisionVolume),
		"highPrecisionAskVol": optionalFloat64JSON(details.HighPrecisionAskVol),
		"highPrecisionBidVol": optionalFloat64JSON(details.HighPrecisionBidVol),
		"extended": map[string]any{
			"preMarket":   extendedMarketQuoteMap(details.PreMarket),
			"afterMarket": extendedMarketQuoteMap(details.AfterMarket),
			"overnight":   extendedMarketQuoteMap(details.Overnight),
		},
		"equity":  equitySecurityDetailsMap(details.Equity),
		"warrant": warrantSecurityDetailsMap(details.Warrant),
		"option":  optionSecurityDetailsMap(details.Option),
		"index":   indexSecurityDetailsMap(details.Index),
		"plate":   plateSecurityDetailsMap(details.Plate),
		"future":  futureSecurityDetailsMap(details.Future),
		"trust":   trustSecurityDetailsMap(details.Trust),
	}
}

func equitySecurityDetailsMap(details *futu.EquitySecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"issuedShares":         details.IssuedShares,
		"issuedMarketValue":    float64JSON(details.IssuedMarketValue),
		"netAsset":             float64JSON(details.NetAsset),
		"netProfit":            float64JSON(details.NetProfit),
		"earningsPerShare":     float64JSON(details.EarningsPerShare),
		"outstandingShares":    details.OutstandingShares,
		"outstandingMarketVal": float64JSON(details.OutstandingMarketVal),
		"netAssetPerShare":     float64JSON(details.NetAssetPerShare),
		"earningsYieldRate":    float64JSON(details.EarningsYieldRate),
		"peRate":               float64JSON(details.PERate),
		"pbRate":               float64JSON(details.PBRate),
		"peTTMRate":            float64JSON(details.PETTMRate),
		"dividendTTM":          optionalFloat64JSON(details.DividendTTM),
		"dividendRatioTTM":     optionalFloat64JSON(details.DividendRatioTTM),
		"dividendLFY":          optionalFloat64JSON(details.DividendLFY),
		"dividendLFYRatio":     optionalFloat64JSON(details.DividendLFYRatio),
	}
}

func warrantSecurityDetailsMap(details *futu.WarrantSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"conversionRate":     float64JSON(details.ConversionRate),
		"warrantType":        details.WarrantType,
		"strikePrice":        float64JSON(details.StrikePrice),
		"maturityTime":       details.MaturityTime,
		"endTradeTime":       details.EndTradeTime,
		"owner":              securityRefMap(details.Owner),
		"recoveryPrice":      float64JSON(details.RecoveryPrice),
		"streetVolume":       details.StreetVolume,
		"issueVolume":        details.IssueVolume,
		"streetRate":         float64JSON(details.StreetRate),
		"delta":              float64JSON(details.Delta),
		"impliedVolatility":  float64JSON(details.ImpliedVolatility),
		"premium":            float64JSON(details.Premium),
		"maturityTimestamp":  optionalFloat64JSON(details.MaturityTimestamp),
		"endTradeTimestamp":  optionalFloat64JSON(details.EndTradeTimestamp),
		"leverage":           optionalFloat64JSON(details.Leverage),
		"inOutPriceRatio":    optionalFloat64JSON(details.InOutPriceRatio),
		"breakEvenPoint":     optionalFloat64JSON(details.BreakEvenPoint),
		"conversionPrice":    optionalFloat64JSON(details.ConversionPrice),
		"priceRecoveryRatio": optionalFloat64JSON(details.PriceRecoveryRatio),
		"score":              optionalFloat64JSON(details.Score),
		"upperStrikePrice":   optionalFloat64JSON(details.UpperStrikePrice),
		"lowerStrikePrice":   optionalFloat64JSON(details.LowerStrikePrice),
		"inLinePriceStatus":  details.InLinePriceStatus,
		"issuerCode":         optionalString(details.IssuerCode),
	}
}

func optionSecurityDetailsMap(details *futu.OptionSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"optionType":           details.OptionType,
		"owner":                securityRefMap(details.Owner),
		"strikeTime":           details.StrikeTime,
		"strikePrice":          float64JSON(details.StrikePrice),
		"contractSize":         details.ContractSize,
		"contractSizeFloat":    optionalFloat64JSON(details.ContractSizeFloat),
		"openInterest":         details.OpenInterest,
		"impliedVolatility":    float64JSON(details.ImpliedVolatility),
		"premium":              float64JSON(details.Premium),
		"delta":                float64JSON(details.Delta),
		"gamma":                float64JSON(details.Gamma),
		"vega":                 float64JSON(details.Vega),
		"theta":                float64JSON(details.Theta),
		"rho":                  float64JSON(details.Rho),
		"strikeTimestamp":      optionalFloat64JSON(details.StrikeTimestamp),
		"indexOptionType":      details.IndexOptionType,
		"netOpenInterest":      optionalInt32(details.NetOpenInterest),
		"expiryDateDistance":   optionalInt32(details.ExpiryDateDistance),
		"contractNominalValue": optionalFloat64JSON(details.ContractNominalValue),
		"ownerLotMultiplier":   optionalFloat64JSON(details.OwnerLotMultiplier),
		"optionAreaType":       details.OptionAreaType,
		"contractMultiplier":   optionalFloat64JSON(details.ContractMultiplier),
	}
}

func indexSecurityDetailsMap(details *futu.IndexSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"raiseCount": details.RaiseCount,
		"fallCount":  details.FallCount,
		"equalCount": details.EqualCount,
	}
}

func plateSecurityDetailsMap(details *futu.PlateSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"raiseCount": details.RaiseCount,
		"fallCount":  details.FallCount,
		"equalCount": details.EqualCount,
	}
}

func futureSecurityDetailsMap(details *futu.FutureSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"lastSettlePrice":    float64JSON(details.LastSettlePrice),
		"position":           details.Position,
		"positionChange":     details.PositionChange,
		"lastTradeTime":      details.LastTradeTime,
		"lastTradeTimestamp": optionalFloat64JSON(details.LastTradeTimestamp),
		"isMainContract":     details.IsMainContract,
	}
}

func trustSecurityDetailsMap(details *futu.TrustSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"dividendYield":   float64JSON(details.DividendYield),
		"aum":             float64JSON(details.AUM),
		"outstandingUnit": details.OutstandingUnit,
		"netAssetValue":   float64JSON(details.NetAssetValue),
		"premium":         float64JSON(details.Premium),
		"assetClass":      details.AssetClass,
	}
}
