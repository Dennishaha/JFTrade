package pine

import "testing"

func TestCoverage98RequestSecurityPurityRejectsUnsafeLoweredExpressions(t *testing.T) {
	for _, expression := range []string{
		"unknown_runtime_call(1)",
		"abs(unknown_runtime_call(1))",
		"member.call()",
	} {
		if requestSecurityLoweredASTIsPure(expression) {
			t.Fatalf("unsafe lowered expression was accepted: %q", expression)
		}
	}
	if !requestSecurityLoweredASTIsPure("market.snapshot.close") {
		t.Fatal("static member access was rejected as an impure lowered expression")
	}
	if _, ok := lowerSupportedRequestSecurityInner("ta.sma(close + open, 20)", "minute"); ok {
		t.Fatal("source-ambiguous moving average was lowered")
	}
	if _, ok := maskPureRequestSecurityTACalls("ta.sma", "minute", func(value string) string { return value }); ok {
		t.Fatal("unterminated TA call was masked")
	}
}

func TestCoverage98RequestSecurityRejectsMalformedAdvancedIndicatorArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func() bool
	}{
		{
			name: "bollinger requires source length multiplier",
			call: func() bool {
				_, ok := lowerRequestSecurityTACall("bb", []string{"close", "20"}, "minute")
				return ok
			},
		},
		{
			name: "stochastic rejects volume source",
			call: func() bool {
				_, ok := lowerRequestSecurityTACall("stoch", []string{"volume", "high", "low", "14"}, "minute")
				return ok
			},
		},
		{
			name: "stochastic requires high low pair",
			call: func() bool {
				_, ok := lowerRequestSecurityTACall("stoch", []string{"close", "low", "high", "14"}, "minute")
				return ok
			},
		},
		{
			name: "correlation requires a supported second source",
			call: func() bool {
				_, ok := lowerAdvancedRequestSecurity("correlation", []string{"close", "last_price", "20"}, "minute")
				return ok
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.call() {
				t.Fatal("malformed advanced indicator arguments were lowered")
			}
		})
	}
}
