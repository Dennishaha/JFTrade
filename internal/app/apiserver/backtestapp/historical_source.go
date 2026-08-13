package backtestapp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	backtestservice "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	backteststore "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/shopspring/decimal"
)

type providerHistoricalSource struct {
	provider   marketdata.Provider
	descriptor marketdata.ProviderDescriptor
}

func NewKLineSyncer(
	ctx context.Context,
	runtime *marketdataapp.Runtime,
	dbPath, providerID string,
) (backtestservice.KLineSyncer, error) {
	lease, err := runtime.AcquireProvider(ctx, providerID, false)
	if err != nil {
		return nil, err
	}
	descriptor, err := lease.Descriptor(ctx)
	if err != nil {
		lease.Release()
		return nil, err
	}
	if !descriptor.Capabilities.HistoricalCandles {
		lease.Release()
		return nil, fmt.Errorf("provider %s does not support historical candles", providerID)
	}
	store, err := backteststore.NewKLineStore(dbPath, providerID)
	if err != nil {
		lease.Release()
		return nil, err
	}
	return backtestservice.NewHistoricalKLineSyncer(
		store,
		&providerHistoricalSource{provider: lease.Provider(), descriptor: descriptor},
		func() error { lease.Release(); return nil },
	), nil
}

func (s *providerHistoricalSource) FetchHistoricalCandles(
	ctx context.Context,
	query backtestservice.HistoricalCandleQuery,
) (backtestservice.HistoricalCandlePage, error) {
	if err := s.ValidateHistoricalCandleQuery(query); err != nil {
		return backtestservice.HistoricalCandlePage{}, err
	}
	sessions := s.providerSessions(query.Sessions)
	response, err := s.provider.GetHistoricalCandles(ctx, marketdata.HistoricalCandlesQuery{
		Market: query.Market, Symbol: query.Symbol, Period: query.Interval,
		Adjustment: string(query.Adjustment), Limit: query.Limit,
		BeforeTime: query.Before.UTC().Format(time.RFC3339Nano),
		Sessions:   sessions, SessionsSpecified: true,
	})
	if err != nil {
		return backtestservice.HistoricalCandlePage{}, err
	}
	return parseHistoricalPage(response)
}

func (s *providerHistoricalSource) ValidateHistoricalCandleQuery(query backtestservice.HistoricalCandleQuery) error {
	capabilities := s.descriptor.Capabilities
	if !slices.Contains(capabilities.CandleIntervals, query.Interval) {
		return fmt.Errorf("provider %s does not support interval %s", s.descriptor.SelectionID, query.Interval)
	}
	adjustment := strings.ToLower(strings.TrimSpace(string(query.Adjustment)))
	if !slices.Contains(capabilities.PriceAdjustments, adjustment) {
		return fmt.Errorf("provider %s does not support %s price adjustment", s.descriptor.SelectionID, adjustment)
	}
	if len(query.Sessions) > 1 && !capabilities.ExtendedHours {
		return fmt.Errorf("provider %s does not support extended sessions", s.descriptor.SelectionID)
	}
	if len(query.Sessions) > 1 &&
		(strings.ToUpper(strings.TrimSpace(query.Market)) != "US" ||
			!slices.Contains([]string{"tick", "1m", "5m", "15m", "30m", "1h"}, query.Interval)) {
		return fmt.Errorf("provider %s supports extended sessions only for US intraday candles", s.descriptor.SelectionID)
	}
	if days := historicalLookbackDays(capabilities, query.Market, query.Interval); days > 0 &&
		query.Since.Before(time.Now().UTC().AddDate(0, 0, -days)) {
		return fmt.Errorf("provider %s limits %s history to %d days", s.descriptor.SelectionID, query.Interval, days)
	}
	return nil
}

