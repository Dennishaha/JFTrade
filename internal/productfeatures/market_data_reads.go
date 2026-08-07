package productfeatures

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

// ReadMarketSnapshot adapts an explicitly selected broker snapshot to the
// stable workspace market-data response shape.
func (s *Service) ReadMarketSnapshot(
	ctx context.Context,
	brokerID string,
	market string,
	symbol string,
	refresh bool,
) (map[string]any, error) {
	market, symbol, instrumentID, err := normalizeWorkspaceInstrument(market, symbol)
	if err != nil {
		return nil, err
	}
	result, err := s.BatchSnapshots(ctx, broker.FeatureQuery{
		BrokerID:  brokerID,
		Market:    market,
		FeatureID: broker.FeatureMarketSnapshot,
		Params: map[string]any{
			"refresh": refresh,
		},
	}, []string{instrumentID})
	if err != nil {
		return nil, err
	}
	var entry map[string]any
	if len(result.Entries) > 0 {
		entry = result.Entries[0]
	}
	return map[string]any{
		"request":  workspaceInstrumentRequest(market, symbol, instrumentID),
		"snapshot": workspaceSnapshot(entry, result.AsOf),
		"meta":     workspaceProviderMeta(result, instrumentID, false),
	}, nil
}

// ReadMarketSecurityDetails adapts broker-neutral profile data to the stable
// workspace security-details response shape.
func (s *Service) ReadMarketSecurityDetails(
	ctx context.Context,
	brokerID string,
	market string,
	symbol string,
) (map[string]any, error) {
	market, symbol, instrumentID, err := normalizeWorkspaceInstrument(market, symbol)
	if err != nil {
		return nil, err
	}
	result, err := s.Query(ctx, broker.FeatureQuery{
		BrokerID:     brokerID,
		Market:       market,
		InstrumentID: instrumentID,
		FeatureID:    broker.FeatureInstrumentProfile,
		Params:       map[string]any{},
	})
	if err != nil {
		return nil, err
	}
	var security map[string]any
	if len(result.Entries) > 0 {
		security = result.Entries[0]
	}
	return map[string]any{
		"request":  workspaceInstrumentRequest(market, symbol, instrumentID),
		"security": security,
		"meta":     workspaceProviderMeta(result, instrumentID, false),
	}, nil
}

// ReadMarketCandles adapts broker-neutral K-lines to the stable workspace
// candles response shape.
func (s *Service) ReadMarketCandles(
	ctx context.Context,
	brokerID string,
	market string,
	symbol string,
	period string,
	limit int,
	fromTime string,
	toTime string,
	beforeTime string,
	sessions []string,
) (map[string]any, error) {
	market, symbol, instrumentID, err := normalizeWorkspaceInstrument(market, symbol)
	if err != nil {
		return nil, err
	}
	pageSize := limit
	if pageSize < 1 {
		pageSize = 500
	}
	result, err := s.Query(ctx, broker.FeatureQuery{
		BrokerID:     brokerID,
		Market:       market,
		InstrumentID: instrumentID,
		FeatureID:    broker.FeatureMarketCandles,
		PageSize:     pageSize,
		Params: map[string]any{
			"operation":  "historical",
			"period":     period,
			"limit":      pageSize,
			"fromTime":   fromTime,
			"toTime":     toTime,
			"beforeTime": beforeTime,
			"sessions":   sessions,
		},
	})
	if err != nil {
		return nil, err
	}
	candles := make([]map[string]any, 0, len(result.Entries))
	for _, entry := range result.Entries {
		candle := maps.Clone(entry)
		candle["period"] = period
		if candle["at"] == nil {
			candle["at"] = candle["time"]
		}
		candles = append(candles, candle)
	}
	meta := workspaceProviderMeta(result, instrumentID, false)
	meta["sessions"] = sessions
	if result.Metadata != nil {
		if extendedHours, ok := result.Metadata["extendedHours"].(bool); ok {
			meta["extendedHours"] = extendedHours
		}
		if session, ok := result.Metadata["session"].(string); ok && session != "" {
			meta["session"] = session
		}
	}
	pagination, err := validateWorkspaceCandlePagination(
		result,
		candles,
		pageSize,
		fromTime,
		toTime,
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"request": map[string]any{
			"instrument": workspaceInstrumentRequest(market, symbol, instrumentID),
			"period":     period,
			"limit":      pageSize,
			"sessions":   sessions,
		},
		"candles":       candles,
		"totalReturned": len(candles),
		"pagination":    pagination,
		"meta":          meta,
	}, nil
}

