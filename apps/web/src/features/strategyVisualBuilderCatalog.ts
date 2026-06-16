import type { StrategyVisualNodeDocument } from "@/contracts";

import {
  literalExpression,
  normalizeVisualExpression,
  parsePineExpressionToVisualExpression,
  sourceExpression,
  type VisualExpression,
} from "./strategyVisualBuilderExpressions";
import {
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  type TechnicalIndicatorConditionMode,
  type TechnicalIndicatorOperator,
  type TechnicalIndicatorPatternType,
  type TechnicalIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";

export type StrategyBlockKind =
  | "onInit"
  | "onKLineClosed"
  | "pineSnippet"
  | "strategyInput"
  | "derivedSeries"
  | "mtfSeries"
  | "stateVariable"
  | "stateUpdate"
  | "collectionStat"
  | "timeFilter"
  | "sessionFilter"
  | "getTechnicalIndicator"
  | "technicalIndicatorCondition"
  | "seriesCondition"
  | "ifCloseAbove"
  | "ifCloseBelow"
  | "log"
  | "notify"
  | "placeOrder"
  | "stopLoss";

export type {
  TechnicalIndicatorConditionMode,
  TechnicalIndicatorOperator,
  TechnicalIndicatorPatternType,
  TechnicalIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";

export type StopLossDirection = "auto" | "long" | "short";

export type StrategySeriesSource = "open" | "high" | "low" | "close" | "volume" | "hl2" | "hlc3" | "ohlc4";
export type SeriesConditionMode = "compare" | "rising" | "falling" | "barssince" | "valuewhen";
export type SeriesConditionOperator = ">" | "<";
export type StrategyInputType = "int" | "float" | "source" | "timeframe" | "time" | "color";
export type DerivedSeriesMode = "history" | "nz" | "math" | "arithmetic" | "cross";
export type DerivedSeriesMathFunction = "min" | "max" | "abs" | "round" | "floor" | "ceil";
export type DerivedSeriesCrossFunction = "crossover" | "crossunder" | "cross";
export type MtfSeriesExpressionType = "source" | "history" | "indicator";
export type StateValueType = "number" | "bool" | "string";
export type CollectionStatFunction = "min" | "max" | "avg" | "sum" | "median" | "stdev" | "variance" | "percentile";
export type TimeFilterMode = "after" | "before" | "between" | "dayOfWeek";
export type TradingSessionScope = "market" | "premarket" | "postmarket";

export interface SeriesConditionBlockProperties {
  blockKind: "seriesCondition";
  mode?: SeriesConditionMode;
  source?: StrategySeriesSource;
  operator?: SeriesConditionOperator;
  threshold?: number;
  length?: number;
  eventSource?: StrategySeriesSource;
  eventOperator?: SeriesConditionOperator;
  eventThreshold?: number;
  valueSource?: StrategySeriesSource;
  occurrence?: number;
  sourceExpressionAst?: VisualExpression;
  leftExpressionAst?: VisualExpression;
  rightExpressionAst?: VisualExpression;
  eventExpressionAst?: VisualExpression;
  valueExpressionAst?: VisualExpression;
}

export const SERIES_SOURCE_OPTIONS: Array<{ value: StrategySeriesSource; label: string }> = [
  { value: "open", label: "Open" },
  { value: "high", label: "High" },
  { value: "low", label: "Low" },
  { value: "close", label: "Close" },
  { value: "volume", label: "Volume" },
  { value: "hl2", label: "HL2" },
  { value: "hlc3", label: "HLC3" },
  { value: "ohlc4", label: "OHLC4" },
];

export const SERIES_CONDITION_MODE_OPTIONS: Array<{ value: SeriesConditionMode; label: string }> = [
  { value: "compare", label: "序列比较" },
  { value: "rising", label: "连续上升" },
  { value: "falling", label: "连续下降" },
  { value: "barssince", label: "距条件发生" },
  { value: "valuewhen", label: "条件发生时取值" },
];

export const STRATEGY_INPUT_TYPE_OPTIONS: Array<{ value: StrategyInputType; label: string }> = [
  { value: "int", label: "整数" },
  { value: "float", label: "浮点数" },
  { value: "source", label: "序列源" },
  { value: "timeframe", label: "时间周期" },
  { value: "time", label: "时间戳" },
  { value: "color", label: "颜色" },
];

export interface StrategyInputBlockProperties {
  blockKind: "strategyInput";
  variableName?: string;
  inputType?: StrategyInputType;
  title?: string;
  defaultValue?: number | string;
}

export interface DerivedSeriesBlockProperties {
  blockKind: "derivedSeries";
  variableName?: string;
  mode?: DerivedSeriesMode;
  source?: StrategySeriesSource;
  historyOffset?: number;
  fallbackValue?: number;
  mathFunction?: DerivedSeriesMathFunction;
  leftExpression?: string;
  leftExpressionAst?: VisualExpression;
  operator?: "+" | "-" | "*" | "/";
  rightExpression?: string;
  rightExpressionAst?: VisualExpression;
  crossFunction?: DerivedSeriesCrossFunction;
  sourceExpressionAst?: VisualExpression;
  fallbackExpressionAst?: VisualExpression;
}

export interface MtfSeriesBlockProperties {
  blockKind: "mtfSeries";
  variableName?: string;
  timeframe?: string;
  expressionType?: MtfSeriesExpressionType;
  source?: StrategySeriesSource;
  historyOffset?: number;
  indicatorExpression?: string;
  indicatorExpressionAst?: VisualExpression;
  mtfField?: string;
}

export interface StateVariableBlockProperties {
  blockKind: "stateVariable";
  variableName?: string;
  valueType?: StateValueType;
  initialValue?: number | boolean | string;
}

export interface StateUpdateBlockProperties {
  blockKind: "stateUpdate";
  variableName?: string;
  expression?: string;
  expressionAst?: VisualExpression;
}

export interface CollectionStatBlockProperties {
  blockKind: "collectionStat";
  variableName?: string;
  statFunction?: CollectionStatFunction;
  sourceA?: StrategySeriesSource;
  sourceB?: StrategySeriesSource;
  sourceC?: StrategySeriesSource;
  sourceAExpressionAst?: VisualExpression;
  sourceBExpressionAst?: VisualExpression;
  sourceCExpressionAst?: VisualExpression;
  percentile?: number;
}

export const COLLECTION_STAT_FUNCTION_OPTIONS: Array<{ value: CollectionStatFunction; label: string }> = [
  { value: "min", label: "最小值" },
  { value: "max", label: "最大值" },
  { value: "avg", label: "均值" },
  { value: "sum", label: "求和" },
  { value: "median", label: "中位数" },
  { value: "stdev", label: "标准差" },
  { value: "variance", label: "方差" },
  { value: "percentile", label: "百分位" },
];

export type StopLossMode = "stopLoss" | "takeProfit" | "trailingStop" | "bracketExit";

export type StopLossTimeUnit = "bar" | "minute" | "hour" | "day" | "week" | "month";

export type StopLossWindowPolicy = "continuous" | "session";

export interface StopLossBlockProperties {
  blockKind: "stopLoss";
  mode?: StopLossMode;
  direction?: StopLossDirection;
  timeValue?: number;
  timeUnit?: StopLossTimeUnit;
  percentage?: number;
  takeProfitPercentage?: number;
  quantityPercentage?: number;
  windowPolicy?: StopLossWindowPolicy;
  stopPriceExpressionAst?: VisualExpression;
  takeProfitPriceExpressionAst?: VisualExpression;
  trailingPriceExpressionAst?: VisualExpression;
}

export interface TimeFilterBlockProperties {
  blockKind: "timeFilter";
  mode?: TimeFilterMode;
  startHour?: number;
  startMinute?: number;
  endHour?: number;
  endMinute?: number;
  dayOfWeek?: number;
}

export interface SessionFilterBlockProperties {
  blockKind: "sessionFilter";
  scope?: TradingSessionScope;
}

export const STOP_LOSS_MODE_OPTIONS: Array<{ value: StopLossMode; label: string }> = [
  { value: "stopLoss", label: "止损" },
  { value: "takeProfit", label: "止盈" },
  { value: "trailingStop", label: "追踪止损" },
  { value: "bracketExit", label: "止盈止损组合" },
];

export const STOP_LOSS_DIRECTION_OPTIONS: Array<{ value: StopLossDirection; label: string }> = [
  { value: "auto", label: "自动识别持仓方向" },
  { value: "long", label: "仅多头止损" },
  { value: "short", label: "仅空头止损" },
];

export const STOP_LOSS_TIME_UNIT_OPTIONS: Array<{ value: StopLossTimeUnit; label: string }> = [
  { value: "bar", label: "柱" },
  { value: "minute", label: "分钟" },
  { value: "hour", label: "小时" },
  { value: "day", label: "日" },
  { value: "week", label: "周" },
  { value: "month", label: "月" },
];

export const STOP_LOSS_WINDOW_POLICY_OPTIONS: Array<{ value: StopLossWindowPolicy; label: string }> = [
  { value: "continuous", label: "连续窗口" },
  { value: "session", label: "交易时段感知" },
];

export interface StrategyBlockDefinition {
  kind: StrategyBlockKind;
  label: string;
  description: string;
  shape: "circle" | "diamond" | "rect";
  text: string;
  properties: Record<string, unknown>;
  accent: string;
  paletteVisible?: boolean;
}

const STRATEGY_BLOCK_CATALOG: StrategyBlockDefinition[] = [
  {
    kind: "onInit",
    label: "策略启动",
    description: "策略启动时执行一次，适合打印上下文或做预热。",
    shape: "circle",
    text: "策略启动",
    properties: { blockKind: "onInit" },
    accent: "#0f766e",
  },
  {
    kind: "onKLineClosed",
    label: "K 线收盘",
    description: "每次 K 线收盘触发，是当前 Pine 策略的核心入口。",
    shape: "circle",
    text: "K 线收盘",
    properties: { blockKind: "onKLineClosed" },
    accent: "#1d4ed8",
  },
  {
    kind: "getTechnicalIndicator",
    label: "指标数据",
    description: "加载一个技术指标结果，供后续判断节点或动作节点复用。",
    shape: "rect",
    text: nextGetTechnicalIndicatorNodeText({
      blockKind: "getTechnicalIndicator",
      indicatorType: "rsi",
      period: 14,
    }),
    properties: {
      blockKind: "getTechnicalIndicator",
      indicatorType: "rsi",
      period: 14,
    },
    accent: "#0f766e",
    paletteVisible: false,
  },
  {
    kind: "technicalIndicatorCondition",
    label: "指标条件判断",
    description: "基于已定义变量里的指标结果做数值或形态判断，并提供 true / false 两个后续分支。",
    shape: "diamond",
    text: nextTechnicalIndicatorConditionNodeText({
      blockKind: "technicalIndicatorCondition",
      indicatorType: "rsi",
      conditionMode: "numeric",
      operator: "<",
      threshold: 30,
    }),
    properties: {
      blockKind: "technicalIndicatorCondition",
      indicatorType: "rsi",
      conditionMode: "numeric",
      operator: "<",
      threshold: 30,
    },
    accent: "#ca8a04",
    paletteVisible: true,
  },
  {
    kind: "seriesCondition",
    label: "序列条件判断",
    description: "基于价格、成交量或派生序列做比较、rising/falling、barssince/valuewhen 判断。",
    shape: "diamond",
    text: "Close > 阈值",
    properties: {
      blockKind: "seriesCondition",
      mode: "compare",
      source: "close",
      operator: ">",
      threshold: 520,
      length: 3,
      eventSource: "close",
      eventOperator: ">",
      eventThreshold: 520,
      valueSource: "close",
      occurrence: 0,
    },
    accent: "#7c3aed",
    paletteVisible: true,
  },
  {
    kind: "pineSnippet",
    label: "Pine 片段",
    description: "保留当前不能稳定映射成标准图块的 Pine v6 语句；保存时会原样写回 Pine。",
    shape: "rect",
    text: "Pine 片段",
    properties: {
      blockKind: "pineSnippet",
      code: "log.info(\"保留 Pine 片段\")",
      snippetSource: "manualSnippet",
    },
    accent: "#475569",
    paletteVisible: false,
  },
  {
    kind: "strategyInput",
    label: "策略参数",
    description: "声明 Pine input 默认参数，供指标、条件、MTF 或自定义表达式引用。",
    shape: "rect",
    text: "参数 length = 20",
    properties: {
      blockKind: "strategyInput",
      variableName: "length",
      inputType: "int",
      title: "Length",
      defaultValue: 20,
    },
    accent: "#2563eb",
  },
  {
    kind: "derivedSeries",
    label: "派生序列",
    description: "生成 history、nz、math、四则表达式或 cross 系列派生变量。",
    shape: "rect",
    text: "派生序列 signal",
    properties: {
      blockKind: "derivedSeries",
      variableName: "signal",
      mode: "history",
      source: "close",
      historyOffset: 1,
      fallbackValue: 0,
      mathFunction: "max",
      leftExpression: "close",
      operator: "-",
      rightExpression: "open",
      crossFunction: "crossover",
    },
    accent: "#0891b2",
  },
  {
    kind: "mtfSeries",
    label: "高周期序列",
    description: "生成同标的静态 timeframe request.security 一阶序列。",
    shape: "rect",
    text: "MTF 日线 close",
    properties: {
      blockKind: "mtfSeries",
      variableName: "mtf_close",
      timeframe: "D",
      expressionType: "source",
      source: "close",
      historyOffset: 1,
      indicatorExpression: "ta.ema(close, 20)",
    },
    accent: "#4f46e5",
  },
  {
    kind: "stateVariable",
    label: "持久状态",
    description: "声明 var 标量状态，支持 number/bool/string 默认值。",
    shape: "rect",
    text: "状态 armed = false",
    properties: {
      blockKind: "stateVariable",
      variableName: "armed",
      valueType: "bool",
      initialValue: false,
    },
    accent: "#64748b",
  },
  {
    kind: "stateUpdate",
    label: "更新状态",
    description: "对已声明的标量状态执行 := 更新。",
    shape: "rect",
    text: "更新状态 armed",
    properties: {
      blockKind: "stateUpdate",
      variableName: "armed",
      expression: "close > open",
    },
    accent: "#64748b",
  },
  {
    kind: "collectionStat",
    label: "集合统计",
    description: "对固定 source 列表执行 array.from(...).min/max/avg/sum/median/stdev/variance/percentile 只读统计。",
    shape: "rect",
    text: "集合统计 range_median",
    properties: {
      blockKind: "collectionStat",
      variableName: "range_median",
      statFunction: "median",
      sourceA: "close",
      sourceB: "open",
      sourceC: "high",
      percentile: 50,
    },
    accent: "#0369a1",
  },
  {
    kind: "timeFilter",
    label: "时间过滤",
    description: "基于闭盘 K 线 hour/minute/dayofweek 生成安全时间条件。",
    shape: "diamond",
    text: "时间过滤",
    properties: {
      blockKind: "timeFilter",
      mode: "between",
      startHour: 9,
      startMinute: 30,
      endHour: 16,
      endMinute: 0,
      dayOfWeek: 2,
    },
    accent: "#0e7490",
  },
  {
    kind: "sessionFilter",
    label: "交易时段过滤",
    description: "基于 closed-bar runtime 的 session.ismarket / ispremarket / ispostmarket 状态过滤。",
    shape: "diamond",
    text: "交易时段过滤",
    properties: {
      blockKind: "sessionFilter",
      scope: "market",
    },
    accent: "#0e7490",
  },
  {
    kind: "ifCloseAbove",
    label: "收盘价高于",
    description: "当 ctx.kline.close 高于阈值时执行后续动作。",
    shape: "diamond",
    text: "收盘价 > 阈值",
    properties: {
      blockKind: "ifCloseAbove",
      threshold: 520,
    },
    accent: "#d97706",
  },
  {
    kind: "ifCloseBelow",
    label: "收盘价低于",
    description: "当 ctx.kline.close 低于阈值时执行后续动作。",
    shape: "diamond",
    text: "收盘价 < 阈值",
    properties: {
      blockKind: "ifCloseBelow",
      threshold: 480,
    },
    accent: "#b45309",
  },
  {
    kind: "log",
    label: "输出日志",
    description: "调用 console.log，把上下文或信号写入运行日志。",
    shape: "rect",
    text: "输出日志",
    properties: {
      blockKind: "log",
      message: "观察到新的策略事件",
    },
    accent: "#475569",
  },
  {
    kind: "notify",
    label: "发送通知",
    description: "调用 notify，把策略信号发到控制台通知流。",
    shape: "rect",
    text: "发送通知",
    properties: {
      blockKind: "notify",
      message: "策略条件命中，准备处理后续动作",
    },
    accent: "#be123c",
  },
  {
    kind: "placeOrder",
    label: "下单",
    description: "向券商提交买入或卖出订单，支持固定股数、固定金额和账户权益百分比三种 Pine 对齐数量模式。",
    shape: "rect",
    text: "下单",
    properties: {
      blockKind: "placeOrder",
      orderAction: "entry",
      orderId: "Long",
      side: "BUY",
      orderType: "MARKET",
      entryPositionPolicy: "sameDirection",
      quantityMode: "shares",
      quantityValue: 100,
      limitPrice: 0,
      stopPrice: 0,
      riskAllowedDirection: "all",
    },
    accent: "#0f766e",
  },
  {
    kind: "stopLoss",
    label: "止损",
    description: "基于 Go 预计算的风险快照，对当前多空仓位执行止损、止盈或追踪止损平仓。",
    shape: "rect",
    text: "自动止损 1柱 2%",
    properties: {
      blockKind: "stopLoss",
      mode: "stopLoss",
      direction: "auto",
      timeValue: 1,
      timeUnit: "bar",
      percentage: 2,
      windowPolicy: "continuous",
    },
    accent: "#b91c1c",
  },
];

export function normalizeStopLossMode(value: unknown): StopLossMode {
  return STOP_LOSS_MODE_OPTIONS.some((option) => option.value === value)
    ? (value as StopLossMode)
    : "stopLoss";
}

export function normalizeSeriesSource(
  value: unknown,
  fallback: StrategySeriesSource = "close",
): StrategySeriesSource {
  return SERIES_SOURCE_OPTIONS.some((option) => option.value === value)
    ? (value as StrategySeriesSource)
    : fallback;
}

export function normalizeSeriesConditionMode(value: unknown): SeriesConditionMode {
  return SERIES_CONDITION_MODE_OPTIONS.some((option) => option.value === value)
    ? (value as SeriesConditionMode)
    : "compare";
}

export function normalizeSeriesConditionOperator(value: unknown): SeriesConditionOperator {
  return value === "<" ? "<" : ">";
}

export function normalizeSeriesConditionBlockProperties(
  properties: Record<string, unknown>,
): SeriesConditionBlockProperties {
  return {
    blockKind: "seriesCondition",
    mode: normalizeSeriesConditionMode(properties.mode),
    source: normalizeSeriesSource(properties.source),
    operator: normalizeSeriesConditionOperator(properties.operator),
    threshold: normalizeStopLossDecimal(properties.threshold, 0),
    length: normalizeStopLossInteger(properties.length, 3),
    eventSource: normalizeSeriesSource(properties.eventSource, "close"),
    eventOperator: normalizeSeriesConditionOperator(properties.eventOperator),
    eventThreshold: normalizeStopLossDecimal(properties.eventThreshold, 520),
    valueSource: normalizeSeriesSource(properties.valueSource, "close"),
    occurrence: normalizeNonNegativeInteger(properties.occurrence, 0),
    sourceExpressionAst: normalizeVisualExpression(
      properties.sourceExpressionAst,
      sourceExpression(normalizeSeriesSource(properties.source)),
    ),
    leftExpressionAst: normalizeVisualExpression(
      properties.leftExpressionAst,
      sourceExpression(normalizeSeriesSource(properties.source)),
    ),
    rightExpressionAst: normalizeVisualExpression(
      properties.rightExpressionAst,
      literalExpression(normalizeStopLossDecimal(properties.threshold, 0)),
    ),
    eventExpressionAst: normalizeVisualExpression(
      properties.eventExpressionAst,
      {
        kind: "binary",
        left: sourceExpression(normalizeSeriesSource(properties.eventSource, "close")),
        operator: normalizeSeriesConditionOperator(properties.eventOperator),
        right: literalExpression(normalizeStopLossDecimal(properties.eventThreshold, 520)),
      },
    ),
    valueExpressionAst: normalizeVisualExpression(
      properties.valueExpressionAst,
      sourceExpression(normalizeSeriesSource(properties.valueSource, "close")),
    ),
  };
}

export function normalizeStrategyInputBlockProperties(
  properties: Record<string, unknown>,
): StrategyInputBlockProperties {
  const inputType = STRATEGY_INPUT_TYPE_OPTIONS.some((option) => option.value === properties.inputType)
    ? (properties.inputType as StrategyInputType)
    : "int";
  return {
    blockKind: "strategyInput",
    variableName: normalizePineName(properties.variableName, "length"),
    inputType,
    title: typeof properties.title === "string" && properties.title.trim() !== ""
      ? properties.title.trim()
      : "Length",
    defaultValue: normalizeInputDefaultValue(inputType, properties.defaultValue),
  };
}

export function normalizeDerivedSeriesBlockProperties(
  properties: Record<string, unknown>,
): DerivedSeriesBlockProperties {
  const mode = isOneOf(properties.mode, ["history", "nz", "math", "arithmetic", "cross"])
    ? properties.mode
    : "history";
  return {
    blockKind: "derivedSeries",
    variableName: normalizePineName(properties.variableName, "signal"),
    mode,
    source: normalizeSeriesSource(properties.source),
    historyOffset: normalizeNonNegativeInteger(properties.historyOffset, 1),
    fallbackValue: normalizeStopLossDecimal(properties.fallbackValue, 0),
    mathFunction: isOneOf(properties.mathFunction, ["min", "max", "abs", "round", "floor", "ceil"])
      ? properties.mathFunction
      : "max",
    leftExpression: normalizeSafeExpression(properties.leftExpression, "close"),
    leftExpressionAst: normalizeVisualExpression(
      properties.leftExpressionAst,
      parsePineExpressionToVisualExpression(normalizeSafeExpression(properties.leftExpression, "close")) ?? sourceExpression("close"),
    ),
    operator: isOneOf(properties.operator, ["+", "-", "*", "/"]) ? properties.operator : "-",
    rightExpression: normalizeSafeExpression(properties.rightExpression, "open"),
    rightExpressionAst: normalizeVisualExpression(
      properties.rightExpressionAst,
      parsePineExpressionToVisualExpression(normalizeSafeExpression(properties.rightExpression, "open")) ?? sourceExpression("open"),
    ),
    crossFunction: isOneOf(properties.crossFunction, ["crossover", "crossunder", "cross"])
      ? properties.crossFunction
      : "crossover",
    sourceExpressionAst: normalizeVisualExpression(
      properties.sourceExpressionAst,
      sourceExpression(normalizeSeriesSource(properties.source)),
    ),
    fallbackExpressionAst: normalizeVisualExpression(
      properties.fallbackExpressionAst,
      literalExpression(normalizeStopLossDecimal(properties.fallbackValue, 0)),
    ),
  };
}

export function normalizeMtfSeriesBlockProperties(
  properties: Record<string, unknown>,
): MtfSeriesBlockProperties {
  const mtfField = normalizePineField(properties.mtfField);
  const indicatorExpression = normalizeSafeExpression(properties.indicatorExpression, "ta.ema(close, 20)");
  const parsedIndicatorExpression = parsePineExpressionToVisualExpression(indicatorExpression);
  const indicatorExpressionAst = properties.indicatorExpressionAst === undefined
    ? parsedIndicatorExpression
    : normalizeVisualExpression(properties.indicatorExpressionAst, parsedIndicatorExpression ?? sourceExpression("close"));
  return {
    blockKind: "mtfSeries",
    variableName: normalizePineName(properties.variableName, "mtf_close"),
    timeframe: normalizeTimeframe(properties.timeframe),
    expressionType: isOneOf(properties.expressionType, ["source", "history", "indicator"])
      ? properties.expressionType
      : "source",
    source: normalizeSeriesSource(properties.source),
    historyOffset: normalizeNonNegativeInteger(properties.historyOffset, 1),
    indicatorExpression,
    ...(indicatorExpressionAst === null ? {} : { indicatorExpressionAst }),
    ...(mtfField === undefined ? {} : { mtfField }),
  };
}

export function normalizeStateVariableBlockProperties(
  properties: Record<string, unknown>,
): StateVariableBlockProperties {
  const valueType = isOneOf(properties.valueType, ["number", "bool", "string"])
    ? properties.valueType
    : "bool";
  return {
    blockKind: "stateVariable",
    variableName: normalizePineName(properties.variableName, "armed"),
    valueType,
    initialValue: normalizeStateInitialValue(valueType, properties.initialValue),
  };
}

export function normalizeStateUpdateBlockProperties(
  properties: Record<string, unknown>,
): StateUpdateBlockProperties {
  const expression = normalizeSafeExpression(properties.expression, "close > open");
  const parsedExpression = parsePineExpressionToVisualExpression(expression);
  const expressionAst = properties.expressionAst === undefined
    ? parsedExpression
    : normalizeVisualExpression(properties.expressionAst, parsedExpression ?? sourceExpression("close"));
  return {
    blockKind: "stateUpdate",
    variableName: normalizePineName(properties.variableName, "armed"),
    expression,
    ...(expressionAst === null ? {} : { expressionAst }),
  };
}

export function normalizeCollectionStatBlockProperties(
  properties: Record<string, unknown>,
): CollectionStatBlockProperties {
  const sourceA = normalizeSeriesSource(properties.sourceA, "close");
  const sourceB = normalizeSeriesSource(properties.sourceB, "open");
  const sourceC = normalizeSeriesSource(properties.sourceC, "high");
  return {
    blockKind: "collectionStat",
    variableName: normalizePineName(properties.variableName, "range_median"),
    statFunction: isOneOf(properties.statFunction, ["min", "max", "avg", "sum", "median", "stdev", "variance", "percentile"])
      ? properties.statFunction
      : "median",
    sourceA,
    sourceB,
    sourceC,
    sourceAExpressionAst: normalizeVisualExpression(properties.sourceAExpressionAst, sourceExpression(sourceA)),
    sourceBExpressionAst: normalizeVisualExpression(properties.sourceBExpressionAst, sourceExpression(sourceB)),
    sourceCExpressionAst: normalizeVisualExpression(properties.sourceCExpressionAst, sourceExpression(sourceC)),
    percentile: Math.min(100, Math.max(0, normalizeStopLossDecimal(properties.percentile, 50))),
  };
}

export function normalizeTimeFilterBlockProperties(
  properties: Record<string, unknown>,
): TimeFilterBlockProperties {
  return {
    blockKind: "timeFilter",
    mode: isOneOf(properties.mode, ["after", "before", "between", "dayOfWeek"]) ? properties.mode : "between",
    startHour: normalizeClockHour(properties.startHour, 9),
    startMinute: normalizeClockMinute(properties.startMinute, 30),
    endHour: normalizeClockHour(properties.endHour, 16),
    endMinute: normalizeClockMinute(properties.endMinute, 0),
    dayOfWeek: normalizeDayOfWeek(properties.dayOfWeek, 2),
  };
}

export function normalizeSessionFilterBlockProperties(
  properties: Record<string, unknown>,
): SessionFilterBlockProperties {
  return {
    blockKind: "sessionFilter",
    scope: isOneOf(properties.scope, ["market", "premarket", "postmarket"]) ? properties.scope : "market",
  };
}

export function nextStrategyInputNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeStrategyInputBlockProperties(rawProperties);
  return `参数 ${properties.variableName} = ${properties.defaultValue}`;
}

export function nextDerivedSeriesNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeDerivedSeriesBlockProperties(rawProperties);
  return `派生 ${properties.variableName} · ${properties.mode}`;
}

