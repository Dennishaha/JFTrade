package dslruntime

import (
	"context"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/futu"
	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestNewStrategyRuntimeUsesExtendedTradingWindowWhenEnabled(t *testing.T) {
	script := `strategy Extended MA
version 1
symbol US.AAPL
interval 1h

on kline_close:
  let slow = ma(MA, 1, day)`

	program, err := strategydsl.ParseScript(script)
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