func validateWorkspaceCandlePagination(
	result *broker.FeatureResult,
	candles []map[string]any,
	limit int,
	fromTime string,
	toTime string,
) (map[string]any, error) {
	if result == nil || result.HasMore == nil {
		return nil, fmt.Errorf("broker candle response is missing hasMore pagination metadata")
	}
	firstAt, firstAtText, err := validateWorkspaceCandleSequence(candles)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(candles) > limit {
		return nil, fmt.Errorf("broker candle page exceeds the requested limit")
	}
	hasRange := strings.TrimSpace(fromTime) != "" || strings.TrimSpace(toTime) != ""
	if hasRange {
		if *result.HasMore {
			return nil, fmt.Errorf("bounded broker candle query returned hasMore=true")
		}
		if strings.TrimSpace(result.NextCursor) != "" {
			return nil, fmt.Errorf("bounded broker candle query contains nextBefore")
		}
		return map[string]any{"hasMore": false}, nil
	}
	if !*result.HasMore {
		if strings.TrimSpace(result.NextCursor) != "" {
			return nil, fmt.Errorf("terminal broker candle page contains nextBefore")
		}
		return map[string]any{"hasMore": false}, nil
	}
	if firstAt.IsZero() || strings.TrimSpace(result.NextCursor) == "" {
		return nil, fmt.Errorf("paged broker candle response is missing nextBefore")
	}
	nextBefore, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(result.NextCursor))
	if err != nil || !nextBefore.Equal(firstAt) {
		return nil, fmt.Errorf("broker candle nextBefore must equal the earliest candle")
	}
	return map[string]any{"hasMore": true, "nextBefore": firstAtText}, nil
}

func validateWorkspaceCandleSequence(candles []map[string]any) (time.Time, string, error) {
	var first time.Time
	var firstText string
	var previous time.Time
	for index, candle := range candles {
		atText, ok := candle["at"].(string)
		if !ok || strings.TrimSpace(atText) == "" {
			return time.Time{}, "", fmt.Errorf("broker candle %d is missing its timestamp", index)
		}
		at, err := time.Parse(time.RFC3339Nano, atText)
		if err != nil {
			return time.Time{}, "", fmt.Errorf("broker candle %d has an invalid timestamp", index)
		}
		if !previous.IsZero() && !previous.Before(at) {
			return time.Time{}, "", fmt.Errorf("broker candles are not strictly ordered")
		}
		if first.IsZero() {
			first = at
			firstText = atText
		}
		previous = at
	}
	return first, firstText, nil
}

// ReadMarketDepth adapts broker-neutral order-book data to the stable
// workspace depth response shape.
func (s *Service) ReadMarketDepth(
	ctx context.Context,
	brokerID string,
	market string,
	symbol string,
	num int,
) (map[string]any, error) {
	market, symbol, instrumentID, err := normalizeWorkspaceInstrument(market, symbol)
	if err != nil {
		return nil, err
	}
	result, err := s.Query(ctx, broker.FeatureQuery{
		BrokerID:     brokerID,
		Market:       market,
		InstrumentID: instrumentID,
		FeatureID:    broker.FeatureMarketDepth,
		Params:       map[string]any{"num": num},
	})
	if err != nil {
		return nil, err
	}
	depth := map[string]any{
		"symbol": instrumentID,
		"bids":   []any{},
		"asks":   []any{},
	}
	if len(result.Entries) > 0 {
		depth = result.Entries[0]
	}
	return map[string]any{
		"request": map[string]any{
			"market":       market,
			"symbol":       symbol,
			"instrumentId": instrumentID,
			"num":          num,
		},
		"depth": depth,
		"meta":  workspaceProviderMeta(result, instrumentID, false),
	}, nil
}

func normalizeWorkspaceInstrument(market, symbol string) (string, string, string, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if qualifiedMarket, qualifiedSymbol, ok := strings.Cut(symbol, "."); ok {
		if market == "" {
			market = qualifiedMarket
			symbol = qualifiedSymbol
		} else if market != "CN" {
			symbol = qualifiedSymbol
		}
	}
	if market == "" || symbol == "" {
		return "", "", "", fmt.Errorf("%w: market and symbol are required", ErrInvalidQuery)
	}
	if market != "CN" {
		return market, symbol, market + "." + symbol, nil
	}
	parsed, err := marketpkg.ParseInstrument(marketpkg.InstrumentInput{
		Market: market,
		Symbol: symbol,
	})
	if err != nil || (parsed.Prefix != "SH" && parsed.Prefix != "SZ") {
		return "", "", "", fmt.Errorf(
			"%w: CN requires an SH. or SZ. qualified symbol", ErrInvalidQuery,
		)
	}
	return parsed.Prefix, parsed.Code, parsed.Symbol, nil
}

