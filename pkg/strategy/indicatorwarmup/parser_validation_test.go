package indicatorwarmup

import "testing"

func TestAdvancedRequirementParserRejectsMalformedShapes(t *testing.T) {
	builder := newIndicatorRequirementSetBuilder()
	tests := []struct {
		name  string
		parse func() error
	}{
		{"advanced default", func() error { return builder.parseAdvancedKey("unknown", []string{"unknown"}) }},
		{"anchored length", func() error {
			return builder.parseAnchoredVWAPKey("anchored_vwap:day", []string{"anchored_vwap", "day"})
		}},
		{"advanced period length", func() error {
			return builder.parseAdvancedSourcePeriodKey("cog:close", []string{"cog", "close"}, "cog", "invalid cog key: %s")
		}},
		{"bbw length", func() error { return builder.parseBBWKey("bbw:close:2", []string{"bbw", "close", "2"}) }},
		{"tsi length", func() error { return builder.parseTSIKey("tsi:close:2", []string{"tsi", "close", "2"}) }},
		{"correlation length", func() error {
			return builder.parseCorrelationKey("correlation:close:high", []string{"correlation", "close", "high"})
		}},
		{"percentile length", func() error {
			return builder.parsePercentileKey("percentile_nearest_rank:close:2", []string{"percentile_nearest_rank", "close", "2"})
		}},
		{"advanced source length", func() error {
			return builder.parseAdvancedSourceKey("obv", []string{"obv"}, "obv", "invalid obv key: %s")
		}},
		{"linreg length", func() error { return builder.parseLinregKey("linreg:close:2", []string{"linreg", "close", "2"}) }},
		{"pivot length", func() error { return builder.parsePivotKey("pivothigh:high:2", []string{"pivothigh", "high", "2"}) }},
		{"keltner length", func() error { return builder.parseKeltnerKey("kc:close:2:1", []string{"kc", "close", "2", "1"}) }},
		{"alma length", func() error { return builder.parseALMAKey("alma:close:2:0.5", []string{"alma", "close", "2", "0.5"}) }},
		{"stoch invalid unit", func() error {
			return builder.parseStochKey("stoch:close:2:bad", []string{"stoch", "close", "2", "bad"})
		}},
		{"rsi divergence length", func() error {
			return builder.parseRSIDivergenceKey("divergence:rsi", []string{"divergence", "rsi"}, "top", 2)
		}},
		{"macd divergence length", func() error {
			return builder.parseMACDDivergenceKey("divergence:macd", []string{"divergence", "macd"}, "top", 2)
		}},
		{"kdj divergence length", func() error {
			return builder.parseKDJDivergenceKey("divergence:kdj", []string{"divergence", "kdj"}, "top", 2)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.parse(); err == nil {
				t.Fatal("malformed requirement unexpectedly parsed")
			}
		})
	}
}

func TestFixedTimeframeValidationRejectsEveryRequirementFamily(t *testing.T) {
	tests := []struct {
		name         string
		requirements indicatorRequirements
	}{
		{"moving average", indicatorRequirements{ma: []movingAverageConfig{{timeUnit: "5m"}}}},
		{"security source", indicatorRequirements{securitySource: []securitySourceConfig{{timeUnit: "5m"}}}},
		{"rsi", indicatorRequirements{rsiSource: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"stdev", indicatorRequirements{stdevSource: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"variance", indicatorRequirements{variance: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"stoch", indicatorRequirements{stoch: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"cci", indicatorRequirements{cciSource: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"mfi", indicatorRequirements{mfi: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"advanced", indicatorRequirements{advanced: []advancedIndicatorConfig{{kind: "cmo", timeUnit: "5m"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateFixedTimeframeRequirements(tt.requirements, 2); err == nil {
				t.Fatal("unaligned fixed timeframe unexpectedly accepted")
			}
		})
	}
}
