package pineruntime

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/futu"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestNewStrategyRuntimeUsesExtendedTradingWindowWhenEnabled(t *testing.T) {
	script := `//@version=6
strategy("Extended MA", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 1))`

	program, err := strategypine.ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}
	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	newRuntime := func(useExtendedHours bool) *strategyRuntime {
		runtime, runtimeErr := newStrategyRuntime(
			context.Background(),
			&Strategy{
				Name:             "extended-ma",
				Symbol:           "US.AAPL",
				Interval:         types.Interval1h,
				Script:           script,
				UseExtendedHours: useExtendedHours,
			},
			program,
			plan,
			nil,
			nil,
		)
		if runtimeErr != nil {
			t.Fatalf("newStrategyRuntime(useExtendedHours=%v) error = %v", useExtendedHours, runtimeErr)
		}
		if runtime.engine == nil {
			t.Fatalf("expected indicator engine for useExtendedHours=%v", useExtendedHours)
		}
		return runtime
	}

	pushBars := func(runtime *strategyRuntime) {
		bars := []struct {
			endTime time.Time
			close   float64
			session futu.MarketSession
		}{
			{endTime: time.Date(2026, time.May, 28, 1, 0, 0, 0, time.UTC), close: 1, session: futu.MarketSessionOvernight},
			{endTime: time.Date(2026, time.May, 28, 7, 0, 0, 0, time.UTC), close: 2, session: futu.MarketSessionOvernight},
			{endTime: time.Date(2026, time.May, 28, 13, 0, 0, 0, time.UTC), close: 3, session: futu.MarketSessionPre},
			{endTime: time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC), close: 4, session: futu.MarketSessionRegular},
			{endTime: time.Date(2026, time.May, 29, 1, 0, 0, 0, time.UTC), close: 10, session: futu.MarketSessionOvernight},
			{endTime: time.Date(2026, time.May, 29, 7, 0, 0, 0, time.UTC), close: 20, session: futu.MarketSessionOvernight},
			{endTime: time.Date(2026, time.May, 29, 13, 0, 0, 0, time.UTC), close: 30, session: futu.MarketSessionPre},
			{endTime: time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC), close: 40, session: futu.MarketSessionRegular},
		}
		for _, bar := range bars {
			runtime.engine.Push(types.KLine{
				Symbol:    "US.AAPL",
				Interval:  types.Interval1h,
				StartTime: types.Time(bar.endTime.Add(-time.Hour)),
				EndTime:   types.Time(bar.endTime),
				Open:      fixedpoint.NewFromFloat(bar.close),
				High:      fixedpoint.NewFromFloat(bar.close),
				Low:       fixedpoint.NewFromFloat(bar.close),
				Close:     fixedpoint.NewFromFloat(bar.close),
				Volume:    fixedpoint.NewFromFloat(1),
			}, bar.session)
		}
	}

	readMAValue := func(runtime *strategyRuntime) float64 {
		snapshot := runtime.engine.SnapshotBorrowed()
		if len(snapshot) != 1 {
			t.Fatalf("snapshot len = %d, want 1", len(snapshot))
		}
		for _, value := range snapshot {
			current, ok := readObjectField(value, "value")
			if !ok || current == missingObjectField {
				t.Fatalf("missing MA value in snapshot: %#v", snapshot)
			}
			parsed, parsedOK := coerceFloatValue(current)
			if !parsedOK {
				t.Fatalf("snapshot value type = %T", current)
			}
			return parsed
		}
		t.Fatal("unexpected empty snapshot")
		return 0
	}

	extendedRuntime := newRuntime(true)
	pushBars(extendedRuntime)
	if value := readMAValue(extendedRuntime); value != 25 {
		t.Fatalf("extended MA(day) value = %v, want 25", value)
	}

	regularRuntime := newRuntime(false)
	pushBars(regularRuntime)
	if value := readMAValue(regularRuntime); value != 40 {
		t.Fatalf("regular MA(day) value = %v, want 40", value)
	}
}