export function nextMtfSeriesNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeMtfSeriesBlockProperties(rawProperties);
  return `MTF ${properties.variableName} · ${properties.timeframe}`;
}

export function nextStateVariableNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeStateVariableBlockProperties(rawProperties);
  return `状态 ${properties.variableName} = ${properties.initialValue}`;
}

export function nextStateUpdateNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeStateUpdateBlockProperties(rawProperties);
  return `更新状态 ${properties.variableName}`;
}

export function nextCollectionStatNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeCollectionStatBlockProperties(rawProperties);
  return `集合统计 ${properties.variableName} · ${properties.statFunction}`;
}

export function nextTimeFilterNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeTimeFilterBlockProperties(rawProperties);
  if (properties.mode === "dayOfWeek") {
    return `星期过滤 · ${dayOfWeekLabel(properties.dayOfWeek ?? 2)}`;
  }
  return `时间过滤 · ${formatClock(properties.startHour ?? 9, properties.startMinute ?? 30)}-${formatClock(properties.endHour ?? 16, properties.endMinute ?? 0)}`;
}

export function nextSessionFilterNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeSessionFilterBlockProperties(rawProperties);
  return `时段过滤 · ${sessionScopeLabel(properties.scope ?? "market")}`;
}

export function nextSeriesConditionNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeSeriesConditionBlockProperties(rawProperties);
  const source = seriesSourceLabel(properties.source ?? "close");
  const event = `${seriesSourceLabel(properties.eventSource ?? "close")} ${properties.eventOperator ?? ">"} ${properties.eventThreshold ?? 520}`;
  switch (properties.mode) {
    case "rising":
      return `${source} 连续上升 ${properties.length ?? 3}`;
    case "falling":
      return `${source} 连续下降 ${properties.length ?? 3}`;
    case "barssince":
      return `距 ${event} < ${properties.length ?? 3}`;
    case "valuewhen":
      return `${seriesSourceLabel(properties.valueSource ?? "close")}@${event} ${properties.operator ?? ">"} ${properties.threshold ?? 0}`;
    case "compare":
    default:
      return `${source} ${properties.operator ?? ">"} ${properties.threshold ?? 0}`;
  }
}

