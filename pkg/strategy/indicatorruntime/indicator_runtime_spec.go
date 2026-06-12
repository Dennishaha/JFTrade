package indicatorruntime

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

var indicatorKeyPattern = regexp.MustCompile(`ctx\.indicators\[(?:"([^"]+)"|'([^']+)')\]`)

const (
	tradingSessionMinutesPerDay   = 390
	tradingSessionMinutesPerWeek  = tradingSessionMinutesPerDay * 5
	tradingSessionMinutesPerMonth = tradingSessionMinutesPerDay * 20
)

type indicatorRequirements struct {
	ma             []movingAverageConfig
	rsi            []int
	macd           []macdConfig
	bollinger      []bollingerConfig
	kdj            []kdjConfig
	atr            []int
	stdev          []int
	cci            []int
	williamsR      []int
	stopLoss       []stopLossConfig
	rsiDivergence  []rsiDivergenceConfig
	macdDivergence []macdDivergenceConfig
	kdjDivergence  []kdjDivergenceConfig
}

type movingAverageConfig struct {
	averageType string
	period      int
	timeUnit    string
}

type stopLossConfig struct {
	mode         string
	direction    string
	timeValue    int
	timeUnit     string
	percentage   float64
	windowPolicy string
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
	keys := make([]string, 0)
	for _, match := range indicatorKeyPattern.FindAllStringSubmatch(script, -1) {
		key := strings.TrimSpace(firstNonEmpty(match[1], match[2]))
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	requirements, _ := parseIndicatorRequirementKeys(keys, false)
	return requirements
}

func indicatorRequirementsFromPlan(plan strategyir.Requirements) (indicatorRequirements, error) {
	keys := make([]string, 0, len(plan.Indicators))
	for _, requirement := range plan.Indicators {
		key := strings.TrimSpace(requirement.Key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	return parseIndicatorRequirementKeys(keys, true)
}

func parseIndicatorRequirementKeys(keys []string, strict bool) (indicatorRequirements, error) {
	maSet := map[movingAverageConfig]struct{}{}
	rsiSet := map[int]struct{}{}
	macdSet := map[macdConfig]struct{}{}
	bollingerSet := map[bollingerConfig]struct{}{}
	kdjSet := map[kdjConfig]struct{}{}
	atrSet := map[int]struct{}{}
	stdevSet := map[int]struct{}{}
	cciSet := map[int]struct{}{}
	williamsRSet := map[int]struct{}{}
	stopLossSet := map[stopLossConfig]struct{}{}
	rsiDivergenceSet := map[rsiDivergenceConfig]struct{}{}
	macdDivergenceSet := map[macdDivergenceConfig]struct{}{}
	kdjDivergenceSet := map[kdjDivergenceConfig]struct{}{}

	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		parts := strings.Split(key, ":")
		if len(parts) < 2 {
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid indicator key: %s", key)
			}
			continue
		}

		switch parts[0] {
		case "ma":
			config, ok := parseMovingAverageConfig(parts)
			if ok {
				maSet[config] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid moving average key: %s", key)
			}
		case "rsi":
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid rsi key: %s", key)
				}
				continue
			}
			period, ok := parsePositiveInt(parts[1])
			if ok {
				rsiSet[period] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid rsi key: %s", key)
			}
		case "macd":
			if len(parts) != 4 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid macd key: %s", key)
				}
				continue
			}
			fast, fastOK := parsePositiveInt(parts[1])
			slow, slowOK := parsePositiveInt(parts[2])
			signal, signalOK := parsePositiveInt(parts[3])
			if fastOK && slowOK && signalOK {
				macdSet[macdConfig{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid macd key: %s", key)
			}
		case "bollinger":
			if len(parts) != 3 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid bollinger key: %s", key)
				}
				continue
			}
			period, periodOK := parsePositiveInt(parts[1])
			multiplier, multiplierErr := strconv.ParseFloat(parts[2], 64)
			if periodOK && multiplierErr == nil && multiplier > 0 {
				bollingerSet[bollingerConfig{period: period, multiplier: multiplier}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid bollinger key: %s", key)
			}
		case "kdj":
			if len(parts) != 4 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid kdj key: %s", key)
				}
				continue
			}
			period, periodOK := parsePositiveInt(parts[1])
			m1, m1OK := parsePositiveInt(parts[2])
			m2, m2OK := parsePositiveInt(parts[3])
			if periodOK && m1OK && m2OK {
				kdjSet[kdjConfig{period: period, m1: m1, m2: m2}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid kdj key: %s", key)
			}
		case "atr":
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid atr key: %s", key)
				}
				continue
			}
			period, ok := parsePositiveInt(parts[1])
			if ok {
				atrSet[period] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid atr key: %s", key)
			}
		case "stdev":
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid stdev key: %s", key)
				}
				continue
			}
			period, ok := parsePositiveInt(parts[1])
			if ok {
				stdevSet[period] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid stdev key: %s", key)
			}
		case "cci":
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid cci key: %s", key)
				}
				continue
			}
			period, ok := parsePositiveInt(parts[1])
			if ok {
				cciSet[period] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid cci key: %s", key)
			}
		case "williamsr":
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid williamsr key: %s", key)
				}
				continue
			}
			period, ok := parsePositiveInt(parts[1])
			if ok {
				williamsRSet[period] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid williamsr key: %s", key)
			}
		case "sl", "risk":
			config, ok := parseStopLossConfig(parts)
			if ok {
				stopLossSet[config] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid risk key: %s", key)
			}
		case "divergence":
			if len(parts) < 5 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
				continue
			}
			direction := strings.TrimSpace(parts[len(parts)-2])
			lookback, lookbackOK := parsePositiveInt(parts[len(parts)-1])
			if !lookbackOK || (direction != "top" && direction != "bottom") {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
				continue
			}
			switch parts[1] {
			case "rsi":
				if len(parts) != 5 {
					if strict {
						return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
					}
					continue
				}
				period, ok := parsePositiveInt(parts[2])
				if ok {
					rsiDivergenceSet[rsiDivergenceConfig{period: period, direction: direction, lookback: lookback}] = struct{}{}
					continue
				}
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
			case "macd":
				if len(parts) != 7 {
					if strict {
						return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
					}
					continue
				}
				fast, fastOK := parsePositiveInt(parts[2])
				slow, slowOK := parsePositiveInt(parts[3])
				signal, signalOK := parsePositiveInt(parts[4])
				if fastOK && slowOK && signalOK {
					macdDivergenceSet[macdDivergenceConfig{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal, direction: direction, lookback: lookback}] = struct{}{}
					continue
				}
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
			case "kdj":
				if len(parts) != 7 {
					if strict {
						return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
					}
					continue
				}
				period, periodOK := parsePositiveInt(parts[2])
				m1, m1OK := parsePositiveInt(parts[3])
				m2, m2OK := parsePositiveInt(parts[4])
				if periodOK && m1OK && m2OK {
					kdjDivergenceSet[kdjDivergenceConfig{period: period, m1: m1, m2: m2, direction: direction, lookback: lookback}] = struct{}{}
					continue
				}
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
			default:
				if strict {
					return indicatorRequirements{}, fmt.Errorf("unsupported divergence key: %s", key)
				}
			}
		default:
			if strict {
				return indicatorRequirements{}, fmt.Errorf("unsupported indicator key: %s", key)
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
		stdev:          sortedInts(stdevSet),
		cci:            sortedInts(cciSet),
		williamsR:      sortedInts(williamsRSet),
		stopLoss:       sortedStopLossConfigs(stopLossSet),
		rsiDivergence:  sortedRSIDivergenceConfigs(rsiDivergenceSet),
		macdDivergence: sortedMACDDivergenceConfigs(macdDivergenceSet),
		kdjDivergence:  sortedKDJDivergenceConfigs(kdjDivergenceSet),
	}, nil
}