func TestParseIndicatorBindingProducesExpectedKeys(t *testing.T) {
	tests := []struct {
		name     string
		alias    string
		expr     string
		wantKind string
		wantKey  string
		wantArgs []string
		wantErr  bool
	}{
		{
			name:     "ma with time unit",
			alias:    "fast",
			expr:     "ma(EMA,14,minute)",
			wantKind: "ma",
			wantKey:  "ma:EMA:14:minute",
			wantArgs: []string{"EMA", "14", "minute"},
		},
		{
			name:     "ma without time unit",
			alias:    "slow",
			expr:     "ma(SMA,20)",
			wantKind: "ma",
			wantKey:  "ma:SMA:20",
			wantArgs: []string{"SMA", "20", ""},
		},
		{
			name:     "ma lowercase type",
			alias:    "avg",
			expr:     "ma(ema,5,h)",
			wantKind: "ma",
			wantKey:  "ma:EMA:5:hour",
			wantArgs: []string{"EMA", "5", "hour"},
		},
		{
			name:     "rsi",
			alias:    "r",
			expr:     "rsi(14)",
			wantKind: "rsi",
			wantKey:  "rsi:14",
			wantArgs: []string{"14"},
		},
		{
			name:     "macd",
			alias:    "m",
			expr:     "macd(12,26,9)",
			wantKind: "macd",
			wantKey:  "macd:12:26:9",
			wantArgs: []string{"12", "26", "9"},
		},
		{
			name:     "kdj",
			alias:    "k",
			expr:     "kdj(9,3,3)",
			wantKind: "kdj",
			wantKey:  "kdj:9:3:3",
			wantArgs: []string{"9", "3", "3"},
		},
		{
			name:     "atr",
			alias:    "a",
			expr:     "atr(20)",
			wantKind: "atr",
			wantKey:  "atr:20",
			wantArgs: []string{"20"},
		},
		{
			name:     "cci",
			alias:    "c",
			expr:     "cci(14)",
			wantKind: "cci",
			wantKey:  "cci:14",
			wantArgs: []string{"14"},
		},
		{
			name:     "williamsr",
			alias:    "wr",
			expr:     "williams_r(14)",
			wantKind: "williamsr",
			wantKey:  "williamsr:14",
			wantArgs: []string{"14"},
		},
		{
			name:     "bollinger integer multiplier",
			alias:    "b",
			expr:     "bollinger(20,2)",
			wantKind: "bollinger",
			wantKey:  "bollinger:20:2",
			wantArgs: []string{"20", "2"},
		},
		{
			name:     "bollinger float multiplier",
			alias:    "b",
			expr:     "bollinger(20,2.5)",
			wantKind: "bollinger",
			wantKey:  "bollinger:20:2.5",
			wantArgs: []string{"20", "2.5"},
		},
		{
			name:    "invalid ma type",
			alias:   "bad",
			expr:    "ma(UNKNOWN,14)",
			wantErr: true,
		},
		{
			name:    "invalid rsi args",
			alias:   "r",
			expr:    "rsi(14,20)",
			wantErr: true,
		},
		{
			name:    "not a function call",
			alias:   "x",
			expr:    "42",
			wantErr: false, // not recognized, no error
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmt := &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: 1},
				Name:       tt.alias,
				Expression: tt.expr,
			}
			binding, recognized, err := parseIndicatorBinding(stmt)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseIndicatorBinding(%q) error = nil, want error", tt.expr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIndicatorBinding(%q) error = %v", tt.expr, err)
			}
			if tt.wantKind == "" && !recognized {
				return // expected unrecognized
			}
			if !recognized {
				t.Fatalf("parseIndicatorBinding(%q) recognized = false", tt.expr)
			}
			if binding.Kind != tt.wantKind {
				t.Fatalf("binding.Kind = %q, want %q", binding.Kind, tt.wantKind)
			}
			if binding.Key != tt.wantKey {
				t.Fatalf("binding.Key = %q, want %q", binding.Key, tt.wantKey)
			}
			if len(binding.Args) != len(tt.wantArgs) {
				t.Fatalf("binding.Args = %v, want %v", binding.Args, tt.wantArgs)
			}
			for i := range binding.Args {
				if binding.Args[i] != tt.wantArgs[i] {
					t.Fatalf("binding.Args[%d] = %q, want %q", i, binding.Args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestRuntimeAndPlannerIndicatorKeysMatch(t *testing.T) {
	program := &strategyir.Program{
		SourceFormat: strategypine.SourceFormatPineV6,
		Hooks: []strategyir.HookBlock{{
			Kind: strategyir.HookKLineClose,
			Statements: []strategyir.Statement{
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
				&strategyir.IfStmt{
					Range:     strategyir.SourceRange{StartLine: 12},
					Condition: "close > 0",
					Then: []strategyir.Statement{&strategyir.ProtectStmt{
						Range:                strategyir.SourceRange{StartLine: 13},
						Direction:            "both",
						Mode:                 "trailing_stop",
						TimeValueExpression:  "2",
						TimeUnit:             "day",
						PercentageExpression: "4%",
						WindowPolicy:         "session",
					}},
				},
			},
		}},
	}
	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	runtimeKeys := make([]string, 0, 12)
	for _, hook := range program.Hooks {
		for _, statement := range hook.Statements {
			switch typed := statement.(type) {
			case *strategyir.LetStmt:
				binding, recognized, bindingErr := parseIndicatorBinding(typed)
				if bindingErr != nil {
					t.Fatalf("parseIndicatorBinding(%q) error = %v", typed.Expression, bindingErr)
				}
				if recognized {
					runtimeKeys = append(runtimeKeys, binding.Key)
				}
			case *strategyir.IfStmt:
				for _, nested := range typed.Then {
					protect, ok := nested.(*strategyir.ProtectStmt)
					if !ok {
						continue
					}
					key, keyErr := buildProtectRequirementKey(protect)
					if keyErr != nil {
						t.Fatalf("buildProtectRequirementKey() error = %v", keyErr)
					}
					runtimeKeys = append(runtimeKeys, key)
				}
			}
		}
	}

	plannerKeys := make([]string, 0, len(plan.Indicators))
	for _, indicator := range plan.Indicators {
		plannerKeys = append(plannerKeys, indicator.Key)
	}
	sort.Strings(runtimeKeys)
	sort.Strings(plannerKeys)
	if !reflect.DeepEqual(runtimeKeys, plannerKeys) {
		t.Fatalf("runtime keys = %v, planner keys = %v", runtimeKeys, plannerKeys)
	}
}