export function seriesSourceLabel(source: StrategySeriesSource): string {
  return SERIES_SOURCE_OPTIONS.find((option) => option.value === source)?.label ?? "Close";
}

export function normalizeStopLossDirection(value: unknown): StopLossDirection {
  return value === "long" || value === "short" ? value : "auto";
}

export function normalizeStopLossTimeUnit(value: unknown): StopLossTimeUnit {
  return STOP_LOSS_TIME_UNIT_OPTIONS.some((option) => option.value === value)
    ? (value as StopLossTimeUnit)
    : "bar";
}

export function normalizeStopLossWindowPolicy(value: unknown): StopLossWindowPolicy {
  return STOP_LOSS_WINDOW_POLICY_OPTIONS.some((option) => option.value === value)
    ? (value as StopLossWindowPolicy)
    : "continuous";
}

export function normalizeStopLossBlockProperties(
  properties: Record<string, unknown>,
): StopLossBlockProperties {
  return {
    blockKind: "stopLoss",
    mode: normalizeStopLossMode(properties.mode),
    direction: normalizeStopLossDirection(properties.direction),
    timeValue: normalizeStopLossInteger(properties.timeValue, 1),
    timeUnit: normalizeStopLossTimeUnit(properties.timeUnit),
    percentage: normalizeStopLossDecimal(properties.percentage, 2),
    takeProfitPercentage: normalizeStopLossDecimal(properties.takeProfitPercentage, 4),
    quantityPercentage: normalizeStopLossDecimal(properties.quantityPercentage, 100),
    windowPolicy: normalizeStopLossWindowPolicy(properties.windowPolicy),
    ...(properties.stopPriceExpressionAst === undefined ? {} : {
      stopPriceExpressionAst: normalizeVisualExpression(properties.stopPriceExpressionAst, sourceExpression("close")),
    }),
    ...(properties.takeProfitPriceExpressionAst === undefined ? {} : {
      takeProfitPriceExpressionAst: normalizeVisualExpression(properties.takeProfitPriceExpressionAst, sourceExpression("close")),
    }),
    ...(properties.trailingPriceExpressionAst === undefined ? {} : {
      trailingPriceExpressionAst: normalizeVisualExpression(properties.trailingPriceExpressionAst, sourceExpression("close")),
    }),
  };
}

