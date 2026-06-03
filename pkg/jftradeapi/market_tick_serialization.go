package jftradeapi

import (
	"strings"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

func priceString(d decimal.Decimal) string {
	return d.String()
}

func optionalPriceString(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return d.String()
}

func snapshotMapFromSample(sample *marketTickSample) map[string]any {
	return map[string]any{
		"price":              priceString(sample.Price),
		"bid":                priceString(sample.Bid),
		"ask":                priceString(sample.Ask),
		"openPrice":          optionalPriceString(sample.OpenPrice),
		"highPrice":          optionalPriceString(sample.HighPrice),
		"lowPrice":           optionalPriceString(sample.LowPrice),
		"previousClosePrice": optionalPriceString(sample.PreviousClosePrice),
		"lastClosePrice":     optionalPriceString(sample.LastClosePrice),
		"volume":             sample.Volume,
		"turnover":           priceString(sample.Turnover),
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

func cloneTickSampleWithObservedAt(sample *marketTickSample, observedAt string) *marketTickSample {
	if sample == nil || strings.TrimSpace(observedAt) == "" {
		return sample
	}
	cloned := *sample
	cloned.ObservedAt = observedAt
	return &cloned
}

func extendedMarketQuoteMap(quote *futu.ExtendedMarketQuote) map[string]any {
	if quote == nil {
		return nil
	}
	return map[string]any{
		"price":      optionalPriceString(quote.Price),
		"highPrice":  optionalPriceString(quote.HighPrice),
		"lowPrice":   optionalPriceString(quote.LowPrice),
		"volume":     quote.Volume,
		"turnover":   optionalPriceString(quote.Turnover),
		"changeVal":  optionalPriceString(quote.ChangeVal),
		"changeRate": optionalPriceString(quote.ChangeRate),
		"amplitude":  optionalPriceString(quote.Amplitude),
	}
}

func liveTickEventFromSample(sample *marketTickSample) map[string]any {
	return liveTickEventFromSampleAt(sample, "")
}

func liveTickEventFromSampleAt(sample *marketTickSample, observedAt string) map[string]any {
	if sample == nil {
		return nil
	}
	sample = cloneTickSampleWithObservedAt(sample, observedAt)
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
			"price":              priceString(sample.Price),
			"bid":                priceString(sample.Bid),
			"ask":                priceString(sample.Ask),
			"openPrice":          optionalPriceString(sample.OpenPrice),
			"highPrice":          optionalPriceString(sample.HighPrice),
			"lowPrice":           optionalPriceString(sample.LowPrice),
			"previousClosePrice": optionalPriceString(sample.PreviousClosePrice),
			"lastClosePrice":     optionalPriceString(sample.LastClosePrice),
			"volume":             sample.Volume,
			"turnover":           priceString(sample.Turnover),
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
