package backtest

import (
	"context"
	"fmt"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

type pineMigrationCorpusCase struct {
	name        string
	script      string
	wantOK      bool
	runBacktest bool
}

func TestPineV13MigrationCorpusGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := pineV13MigrationCorpus()
	if len(cases) < 60 {
		t.Fatalf("migration corpus size = %d, want at least 60", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 12 {
		t.Fatalf("run corpus size = %d, want at least 12 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v1.3 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 75 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 75", weighted)
	}
}

func TestPineV14MigrationCorpusGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := pineV14MigrationCorpus()
	if len(cases) < 80 {
		t.Fatalf("migration corpus size = %d, want at least 80", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 20 {
		t.Fatalf("run corpus size = %d, want at least 20 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v1.4 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 82 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 82", weighted)
	}
}

func TestPineV15MigrationCorpusGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := pineV15MigrationCorpus()
	if len(cases) < 100 {
		t.Fatalf("migration corpus size = %d, want at least 100", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 28 {
		t.Fatalf("run corpus size = %d, want at least 28 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v1.5 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 87 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 87", weighted)
	}
}

func pineV13MigrationCorpus() []pineMigrationCorpusCase {
	base := []pineMigrationCorpusCase{
		corpusCase("ema cross", true, true, `fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("rsi close", true, true, `r = ta.rsi(close, 14)
if r < 35
    strategy.entry("Long", strategy.long, qty=1)
if r > 65
    strategy.close("Long")`),
		corpusCase("mtf source", true, true, `mtf = request.security(syminfo.tickerid, "15", close)
if close > mtf
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("mtf ema hlc3", true, true, `mtf = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 3))
if close > mtf
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("bracket exit", true, true, `if close > open
    strategy.entry("Long", strategy.long, qty=1)
    strategy.exit("Bracket", "Long", stop=close * 0.98, limit=close * 1.04)`),
		corpusCase("pending stop cancel", true, true, `if close < open
    strategy.entry("Breakout", strategy.long, stop=high + 1, qty=1)
else
    strategy.cancel("Breakout")`),
		corpusCase("allow long", true, true, `strategy.risk.allow_entry_in(strategy.direction.long)
if close > open
    strategy.entry("Long", strategy.long, qty=1)
else
    strategy.entry("ShortBlocked", strategy.short, qty=1)`),
		corpusCase("switch expression", true, true, `signal = switch
    close > open => 1
    close < open => -1
    => 0
if signal > 0
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("multi udf", true, true, `score(src) =>
    base = src - src[1]
    if base > 0
        base
    else
        0
if score(close) > 0
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("v13 indicators", true, true, `cmoValue = ta.cmo(close, 5)
rankValue = ta.percentrank(close, 5)
swmaValue = ta.swma(close)
if cmoValue > 0 and rankValue > 50 and swmaValue > 0
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("v13 mtf indicators", true, true, `mtfCmo = request.security(syminfo.tickerid, "15", ta.cmo(close, 5))
mtfSwma = request.security(syminfo.tickerid, "15", ta.swma(close))
if mtfCmo > 0 and mtfSwma > 0
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("math mintick", true, true, `mid = math.round_to_mintick(math.avg(open, close))
if close > mid
    strategy.entry("Long", strategy.long, qty=1)`),
	}
	for index, indicator := range []string{
		"ta.linreg(close, 5, 0)", "ta.obv", "ta.pivothigh(high, 2, 2)", "ta.pivotlow(low, 2, 2)",
		"ta.alma(close, 5, 0.85, 6)", "ta.correlation(close, high, 5)", "ta.dev(close, 5)",
		"ta.median(close, 5)", "ta.percentile_linear_interpolation(close, 5, 50)",
		"ta.percentile_nearest_rank(close, 5, 80)", "ta.tsi(close, 2, 3)",
	} {
		base = append(base, corpusCase(fmt.Sprintf("indicator-%02d", index+1), true, false, fmt.Sprintf(`value = %s
if nz(value, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`, indicator)))
	}
	for index, length := range []int{3, 5, 8, 13, 21, 34, 55, 89, 144, 233} {
		base = append(base, corpusCase(fmt.Sprintf("ma-family-%02d", index+1), true, false, fmt.Sprintf(`fast = ta.ema(close, %d)
slow = ta.sma(close, %d)
if fast > slow
    strategy.entry("Long", strategy.long, qty=1)`, length, length+2)))
	}
	for index, tf := range []string{"1", "5", "15", "30", "45", "60", "120", "240", "D", "W"} {
		base = append(base, corpusCase(fmt.Sprintf("security-source-%02d", index+1), true, false, fmt.Sprintf(`mtf = request.security(syminfo.tickerid, "%s", close)
if close >= mtf
    strategy.entry("Long", strategy.long, qty=1)`, tf)))
	}
	for index, body := range []string{
		`upper = ta.highest(high, 20)
lower = ta.lowest(low, 20)
if close > upper[1]
    strategy.entry("Long", strategy.long, qty=1)
if close < lower[1]
    strategy.close("Long")`,
		`[macdLine, signalLine, histLine] = ta.macd(close, 12, 26, 9)
if histLine > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[basis, upper, lower] = ta.bb(close, 20, 2)
if close < lower
    strategy.entry("Long", strategy.long, qty=1)`,
		`[basis, upper, lower] = ta.kc(close, 5, 1.5)
width = ta.kcw(close, 5, 1.5)
if close > basis and width > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`for i = 0 to 2
    total := nz(total, 0) + nz(close[i], close)
if total > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`momentum = ta.mom(close, 5)
rate = ta.roc(close, 5)
if momentum > 0 and rate > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`if barstate.isconfirmed and session.ismarket
    strategy.entry("Long", strategy.long, qty_percent=10)`,
		`strategy.entry("Long", strategy.long, qty=1, limit=low)
strategy.cancel_all()`,
		`strategy.entry("Long", strategy.long, qty=1)
strategy.close_all(immediately=true)`,
		`almaValue = request.security(syminfo.tickerid, "15", ta.alma(close, 5, 0.85, 6))
if almaValue > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`pct = request.security(syminfo.tickerid, "15", ta.percentile_nearest_rank(close, 5, 80))
if pct > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`corr = request.security(syminfo.tickerid, "15", ta.correlation(close, high, 5))
if corr > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`value = ta.valuewhen(close > open, close, 0)
if nz(value, 0) > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`bars = ta.barssince(close > open)
if nz(bars, 999) < 3
    strategy.entry("Long", strategy.long, qty=1)`,
		`trValue = ta.tr(true)
if trValue > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`if time > timestamp(2026, 1, 1)
    strategy.entry("Long", strategy.long, qty=1)`,
		`source = input.source(close, "Source")
len = input.int(5, "Length")
avg = ta.sma(source, len)
if close > avg
    strategy.entry("Long", strategy.long, qty=1)`,
		`tf = input.timeframe("15", "TF")
mtf = request.security(syminfo.tickerid, tf, close)
if mtf > 0
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("pattern-%02d", index+1), true, false, body))
	}
	for index, body := range []string{
		`arr = array.new_float(0)`,
		`while close > open
    strategy.entry("Long", strategy.long, qty=1)`,
		`x = request.security("NASDAQ:AAPL", "D", close)`,
		`x = request.security(syminfo.tickerid, "D", close, lookahead=barmerge.lookahead_on)`,
		`x = request.security(syminfo.tickerid, "D", ta.stoch(close, high, low, 14))`,
		`import TradingView/ta/7`,
		`type Foo
    int bar`,
		`m = matrix.new<float>(1, 1)`,
		`strategy.exit("TrailMix", "Long", stop=close * 0.98, trail_points=10, trail_offset=5)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("unsupported-%02d", index+1), false, false, body))
	}
	return base
}

func pineV14MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV13MigrationCorpus()...)
	for index, body := range []string{
		`signal = request.security(syminfo.tickerid, "15", close > ta.sma(close, 3))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(close[1], close) < close and open < close)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`mtf = request.security(syminfo.tickerid, "15", close > ta.ema(hlc3, 3) and volume > 0)
if mtf
    strategy.entry("Long", strategy.long, qty=1)`,
		`bars = ta.barssince(close > open)
value = ta.valuewhen(close > open, close, 0)
if nz(bars, 999) < 4 and nz(value, close) >= close
    strategy.entry("Long", strategy.long, qty=1)`,
		`trA = ta.tr(true)
trB = ta.tr(false)
if trA >= trB and trA > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`hb = ta.highestbars(high, 5)
lb = ta.lowestbars(low, 5)
if hb >= 0 or lb >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`delta = ta.change(close)
momentum = ta.mom(close, 3)
rate = ta.roc(close, 3)
if nz(delta, 0) + nz(momentum, 0) + nz(rate, 0) > -100
    strategy.entry("Long", strategy.long, qty=1)`,
		`up = ta.rising(close, 3)
down = ta.falling(close, 3)
if up or not down
    strategy.entry("Long", strategy.long, qty=1)`,
		`dev = ta.stdev(close, 5)
variance = ta.variance(close, 5)
if nz(dev, 0) >= 0 and nz(variance, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`mtf = request.security(syminfo.tickerid, "15", close > ta.sma(close, 3) ? 1 : 0)
if mtf > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`mtf = request.security(syminfo.tickerid, "15", math.avg(close, open) > ta.sma(close, 3))
if mtf
    strategy.entry("Long", strategy.long, qty=1)`,
		`mtf = request.security(syminfo.tickerid, "15", nz(close[2], close) <= math.max(close, open))
if mtf
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v14-run-%02d", index+1), true, true, body))
	}
	for index, body := range []string{
		`x = request.security(syminfo.tickerid, "15", alert("x"))`,
		`x = request.security(syminfo.tickerid, "15", strategy.position_size)`,
		`x = request.security(syminfo.tickerid, "15", ta.stoch(close, high, low, 14))`,
		`x = request.security(syminfo.tickerid, "15", [close, open])`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v14-unsupported-%02d", index+1), false, false, body))
	}
	return base
}

func pineV15MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV14MigrationCorpus()...)
	for index, body := range []string{
		`signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(close, 14), 50) > 50)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.macd(close, 12, 26, 9).diff, 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.atr(14), 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.bb(close, 20, 2).upper, close) > close)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.supertrend(3, 10).direction, 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(hlc3, 7), 45) > 45 and nz(ta.atr(7), 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.macd(hlc3, 5, 13, 4).diff, 0) > nz(ta.macd(hlc3, 5, 13, 4).signal, 0))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.bb(hlc3, 10, 1.5).lower, close) < close and nz(ta.rsi(close, 5), 40) > 40)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
