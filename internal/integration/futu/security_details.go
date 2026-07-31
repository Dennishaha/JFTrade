package futu

import (
	"encoding/json"

	"github.com/shopspring/decimal"

	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
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

func ExtendedMarketQuoteSecurityMap(quote *pkgfutu.ExtendedMarketQuote) map[string]any {
	if quote == nil {
		return nil
	}
	return map[string]any{
		"price":      optionalDecimalJSON(quote.Price),
		"highPrice":  optionalDecimalJSON(quote.HighPrice),
		"lowPrice":   optionalDecimalJSON(quote.LowPrice),
		"volume":     optionalDecimalJSON(quote.Volume),
		"turnover":   optionalDecimalJSON(quote.Turnover),
		"changeVal":  optionalDecimalJSON(quote.ChangeVal),
		"changeRate": optionalDecimalJSON(quote.ChangeRate),
		"amplitude":  optionalDecimalJSON(quote.Amplitude),
		"quoteTime":  quote.QuoteTime,
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

func SecurityRefMap(ref *pkgfutu.SecurityRef) map[string]any {
	if ref == nil {
		return nil
	}
	return map[string]any{
		"instrumentId": ref.InstrumentID,
		"market":       ref.Market,
		"symbol":       ref.Symbol,
	}
}

// SecurityDetailsMap converts Futu's protocol-backed model to the neutral wire
// representation consumed by the marketdata domain.
func SecurityDetailsMap(details *pkgfutu.SecurityDetails) map[string]any {
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
		"productClass":        details.ProductClass,
		"marketSegment":       details.MarketSegment,
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
		"volume":              decimalJSON(details.Volume),
		"turnover":            decimalJSON(details.Turnover),
		"turnoverRate":        decimalJSON(details.TurnoverRate),
		"askPrice":            optionalDecimalJSON(details.AskPrice),
		"bidPrice":            optionalDecimalJSON(details.BidPrice),
		"askVolume":           optionalDecimalJSON(details.AskVolume),
		"bidVolume":           optionalDecimalJSON(details.BidVolume),
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
		"extended": map[string]any{
			"preMarket":   ExtendedMarketQuoteSecurityMap(details.PreMarket),
			"afterMarket": ExtendedMarketQuoteSecurityMap(details.AfterMarket),
			"overnight":   ExtendedMarketQuoteSecurityMap(details.Overnight),
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

func equitySecurityDetailsMap(details *pkgfutu.EquitySecurityDetails) map[string]any {
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

func warrantSecurityDetailsMap(details *pkgfutu.WarrantSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"conversionRate":     decimalJSON(details.ConversionRate),
		"warrantType":        details.WarrantType,
		"strikePrice":        decimalJSON(details.StrikePrice),
		"maturityTime":       details.MaturityTime,
		"endTradeTime":       details.EndTradeTime,
		"owner":              SecurityRefMap(details.Owner),
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

func optionSecurityDetailsMap(details *pkgfutu.OptionSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"optionType":           details.OptionType,
		"owner":                SecurityRefMap(details.Owner),
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

func indexSecurityDetailsMap(details *pkgfutu.IndexSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"raiseCount": details.RaiseCount,
		"fallCount":  details.FallCount,
		"equalCount": details.EqualCount,
	}
}

func plateSecurityDetailsMap(details *pkgfutu.PlateSecurityDetails) map[string]any {
	if details == nil {
		return nil
	}
	return map[string]any{
		"raiseCount": details.RaiseCount,
		"fallCount":  details.FallCount,
		"equalCount": details.EqualCount,
	}
}

func futureSecurityDetailsMap(details *pkgfutu.FutureSecurityDetails) map[string]any {
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

func trustSecurityDetailsMap(details *pkgfutu.TrustSecurityDetails) map[string]any {
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
