package backtest

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

// ResultView returns a bounded, tool-friendly view of a backtest result. Large
// series are windowed and optionally aggregated so agents can inspect charts in
// several smaller calls instead of loading the full result into context.
func (s *Service) ResultView(req ResultViewRequest) (map[string]any, error) {
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		return nil, requestErrorf("runId is required")
	}
	run, ok, err := s.GetResult(runID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, requestErrorf("backtest run not found: %s", runID)
	}

	view := strings.ToLower(strings.TrimSpace(req.View))
	if view == "" {
		view = "summary"
	}
	switch view {
	case "summary", "chart", "orders", "logs", "errors":
	default:
		return nil, requestErrorf("view must be one of summary, chart, orders, logs, errors")
	}

	limit := normalizeResultViewLimit(req.Limit)
	offset, err := parseResultViewCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	startTime, err := parseOptionalResultViewTime(req.StartTime, "startTime")
	if err != nil {
		return nil, err
	}
	endTime, err := parseOptionalResultViewTime(req.EndTime, "endTime")
	if err != nil {
		return nil, err
	}
	if startTime != nil && endTime != nil && endTime.Before(*startTime) {
		return nil, requestErrorf("endTime must be after or equal to startTime")
	}

	result := run.Result
	payload := map[string]any{
		"view":    view,
		"run":     resultViewRunPayload(run),
		"summary": resultViewSummaryPayload(run),
		"window": map[string]any{
			"startTime":      req.StartTime,
			"endTime":        req.EndTime,
			"nativeInterval": run.Request.Interval,
			"limit":          limit,
			"cursor":         req.Cursor,
			"offset":         offset,
			"returned":       map[string]int{},
			"truncated":      false,
			"nextCursor":     "",
		},
		"series": map[string]any{},
	}
	if result == nil {
		return payload, nil
	}

	window := jftradeCheckedTypeAssertion[map[string]any](payload["window"])
	series := jftradeCheckedTypeAssertion[map[string]any](payload["series"])
	returned := jftradeCheckedTypeAssertion[map[string]int](window["returned"])

	switch view {
	case "summary":
		return payload, nil
	case "chart":
		include := resultViewIncludeSet(req.Include, []string{"candles", "trades", "pnlCurve", "drawdownCurve"})
		resolution, candles, err := resultViewCandles(result.Candles, run.Request.Interval, req.Resolution, startTime, endTime, limit)
		if err != nil {
			return nil, err
		}
		window["resolution"] = resolution
		if include["candles"] {
			items, next := sliceResultViewItems(candles, offset, limit)
			series["candles"] = items
			returned["candles"] = len(items)
			applyResultViewNextCursor(window, next)
		}
		if include["trades"] {
			filtered := filterResultViewTimedItems(result.Trades, startTime, endTime, func(item bt.TradeEvent) string { return item.Time })
			items, next := sliceResultViewItems(filtered, offset, limit)
			series["trades"] = items
			returned["trades"] = len(items)
			applyResultViewNextCursor(window, next)
		}
		if include["pnlCurve"] {
			filtered := filterResultViewTimedItems(result.PnLCurve, startTime, endTime, func(item bt.PnLPoint) string { return item.Time })
			items, next := sliceResultViewItems(filtered, offset, limit)
			series["pnlCurve"] = items
			returned["pnlCurve"] = len(items)
			applyResultViewNextCursor(window, next)
		}
		if include["drawdownCurve"] {
			filtered := filterResultViewTimedItems(result.DrawdownCurve, startTime, endTime, func(item bt.DrawdownPoint) string { return item.Time })
			items, next := sliceResultViewItems(filtered, offset, limit)
			series["drawdownCurve"] = items
			returned["drawdownCurve"] = len(items)
			applyResultViewNextCursor(window, next)
		}
	case "orders":
		items, next := sliceResultViewItems(filterResultViewOrders(result.OrderBook, startTime, endTime), offset, limit)
		series["orderBook"] = items
		returned["orderBook"] = len(items)
		applyResultViewNextCursor(window, next)
	case "logs":
		items, next := sliceResultViewItems(result.Logs, offset, limit)
		series["logs"] = items
		returned["logs"] = len(items)
		applyResultViewNextCursor(window, next)
	case "warnings":
		items, next := sliceResultViewItems(result.Warnings, offset, limit)
		series["warnings"] = items
		returned["warnings"] = len(items)
		applyResultViewNextCursor(window, next)
	case "errors":
		items, next := sliceResultViewItems(result.RuntimeErrors, offset, limit)
		series["runtimeErrors"] = items
		returned["runtimeErrors"] = len(items)
		applyResultViewNextCursor(window, next)
	}
	return payload, nil
}

