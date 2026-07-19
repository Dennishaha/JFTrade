package backtest

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

// parseInstrument 统一标的输入解析（市场 + 代码/符号 → 标准化的市场/前缀/代码/符号）。
func parseInstrument(marketInput, symbol, code string) (struct{ Market, Prefix, Code, Symbol string }, error) {
	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market: marketInput,
		Symbol: symbol,
		Code:   code,
	})
	if err != nil {
		return struct{ Market, Prefix, Code, Symbol string }{}, err
	}
	return struct{ Market, Prefix, Code, Symbol string }{
		Market: instrument.Market,
		Prefix: instrument.Prefix,
		Code:   instrument.Code,
		Symbol: instrument.Symbol,
	}, nil
}

func resolveBacktestTimeRange(symbol, startDate, endDate, startTime, endTime string) (time.Time, time.Time, string, string, string, error) {
	profile, ok := market.ProfileForSymbol(symbol)
	if !ok || profile.Location == nil {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("unsupported market timezone for %s", symbol)
	}
	startDate = strings.TrimSpace(startDate)
	endDate = strings.TrimSpace(endDate)
	if startDate != "" || endDate != "" {
		return resolveBacktestDateRange(profile.Location, startDate, endDate)
	}
	start, err := parseRFC3339Time(startTime)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid startTime, use RFC3339 format")
	}
	end, err := parseRFC3339Time(endTime)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid endTime, use RFC3339 format")
	}
	return start.UTC(), end.UTC(), "", "", profile.Location.String(), nil
}

func resolveBacktestDateRange(location *time.Location, startDate string, endDate string) (time.Time, time.Time, string, string, string, error) {
	if startDate == "" || endDate == "" {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("startDate and endDate must be provided together")
	}
	startLocal, err := parseMarketDate(startDate, location)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid startDate, use YYYY-MM-DD format")
	}
	endLocal, err := parseMarketDate(endDate, location)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid endDate, use YYYY-MM-DD format")
	}
	if endLocal.Before(startLocal) {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("endDate must not be before startDate")
	}
	return startLocal.UTC(), endLocal.AddDate(0, 0, 1).Add(-time.Nanosecond).UTC(), startDate, endDate, location.String(), nil
}

func resolveSyncTimeRange(symbol, startDate, endDate, since, until string) (time.Time, time.Time, string, string, string, error) {
	if strings.TrimSpace(startDate) != "" || strings.TrimSpace(endDate) != "" {
		return resolveBacktestTimeRange(symbol, startDate, endDate, "", "")
	}
	now := time.Now().UTC()
	sinceTime := now.AddDate(0, 0, -30)
	untilTime := now
	var err error
	if strings.TrimSpace(since) != "" {
		sinceTime, err = parseRFC3339Time(since)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid since time, use RFC3339")
		}
	}
	if strings.TrimSpace(until) != "" {
		untilTime, err = parseRFC3339Time(until)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid until time, use RFC3339")
		}
	}
	return sinceTime.UTC(), untilTime.UTC(), "", "", "", nil
}

func parseMarketDate(value string, location *time.Location) (time.Time, error) {
	if len(value) != len("2006-01-02") {
		return time.Time{}, fmt.Errorf("invalid date")
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, fmt.Errorf("invalid date")
	}
	return parsed, nil
}

func parseRFC3339Time(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

func parseResultViewFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err == nil
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}
