package ir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

type sarPlannerConfig struct {
	start     float64
	increment float64
	maximum   float64
}

func sarPlannerKey(config sarPlannerConfig) string {
	return "sar:" +
		strconv.FormatFloat(config.start, 'f', -1, 64) + ":" +
		strconv.FormatFloat(config.increment, 'f', -1, 64) + ":" +
		strconv.FormatFloat(config.maximum, 'f', -1, 64)
}

func securitySourcePlannerKey(source string, timeUnit string, lookback int) string {
	key := "security_source:" + timeUnit + ":" + source
	if lookback > 0 {
		key += ":" + strconv.Itoa(lookback)
	}
	return key
}

func parseSourcePeriodArgs(lineNumber int, name string, args []string, defaultSource string, defaultPeriod string) (string, int, error) {
	sourceText, periodText := defaultSource, defaultPeriod
	if len(args) == 1 {
		periodText = strings.TrimSpace(args[0])
	} else if len(args) >= 2 {
		sourceText = strings.TrimSpace(args[0])
		periodText = strings.TrimSpace(args[1])
	}
	source, ok := indicatorbinding.ParsePriceSource(sourceText)
	if !ok {
		return "", 0, fmt.Errorf("pine line %d: %s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, name, sourceText)
	}
	period, err := indicatorbinding.ParsePositiveInt(periodText)
	if err != nil {
		return "", 0, fmt.Errorf("pine line %d: %s() length must be a positive integer", lineNumber, name)
	}
	return source, period, nil
}

func sourcePeriodKey(prefix string, source string, period int, legacySource string) string {
	if strings.TrimSpace(source) == "" || source == legacySource {
		return prefix + ":" + strconv.Itoa(period)
	}
	return prefix + ":" + source + ":" + strconv.Itoa(period)
}

func sourcePeriodArgs(source string, period int, legacySource string) []string {
	periodText := strconv.Itoa(period)
	if strings.TrimSpace(source) == "" || source == legacySource {
		return []string{periodText}
	}
	return []string{source, periodText}
}

func parseStochSource(value string) (string, bool) {
	source, ok := indicatorbinding.ParsePriceSource(value)
	if !ok || source == "volume" {
		return "", false
	}
	return source, true
}