recent = ta.barssince(ta.cross(fast, slow))
last = ta.valuewhen(ta.crossover(fast, slow), close, 0)
if ta.crossover(fast, slow) or (nz(recent, 999) < 5 and close > nz(last, close))
    strategy.entry("Long", strategy.long, qty=1)`,
		`fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
if ta.crossunder(fast, slow)
    strategy.close("Long")
if ta.cross(fast, slow)
    strategy.entry("Long", strategy.long, qty=1)`,
		`score = 0
for i = 1 to 4
    score := score + i
    continue
    score := score + 100
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`score = 0
for i = 1 to 4
    score := score + i
    break
    score := score + 100
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v15-run-%02d", index+1), true, true, body))
	}
	for index, body := range []string{
		`signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(close, 5), 0) > ta.sma(close, 3))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.atr(5), 0) > math.max(high - low, 0))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.supertrend(2.5, 7).line, close) < close)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.bb(close, 5, 2).middle, close) > nz(close[1], close))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`value = ta.range(close, 5)
if nz(value, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`modeValue = ta.mode(close, 5)
if nz(modeValue, close) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`mixed = ta.rma(hlc3, 5) + ta.wma(close, 5)
if mixed > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`state = ta.valuewhen(ta.cross(close, open), ta.rsi(close, 5), 0)
if nz(state, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v15-pattern-%02d", index+1), true, false, body))
	}
	for index, body := range []string{
		`x = request.security(syminfo.tickerid, "15", ta.stoch(close, high, low, 14))`,
		`x = request.security(syminfo.tickerid, "15", ta.rsi(close, 14), lookahead=barmerge.lookahead_on)`,
		`x = request.security(syminfo.tickerid, "15", strategy.position_size + ta.rsi(close, 14))`,
		`for i = 1 to 4
    if close > open
        break`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v15-unsupported-%02d", index+1), false, false, body))
	}
	return base
}

func corpusCase(name string, wantOK bool, runBacktest bool, body string) pineMigrationCorpusCase {
	return pineMigrationCorpusCase{
		name:        name,
		wantOK:      wantOK,
		runBacktest: runBacktest,
		script: `//@version=6
strategy("` + name + `", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)
` + body,
	}
}
