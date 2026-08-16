package akshare

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

const sourceID = "akshare"

var candlePeriodOrder = []string{"1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"}

func convertMarkets(profiles []remoteMarketProfile) ([]marketdata.MarketProfile, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("%w: markets list is empty", ErrInvalidResponse)
	}
	result := make([]marketdata.MarketProfile, 0, len(profiles))
	seen := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		converted, code, err := convertMarket(profile)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[code]; ok {
			return nil, fmt.Errorf("%w: duplicate market %s", ErrInvalidResponse, code)
		}
		seen[code] = struct{}{}
		result = append(result, converted)
	}
	return result, nil
}

func convertMarket(profile remoteMarketProfile) (marketdata.MarketProfile, string, error) {
	code, err := canonicalMarket(profile.Code)
	if err != nil || !isSupportedLeafMarket(code) {
		return nil, "", fmt.Errorf("%w: market code %q", ErrInvalidResponse, profile.Code)
	}
	resolved := strings.ToUpper(strings.TrimSpace(profile.ResolvedMarket))
	prefix := strings.ToUpper(strings.TrimSpace(profile.PreferredPrefix))
	if resolved == "" {
		resolved = code
	}
	if prefix == "" {
		prefix = code
	}
	if prefix != code || !validResolvedMarket(code, resolved) {
		return nil, "", fmt.Errorf("%w: unsupported market route %s/%s", ErrInvalidResponse, resolved, prefix)
	}
	sessions := make([]map[string]any, 0, len(profile.RegularSessions))
	for _, session := range profile.RegularSessions {
		if session.StartMinute < 0 || session.EndMinute <= session.StartMinute || session.EndMinute > 24*60 {
			return nil, "", fmt.Errorf("%w: invalid regular session", ErrInvalidResponse)
		}
		sessions = append(sessions, map[string]any{
			"startMinute": session.StartMinute, "endMinute": session.EndMinute,
			"label": strings.TrimSpace(session.Label),
		})
	}
	result := marketdata.MarketProfile{
		"code": code, "resolvedMarket": resolved, "preferredPrefix": prefix,
		"displayName":            strings.TrimSpace(profile.DisplayName),
		"quoteCurrency":          strings.ToUpper(strings.TrimSpace(profile.QuoteCurrency)),
		"timezone":               strings.TrimSpace(profile.Timezone),
		"supportsExtendedHours":  false,
		"requiresExchangePrefix": profile.RequiresExchangePrefix,
		"aliases":                append([]string(nil), profile.Aliases...), "regularSessions": sessions,
		"precision": map[string]any{"price": profile.Precision.Price, "quote": profile.Precision.Quote},
	}
	if profile.TickSize != nil {
		tickSize, err := positiveFloat("tick_size", profile.TickSize)
		if err != nil {
			return nil, "", err
		}
		result["tickSize"] = tickSize
	}
	return result, code, nil
}

func convertCandidates(entries []remoteInstrument) ([]marketdata.InstrumentCandidate, error) {
	result := make([]marketdata.InstrumentCandidate, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		identity, err := normalizeIdentity(entry.Market, entry.Symbol, entry.InstrumentID)
		if err != nil {
			return nil, fmt.Errorf("%w: search entry: %w", ErrInvalidResponse, err)
		}
		code := canonicalInstrumentCode(identity.market, entry.Code)
		if code != "" && code != identity.symbol {
			return nil, fmt.Errorf("%w: search code %q does not match %s", ErrInvalidResponse, code, identity.id)
		}
		resolved := strings.ToUpper(strings.TrimSpace(entry.ResolvedMarket))
		if resolved == "" {
			resolved = identity.market
		}
		if !validResolvedMarket(identity.market, resolved) {
			return nil, fmt.Errorf("%w: search resolved market %q", ErrInvalidResponse, resolved)
		}
		periods, err := normalizeSupportedPeriods(entry.SupportedPeriods)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[identity.id]; duplicate {
			continue
		}
		seen[identity.id] = struct{}{}
		candidate := marketdata.InstrumentCandidate{
			Market: identity.market, ResolvedMarket: resolved, InstrumentID: identity.id,
			Code: identity.symbol, Symbol: identity.symbol, Name: strings.TrimSpace(entry.Name),
			SecurityType: strings.TrimSpace(entry.SecurityType), SupportedPeriods: periods,
			Source: defaultSource(entry.Source), Selectable: entry.Selectable,
		}
		if !candidate.Selectable {
			candidate.UnavailableReason = "AKShare returned a non-selectable instrument"
		}
		result = append(result, candidate)
	}
	return result, nil
}

