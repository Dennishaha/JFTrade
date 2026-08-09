import type { StrategyVisualNodeDocument } from "@/types";

import type { StrategySeriesSource } from "./strategyVisualBuilderCatalog";
import type { GetTechnicalIndicatorBlockProperties } from "./strategyVisualBuilderIndicatorBlock";
import {
  isPineIdentifier,
  sanitizePineIdentifier,
  toPineStringLiteral,
} from "./strategyVisualBuilderPineFormat";

export function buildKDJIndicatorStatements(
  variableName: string,
  properties: GetTechnicalIndicatorBlockProperties,
): string[] {
  const period = Math.max(1, Math.round(properties.period!));
  const m1 = Math.max(1, Math.round(properties.m1!));
  const m2 = Math.max(1, Math.round(properties.m2!));
  const highName = `${variableName}_highest`;
  const lowName = `${variableName}_lowest`;
  const rsvName = `${variableName}_rsv`;
  const kName = `${variableName}_k`;
  const dName = `${variableName}_d`;
  const jName = `${variableName}_j`;
  return [
    `${highName} = ta.highest(high, ${period})`,
    `${lowName} = ta.lowest(low, ${period})`,
    `${rsvName} = ${highName} == ${lowName} ? 50 : ((close - ${lowName}) / (${highName} - ${lowName})) * 100`,
    `var ${kName} = 50.0`,
    `var ${dName} = 50.0`,
    `${kName} := ((${m1 - 1}) * nz(${kName}[1], 50) + ${rsvName}) / ${m1}`,
    `${dName} := ((${m2 - 1}) * nz(${dName}[1], 50) + ${kName}) / ${m2}`,
    `${jName} = 3 * ${kName} - 2 * ${dName}`,
  ];
}

export function readSyntheticKDJVariableName(value: unknown, field: "k" | "d" | "j"): string {
  const base = typeof value === "string" && isPineIdentifier(value)
    ? value
    : "kdj";
  return `${base}_${field}`;
}

export function buildIndicatorExpression(properties: GetTechnicalIndicatorBlockProperties): string {
  const wrapTimeframe = (expression: string): string =>
    wrapIndicatorTimeframe(properties, expression);

  switch (properties.indicatorType) {
    case "movingAverage": {
      const movingAverageType = properties.movingAverageType!;
      const windowSize = properties.windowSize!;
      const expression = buildMovingAverageExpression(movingAverageType, windowSize, properties.source!);
      return wrapTimeframe(expression);
    }
    case "macd":
      return wrapTimeframe(`ta.macd(close, ${properties.fastPeriod!}, ${properties.slowPeriod!}, ${properties.signalPeriod!})`);
    case "kdj":
      return `${readSyntheticKDJVariableName(properties.variableName, "j")}`;
    case "bollinger":
      return wrapTimeframe(`ta.bb(close, ${properties.period!}, ${properties.multiplier!})`);
    case "atr":
      return wrapTimeframe(`ta.atr(${properties.period!})`);
    case "cci":
      return wrapTimeframe(`ta.cci(${pineIndicatorSource(properties.source!)}, ${properties.period!})`);
    case "williamsR":
      return `ta.wpr(${properties.period!})`;
    case "stdev":
      return wrapTimeframe(`ta.stdev(${pineIndicatorSource(properties.source!)}, ${properties.period!})`);
    case "variance":
      return wrapTimeframe(`ta.variance(${pineIndicatorSource(properties.source!)}, ${properties.period!})`);
    case "highest":
      return wrapTimeframe(`ta.highest(${pineIndicatorSource(properties.source!)}, ${properties.period!})`);
    case "lowest":
      return wrapTimeframe(`ta.lowest(${pineIndicatorSource(properties.source!)}, ${properties.period!})`);
    case "sum":
      return wrapTimeframe(`ta.sum(${pineIndicatorSource(properties.source!)}, ${properties.period!})`);
    case "vwap":
      return `ta.vwap(${pineIndicatorSource(properties.source!)})`;
    case "mfi":
      return wrapTimeframe(`ta.mfi(${pineIndicatorSource(properties.source!)}, ${properties.period!})`);
    case "dmi":
      return `ta.dmi(${properties.period!}, ${properties.adxSmoothing!})`;
    case "supertrend":
      return wrapTimeframe(`ta.supertrend(${properties.factor!}, ${properties.period!})`);
    case "sar":
      return `ta.sar(${properties.start!}, ${properties.increment!}, ${properties.maximum!})`;
    case "linreg":
      return wrapTimeframe(`ta.linreg(${pineIndicatorSource(properties.source!)}, ${properties.period!}, ${properties.offset!})`);
    case "obv":
      return wrapTimeframe("ta.obv");
    case "pivotHigh":
      return wrapTimeframe(`ta.pivothigh(${pineIndicatorSource(properties.source!)}, ${properties.leftBars!}, ${properties.rightBars!})`);
    case "pivotLow":
      return wrapTimeframe(`ta.pivotlow(${pineIndicatorSource(properties.source!)}, ${properties.leftBars!}, ${properties.rightBars!})`);
    case "keltner":
      return wrapTimeframe(`ta.kc(${pineIndicatorSource(properties.source!)}, ${properties.period!}, ${properties.multiplier!}, true)`);
    case "alma":
      return wrapTimeframe(`ta.alma(${pineIndicatorSource(properties.source!)}, ${properties.period!}, ${properties.offset!}, ${properties.sigma!})`);
    case "rsi":
    default:
      return wrapTimeframe(`ta.rsi(close, ${properties.period!})`);
  }
}

