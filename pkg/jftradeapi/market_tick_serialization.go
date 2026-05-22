package jftradeapi

import (
	"encoding/json"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

// priceJSON serialises a decimal.Decimal as a json.Number so the JSON
// encoder emits a literal numeric token (no quotes) with exact decimal digits.
func priceJSON(d decimal.Decimal) json.Number {
	return json.Number(d.String())
}

// optionalPriceJSON serialises a *decimal.Decimal as either null or a
// json.Number literal numeric token.
func optionalPriceJSON(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return json.Number(d.String())
}

// floatPtrToJSONNumber converts a *float64 from a Futu proto adapter struct
// (e.g. ExtendedMarketQuote) to a json.Number for serialisation.
func floatPtrToJSONNumber(v *float64) any {
	if v == nil {
		return nil
	}
	return json.Number(decimal.NewFromFloat(*v).String())
}

func snapshotMapFromSample(sample *marketTickSample) map[string]any {
	return map[string]any{
		"price":              priceJSON(sample.Price),
		"bid":                priceJSON(sample.Bid),
		"ask":                priceJSON(sample.Ask),
		"openPrice":          optionalPriceJSON(sample.OpenPrice),
		"highPrice":          optionalPriceJSON(sample.HighPrice),
		"lowPrice":           optionalPriceJSON(sample.LowPrice),
		"previousClosePrice": optionalPriceJSON(sample.PreviousClosePrice),
		"lastClosePrice":     optionalPriceJSON(sample.LastClosePrice),
		"volume":             sample.Volume,
		"turnover":           sample.Turnover,
		"at":                 sample.QuoteAt,
		"observedAt":         sample.ObservedAt,
		"session":            sample.Session,
		"extendedHours":      sample.ExtendedHours,
		"extended": map[string]any{
			"preMarket":   extendedMarketQuoteMap(sample.PreMarket),
			"afterMarket": extendedMarketQuoteMap(sample.AfterMarket),
			"overnight":   extendedMarketQuoteMap(sample.Overnight),
		},
	}
}

func extendedMarketQuoteMap(quote *futu.ExtendedMarketQuote) map[string]any {
	if quote == nil {
		return nil
	}
	return map[string]any{
		"price":      floatPtrToJSONNumber(quote.Price),
		"highPrice":  floatPtrToJSONNumber(quote.HighPrice),
		"lowPrice":   floatPtrToJSONNumber(quote.LowPrice),
		"volume":     quote.Volume,
		"turnover":   floatPtrToJSONNumber(quote.Turnover),
		"changeVal":  floatPtrToJSONNumber(quote.ChangeVal),
		"changeRate": quote.ChangeRate,
		"amplitude":  quote.Amplitude,
	}
}

func liveTickEventFromSample(sample *marketTickSample) map[string]any {
	if sample == nil {
		return nil
	}
	return map[string]any{
		"type":     "market-data.tick",
		"at":       sample.ObservedAt,
		"brokerId": "futu",
		"instrument": map[string]any{
			"market":       sample.Market,
			"symbol":       sample.Symbol,
			"instrumentId": sample.InstrumentID,
		},
		"snapshot": map[string]any{
			"price":              priceJSON(sample.Price),
			"bid":                priceJSON(sample.Bid),
			"ask":                priceJSON(sample.Ask),
			"openPrice":          optionalPriceJSON(sample.OpenPrice),
			"highPrice":          optionalPriceJSON(sample.HighPrice),
			"lowPrice":           optionalPriceJSON(sample.LowPrice),
			"previousClosePrice": optionalPriceJSON(sample.PreviousClosePrice),
			"lastClosePrice":     optionalPriceJSON(sample.LastClosePrice),
			"volume":             sample.Volume,
			"turnover":           sample.Turnover,
			"at":                 sample.QuoteAt,
			"observedAt":         sample.ObservedAt,
			"session":            sample.Session,
			"extendedHours":      sample.ExtendedHours,
			"extended": map[string]any{
				"preMarket":   extendedMarketQuoteMap(sample.PreMarket),
				"afterMarket": extendedMarketQuoteMap(sample.AfterMarket),
				"overnight":   extendedMarketQuoteMap(sample.Overnight),
			},
		},
		"source": sample.Source,
	}
}