func convertSecurity(
	response remoteSecurity,
	expected normalizedInstrument,
	resolvedAt time.Time,
) (marketdata.SecurityDetails, error) {
	identity, err := normalizeIdentity(response.Market, response.Symbol, response.InstrumentID)
	if err != nil || identity.id != expected.id {
		return nil, fmt.Errorf("%w: security identity does not match %s", ErrInvalidResponse, expected.id)
	}
	periods, err := normalizeSupportedPeriods(response.SupportedPeriods)
	if err != nil {
		return nil, err
	}
	marketCap, err := optionalNonNegativeNumber("market_cap", response.MarketCap)
	if err != nil {
		return nil, err
	}
	averageVolume, err := optionalNonNegativeNumber("average_volume", response.AverageVolume)
	if err != nil {
		return nil, err
	}
	security := map[string]any{
		"instrumentId": identity.id, "market": identity.market, "symbol": identity.symbol,
		"name": strings.TrimSpace(response.Name), "exchange": strings.TrimSpace(response.Exchange),
		"currency":     strings.ToUpper(strings.TrimSpace(response.Currency)),
		"timezone":     strings.TrimSpace(response.Timezone),
		"securityType": strings.TrimSpace(response.SecurityType),
		"industry":     strings.TrimSpace(response.Industry), "sector": strings.TrimSpace(response.Sector),
		"marketCap": marketCap, "averageVolume": averageVolume, "supportedPeriods": periods,
	}
	// Fundamentals are optional sidecar enrichments; omit them when the
	// upstream source has no value, matching the canonical yfinance keys.
	if response.TrailingPE != nil {
		security["trailingPe"] = *response.TrailingPE
	}
	if response.SharesOutstanding != nil {
		security["sharesOutstanding"] = *response.SharesOutstanding
	}
	return marketdata.SecurityDetails{
		"request": map[string]any{
			"market": expected.market, "symbol": expected.symbol, "instrumentId": expected.id,
		},
		"security": security,
		"meta": map[string]any{
			"instrumentId": expected.id, "source": defaultSource(response.Source),
			"resolvedAt": resolvedAt.UTC().Format(time.RFC3339Nano), "fromCache": false,
		},
	}, nil
}

func convertSnapshot(
	response remoteSnapshot,
	expected normalizedInstrument,
	fallbackObservedAt time.Time,
) (*marketdata.Tick, error) {
	identity, err := normalizeIdentity(response.Market, response.Symbol, response.InstrumentID)
	if err != nil || identity.id != expected.id {
		return nil, fmt.Errorf("%w: snapshot identity does not match %s", ErrInvalidResponse, expected.id)
	}
	price, err := requiredPositiveDecimal("price", response.Price)
	if err != nil {
		return nil, err
	}
	bid, err := optionalNonNegativeDecimal("bid", response.Bid)
	if err != nil {
		return nil, err
	}
	ask, err := optionalNonNegativeDecimal("ask", response.Ask)
	if err != nil {
		return nil, err
	}
	volume, err := optionalNonNegativeDecimal("volume", response.Volume)
	if err != nil {
		return nil, err
	}
	turnover, err := optionalNonNegativeDecimal("turnover", response.Turnover)
	if err != nil {
		return nil, err
	}
	prices, err := parseSnapshotPrices(response)
	if err != nil {
		return nil, err
	}
	observedAt, err := responseTime("observed_at", response.ObservedAt, fallbackObservedAt)
	if err != nil {
		return nil, err
	}
	quoteAt := ""
	if strings.TrimSpace(response.QuoteAt) != "" {
		quoteAt, err = responseTime("quote_at", response.QuoteAt, time.Time{})
		if err != nil {
			return nil, err
		}
	}
	observedTime, _ := time.Parse(time.RFC3339Nano, observedAt)
	session := marketpkg.ClassifySession(identity.id, observedTime)
	if session != marketpkg.SessionRegular {
		session = marketpkg.SessionClosed
	}
	tick := &marketdata.Tick{
		InstrumentID: identity.id, Market: identity.market, Symbol: identity.symbol,
		Price: price, OpenPrice: prices.open, HighPrice: prices.high, LowPrice: prices.low,
		PreviousClosePrice: prices.previousClose, LastClosePrice: prices.lastClose,
		QuoteAt: quoteAt, ObservedAt: observedAt, Source: defaultSource(response.Source),
		Session: string(session), ExtendedHours: false, Kind: marketdata.TickKindQuote,
		Availability: marketdata.QuoteFieldAvailability{
			Authoritative: true, Bid: bid != nil, Ask: ask != nil,
			Volume: volume != nil, Turnover: turnover != nil,
		},
	}
	assignOptionalDecimal(&tick.Bid, bid)
	assignOptionalDecimal(&tick.Ask, ask)
	assignOptionalDecimal(&tick.Volume, volume)
	assignOptionalDecimal(&tick.Turnover, turnover)
	return tick, nil
}