func (r indicatorRequirements) isEmpty() bool {
	return len(r.ma) == 0 &&
		len(r.rsi) == 0 &&
		len(r.macd) == 0 &&
		len(r.bollinger) == 0 &&
		len(r.kdj) == 0 &&
		len(r.atr) == 0 &&
		len(r.stdev) == 0 &&
		len(r.cci) == 0 &&
		len(r.williamsR) == 0 &&
		len(r.stopLoss) == 0 &&
		len(r.rsiDivergence) == 0 &&
		len(r.macdDivergence) == 0 &&
		len(r.kdjDivergence) == 0
}

func maIndicatorKey(config movingAverageConfig) string {
	base := "ma:" + normalizeMovingAverageType(config.averageType) + ":" + strconv.Itoa(config.period)
	timeUnit := normalizeIndicatorTimeUnit(config.timeUnit)
	if timeUnit == "" {
		return base
	}
	return base + ":" + timeUnit
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

func stdevIndicatorKey(period int) string {
	return "stdev:" + strconv.Itoa(period)
}

func cciIndicatorKey(period int) string {
	return "cci:" + strconv.Itoa(period)
}

func williamsRIndicatorKey(period int) string {
	return "williamsr:" + strconv.Itoa(period)
}

func stopLossIndicatorKey(config stopLossConfig) string {
	timeUnit := normalizeIndicatorTimeUnit(config.timeUnit)
	if timeUnit == "" {
		timeUnit = "bar"
	}
	mode := normalizeStopLossMode(config.mode)
	windowPolicy := normalizeStopLossWindowPolicy(config.windowPolicy)
	if mode == "stopLoss" && windowPolicy == "continuous" {
		return "sl:" + normalizeStopLossDirection(config.direction) + ":" + strconv.Itoa(config.timeValue) + ":" + timeUnit + ":" + strconv.FormatFloat(config.percentage, 'f', -1, 64)
	}
	return "risk:" + mode + ":" + normalizeStopLossDirection(config.direction) + ":" + strconv.Itoa(config.timeValue) + ":" + timeUnit + ":" + strconv.FormatFloat(config.percentage, 'f', -1, 64) + ":" + windowPolicy
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
		if result[left].averageType != result[right].averageType {
			return result[left].averageType < result[right].averageType
		}
		return normalizeIndicatorTimeUnit(result[left].timeUnit) < normalizeIndicatorTimeUnit(result[right].timeUnit)
	})
	return result
}

