package ir_test

import (
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestPlanRequirementsCollectsPineIndicatorsAndRuntimeNeeds(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Mean Revert", overlay=true)
fast = request.security(syminfo.tickerid, "D", ta.ema(close, 5))
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
signal = ta.macd(close, 12, 26, 9)
if ta.crossover(fast, slow) and divergence_top(signal, 6)
    strategy.entry("Long", strategy.long, qty=(strategy.equity * 50 / 100) / close)
else
    strategy.exit("Long trail", "Long", trail_points=close * 4 / 100, trail_offset=close * 4 / 100)`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	expectedKeys := []string{
		"divergence:macd:12:26:9:top:6",
		"ma:EMA:5:day",
		"ma:SMA:20:day",
		"macd:12:26:9",
	}
	if len(requirements.Indicators) != len(expectedKeys) {
		t.Fatalf("len(requirements.Indicators) = %d, want %d", len(requirements.Indicators), len(expectedKeys))
	}
	for index, key := range expectedKeys {
		if requirements.Indicators[index].Key != key {
			t.Fatalf("requirements.Indicators[%d].Key = %q, want %q", index, requirements.Indicators[index].Key, key)
		}
	}
	if !requirements.RequiresPosition {
		t.Fatal("RequiresPosition = false, want true")
	}
	if !requirements.RequiresTotalAccountValue {
		t.Fatal("RequiresTotalAccountValue = false, want true")
	}
}

func TestPlanRequirementsRejectsInvalidIndicatorBinding(t *testing.T) {
	program := programWithStatements(&strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: 1},
		Name:       "fast",
		Expression: "ma(EMA, nope, day)",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid ma binding error")
	}
}

func TestPlanRequirementsRejectsInvalidMovingAverageType(t *testing.T) {
	program := programWithStatements(&strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: 1},
		Name:       "fast",
		Expression: "ma(WILD, 5, day)",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid moving average type error")
	}
}

func TestPlanRequirementsRejectsUnsupportedOrderQuantityMode(t *testing.T) {
	program := programWithStatements(&strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 1},
		Action:             strategyir.OrderActionBuy,
		QuantityMode:       "bananas",
		QuantityExpression: "100",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want unsupported order quantity mode error")
	}
}

func TestPlanRequirementsRejectsInvalidProtectTimeUnit(t *testing.T) {
	program := programWithStatements(&strategyir.ProtectStmt{
		Range:                strategyir.SourceRange{StartLine: 1},
		Direction:            "auto",
		Mode:                 "trailing_stop",
		TimeValueExpression:  "2",
		TimeUnit:             "lunar",
		PercentageExpression: "4%",
		WindowPolicy:         "session",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid protect time unit error")
	}
}

func TestPlanRequirementsDetectsPositionVariablesInExpressions(t *testing.T) {
	program := programWithStatements(
		&strategyir.LetStmt{
			Range:      strategyir.SourceRange{StartLine: 1},
			Name:       "stopPrice",
			Expression: "position_avg_price * 0.95",
		},
		&strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: 2},
			Condition: "position_size > 0",
		},
	)

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}
	if !requirements.RequiresPosition {
		t.Fatal("RequiresPosition = false, want true")
	}
}

func TestPlanRequirementsIndicatorKeysMatchRuntimeBindingParity(t *testing.T) {
	program := programWithStatements(
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "fast", Expression: "ma(EMA,14,minute)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "slow", Expression: "ma(SMA,20)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 3}, Name: "avg", Expression: "ma(ema,5,h)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 4}, Name: "r", Expression: "rsi(14)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 5}, Name: "m", Expression: "macd(12,26,9)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 6}, Name: "k", Expression: "kdj(9,3,3)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 7}, Name: "a", Expression: "atr(20)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 8}, Name: "c", Expression: "cci(14)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 9}, Name: "wr", Expression: "williams_r(14)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 10}, Name: "bInt", Expression: "bollinger(20,2)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 11}, Name: "bFloat", Expression: "bollinger(20,2.5)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 12}, Name: "sd", Expression: "stdev(20)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 13}, Name: "hh", Expression: "highest(high,20)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 14}, Name: "ll", Expression: "lowest(low,10)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 15}, Name: "delta", Expression: "change(close,1)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 16}, Name: "momentum", Expression: "mom(close,5)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 17}, Name: "rate", Expression: "roc(close,12)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 18}, Name: "up", Expression: "rising(close,3)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 19}, Name: "down", Expression: "falling(close,3)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 20}, Name: "avgVol", Expression: "ma(SMA,20,volume)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 21}, Name: "emaHigh", Expression: "ma(EMA,5,high)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 22}, Name: "volSum", Expression: "sum(volume,20)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 23}, Name: "sar", Expression: "sar(0.02,0.02,0.2)"},
		&strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: 24},
			Condition: "cross_over(fast, slow)",
			Then: []strategyir.Statement{&strategyir.ProtectStmt{
				Range:                strategyir.SourceRange{StartLine: 25},
				Direction:            "auto",
				Mode:                 "trailing_stop",
				TimeValueExpression:  "2",
				TimeUnit:             "day",
				PercentageExpression: "4%",
				WindowPolicy:         "session",
			}},
		},
	)

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	expectedKeys := map[string]bool{
		"ma:EMA:14:minute":                       true,
		"ma:SMA:20":                              true,
		"ma:EMA:5:hour":                          true,
		"rsi:14":                                 true,
		"macd:12:26:9":                           true,
		"kdj:9:3:3":                              true,
		"atr:20":                                 true,
		"cci:14":                                 true,
		"williamsr:14":                           true,
		"bollinger:20:2":                         true,
		"bollinger:20:2.5":                       true,
		"stdev:20":                               true,
		"highest:high:20":                        true,
		"lowest:low:10":                          true,
		"change:close:1":                         true,
		"mom:close:5":                            true,
		"roc:close:12":                           true,
		"rising:close:3":                         true,
		"falling:close:3":                        true,
		"ma:SMA:20:volume":                       true,
		"ma:EMA:5:high":                          true,
		"sum:volume:20":                          true,
		"sar:0.02:0.02:0.2":                      true,
		"risk:trailingStop:auto:2:day:4:session": true,
	}
	for _, ind := range requirements.Indicators {
		if !expectedKeys[ind.Key] {
			t.Fatalf("unexpected indicator key %q in plan", ind.Key)
		}
		delete(expectedKeys, ind.Key)
	}
	for key := range expectedKeys {
		t.Fatalf("missing indicator key %q in plan", key)
	}
}

func TestPlanRequirementsPreservesLegacyCloseKeysAndSourceAwareKeys(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Source Keys", overlay=true)
closeSma = ta.sma(close, 20)
volumeSma = ta.sma(volume, 20)
hlc3Ema = ta.ema(hlc3, 20)
closeRsi = ta.rsi(close, 14)
hlc3Rsi = ta.rsi(hlc3, 14)
legacyCci = ta.cci(hlc3, 20)
closeCci = ta.cci(close, 20)
if close > closeSma and volume > volumeSma and hlc3Ema > 0 and closeRsi > hlc3Rsi and closeCci > legacyCci
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	keys := map[string]bool{}
	for _, indicator := range requirements.Indicators {
		keys[indicator.Key] = true
	}
	expected := []string{
		"ma:SMA:20",
		"ma:SMA:20:volume",
		"ma:EMA:20:hlc3",
		"rsi:14",
		"rsi:hlc3:14",
		"cci:20",
		"cci:close:20",
	}
	for _, key := range expected {
		if !keys[key] {
			t.Fatalf("missing indicator key %q; got %#v", key, requirements.Indicators)
		}
	}
	for _, unexpected := range []string{
		"ma:SMA:20:close",
		"rsi:close:14",
		"cci:hlc3:20",
	} {
		if keys[unexpected] {
			t.Fatalf("unexpected non-legacy key %q in %#v", unexpected, requirements.Indicators)
		}
	}
}

func TestPlanRequirementsRejectsUnsupportedWindowSource(t *testing.T) {
	program := programWithStatements(&strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: 7},
		Name:       "spreadHigh",
		Expression: "highest(close - open, 20)",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want unsupported source error")
	}
}

func TestPlanRequirementsCollectsAdvancedIndicatorBindings(t *testing.T) {
	program := programWithStatements(
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "cog", Expression: "cog(close, 10, day)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "bbw", Expression: "bbw(close, 20, 2.0, week)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 3}, Name: "tsi", Expression: "tsi(close, 13, 25, hour)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 4}, Name: "corr", Expression: "correlation(close, open, 10, day)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 5}, Name: "pctLin", Expression: "percentile_linear_interpolation(close, 20, 80, week)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 6}, Name: "pctRank", Expression: "percentile_nearest_rank(hlc3, 20, 95, month)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 7}, Name: "sw", Expression: "swma(close, day)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 8}, Name: "lin", Expression: "linreg(close, 20, 1, hour)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 9}, Name: "obv", Expression: "obv(close)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 10}, Name: "pivotTop", Expression: "pivothigh(high, 2, 2, day)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 11}, Name: "kc", Expression: "kc(close, 20, 1.5, true, day)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 12}, Name: "alma", Expression: "alma(close, 9, 0.85, 6, day)"},
	)

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	keys := map[string]bool{}
	for _, indicator := range requirements.Indicators {
		keys[indicator.Key] = true
	}
	for _, key := range []string{
		"cog:close:10:day",
		"bbw:close:20:2:week",
		"tsi:close:13:25:hour",
		"correlation:close:open:10:day",
		"percentile_linear_interpolation:close:20:80:week",
		"percentile_nearest_rank:hlc3:20:95:month",
		"swma:close:day",
		"linreg:close:20:1:hour",
		"obv:close",
		"pivothigh:high:2:2:day",
		"kc:close:20:1.5:true:day",
		"alma:close:9:0.85:6:day",
	} {
		if !keys[key] {
			t.Fatalf("missing advanced indicator key %q in %#v", key, requirements.Indicators)
		}
	}
}

func TestPlanRequirementsCollectsExpressionIndicatorsAcrossStatementShapes(t *testing.T) {
	program := programWithStatements(
		&strategyir.CollectionStmt{
			Range: strategyir.SourceRange{StartLine: 1},
			Arguments: []string{
				"stoch(close, high, low, 14, day)",
				"security_source(close, week, 2)",
				"variance(close, 10)",
				"cum(volume)",
				"anchored_vwap(hlc3, month)",
				"mfi(hlc3, 14)",
				"dmi(14, 14)",
				"supertrend(3, 10, day)",
				"sar(0.02, 0.02, 0.2)",
				"bollinger(20, 2, day, close)",
				"vwap(close)",
			},
		},
		&strategyir.TupleStmt{
			Range: strategyir.SourceRange{StartLine: 2},
			Expressions: []string{
				"macd(12, 26, 9, day, close)",
				"atr(14, week)",
			},
		},
	)

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	keys := map[string]bool{}
	for _, indicator := range requirements.Indicators {
		keys[indicator.Key] = true
	}
	for _, key := range []string{
		"stoch:close:14:day",
		"security_source:week:close:2",
		"variance:close:10",
		"cum:volume",
		"anchored_vwap:month:hlc3",
		"mfi:hlc3:14",
		"dmi:14:14",
		"supertrend:3:10:day",
		"sar:0.02:0.02:0.2",
		"bollinger:close:20:2:day",
		"vwap:close",
		"macd:close:12:26:9:day",
		"atr:14:week",
	} {
		if !keys[key] {
			t.Fatalf("missing expression requirement %q in %#v", key, requirements.Indicators)
		}
	}
}

func programWithStatements(statements ...strategyir.Statement) *strategyir.Program {
	return &strategyir.Program{
		SourceFormat: strategypine.SourceFormatPineV6,
		Hooks: []strategyir.HookBlock{{
			Kind:       strategyir.HookKLineClose,
			Statements: statements,
		}},
	}
}
