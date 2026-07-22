package marketdata

import (
	"strings"

	"github.com/shopspring/decimal"
)

func SnapshotJSON(sample *Tick) map[string]any {
	if sample == nil {
		return nil
	}
	return map[string]any{
		"price":              sample.Price.String(),
		"bid":                sample.Bid.String(),
		"ask":                sample.Ask.String(),
		"openPrice":          optionalPriceString(sample.OpenPrice),
		"highPrice":          optionalPriceString(sample.HighPrice),
		"lowPrice":           optionalPriceString(sample.LowPrice),
		"previousClosePrice": optionalPriceString(sample.PreviousClosePrice),
		"lastClosePrice":     optionalPriceString(sample.LastClosePrice),
		"volume":             sample.Volume,
		"turnover":           sample.Turnover.String(),
		"at":                 sample.QuoteAt,
		"observedAt":         sample.ObservedAt,
		"session":            sample.Session,
		"extendedHours":      sample.ExtendedHours,
		"extended": map[string]any{
			"preMarket":   extendedQuoteJSON(sample.PreMarket),
			"afterMarket": extendedQuoteJSON(sample.AfterMarket),
			"overnight":   extendedQuoteJSON(sample.Overnight),
		},
	}
}

func LiveTickJSON(sample *Tick, observedAt string) map[string]any {
	if sample == nil {
		return nil
	}
	cloned := *sample
	if strings.TrimSpace(observedAt) != "" {
		cloned.ObservedAt = observedAt
	}
	return TickEventDTO{
		Instrument: InstrumentDTO{
			Market:       cloned.Market,
			Symbol:       cloned.Symbol,
			InstrumentID: cloned.InstrumentID,
		},
		Snapshot:         SnapshotJSON(&cloned),
		ObservedAt:       cloned.ObservedAt,
		BrokerID:         "futu",
		Source:           cloned.Source,
		CumulativeVolume: cloned.Volume,
		VolumeDelta:      cloned.VolumeDelta,
	}.JSON()
}

func LatestTicksJSON(samples []*Tick) TicksResponse {
	ticks := make([]map[string]any, 0, len(samples))
	for _, sample := range samples {
		if sample == nil {
			continue
		}
		ticks = append(ticks, map[string]any{
			"instrumentId":     sample.InstrumentID,
			"market":           sample.Market,
			"symbol":           sample.Symbol,
			"price":            sample.Price.String(),
			"bid":              sample.Bid.String(),
			"ask":              sample.Ask.String(),
			"volume":           sample.Volume,
			"cumulativeVolume": sample.Volume,
			"volumeDelta":      sample.VolumeDelta,
			"observedAt":       sample.ObservedAt,
			"session":          sample.Session,
			"extendedHours":    sample.ExtendedHours,
		})
	}
	return TicksResponse{"ticks": ticks, "totalReturned": len(ticks)}
}

func optionalPriceString(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func extendedQuoteJSON(quote *ExtendedQuote) any {
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
		"quoteTime":  strings.TrimSpace(quote.QuoteTime),
	}
}