type snapshotPrices struct {
	open          *decimal.Decimal
	high          *decimal.Decimal
	low           *decimal.Decimal
	previousClose *decimal.Decimal
	lastClose     *decimal.Decimal
}

func parseSnapshotPrices(response remoteSnapshot) (snapshotPrices, error) {
	openPrice, err := optionalPositiveDecimal("open_price", response.OpenPrice)
	if err != nil {
		return snapshotPrices{}, err
	}
	highPrice, err := optionalPositiveDecimal("high_price", response.HighPrice)
	if err != nil {
		return snapshotPrices{}, err
	}
	lowPrice, err := optionalPositiveDecimal("low_price", response.LowPrice)
	if err != nil {
		return snapshotPrices{}, err
	}
	if highPrice != nil && lowPrice != nil && highPrice.LessThan(*lowPrice) {
		return snapshotPrices{}, fmt.Errorf("%w: high_price is below low_price", ErrInvalidResponse)
	}
	previousClose, err := optionalPositiveDecimal("previous_close_price", response.PreviousClosePrice)
	if err != nil {
		return snapshotPrices{}, err
	}
	lastClose, err := optionalPositiveDecimal("last_close_price", response.LastClosePrice)
	if err != nil {
		return snapshotPrices{}, err
	}
	return snapshotPrices{
		open: openPrice, high: highPrice, low: lowPrice,
		previousClose: previousClose, lastClose: lastClose,
	}, nil
}

func convertCandles(
	response remoteCandles,
	expected normalizedInstrument,
	period string,
	limit int,
	resolvedAt time.Time,
) (marketdata.CandlesResponse, error) {
	return convertCandlesForSessions(
		response,
		expected,
		period,
		limit,
		[]marketdata.CandleSession{marketdata.CandleSessionRegular},
		resolvedAt,
	)
}

func convertCandlesForSessions(
	response remoteCandles,
	expected normalizedInstrument,
	period string,
	limit int,
	sessions []marketdata.CandleSession,
	resolvedAt time.Time,
) (marketdata.CandlesResponse, error) {
	identity, err := normalizeIdentity(response.Market, response.Symbol, response.InstrumentID)
	if err != nil || identity.id != expected.id || strings.TrimSpace(response.Period) != period {
		return nil, fmt.Errorf("%w: candle identity or period mismatch", ErrInvalidResponse)
	}
	if response.ExtendedHours {
		return nil, fmt.Errorf("%w: AKShare returned extended-hours candles", ErrInvalidResponse)
	}
	if response.TotalReturned != len(response.Candles) {
		return nil, fmt.Errorf("%w: candle count mismatch", ErrInvalidResponse)
	}
	candles := make([]map[string]any, 0, len(response.Candles))
	for index, candle := range response.Candles {
		converted, err := convertCandle(candle, period)
		if err != nil {
			return nil, fmt.Errorf("candle %d: %w", index, err)
		}
		candles = append(candles, converted)
	}
	pagination, err := validateCandlePagination(response, candles, limit)
	if err != nil {
		return nil, err
	}
	return marketdata.CandlesResponseDTO{
		Instrument: marketdata.InstrumentDTO{
			Market: expected.market, Symbol: expected.symbol, InstrumentID: expected.id,
		},
		Period: period, Limit: limit, Candles: candles, Pagination: pagination, Source: defaultSource(response.Source),
		ResolvedAt: resolvedAt.UTC().Format(time.RFC3339Nano), FromCache: false,
		ExtendedHours: false, IncludeSession: false,
		Sessions: sessions,
	}.JSON(), nil
}

