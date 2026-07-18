package productfeatures

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
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
) (map[string]any, error) {
	market, symbol, instrumentID, err := normalizeWorkspaceInstrument(market, symbol)
	if err != nil {
		return nil, err
	}
	result, err := s.Query(ctx, broker.FeatureQuery{
		BrokerID:     brokerID,
		Market:       market,
		InstrumentID: instrumentID,
		FeatureID:    broker.FeatureMarketCandles,
		PageSize:     limit,
		Params: map[string]any{
			"operation": "historical",
			"period":    period,
			"limit":     limit,
			"fromTime":  fromTime,
			"toTime":    toTime,
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
	meta["extendedHours"] = false
	meta["session"] = "regular"
	return map[string]any{
		"request": map[string]any{
			"instrument": workspaceInstrumentRequest(market, symbol, instrumentID),
			"period":     period,
			"limit":      limit,
		},
		"candles":       candles,
		"totalReturned": len(candles),
		"meta":          meta,
	}, nil
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
		}
		symbol = qualifiedSymbol
	}
	if market == "" || symbol == "" {
		return "", "", "", fmt.Errorf("%w: market and symbol are required", ErrInvalidQuery)
	}
	return market, symbol, market + "." + symbol, nil
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
	observedAt := entry["observedAt"]
	if observedAt == nil {
		observedAt = entry["updateTime"]
	}
	if observedAt == nil && !fallback.IsZero() {
		observedAt = fallback.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{
		"price":              entry["lastPrice"],
		"bid":                entry["bidPrice"],
		"ask":                entry["askPrice"],
		"openPrice":          entry["openPrice"],
		"highPrice":          entry["highPrice"],
		"lowPrice":           entry["lowPrice"],
		"previousClosePrice": entry["previousClose"],
		"lastClosePrice":     entry["previousClose"],
		"volume":             entry["volume"],
		"turnover":           entry["turnover"],
		"at":                 observedAt,
		"observedAt":         observedAt,
		"session":            entry["session"],
		"extendedHours":      entry["preMarket"] != nil || entry["afterMarket"] != nil || entry["overnight"] != nil,
		"extended": map[string]any{
			"preMarket":   entry["preMarket"],
			"afterMarket": entry["afterMarket"],
			"overnight":   entry["overnight"],
		},
	}
}