func normalizeResultViewLimit(limit int) int {
	if limit <= 0 {
		return 500
	}
	if limit > 2000 {
		return 2000
	}
	return limit
}

func parseResultViewCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, requestErrorf("cursor must be a non-negative integer offset")
	}
	return offset, nil
}

func parseOptionalResultViewTime(value string, field string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := parseResultViewTime(value)
	if err != nil {
		return nil, requestErrorf("invalid %s, use RFC3339 format", field)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func parseResultViewTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

func resultViewRunPayload(run *RunState) map[string]any {
	if run == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":                run.ID,
		"status":            run.Status,
		"definitionId":      run.Request.DefinitionID,
		"definitionVersion": run.Request.DefinitionVersion,
		"market":            run.Request.Market,
		"code":              run.Request.Code,
		"symbol":            run.Request.Symbol,
		"instrumentType":    run.Request.InstrumentType,
		"interval":          run.Request.Interval,
		"startDate":         run.Request.StartDate,
		"endDate":           run.Request.EndDate,
		"startTime":         run.Request.StartTime,
		"endTime":           run.Request.EndTime,
		"marketTimezone":    run.Request.MarketTimezone,
		"initialBalance":    run.Request.InitialBalance,
		"rehabType":         run.Request.RehabType,
		"useExtendedHours":  run.Request.UseExtendedHours,
		"tradingCosts":      run.Request.TradingCosts,
		"createdAt":         run.CreatedAt,
		"updatedAt":         run.UpdatedAt,
	}
}

func resultViewSummaryPayload(run *RunState) map[string]any {
	summary := map[string]any{}
	if run == nil || run.Result == nil {
		return summary
	}
	result := run.Result
	summary["quoteCurrency"] = result.QuoteCurrency
	summary["finalBalance"] = result.FinalBalance
	summary["pnl"] = result.PnL
	summary["totalBrokerFees"] = result.TotalBrokerFees
	summary["totalMarketFees"] = result.TotalMarketFees
	summary["totalFees"] = result.TotalFees
	summary["feeBreakdown"] = result.FeeBreakdown
	if run.Request.InitialBalance > 0 {
		summary["totalReturn"] = result.PnL / run.Request.InitialBalance
	}
	summary["maxDrawdown"] = result.MaxDrawdown
	summary["currentDrawdown"] = result.CurrentDrawdown
	summary["totalTrades"] = result.TotalTrades
	summary["winRate"] = result.WinRate
	summary["candlesCount"] = len(result.Candles)
	summary["tradesCount"] = len(result.Trades)
	summary["orderBookCount"] = len(result.OrderBook)
	summary["pnlCurveCount"] = len(result.PnLCurve)
	summary["drawdownCurveCount"] = len(result.DrawdownCurve)
	summary["logsCount"] = len(result.Logs)
	summary["warningCount"] = len(result.Warnings)
	summary["warningTotal"] = result.WarningTotal
	summary["warningsTruncated"] = result.WarningsTruncated
	summary["ignoredOrders"] = result.IgnoredOrders
	summary["runtimeErrorCount"] = len(result.RuntimeErrors)
	summary["runtimeErrorTotal"] = result.RuntimeErrorTotal
	summary["error"] = result.Error
	if len(result.Logs) > 0 {
		summary["latestLog"] = result.Logs[len(result.Logs)-1]
	}
	if len(result.Warnings) > 0 {
		summary["latestWarning"] = result.Warnings[len(result.Warnings)-1]
	}
	if len(result.RuntimeErrors) > 0 {
		summary["latestRuntimeError"] = result.RuntimeErrors[len(result.RuntimeErrors)-1]
	}
	return summary
}

func resultViewIncludeSet(include []string, defaults []string) map[string]bool {
	if len(include) == 0 {
		include = defaults
	}
	set := make(map[string]bool, len(include))
	for _, item := range include {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		set[item] = true
	}
	return set
}

func sliceResultViewItems[T any](items []T, offset int, limit int) ([]T, string) {
	if offset >= len(items) {
		return []T{}, ""
	}
	end := min(offset+limit, len(items))
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}
	return append([]T(nil), items[offset:end]...), next
}

