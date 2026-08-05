package yfinance

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/market"
)

const sourceID = "yfinance"

func convertMarkets(profiles []remoteMarketProfile) ([]marketdata.MarketProfile, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("%w: markets list is empty", ErrInvalidResponse)
	}
	result := make([]marketdata.MarketProfile, 0, len(profiles))
	for _, profile := range profiles {
		converted, err := convertMarket(profile)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func convertMarket(profile remoteMarketProfile) (marketdata.MarketProfile, error) {
	code, err := canonicalMarket(profile.Code)
	if err != nil || code == "" {
		return nil, fmt.Errorf("%w: market code %q", ErrInvalidResponse, profile.Code)
	}
	resolved := strings.ToUpper(strings.TrimSpace(profile.ResolvedMarket))
	prefix := strings.ToUpper(strings.TrimSpace(profile.PreferredPrefix))
	if resolved == "" {
		resolved = code
	}
	if prefix == "" {
		prefix = code
	}
	if !isSupportedLeafMarket(code) || prefix != code || !validResolvedMarket(code, resolved) {
		return nil, fmt.Errorf("%w: unsupported market route %s/%s", ErrInvalidResponse, resolved, prefix)
	}
	sessions := make([]map[string]any, 0, len(profile.RegularSessions))
	for _, session := range profile.RegularSessions {
		if session.StartMinute < 0 || session.EndMinute <= session.StartMinute || session.EndMinute > 24*60 {
			return nil, fmt.Errorf("%w: invalid regular session", ErrInvalidResponse)
		}
		sessions = append(sessions, map[string]any{
			"startMinute": session.StartMinute,
			"endMinute":   session.EndMinute,
			"label":       strings.TrimSpace(session.Label),
		})
	}
	tickSize, err := positiveFloat("tick_size", profile.TickSize)
	if err != nil {
		return nil, err
	}
	return marketdata.MarketProfile{
		"code":                   code,
		"resolvedMarket":         resolved,
		"preferredPrefix":        prefix,
		"displayName":            strings.TrimSpace(profile.DisplayName),
		"quoteCurrency":          strings.ToUpper(strings.TrimSpace(profile.QuoteCurrency)),
		"timezone":               strings.TrimSpace(profile.Timezone),
		"supportsExtendedHours":  profile.SupportsExtendedHours,
		"requiresExchangePrefix": profile.RequiresExchangePrefix,
		"aliases":                append([]string(nil), profile.Aliases...),
		"regularSessions":        sessions,
		"precision": map[string]any{
			"price": profile.Precision.Price,
			"quote": profile.Precision.Quote,
		},
		"tickSize": tickSize,
	}, nil
}

func isSupportedLeafMarket(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "US", "HK", "SH", "SZ":
		return true
	default:
		return false
	}
}

func validResolvedMarket(leaf, resolved string) bool {
	leaf = strings.ToUpper(strings.TrimSpace(leaf))
	resolved = strings.ToUpper(strings.TrimSpace(resolved))
	if leaf == "SH" || leaf == "SZ" {
		return resolved == "CN" || resolved == leaf
	}
	return resolved == leaf
}