export function stopLossModeLabel(mode: StopLossMode): string {
  switch (mode) {
    case "takeProfit":
      return "止盈";
    case "trailingStop":
      return "追踪止损";
    case "bracketExit":
      return "止盈止损";
    case "stopLoss":
    default:
      return "止损";
  }
}

export function stopLossDirectionLabel(direction: StopLossDirection): string {
  switch (direction) {
    case "long":
      return "多头";
    case "short":
      return "空头";
    default:
      return "自动";
  }
}

export function stopLossTimeUnitLabel(unit: StopLossTimeUnit): string {
  switch (unit) {
    case "bar":
      return "柱";
    case "minute":
      return "分钟";
    case "hour":
      return "小时";
    case "week":
      return "周";
    case "month":
      return "月";
    case "day":
    default:
      return "日";
  }
}

export function stopLossWindowPolicyLabel(policy: StopLossWindowPolicy): string {
  return policy === "session" ? "交易时段感知" : "连续窗口";
}

export function stopLossRuleLabel(properties: StopLossBlockProperties): string {
  switch (properties.mode ?? "stopLoss") {
    case "takeProfit":
      return `顺向波动 >= ${properties.percentage ?? 2}%`;
    case "trailingStop":
      return `回撤 / 反弹 >= ${properties.percentage ?? 2}%`;
    case "bracketExit":
      return `反向 >= ${properties.percentage ?? 2}% 或顺向 >= ${properties.takeProfitPercentage ?? 4}%`;
    case "stopLoss":
    default:
      return `反向波动 >= ${properties.percentage ?? 2}%`;
  }
}

