package indicatorruntime

import (
	"slices"
	"strings"
	"testing"
)

func TestParseIndicatorRequirementKeysCoversSourceAwareAdvancedAndRiskKeys(t *testing.T) {
	requirements, err := parseIndicatorRequirementKeys([]string{
		"ma:EMA:20:day:hlc3",
		"ma:10:volume",
		"security_source:week:hlc3:2",
		"rsi:hlc3:14",
		"rsi:close:9:day",
		"stdev:hlc3:11",
		"variance:volume:8",
		"cum:volume",
		"stoch:hlc3:12:day",
		"cci:close:20",
		"vwap:ohlc4",
		"mfi:hlc3:15",
		"dmi:14:14",
		"supertrend:3:10",
		"supertrend:2.5:8:hour",
		"sar:0.02:0.02:0.2",
		"sl:long:3:bar:5",
		"risk:trailingStop:short:2:day:4:session",
		"divergence:rsi:14:bottom:5",
		"divergence:macd:12:26:9:top:6",
		"divergence:kdj:9:3:3:bottom:4",
		"anchored_vwap:month:hlc3",
		"cog:close:10:day",
		"cmo:close:9",
		"dev:close:5",
		"median:close:5",
		"percentrank:close:5",
		"bbw:close:20:2:week",
		"tsi:close:13:25:hour",
		"correlation:close:open:10:day",
		"percentile_linear_interpolation:close:20:80:week",
		"percentile_nearest_rank:hlc3:20:95",
		"swma:close:day",
		"linreg:close:20:2:hour",
		"obv:close",
		"pivothigh:high:2:3:day",
		"pivotlow:low:2:3",
		"kc:close:20:1.5:true:day",
		"kcw:close:20:1.5:false",
		"alma:close:9:0.85:6:day",
		"macd:hlc3:12:26:9:day",
		"bollinger:hlc3:20:2:week",
		"atr:14:day",
	}, true)
	if err != nil {
		t.Fatalf("parseIndicatorRequirementKeys() error = %v", err)
	}

	assertHasMAConfig(t, requirements, movingAverageConfig{averageType: "EMA", period: 20, timeUnit: "day", source: "hlc3"})
	assertHasMAConfig(t, requirements, movingAverageConfig{averageType: "MA", period: 10, source: "volume"})
	assertHasSecuritySourceConfig(t, requirements, securitySourceConfig{source: "hlc3", timeUnit: "week", lookback: 2})
	assertHasSourcePeriodConfig(t, "rsiSource", requirements.rsiSource, sourcePeriodConfig{source: "hlc3", period: 14})
	assertHasSourcePeriodConfig(t, "stdevSource", requirements.stdevSource, sourcePeriodConfig{source: "hlc3", period: 11})
	assertHasSourcePeriodConfig(t, "variance", requirements.variance, sourcePeriodConfig{source: "volume", period: 8})
	assertHasSourceConfig(t, "cum", requirements.cum, sourceConfig{source: "volume"})
	assertHasSourcePeriodConfig(t, "stoch", requirements.stoch, sourcePeriodConfig{source: "hlc3", period: 12, timeUnit: "day"})
	assertHasSourcePeriodConfig(t, "cciSource", requirements.cciSource, sourcePeriodConfig{source: "close", period: 20})
	assertHasSourceConfig(t, "vwap", requirements.vwap, sourceConfig{source: "ohlc4"})
	assertHasSourcePeriodConfig(t, "mfi", requirements.mfi, sourcePeriodConfig{source: "hlc3", period: 15})
	if len(requirements.dmi) != 1 || requirements.dmi[0] != (dmiConfig{diLength: 14, adxSmoothing: 14}) {
		t.Fatalf("dmi requirements = %#v", requirements.dmi)
	}
	if len(requirements.supertrend) != 1 || requirements.supertrend[0] != (supertrendConfig{factor: 3, atrPeriod: 10}) {
		t.Fatalf("supertrend requirements = %#v", requirements.supertrend)
	}
	if len(requirements.sar) != 1 || requirements.sar[0] != (sarConfig{start: 0.02, increment: 0.02, maximum: 0.2}) {
		t.Fatalf("sar requirements = %#v", requirements.sar)
	}
	assertHasStopLossConfig(t, requirements, stopLossConfig{mode: "stopLoss", direction: "long", timeValue: 3, percentage: 5, windowPolicy: "continuous"})
	assertHasStopLossConfig(t, requirements, stopLossConfig{mode: "trailingStop", direction: "short", timeValue: 2, timeUnit: "day", percentage: 4, windowPolicy: "session"})
	if len(requirements.rsiDivergence) != 1 || requirements.rsiDivergence[0] != (rsiDivergenceConfig{period: 14, direction: "bottom", lookback: 5}) {
		t.Fatalf("rsi divergence requirements = %#v", requirements.rsiDivergence)
	}
	if len(requirements.macdDivergence) != 1 || requirements.macdDivergence[0] != (macdDivergenceConfig{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "top", lookback: 6}) {
		t.Fatalf("macd divergence requirements = %#v", requirements.macdDivergence)
	}
	if len(requirements.kdjDivergence) != 1 || requirements.kdjDivergence[0] != (kdjDivergenceConfig{period: 9, m1: 3, m2: 3, direction: "bottom", lookback: 4}) {
		t.Fatalf("kdj divergence requirements = %#v", requirements.kdjDivergence)
	}

	advancedByKey := map[string]advancedIndicatorConfig{}
	for _, config := range requirements.advanced {
		advancedByKey[config.key] = config
	}
	for _, key := range []string{
		"anchored_vwap:month:hlc3",
		"cog:close:10:day",
		"cmo:close:9",
		"dev:close:5",
		"median:close:5",
		"percentrank:close:5",
		"bbw:close:20:2:week",
		"tsi:close:13:25:hour",
		"correlation:close:open:10:day",
		"percentile_linear_interpolation:close:20:80:week",
		"percentile_nearest_rank:hlc3:20:95",
		"swma:close:day",
		"linreg:close:20:2:hour",
		"obv:close",
		"pivothigh:high:2:3:day",
		"pivotlow:low:2:3",
		"kc:close:20:1.5:true:day",
		"kcw:close:20:1.5:false",
		"alma:close:9:0.85:6:day",
		"macd:hlc3:12:26:9:day",
		"bollinger:hlc3:20:2:week",
		"atr:14:day",
		"rsi:close:9:day",
		"supertrend:2.5:8:hour",
	} {
		if _, ok := advancedByKey[key]; !ok {
			t.Fatalf("missing advanced requirement %q in %#v", key, requirements.advanced)
		}
	}
	if got := advancedByKey["kcw:close:20:1.5:false"]; got.kind != "kcw" || got.useTR {
		t.Fatalf("kcw requirement = %#v, want kind kcw useTR=false", got)
	}
	if got := advancedByKey["alma:close:9:0.85:6:day"]; got.multiplier != 0.85 || got.parameter != 6 {
		t.Fatalf("alma requirement = %#v", got)
	}
}

