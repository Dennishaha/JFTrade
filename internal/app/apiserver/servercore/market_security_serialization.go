package servercore

import (
	"encoding/json"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

func float64JSON(v float64) json.Number {
	return json.Number(decimal.NewFromFloat(v).String())
}

func decimalJSON(v decimal.Decimal) string {
	return v.String()
}

func optionalDecimalJSON(v *decimal.Decimal) any {
	if v == nil {
		return nil
	}
	return v.String()
}

func extendedMarketQuoteSecurityMap(quote *futu.ExtendedMarketQuote) map[string]any {
	if quote == nil {
		return nil
	}
	return map[string]any{
		"price":      optionalDecimalJSON(quote.Price),
		"highPrice":  optionalDecimalJSON(quote.HighPrice),
		"lowPrice":   optionalDecimalJSON(quote.LowPrice),
		"volume":     quote.Volume,
		"turnover":   optionalDecimalJSON(quote.Turnover),
		"changeVal":  optionalDecimalJSON(quote.ChangeVal),
		"changeRate": optionalDecimalJSON(quote.ChangeRate),
		"amplitude":  optionalDecimalJSON(quote.Amplitude),
	}
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
		"priceSpread":         decimalJSON(details.PriceSpread),
		"updateTime":          details.UpdateTime,
		"updateTimestamp":     optionalFloat64JSON(details.UpdateTimestamp),
		"highPrice":           decimalJSON(details.HighPrice),
		"openPrice":           decimalJSON(details.OpenPrice),
		"lowPrice":            decimalJSON(details.LowPrice),
		"lastClosePrice":      decimalJSON(details.LastClosePrice),
		"currentPrice":        decimalJSON(details.CurrentPrice),
		"volume":              details.Volume,
		"turnover":            decimalJSON(details.Turnover),
		"turnoverRate":        decimalJSON(details.TurnoverRate),
		"askPrice":            optionalDecimalJSON(details.AskPrice),
		"bidPrice":            optionalDecimalJSON(details.BidPrice),
		"askVolume":           optionalInt64(details.AskVolume),
		"bidVolume":           optionalInt64(details.BidVolume),
		"amplitude":           optionalDecimalJSON(details.Amplitude),
		"averagePrice":        optionalDecimalJSON(details.AveragePrice),
		"bidAskRatio":         optionalDecimalJSON(details.BidAskRatio),
		"volumeRatio":         optionalDecimalJSON(details.VolumeRatio),
		"highest52WeeksPrice": optionalDecimalJSON(details.Highest52WeeksPrice),
		"lowest52WeeksPrice":  optionalDecimalJSON(details.Lowest52WeeksPrice),
		"highestHistoryPrice": optionalDecimalJSON(details.HighestHistoryPrice),
		"lowestHistoryPrice":  optionalDecimalJSON(details.LowestHistoryPrice),
		"sessionStatus":       details.SessionStatus,
		"closePrice5Minute":   optionalDecimalJSON(details.ClosePrice5Minute),
		"highPrecisionVolume": optionalDecimalJSON(details.HighPrecisionVolume),
		"highPrecisionAskVol": optionalDecimalJSON(details.HighPrecisionAskVol),
		"highPrecisionBidVol": optionalDecimalJSON(details.HighPrecisionBidVol),
		"extended": map[string]any{
			"preMarket":   extendedMarketQuoteSecurityMap(details.PreMarket),
			"afterMarket": extendedMarketQuoteSecurityMap(details.AfterMarket),
			"overnight":   extendedMarketQuoteSecurityMap(details.Overnight),
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
		"issuedMarketValue":    decimalJSON(details.IssuedMarketValue),
		"netAsset":             decimalJSON(details.NetAsset),
		"netProfit":            decimalJSON(details.NetProfit),
		"earningsPerShare":     decimalJSON(details.EarningsPerShare),
		"outstandingShares":    details.OutstandingShares,
		"outstandingMarketVal": decimalJSON(details.OutstandingMarketVal),
		"netAssetPerShare":     decimalJSON(details.NetAssetPerShare),
		"earningsYieldRate":    decimalJSON(details.EarningsYieldRate),
		"peRate":               decimalJSON(details.PERate),
		"pbRate":               decimalJSON(details.PBRate),
		"peTTMRate":            decimalJSON(details.PETTMRate),
		"dividendTTM":          optionalDecimalJSON(details.DividendTTM),
		"dividendRatioTTM":     optionalDecimalJSON(details.DividendRatioTTM),
		"dividendLFY":          optionalDecimalJSON(details.DividendLFY),
		"dividendLFYRatio":     optionalDecimalJSON(details.DividendLFYRatio),
	}
}

func warrantSecurityDetailsMap(details *futu.WarrantSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"conversionRate":     decimalJSON(details.ConversionRate),
		"warrantType":        details.WarrantType,
		"strikePrice":        decimalJSON(details.StrikePrice),
		"maturityTime":       details.MaturityTime,
		"endTradeTime":       details.EndTradeTime,
		"owner":              securityRefMap(details.Owner),
		"recoveryPrice":      decimalJSON(details.RecoveryPrice),
		"streetVolume":       details.StreetVolume,
		"issueVolume":        details.IssueVolume,
		"streetRate":         decimalJSON(details.StreetRate),
		"delta":              decimalJSON(details.Delta),
		"impliedVolatility":  decimalJSON(details.ImpliedVolatility),
		"premium":            decimalJSON(details.Premium),
		"maturityTimestamp":  optionalFloat64JSON(details.MaturityTimestamp),
		"endTradeTimestamp":  optionalFloat64JSON(details.EndTradeTimestamp),
		"leverage":           optionalDecimalJSON(details.Leverage),
		"inOutPriceRatio":    optionalDecimalJSON(details.InOutPriceRatio),
		"breakEvenPoint":     optionalDecimalJSON(details.BreakEvenPoint),
		"conversionPrice":    optionalDecimalJSON(details.ConversionPrice),
		"priceRecoveryRatio": optionalDecimalJSON(details.PriceRecoveryRatio),
		"score":              optionalDecimalJSON(details.Score),
		"upperStrikePrice":   optionalDecimalJSON(details.UpperStrikePrice),
		"lowerStrikePrice":   optionalDecimalJSON(details.LowerStrikePrice),
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
		"strikePrice":          decimalJSON(details.StrikePrice),
		"contractSize":         details.ContractSize,
		"contractSizeFloat":    optionalDecimalJSON(details.ContractSizeFloat),
		"openInterest":         details.OpenInterest,
		"impliedVolatility":    decimalJSON(details.ImpliedVolatility),
		"premium":              decimalJSON(details.Premium),
		"delta":                decimalJSON(details.Delta),
		"gamma":                decimalJSON(details.Gamma),
		"vega":                 decimalJSON(details.Vega),
		"theta":                decimalJSON(details.Theta),
		"rho":                  decimalJSON(details.Rho),
		"strikeTimestamp":      optionalFloat64JSON(details.StrikeTimestamp),
		"indexOptionType":      details.IndexOptionType,
		"netOpenInterest":      optionalInt32(details.NetOpenInterest),
		"expiryDateDistance":   optionalInt32(details.ExpiryDateDistance),
		"contractNominalValue": optionalDecimalJSON(details.ContractNominalValue),
		"ownerLotMultiplier":   optionalDecimalJSON(details.OwnerLotMultiplier),
		"optionAreaType":       details.OptionAreaType,
		"contractMultiplier":   optionalDecimalJSON(details.ContractMultiplier),
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
		"lastSettlePrice":    decimalJSON(details.LastSettlePrice),
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
		"dividendYield":   decimalJSON(details.DividendYield),
		"aum":             decimalJSON(details.AUM),
		"outstandingUnit": details.OutstandingUnit,
		"netAssetValue":   decimalJSON(details.NetAssetValue),
		"premium":         decimalJSON(details.Premium),
		"assetClass":      details.AssetClass,
	}
}
