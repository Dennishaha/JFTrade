package servercore

import (
	"fmt"
	"strings"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func pathTail(path string, prefix string) (string, string) {
	tail := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(tail, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func decodeMarketSnapshotQuery(values map[string][]string) (marketSnapshotQuery, error) {
	var query marketSnapshotQuery
	if raw, ok := firstQueryValue(values, "refresh"); ok && raw != "" {
		if err := query.Refresh.UnmarshalText([]byte(raw)); err != nil {
			return marketSnapshotQuery{}, fmt.Errorf("invalid refresh query: %w", err)
		}
	}
	return query, nil
}

func decodeMarketCandlesQuery(values map[string][]string) (marketCandlesQuery, error) {
	var query marketCandlesQuery
	if raw, ok := values["sessions"]; ok {
		sessions, err := mdsrv.ParseCandleSessions(raw)
		if err != nil {
			return marketCandlesQuery{}, err
		}
		query.Sessions = sessions
		query.SessionsSpecified = true
	}
	if raw, ok := firstQueryValue(values, "period"); ok && raw != "" {
		if err := query.Period.UnmarshalText([]byte(raw)); err != nil {
			return marketCandlesQuery{}, err
		}
	}
	if raw, ok := firstQueryValue(values, "limit"); ok && raw != "" {
		if err := query.Limit.UnmarshalText([]byte(raw)); err != nil {
			return marketCandlesQuery{}, fmt.Errorf("invalid limit query: %w", err)
		}
	}
	if raw, ok := firstQueryValue(values, "fromTime"); ok && raw != "" {
		if err := query.FromTime.UnmarshalText([]byte(raw)); err != nil {
			return marketCandlesQuery{}, fmt.Errorf("invalid fromTime query: %w", err)
		}
	}
	if raw, ok := firstQueryValue(values, "toTime"); ok && raw != "" {
		if err := query.ToTime.UnmarshalText([]byte(raw)); err != nil {
			return marketCandlesQuery{}, fmt.Errorf("invalid toTime query: %w", err)
		}
	}
	if raw, ok := firstQueryValue(values, "from"); ok && raw != "" {
		if err := query.From.UnmarshalText([]byte(raw)); err != nil {
			return marketCandlesQuery{}, fmt.Errorf("invalid from query: %w", err)
		}
	}
	if raw, ok := firstQueryValue(values, "to"); ok && raw != "" {
		if err := query.To.UnmarshalText([]byte(raw)); err != nil {
			return marketCandlesQuery{}, fmt.Errorf("invalid to query: %w", err)
		}
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
