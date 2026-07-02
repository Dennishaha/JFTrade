package indicatorruntime

import (
	"math"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestIndicatorRuntimeSnapshotCoversBroadBusinessIndicatorDashboard(t *testing.T) {
	runtime := newIndicatorRuntime(`
		function onKLineClosed(ctx) {
			ctx.indicators["ma:SMA:5"];
			ctx.indicators["ma:VWMA:5:close"];
			ctx.indicators["ma:TMA:5"];
			ctx.indicators["ma:HMA:6"];
			ctx.indicators["ma:SMMA:5"];
			ctx.indicators["ma:LWMA:5"];
			ctx.indicators["rsi:hlc3:5"];
			ctx.indicators["macd:3:6:3"];
			ctx.indicators["bollinger:5:2"];
			ctx.indicators["kdj:5:3:3"];
			ctx.indicators["atr:5"];
			ctx.indicators["stdev:high:5"];
			ctx.indicators["variance:low:5"];
			ctx.indicators["range:high:5"];
			ctx.indicators["mode:close:5"];
			ctx.indicators["highestbars:high:5"];
			ctx.indicators["lowestbars:low:5"];
			ctx.indicators["cum:volume"];
			ctx.indicators["stoch:hlc3:5"];
			ctx.indicators["cci:close:5"];
			ctx.indicators["williamsr:5"];
			ctx.indicators["vwap:hlc3"];
			ctx.indicators["mfi:hlc3:5"];
			ctx.indicators["dmi:5:5"];
			ctx.indicators["supertrend:2:5"];
			ctx.indicators["sar:0.02:0.02:0.2"];
			ctx.indicators["anchored_vwap:day:hlc3"];
			ctx.indicators["cog:close:5"];
			ctx.indicators["bbw:close:5:2"];
			ctx.indicators["linreg:close:5:0"];
			ctx.indicators["pivothigh:high:2:2"];
			ctx.indicators["pivotlow:low:2:2"];
			ctx.indicators["kc:close:5:1.5:true"];
			ctx.indicators["kcw:close:5:1.5:true"];
			ctx.indicators["obv:close"];
			ctx.indicators["alma:close:5:0.85:6"];
			ctx.indicators["rising:close:5"];
			ctx.indicators["falling:close:5"];
			ctx.indicators["divergence:rsi:5:bottom:5"];
			ctx.indicators["divergence:macd:3:6:3:top:5"];
			ctx.indicators["divergence:kdj:5:3:3:bottom:5"];
		}
	`, types.Interval1m, "US.AAPL")
	if runtime == nil {
		t.Fatal("expected indicator runtime")
	}

	base := time.Date(2026, time.June, 15, 13, 30, 0, 0, time.UTC)
	closes := []float64{
		100, 101, 103, 102, 105, 107, 106, 109, 111, 108,
		110, 113, 115, 112, 116, 118, 117, 119, 121, 120,
		122, 124, 123, 126, 128, 125, 129, 131, 130, 132,
		134, 133, 136, 138, 135, 139, 141, 140, 142, 144,
	}
	for index, closeValue := range closes {
		start := base.Add(time.Duration(index) * time.Minute)
		highValue := closeValue + 1 + float64(index%3)*0.1
		lowValue := closeValue - 1 - float64(index%2)*0.1
		if index == len(closes)-3 {
			highValue = closeValue + 15
			lowValue = closeValue - 15
		}
		runtime.push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(start),
			EndTime:   types.Time(start.Add(time.Minute - time.Millisecond)),
			Open:      fixedpoint.NewFromFloat(closeValue - 0.4),
			High:      fixedpoint.NewFromFloat(highValue),
			Low:       fixedpoint.NewFromFloat(lowValue),
			Close:     fixedpoint.NewFromFloat(closeValue),
			Volume:    fixedpoint.NewFromFloat(1000 + float64(index%7)*50),
		}, market.SessionRegular)
	}

	snapshot := runtime.snapshot()
	if snapshot == nil {
		t.Fatal("expected broad indicator snapshot")
	}

	for _, key := range []string{
		"ma:SMA:5",
		"ma:VWMA:5",
		"ma:TMA:5",
		"ma:HMA:6",
		"ma:SMMA:5",
		"ma:LWMA:5",
		"rsi:hlc3:5",
		"atr:5",
		"stdev:high:5",
		"variance:low:5",
		"range:high:5",
		"mode:close:5",
		"cum:volume",
		"stoch:hlc3:5",
		"cci:close:5",
		"williamsr:5",
		"vwap:hlc3",
		"mfi:hlc3:5",
		"sar:0.02:0.02:0.2",
		"anchored_vwap:day:hlc3",
		"cog:close:5",
		"bbw:close:5:2",
		"linreg:close:5:0",
		"obv:close",
		"alma:close:5:0.85:6",
	} {
		requireBusinessScalarSnapshot(t, snapshot, key)
	}

	requireBusinessFieldSnapshot(t, snapshot, "macd:3:6:3", "diff", "signal", "histogram", "previousDiff")
	requireBusinessFieldSnapshot(t, snapshot, "bollinger:5:2", "middle", "upper", "lower")
	requireBusinessFieldSnapshot(t, snapshot, "kdj:5:3:3", "k", "d", "j", "previousK")
	requireBusinessFieldSnapshot(t, snapshot, "dmi:5:5", "plus", "minus", "adx")
	requireBusinessFieldSnapshot(t, snapshot, "supertrend:2:5", "line", "direction")
	requireBusinessFieldSnapshot(t, snapshot, "kc:close:5:1.5:true", "basis", "upper", "lower", "width")
	requireBusinessScalarSnapshot(t, snapshot, "kcw:close:5:1.5:true")

	requireBusinessScalarSnapshot(t, snapshot, "pivothigh:high:2:2")
	requireBusinessScalarSnapshot(t, snapshot, "pivotlow:low:2:2")
	for _, key := range []string{"rising:close:5", "falling:close:5", "divergence:rsi:5:bottom:5", "divergence:macd:3:6:3:top:5", "divergence:kdj:5:3:3:bottom:5"} {
		if _, ok := snapshot[key].(bool); !ok {
			t.Fatalf("snapshot %s type = %T, want bool", key, snapshot[key])
		}
	}
}

