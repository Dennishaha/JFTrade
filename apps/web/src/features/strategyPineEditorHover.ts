import type { MonacoHoverDefinition } from "./strategyMonacoIntelliSenseTypes";

export const strategyPineEditorHoverItems: MonacoHoverDefinition[] = [
  {
    target: "ta.ema",
    signature: "ta.ema(source, length)",
    documentation: "Pine v6 EMA；source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。",
  },
  {
    target: "ta.sma",
    signature: "ta.sma(source, length)",
    documentation: "Pine v6 SMA；source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。",
  },
  {
    target: "ta.rsi",
    signature: "ta.rsi(source, length)",
    documentation: "Pine v6 RSI；PineTS worker 支持 source-aware 计算。",
  },
  {
    target: "ta.macd",
    signature: "[macdLine, signalLine, histLine] = ta.macd(source, fast, slow, signal)",
    documentation: "Pine v6 MACD 三元组；可在 tuple assignment、字段读取和支持的 MTF 表达式中使用。",
  },
  {
    target: "ta.roc",
    signature: "ta.roc(source, length)",
    documentation: "Rate of Change；source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。",
  },
  {
    target: "ta.crossover",
    signature: "ta.crossover(left, right) -> bool",
    documentation: "Pine v6 交叉检测；在 PineTS worker 中按上一根与当前 closed bar 判断。",
  },
  {
    target: "ta.crossunder",
    signature: "ta.crossunder(left, right) -> bool",
    documentation: "Pine v6 下穿检测；在 PineTS worker 中按上一根与当前 closed bar 判断。",
  },
  {
    target: "ta.cross",
    signature: "ta.cross(left, right) -> bool",
    documentation: "Pine v6 任意方向交叉检测；在 PineTS worker 中执行。",
  },
  {
    target: "ta.highest",
    signature: "ta.highest(source, length) / ta.highest(length)",
    documentation: "JFTrade 支持 open/high/low/close/volume/hl2/hlc3/ohlc4 source；单参数 highest(length) 默认 high。",
  },
  {
    target: "ta.lowest",
    signature: "ta.lowest(source, length) / ta.lowest(length)",
    documentation: "JFTrade 支持 open/high/low/close/volume/hl2/hlc3/ohlc4 source；单参数 lowest(length) 默认 low。",
  },
  {
    target: "ta.bb",
    signature: "[basis, upper, lower] = ta.bb(close, length, mult)",
    documentation: "Pine v6 Bollinger Bands 三元组；可在条件、字段读取和支持的 MTF 表达式中使用。",
  },
  {
    target: "ta.wpr",
    signature: "ta.wpr(length)",
    documentation: "Pine v6 Williams %R；按 closed bar 数据计算。",
  },
  {
    target: "ta.sum",
    signature: "ta.sum(source, length)",
    documentation: "滚动窗口求和；source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。",
  },
  {
    target: "ta.rma",
    signature: "ta.rma(source, length)",
    documentation: "Pine v6 RMA；PineTS worker 支持 source-aware 计算。",
  },
  {
    target: "ta.wma",
    signature: "ta.wma(source, length)",
    documentation: "Pine v6 WMA；PineTS worker 支持 source-aware 计算。",
  },
  {
    target: "ta.hma",
    signature: "ta.hma(source, length)",
    documentation: "Pine v6 HMA；PineTS worker 支持 source-aware 计算。",
  },
  {
    target: "ta.vwma",
    signature: "ta.vwma(source, length)",
    documentation: "Pine v6 VWMA；使用 source 与 volume 计算成交量加权均线。",
  },
  {
    target: "ta.atr",
    signature: "ta.atr(length)",
    documentation: "Average True Range；可在策略条件、变量和静态同标的 MTF 表达式中使用。",
  },
  {
    target: "ta.cci",
    signature: "ta.cci(source, length)",
    documentation: "Commodity Channel Index；source-aware CCI，默认常用 hlc3。",
  },
  {
    target: "ta.highestbars",
    signature: "ta.highestbars(source, length)",
    documentation: "返回窗口最高值相对当前 bar 的偏移。",
  },
  {
    target: "ta.lowestbars",
    signature: "ta.lowestbars(source, length)",
    documentation: "返回窗口最低值相对当前 bar 的偏移。",
  },
  {
    target: "ta.change",
    signature: "ta.change(source[, length])",
    documentation: "返回 source 与 length 根前的差值；未传 length 时默认 1。",
  },
  {
    target: "ta.mom",
    signature: "ta.mom(source, length)",
    documentation: "Momentum；source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。",
  },
  {
    target: "ta.range",
    signature: "ta.range(source, length)",
    documentation: "滚动最高值与最低值之差。",
  },
  {
    target: "ta.mode",
    signature: "ta.mode(source, length)",
    documentation: "滚动众数；并列时返回较小值。",
  },
  {
    target: "ta.rising",
    signature: "ta.rising(source, length)",
    documentation: "判断 source 是否连续上升 length 根。",
  },
  {
    target: "ta.falling",
    signature: "ta.falling(source, length)",
    documentation: "判断 source 是否连续下降 length 根。",
  },
  {
    target: "ta.stdev",
    signature: "ta.stdev(source, length)",
    documentation: "滚动标准差；PineTS worker 支持 source-aware 计算。",
  },
  {
    target: "ta.variance",
    signature: "ta.variance(source, length)",
    documentation: "滚动方差；PineTS worker 支持 source-aware 计算。",
  },
  {
    target: "ta.cum",
    signature: "ta.cum(source)",
    documentation: "从首根 bar 起累计 source。",
  },
  {
    target: "ta.stoch",
    signature: "ta.stoch(source, high, low, length)",
    documentation: "Stochastic；支持静态同标的 request.security。",
  },
  {
    target: "ta.tr",
    signature: "ta.tr(handle_na?)",
    documentation: "True Range；可直接读取当前 bar 的 TR。",
  },
  {
    target: "hlc3",
    signature: "hlc3",
    documentation: "派生价格源 (high + low + close) / 3；同批支持 hl2 和 ohlc4。",
  },
  {
    target: "series[n]",
    signature: "identifier[n] / object.field[n]",
    documentation: "读取前 n 根 closed bar 的历史值；函数调用结果请先赋值后再使用历史引用。",
  },
  {
    target: "=>",
    signature: "name(arg1, arg2) => expression",
    documentation: "JFTrade 支持顶层表达式 UDF，编译期内联；参数数量必须匹配，不支持递归或多语句函数。",
  },
  {
    target: "for",
    signature: "for i = start to end [by step]",
    documentation: "JFTrade 支持静态整数边界循环，按 Pine inclusive to 语义最多展开 100 次；v2.4 中静态 for 遇到条件 break/continue 会 fallback 到有界 runtime loop。",
  },
  {
    target: "alertcondition",
    signature: "alertcondition(condition, title?, message?)",
    documentation: "PineTS worker 会产出 alertcondition 事件；JFTrade 通过 alerts 边界透出，交易执行不消费该声明。",
  },
  {
    target: "plot",
    signature: "plot(series, ...)",
    documentation: "PineTS worker 会计算 plot 序列并透出到 plots；Go 交易链路不消费 plot。",
  },
  {
    target: "plotshape",
    signature: "plotshape(condition, title?, text?, ...)",
    documentation: "PineTS worker 支持形状标记；JFTrade 将其归为 visual output，不进入订单 intent。",
  },
  {
    target: "plotchar",
    signature: "plotchar(condition, title?, char?, ...)",
    documentation: "PineTS worker 支持字符标记；JFTrade 将其归为 visual output，不进入订单 intent。",
  },
  {
    target: "label.new",
    signature: "label.new(x, y, text, ...)",
    documentation: "PineTS worker 支持 label drawing；JFTrade 将 drawing 对象归为 visual output。",
  },
  {
    target: "line.new",
    signature: "line.new(x1, y1, x2, y2, ...)",
    documentation: "PineTS worker 支持 line drawing；JFTrade 将 drawing 对象归为 visual output。",
  },
  {
    target: "box.new",
    signature: "box.new(left, top, right, bottom, ...)",
    documentation: "PineTS worker 支持 box drawing；JFTrade 将 drawing 对象归为 visual output。",
  },
  {
    target: "table.new",
    signature: "table.new(position, columns, rows, ...)",
    documentation: "PineTS worker 支持 table 对象；JFTrade 将 table 归为 visual output。",
  },
  {
    target: "array.new_float",
    signature: "array.new_float(size, initial_value?)",
    documentation: "v2.4 可执行 array 核心；支持常用读写、队列操作、from/concat/sort/sort_indices/binary_search/join/median/mode/range 与跨 K 线 var 状态。",
  },
  {
    target: "array.from",
    signature: "array.from(value1, value2, ...)",
    documentation: "v2.4 支持从参数列表构造 array，可继续使用 method 形式调用 concat/sort/join/aggregate。",
  },
  {
    target: "array.sort",
    signature: "array.sort(id, order?) / id.sort(order?)",
    documentation: "v2.4 支持 number/string/bool/nil 的确定性排序；nil 放末尾，order 支持 order.ascending/order.descending。",
  },
  {
    target: "array.sort_indices",
    signature: "array.sort_indices(id, order?) / id.sort_indices(order?)",
    documentation: "v2.4 返回排序后的原索引 array，不修改原 array；排序规则与 array.sort 一致。",
  },
  {
    target: "array.binary_search",
    signature: "array.binary_search(id, value) / id.binary_search(value)",
    documentation: "v2.4 在已排序 array 中查找目标值，未命中返回 -1。",
  },
  {
    target: "array.percentile_linear_interpolation",
    signature: "array.percentile_linear_interpolation(id, percentage) / id.percentile_linear_interpolation(percentage)",
    documentation: "v2.5 支持数值 array percentile/percentrank/stdev/variance/covariance 统计子集。",
  },
  {
    target: "for...in",
    signature: "for value in array / for [index, value] in array",
    documentation: "v2.6 支持 array for-in，使用 bounded runtime loop；map 请通过 keys()/values() 转为确定性 array。",
  },
  {
    target: "array history",
    signature: "arr[n].get(index) / arr[n].size() / arr[n].range() / arr[n].stdev()",
    documentation: "v2.7 支持只读 collection history snapshot 和常用 aggregate/stat；历史 collection mutation 不执行。",
  },
  {
    target: "str.format",
    signature: "str.format(format, value1, value2, ...)",
    documentation: "v2.5 支持常用 str.* helper；str.format 使用 {0}/{1} 占位替换。",
  },
  {
    target: "str.length",
    signature: "str.length(source)",
    documentation: "返回字符串长度。",
  },
  {
    target: "str.tostring",
    signature: "str.tostring(value, format?)",
    documentation: "把值转换为字符串；可选 format。",
  },
  {
    target: "str.contains",
    signature: "str.contains(source, needle)",
    documentation: "判断 source 是否包含 needle。",
  },
  {
    target: "str.pos",
    signature: "str.pos(source, needle)",
    documentation: "返回 needle 在 source 中的位置，未找到时返回 -1。",
  },
  {
    target: "str.substring",
    signature: "str.substring(source, begin, end?)",
    documentation: "返回字符串子串。",
  },
  {
    target: "str.replace",
    signature: "str.replace(source, target, replacement)",
    documentation: "替换 source 中的 target。",
  },
  {
    target: "str.upper",
    signature: "str.upper(source)",
    documentation: "返回大写字符串。",
  },
  {
    target: "str.lower",
    signature: "str.lower(source)",
    documentation: "返回小写字符串。",
  },
  {
    target: "timeframe.change",
    signature: "timeframe.change(static_timeframe)",
    documentation: "v2.5 支持静态 timeframe 边界判断；同批支持 time_close。",
  },
  {
    target: "timeframe.in_seconds",
    signature: "timeframe.in_seconds(timeframe?)",
    documentation: "v2.7 支持静态 timeframe 秒数转换；无参数时使用当前 runtime interval。",
  },
  {
    target: "array.median",
    signature: "array.median(id) / id.median()",
    documentation: "v2.4 支持数值 array 中位数；同批支持 mode/range 聚合。",
  },
  {
    target: "order.ascending",
    signature: "order.ascending / order.descending",
    documentation: "v2.4 array.sort 和 array.sort_indices 的排序方向常量。",
  },
  {
    target: "map.new",
    signature: "map.new<key_type, value_type>()",
    documentation: "v2.4 可执行 map 核心；支持 get/put/remove/contains/size/clear/copy/keys/values。",
  },
  {
    target: "map.keys",
    signature: "map.keys(id) / id.keys()",
    documentation: "v2.4 返回 key array；顺序按 key 的稳定字符串序排序，保证回测可复现。",
  },
  {
    target: "map.values",
    signature: "map.values(id) / id.values()",
    documentation: "v2.4 返回 value array；顺序跟 map.keys 一致，按 key 的稳定字符串序排序。",
  },
  {
    target: "map iteration",
    signature: "for key in map.keys()",
    documentation: "v2.7 支持通过 keys()/values() 得到的确定性 array 做 for-in。",
  },
  {
    target: "matrix.new",
    signature: "matrix.new<T>(rows, columns, initial_value?)",
    documentation: "v2.3 可执行 matrix 核心；支持 get/set/rows/columns/fill/copy/reshape/add_row/add_col/remove_row/remove_col。",
  },
  {
    target: "type",
    signature: "type Name",
    documentation: "v2.6 支持纯 UDT constructor、字段默认值、命名 constructor 参数、局部和 var object 字段重赋值，以及 collection 字段引用；完整 Pine type/library 系统仍只进入诊断。",
  },
  {
    target: "method",
    signature: "method name(Type self, ...)",
    documentation: "v2.8 支持单表达式和受控多语句纯 method、默认值、命名参数与无副作用 method chain；表达式 method 可在受限 request.security pure expression 内调用。",
  },
  {
    target: "object history",
    signature: "box[n].field / box[n].method(args)",
    documentation: "v2.9 支持 object 历史字段读取和纯 method receiver，返回只读历史 snapshot 的字段值或 method 结果。",
  },
  {
    target: "varip",
    signature: "varip name = expression",
    documentation: "v3.0 PineTS worker 将 varip 按 var 语义执行，并通过 warning 标出 intrabar 语义边界。",
  },
  {
    target: "semantic declarations",
    signature: "signature / unsupportedReason",
    documentation: "v3.0 AnalyzeScript declarations 增补稳定 signature 与 unsupportedReason 字段；旧 reason 字段保留兼容。",
  },
  {
    target: "export",
    signature: "export function/type/method",
    documentation: "v2.8 export 会进入 semantic declarations，并标记 exportedKind=function/type/method；不加载外部 library。",
  },
  {
    target: "import",
    signature: "import user/library/version",
    documentation: "v2.0 语言底座：import 会进入 semantic declarations 与 alias 诊断；当前不会加载 TradingView library。",
  },
  {
    target: "library",
    signature: "library(title, ...)",
    documentation: "v2.0 语言底座：library(...) 会进入 semantic declarations；JFTrade 可运行脚本仍以 strategy(...) 为主入口。",
  },
  {
    target: "ta.vwap",
    signature: "ta.vwap(source?) / ta.vwap(source, timeframe.change(\"D\"|\"W\"|\"M\"))",
    documentation: "JFTrade 支持交易日 VWAP 和闭盘日/周/月锚定重置；source 省略时默认 hlc3。",
  },
  {
    target: "ta.bbw",
    signature: "ta.bbw(source, length, mult)",
    documentation: "PineTS worker 支持 Bollinger Band Width 与静态同标的 MTF 计算。",
  },
  {
    target: "ta.cog",
    signature: "ta.cog(source, length)",
    documentation: "PineTS worker 支持 Center of Gravity 与静态同标的 MTF 计算。",
  },
  {
    target: "ta.mfi",
    signature: "ta.mfi(source, length)",
    documentation: "JFTrade 支持基于 source 与 volume 的 Money Flow Index。",
  },
  {
    target: "ta.dmi",
    signature: "[plusDI, minusDI, adx] = ta.dmi(diLength, adxSmoothing)",
    documentation: "JFTrade 支持 DMI tuple assignment；字段包括 plus、minus、adx。",
  },
  {
    target: "ta.supertrend",
    signature: "[line, direction] = ta.supertrend(factor, atrPeriod)",
    documentation: "JFTrade 支持 Supertrend line/direction tuple assignment。",
  },
  {
    target: "ta.sar",
    signature: "ta.sar(start, increment, max)",
    documentation: "JFTrade 支持 Parabolic SAR，planner requirement key 形如 sar:0.02:0.02:0.2。",
  },
  {
    target: "ta.linreg",
    signature: "ta.linreg(source, length, offset)",
    documentation: "线性回归值；offset 必须是非负静态整数。",
  },
  {
    target: "ta.pivothigh",
    signature: "ta.pivothigh(source?, leftbars, rightbars)",
    documentation: "在 right bars 后确认 pivot high；确认前返回 na。",
  },
  {
    target: "ta.pivotlow",
    signature: "ta.pivotlow(source?, leftbars, rightbars)",
    documentation: "在 right bars 后确认 pivot low；确认前返回 na。",
  },
  {
    target: "ta.kc",
    signature: "[basis, upper, lower] = ta.kc(source, length, mult, useTrueRange?)",
    documentation: "JFTrade 支持 Keltner Channel tuple assignment 与字段读取。",
  },
  {
    target: "ta.kcw",
    signature: "ta.kcw(source, length, mult, useTrueRange?)",
    documentation: "返回归一化 Keltner Channel 宽度。",
  },
  {
    target: "ta.alma",
    signature: "ta.alma(source, length, offset, sigma)",
    documentation: "Arnaud Legoux Moving Average。",
  },
  {
    target: "ta.cmo",
    signature: "ta.cmo(source, length)",
    documentation: "Chande Momentum Oscillator；PineTS worker 支持 source-aware 计算。",
  },
  {
    target: "ta.tsi",
    signature: "ta.tsi(source, shortLength, longLength)",
    documentation: "True Strength Index；使用双 EMA 平滑 momentum 和绝对 momentum。",
  },
  {
    target: "ta.correlation",
    signature: "ta.correlation(source1, source2, length)",
    documentation: "滚动 Pearson 相关系数。",
  },
  {
    target: "ta.dev",
    signature: "ta.dev(source, length)",
    documentation: "滚动平均绝对偏差。",
  },
  {
    target: "ta.median",
    signature: "ta.median(source, length)",
    documentation: "滚动中位数。",
  },
  {
    target: "ta.percentile_linear_interpolation",
    signature: "ta.percentile_linear_interpolation(source, length, percentage)",
    documentation: "滚动百分位线性插值，percentage 必须为 0..100。",
  },
  {
    target: "ta.percentile_nearest_rank",
    signature: "ta.percentile_nearest_rank(source, length, percentage)",
    documentation: "滚动 nearest-rank 百分位，percentage 必须为 0..100。",
  },
  {
    target: "ta.percentrank",
    signature: "ta.percentrank(source, length)",
    documentation: "当前值在滚动窗口中的百分排名。",
  },
  {
    target: "ta.swma",
    signature: "ta.swma(source)",
    documentation: "4-bar symmetric weighted moving average。",
  },
  {
    target: "ta.barssince",
    signature: "ta.barssince(condition)",
    documentation: "首次触发前返回 na，当前 bar 触发返回 0。",
  },
  {
    target: "ta.valuewhen",
    signature: "ta.valuewhen(condition, sourceExpression, occurrence)",
    documentation: "occurrence 必须为非负整数；同一 bar 多次读取不会重复推进状态。",
  },
  {
    target: "timestamp",
    signature: "timestamp(year, month, day[, hour, minute])",
    documentation: "按当前标的交易所时区解释并返回 Unix milliseconds；第一版不支持显式 timezone 参数。",
  },
  {
    target: "input.time",
    signature: "input.time(defval, title?)",
    documentation: "JFTrade 只取默认时间值；常用 defval 可写 timestamp(year, month, day[, hour, minute])。",
  },
  {
    target: "input.timeframe",
    signature: "input.timeframe(defval, title?)",
    documentation: "JFTrade 只取默认 timeframe 字符串；支持 1/5/15/30/45/60/120/240/D/W/M 的 request.security 子集。",
  },
  {
    target: "input.color",
    signature: "input.color(defval, title?)",
    documentation: "JFTrade 只取默认颜色值；颜色主要用于兼容 Pine 模板，不参与交易数值语义。",
  },
  {
    target: "barstate.isconfirmed",
    signature: "barstate.isconfirmed",
    documentation: "PineTS worker 中当前已知 K 线执行时为 true；同批支持 isfirst/isnew/ishistory/isrealtime/islast。",
  },
  {
    target: "session.ismarket",
    signature: "session.ismarket",
    documentation: "当前 K 线属于 regular session 时为 true；同批支持 ispremarket/ispostmarket。",
  },
  {
    target: "dayofweek.monday",
    signature: "dayofweek.monday",
    documentation: "PineTS worker 将 dayofweek.sunday...saturday 归一为 1...7。",
  },
  {
    target: "month.january",
    signature: "month.january",
    documentation: "PineTS worker 将 month.january...december 归一为 1...12。",
  },
  {
    target: "color.rgb",
    signature: "color.rgb(r, g, b)",
    documentation: "PineTS worker normalizes 为稳定十六进制颜色字符串。",
  },
  {
    target: "color.new",
    signature: "color.new(color, transp)",
    documentation: "JFTrade 当前忽略 transp 并返回原颜色，用于兼容常见 Pine 模板。",
  },
  {
    target: "input.int",
    signature: "input.int(defval, title?)",
    documentation: "JFTrade 取默认值并作为常量执行，不提供 TradingView 设置面板运行时覆盖。",
  },
  {
    target: "input",
    signature: "input(defval, title?)",
    documentation: "JFTrade 取默认值并作为常量执行，不提供 TradingView 设置面板运行时覆盖。",
  },
  {
    target: "input.float",
    signature: "input.float(defval, title?)",
    documentation: "JFTrade 取默认浮点值；支持 defval= 命名参数。",
  },
  {
    target: "input.bool",
    signature: "input.bool(defval, title?)",
    documentation: "JFTrade 取默认 bool 值并作为常量执行。",
  },
  {
    target: "input.string",
    signature: "input.string(defval, title?)",
    documentation: "JFTrade 取默认字符串值并作为常量执行。",
  },
  {
    target: "input.source",
    signature: "input.source(defval, title?)",
    documentation: "PineTS worker 取默认 OHLCV source；后续 ta.sma(src, n) 按 source-aware MA 计算。",
  },
  {
    target: "math.abs",
    signature: "math.abs(number)",
    documentation: "PineTS worker normalizes 到 abs(number)。",
  },
  {
    target: "math.min",
    signature: "math.min(a, b, ...)",
    documentation: "PineTS worker normalizes 到 min(a, b, ...)。",
  },
  {
    target: "math.max",
    signature: "math.max(a, b, ...)",
    documentation: "PineTS worker normalizes 到 max(a, b, ...)。",
  },
  {
    target: "math.avg",
    signature: "math.avg(a, b, ...)",
    documentation: "PineTS worker normalizes 到 avg(a, b, ...)。",
  },
  {
    target: "math.round",
    signature: "math.round(number, precision?)",
    documentation: "PineTS worker normalizes 到 round(number, precision?)。",
  },
  {
    target: "math.round_to_mintick",
    signature: "math.round_to_mintick(number)",
    documentation: "按当前市场 tick size 四舍五入，缺省 tick 为 0.01。",
  },
  {
    target: "math.floor",
    signature: "math.floor(number)",
    documentation: "PineTS worker normalizes 到 floor(number)。",
  },
  {
    target: "math.ceil",
    signature: "math.ceil(number)",
    documentation: "PineTS worker normalizes 到 ceil(number)。",
  },
  {
    target: "math.sqrt",
    signature: "math.sqrt(number)",
    documentation: "PineTS worker normalizes 到 sqrt(number)。",
  },
  {
    target: "math.pow",
    signature: "math.pow(base, exponent)",
    documentation: "PineTS worker normalizes 到 pow(base, exponent)。",
  },
  {
    target: "math.log",
    signature: "math.log(number)",
    documentation: "PineTS worker normalizes 到 log(number)。",
  },
  {
    target: "math.sign",
    signature: "math.sign(number)",
    documentation: "PineTS worker normalizes 到 sign(number)。",
  },
  {
    target: "bar_index",
    signature: "bar_index",
    documentation: "当前策略收到的 K 线序号，从 0 开始。",
  },
  {
    target: "time",
    signature: "time",
    documentation: "当前 K 线时间，Unix milliseconds；同时支持 hour/minute/dayofweek/dayofmonth/month/year。",
  },
  {
    target: "strategy.entry",
    signature: "strategy.entry(id, direction, qty?)",
    documentation: "显式 qty/qty_percent 优先；未写 qty 时继承 strategy(...) 的 default_qty_type/default_qty_value。",
  },
  {
    target: "strategy.order",
    signature: "strategy.order(id, direction, qty?, qty_percent?, stop?, limit?)",
    documentation: "JFTrade 将其作为净额订单执行，不套用 strategy.entry 的 pyramiding gate；支持 stop、limit 和 stop-limit，OCA 暂不支持。",
  },
  {
    target: "strategy.close",
    signature: "strategy.close(id, qty?, qty_percent?, immediately?, comment?, alert_message?, disable_alert?)",
    documentation: "映射为 JFTrade 平仓；支持立即平仓和订单日志/通知元数据。",
  },
  {
    target: "strategy.close_all",
    signature: "strategy.close_all(immediately?, comment?, alert_message?, disable_alert?)",
    documentation: "按当前实际持仓方向 flatten 当前策略 symbol；支持 immediately=true。",
  },
  {
    target: "var",
    signature: "var name = expression",
    documentation: "声明跨 K 线保留的变量；当前 JFTrade runtime 会保存当前值和上一值。",
  },
  {
    target: "nz",
    signature: "nz(value, fallback?)",
    documentation: "value 为 na 时返回 fallback；未传 fallback 时返回 0。",
  },
  {
    target: "na",
    signature: "na",
    documentation: "空值常量，可用于比较或配合 nz。",
  },
  {
    target: "request.security",
    signature: "request.security(syminfo.tickerid, timeframe, source | source[n] | ta.* | [expr, expr] | pure object/collection expr)",
    documentation: "PineTS worker 支持同标的 1/5/15/30/45/60/120/240/D/W/M，source/source[n]、source-aware 均线、v2.4 ta.stoch、纯表达式、2-8 元 tuple，以及纯 collection/object 表达式；lookahead_on/gaps_on、多标的和副作用表达式会被明确诊断。",
  },
  {
    target: "ticker.heikinashi",
    signature: "ticker.heikinashi(syminfo.tickerid)",
    documentation: "返回当前标的的 Heikin Ashi extended ticker；只可用于静态同标的 request.security。",
  },
  {
    target: "ticker.standard",
    signature: "ticker.standard(syminfo.tickerid?)",
    documentation: "返回当前标的的标准 ticker；在 Heikin Ashi 主图上可用于读取标准 OHLC。",
  },
  {
    target: "ticker.inherit",
    signature: "ticker.inherit(from_tickerid, syminfo.tickerid)",
    documentation: "把受支持的当前标的 extended ticker 修饰符继承到当前标的；不支持外部或动态标的。",
  },
  {
    target: "chart.is_heikinashi",
    signature: "chart.is_heikinashi",
    documentation: "主图为 Heikin Ashi 时为 true。",
  },
  {
    target: "chart.is_standard",
    signature: "chart.is_standard",
    documentation: "主图为标准 K 线时为 true。",
  },
  {
    target: "strategy.exit",
    signature: "strategy.exit(id, from_entry, stop/limit | trail_points|trail_price + trail_offset, qty?, qty_percent?)",
    documentation: "支持基础止损、止盈、stop+limit bracket、按 tick 解释的追踪止损和部分退出；OCA、partial fill 和 intrabar broker emulator 语义不会静默执行。",
  },
  {
    target: "strategy.cancel",
    signature: "strategy.cancel(id)",
    documentation: "取消当前策略 symbol 中指定 id 的未触发 pending order；不存在的 id 会跳过。",
  },
  {
    target: "strategy.cancel_all",
    signature: "strategy.cancel_all()",
    documentation: "取消当前策略 symbol 的全部未触发 pending orders。",
  },
];