func validateCandlePagination(
	response remoteCandles,
	candles []map[string]any,
	limit int,
) (marketdata.CandlePagination, error) {
	if response.HasMore == nil {
		return marketdata.CandlePagination{}, fmt.Errorf("%w: candle pagination has_more is required", ErrInvalidResponse)
	}
	if err := validateCandleSequence(candles); err != nil {
		return marketdata.CandlePagination{}, err
	}
	if limit > 0 && len(candles) > limit {
		return marketdata.CandlePagination{}, fmt.Errorf("%w: candle page exceeds the requested limit", ErrInvalidResponse)
	}
	if !*response.HasMore {
		if strings.TrimSpace(response.NextBefore) != "" {
			return marketdata.CandlePagination{}, fmt.Errorf("%w: terminal candle page has next_before", ErrInvalidResponse)
		}
		return marketdata.CandlePagination{}, nil
	}
	if len(candles) == 0 {
		return marketdata.CandlePagination{}, fmt.Errorf("%w: invalid paged candle count", ErrInvalidResponse)
	}
	nextBefore, err := responseTime("next_before", response.NextBefore, time.Time{})
	if err != nil {
		return marketdata.CandlePagination{}, err
	}
	earliest, ok := candles[0]["at"].(string)
	if !ok || earliest != nextBefore {
		return marketdata.CandlePagination{}, fmt.Errorf("%w: next_before must equal earliest candle", ErrInvalidResponse)
	}
	return marketdata.CandlePagination{HasMore: true, NextBefore: nextBefore}, nil
}

func validateCandleSequence(candles []map[string]any) error {
	var previous time.Time
	for index, candle := range candles {
		at, ok := candle["at"].(string)
		if !ok {
			return fmt.Errorf("%w: candle %d timestamp is missing", ErrInvalidResponse, index)
		}
		parsed, err := time.Parse(time.RFC3339Nano, at)
		if err != nil || (!previous.IsZero() && !previous.Before(parsed)) {
			return fmt.Errorf("%w: candles are not strictly ordered", ErrInvalidResponse)
		}
		previous = parsed
	}
	return nil
}

