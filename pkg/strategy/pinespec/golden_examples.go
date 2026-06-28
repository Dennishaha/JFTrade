package pinespec

var goldenExamples = []Example{
	{
		ID:          "golden-ma-cross",
		Title:       "均线交叉",
		Description: "覆盖 close-source EMA/SMA、crossover 和基础 entry。",
		Script: `//@version=6
strategy("Golden MA Cross", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)

fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long)`,
		RequirementKeys: []string{"ma:EMA:8", "ma:SMA:21"},
	},
	{
		ID:          "golden-oscillators-bands",
		Title:       "RSI/CCI/Williams/Bollinger",
		Description: "覆盖常见震荡指标、Bollinger 三元组和 close/hlc3 legacy key。",
		Script: `//@version=6
strategy("Golden Oscillators", overlay=true)

rsi14 = ta.rsi(close, 14)
cci20 = ta.cci(hlc3, 20)
williams = ta.wpr(14)
[basis, upper, lower] = ta.bb(close, 20, 2)
if rsi14 < 35 and cci20 < -100 and williams < -80 and close < lower
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{"rsi:14", "cci:20", "williamsr:14", "bollinger:20:2"},
	},
	{
		ID:          "golden-donchian-volume-sar",
		Title:       "Donchian、volume MA 与 SAR",
		Description: "覆盖 rolling highest/lowest、source-aware volume SMA 和 Parabolic SAR。",
		Script: `//@version=6
strategy("Golden Donchian Volume SAR", overlay=true)

upper = ta.highest(high, 20)
lower = ta.lowest(low, 20)
avgVol = ta.sma(volume, 10)
sar = ta.sar(0.02, 0.02, 0.2)
if close > upper and volume > avgVol and close > sar
    strategy.entry("Long", strategy.long, qty=1)
if close < lower
    strategy.close("Long")`,
		RequirementKeys: []string{"highest:high:20", "lowest:low:20", "ma:SMA:10:volume", "sar:0.02:0.02:0.2"},
	},
	{
		ID:          "golden-mtf-source-ma",
		Title:       "MTF source、MA 与高级指标",
		Description: "覆盖 input.timeframe、request.security source/source[n]、source-aware MTF EMA 与静态 intraday MTF linreg。",
		Script: `//@version=6
strategy("Golden MTF", overlay=true)

tf = input.timeframe("15", "Signal TF")
mtfClose = request.security(syminfo.tickerid, tf, close)
mtfPrevClose = request.security(syminfo.tickerid, tf, close[1])
mtfEma = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 3))
mtfLinreg = request.security(syminfo.tickerid, "15", ta.linreg(close, 5, 0))
if mtfClose > mtfPrevClose and close > mtfEma and close > mtfLinreg
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{"security_source:15m:close", "security_source:15m:close:1", "ma:EMA:3:15m:hlc3", "linreg:close:5:0:15m"},
	},
	{
		ID:          "golden-orders-exits",
		Title:       "qty_percent、pending、bracket、cancel",
		Description: "覆盖 percent sizing、strategy.order、pending stop、bracket exit 和 cancel。",
		Script: `//@version=6
strategy("Golden Orders", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)

if close > open
    strategy.entry("Long", strategy.long, qty_percent=10)
    strategy.order("NetLong", strategy.long, qty=1)
    strategy.exit("Bracket", "Long", stop=close * 0.98, limit=close * 1.04, qty_percent=50)
else
    strategy.entry("Breakout", strategy.long, stop=high + 1, qty=1)
    strategy.cancel("Breakout")`,
		RequirementKeys: []string{},
	},
	{
		ID:          "golden-udf-static-for",
		Title:       "UDF 与静态 for",
		Description: "覆盖单表达式 UDF、历史引用、input.int 默认值和静态 for 展开。",
		Script: `//@version=6
strategy("Golden UDF Static For", overlay=true)

isBull(src) => src > src[1]
len = input.int(3, "Length")
fast = ta.ema(close, len)
sum = 0
for i = 0 to 2
    sum := sum + nz(close[i], close)
if isBull(close) and fast > fast[1] and sum > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{"ma:EMA:3"},
	},
	{
		ID:          "golden-v12-advanced-indicators",
		Title:       "v1.2 高频迁移指标",
		Description: "覆盖 linreg、OBV、pivot、Keltner Channel/KCW 与 ALMA。",
		Script: `//@version=6
strategy("Golden v1.2 Indicators", overlay=true)

lr = ta.linreg(close, 5, 0)
obvValue = ta.obv
pivotHigh = ta.pivothigh(high, 2, 2)
pivotLow = ta.pivotlow(low, 2, 2)
[basis, upper, lower] = ta.kc(close, 5, 1.5)
width = ta.kcw(close, 5, 1.5)
almaValue = ta.alma(close, 5, 0.85, 6)
if close > lr and obvValue > 0 and upper > lower and width > 0 and almaValue > 0 and nz(pivotHigh, close) >= nz(pivotLow, close)
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{
			"linreg:close:5:0",
			"obv:close",
			"pivothigh:high:2:2",
			"pivotlow:low:2:2",
			"kc:close:5:1.5:true",
			"kcw:close:5:1.5:true",
			"alma:close:5:0.85:6",
		},
	},
	{
		ID:          "golden-v13-migration-indicators",
		Title:       "v1.3 高频迁移指标",
		Description: "覆盖 CMO、TSI、correlation、dev、median、percentile、percentrank、SWMA、math.avg/round_to_mintick 和 v1.3 intraday MTF 指标。",
		Script: `//@version=6
strategy("Golden v1.3 Indicators", overlay=true)

cmoValue = ta.cmo(close, 5)
tsiValue = ta.tsi(close, 2, 3)
corrValue = ta.correlation(close, high, 5)
devValue = ta.dev(close, 5)
medianValue = ta.median(close, 5)
pLinear = ta.percentile_linear_interpolation(close, 5, 50)
pNearest = ta.percentile_nearest_rank(close, 5, 80)
rankValue = ta.percentrank(close, 5)
swmaValue = ta.swma(close)
mtfCmo = request.security(syminfo.tickerid, "15", ta.cmo(close, 5))
rounded = math.round_to_mintick(math.avg(close, open))
if cmoValue > 0 and tsiValue > 0 and corrValue > 0 and devValue > 0 and medianValue > 0 and pLinear > 0 and pNearest > 0 and rankValue > 0 and swmaValue > 0 and mtfCmo > 0 and rounded > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{
			"cmo:close:5",
			"tsi:close:2:3",
			"correlation:close:high:5",
			"dev:close:5",
			"median:close:5",
			"percentile_linear_interpolation:close:5:50",
			"percentile_nearest_rank:close:5:80",
			"percentrank:close:5",
			"swma:close",
			"cmo:close:5:15m",
		},
	},
	{
		ID:          "golden-v14-window-momentum",
		Title:       "v1.4 窗口与动量指标",
		Description: "覆盖 highestbars、lowestbars、change、mom、roc、rising、falling、stdev 与 variance。",
		Script: `//@version=6
strategy("Golden v1.4 Window Momentum", overlay=true)

dev = ta.stdev(close, 5)
variance = ta.variance(close, 5)
hb = ta.highestbars(high, 5)
lb = ta.lowestbars(low, 5)
delta = ta.change(close)
momentum = ta.mom(close, 3)
rate = ta.roc(close, 3)
up = ta.rising(close, 3)
down = ta.falling(close, 3)
if up and not down and nz(dev, 0) >= 0 and nz(variance, 0) >= 0 and hb >= 0 and lb >= 0 and nz(delta, 0) + nz(momentum, 0) + nz(rate, 0) > -100
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{
			"stdev:5",
			"variance:close:5",
			"highestbars:high:5",
			"lowestbars:low:5",
			"change:close:1",
			"mom:close:3",
			"roc:close:3",
			"rising:close:3",
			"falling:close:3",
		},
	},
	{
		ID:          "golden-v14-state-events",
		Title:       "v1.4 状态事件函数",
		Description: "覆盖 barssince 与 valuewhen 的 closed-bar 状态语义。",
		Script: `//@version=6
strategy("Golden v1.4 State Events", overlay=true)

bars = ta.barssince(close > open)
value = ta.valuewhen(close > open, close, 0)
if nz(bars, 999) < 4 and nz(value, close) >= close
    strategy.entry("Long", strategy.long, qty=1)`,
	},
	{
		ID:          "golden-v14-tr-atr",
		Title:       "v1.4 TR/ATR 组合",
		Description: "覆盖 ta.tr(true|false) 与 ta.atr 的边界组合。",
		Script: `//@version=6
strategy("Golden v1.4 TR ATR", overlay=true)

trTrue = ta.tr(true)
trFalse = ta.tr(false)
range = ta.atr(5)
if trTrue >= trFalse and trTrue > 0 and nz(range, trTrue) > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{"atr:5"},
	},
	{
		ID:          "golden-v14-mtf-pure-expression",
		Title:       "v1.4 MTF 纯表达式",
		Description: "覆盖同标的静态 timeframe 的 request.security 纯表达式、source history、MA、math 与 nz 组合。",
		Script: `//@version=6
strategy("Golden v1.4 MTF Pure", overlay=true)

signal = request.security(syminfo.tickerid, "15", close > ta.sma(close, 3) and nz(close[1], close) > open and math.avg(close, open) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{
			"security_source:15m:close",
			"security_source:15m:close:1",
			"security_source:15m:open",
			"ma:SMA:3:15m",
		},
	},
	{
		ID:          "golden-v15-mtf-common-ta",
		Title:       "v1.5 MTF common TA",
		Description: "覆盖 request.security 纯表达式中的 RSI、MACD、ATR、Bollinger 与 Supertrend 成员读取。",
		Script: `//@version=6
strategy("Golden v1.5 MTF Common TA", overlay=true)

signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(close, 14), 50) > 50 and nz(ta.macd(close, 12, 26, 9).diff, 0) > 0 and nz(ta.atr(14), 0) > 0 and nz(ta.bb(close, 20, 2).upper, close) > close and nz(ta.supertrend(3, 10).direction, 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{
			"security_source:15m:close",
			"rsi:close:14:15m",
			"macd:close:12:26:9:15m",
			"atr:14:15m",
			"bollinger:close:20:2:15m",
			"supertrend:3:10:15m",
		},
	},
	{
		ID:          "golden-v15-cross-state",
		Title:       "v1.5 交叉与状态事件",
		Description: "覆盖 crossover/crossunder/cross 与 barssince/valuewhen 的常见迁移组合。",
		Script: `//@version=6
strategy("Golden v1.5 Cross State", overlay=true)

fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
recentCross = ta.barssince(ta.cross(fast, slow))
lastCrossClose = ta.valuewhen(ta.crossover(fast, slow), close, 0)
if ta.crossover(fast, slow) or (nz(recentCross, 999) < 5 and close > nz(lastCrossClose, close))
    strategy.entry("Long", strategy.long, qty=1)
if ta.crossunder(fast, slow)
    strategy.close("Long")`,
		RequirementKeys: []string{"ma:EMA:8", "ma:SMA:21"},
	},
	{
		ID:          "golden-v15-static-loop-control",
		Title:       "v1.5 静态 for 控制",
		Description: "覆盖静态 for 展开中的无条件 continue 与 break 子集。",
		Script: `//@version=6
strategy("Golden v1.5 Static Loop Control", overlay=true)

score = 0
for i = 1 to 4
    score := score + i
    continue
    score := score + 100
for j = 1 to 4
    score := score + j
    break
    score := score + 100
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`,
	},
	{
		ID:          "golden-v16-mtf-tuple-whitelist",
		Title:       "v1.6 MTF tuple 白名单",
		Description: "覆盖 request.security 的 source、纯表达式与常见多返回 TA tuple 白名单。",
		Script: `//@version=6
strategy("Golden v1.6 MTF Tuple", overlay=true)

[mtfClose, mtfFast, mtfUp] = request.security(syminfo.tickerid, "15", [close, ta.ema(hlc3, 5), close > ta.sma(close, 3)])
[macdLine, signalLine, histLine] = request.security(syminfo.tickerid, "15", ta.macd(close, 12, 26, 9))
[basis, upper, lower] = request.security(syminfo.tickerid, "15", ta.bb(close, 20, 2))
if mtfClose > mtfFast and mtfUp and histLine > signalLine and close < lower
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{
			"security_source:15m:close",
			"ma:EMA:5:15m:hlc3",
			"ma:SMA:3:15m",
			"macd:close:12:26:9:15m",
			"bollinger:close:20:2:15m",
		},
	},
	{
		ID:          "golden-v17-semantic-transition",
		Title:       "v1.7 Semantic 过渡",
		Description: "覆盖 semantic summary 可识别的 input、series symbol、MTF tuple、UDF 与函数签名路径。",
		Script: `//@version=6
strategy("Golden v1.7 Semantic", overlay=true)

len = input.int(8, "Length")
score(src) =>
    base = ta.sma(src, 8)
    if base > 0
        src / base
    else
        1
fast = ta.ema(close, len)
[mtfClose, mtfFast] = request.security(syminfo.tickerid, "15", [close, ta.ema(close, 5)])
if score(close) > 0 and mtfClose > mtfFast and fast > fast[1]
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{
			"ma:SMA:8",
			"ma:EMA:8",
			"security_source:15m:close",
			"ma:EMA:5:15m",
		},
	},
}