export function dayOfWeekLabel(value: number): string {
  switch (normalizeDayOfWeek(value, 2)) {
    case 1:
      return "周日";
    case 2:
      return "周一";
    case 3:
      return "周二";
    case 4:
      return "周三";
    case 5:
      return "周四";
    case 6:
      return "周五";
    case 7:
    default:
      return "周六";
  }
}

export function sessionScopeLabel(value: TradingSessionScope): string {
  switch (value) {
    case "premarket":
      return "盘前";
    case "postmarket":
      return "盘后";
    case "market":
    default:
      return "常规交易时段";
  }
}

export function nextStopLossNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeStopLossBlockProperties(rawProperties);
  return `${stopLossDirectionLabel(properties.direction ?? "auto")}${stopLossModeLabel(properties.mode ?? "stopLoss")} ${properties.timeValue ?? 1}${stopLossTimeUnitLabel(properties.timeUnit ?? "bar")} ${properties.percentage ?? 2}%${properties.windowPolicy === "session" ? " 时段感知" : ""}`;
}

export function getStrategyBlockCatalog(): StrategyBlockDefinition[] {
  return STRATEGY_BLOCK_CATALOG.map((block) => ({
    ...block,
    properties: { ...block.properties },
  }));
}

export function getStrategyBlockDefinition(
  kind: string | null | undefined,
): StrategyBlockDefinition | null {
  return STRATEGY_BLOCK_CATALOG.find((block) => block.kind === kind) ?? null;
}

