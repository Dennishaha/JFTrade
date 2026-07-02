package indicatorruntime

import (
	"reflect"
	"testing"
)

func TestSortedIndicatorConfigsRespectDeterministicBusinessPriority(t *testing.T) {
	assertConfigOrder(t, sortedWindowConfigs(map[windowConfig]struct{}{
		{function: "lowest", source: "close", period: 5}:   {},
		{function: "highest", source: "volume", period: 2}: {},
		{function: "highest", source: "close", period: 9}:  {},
		{function: "highest", source: "close", period: 3}:  {},
	}), []windowConfig{
		{function: "highest", source: "close", period: 3},
		{function: "highest", source: "close", period: 9},
		{function: "highest", source: "volume", period: 2},
		{function: "lowest", source: "close", period: 5},
	})

	assertConfigOrder(t, sortedSourcePeriodConfigs(map[sourcePeriodConfig]struct{}{
		{source: "open", period: 20, timeUnit: "day"}:   {},
		{source: "volume", period: 10, timeUnit: "day"}: {},
		{source: "close", period: 10, timeUnit: "day"}:  {},
		{source: "close", period: 10, timeUnit: "hour"}: {},
	}), []sourcePeriodConfig{
		{source: "close", period: 10, timeUnit: "day"},
		{source: "volume", period: 10, timeUnit: "day"},
		{source: "close", period: 10, timeUnit: "hour"},
		{source: "open", period: 20, timeUnit: "day"},
	})

	assertConfigOrder(t, sortedDMIConfigs(map[dmiConfig]struct{}{
		{diLength: 14, adxSmoothing: 21}: {},
		{diLength: 7, adxSmoothing: 14}:  {},
		{diLength: 14, adxSmoothing: 10}: {},
	}), []dmiConfig{
		{diLength: 7, adxSmoothing: 14},
		{diLength: 14, adxSmoothing: 10},
		{diLength: 14, adxSmoothing: 21},
	})

	assertConfigOrder(t, sortedSupertrendConfigs(map[supertrendConfig]struct{}{
		{factor: 3.0, atrPeriod: 20}: {},
		{factor: 2.0, atrPeriod: 10}: {},
		{factor: 1.5, atrPeriod: 10}: {},
	}), []supertrendConfig{
		{factor: 1.5, atrPeriod: 10},
		{factor: 2.0, atrPeriod: 10},
		{factor: 3.0, atrPeriod: 20},
	})

	assertConfigOrder(t, sortedSARConfigs(map[sarConfig]struct{}{
		{start: 0.03, increment: 0.02, maximum: 0.2}: {},
		{start: 0.02, increment: 0.03, maximum: 0.2}: {},
		{start: 0.02, increment: 0.02, maximum: 0.3}: {},
		{start: 0.02, increment: 0.02, maximum: 0.2}: {},
	}), []sarConfig{
		{start: 0.02, increment: 0.02, maximum: 0.2},
		{start: 0.02, increment: 0.02, maximum: 0.3},
		{start: 0.02, increment: 0.03, maximum: 0.2},
		{start: 0.03, increment: 0.02, maximum: 0.2},
	})
}

