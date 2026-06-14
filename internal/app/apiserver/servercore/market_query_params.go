package servercore

import (
	"strings"
	"time"
)

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