func validateHistoricalCandleCursor(
	response marketdata.CandlesResponse,
	beforeTime string,
) (marketdata.CandlesResponse, error) {
	beforeTime = strings.TrimSpace(beforeTime)
	if beforeTime == "" {
		return response, nil
	}
	before, err := time.Parse(time.RFC3339Nano, beforeTime)
	if err != nil {
		return nil, fmt.Errorf("%w: before cursor must be RFC3339", ErrInvalidResponse)
	}
	candles, ok := response["candles"].([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: candle response is malformed", ErrInvalidResponse)
	}
	for index, candle := range candles {
		at, ok := candle["at"].(string)
		if !ok {
			return nil, fmt.Errorf("%w: candle %d timestamp is missing", ErrInvalidResponse, index)
		}
		parsed, err := time.Parse(time.RFC3339Nano, at)
		if err != nil || !parsed.Before(before) {
			return nil, fmt.Errorf("%w: candle page violates before cursor", ErrInvalidResponse)
		}
	}
	return response, nil
}

func validateHistoricalCandleResponse(
	response marketdata.CandlesResponse,
	beforeTime string,
	fromTime string,
	toTime string,
) (marketdata.CandlesResponse, error) {
	response, err := validateHistoricalCandleCursor(response, beforeTime)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(fromTime) == "" && strings.TrimSpace(toTime) == "" {
		return response, nil
	}
	pagination, ok := response["pagination"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: bounded candle response is missing pagination", ErrInvalidResponse)
	}
	hasMore, ok := pagination["hasMore"].(bool)
	if !ok || hasMore {
		return nil, fmt.Errorf("%w: bounded candle response cannot continue pagination", ErrInvalidResponse)
	}
	if nextBefore, exists := pagination["nextBefore"]; exists {
		value, valid := nextBefore.(string)
		if !valid || strings.TrimSpace(value) != "" {
			return nil, fmt.Errorf("%w: bounded candle response contains nextBefore", ErrInvalidResponse)
		}
	}
	return response, nil
}

func convertCandle(candle remoteCandle, period string) (map[string]any, error) {
	at, err := responseTime("at", candle.At, time.Time{})
	if err != nil {
		return nil, err
	}
	values := make(map[string]decimal.Decimal, 4)
	for name, value := range map[string]*json.Number{
		"open": candle.Open, "high": candle.High, "low": candle.Low, "close": candle.Close,
	} {
		converted, err := requiredPositiveDecimal(name, value)
		if err != nil {
			return nil, err
		}
		values[name] = converted
	}
	if values["high"].LessThan(values["low"]) || values["high"].LessThan(values["open"]) ||
		values["high"].LessThan(values["close"]) || values["low"].GreaterThan(values["open"]) ||
		values["low"].GreaterThan(values["close"]) {
		return nil, fmt.Errorf("%w: invalid OHLC bounds", ErrInvalidResponse)
	}
	volume, err := optionalNonNegativeDecimal("volume", candle.Volume)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"period": period, "at": at, "open": values["open"].String(),
		"high": values["high"].String(), "low": values["low"].String(),
		"close": values["close"].String(), "volume": nil, "session": nil,
	}
	if volume != nil {
		result["volume"] = volume.String()
	}
	return result, nil
}

func normalizeSupportedPeriods(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		period := strings.ToLower(strings.TrimSpace(value))
		if !supportedCandlePeriod(period) {
			return nil, fmt.Errorf("%w: unsupported candle period %q", ErrInvalidResponse, value)
		}
		seen[period] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for _, period := range candlePeriodOrder {
		if _, ok := seen[period]; ok {
			result = append(result, period)
		}
	}
	return result, nil
}

func supportedCandlePeriod(period string) bool {
	return slices.Contains(candlePeriodOrder, period)
}

func requiredPositiveDecimal(field string, value *json.Number) (decimal.Decimal, error) {
	if value == nil {
		return decimal.Zero, fmt.Errorf("%w: %s is missing", ErrInvalidResponse, field)
	}
	parsed, err := decimal.NewFromString(value.String())
	if err != nil || !parsed.GreaterThan(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("%w: %s must be positive", ErrInvalidResponse, field)
	}
	return parsed, nil
}

func optionalPositiveDecimal(field string, value *json.Number) (*decimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := requiredPositiveDecimal(field, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func optionalNonNegativeDecimal(field string, value *json.Number) (*decimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := decimal.NewFromString(value.String())
	if err != nil || parsed.IsNegative() {
		return nil, fmt.Errorf("%w: %s must be non-negative", ErrInvalidResponse, field)
	}
	return &parsed, nil
}

func optionalNonNegativeNumber(field string, value *json.Number) (*json.Number, error) {
	if value == nil {
		return nil, nil
	}
	if _, err := optionalNonNegativeDecimal(field, value); err != nil {
		return nil, err
	}
	return value, nil
}

func positiveFloat(field string, value *json.Number) (float64, error) {
	if value == nil {
		return 0, fmt.Errorf("%w: %s is missing", ErrInvalidResponse, field)
	}
	parsed, err := value.Float64()
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("%w: %s must be a finite positive number", ErrInvalidResponse, field)
	}
	return parsed, nil
}

func responseTime(field, value string, fallback time.Time) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" && !fallback.IsZero() {
		return fallback.UTC().Format(time.RFC3339Nano), nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", fmt.Errorf("%w: %s must be RFC3339", ErrInvalidResponse, field)
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func defaultSource(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return sourceID
}

func assignOptionalDecimal(target *decimal.Decimal, value *decimal.Decimal) {
	if value != nil {
		*target = *value
	}
}