func requireBusinessScalarSnapshot(t *testing.T, snapshot map[string]any, key string) float64 {
	t.Helper()
	value, ok := snapshot[key]
	if !ok {
		t.Fatalf("snapshot missing %s: %#v", key, snapshot)
	}
	if reader, ok := value.(interface {
		ScalarValue() (float64, bool)
	}); ok {
		if scalar, scalarOK := reader.ScalarValue(); scalarOK && !math.IsNaN(scalar) && !math.IsInf(scalar, 0) {
			return scalar
		}
		t.Fatalf("snapshot %s has unavailable scalar: %#v", key, value)
	}
	if reader, ok := value.(interface {
		PreferredScalarValue() (float64, bool)
	}); ok {
		if scalar, scalarOK := reader.PreferredScalarValue(); scalarOK && !math.IsNaN(scalar) && !math.IsInf(scalar, 0) {
			return scalar
		}
		t.Fatalf("snapshot %s has unavailable preferred scalar: %#v", key, value)
	}
	t.Fatalf("snapshot %s type = %T, want scalar-capable snapshot", key, value)
	return 0
}

func requireBusinessFieldSnapshot(t *testing.T, snapshot map[string]any, key string, fields ...string) {
	t.Helper()
	value, ok := snapshot[key]
	if !ok {
		t.Fatalf("snapshot missing %s: %#v", key, snapshot)
	}
	values := snapshotToMap(value, fields)
	if values == nil {
		t.Fatalf("snapshot %s type = %T, want field-capable snapshot", key, value)
	}
	for _, field := range fields {
		fieldValue, ok := values[field]
		if !ok || fieldValue == nil {
			t.Fatalf("snapshot %s missing field %s: %#v", key, field, values)
		}
		if number, ok := fieldValue.(float64); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
			t.Fatalf("snapshot %s field %s = %v, want finite number", key, field, number)
		}
	}
}