func TestParseIndicatorRequirementsIgnoresInvalidKeysInScriptMode(t *testing.T) {
	requirements := parseIndicatorRequirements(`
		const ok = ctx.indicators["mfi:hlc3:14"];
		const ignored = ctx.indicators["stoch:volume:14"];
		const malformed = ctx.indicators["not-a-valid-key"];
		const singleQuoted = ctx.indicators['atr:14:day'];
	`)

	assertHasSourcePeriodConfig(t, "mfi", requirements.mfi, sourcePeriodConfig{source: "hlc3", period: 14})
	if len(requirements.stoch) != 0 {
		t.Fatalf("script mode should ignore invalid stoch volume source, got %#v", requirements.stoch)
	}
	if len(requirements.advanced) != 1 || requirements.advanced[0].key != "atr:14:day" {
		t.Fatalf("script mode advanced requirements = %#v", requirements.advanced)
	}
}

func TestParseIndicatorRequirementKeysStrictRejectsInvalidBusinessKeys(t *testing.T) {
	tests := []struct {
		key       string
		wantError string
	}{
		{"anchored_vwap:quarter:close", "invalid anchored_vwap key"},
		{"percentile_nearest_rank:close:20:101", "invalid percentile key"},
		{"kc:close:20:1.5:maybe", "invalid kc key"},
		{"stoch:volume:14", "invalid stoch key"},
		{"divergence:ema:20:top:4", "unsupported divergence key"},
		{"risk:badMode:auto:2:day:4:session", "invalid risk key"},
		{"unknown:thing", "unsupported indicator key"},
		{"ma", "invalid indicator key"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			_, err := parseIndicatorRequirementKeys([]string{tt.key}, true)
			if err == nil {
				t.Fatal("parseIndicatorRequirementKeys(strict) error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want detail %q", err, tt.wantError)
			}
		})
	}
}

func assertHasMAConfig(t *testing.T, requirements indicatorRequirements, want movingAverageConfig) {
	t.Helper()
	if slices.Contains(requirements.ma, want) {
		return
	}
	t.Fatalf("missing ma config %#v in %#v", want, requirements.ma)
}

func assertHasSecuritySourceConfig(t *testing.T, requirements indicatorRequirements, want securitySourceConfig) {
	t.Helper()
	if slices.Contains(requirements.securitySource, want) {
		return
	}
	t.Fatalf("missing security_source config %#v in %#v", want, requirements.securitySource)
}

func assertHasStopLossConfig(t *testing.T, requirements indicatorRequirements, want stopLossConfig) {
	t.Helper()
	if slices.Contains(requirements.stopLoss, want) {
		return
	}
	t.Fatalf("missing stop-loss config %#v in %#v", want, requirements.stopLoss)
}

func assertHasSourcePeriodConfig(t *testing.T, name string, got []sourcePeriodConfig, want sourcePeriodConfig) {
	t.Helper()
	if slices.Contains(got, want) {
		return
	}
	t.Fatalf("missing %s config %#v in %#v", name, want, got)
}

func assertHasSourceConfig(t *testing.T, name string, got []sourceConfig, want sourceConfig) {
	t.Helper()
	if slices.Contains(got, want) {
		return
	}
	t.Fatalf("missing %s config %#v in %#v", name, want, got)
}