func applyResultViewNextCursor(window map[string]any, next string) {
	if strings.TrimSpace(next) == "" {
		return
	}
	window["truncated"] = true
	if strings.TrimSpace(fmt.Sprint(window["nextCursor"])) == "" {
		window["nextCursor"] = next
	}
}

func filterResultViewTimedItems[T any](items []T, startTime *time.Time, endTime *time.Time, timeFn func(T) string) []T {
	if startTime == nil && endTime == nil {
		return append([]T(nil), items...)
	}
	out := make([]T, 0, len(items))
	for _, item := range items {
		parsed, err := parseResultViewTime(timeFn(item))
		if err != nil {
			continue
		}
		parsed = parsed.UTC()
		if startTime != nil && parsed.Before(*startTime) {
			continue
		}
		if endTime != nil && parsed.After(*endTime) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterResultViewOrders(items []bt.OrderBookEntry, startTime *time.Time, endTime *time.Time) []bt.OrderBookEntry {
	if startTime == nil && endTime == nil {
		return append([]bt.OrderBookEntry(nil), items...)
	}
	out := make([]bt.OrderBookEntry, 0, len(items))
	for _, item := range items {
		if resultViewTimeInWindow(item.SubmittedAt, startTime, endTime) || resultViewTimeInWindow(item.FilledAt, startTime, endTime) {
			out = append(out, item)
		}
	}
	return out
}

func resultViewTimeInWindow(value string, startTime *time.Time, endTime *time.Time) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	parsed, err := parseResultViewTime(value)
	if err != nil {
		return false
	}
	parsed = parsed.UTC()
	if startTime != nil && parsed.Before(*startTime) {
		return false
	}
	if endTime != nil && parsed.After(*endTime) {
		return false
	}
	return true
}

func resultViewCandles(
	candles []bt.Candle,
	nativeInterval string,
	resolution string,
	startTime *time.Time,
	endTime *time.Time,
	limit int,
) (string, []bt.Candle, error) {
	filtered := filterResultViewTimedItems(candles, startTime, endTime, func(item bt.Candle) string { return item.Time })
	nativeDuration, err := resultViewIntervalDuration(nativeInterval)
	if err != nil {
		return "", nil, err
	}
	var targetDuration time.Duration
	normalizedResolution := strings.ToLower(strings.TrimSpace(resolution))
	if normalizedResolution == "" || normalizedResolution == "auto" {
		targetDuration = chooseResultViewAutoResolution(nativeDuration, len(filtered), limit)
	} else {
		targetDuration, err = resultViewIntervalDuration(normalizedResolution)
		if err != nil {
			return "", nil, err
		}
		if targetDuration < nativeDuration {
			return "", nil, requestErrorf("resolution %s is finer than native interval %s", resolution, nativeInterval)
		}
	}
	label := resultViewResolutionLabel(targetDuration)
	if targetDuration <= nativeDuration || len(filtered) == 0 {
		return label, filtered, nil
	}
	return label, aggregateResultViewCandles(filtered, targetDuration), nil
}

func resultViewIntervalDuration(value string) (time.Duration, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return 0, requestErrorf("resolution is required")
	}
	if duration, ok := safeResultViewIntervalDuration(value); ok {
		return duration, nil
	}
	unit := value[len(value)-1]
	numberText := value[:len(value)-1]
	if unit >= '0' && unit <= '9' {
		numberText = value
		unit = 'm'
	}
	number, err := strconv.Atoi(numberText)
	if err != nil || number <= 0 {
		return 0, requestErrorf("unsupported interval or resolution: %s", value)
	}
	switch unit {
	case 's':
		return time.Duration(number) * time.Second, nil
	case 'm':
		return time.Duration(number) * time.Minute, nil
	case 'h':
		return time.Duration(number) * time.Hour, nil
	case 'd':
		return time.Duration(number) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(number) * 7 * 24 * time.Hour, nil
	default:
		return 0, requestErrorf("unsupported interval or resolution: %s", value)
	}
}

