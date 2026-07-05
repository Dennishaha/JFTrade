package pine

import (
	"strings"
	"testing"
)

func TestCompileRejectsMalformedSwitchAndTupleContracts(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "empty switch expression", body: "signal = switch", message: "switch requires at least one arm"},
		{name: "empty switch statement", body: "switch", message: "switch requires at least one arm"},
		{name: "malformed switch arm", body: "signal = switch\n    close", message: "switch arms must use condition =>"},
		{name: "invalid switch value", body: "signal = switch\n    close > open => close >", message: "invalid switch expression"},
		{name: "invalid switch condition", body: "switch\n    close > => log.info(\"bad\")", message: "invalid"},
		{name: "unsupported switch statement", body: "switch\n    close > open => broker.submit()", message: "unsupported switch statement"},
		{name: "tuple width mismatch", body: "[fast, slow] = [close, open, high]", message: "tuple returns 3 values but assignment has 2 aliases"},
		{name: "tuple expression invalid", body: "[fast, slow] = [close >, open]", message: "invalid tuple expression"},
		{name: "bollinger arguments", body: "[basis, upper, lower] = ta.bb(close, 20)", message: "ta.bb expects ta.bb(source, length, mult)"},
		{name: "dmi arguments", body: "[plus, minus, adx] = ta.dmi(14)", message: "ta.dmi expects ta.dmi(diLength, adxSmoothing)"},
		{name: "supertrend arguments", body: "[trend, direction] = ta.supertrend(3)", message: "ta.supertrend expects ta.supertrend(factor, atrPeriod)"},
		{name: "kc arguments", body: "[basis, upper, lower] = ta.kc(close, 20)", message: "ta.kc expects ta.kc(source, length, mult, useTrueRange?)"},
		{name: "macd arguments", body: "[macd, signal, histogram] = ta.macd(close, 12, 26)", message: "ta.macd expects ta.macd(source, fast, slow, signal)"},
		{name: "unsupported tuple source", body: "[first, second, third] = ta.rsi(close, 14)", message: "tuple assignment is supported only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := "//@version=6\nstrategy(\"Tuple switch rejection\", overlay=true)\n" + tc.body
			_, err := Compile(script)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Compile() error = %v, want message containing %q", err, tc.message)
			}
		})
	}
}
