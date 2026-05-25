package quickjs

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var indicatorKeyPattern = regexp.MustCompile(`ctx\.indicators\[(?:"([^"]+)"|'([^']+)')\]`)

type indicatorRequirements struct {
	ma             []movingAverageConfig
	rsi            []int
	macd           []macdConfig
	bollinger      []bollingerConfig
	kdj            []kdjConfig
	atr            []int
	cci            []int
	williamsR      []int
	rsiDivergence  []rsiDivergenceConfig
	macdDivergence []macdDivergenceConfig
	kdjDivergence  []kdjDivergenceConfig
}

type movingAverageConfig struct {
	averageType string
	period      int
}

type macdConfig struct {
	fastPeriod   int
	slowPeriod   int
	signalPeriod int
}

type bollingerConfig struct {
	period     int
	multiplier float64
}

type kdjConfig struct {
	period int
	m1     int
	m2     int
}

type rsiDivergenceConfig struct {
	period    int
	direction string
	lookback  int
}

type macdDivergenceConfig struct {
	fastPeriod   int
	slowPeriod   int
	signalPeriod int
	direction    string
	lookback     int
}

type kdjDivergenceConfig struct {
	period    int
	m1        int
	m2        int
	direction string
	lookback  int
}

func parseIndicatorRequirements(script string) indicatorRequirements {
	maSet := map[movingAverageConfig]struct{}{}
	rsiSet := map[int]struct{}{}
	macdSet := map[macdConfig]struct{}{}
	bollingerSet := map[bollingerConfig]struct{}{}
	kdjSet := map[kdjConfig]struct{}{}
	atrSet := map[int]struct{}{}
	cciSet := map[int]struct{}{}
	williamsRSet := map[int]struct{}{}
	rsiDivergenceSet := map[rsiDivergenceConfig]struct{}{}
	macdDivergenceSet := map[macdDivergenceConfig]struct{}{}
	kdjDivergenceSet := map[kdjDivergenceConfig]struct{}{}

	for _, match := range indicatorKeyPattern.FindAllStringSubmatch(script, -1) {
		key := strings.TrimSpace(firstNonEmpty(match[1], match[2]))
		parts := strings.Split(key, ":")
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "ma":
			config, ok := parseMovingAverageConfig(parts)
			if ok {
				maSet[config] = struct{}{}
			}
		case "rsi":
			period, ok := parsePositiveInt(parts[1])
			if ok {
				rsiSet[period] = struct{}{}
			}
		case "macd":
			if len(parts) != 4 {
				continue
			}
			fast, fastOK := parsePositiveInt(parts[1])
			slow, slowOK := parsePositiveInt(parts[2])
			signal, signalOK := parsePositiveInt(parts[3])
			if fastOK && slowOK && signalOK {
				macdSet[macdConfig{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal}] = struct{}{}
			}
		case "bollinger":
			if len(parts) != 3 {
				continue
			}
			period, periodOK := parsePositiveInt(parts[1])
			multiplier, multiplierErr := strconv.ParseFloat(parts[2], 64)
			if periodOK && multiplierErr == nil {
				bollingerSet[bollingerConfig{period: period, multiplier: multiplier}] = struct{}{}
			}
		case "kdj":
			if len(parts) != 4 {
				continue
			}
			period, periodOK := parsePositiveInt(parts[1])
			m1, m1OK := parsePositiveInt(parts[2])
			m2, m2OK := parsePositiveInt(parts[3])
			if periodOK && m1OK && m2OK {
				kdjSet[kdjConfig{period: period, m1: m1, m2: m2}] = struct{}{}
			}
		case "atr":
			period, ok := parsePositiveInt(parts[1])
			if ok {
				atrSet[period] = struct{}{}
			}
		case "cci":
			period, ok := parsePositiveInt(parts[1])
			if ok {
				cciSet[period] = struct{}{}
			}
		case "williamsr":
			period, ok := parsePositiveInt(parts[1])
			if ok {
				williamsRSet[period] = struct{}{}
			}
		case "divergence":
			if len(parts) < 5 {
				continue
			}
			direction := strings.TrimSpace(parts[len(parts)-2])
			lookback, lookbackOK := parsePositiveInt(parts[len(parts)-1])
			if !lookbackOK || (direction != "top" && direction != "bottom") {
				continue
			}
			switch parts[1] {
			case "rsi":
				if len(parts) != 5 {
					continue
				}
				period, ok := parsePositiveInt(parts[2])
				if ok {
					rsiDivergenceSet[rsiDivergenceConfig{period: period, direction: direction, lookback: lookback}] = struct{}{}
				}
			case "macd":
				if len(parts) != 7 {
					continue
				}
				fast, fastOK := parsePositiveInt(parts[2])
				slow, slowOK := parsePositiveInt(parts[3])
				signal, signalOK := parsePositiveInt(parts[4])
				if fastOK && slowOK && signalOK {
					macdDivergenceSet[macdDivergenceConfig{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal, direction: direction, lookback: lookback}] = struct{}{}
				}
			case "kdj":
				if len(parts) != 7 {
					continue
				}
				period, periodOK := parsePositiveInt(parts[2])
				m1, m1OK := parsePositiveInt(parts[3])
				m2, m2OK := parsePositiveInt(parts[4])
				if periodOK && m1OK && m2OK {
					kdjDivergenceSet[kdjDivergenceConfig{period: period, m1: m1, m2: m2, direction: direction, lookback: lookback}] = struct{}{}
				}
			}
		}
	}

	return indicatorRequirements{
		ma:             sortedMovingAverageConfigs(maSet),
		rsi:            sortedInts(rsiSet),
		macd:           sortedMACDConfigs(macdSet),
		bollinger:      sortedBollingerConfigs(bollingerSet),
		kdj:            sortedKDJConfigs(kdjSet),
		atr:            sortedInts(atrSet),
		cci:            sortedInts(cciSet),
		williamsR:      sortedInts(williamsRSet),
		rsiDivergence:  sortedRSIDivergenceConfigs(rsiDivergenceSet),
		macdDivergence: sortedMACDDivergenceConfigs(macdDivergenceSet),
		kdjDivergence:  sortedKDJDivergenceConfigs(kdjDivergenceSet),
	}
}

