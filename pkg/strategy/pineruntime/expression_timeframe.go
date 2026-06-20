package pineruntime

import (
	"fmt"
	"strings"
	"time"

	exprast "github.com/expr-lang/expr/ast"
)

func evaluateTimeframeChangeExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 1 {
		return nil, fmt.Errorf("timeframe.change() requires a static timeframe")
	}
	value, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	timeframe := strings.TrimSpace(stringifyPineValue(value))
	if timeframe == "" || timeframe == "na" {
		return nil, fmt.Errorf("timeframe.change() requires a static timeframe")
	}
	if scope == nil || scope.currentKline == nil {
		return false, nil
	}
	current := pineBarTime(scope.currentKline)
	if current.IsZero() {
		return false, nil
	}
	if scope.runtime == nil || !scope.runtime.hasPreviousBarTime {
		return true, nil
	}
	return pineTimeframeBucketChanged(scope.runtime.previousBarTime, current, timeframe, pineExchangeLocation(scope)), nil
}

func evaluateTimeframeInSecondsExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) > 1 {
		return nil, fmt.Errorf("timeframe.in_seconds() accepts at most one timeframe")
	}
	if len(arguments) == 0 {
		if scope == nil {
			return nil, fmt.Errorf("timeframe.in_seconds() requires a runtime timeframe")
		}
		duration, ok := pineIntervalDuration(scope.runtimeInterval())
		if !ok {
			return nil, fmt.Errorf("timeframe.in_seconds() cannot resolve runtime timeframe")
		}
		return float64(duration / time.Second), nil
	}
	value, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	duration, ok := pineStaticTimeframeDuration(strings.TrimSpace(stringifyPineValue(value)))
	if !ok {
		return nil, fmt.Errorf("timeframe.in_seconds() requires a supported static timeframe")
	}
	return float64(duration / time.Second), nil
}

func pineTimeframeBucketChanged(previous time.Time, current time.Time, timeframe string, location *time.Location) bool {
	unit, duration, ok := pineStaticTimeframeBucket(timeframe)
	if !ok {
		return false
	}
	if location == nil {
		location = time.UTC
	}
	switch unit {
	case "month":
		previous = previous.In(location)
		current = current.In(location)
		py, pm, _ := previous.Date()
		cy, cm, _ := current.Date()
		return py != cy || pm != cm
	case "week":
		previous = previous.In(location)
		current = current.In(location)
		py, pw := previous.ISOWeek()
		cy, cw := current.ISOWeek()
		return py != cy || pw != cw
	case "day":
		previous = previous.In(location)
		current = current.In(location)
		py, pm, pd := previous.Date()
		cy, cm, cd := current.Date()
		return py != cy || pm != cm || pd != cd
	default:
		if duration <= 0 {
			return false
		}
		return previous.Unix()/int64(duration.Seconds()) != current.Unix()/int64(duration.Seconds())
	}
}

func pineStaticTimeframeDuration(value string) (time.Duration, bool) {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	if trimmed == "" || trimmed == "NA" {
		return 0, false
	}
	switch trimmed {
	case "D", "1D":
		return 24 * time.Hour, true
	case "W", "1W":
		return 7 * 24 * time.Hour, true
	case "M", "1M":
		return 30 * 24 * time.Hour, true
	}
	for _, suffix := range []struct {
		value string
		unit  time.Duration
	}{
		{value: "S", unit: time.Second},
		{value: "D", unit: 24 * time.Hour},
		{value: "W", unit: 7 * 24 * time.Hour},
		{value: "M", unit: 30 * 24 * time.Hour},
	} {
		if strings.HasSuffix(trimmed, suffix.value) {
			value := strings.TrimSuffix(trimmed, suffix.value)
			if value == "" {
				value = "1"
			}
			multiplier, ok := parsePositiveTimeframeNumber(value)
			if !ok {
				return 0, false
			}
			return time.Duration(multiplier) * suffix.unit, true
		}
	}
	minutes, ok := parsePositiveTimeframeNumber(trimmed)
	if !ok {
		return 0, false
	}
	return time.Duration(minutes) * time.Minute, true
}

func parsePositiveTimeframeNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	result := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
		result = result*10 + int(char-'0')
	}
	return result, result > 0
}

func pineStaticTimeframeBucket(value string) (string, time.Duration, bool) {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	switch trimmed {
	case "D", "1D":
		return "day", 24 * time.Hour, true
	case "W", "1W":
		return "week", 7 * 24 * time.Hour, true
	case "M", "1M":
		return "month", 0, true
	}
	if strings.HasSuffix(trimmed, "S") {
		duration, ok := pineStaticTimeframeDuration(trimmed)
		return "intraday", duration, ok
	}
	if minutes, ok := parsePositiveTimeframeNumber(trimmed); ok {
		duration := time.Duration(minutes) * time.Minute
		return "intraday", duration, true
	}
	return "", 0, false
}