export function wrapIndicatorTimeframe(
  properties: GetTechnicalIndicatorBlockProperties,
  expression: string,
): string {
  if (!supportsIndicatorRequestSecurity(properties.indicatorType)) {
    return expression;
  }
  const timeframe = properties.timeframe?.trim();
  return timeframe == null || timeframe === ""
    ? expression
    : `request.security(syminfo.tickerid, ${toPineStringLiteral(timeframe)}, ${expression})`;
}

export function supportsIndicatorRequestSecurity(
  indicatorType: GetTechnicalIndicatorBlockProperties["indicatorType"],
): boolean {
  return [
    "movingAverage",
    "rsi",
    "macd",
    "atr",
    "cci",
    "bollinger",
    "stdev",
    "variance",
    "highest",
    "lowest",
    "sum",
    "mfi",
    "supertrend",
    "linreg",
    "obv",
    "pivotHigh",
    "pivotLow",
    "keltner",
    "alma",
  ].includes(indicatorType);
}

export function buildMovingAverageExpression(
  movingAverageType: string,
  windowSize: number,
  source: string = "close",
): string {
  const pineSource = pineMovingAverageSource(source);
  switch (movingAverageType) {
    case "EMA":
    case "EXPMA":
      return `ta.ema(${pineSource}, ${windowSize})`;
    case "SMMA":
      return `ta.rma(${pineSource}, ${windowSize})`;
    case "LWMA":
      return `ta.wma(${pineSource}, ${windowSize})`;
    case "HMA":
      return `ta.hma(${pineSource}, ${windowSize})`;
    case "VWMA":
      return `ta.vwma(${pineSource}, ${windowSize})`;
    case "MA":
    case "SMA":
    case "TMA":
    case "BOLL":
    default:
      return `ta.sma(${pineSource}, ${windowSize})`;
  }
}

export function pineMovingAverageSource(source: string): string {
  return pineIndicatorSource(source);
}

export function pineIndicatorSource(source: string): string {
  switch (source) {
    case "open":
    case "high":
    case "low":
    case "volume":
    case "hl2":
    case "hlc3":
    case "ohlc4":
      return source;
    case "close":
    default:
      return "close";
  }
}

export function pineSeriesSource(source: StrategySeriesSource): string {
  return pineIndicatorSource(source);
}

export function readIndicatorVariableName(
  node: StrategyVisualNodeDocument,
  properties: GetTechnicalIndicatorBlockProperties,
): string {
  const fromProperties = typeof properties.variableName === "string" ? properties.variableName : "";
  if (isPineIdentifier(fromProperties)) {
    return fromProperties;
  }
  return sanitizePineIdentifier(node.id, "indicator");
}