func safeResultViewIntervalDuration(value string) (duration time.Duration, ok bool) {
	defer func() {
		if recover() != nil {
			duration = 0
			ok = false
		}
	}()
	duration = bbgotypes.Interval(value).Duration()
	return duration, duration > 0
}

func chooseResultViewAutoResolution(nativeDuration time.Duration, count int, limit int) time.Duration {
	if count <= limit || limit <= 0 {
		return nativeDuration
	}
	required := nativeDuration * time.Duration(int(math.Ceil(float64(count)/float64(limit))))
	for _, candidate := range []time.Duration{
		time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute,
		time.Hour, 2 * time.Hour, 4 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour,
	} {
		if candidate >= nativeDuration && candidate >= required {
			return candidate
		}
	}
	return required
}

func resultViewResolutionLabel(duration time.Duration) string {
	switch {
	case duration%(7*24*time.Hour) == 0:
		return fmt.Sprintf("%dw", int(duration/(7*24*time.Hour)))
	case duration%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(duration/(24*time.Hour)))
	case duration%time.Hour == 0:
		return fmt.Sprintf("%dh", int(duration/time.Hour))
	case duration%time.Minute == 0:
		return fmt.Sprintf("%dm", int(duration/time.Minute))
	default:
		return fmt.Sprintf("%ds", int(duration/time.Second))
	}
}

func aggregateResultViewCandles(candles []bt.Candle, resolution time.Duration) []bt.Candle {
	if len(candles) == 0 || resolution <= 0 {
		return append([]bt.Candle(nil), candles...)
	}
	out := make([]bt.Candle, 0, len(candles))
	var current *bt.Candle
	var currentBucket int64
	var volumeSum float64
	var volumeOK bool
	for _, candle := range candles {
		parsed, err := parseResultViewTime(candle.Time)
		if err != nil {
			continue
		}
		bucket := parsed.UTC().Unix() / int64(resolution.Seconds())
		if current == nil || bucket != currentBucket {
			if current != nil {
				if volumeOK {
					current.Volume = strconv.FormatFloat(volumeSum, 'f', -1, 64)
				} else {
					current.Volume = ""
				}
				out = append(out, *current)
			}
			clone := candle
			clone.Time = time.Unix(bucket*int64(resolution.Seconds()), 0).UTC().Format(time.RFC3339Nano)
			current = &clone
			currentBucket = bucket
			volumeSum, volumeOK = parseResultViewFloat(candle.Volume)
			continue
		}
		current.High = resultViewMaxString(current.High, candle.High)
		current.Low = resultViewMinString(current.Low, candle.Low)
		current.Close = candle.Close
		if volume, ok := parseResultViewFloat(candle.Volume); ok && volumeOK {
			volumeSum += volume
		} else {
			volumeOK = false
		}
	}
	if current != nil {
		if volumeOK {
			current.Volume = strconv.FormatFloat(volumeSum, 'f', -1, 64)
		} else {
			current.Volume = ""
		}
		out = append(out, *current)
	}
	return out
}

func resultViewMaxString(left string, right string) string {
	leftValue, leftOK := parseResultViewFloat(left)
	rightValue, rightOK := parseResultViewFloat(right)
	if leftOK && rightOK && rightValue > leftValue {
		return right
	}
	return left
}

func resultViewMinString(left string, right string) string {
	leftValue, leftOK := parseResultViewFloat(left)
	rightValue, rightOK := parseResultViewFloat(right)
	if leftOK && rightOK && rightValue < leftValue {
		return right
	}
	return left
}
