package jftradeapi

import (
	"fmt"
	"strings"
	"time"
)

func normalizeCandlePeriod(period string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "tick", "ticker", "k_tick":
		return "tick", nil
	case "1m", "1min", "k_1m":
		return "1m", nil
	case "3m", "3min", "k_3m":
		return "3m", nil
	case "5m", "5min", "k_5m":
		return "5m", nil
	case "10m", "10min", "k_10m":
		return "10m", nil
	case "15m", "15min", "k_15m":
		return "15m", nil
	case "30m", "30min", "k_30m":
		return "30m", nil
	case "60m", "60min", "1h", "k_60m":
		return "1h", nil
	case "1d", "day", "d", "k_day":
		return "1d", nil
	case "1w", "week", "w", "k_week":
		return "1w", nil
	case "1mo", "month", "mth", "k_month":
		return "1mo", nil
	default:
		return "", fmt.Errorf("unsupported period %q", period)
	}
}

func pathTail(path string, prefix string) (string, string) {
	tail := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(tail, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func decodeMarketSnapshotQuery(values map[string][]string) marketSnapshotQuery {
	var query marketSnapshotQuery
	if raw, ok := firstQueryValue(values, "refresh"); ok && raw != "" {
		_ = query.Refresh.UnmarshalText([]byte(raw))
	}
	return query
}

func decodeMarketCandlesQuery(values map[string][]string) (marketCandlesQuery, error) {
	var query marketCandlesQuery
	if raw, ok := firstQueryValue(values, "period"); ok && raw != "" {
		if err := query.Period.UnmarshalText([]byte(raw)); err != nil {
			return marketCandlesQuery{}, err
		}
	}
	if raw, ok := firstQueryValue(values, "limit"); ok && raw != "" {
		_ = query.Limit.UnmarshalText([]byte(raw))
	}
	if raw, ok := firstQueryValue(values, "fromTime"); ok && raw != "" {
		_ = query.FromTime.UnmarshalText([]byte(raw))
	}
	if raw, ok := firstQueryValue(values, "toTime"); ok && raw != "" {
		_ = query.ToTime.UnmarshalText([]byte(raw))
	}
	if raw, ok := firstQueryValue(values, "from"); ok && raw != "" {
		_ = query.From.UnmarshalText([]byte(raw))
	}
	if raw, ok := firstQueryValue(values, "to"); ok && raw != "" {
		_ = query.To.UnmarshalText([]byte(raw))
	}
	return query, nil
}

func decodeMarketDepthQuery(values map[string][]string) marketDepthQuery {
	var query marketDepthQuery
	if raw, ok := firstQueryValue(values, "num"); ok && raw != "" {
		_ = query.Num.UnmarshalText([]byte(raw))
	}
	return query
}

func firstQueryValue(query map[string][]string, key string) (string, bool) {
	values, ok := query[key]
	if !ok || len(values) == 0 {
		return "", false
	}
	return values[0], true
}

func kLineQueryWindow(query marketCandlesQuery, periodDuration time.Duration, limit int) (time.Time, time.Time) {
	endAt := time.Now()
	if !query.ToTime.Time.IsZero() {
		endAt = query.ToTime.Time
	}
	if !query.To.Time.IsZero() {
		endAt = query.To.Time
	}
	lookback := periodDuration * time.Duration(limit) * 4
	minimumLookback := 36 * time.Hour
	if periodDuration >= 24*time.Hour {
		minimumLookback = 45 * 24 * time.Hour
	}
	if lookback < minimumLookback {
		lookback = minimumLookback
	}
	defaultBegin := endAt.Add(-lookback)
	beginAt := defaultBegin
	if !query.FromTime.Time.IsZero() {
		beginAt = query.FromTime.Time
	}
	if !query.From.Time.IsZero() {
		beginAt = query.From.Time
	}
	if !beginAt.Before(endAt) {
		beginAt = defaultBegin
	}
	return beginAt, endAt
}

func parseQueryTime(value string, fallback time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