func (r indicatorRequirements) isEmpty() bool {
	return len(r.ma) == 0 &&
		len(r.rsi) == 0 &&
		len(r.macd) == 0 &&
		len(r.bollinger) == 0 &&
		len(r.kdj) == 0 &&
		len(r.atr) == 0 &&
		len(r.cci) == 0 &&
		len(r.williamsR) == 0 &&
		len(r.rsiDivergence) == 0 &&
		len(r.macdDivergence) == 0 &&
		len(r.kdjDivergence) == 0
}

func maIndicatorKey(averageType string, period int) string {
	return "ma:" + normalizeMovingAverageType(averageType) + ":" + strconv.Itoa(period)
}

func legacyMAIndicatorKey(period int) string {
	return "ma:" + strconv.Itoa(period)
}

func rsiIndicatorKey(period int) string {
	return "rsi:" + strconv.Itoa(period)
}

func macdIndicatorKey(fastPeriod, slowPeriod, signalPeriod int) string {
	return "macd:" + strconv.Itoa(fastPeriod) + ":" + strconv.Itoa(slowPeriod) + ":" + strconv.Itoa(signalPeriod)
}

func bollingerIndicatorKey(period int, multiplier float64) string {
	return "bollinger:" + strconv.Itoa(period) + ":" + strconv.FormatFloat(multiplier, 'f', -1, 64)
}

func kdjIndicatorKey(period, m1, m2 int) string {
	return "kdj:" + strconv.Itoa(period) + ":" + strconv.Itoa(m1) + ":" + strconv.Itoa(m2)
}

func atrIndicatorKey(period int) string {
	return "atr:" + strconv.Itoa(period)
}

func cciIndicatorKey(period int) string {
	return "cci:" + strconv.Itoa(period)
}

func williamsRIndicatorKey(period int) string {
	return "williamsr:" + strconv.Itoa(period)
}

func rsiDivergenceIndicatorKey(period int, direction string, lookback int) string {
	return "divergence:rsi:" + strconv.Itoa(period) + ":" + direction + ":" + strconv.Itoa(lookback)
}