func historicalLookbackDays(capabilities marketdata.ProviderCapabilities, market, interval string) int {
	marketKey := strings.ToUpper(strings.TrimSpace(market)) + ":" + strings.TrimSpace(interval)
	if days := capabilities.HistoricalLookbackDays[marketKey]; days > 0 {
		return days
	}
	return capabilities.HistoricalLookbackDays[strings.TrimSpace(interval)]
}

func (s *providerHistoricalSource) providerSessions(requested []string) []marketdata.CandleSession {
	result := make([]marketdata.CandleSession, 0, len(requested))
	for _, value := range requested {
		session := marketdata.CandleSession(strings.ToLower(strings.TrimSpace(value)))
		if session != marketdata.CandleSessionOvernight || s.supportsOvernight() {
			result = append(result, session)
		}
	}
	return result
}

func (s *providerHistoricalSource) supportsOvernight() bool {
	for _, session := range s.descriptor.Capabilities.Sessions {
		if strings.EqualFold(strings.TrimSpace(session), "overnight") {
			return true
		}
	}
	return false
}

func parseHistoricalPage(response marketdata.CandlesResponse) (backtestservice.HistoricalCandlePage, error) {
	values, ok := response["candles"].([]map[string]any)
	if !ok {
		return backtestservice.HistoricalCandlePage{}, fmt.Errorf("historical candle response is malformed")
	}
	page := backtestservice.HistoricalCandlePage{Candles: make([]backtestservice.HistoricalCandle, 0, len(values))}
	for index, value := range values {
		atValue, _ := value["at"].(string)
		at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(atValue))
		if err != nil {
			return backtestservice.HistoricalCandlePage{}, fmt.Errorf("parse candle %d timestamp: %w", index, err)
		}
		open, err := decimalString(value["open"])
		if err != nil {
			return page, fmt.Errorf("candle %d open: %w", index, err)
		}
		high, err := decimalString(value["high"])
		if err != nil {
			return page, fmt.Errorf("candle %d high: %w", index, err)
		}
		low, err := decimalString(value["low"])
		if err != nil {
			return page, fmt.Errorf("candle %d low: %w", index, err)
		}
		closeValue, err := decimalString(value["close"])
		if err != nil {
			return page, fmt.Errorf("candle %d close: %w", index, err)
		}
		volume := "0"
		if value["volume"] != nil {
			volume, err = decimalString(value["volume"])
			if err != nil {
				return page, fmt.Errorf("candle %d volume: %w", index, err)
			}
		}
		session, _ := value["session"].(string)
		page.Candles = append(page.Candles, backtestservice.HistoricalCandle{
			At: at, Open: open, High: high, Low: low, Close: closeValue, Volume: volume, Session: session,
		})
	}
	if pagination, ok := response["pagination"].(map[string]any); ok {
		page.HasMore, _ = pagination["hasMore"].(bool)
		if cursor, _ := pagination["nextBefore"].(string); strings.TrimSpace(cursor) != "" {
			var err error
			page.NextBefore, err = time.Parse(time.RFC3339Nano, cursor)
			if err != nil {
				return backtestservice.HistoricalCandlePage{}, fmt.Errorf("parse historical cursor: %w", err)
			}
		}
	}
	return page, nil
}

func decimalString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		parsed, err := decimal.NewFromString(strings.TrimSpace(typed))
		if err != nil {
			return "", err
		}
		return parsed.String(), nil
	case json.Number:
		parsed, err := decimal.NewFromString(typed.String())
		if err != nil {
			return "", err
		}
		return parsed.String(), nil
	case decimal.Decimal:
		return typed.String(), nil
	case float64:
		return decimal.NewFromFloat(typed).String(), nil
	case float32:
		return decimal.NewFromFloat32(typed).String(), nil
	case int:
		return decimal.NewFromInt(int64(typed)).String(), nil
	case int64:
		return decimal.NewFromInt(typed).String(), nil
	default:
		return "", fmt.Errorf("unsupported decimal value %T", value)
	}
}