func convertCandidates(entries []remoteInstrument) ([]marketdata.InstrumentCandidate, error) {
	result := make([]marketdata.InstrumentCandidate, 0, len(entries))
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
		source := strings.TrimSpace(entry.Source)
		if source == "" {
			source = sourceID
		}
		candidate := marketdata.InstrumentCandidate{
			Market:           identity.market,
			ResolvedMarket:   resolved,
			InstrumentID:     identity.id,
			Code:             identity.symbol,
			Symbol:           identity.symbol,
			Name:             strings.TrimSpace(entry.Name),
			SecurityType:     strings.TrimSpace(entry.SecurityType),
			SupportedPeriods: append([]string(nil), entry.SupportedPeriods...),
			Source:           source,
			Selectable:       entry.Selectable,
		}
		if !candidate.Selectable {
			candidate.UnavailableReason = "Yahoo Finance returned a non-selectable instrument"
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
	dividendYield, err := yahooDividendYieldPercent(response.DividendYield)
	if err != nil {
		return nil, err
	}
	source := strings.TrimSpace(response.Source)
	if source == "" {
		source = sourceID
	}
	security := map[string]any{
		"instrumentId": identity.id, "market": identity.market, "symbol": identity.symbol,
		"name": strings.TrimSpace(response.Name), "exchange": strings.TrimSpace(response.Exchange),
		"currency": strings.ToUpper(strings.TrimSpace(response.Currency)), "timezone": strings.TrimSpace(response.Timezone),
		"securityType": strings.TrimSpace(response.SecurityType), "industry": strings.TrimSpace(response.Industry),
		"sector": strings.TrimSpace(response.Sector), "website": strings.TrimSpace(response.Website),
		"businessSummary": response.BusinessSummary, "marketCap": response.MarketCap,
		"trailingPe": response.TrailingPE, "forwardPe": response.ForwardPE,
		"trailingEps": response.TrailingEPS, "forwardEps": response.ForwardEPS,
		"dividendRate": response.DividendRate, "dividendYield": dividendYield,
		"fiftyTwoWeekHigh": response.FiftyTwoWeekHigh, "fiftyTwoWeekLow": response.FiftyTwoWeekLow,
		"averageVolume": response.AverageVolume, "sharesOutstanding": response.SharesOutstanding,
	}
	if len(response.SupportedPeriods) > 0 {
		security["supportedPeriods"] = append([]string(nil), response.SupportedPeriods...)
	}
	return marketdata.SecurityDetails{
		"request": map[string]any{
			"market": expected.market, "symbol": expected.symbol, "instrumentId": expected.id,
		},
		"security": security,
		"meta": map[string]any{
			"instrumentId": expected.id, "source": source,
			"resolvedAt": resolvedAt.UTC().Format(time.RFC3339Nano), "fromCache": false,
		},
	}, nil
}

// yahooDividendYieldPercent adapts Yahoo's fractional yield (0.004 for
// 0.40%) to the percentage convention used by the broker-neutral UI fields.
func yahooDividendYieldPercent(value *json.Number) (*float64, error) {
	if value == nil {
		return nil, nil
	}
	ratio, err := nonNegativeFloat("dividend_yield", value)
	if err != nil {
		return nil, err
	}
	percent := ratio * 100
	return &percent, nil
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
	values, err := parseSnapshotValues(response, fallbackObservedAt)
	if err != nil {
		return nil, err
	}
	observedAt, err := time.Parse(time.RFC3339Nano, values.observedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: observed_at must be RFC3339", ErrInvalidResponse)
	}
	values.session = yahooSessionAt(identity.id, observedAt)
	values.preMarket = normalizeYahooExtendedQuote(identity.id, values.preMarket, market.SessionPre)
	values.afterMarket = normalizeYahooExtendedQuote(identity.id, values.afterMarket, market.SessionAfter)
	values = retainRelevantYahooExtendedQuotes(identity.id, values, observedAt)
	values.previousClose, values.lastClose = snapshotClosePrices(
		response.Market, response, values.session, values.regularQuote,
	)
	values = selectYahooActiveQuote(values)
	return &marketdata.Tick{
		InstrumentID: identity.id, Market: identity.market, Symbol: identity.symbol,
		Price: values.price, Bid: values.bid, Ask: values.ask,
		OpenPrice: optionalDecimal(response.OpenPrice), HighPrice: optionalDecimal(response.HighPrice),
		LowPrice: optionalDecimal(response.LowPrice), PreviousClosePrice: values.previousClose,
		LastClosePrice: values.lastClose, Volume: values.volume, Turnover: values.turnover,
		QuoteAt: values.quoteAt, ObservedAt: values.observedAt, Source: values.source, Session: values.session,
		ExtendedHours: values.session == "pre" || values.session == "after",
		PreMarket:     values.preMarket, AfterMarket: values.afterMarket,
		Kind: marketdata.TickKindQuote,
	}, nil
}

type snapshotValues struct {
	price, bid, ask          decimal.Decimal
	volume                   decimal.Decimal
	turnover                 decimal.Decimal
	quoteAt, observedAt      string
	source, session          string
	previousClose, lastClose *decimal.Decimal
	regularQuote             *marketdata.ExtendedQuote
	preMarket, afterMarket   *marketdata.ExtendedQuote
}

func parseSnapshotValues(response remoteSnapshot, fallbackObservedAt time.Time) (snapshotValues, error) {
	price, err := requiredPositiveDecimal("price", response.Price)
	if err != nil {
		return snapshotValues{}, err
	}
	bid, err := quoteDecimal("bid", response.Bid, price)
	if err != nil {
		return snapshotValues{}, err
	}
	ask, err := quoteDecimal("ask", response.Ask, price)
	if err != nil {
		return snapshotValues{}, err
	}
	observedAt, quoteAt, err := snapshotTimes(response, fallbackObservedAt)
	if err != nil {
		return snapshotValues{}, err
	}
	volume, err := nonNegativeDecimal("volume", response.Volume)
	if err != nil {
		return snapshotValues{}, err
	}
	turnover, err := nonNegativeDecimal("turnover", response.Turnover)
	if err != nil {
		return snapshotValues{}, err
	}
	preMarket, err := convertSnapshotQuote("pre_market_quote", response.PreMarketQuote)
	if err != nil {
		return snapshotValues{}, err
	}
	afterMarket, err := convertSnapshotQuote("after_market_quote", response.AfterMarketQuote)
	if err != nil {
		return snapshotValues{}, err
	}
	regularQuote, err := convertSnapshotQuote("regular_quote", response.RegularQuote)
	if err != nil {
		return snapshotValues{}, err
	}
	source := strings.TrimSpace(response.Source)
	if source == "" {
		source = sourceID
	}
	return snapshotValues{
		price: price, bid: bid, ask: ask, volume: volume, turnover: turnover,
		quoteAt: quoteAt, observedAt: observedAt, source: source,
		regularQuote: regularQuote, preMarket: preMarket, afterMarket: afterMarket,
	}, nil
}

func snapshotTimes(response remoteSnapshot, fallback time.Time) (string, string, error) {
	observedAt, err := responseTime("observed_at", response.ObservedAt, fallback)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(response.QuoteAt) == "" {
		// quoteAt is the upstream quote timestamp, while observedAt is the
		// local fetch timestamp. Do not make a missing upstream timestamp look
		// like a market quote received at the time of polling.
		return observedAt, "", nil
	}
	quoteAt, err := responseTime("quote_at", response.QuoteAt, time.Time{})
	return observedAt, quoteAt, err
}

func snapshotClosePrices(
	market string,
	response remoteSnapshot,
	session string,
	regularQuote *marketdata.ExtendedQuote,
) (*decimal.Decimal, *decimal.Decimal) {
	previousClose := optionalDecimal(response.PreviousClosePrice)
	lastClose := optionalDecimal(response.LastClosePrice)
	if lastClose == nil {
		lastClose = optionalDecimal(response.PreviousClosePrice)
	}
	if strings.EqualFold(strings.TrimSpace(market), "US") &&
		(session == "pre" || session == "after" || session == "closed") &&
		regularQuote != nil && regularQuote.Price != nil {
		previousClose = regularQuote.Price
	}
	return previousClose, lastClose
}

func convertSnapshotQuote(field string, quote *remoteSnapshotQuote) (*marketdata.ExtendedQuote, error) {
	if quote == nil {
		return nil, nil
	}
	price, err := optionalNonNegativeDecimal(field+".price", quote.Price)
	if err != nil {
		return nil, err
	}
	high, err := optionalNonNegativeDecimal(field+".high_price", quote.HighPrice)
	if err != nil {
		return nil, err
	}
	low, err := optionalNonNegativeDecimal(field+".low_price", quote.LowPrice)
	if err != nil {
		return nil, err
	}
	volume, err := optionalNonNegativeDecimal(field+".volume", quote.Volume)
	if err != nil {
		return nil, err
	}
	turnover, err := optionalNonNegativeDecimal(field+".turnover", quote.Turnover)
	if err != nil {
		return nil, err
	}
	changeValue, err := optionalSignedDecimal(field+".change_value", quote.ChangeValue)
	if err != nil {
		return nil, err
	}
	changeRate, err := optionalSignedDecimal(field+".change_rate", quote.ChangeRate)
	if err != nil {
		return nil, err
	}
	quoteAt := ""
	if strings.TrimSpace(quote.QuoteAt) != "" {
		quoteAt, err = responseTime(field+".quote_at", quote.QuoteAt, time.Time{})
		if err != nil {
			return nil, err
		}
	}
	return &marketdata.ExtendedQuote{
		Price: price, HighPrice: high, LowPrice: low, Volume: volume,
		Turnover: turnover, ChangeVal: changeValue, ChangeRate: changeRate, QuoteTime: quoteAt,
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
		[]marketdata.CandleSession{
			marketdata.CandleSessionRegular,
			marketdata.CandleSessionExtended,
			marketdata.CandleSessionOvernight,
		},
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
	if response.TotalReturned != len(response.Candles) {
		return nil, fmt.Errorf("%w: candle count mismatch", ErrInvalidResponse)
	}
	candles := make([]map[string]any, 0, len(response.Candles))
	includeSession := false
	for index, candle := range response.Candles {
		converted, keep, err := convertCandle(candle, expected.id, period)
		if err != nil {
			return nil, fmt.Errorf("candle %d: %w", index, err)
		}
		if !keep {
			continue
		}
		sessionGroup, err := convertedCandleSessionGroup(converted)
		if err != nil {
			return nil, fmt.Errorf("candle %d: %w", index, err)
		}
		if !marketdata.ContainsCandleSession(sessions, sessionGroup) {
			continue
		}
		if converted["session"] != nil {
			includeSession = true
		}
		candles = append(candles, converted)
	}
	if limit > 0 && len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	source := strings.TrimSpace(response.Source)
	if source == "" {
		source = sourceID
	}
	return marketdata.CandlesResponseDTO{
		Instrument: marketdata.InstrumentDTO{
			Market: expected.market, Symbol: expected.symbol, InstrumentID: expected.id,
		},
		Period: period, Limit: limit, Candles: candles, Source: source,
		ResolvedAt: resolvedAt.UTC().Format(time.RFC3339Nano), FromCache: false,
		ExtendedHours: response.ExtendedHours && includeSession, IncludeSession: includeSession,
		Sessions: sessions,
	}.JSON(), nil
}

func convertedCandleSessionGroup(candle map[string]any) (marketdata.CandleSession, error) {
	group := marketdata.CandleSessionRegular
	if label, ok := candle["session"].(string); ok {
		group = marketdata.CandleSessionForLabel(label)
		if group == "" {
			return "", fmt.Errorf("%w: unknown session %q", ErrInvalidResponse, label)
		}
	}
	return group, nil
}

func convertCandle(candle remoteCandle, instrumentID string, period string) (map[string]any, bool, error) {
	at, err := responseTime("at", candle.At, time.Time{})
	if err != nil {
		return nil, false, err
	}
	session, keep, err := yahooCandleSession(instrumentID, at, period)
	if err != nil || !keep {
		return nil, keep, err
	}
	result := map[string]any{"period": period, "at": at}
	for name, value := range map[string]*json.Number{
		"open": candle.Open, "high": candle.High, "low": candle.Low, "close": candle.Close,
	} {
		converted, err := requiredPositiveDecimal(name, value)
		if err != nil {
			return nil, false, err
		}
		result[name] = converted.String()
	}
	volume, err := yahooCandleVolume(candle.Volume, session)
	if err != nil {
		return nil, false, err
	}
	result["volume"] = volume
	if session == "" {
		result["session"] = nil
	} else {
		result["session"] = session
	}
	return result, true, nil
}

func yahooCandleVolume(value *json.Number, session string) (any, error) {
	// Yahoo's US extended-hours rows are price samples, not trustworthy
	// per-bar volume observations. They commonly carry zero in pre-market and
	// can carry a session-cumulative snapshot in one after-market minute.
	if session == string(market.SessionPre) || session == string(market.SessionAfter) {
		return nil, nil
	}
	volume, err := nonNegativeDecimal("volume", value)
	if err != nil {
		return nil, err
	}
	return volume.String(), nil
}

func yahooCandleSession(instrumentID string, atValue string, period string) (string, bool, error) {
	if period == "1d" || period == "1w" || period == "1mo" {
		return "", true, nil
	}
	at, err := time.Parse(time.RFC3339Nano, atValue)
	if err != nil {
		return "", false, fmt.Errorf("%w: at must be RFC3339", ErrInvalidResponse)
	}
	session := market.ClassifySession(instrumentID, at)
	if session == market.SessionClosed || session == market.SessionUnknown || session == market.SessionOvernight {
		return string(session), false, nil
	}
	return string(session), true, nil
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

func quoteDecimal(field string, value *json.Number, fallback decimal.Decimal) (decimal.Decimal, error) {
	if value == nil {
		return fallback, nil
	}
	parsed, err := decimal.NewFromString(value.String())
	if err != nil || parsed.IsNegative() {
		return decimal.Zero, fmt.Errorf("%w: %s must be non-negative", ErrInvalidResponse, field)
	}
	if parsed.IsZero() {
		return fallback, nil
	}
	return parsed, nil
}

func optionalDecimal(value *json.Number) *decimal.Decimal {
	if value == nil {
		return nil
	}
	parsed, err := decimal.NewFromString(value.String())
	if err != nil {
		return nil
	}
	return &parsed
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

func optionalSignedDecimal(field string, value *json.Number) (*decimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := decimal.NewFromString(value.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be a decimal", ErrInvalidResponse, field)
	}
	return &parsed, nil
}

func nonNegativeDecimal(field string, value *json.Number) (decimal.Decimal, error) {
	if value == nil {
		return decimal.Zero, nil
	}
	parsed, err := decimal.NewFromString(value.String())
	if err != nil || parsed.IsNegative() {
		return decimal.Zero, fmt.Errorf("%w: %s must be non-negative", ErrInvalidResponse, field)
	}
	return parsed, nil
}

func nonNegativeFloat(field string, value *json.Number) (float64, error) {
	if value == nil {
		return 0, nil
	}
	parsed, err := value.Float64()
	if err != nil || parsed < 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("%w: %s must be a finite non-negative number", ErrInvalidResponse, field)
	}
	return parsed, nil
}

func positiveFloat(field string, value json.Number) (float64, error) {
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
