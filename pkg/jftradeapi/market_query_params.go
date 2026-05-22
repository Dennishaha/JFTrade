package jftradeapi

import (
	"fmt"
	"strconv"
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

func firstQuery(query map[string][]string, key string, fallback string) string {
	values := query[key]
	if len(values) == 0 || values[0] == "" {
		return fallback
	}
	return values[0]
}

func intQuery(query map[string][]string, key string, fallback int) int {
	value, err := strconv.Atoi(firstQuery(query, key, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return value
}

func boolQuery(query map[string][]string, key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(firstQuery(query, key, "")))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func kLineQueryWindow(query map[string][]string, periodDuration time.Duration, limit int) (time.Time, time.Time) {
	endAt := parseQueryTime(firstQuery(query, "toTime", ""), time.Now())
	if queryEnd := firstQuery(query, "to", ""); queryEnd != "" {
		endAt = parseQueryTime(queryEnd, endAt)
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
	beginAt := parseQueryTime(firstQuery(query, "fromTime", ""), defaultBegin)
	if queryBegin := firstQuery(query, "from", ""); queryBegin != "" {
		beginAt = parseQueryTime(queryBegin, beginAt)
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