func TestSortedRiskAndDivergenceConfigsRespectTieBreakers(t *testing.T) {
	assertConfigOrder(t, sortedStopLossConfigs(map[stopLossConfig]struct{}{
		{mode: "trailingStop", direction: "short", timeValue: 2, timeUnit: "day", percentage: 3, windowPolicy: "session"}: {},
		{mode: "stopLoss", direction: "long", timeValue: 5, timeUnit: "day", percentage: 1, windowPolicy: "session"}:      {},
		{mode: "stopLoss", direction: "short", timeValue: 3, timeUnit: "day", percentage: 2, windowPolicy: "session"}:     {},
		{mode: "stopLoss", direction: "long", timeValue: 3, timeUnit: "day", percentage: 2, windowPolicy: "continuous"}:   {},
		{mode: "stopLoss", direction: "long", timeValue: 3, timeUnit: "hour", percentage: 2, windowPolicy: "continuous"}:  {},
		{mode: "takeProfit", direction: "auto", timeValue: 1, timeUnit: "day", percentage: 4, windowPolicy: "continuous"}: {},
	}), []stopLossConfig{
		{mode: "stopLoss", direction: "long", timeValue: 3, timeUnit: "day", percentage: 2, windowPolicy: "continuous"},
		{mode: "stopLoss", direction: "short", timeValue: 3, timeUnit: "day", percentage: 2, windowPolicy: "session"},
		{mode: "stopLoss", direction: "long", timeValue: 3, timeUnit: "hour", percentage: 2, windowPolicy: "continuous"},
		{mode: "stopLoss", direction: "long", timeValue: 5, timeUnit: "day", percentage: 1, windowPolicy: "session"},
		{mode: "takeProfit", direction: "auto", timeValue: 1, timeUnit: "day", percentage: 4, windowPolicy: "continuous"},
		{mode: "trailingStop", direction: "short", timeValue: 2, timeUnit: "day", percentage: 3, windowPolicy: "session"},
	})

	assertConfigOrder(t, sortedMACDConfigs(map[macdConfig]struct{}{
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9}: {},
		{fastPeriod: 8, slowPeriod: 26, signalPeriod: 9}:  {},
		{fastPeriod: 12, slowPeriod: 20, signalPeriod: 9}: {},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 5}: {},
	}), []macdConfig{
		{fastPeriod: 8, slowPeriod: 26, signalPeriod: 9},
		{fastPeriod: 12, slowPeriod: 20, signalPeriod: 9},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 5},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9},
	})

	assertConfigOrder(t, sortedBollingerConfigs(map[bollingerConfig]struct{}{
		{period: 20, multiplier: 2}:   {},
		{period: 10, multiplier: 3}:   {},
		{period: 10, multiplier: 1.5}: {},
	}), []bollingerConfig{
		{period: 10, multiplier: 1.5},
		{period: 10, multiplier: 3},
		{period: 20, multiplier: 2},
	})

	assertConfigOrder(t, sortedKDJConfigs(map[kdjConfig]struct{}{
		{period: 9, m1: 3, m2: 3}: {},
		{period: 5, m1: 3, m2: 3}: {},
		{period: 9, m1: 2, m2: 3}: {},
		{period: 9, m1: 3, m2: 2}: {},
	}), []kdjConfig{
		{period: 5, m1: 3, m2: 3},
		{period: 9, m1: 2, m2: 3},
		{period: 9, m1: 3, m2: 2},
		{period: 9, m1: 3, m2: 3},
	})

	assertConfigOrder(t, sortedRSIDivergenceConfigs(map[rsiDivergenceConfig]struct{}{
		{period: 14, direction: "top", lookback: 5}:    {},
		{period: 7, direction: "bottom", lookback: 5}:  {},
		{period: 14, direction: "bottom", lookback: 8}: {},
		{period: 14, direction: "bottom", lookback: 3}: {},
	}), []rsiDivergenceConfig{
		{period: 7, direction: "bottom", lookback: 5},
		{period: 14, direction: "bottom", lookback: 3},
		{period: 14, direction: "bottom", lookback: 8},
		{period: 14, direction: "top", lookback: 5},
	})

	assertConfigOrder(t, sortedMACDDivergenceConfigs(map[macdDivergenceConfig]struct{}{
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "top", lookback: 5}:    {},
		{fastPeriod: 8, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 5}:  {},
		{fastPeriod: 12, slowPeriod: 20, signalPeriod: 9, direction: "bottom", lookback: 5}: {},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 5, direction: "bottom", lookback: 5}: {},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 8}: {},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 3}: {},
	}), []macdDivergenceConfig{
		{fastPeriod: 8, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 5},
		{fastPeriod: 12, slowPeriod: 20, signalPeriod: 9, direction: "bottom", lookback: 5},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 5, direction: "bottom", lookback: 5},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 3},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 8},
		{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "top", lookback: 5},
	})

	assertConfigOrder(t, sortedKDJDivergenceConfigs(map[kdjDivergenceConfig]struct{}{
		{period: 9, m1: 3, m2: 3, direction: "top", lookback: 5}:    {},
		{period: 5, m1: 3, m2: 3, direction: "bottom", lookback: 5}: {},
		{period: 9, m1: 2, m2: 3, direction: "bottom", lookback: 5}: {},
		{period: 9, m1: 3, m2: 2, direction: "bottom", lookback: 5}: {},
		{period: 9, m1: 3, m2: 3, direction: "bottom", lookback: 8}: {},
		{period: 9, m1: 3, m2: 3, direction: "bottom", lookback: 3}: {},
	}), []kdjDivergenceConfig{
		{period: 5, m1: 3, m2: 3, direction: "bottom", lookback: 5},
		{period: 9, m1: 2, m2: 3, direction: "bottom", lookback: 5},
		{period: 9, m1: 3, m2: 2, direction: "bottom", lookback: 5},
		{period: 9, m1: 3, m2: 3, direction: "bottom", lookback: 3},
		{period: 9, m1: 3, m2: 3, direction: "bottom", lookback: 8},
		{period: 9, m1: 3, m2: 3, direction: "top", lookback: 5},
	})
}

func assertConfigOrder[T any](t *testing.T, got, want []T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %#v, want %#v", got, want)
	}
}
