package indicatorruntime

import "testing"

func TestCoverage98RiskSpecificationRejectsMalformedTimeAndPolicyContracts(t *testing.T) {
	for _, parts := range [][]string{
		{"risk", "stopLoss", "long"},
		{"risk", "stopLoss", "long", "0", "day", "1", "continuous"},
		{"risk", "stopLoss", "long", "2", "fortnight", "1", "continuous"},
		{"risk", "stopLoss", "long", "2", "day", "1", "unknown-policy"},
		{"unrecognized"},
	} {
		if config, ok := parseStopLossConfig(parts); ok {
			t.Fatalf("malformed risk specification was accepted: %#v => %#v", parts, config)
		}
	}

	if unit, ok := parseStopLossTimeUnit("fortnight"); ok || unit != "" {
		t.Fatalf("unsupported stop-loss unit = %q/%v", unit, ok)
	}
	if unit, ok := parseIndicatorTimeUnit("fortnight"); ok || unit != "" {
		t.Fatalf("unsupported indicator unit = %q/%v", unit, ok)
	}
	if bars := resolveBarCount(3, "fortnight", 5); bars != 3 {
		t.Fatalf("unknown timeframe must retain explicit bar count, got %d", bars)
	}
}
