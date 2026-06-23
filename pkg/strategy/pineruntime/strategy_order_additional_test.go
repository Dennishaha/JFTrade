package pineruntime

import (
	"testing"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestParseAdvancedRuntimeBindingsAndSourceAwareMACD(t *testing.T) {
	t.Run("advanced bindings normalize keys and args", func(t *testing.T) {
		tests := []struct {
			name     string
			line     int
			alias    string
			fn       string
			args     []string
			wantKey  string
			wantKind string
		}{
			{name: "bbw", line: 1, alias: "bbw", fn: "bbw", args: []string{"hlc3", "20", "2", "week"}, wantKey: "bbw:hlc3:20:2:week", wantKind: "bbw"},
			{name: "obv", line: 2, alias: "obv", fn: "obv", args: nil, wantKey: "obv:close", wantKind: "obv"},
			{name: "linreg", line: 3, alias: "lin", fn: "linreg", args: []string{"close", "20", "1", "hour"}, wantKey: "linreg:close:20:1:hour", wantKind: "linreg"},
			{name: "kc", line: 4, alias: "kc", fn: "kc", args: []string{"close", "20", "1.5", "true", "day"}, wantKey: "kc:close:20:1.5:true:day", wantKind: "kc"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				binding, recognized, err := parseAdvancedRuntimeBinding(tc.line, tc.alias, tc.fn, tc.args)
				if err != nil {
					t.Fatalf("parseAdvancedRuntimeBinding(%s) error = %v", tc.fn, err)
				}
				if !recognized {
					t.Fatalf("parseAdvancedRuntimeBinding(%s) recognized = false", tc.fn)
				}
				if binding.Kind != tc.wantKind || binding.Key != tc.wantKey {
					t.Fatalf("binding = %#v, want kind=%q key=%q", binding, tc.wantKind, tc.wantKey)
				}
			})
		}
	})

	t.Run("source aware macd binding preserves timeframe and source", func(t *testing.T) {
		binding, err := parseMACDIndicatorBinding(9, "signal", "macd", []string{"12", "26", "9", "day", "hlc3"})
		if err != nil {
			t.Fatalf("parseMACDIndicatorBinding() error = %v", err)
		}
		if binding.Key != "macd:hlc3:12:26:9:day" {
			t.Fatalf("binding.Key = %q", binding.Key)
		}
		wantArgs := []string{"12", "26", "9", "day", "hlc3"}
		for index, want := range wantArgs {
			if binding.Args[index] != want {
				t.Fatalf("binding.Args[%d] = %q, want %q", index, binding.Args[index], want)
			}
		}
	})
}

func TestStrategyOrderHelpersRejectInvalidRuntimeBindings(t *testing.T) {
	if _, ok := parseStochSource("volume"); ok {
		t.Fatal("parseStochSource(volume) ok = true, want false")
	}
	if source, ok := parseStochSource("hlc3"); !ok || source != "hlc3" {
		t.Fatalf("parseStochSource(hlc3) = %q, %v", source, ok)
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "advanced binding rejects invalid timeframe",
			run: func() error {
				_, _, err := parseAdvancedRuntimeBinding(11, "bbw", "bbw", []string{"close", "20", "2", "noon"})
				return err
			},
		},
		{
			name: "advanced binding rejects out of range percentile",
			run: func() error {
				_, _, err := parseAdvancedRuntimeBinding(12, "pct", "percentile_nearest_rank", []string{"hlc3", "20", "150"})
				return err
			},
		},
		{
			name: "macd rejects invalid source aware args",
			run: func() error {
				_, err := parseMACDIndicatorBinding(13, "signal", "macd", []string{"12", "26", "9", "day", "spread"})
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	if key, ok := buildDivergenceRequirementKey(indicatorBinding{Kind: "macd", Key: "macd:12:26:9"}, "top", 6); !ok || key != "divergence:macd:12:26:9:top:6" {
		t.Fatalf("buildDivergenceRequirementKey(macd) = %q, %v", key, ok)
	}
	if key, ok := buildDivergenceRequirementKey(indicatorBinding{Kind: "ma", Key: "ma:EMA:5"}, "top", 6); ok || key != "" {
		t.Fatalf("buildDivergenceRequirementKey(ma) = %q, %v", key, ok)
	}
}

func TestResolveOrderQuantityHandlesAmountFallbackAndDefaultModes(t *testing.T) {
	runtime := &strategyRuntime{
		session: newPineTestSession(),
		symbol:  "US.AAPL",
	}
	runtime.session.GetAccount().TotalAccountValue = fixedpoint.NewFromFloat(1000)
	scope := &evaluationScope{
		runtime:   runtime,
		variables: map[string]any{},
		currentKline: &types.KLine{
			Symbol: "US.AAPL",
			Close:  fixedpoint.NewFromFloat(100),
		},
		closeSeries: seriesNumber{Current: 100, HasCurrent: true},
		hasBarData:  true,
	}

	if quantity, err := runtime.resolveOrderQuantity(&strategyir.OrderStmt{
		Action:             strategyir.OrderActionBuy,
		QuantityMode:       "amount",
		QuantityExpression: "50",
	}, scope, nil, 0, 100, "amount"); err != nil || quantity != 0 {
		t.Fatalf("resolveOrderQuantity(amount under one share) = %v, %v", quantity, err)
	}

	if quantity, err := runtime.resolveOrderQuantity(&strategyir.OrderStmt{
		Action:             strategyir.OrderActionBuy,
		QuantityMode:       "weird_runtime_default",
		QuantityExpression: "2.9",
	}, scope, nil, 0, 100, "weird_runtime_default"); err != nil || quantity != 2 {
		t.Fatalf("resolveOrderQuantity(default fallback) = %v, %v", quantity, err)
	}

	if quantity, err := runtime.resolveOrderQuantity(&strategyir.OrderStmt{
		Action:             strategyir.OrderActionSell,
		Intent:             strategyir.OrderIntentClose,
		QuantityMode:       "symbol_position_percent",
		QuantityExpression: "10",
	}, scope, &positionSnapshot{Direction: "LONG", Quantity: 5, AvailableQuantity: 5, MarketValue: 500}, 5, 100, "symbol_position_percent"); err != nil || quantity != 1 {
		t.Fatalf("resolveOrderQuantity(symbol_position_percent close) = %v, %v", quantity, err)
	}

	if quantity, err := runtime.resolveOrderQuantity(&strategyir.OrderStmt{
		Action:             strategyir.OrderActionBuy,
		QuantityMode:       "account_position_percent",
		QuantityExpression: "0.5",
	}, scope, nil, 0, 100, "account_position_percent"); err != nil || quantity != 0 {
		t.Fatalf("resolveOrderQuantity(account_position_percent tiny) = %v, %v", quantity, err)
	}

	if got := clampPercentBasedQuantity(0, 0, true); got != 0 {
		t.Fatalf("clampPercentBasedQuantity(zero available) = %v", got)
	}
	if value, ok, err := evaluateOptionalFloatExpression("close", scope); err != nil || !ok || value != 100 {
		t.Fatalf("evaluateOptionalFloatExpression(close) = %v %v %v", value, ok, err)
	}
	if high, low, close := currentBarPrices(&evaluationScope{}); high != 0 || low != 0 || close != 0 {
		t.Fatalf("currentBarPrices(nil kline) = %v %v %v", high, low, close)
	}
}
