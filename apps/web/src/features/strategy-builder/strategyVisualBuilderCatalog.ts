import type { StrategyVisualNodeDocument } from "@/types";

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
import { STRATEGY_BLOCK_CATALOG } from "./strategyVisualBuilderCatalogData";
import { dayOfWeekLabel, sessionScopeLabel } from "./strategyVisualBuilderCatalogLabels";
import {
  formatClock,
  isOneOf,
  normalizeClockHour,
  normalizeClockMinute,
  normalizeDayOfWeek,
  normalizeInputDefaultValue,
  normalizeIntegerValue,
  normalizeNonNegativeInteger,
  normalizePineField,
  normalizePineName,
  normalizeSafeExpression,
  normalizeStateInitialValue,
  normalizeStopLossDecimal,
  normalizeStopLossInteger,
  normalizeTimeframe,
} from "./strategyVisualBuilderCatalogNormalization";

export type StrategyBlockKind =
  | "onInit"
  | "onKLineClosed"
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
  | "riskRule"
  | "stopLoss";

export type {
  TechnicalIndicatorConditionMode,
  TechnicalIndicatorOperator,
  TechnicalIndicatorPatternType,
  TechnicalIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";
export {
  nextRiskRuleNodeText,
  normalizeRiskAmountType,
  normalizeRiskRuleBlockProperties,
  normalizeRiskRuleDirection,
  normalizeRiskRuleType,
  RISK_AMOUNT_TYPE_OPTIONS,
  RISK_RULE_TYPE_OPTIONS,
  riskAmountTypeLabel,
  riskRuleTypeLabel,
} from "./strategyVisualBuilderRiskBlock";
export type {
  RiskAmountType,
  RiskRuleBlockProperties,
  RiskRuleType,
} from "./strategyVisualBuilderRiskBlock";

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
  mode: SeriesConditionMode;
  source: StrategySeriesSource;
  operator: SeriesConditionOperator;
  threshold: number;
  length: number;
  eventSource: StrategySeriesSource;
  eventOperator: SeriesConditionOperator;
  eventThreshold: number;
  valueSource: StrategySeriesSource;
  occurrence: number;
  sourceExpressionAst: VisualExpression;
  leftExpressionAst: VisualExpression;
  rightExpressionAst: VisualExpression;
  eventExpressionAst: VisualExpression;
  valueExpressionAst: VisualExpression;
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
  variableName: string;
  inputType: StrategyInputType;
  title: string;
  defaultValue: number | string;
}

export interface DerivedSeriesBlockProperties {
  blockKind: "derivedSeries";
  variableName: string;
  mode: DerivedSeriesMode;
  source: StrategySeriesSource;
  historyOffset: number;
  fallbackValue: number;
  mathFunction: DerivedSeriesMathFunction;
  leftExpression: string;
  leftExpressionAst: VisualExpression;
  operator: "+" | "-" | "*" | "/";
  rightExpression: string;
  rightExpressionAst: VisualExpression;
  crossFunction: DerivedSeriesCrossFunction;
  sourceExpressionAst: VisualExpression;
  fallbackExpressionAst: VisualExpression;
}

export interface MtfSeriesBlockProperties {
  blockKind: "mtfSeries";
  variableName: string;
  timeframe: string;
  expressionType: MtfSeriesExpressionType;
  source: StrategySeriesSource;
  historyOffset: number;
  indicatorExpression: string;
  indicatorExpressionAst?: VisualExpression;
  mtfField?: string;
}

export interface StateVariableBlockProperties {
  blockKind: "stateVariable";
  variableName: string;
  valueType: StateValueType;
  initialValue: number | boolean | string;
}

export interface StateUpdateBlockProperties {
  blockKind: "stateUpdate";
  variableName: string;
  expression: string;
  expressionAst?: VisualExpression;
}

