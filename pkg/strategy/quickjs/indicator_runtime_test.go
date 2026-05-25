package quickjs

import (
	"context"
	"testing"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	qjs "modernc.org/quickjs"
)

func TestParseIndicatorRequirements(t *testing.T) {
	requirements := parseIndicatorRequirements(`
		function onKLineClosed(ctx) {
			const fastAverage = ctx.indicators["ma:5"];
			const slowAverage = ctx.indicators['ma:20'];
			const latestRsi = ctx.indicators["rsi:14"];
			const latestMacd = ctx.indicators["macd:12:26:9"];
			const latestBollinger = ctx.indicators["bollinger:20:2"];
				const latestKdj = ctx.indicators["kdj:9:3:3"];
				const latestAtr = ctx.indicators["atr:14"];
				const latestCci = ctx.indicators["cci:20"];
				const latestWilliamsR = ctx.indicators["williamsr:14"];
				const topRsiDivergence = ctx.indicators["divergence:rsi:14:top:5"];
				const bottomMacdDivergence = ctx.indicators["divergence:macd:12:26:9:bottom:6"];
				const topKdjDivergence = ctx.indicators["divergence:kdj:9:3:3:top:4"];
		}
	`)

	if len(requirements.ma) != 2 || requirements.ma[0] != 5 || requirements.ma[1] != 20 {
		t.Fatalf("ma requirements = %#v", requirements.ma)
	}
	if len(requirements.rsi) != 1 || requirements.rsi[0] != 14 {
		t.Fatalf("rsi requirements = %#v", requirements.rsi)
	}
	if len(requirements.macd) != 1 || requirements.macd[0] != (macdConfig{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9}) {
		t.Fatalf("macd requirements = %#v", requirements.macd)
	}
	if len(requirements.bollinger) != 1 || requirements.bollinger[0] != (bollingerConfig{period: 20, multiplier: 2}) {
		t.Fatalf("bollinger requirements = %#v", requirements.bollinger)
	}
	if len(requirements.kdj) != 1 || requirements.kdj[0] != (kdjConfig{period: 9, m1: 3, m2: 3}) {
		t.Fatalf("kdj requirements = %#v", requirements.kdj)
	}
	if len(requirements.atr) != 1 || requirements.atr[0] != 14 {
		t.Fatalf("atr requirements = %#v", requirements.atr)
	}
	if len(requirements.cci) != 1 || requirements.cci[0] != 20 {
		t.Fatalf("cci requirements = %#v", requirements.cci)
	}
	if len(requirements.williamsR) != 1 || requirements.williamsR[0] != 14 {
		t.Fatalf("williamsR requirements = %#v", requirements.williamsR)
	}
	if len(requirements.rsiDivergence) != 1 || requirements.rsiDivergence[0] != (rsiDivergenceConfig{period: 14, direction: "top", lookback: 5}) {
		t.Fatalf("rsi divergence requirements = %#v", requirements.rsiDivergence)
	}
	if len(requirements.macdDivergence) != 1 || requirements.macdDivergence[0] != (macdDivergenceConfig{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 6}) {
		t.Fatalf("macd divergence requirements = %#v", requirements.macdDivergence)
	}
	if len(requirements.kdjDivergence) != 1 || requirements.kdjDivergence[0] != (kdjDivergenceConfig{period: 9, m1: 3, m2: 3, direction: "top", lookback: 4}) {
		t.Fatalf("kdj divergence requirements = %#v", requirements.kdjDivergence)
	}
}

func TestRuntimeBridgeInjectsIndicators(t *testing.T) {
	session := &bbgo2.ExchangeSession{Account: types.NewAccount()}
	bridge, err := newRuntimeBridge(context.Background(), &Strategy{
		Name:   "quickjs-indicator-test",
		Symbol: "BTCUSDT",
		Script: `function onKLineClosed(ctx) {
			globalThis.__indicatorJSON = JSON.stringify([
				ctx.indicators["ma:5"] !== null,
				ctx.indicators["rsi:3"],
				ctx.indicators["macd:3:5:2"] !== null,
				ctx.indicators["bollinger:3:2"] !== null,
				ctx.indicators["kdj:3:3:3"] !== null,
				ctx.indicators["atr:3"] !== null,
				ctx.indicators["cci:3"] !== null,
				ctx.indicators["williamsr:3"] !== null,
				typeof ctx.indicators["divergence:rsi:3:top:3"] === "boolean",
				typeof ctx.indicators["divergence:macd:3:5:2:bottom:3"] === "boolean",
				typeof ctx.indicators["divergence:kdj:3:3:3:top:3"] === "boolean"
			]);
		}`,
	}, &stubOrderExecutor{}, session)
	if err != nil {
		t.Fatalf("newRuntimeBridge() error = %v", err)
	}
	defer bridge.close()

	for index, closePrice := range []float64{100, 102, 104, 103, 105, 107, 108} {
		high := closePrice + 1 + float64(index%2)
		low := closePrice - 1 - float64(index%2)
		bridge.pushIndicators(types.KLine{
			High:  fixedpoint.NewFromFloat(high),
			Low:   fixedpoint.NewFromFloat(low),
			Close: fixedpoint.NewFromFloat(closePrice),
		})
	}

	err = bridge.invokeHook("onKLineClosed", map[string]any{
		"symbol":     "BTCUSDT",
		"kline":      map[string]any{"close": 108},
		"indicators": bridge.indicatorPayload(),
	}, nil)
	if err != nil {
		t.Fatalf("invokeHook() error = %v", err)
	}

	encoded, err := bridge.vm.Eval(`globalThis.__indicatorJSON`, qjs.EvalGlobal)
	if err != nil {
		t.Fatalf("read indicator JSON error = %v", err)
	}
	text, ok := encoded.(string)
	if !ok {
		t.Fatalf("indicator JSON type = %T", encoded)
	}
	if text == "" || text == `[false,null,false,false,false,false,false,false]` {
		t.Fatalf("indicator JSON = %s", text)
	}
	if text == "" || text == `[false,null,false,false,false,false,false,false,false,false,false]` {
		t.Fatalf("indicator JSON = %s", text)
	}
}

func TestDetectDivergence(t *testing.T) {
	if !detectDivergence([]float64{10, 11, 12, 13}, []float64{60, 65, 63, 61}, "top", 3) {
		t.Fatal("expected top divergence to be detected")
	}
	if !detectDivergence([]float64{10, 9, 8, 7}, []float64{40, 35, 37, 39}, "bottom", 3) {
		t.Fatal("expected bottom divergence to be detected")
	}
	if detectDivergence([]float64{10, 11, 12, 13}, []float64{60, 62, 64, 66}, "top", 3) {
		t.Fatal("did not expect divergence when indicator confirms price")
	}
}