func sortedStopLossConfigs(values map[stopLossConfig]struct{}) []stopLossConfig {
	result := make([]stopLossConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if normalizeStopLossMode(result[left].mode) != normalizeStopLossMode(result[right].mode) {
			return normalizeStopLossMode(result[left].mode) < normalizeStopLossMode(result[right].mode)
		}
		if result[left].timeValue != result[right].timeValue {
			return result[left].timeValue < result[right].timeValue
		}
		if normalizeIndicatorTimeUnit(result[left].timeUnit) != normalizeIndicatorTimeUnit(result[right].timeUnit) {
			return normalizeIndicatorTimeUnit(result[left].timeUnit) < normalizeIndicatorTimeUnit(result[right].timeUnit)
		}
		if result[left].percentage != result[right].percentage {
			return result[left].percentage < result[right].percentage
		}
		if normalizeStopLossWindowPolicy(result[left].windowPolicy) != normalizeStopLossWindowPolicy(result[right].windowPolicy) {
			return normalizeStopLossWindowPolicy(result[left].windowPolicy) < normalizeStopLossWindowPolicy(result[right].windowPolicy)
		}
		return normalizeStopLossDirection(result[left].direction) < normalizeStopLossDirection(result[right].direction)
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
	if len(parts) == 3 {
		if period, ok := parsePositiveInt(parts[1]); ok {
			return movingAverageConfig{
				averageType: "MA",
				period:      period,
				timeUnit:    normalizeIndicatorTimeUnit(parts[2]),
			}, true
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
	if len(parts) != 4 {
		return movingAverageConfig{}, false
	}
	period, ok := parsePositiveInt(parts[2])
	if !ok {
		return movingAverageConfig{}, false
	}
	return movingAverageConfig{
		averageType: normalizeMovingAverageType(parts[1]),
		period:      period,
		timeUnit:    normalizeIndicatorTimeUnit(parts[3]),
	}, true
}

func parseStopLossConfig(parts []string) (stopLossConfig, bool) {
	switch firstNonEmpty(parts[0]) {
	case "sl":
		if len(parts) != 5 {
			return stopLossConfig{}, false
		}
		timeValue, ok := parsePositiveInt(parts[2])
		if !ok {
			return stopLossConfig{}, false
		}
		percentage, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		if err != nil || percentage <= 0 {
			return stopLossConfig{}, false
		}
		return stopLossConfig{
			mode:         "stopLoss",
			direction:    normalizeStopLossDirection(parts[1]),
			timeValue:    timeValue,
			timeUnit:     normalizeIndicatorTimeUnit(parts[3]),
			percentage:   percentage,
			windowPolicy: "continuous",
		}, true
	case "risk":
		if len(parts) != 7 {
			return stopLossConfig{}, false
		}
		mode, ok := parseStopLossMode(parts[1])
		if !ok {
			return stopLossConfig{}, false
		}
		timeValue, ok := parsePositiveInt(parts[3])
		if !ok {
			return stopLossConfig{}, false
		}
		percentage, err := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64)
		if err != nil || percentage <= 0 {
			return stopLossConfig{}, false
		}
		windowPolicy, ok := parseStopLossWindowPolicy(parts[6])
		if !ok {
			return stopLossConfig{}, false
		}
		return stopLossConfig{
			mode:         mode,
			direction:    normalizeStopLossDirection(parts[2]),
			timeValue:    timeValue,
			timeUnit:     normalizeIndicatorTimeUnit(parts[4]),
			percentage:   percentage,
			windowPolicy: windowPolicy,
		}, true
	default:
		return stopLossConfig{}, false
	}
}

func resolveBarCount(period int, timeUnit string, intervalMinutes int) int {
	if period <= 0 {
		return 0
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "":
		return period
	case "minute":
		return max(1, int(math.Ceil(float64(period)/float64(intervalMinutes))))
	case "hour":
		return max(1, int(math.Ceil(float64(period*60)/float64(intervalMinutes))))
	case "day":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerDay)/float64(intervalMinutes))))
	case "week":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerWeek)/float64(intervalMinutes))))
	case "month":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerMonth)/float64(intervalMinutes))))
	default:
		return period
	}
}