func macdDivergenceIndicatorKey(fastPeriod, slowPeriod, signalPeriod int, direction string, lookback int) string {
	return "divergence:macd:" + strconv.Itoa(fastPeriod) + ":" + strconv.Itoa(slowPeriod) + ":" + strconv.Itoa(signalPeriod) + ":" + direction + ":" + strconv.Itoa(lookback)
}

func kdjDivergenceIndicatorKey(period, m1, m2 int, direction string, lookback int) string {
	return "divergence:kdj:" + strconv.Itoa(period) + ":" + strconv.Itoa(m1) + ":" + strconv.Itoa(m2) + ":" + direction + ":" + strconv.Itoa(lookback)
}

func sortedMovingAverageConfigs(values map[movingAverageConfig]struct{}) []movingAverageConfig {
	result := make([]movingAverageConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		return result[left].averageType < result[right].averageType
	})
	return result
}

func sortedInts(values map[int]struct{}) []int {
	result := make([]int, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func sortedMACDConfigs(values map[macdConfig]struct{}) []macdConfig {
	result := make([]macdConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].fastPeriod != result[right].fastPeriod {
			return result[left].fastPeriod < result[right].fastPeriod
		}
		if result[left].slowPeriod != result[right].slowPeriod {
			return result[left].slowPeriod < result[right].slowPeriod
		}
		return result[left].signalPeriod < result[right].signalPeriod
	})
	return result
}

func sortedBollingerConfigs(values map[bollingerConfig]struct{}) []bollingerConfig {
	result := make([]bollingerConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		return result[left].multiplier < result[right].multiplier
	})
	return result
}

func sortedKDJConfigs(values map[kdjConfig]struct{}) []kdjConfig {
	result := make([]kdjConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		if result[left].m1 != result[right].m1 {
			return result[left].m1 < result[right].m1
		}
		return result[left].m2 < result[right].m2
	})
	return result
}

func sortedRSIDivergenceConfigs(values map[rsiDivergenceConfig]struct{}) []rsiDivergenceConfig {
	result := make([]rsiDivergenceConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		if result[left].direction != result[right].direction {
			return result[left].direction < result[right].direction
		}
		return result[left].lookback < result[right].lookback
	})
	return result
}

func sortedMACDDivergenceConfigs(values map[macdDivergenceConfig]struct{}) []macdDivergenceConfig {
	result := make([]macdDivergenceConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].fastPeriod != result[right].fastPeriod {
			return result[left].fastPeriod < result[right].fastPeriod
		}
		if result[left].slowPeriod != result[right].slowPeriod {
			return result[left].slowPeriod < result[right].slowPeriod
		}
		if result[left].signalPeriod != result[right].signalPeriod {
			return result[left].signalPeriod < result[right].signalPeriod
		}
		if result[left].direction != result[right].direction {
			return result[left].direction < result[right].direction
		}
		return result[left].lookback < result[right].lookback
	})
	return result
}

func sortedKDJDivergenceConfigs(values map[kdjDivergenceConfig]struct{}) []kdjDivergenceConfig {
	result := make([]kdjDivergenceConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		if result[left].m1 != result[right].m1 {
			return result[left].m1 < result[right].m1
		}
		if result[left].m2 != result[right].m2 {
			return result[left].m2 < result[right].m2
		}
		if result[left].direction != result[right].direction {
			return result[left].direction < result[right].direction
		}
		return result[left].lookback < result[right].lookback
	})
	return result
}

func parsePositiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil && parsed > 0
}

func parseMovingAverageConfig(parts []string) (movingAverageConfig, bool) {
	if len(parts) == 2 {
		period, ok := parsePositiveInt(parts[1])
		if !ok {
			return movingAverageConfig{}, false
		}
		return movingAverageConfig{averageType: "MA", period: period}, true
	}
	if len(parts) != 3 {
		return movingAverageConfig{}, false
	}
	period, ok := parsePositiveInt(parts[2])
	if !ok {
		return movingAverageConfig{}, false
	}
	return movingAverageConfig{
		averageType: normalizeMovingAverageType(parts[1]),
		period:      period,
	}, true
}

func normalizeMovingAverageType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "EMA", "SMA", "SMMA", "LWMA", "TMA", "EXPMA", "HMA", "VWMA", "BOLL":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "MA"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
