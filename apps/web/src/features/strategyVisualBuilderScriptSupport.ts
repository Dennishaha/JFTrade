import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";

export interface StrategyScriptRuntimeFlags {
  usesMovingAverageRuntime: boolean;
  usesRSIRuntime: boolean;
  usesMACDRuntime: boolean;
  usesBollingerRuntime: boolean;
  usesSimpleMovingAverageHelper: boolean;
  usesSeriesStateRuntime: boolean;
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
    `${indent(1)}state.closes.push(close);`,
    `${indent(1)}if (state.closes.length > MAX_CACHE_SIZE) {`,
    `${indent(2)}state.closes.shift();`,
    `${indent(1)}}`,
    "",
    ...(flags.usesMovingAverageRuntime
      ? [
          `${indent(1)}let fastAverage = null;`,
          `${indent(1)}let slowAverage = null;`,
          `${indent(1)}const prevFastAverage = state.prevFastAverage;`,
          `${indent(1)}const prevSlowAverage = state.prevSlowAverage;`,
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
    ...(flags.usesBollingerRuntime
      ? [
          `${indent(1)}let latestBollinger = null;`,
          `${indent(1)}let latestBollingerMiddle = null;`,
          `${indent(1)}let latestBollingerUpper = null;`,
          `${indent(1)}let latestBollingerLower = null;`,
        ]
      : []),
  ];
}

export function buildScriptRuntimeBlocks(
  flags: StrategyScriptRuntimeFlags,
): string[] {
  return [
    ...(flags.usesSeriesStateRuntime
      ? [
          "const MAX_CACHE_SIZE = 96;",
          "",
          "const state = {",
          "  closes: [],",
          ...(flags.usesMovingAverageRuntime
            ? [
                "  prevFastAverage: null,",
                "  prevSlowAverage: null,",
              ]
            : []),
          "};",
          "",
        ]
      : []),
    ...(flags.usesSimpleMovingAverageHelper
      ? [
          "/** @param {number[]} values @param {number} windowSize @returns {number | null} */",
          "function simpleMovingAverage(values, windowSize) {",
          "  if (values.length < windowSize) {",
          "    return null;",
          "  }",
          "  let sum = 0;",
          "  for (let index = values.length - windowSize; index < values.length; index += 1) {",
          "    sum += values[index];",
          "  }",
          "  return sum / windowSize;",
          "}",
          "",
        ]
      : []),
    ...(flags.usesRSIRuntime
      ? [
          "/** @param {number[]} values @param {number} period @returns {number | null} */",
          "function calculateRSI(values, period) {",
          "  if (values.length <= period) {",
          "    return null;",
          "  }",
          "  let gains = 0;",
          "  let losses = 0;",
          "  for (let index = values.length - period; index < values.length; index += 1) {",
          "    const delta = values[index] - values[index - 1];",
          "    if (delta >= 0) {",
          "      gains += delta;",
          "    } else {",
          "      losses += Math.abs(delta);",
          "    }",
          "  }",
          "  if (losses === 0) {",
          "    return 100;",
          "  }",
          "  const relativeStrength = gains / losses;",
          "  return 100 - 100 / (1 + relativeStrength);",
          "}",
          "",
        ]
      : []),
    ...(flags.usesMACDRuntime
      ? [
          "/** @param {number[]} values @param {number} period @returns {number[]} */",
          "function calculateEMASequence(values, period) {",
          "  const multiplier = 2 / (period + 1);",
          "  let previous = null;",
          "  return values.map((value) => {",
          "    previous = previous === null ? value : previous + (value - previous) * multiplier;",
          "    return previous;",
          "  });",
          "}",
          "",
          "/** @param {number[]} values @param {number} fastPeriod @param {number} slowPeriod @param {number} signalPeriod @returns {{ diff: number; signal: number; histogram: number } | null} */",
          "function calculateMACD(values, fastPeriod, slowPeriod, signalPeriod) {",
          "  if (values.length < Math.max(fastPeriod, slowPeriod) + signalPeriod) {",
          "    return null;",
          "  }",
          "  const fastSequence = calculateEMASequence(values, fastPeriod);",
          "  const slowSequence = calculateEMASequence(values, slowPeriod);",
          "  const diffSequence = fastSequence.map((value, index) => value - slowSequence[index]);",
          "  const signalSequence = calculateEMASequence(diffSequence, signalPeriod);",
          "  const diff = diffSequence[diffSequence.length - 1];",
          "  const signal = signalSequence[signalSequence.length - 1];",
          "  if (diff === undefined || signal === undefined) {",
          "    return null;",
          "  }",
          "  return {",
          "    diff,",
          "    signal,",
          "    histogram: (diff - signal) * 2,",
          "  };",
          "}",
          "",
        ]
      : []),
    ...(flags.usesBollingerRuntime
      ? [
          "/** @param {number[]} values @param {number} average @returns {number} */",
          "function calculateStandardDeviation(values, average) {",
          "  const variance = values.reduce((sum, value) => sum + (value - average) * (value - average), 0) / values.length;",
          "  return Math.sqrt(variance);",
          "}",
          "",
          "/** @param {number[]} values @param {number} period @param {number} multiplier @returns {{ middle: number; upper: number; lower: number } | null} */",
          "function calculateBollingerBands(values, period, multiplier) {",
          "  if (values.length < period) {",
          "    return null;",
          "  }",
          "  const windowValues = values.slice(values.length - period);",
          "  const middle = simpleMovingAverage(windowValues, period);",
          "  if (middle === null) {",
          "    return null;",
          "  }",
          "  const standardDeviation = calculateStandardDeviation(windowValues, middle);",
          "  return {",
          "    middle,",
          "    upper: middle + standardDeviation * multiplier,",
          "    lower: middle - standardDeviation * multiplier,",
          "  };",
          "}",
          "",
        ]
      : []),
  ];
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

export type QuantityMode = "shares" | "amount" | "positionPercent" | "cashPercent";

export function normalizeQuantityMode(value: unknown): QuantityMode {
  if (
    value === "shares" ||
    value === "amount" ||
    value === "positionPercent" ||
    value === "cashPercent"
  ) {
    return value;
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