func normalizeMovingAverageType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "EMA", "SMA", "SMMA", "LWMA", "TMA", "EXPMA", "HMA", "VWMA", "BOLL":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "MA"
	}
}

func normalizeIndicatorTimeUnit(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "bar", "bars":
		return ""
	case "m", "min", "mins", "minute", "minutes":
		return "minute"
	case "h", "hr", "hrs", "hour", "hours":
		return "hour"
	case "d", "day", "days":
		return "day"
	case "w", "week", "weeks":
		return "week"
	case "mo", "mon", "month", "months":
		return "month"
	default:
		return ""
	}
}

func normalizeStopLossDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "long":
		return "long"
	case "short":
		return "short"
	default:
		return "auto"
	}
}

func normalizeStopLossMode(value string) string {
	switch strings.TrimSpace(value) {
	case "takeProfit":
		return "takeProfit"
	case "trailingStop":
		return "trailingStop"
	default:
		return "stopLoss"
	}
}

func parseStopLossMode(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "stopLoss", "takeProfit", "trailingStop":
		return strings.TrimSpace(value), true
	default:
		return "", false
	}
}

func normalizeStopLossWindowPolicy(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "session") {
		return "session"
	}
	return "continuous"
}

func parseStopLossWindowPolicy(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "continuous", "session":
		return strings.TrimSpace(value), true
	default:
		return "", false
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
