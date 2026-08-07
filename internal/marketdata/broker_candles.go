package marketdata

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/market"
)

// BrokerKLineCandlesResponse projects a broker-neutral K-line page onto the
// market-data candle contract while preserving strict pagination semantics.
func BrokerKLineCandlesResponse(
	marketCode string,
	symbol string,
	instrumentID string,
	period string,
	limit int,
	request HistoricalCandlesQuery,
	sessions []CandleSession,
	includeSession bool,
	snapshot *broker.KLineSnapshot,
	source string,
) (CandlesResponse, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("broker returned an empty K-line snapshot")
	}
	before, err := historicalCandleBeforeTime(request.BeforeTime)
	if err != nil {
		return nil, err
	}
	candles := make([]map[string]any, 0, len(snapshot.KLines))
	var previousAt time.Time
	for index, item := range snapshot.KLines {
		candle, at, candleErr := brokerKLineCandle(item, instrumentID, period, includeSession, sessions)
		if candleErr != nil {
			return nil, fmt.Errorf("candle %d: %w", index, candleErr)
		}
		if !previousAt.IsZero() && !previousAt.Before(at) {
			return nil, fmt.Errorf("broker returned K-lines that are not strictly ordered")
		}
		if !before.IsZero() && !at.Before(before) {
			return nil, fmt.Errorf("broker returned a K-line at or after the before cursor")
		}
		previousAt = at
		candles = append(candles, candle)
	}
	pagination, err := brokerKLinePagination(snapshot.Pagination, candles, request)
	if err != nil {
		return nil, err
	}
	extendedHours := includeSession && (ContainsCandleSession(sessions, CandleSessionExtended) ||
		ContainsCandleSession(sessions, CandleSessionOvernight))
	return CandlesResponseDTO{
		Instrument: InstrumentDTO{Market: marketCode, Symbol: symbol, InstrumentID: instrumentID},
		Period:     period, Limit: limit, Candles: candles, Pagination: pagination, Source: strings.TrimSpace(source),
		ResolvedAt: time.Now().UTC().Format(time.RFC3339Nano), ExtendedHours: extendedHours,
		IncludeSession: includeSession, Sessions: sessions,
	}.JSON(), nil
}

func historicalCandleBeforeTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	before, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid K-line before cursor: %w", err)
	}
	return before.UTC(), nil
}

func brokerKLineCandle(
	item broker.KLineItem,
	instrumentID string,
	period string,
	includeSession bool,
	sessions []CandleSession,
) (map[string]any, time.Time, error) {
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(item.Time))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("invalid K-line time: %w", err)
	}
	at = at.UTC()
	candle := map[string]any{"period": period, "at": at.Format(time.RFC3339Nano)}
	for name, value := range map[string]*float64{
		"open": item.Open, "high": item.High, "low": item.Low, "close": item.Close, "volume": item.Volume,
	} {
		formatted, formatErr := brokerKLineNumber(value, name)
		if formatErr != nil {
			return nil, time.Time{}, formatErr
		}
		candle[name] = formatted
	}
	if includeSession {
		label, group, sessionErr := brokerKLineSession(item.Session, instrumentID, at)
		if sessionErr != nil {
			return nil, time.Time{}, sessionErr
		}
		if !ContainsCandleSession(sessions, group) {
			return nil, time.Time{}, fmt.Errorf("k-line session %q was not requested", label)
		}
		candle["session"] = label
	}
	return candle, at, nil
}

func brokerKLineNumber(value *float64, field string) (string, error) {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return "", fmt.Errorf("k-line %s is missing or non-finite", field)
	}
	return strconv.FormatFloat(*value, 'f', -1, 64), nil
}

func brokerKLineSession(
	label string,
	instrumentID string,
	at time.Time,
) (string, CandleSession, error) {
	classified := market.ClassifySession(instrumentID, at)
	if classified == market.SessionClosed {
		return "", "", fmt.Errorf("unable to classify K-line session at %s", at.Format(time.RFC3339Nano))
	}
	label = strings.ToLower(strings.TrimSpace(label))
	if label != "" {
		group := CandleSessionForLabel(label)
		if group == "" {
			return "", "", fmt.Errorf("unable to classify K-line session %q", label)
		}
		return label, group, nil
	}
	group := CandleSessionForLabel(string(classified))
	if group == "" {
		return "", "", fmt.Errorf("unable to classify K-line session at %s", at.Format(time.RFC3339Nano))
	}
	return string(classified), group, nil
}

func brokerKLinePagination(
	pagination broker.KLinePagination,
	candles []map[string]any,
	request HistoricalCandlesQuery,
) (CandlePagination, error) {
	if request.Limit > 0 && len(candles) > request.Limit {
		return CandlePagination{}, fmt.Errorf("k-line page exceeds the requested limit")
	}
	hasRange := strings.TrimSpace(request.FromTime) != "" || strings.TrimSpace(request.ToTime) != ""
	if hasRange {
		if pagination.HasMore {
			return CandlePagination{}, fmt.Errorf("bounded K-line query returned hasMore=true")
		}
		if strings.TrimSpace(pagination.NextBefore) != "" {
			return CandlePagination{}, fmt.Errorf("bounded K-line query contains nextBefore")
		}
		return CandlePagination{}, nil
	}
	if !pagination.HasMore {
		if strings.TrimSpace(pagination.NextBefore) != "" {
			return CandlePagination{}, fmt.Errorf("terminal K-line page contains nextBefore")
		}
		return CandlePagination{}, nil
	}
	if len(candles) == 0 || strings.TrimSpace(pagination.NextBefore) == "" {
		return CandlePagination{}, fmt.Errorf("paged K-line response is missing its cursor")
	}
	next, err := time.Parse(time.RFC3339Nano, pagination.NextBefore)
	if err != nil {
		return CandlePagination{}, fmt.Errorf("invalid K-line nextBefore: %w", err)
	}
	nextBefore := next.UTC().Format(time.RFC3339Nano)
	if nextBefore != candles[0]["at"] {
		return CandlePagination{}, fmt.Errorf("k-line nextBefore does not equal the earliest candle")
	}
	return CandlePagination{HasMore: true, NextBefore: nextBefore}, nil
}