export function getStrategyBlockKind(
  node: StrategyVisualNodeDocument | null | undefined,
): StrategyBlockKind | null {
  const value = node?.properties.blockKind;
  return typeof value === "string" ? (value as StrategyBlockKind) : null;
}

export function createStrategyPaletteItems(): Array<{
  type: string;
  text: string;
  label: string;
  icon: string;
  properties: Record<string, unknown>;
}> {
  return STRATEGY_BLOCK_CATALOG
    .filter((block) => block.paletteVisible !== false)
    .map((block) => ({
      type: block.shape,
      text: block.text,
      label: block.label,
      icon: buildPaletteIcon(block.accent, block.label.slice(0, 2)),
      properties: {
        ...block.properties,
      },
    }));
}

function buildPaletteIcon(fill: string, text: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="72" height="72" viewBox="0 0 72 72"><rect width="72" height="72" rx="18" fill="${fill}"/><text x="36" y="41" text-anchor="middle" font-size="22" font-family="Georgia, serif" fill="white">${escapeXml(text)}</text></svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

function escapeXml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

function normalizeStopLossInteger(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.max(1, Math.round(value));
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return Math.max(1, Math.round(parsed));
    }
  }
  return fallback;
}

function normalizeNonNegativeInteger(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.max(0, Math.round(value));
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return Math.max(0, Math.round(parsed));
    }
  }
  return fallback;
}

