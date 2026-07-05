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
		jftradeErr5 := query.Refresh.UnmarshalText([]byte(raw))
		jftradeLogError(jftradeErr5)
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
		jftradeErr3 := query.Limit.UnmarshalText([]byte(raw))
		jftradeLogError(jftradeErr3)
	}
	if raw, ok := firstQueryValue(values, "fromTime"); ok && raw != "" {
		jftradeErr6 := query.FromTime.UnmarshalText([]byte(raw))
		jftradeLogError(jftradeErr6)
	}
	if raw, ok := firstQueryValue(values, "toTime"); ok && raw != "" {
		jftradeErr4 := query.ToTime.UnmarshalText([]byte(raw))
		jftradeLogError(jftradeErr4)
	}
	if raw, ok := firstQueryValue(values, "from"); ok && raw != "" {
		jftradeErr2 := query.From.UnmarshalText([]byte(raw))
		jftradeLogError(jftradeErr2)
	}
	if raw, ok := firstQueryValue(values, "to"); ok && raw != "" {
		jftradeErr1 := query.To.UnmarshalText([]byte(raw))
		jftradeLogError(jftradeErr1)
	}
	return query, nil
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
	if !query.ToTime.IsZero() {
		endAt = query.ToTime.Time
	}
	if !query.To.IsZero() {
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
	if !query.FromTime.IsZero() {
		beginAt = query.FromTime.Time
	}
	if !query.From.IsZero() {
		beginAt = query.From.Time
	}
	if !beginAt.Before(endAt) {
		beginAt = defaultBegin
	}
	return beginAt, endAt
}