export interface CollectionStatBlockProperties {
  blockKind: "collectionStat";
  variableName: string;
  statFunction: CollectionStatFunction;
  sourceA: StrategySeriesSource;
  sourceB: StrategySeriesSource;
  sourceC: StrategySeriesSource;
  sourceAExpressionAst: VisualExpression;
  sourceBExpressionAst: VisualExpression;
  sourceCExpressionAst: VisualExpression;
  percentile: number;
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
export type StopLossTrailingPriceMode = "points" | "price";
export type StopLossFromEntryMode = "explicit" | "auto";

export interface StopLossBlockProperties {
  blockKind: "stopLoss";
  exitId?: string;
  fromEntryId?: string;
  mode?: StopLossMode;
  direction?: StopLossDirection;
  timeValue?: number;
  timeUnit?: StopLossTimeUnit;
  percentage?: number;
  takeProfitPercentage?: number;
  profitTicks?: number;
  lossTicks?: number;
  quantityPercentage?: number;
  windowPolicy?: StopLossWindowPolicy;
  stopPriceExpressionAst?: VisualExpression;
  takeProfitPriceExpressionAst?: VisualExpression;
  trailingPriceExpressionAst?: VisualExpression;
  trailingOffsetExpressionAst?: VisualExpression;
  trailingPriceMode?: StopLossTrailingPriceMode;
  fromEntryMode?: StopLossFromEntryMode;
  comment?: string;
  comment_profit?: string;
  comment_loss?: string;
  comment_trailing?: string;
  alert_message?: string;
  alert_profit?: string;
  alert_loss?: string;
  alert_trailing?: string;
  disable_alert?: boolean;
  when?: string;
}

export interface TimeFilterBlockProperties {
  blockKind: "timeFilter";
  mode: TimeFilterMode;
  startHour: number;
  startMinute: number;
  endHour: number;
  endMinute: number;
  dayOfWeek: number;
}

export interface SessionFilterBlockProperties {
  blockKind: "sessionFilter";
  scope: TradingSessionScope;
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
    ...(typeof properties.exitId === "string" && properties.exitId.trim() !== ""
      ? { exitId: properties.exitId.trim() }
      : {}),
    ...(typeof properties.fromEntryId === "string" && properties.fromEntryId.trim() !== ""
      ? { fromEntryId: properties.fromEntryId.trim() }
      : {}),
    mode: normalizeStopLossMode(properties.mode),
    direction: normalizeStopLossDirection(properties.direction),
    timeValue: normalizeStopLossInteger(properties.timeValue, 1),
    timeUnit: normalizeStopLossTimeUnit(properties.timeUnit),
    percentage: normalizeStopLossDecimal(properties.percentage, 2),
    takeProfitPercentage: normalizeStopLossDecimal(properties.takeProfitPercentage, 4),
    ...(properties.profitTicks === undefined ? {} : { profitTicks: normalizeStopLossDecimal(properties.profitTicks, 50) }),
    ...(properties.lossTicks === undefined ? {} : { lossTicks: normalizeStopLossDecimal(properties.lossTicks, 25) }),
    quantityPercentage: normalizeStopLossDecimal(properties.quantityPercentage, 100),
    windowPolicy: normalizeStopLossWindowPolicy(properties.windowPolicy),
    trailingPriceMode: normalizeStopLossTrailingPriceMode(properties.trailingPriceMode),
    fromEntryMode: properties.fromEntryMode === "auto" ? "auto" : "explicit",
    ...(properties.stopPriceExpressionAst === undefined ? {} : {
      stopPriceExpressionAst: normalizeVisualExpression(properties.stopPriceExpressionAst, sourceExpression("close")),
    }),
    ...(properties.takeProfitPriceExpressionAst === undefined ? {} : {
      takeProfitPriceExpressionAst: normalizeVisualExpression(properties.takeProfitPriceExpressionAst, sourceExpression("close")),
    }),
    ...(properties.trailingPriceExpressionAst === undefined ? {} : {
      trailingPriceExpressionAst: normalizeVisualExpression(properties.trailingPriceExpressionAst, sourceExpression("close")),
    }),
    ...(properties.trailingOffsetExpressionAst === undefined ? {} : {
      trailingOffsetExpressionAst: normalizeVisualExpression(properties.trailingOffsetExpressionAst, sourceExpression("close")),
    }),
    ...(typeof properties.comment === "string" && properties.comment.trim() !== ""
      ? { comment: properties.comment.trim() }
      : {}),
    ...(typeof properties.comment_profit === "string" && properties.comment_profit.trim() !== ""
      ? { comment_profit: properties.comment_profit.trim() }
      : {}),
    ...(typeof properties.comment_loss === "string" && properties.comment_loss.trim() !== ""
      ? { comment_loss: properties.comment_loss.trim() }
      : {}),
    ...(typeof properties.comment_trailing === "string" && properties.comment_trailing.trim() !== ""
      ? { comment_trailing: properties.comment_trailing.trim() }
      : {}),
    ...(typeof properties.alert_message === "string" && properties.alert_message.trim() !== ""
      ? { alert_message: properties.alert_message.trim() }
      : {}),
    ...(typeof properties.alert_profit === "string" && properties.alert_profit.trim() !== ""
      ? { alert_profit: properties.alert_profit.trim() }
      : {}),
    ...(typeof properties.alert_loss === "string" && properties.alert_loss.trim() !== ""
      ? { alert_loss: properties.alert_loss.trim() }
      : {}),
    ...(typeof properties.alert_trailing === "string" && properties.alert_trailing.trim() !== ""
      ? { alert_trailing: properties.alert_trailing.trim() }
      : {}),
    ...(typeof properties.disable_alert === "boolean"
      ? { disable_alert: properties.disable_alert }
      : {}),
    ...(typeof properties.when === "string" && properties.when.trim() !== ""
      ? { when: properties.when.trim() }
      : {}),
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

export function normalizeStopLossTrailingPriceMode(value: unknown): StopLossTrailingPriceMode {
  return value === "price" ? "price" : "points";
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
export { createStrategyPaletteItems } from "./strategyVisualBuilderCatalogPalette";
export { dayOfWeekLabel, sessionScopeLabel } from "./strategyVisualBuilderCatalogLabels";