function normalizeClockHour(value: unknown, fallback: number): number {
  return Math.min(23, Math.max(0, normalizeIntegerValue(value, fallback)));
}

function normalizeClockMinute(value: unknown, fallback: number): number {
  return Math.min(59, Math.max(0, normalizeIntegerValue(value, fallback)));
}

function normalizeDayOfWeek(value: unknown, fallback: number): number {
  return Math.min(7, Math.max(1, normalizeIntegerValue(value, fallback)));
}

function formatClock(hour: number, minute: number): string {
  return `${String(normalizeClockHour(hour, 0)).padStart(2, "0")}:${String(normalizeClockMinute(minute, 0)).padStart(2, "0")}`;
}

function normalizeIntegerValue(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.round(value);
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return Math.round(parsed);
    }
  }
  return fallback;
}

function normalizeStopLossDecimal(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return fallback;
}

function normalizePineName(value: unknown, fallback: string): string {
  const raw = typeof value === "string" ? value.trim() : "";
  const normalized = raw
    .replace(/[^A-Za-z0-9_]+/g, "_")
    .replace(/^([0-9])/, "_$1")
    .replace(/^_+|_+$/g, "");
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(normalized) ? normalized : fallback;
}

function normalizePineField(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(trimmed) ? trimmed : undefined;
}