func workspaceInstrumentRequest(market, symbol, instrumentID string) map[string]any {
	return map[string]any{
		"market":       market,
		"symbol":       symbol,
		"instrumentId": instrumentID,
	}
}

func workspaceProviderMeta(
	result *broker.FeatureResult,
	instrumentID string,
	fromCache bool,
) map[string]any {
	resolvedAt := time.Now().UTC()
	source := ""
	brokerID := ""
	if result != nil {
		if !result.AsOf.IsZero() {
			resolvedAt = result.AsOf
		}
		brokerID = strings.ToLower(strings.TrimSpace(result.Provider.BrokerID))
		source = brokerID
	}
	return map[string]any{
		"instrumentId": instrumentID,
		"source":       source,
		"brokerId":     brokerID,
		"resolvedAt":   resolvedAt.Format(time.RFC3339Nano),
		"fromCache":    fromCache,
	}
}

func workspaceSnapshot(entry map[string]any, fallback time.Time) map[string]any {
	if entry == nil {
		return nil
	}
	session := strings.ToLower(stringValue(entry["session"]))
	observedAt := entry["observedAt"]
	if observedAt == nil {
		observedAt = entry["updateTime"]
	}
	if observedAt == nil && !fallback.IsZero() {
		observedAt = fallback.UTC().Format(time.RFC3339Nano)
	}
	price := entry["lastPrice"]
	highPrice := entry["highPrice"]
	lowPrice := entry["lowPrice"]
	volume := entry["volume"]
	turnover := entry["turnover"]
	if active := workspaceActiveExtendedQuote(entry, session); workspacePositiveNumber(active["price"]) {
		price = active["price"]
		highPrice = workspaceExtendedValue(active, "highPrice", highPrice, false)
		lowPrice = workspaceExtendedValue(active, "lowPrice", lowPrice, false)
		volume = workspaceExtendedValue(active, "volume", volume, true)
		turnover = workspaceExtendedValue(active, "turnover", turnover, true)
	}
	previousClosePrice := entry["previousClose"]
	lastClosePrice := entry["previousClose"]
	if marketpkg.IsUSSymbol(stringValue(entry["symbol"])) &&
		isOutsideRegularSession(session) {
		// The broker-neutral snapshot keeps OpenD's raw LastClosePrice in
		// previousClose and the latest regular close in lastPrice. Restore the
		// workspace contract used by the legacy quote path: outside regular
		// hours, previousClosePrice is the latest regular close while
		// lastClosePrice remains the prior trading-day close.
		previousClosePrice = entry["lastPrice"]
	}
	return map[string]any{
		"price":              price,
		"bid":                entry["bidPrice"],
		"ask":                entry["askPrice"],
		"openPrice":          entry["openPrice"],
		"highPrice":          highPrice,
		"lowPrice":           lowPrice,
		"previousClosePrice": previousClosePrice,
		"lastClosePrice":     lastClosePrice,
		"volume":             volume,
		"turnover":           turnover,
		"at":                 observedAt,
		"observedAt":         observedAt,
		"session":            entry["session"],
		"extendedHours":      isExtendedSession(session),
		"extended": map[string]any{
			"preMarket":   entry["preMarket"],
			"afterMarket": entry["afterMarket"],
			"overnight":   entry["overnight"],
		},
	}
}

func workspaceActiveExtendedQuote(entry map[string]any, session string) map[string]any {
	var key string
	switch session {
	case "pre":
		key = "preMarket"
	case "after":
		key = "afterMarket"
	case "overnight":
		key = "overnight"
	default:
		return nil
	}
	active, _ := entry[key].(map[string]any)
	return active
}

func workspaceExtendedValue(active map[string]any, key string, fallback any, allowZero bool) any {
	value := active[key]
	if workspacePositiveNumber(value) || allowZero && workspaceZero(value) {
		return value
	}
	return fallback
}

func workspacePositiveNumber(value any) bool {
	number, ok := value.(float64)
	return ok && number > 0
}

func workspaceZero(value any) bool {
	number, ok := value.(float64)
	return ok && number == 0
}

func isExtendedSession(session string) bool {
	switch strings.ToLower(strings.TrimSpace(session)) {
	case "pre", "after", "overnight":
		return true
	default:
		return false
	}
}

func isOutsideRegularSession(session string) bool {
	switch strings.ToLower(strings.TrimSpace(session)) {
	case "pre", "after", "overnight", "closed":
		return true
	default:
		return false
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
