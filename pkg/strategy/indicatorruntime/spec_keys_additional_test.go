package indicatorruntime

import "testing"

func TestIndicatorKeyHelpersNormalizeSourcesUnitsAndLegacyFallbacks(t *testing.T) {
	if got := maIndicatorKey(movingAverageConfig{averageType: "EMA", period: 9, timeUnit: "day", source: "hl2"}); got != "ma:EMA:9:day:hl2" {
		t.Fatalf("maIndicatorKey() = %q", got)
	}
	if got := maIndicatorKey(movingAverageConfig{averageType: "SMA", period: 5, source: "close"}); got != "ma:SMA:5" {
		t.Fatalf("maIndicatorKey(close default) = %q", got)
	}
	if got := legacyMAIndicatorKey(20); got != "ma:20" {
		t.Fatalf("legacyMAIndicatorKey() = %q", got)
	}

	if got := sourcePeriodIndicatorKey("rsi", sourcePeriodConfig{period: 14, source: "", timeUnit: ""}, "close"); got != "rsi:14" {
		t.Fatalf("sourcePeriodIndicatorKey(legacy close) = %q", got)
	}
	if got := sourcePeriodIndicatorKey("rsi", sourcePeriodConfig{period: 14, source: "hlc3"}, "close"); got != "rsi:hlc3:14" {
		t.Fatalf("sourcePeriodIndicatorKey(custom source) = %q", got)
	}
	if got := securitySourceIndicatorKey(securitySourceConfig{source: "", timeUnit: "week", lookback: 3}); got != "security_source:week:close:3" {
		t.Fatalf("securitySourceIndicatorKey() = %q", got)
	}
	if got := sourceIndicatorKey("cum", sourceConfig{source: "OHLC4"}); got != "cum:ohlc4" {
		t.Fatalf("sourceIndicatorKey() = %q", got)
	}
}

func TestIndicatorKeyHelpersCoverSpecializedIndicatorsAndStopLossModes(t *testing.T) {
	if got := bollingerIndicatorKey(20, 2); got != "bollinger:20:2" {
		t.Fatalf("bollingerIndicatorKey() = %q", got)
	}
	if got := kdjIndicatorKey(9, 3, 3); got != "kdj:9:3:3" {
		t.Fatalf("kdjIndicatorKey() = %q", got)
	}
	if got := dmiIndicatorKey(dmiConfig{diLength: 14, adxSmoothing: 6}); got != "dmi:14:6" {
		t.Fatalf("dmiIndicatorKey() = %q", got)
	}
	if got := supertrendIndicatorKey(supertrendConfig{factor: 3, atrPeriod: 10}); got != "supertrend:3:10" {
		t.Fatalf("supertrendIndicatorKey() = %q", got)
	}
	if got := cciIndicatorKey(20); got != "cci:20" {
		t.Fatalf("cciIndicatorKey() = %q", got)
	}
	if got := williamsRIndicatorKey(14); got != "williamsr:14" {
		t.Fatalf("williamsRIndicatorKey() = %q", got)
	}
	if got := windowIndicatorKey(windowConfig{function: " RANGE ", source: "HLC3", period: 10}); got != "range:hlc3:10" {
		t.Fatalf("windowIndicatorKey() = %q", got)
	}
	if got := varianceIndicatorKey(sourcePeriodConfig{source: "", period: 8}); got != "variance:close:8" {
		t.Fatalf("varianceIndicatorKey() = %q", got)
	}

	continuousSL := stopLossIndicatorKey(stopLossConfig{
		mode: "stopLoss", direction: "long", timeValue: 5, timeUnit: "bar", percentage: 2.5, windowPolicy: "continuous",
	})
	if continuousSL != "sl:long:5:bar:2.5" {
		t.Fatalf("stopLossIndicatorKey(continuous stop loss) = %q", continuousSL)
	}
	sessionTrailing := stopLossIndicatorKey(stopLossConfig{
		mode: "trailingStop", direction: "short", timeValue: 3, timeUnit: "day", percentage: 1.2, windowPolicy: "session",
	})
	if sessionTrailing != "risk:trailingStop:short:3:day:1.2:session" {
		t.Fatalf("stopLossIndicatorKey(session trailing) = %q", sessionTrailing)
	}
}