function normalizeSafeExpression(value: unknown, fallback: string): string {
  const raw = typeof value === "string" ? value.trim() : "";
  if (raw === "" || /(?:array\.|map\.|matrix\.|request\.security|strategy\.|line\.|label\.|table\.|:=|\bfor\b|\bwhile\b)/i.test(raw)) {
    return fallback;
  }
  return raw.replace(/[\r\n]+/g, " ");
}

function normalizeTimeframe(value: unknown): string {
  const raw = typeof value === "string" ? value.trim().toUpperCase() : "";
  return ["1", "5", "15", "30", "45", "60", "120", "240", "D", "W", "M"].includes(raw)
    ? raw
    : "D";
}

function normalizeInputDefaultValue(
  inputType: StrategyInputType,
  value: unknown,
): number | string {
  switch (inputType) {
    case "float":
      return normalizeStopLossDecimal(value, 2);
    case "source":
      return normalizeSeriesSource(value);
    case "timeframe":
      return normalizeTimeframe(value);
    case "time":
      return typeof value === "string" && value.trim() !== "" ? value.trim() : "timestamp(2026, 1, 1)";
    case "color":
      return typeof value === "string" && value.trim() !== "" ? value.trim() : "color.green";
    case "int":
    default:
      return normalizeStopLossInteger(value, 20);
  }
}

function normalizeStateInitialValue(
  valueType: StateValueType,
  value: unknown,
): number | boolean | string {
  switch (valueType) {
    case "number":
      return normalizeStopLossDecimal(value, 0);
    case "string":
      return typeof value === "string" ? value : "";
    case "bool":
    default:
      return value === true || value === "true";
  }
}

function isOneOf<const T extends string>(
  value: unknown,
  options: readonly T[],
): value is T {
  return typeof value === "string" && (options as readonly string[]).includes(value);
}
