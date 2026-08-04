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
		"bid":                availableDecimalString(sample.Bid, sample.Availability, quoteFieldBid),
		"ask":                availableDecimalString(sample.Ask, sample.Availability, quoteFieldAsk),
		"openPrice":          optionalPriceString(sample.OpenPrice),
		"highPrice":          optionalPriceString(sample.HighPrice),
		"lowPrice":           optionalPriceString(sample.LowPrice),
		"previousClosePrice": optionalPriceString(sample.PreviousClosePrice),
		"lastClosePrice":     optionalPriceString(sample.LastClosePrice),
		"volume":             availableDecimalString(sample.Volume, sample.Availability, quoteFieldVolume),
		"turnover":           availableDecimalString(sample.Turnover, sample.Availability, quoteFieldTurnover),
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
		BrokerID:         brokerIDFromTickSource(cloned.Source),
		Source:           cloned.Source,
		CumulativeVolume: availableDecimalPointer(cloned.Volume, cloned.Availability, quoteFieldVolume),
		VolumeDelta:      cloned.VolumeDelta,
	}.JSON()
}

func brokerIDFromTickSource(source string) string {
	normalized := strings.ToLower(strings.TrimSpace(source))
	switch {
	case normalized == "akshare" || strings.HasPrefix(normalized, "akshare:"):
		return "akshare"
	case normalized == "yfinance" || strings.HasPrefix(normalized, "yfinance:"):
		return "yfinance"
	default:
		return "futu"
	}
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
			"bid":              availableDecimalString(sample.Bid, sample.Availability, quoteFieldBid),
			"ask":              availableDecimalString(sample.Ask, sample.Availability, quoteFieldAsk),
			"volume":           availableDecimalString(sample.Volume, sample.Availability, quoteFieldVolume),
			"cumulativeVolume": availableDecimalString(sample.Volume, sample.Availability, quoteFieldVolume),
			"volumeDelta":      sample.VolumeDelta.String(),
			"observedAt":       sample.ObservedAt,
			"session":          sample.Session,
			"extendedHours":    sample.ExtendedHours,
		})
	}
	return TicksResponse{"ticks": ticks, "totalReturned": len(ticks)}
}

type quoteField uint8

const (
	quoteFieldBid quoteField = iota
	quoteFieldAsk
	quoteFieldVolume
	quoteFieldTurnover
)

func availableDecimalString(
	value decimal.Decimal,
	availability QuoteFieldAvailability,
	field quoteField,
) any {
	pointer := availableDecimalPointer(value, availability, field)
	if pointer == nil {
		return nil
	}
	return pointer.String()
}

func availableDecimalPointer(
	value decimal.Decimal,
	availability QuoteFieldAvailability,
	field quoteField,
) *decimal.Decimal {
	if availability.Authoritative && !quoteFieldAvailable(availability, field) {
		return nil
	}
	return new(value)
}

func quoteFieldAvailable(availability QuoteFieldAvailability, field quoteField) bool {
	switch field {
	case quoteFieldBid:
		return availability.Bid
	case quoteFieldAsk:
		return availability.Ask
	case quoteFieldVolume:
		return availability.Volume
	case quoteFieldTurnover:
		return availability.Turnover
	default:
		return false
	}
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
	result := map[string]any{
		"price":      optionalPriceString(quote.Price),
		"highPrice":  optionalPriceString(quote.HighPrice),
		"lowPrice":   optionalPriceString(quote.LowPrice),
		"volume":     optionalDecimalString(quote.Volume),
		"turnover":   optionalPriceString(quote.Turnover),
		"changeVal":  optionalPriceString(quote.ChangeVal),
		"changeRate": optionalPriceString(quote.ChangeRate),
		"amplitude":  optionalPriceString(quote.Amplitude),
		"quoteTime":  strings.TrimSpace(quote.QuoteTime),
	}
	for key, value := range map[string]string{
		"tradingDate": quote.TradingDate, "exchangeTimezone": quote.ExchangeTimezone,
		"sessionStartAt": quote.SessionStartAt, "sessionEndAt": quote.SessionEndAt,
	} {
		if normalized := strings.TrimSpace(value); normalized != "" {
			result[key] = normalized
		}
	}
	return result
}

func optionalDecimalString(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}
	return value.String()
}
