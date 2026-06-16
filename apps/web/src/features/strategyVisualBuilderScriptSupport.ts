import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import type {
  StopLossDirection,
  StopLossMode,
  StopLossTimeUnit,
  StopLossWindowPolicy,
} from "./strategyVisualBuilderCatalog";
import type {
  IndicatorPeriodUnit,
  MovingAverageIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";

export interface StrategyScriptRuntimeFlags {
  usesMovingAverageRuntime: boolean;
  usesRSIRuntime: boolean;
  usesMACDRuntime: boolean;
  usesKDJRuntime: boolean;
  usesATRRuntime: boolean;
  usesCCIRuntime: boolean;
  usesWilliamsRRuntime: boolean;
  usesBollingerRuntime: boolean;
  usesSimpleMovingAverageHelper: boolean;
  usesSeriesStateRuntime: boolean;
  usesDivergenceRuntime: boolean;
}

export function buildHookPrelude(
  kind: StrategyBlockKind,
  flags: StrategyScriptRuntimeFlags,
): string[] {
  if (!flags.usesSeriesStateRuntime || kind !== "onKLineClosed") {
    return [];
  }

  return [
    `${indent(1)}const close = Number(ctx && ctx.kline ? ctx.kline.close : NaN);`,
    `${indent(1)}if (!Number.isFinite(close)) {`,
    `${indent(2)}console.log("skip candle because close is not a finite number");`,
    `${indent(2)}return;`,
    `${indent(1)}}`,
    "",
    ...(flags.usesMovingAverageRuntime
      ? [
          `${indent(1)}let fastAverageSnapshot = null;`,
          `${indent(1)}let slowAverageSnapshot = null;`,
          `${indent(1)}let fastAverage = null;`,
          `${indent(1)}let slowAverage = null;`,
        `${indent(1)}let prevFastAverage = null;`,
        `${indent(1)}let prevSlowAverage = null;`,
        ]
      : []),
    ...(flags.usesRSIRuntime ? [`${indent(1)}let latestRsi = null;`] : []),
    ...(flags.usesMACDRuntime
      ? [
          `${indent(1)}let latestMacd = null;`,
          `${indent(1)}let latestMacdDiff = null;`,
          `${indent(1)}let latestMacdSignal = null;`,
          `${indent(1)}let latestMacdHistogram = null;`,
        ]
      : []),
    ...(flags.usesKDJRuntime
      ? [
          `${indent(1)}let latestKdj = null;`,
          `${indent(1)}let latestKValue = null;`,
          `${indent(1)}let latestDValue = null;`,
          `${indent(1)}let latestJValue = null;`,
          `${indent(1)}let previousKValue = null;`,
          `${indent(1)}let previousDValue = null;`,
        ]
      : []),
    ...(flags.usesATRRuntime ? [`${indent(1)}let latestAtr = null;`] : []),
    ...(flags.usesCCIRuntime ? [`${indent(1)}let latestCci = null;`] : []),
    ...(flags.usesWilliamsRRuntime ? [`${indent(1)}let latestWilliamsR = null;`] : []),
    ...(flags.usesBollingerRuntime
      ? [
          `${indent(1)}let latestBollinger = null;`,
          `${indent(1)}let latestBollingerMiddle = null;`,
          `${indent(1)}let latestBollingerUpper = null;`,
          `${indent(1)}let latestBollingerLower = null;`,
        ]
      : []),
    ...(flags.usesDivergenceRuntime ? [`${indent(1)}let divergenceSignal = false;`] : []),
  ];
}

export function buildScriptRuntimeBlocks(
  flags: StrategyScriptRuntimeFlags,
): string[] {
  void flags;
  return [];
}

export function buildMovingAverageIndicatorKey(
  windowSize: number,
  movingAverageType: MovingAverageIndicatorType = "MA",
  periodUnit: IndicatorPeriodUnit = "bar",
): string {
  return periodUnit === "bar"
    ? `ma:${movingAverageType}:${windowSize}`
    : `ma:${movingAverageType}:${windowSize}:${periodUnit}`;
}

export function buildStopLossIndicatorKey(
  direction: StopLossDirection,
  timeValue: number,
  timeUnit: StopLossTimeUnit,
  percentage: number,
  mode: StopLossMode = "stopLoss",
  windowPolicy: StopLossWindowPolicy = "continuous",
): string {
  if (mode === "stopLoss" && windowPolicy === "continuous") {
    return `sl:${direction}:${timeValue}:${timeUnit}:${percentage}`;
  }
  return `risk:${mode}:${direction}:${timeValue}:${timeUnit}:${percentage}:${windowPolicy}`;
}

export function buildRsiIndicatorKey(period: number): string {
  return `rsi:${period}`;
}

export function buildMacdIndicatorKey(
  fastPeriod: number,
  slowPeriod: number,
  signalPeriod: number,
): string {
  return `macd:${fastPeriod}:${slowPeriod}:${signalPeriod}`;
}

export function buildBollingerIndicatorKey(
  period: number,
  multiplier: number,
): string {
  return `bollinger:${period}:${multiplier}`;
}

export function buildKdjIndicatorKey(
  period: number,
  m1: number,
  m2: number,
): string {
  return `kdj:${period}:${m1}:${m2}`;
}

export function buildAtrIndicatorKey(period: number): string {
  return `atr:${period}`;
}

export function buildCciIndicatorKey(period: number): string {
  return `cci:${period}`;
}

export function buildWilliamsRIndicatorKey(period: number): string {
  return `williamsr:${period}`;
}

export function buildDivergenceIndicatorKey(
  indicatorType: "rsi" | "macd" | "kdj",
  params: number[],
  direction: "top" | "bottom",
  lookback: number,
): string {
  return ["divergence", indicatorType, ...params.map(String), direction, String(lookback)].join(":");
}

export function normalizeMessage(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() !== ""
    ? value.trim()
    : fallback;
}

export function normalizeThreshold(value: unknown, fallback: number): number {
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

export function normalizeWindowSize(value: unknown, fallback: number): number {
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

export function normalizeDecimal(value: unknown, fallback: number): number {
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

export type VisualOrderSide = "BUY" | "SELL" | "SELL_SHORT" | "BUY_COVER";

export function normalizeOrderSide(value: unknown): VisualOrderSide {
  if (
    value === "BUY" ||
    value === "SELL" ||
    value === "SELL_SHORT" ||
    value === "BUY_COVER"
  ) {
    return value;
  }
  return "BUY";
}

/** Convert visual side to the actual order side sent to the exchange. */
export function orderSideForExchange(visualSide: VisualOrderSide): "BUY" | "SELL" {
  return visualSide === "SELL" || visualSide === "SELL_SHORT" ? "SELL" : "BUY";
}

export function orderSideLabel(visualSide: VisualOrderSide): string {
  switch (visualSide) {
    case "BUY":
      return "买入开多";
    case "SELL":
      return "卖出平多";
    case "SELL_SHORT":
      return "卖出开空";
    case "BUY_COVER":
      return "买入平空";
    default:
      return "买入";
  }
}

export function normalizeOrderType(value: unknown): "MARKET" | "LIMIT" {
  return value === "LIMIT" ? "LIMIT" : "MARKET";
}

export type PineOrderAction =
  | "entry"
  | "order"
  | "close"
  | "closeAll"
  | "cancel"
  | "cancelAll"
  | "riskAllowEntryIn";

export function normalizePineOrderAction(value: unknown): PineOrderAction {
  if (
    value === "entry" ||
    value === "order" ||
    value === "close" ||
    value === "closeAll" ||
    value === "cancel" ||
    value === "cancelAll" ||
    value === "riskAllowEntryIn"
  ) {
    return value;
  }
  return "entry";
}

export type PineRiskAllowEntryDirection = "all" | "long" | "short";

export function normalizePineRiskAllowEntryDirection(
  value: unknown,
): PineRiskAllowEntryDirection {
  if (value === "long" || value === "short") {
    return value;
  }
  return "all";
}

export type EntryPositionPolicy = "sameDirection" | "flatOnly" | "allow";

export function normalizeEntryPositionPolicy(value: unknown): EntryPositionPolicy {
  if (value === "flatOnly" || value === "allow") {
    return value;
  }
  return "sameDirection";
}

export function entryPositionPolicyLabel(value: EntryPositionPolicy): string {
  switch (value) {
    case "flatOnly":
      return "必须空仓";
    case "allow":
      return "允许加仓";
    default:
      return "拦截同向加仓";
  }
}

export function entryPositionPolicyToSnakeCase(value: EntryPositionPolicy): string {
  switch (value) {
    case "flatOnly":
      return "flat_only";
    case "allow":
      return "allow";
    case "sameDirection":
    default:
      return "same_direction";
  }
}

export type QuantityMode =
  | "shares"
  | "amount"
  | "equityPercent";

export function normalizeQuantityMode(value: unknown): QuantityMode {
  if (
    value === "shares" ||
    value === "amount" ||
    value === "equityPercent" ||
    value === "accountPositionPercent"
  ) {
    return value === "accountPositionPercent" ? "equityPercent" : value;
  }
  return "shares";
}

export function isQuantityModeAllowedForSide(
  quantityMode: QuantityMode,
  visualSide: VisualOrderSide,
): boolean {
  void quantityMode;
  void visualSide;
  return true;
}

export function normalizeQuantityModeForSide(
  value: unknown,
  visualSide: VisualOrderSide,
): QuantityMode {
  const quantityMode = normalizeQuantityMode(value);
  if (isQuantityModeAllowedForSide(quantityMode, visualSide)) {
    return quantityMode;
  }
  return "shares";
}

export function toScriptMessage(message: string): string {
  if (message.includes("${")) {
    return `\`${escapeTemplateLiteral(message)}\``;
  }
  return JSON.stringify(message);
}

export function toConsoleLogArgument(message: string): string {
  const pureExpression = readPureTemplateExpression(message);
  return pureExpression ?? toScriptMessage(message);
}

function readPureTemplateExpression(message: string): string | null {
  const trimmed = message.trim();
  const match = trimmed.match(/^\$\{([\s\S]+)\}$/);
  if (match === null) {
    return null;
  }

  const expression = match[1]?.trim() ?? "";
  return expression === "" ? null : expression;
}

function escapeTemplateLiteral(value: string): string {
  return value.replace(/`/g, "\\`");
}

function indent(depth: number): string {
  return "  ".repeat(depth);
}
