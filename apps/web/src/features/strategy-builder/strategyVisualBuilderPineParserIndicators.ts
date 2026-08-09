import { parsePineExpressionToVisualExpression } from "./strategyVisualBuilderExpressions";
import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import {
  createNodeFromParts,
  failUnsupportedPineStatement,
} from "./strategyVisualBuilderPineParserModel";
import {
  defaultIndicatorText,
  indicatorTypeForCondition,
  parseNamedArgs,
  readPineBooleanArg,
  readAnyNumber,
  readCallArgs,
  readNonNegativeNumber,
  readNumber,
  readPineLiteral,
  readPineOptionalStringArg,
  readPineRawArg,
  readSource,
  splitArguments,
  stripWrappingParens,
} from "./strategyVisualBuilderPineParserSyntax";
import type {
  ParseState,
  ParsedNodeResult,
  ParsedPineEntry,
} from "./strategyVisualBuilderPineParserTypes";
import type { StrategyFlowNodeJsDoc } from "./strategyVisualBuilderShared";

export function parsePineExitNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult | null {
  const properties = parsePineExit(entry.trimmed);
  if (properties === null) {
    return failUnsupportedPineStatement(state, entry);
  }
  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: explicitKind ?? "stopLoss",
      defaultText: entry.annotation?.nodeText ?? "风控退出",
      defaultType: "rect",
      properties: {
        blockKind: explicitKind ?? "stopLoss",
        ...properties,
      },
    }),
    isCondition: false,
  };
}

export function parsePineExit(trimmed: string): Record<string, unknown> | null {
  const args = splitArguments(readCallArgs(trimmed));
  if (args.length < 1) {
    return null;
  }
  const exitId = readPineLiteral(args[0] ?? "");
  let orderArgs = args.slice(1);
  const fromEntryArg = parseNamedArgs(orderArgs).get("from_entry");
  const firstOrderArg = orderArgs[0] ?? "";
  const hasPositionalFromEntry = firstOrderArg !== "" && !firstOrderArg.includes("=");
  const fromEntry = fromEntryArg !== undefined
    ? readPineLiteral(fromEntryArg)
    : hasPositionalFromEntry
      ? readPineLiteral(firstOrderArg)
      : "";
  if (fromEntryArg === undefined && hasPositionalFromEntry) {
    orderArgs = orderArgs.slice(1);
  }
  const fromEntryLower = fromEntry.toLowerCase();
  const direction = fromEntry === "" ? "auto" : fromEntryLower.includes("short") ? "short" : "long";
  const fromEntryMode = fromEntry === "" ? "auto" : "explicit";
  const identity = {
    exitId,
    ...(fromEntry === "" ? {} : { fromEntryId: fromEntry }),
  };
  const namedArgs = parseNamedArgs(orderArgs);
  const quantityPercentage = readAnyNumber(namedArgs.get("qty_percent"), 100);
  const metadata = parsePineExitMetadata(namedArgs);
  const stop = namedArgs.get("stop");
  const limit = namedArgs.get("limit");
  const profit = namedArgs.get("profit");
  const loss = namedArgs.get("loss");
  const stopExpressionAst = stop === undefined ? null : parsePineExpressionToVisualExpression(stop);
  const limitExpressionAst = limit === undefined ? null : parsePineExpressionToVisualExpression(limit);
  if (stop !== undefined || limit !== undefined || profit !== undefined || loss !== undefined) {
    const mode = stop !== undefined || loss !== undefined
      ? (limit !== undefined || profit !== undefined ? "bracketExit" : "stopLoss")
      : "takeProfit";
    const stopPercentage = stop === undefined ? null : parsePineExitPricePercent(stop);
    const takeProfitPercentage = limit === undefined ? null : parsePineExitPricePercent(limit);
    if (
      (stop !== undefined && stopPercentage === null && stopExpressionAst === null)
      || (limit !== undefined && takeProfitPercentage === null && limitExpressionAst === null)
    ) {
      return null;
    }
    const properties = pineExitProperties(
      direction,
      mode,
      stopPercentage ?? readAnyNumber(loss, 2),
      takeProfitPercentage ?? readAnyNumber(profit, 4),
      quantityPercentage,
    );
    return {
      ...properties,
      ...identity,
      fromEntryMode,
      ...metadata,
      ...(profit === undefined ? {} : { profitTicks: readAnyNumber(profit, 50) }),
      ...(loss === undefined ? {} : { lossTicks: readAnyNumber(loss, 25) }),
      ...(stopExpressionAst === null ? {} : { stopPriceExpressionAst: stopExpressionAst }),
      ...(limitExpressionAst === null ? {} : { takeProfitPriceExpressionAst: limitExpressionAst }),
    };
  }
  const trailPoints = namedArgs.get("trail_points");
  const trailPrice = namedArgs.get("trail_price");
  const trailOffset = namedArgs.get("trail_offset");
  const trailValue = trailPoints ?? trailPrice;
  if (trailValue !== undefined && trailOffset !== undefined) {
    const percentage = parsePineExitTrailPercent(trailValue);
    const offsetPercentage = parsePineExitTrailPercent(trailOffset);
    const trailExpressionAst = parsePineExpressionToVisualExpression(trailValue);
    const trailOffsetExpressionAst = parsePineExpressionToVisualExpression(trailOffset);
    if (
      (percentage === null && trailExpressionAst === null)
      || (offsetPercentage === null && trailOffsetExpressionAst === null)
    ) {
      return null;
    }
    const properties = pineExitProperties(
      direction,
      "trailingStop",
      percentage ?? offsetPercentage ?? 2,
      undefined,
      quantityPercentage,
    );
    return {
      ...properties,
      ...identity,
      fromEntryMode,
      ...metadata,
      trailingPriceMode: trailPrice === undefined ? "points" : "price",
      ...(trailExpressionAst === null ? {} : { trailingPriceExpressionAst: trailExpressionAst }),
      ...(trailOffsetExpressionAst === null ? {} : { trailingOffsetExpressionAst: trailOffsetExpressionAst }),
    };
  }
  return null;
}

export function parsePineExitMetadata(namedArgs: Map<string, string>): Record<string, unknown> {
  return {
    ...(readPineOptionalStringArg(namedArgs.get("comment")) === undefined ? {} : { comment: readPineOptionalStringArg(namedArgs.get("comment")) }),
    ...(readPineOptionalStringArg(namedArgs.get("comment_profit")) === undefined ? {} : { comment_profit: readPineOptionalStringArg(namedArgs.get("comment_profit")) }),
    ...(readPineOptionalStringArg(namedArgs.get("comment_loss")) === undefined ? {} : { comment_loss: readPineOptionalStringArg(namedArgs.get("comment_loss")) }),
    ...(readPineOptionalStringArg(namedArgs.get("comment_trailing")) === undefined ? {} : { comment_trailing: readPineOptionalStringArg(namedArgs.get("comment_trailing")) }),
    ...(readPineOptionalStringArg(namedArgs.get("alert_message")) === undefined ? {} : { alert_message: readPineOptionalStringArg(namedArgs.get("alert_message")) }),
    ...(readPineOptionalStringArg(namedArgs.get("alert_profit")) === undefined ? {} : { alert_profit: readPineOptionalStringArg(namedArgs.get("alert_profit")) }),
    ...(readPineOptionalStringArg(namedArgs.get("alert_loss")) === undefined ? {} : { alert_loss: readPineOptionalStringArg(namedArgs.get("alert_loss")) }),
    ...(readPineOptionalStringArg(namedArgs.get("alert_trailing")) === undefined ? {} : { alert_trailing: readPineOptionalStringArg(namedArgs.get("alert_trailing")) }),
    ...(readPineBooleanArg(namedArgs.get("disable_alert")) === undefined ? {} : { disable_alert: readPineBooleanArg(namedArgs.get("disable_alert")) }),
    ...(readPineRawArg(namedArgs.get("when")) === undefined ? {} : { when: readPineRawArg(namedArgs.get("when")) }),
  };
}

export function pineExitProperties(
  direction: "auto" | "long" | "short",
  mode: "stopLoss" | "takeProfit" | "trailingStop" | "bracketExit",
  percentage: number,
  takeProfitPercentage?: number,
  quantityPercentage: number = 100,
): Record<string, unknown> {
  return {
    direction,
    mode,
    timeValue: 1,
    timeUnit: "bar",
    percentage,
    ...(takeProfitPercentage === undefined ? {} : { takeProfitPercentage }),
    quantityPercentage,
    windowPolicy: "continuous",
  };
}

export function parsePineExitPricePercent(value: string): number | null {
  const normalized = stripWrappingParens(value).replace(/\s+/g, " ");
  const match = normalized.match(/^close \* \(?1 [+-] (-?\d+(?:\.\d+)?) \/ 100\)?$/i);
  if (match === null) {
    return null;
  }
  const parsed = Number(match[1]);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

export function parsePineExitTrailPercent(value: string): number | null {
  const normalized = stripWrappingParens(value).replace(/\s+/g, " ");
  const match = normalized.match(/^close \* (-?\d+(?:\.\d+)?) \/ 100$/i);
  if (match === null) {
    return null;
  }
  const parsed = Number(match[1]);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

export function parseIndicatorCondition(
  condition: string,
  state: ParseState,
  annotation: StrategyFlowNodeJsDoc | null,
): { properties: Record<string, unknown>; inputs: Array<{ nodeId: string; slot: "primary" | "fast" | "slow" }> } | null {
  const numericMatch = condition.match(/^([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\s*([<>])\s*(-?\d+(?:\.\d+)?)$/);
  if (numericMatch !== null) {
    const alias = numericMatch[1]!.split(".")[0]!;
    const binding = state.aliasByName.get(alias);
    if (binding === undefined) {
      return null;
    }
    const primaryNodeId = annotation?.inputPrimaryNodeId ?? binding.nodeId;
    return {
      properties: {
        blockKind: "technicalIndicatorCondition",
        indicatorType: indicatorTypeForCondition(binding.indicatorType),
        conditionMode: "numeric",
        operator: numericMatch[2]!,
        threshold: Number(numericMatch[3]!),
        inputPrimaryNodeId: primaryNodeId,
      },
      inputs: [{ nodeId: primaryNodeId, slot: "primary" }],
    };
  }

  const crossMatch = condition.match(/^ta\.cross(over|under)\(([^,]+),\s*([^\)]+)\)$/);
  if (crossMatch !== null) {
    const direction = crossMatch[1]!;
    const leftAlias = crossMatch[2]!.trim().split(".")[0]!;
    const rightAlias = crossMatch[3]!.trim().split(".")[0]!;
    const leftBinding = state.aliasByName.get(leftAlias);
    const rightBinding = state.aliasByName.get(rightAlias);
    if (leftBinding === undefined || rightBinding === undefined) {
      return null;
    }
    const isMovingAverage = leftBinding.indicatorType === "movingAverage" || rightBinding.indicatorType === "movingAverage";
    const indicatorType = isMovingAverage ? "movingAverage" : leftBinding.indicatorType;
    const fastNodeId = annotation?.inputFastNodeId ?? leftBinding.nodeId;
    const slowNodeId = annotation?.inputSlowNodeId ?? rightBinding.nodeId;
    const primaryNodeId = annotation?.inputPrimaryNodeId ?? leftBinding.nodeId;
    return {
      properties: {
        blockKind: "technicalIndicatorCondition",
        indicatorType: indicatorTypeForCondition(indicatorType),
        conditionMode: "pattern",
        patternType: direction === "under" ? "deathCross" : "goldenCross",
        ...(isMovingAverage
          ? { inputFastNodeId: fastNodeId, inputSlowNodeId: slowNodeId }
          : { inputPrimaryNodeId: primaryNodeId }),
      },
      inputs: isMovingAverage
        ? [
            { nodeId: fastNodeId, slot: "fast" },
            { nodeId: slowNodeId, slot: "slow" },
          ]
        : [{ nodeId: primaryNodeId, slot: "primary" }],
    };
  }

  const divergenceMatch = condition.match(/^divergence_(top|bottom)\(([A-Za-z_][A-Za-z0-9_]*),\s*(\d+)\)$/);
  if (divergenceMatch !== null) {
    const alias = divergenceMatch[2]!;
    const binding = state.aliasByName.get(alias);
    if (binding === undefined) {
      return null;
    }
    const primaryNodeId = annotation?.inputPrimaryNodeId ?? binding.nodeId;
    return {
      properties: {
        blockKind: "technicalIndicatorCondition",
        indicatorType: indicatorTypeForCondition(binding.indicatorType),
        conditionMode: "pattern",
        patternType: divergenceMatch[1] === "top" ? "topDivergence" : "bottomDivergence",
        lookback: Number(divergenceMatch[3]!),
        inputPrimaryNodeId: primaryNodeId,
      },
      inputs: [{ nodeId: primaryNodeId, slot: "primary" }],
    };
  }

  const bollingerMatch = condition.match(/^close\s*([<>])\s*([A-Za-z_][A-Za-z0-9_]*)\.(upper|lower)$/);
  if (bollingerMatch !== null) {
    const alias = bollingerMatch[2]!;
    const binding = state.aliasByName.get(alias);
    if (binding === undefined) {
      return null;
    }
    const primaryNodeId = annotation?.inputPrimaryNodeId ?? binding.nodeId;
    return {
      properties: {
        blockKind: "technicalIndicatorCondition",
        indicatorType: "bollinger",
        conditionMode: "pattern",
        patternType: bollingerMatch[3] === "upper" ? "closeAboveUpperBand" : "closeBelowLowerBand",
        inputPrimaryNodeId: primaryNodeId,
      },
      inputs: [{ nodeId: primaryNodeId, slot: "primary" }],
    };
  }

  return null;
}

export function parseCloseCondition(
  condition: string,
  explicitKind: StrategyBlockKind | undefined,
): { kind: "ifCloseAbove" | "ifCloseBelow"; threshold: number; text: string } | null {
  const match = condition.match(/^close\s*([<>])\s*(-?\d+(?:\.\d+)?)$/);
  if (match === null && explicitKind !== "ifCloseAbove" && explicitKind !== "ifCloseBelow") {
    return null;
  }
  const operator = match?.[1] ?? (explicitKind === "ifCloseBelow" ? "<" : ">");
  const threshold = Number(match?.[2] ?? 0);
  const kind = operator === "<" ? "ifCloseBelow" : "ifCloseAbove";
  return {
    kind,
    threshold: Number.isFinite(threshold) ? threshold : 0,
    text: kind === "ifCloseBelow" ? "收盘价 < 阈值" : "收盘价 > 阈值",
  };
}

export function parseIndicatorExpression(expression: string): Record<string, unknown> | null {
  const call = expression.match(/^([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\((.*)\)$/);
  if (call === null) {
    return null;
  }
  const functionName = call[1]!.toLowerCase();
  const args = splitArguments(call[2] ?? "");

  if (functionName === "request.security") {
    return parseRequestSecurityIndicator(args);
  }

  switch (functionName) {
    case "ta.ema":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "EMA",
        source: readSource(args[0]),
        windowSize: readNumber(args[1] ?? args[0], 20),
        timeframe: "",
      };
    case "ta.rma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "SMMA",
        source: readSource(args[0]),
        windowSize: readNumber(args[1] ?? args[0], 20),
        timeframe: "",
      };
    case "ta.wma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "LWMA",
        source: readSource(args[0]),
        windowSize: readNumber(args[1] ?? args[0], 20),
        timeframe: "",
      };
    case "ta.hma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "HMA",
        source: readSource(args[0]),
        windowSize: readNumber(args[1] ?? args[0], 20),
        timeframe: "",
      };
    case "ta.vwma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "VWMA",
        source: readSource(args[0]),
        windowSize: readNumber(args[1] ?? args[0], 20),
        timeframe: "",
      };
    case "ta.sma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "SMA",
        source: readSource(args[0]),
        windowSize: readNumber(args[1] ?? args[0], 20),
        timeframe: "",
      };
    case "ta.rsi":
      return { blockKind: "getTechnicalIndicator", indicatorType: "rsi", period: readNumber(args[1] ?? args[0], 14) };
    case "ta.macd":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "macd",
        fastPeriod: readNumber(args[1], 12),
        slowPeriod: readNumber(args[2], 26),
        signalPeriod: readNumber(args[3], 9),
      };
    case "ta.atr":
      return { blockKind: "getTechnicalIndicator", indicatorType: "atr", period: readNumber(args[0], 14) };
    case "ta.cci":
      return { blockKind: "getTechnicalIndicator", indicatorType: "cci", source: readSource(args[0], "hlc3"), period: readNumber(args[1] ?? args[0], 20) };
    case "ta.bb":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "bollinger",
        period: readNumber(args[1] ?? args[0], 20),
        multiplier: readNumber(args[2], 2),
      };
    case "ta.wpr":
      return { blockKind: "getTechnicalIndicator", indicatorType: "williamsR", period: readNumber(args[0], 14) };
    case "ta.stdev":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "stdev",
        source: readSource(args[0], "close"),
        period: readNumber(args[1] ?? args[0], 20),
      };
    case "ta.variance":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "variance",
        source: readSource(args[0], "close"),
        period: readNumber(args[1] ?? args[0], 20),
      };
    case "ta.highest":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "highest",
        source: readSource(args.length > 1 ? args[0] : undefined, "high"),
        period: readNumber(args.length > 1 ? args[1] : args[0], 20),
      };
    case "ta.lowest":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "lowest",
        source: readSource(args.length > 1 ? args[0] : undefined, "low"),
        period: readNumber(args.length > 1 ? args[1] : args[0], 20),
      };
    case "ta.sum":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "sum",
        source: readSource(args[0], "volume"),
        period: readNumber(args[1] ?? args[0], 20),
      };
    case "ta.vwap":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "vwap",
        source: readSource(args[0], "hlc3"),
      };
    case "ta.mfi":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "mfi",
        source: readSource(args[0], "hlc3"),
        period: readNumber(args[1] ?? args[0], 14),
      };
    case "ta.dmi":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "dmi",
        period: readNumber(args[0], 14),
        adxSmoothing: readNumber(args[1], 14),
      };
    case "ta.supertrend":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "supertrend",
        factor: readNumber(args[0], 3),
        period: readNumber(args[1], 10),
      };
    case "ta.sar":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "sar",
        start: readNumber(args[0], 0.02),
        increment: readNumber(args[1], 0.02),
        maximum: readNumber(args[2], 0.2),
      };
    case "ta.linreg":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "linreg",
        source: readSource(args[0], "close"),
        period: readNumber(args[1], 5),
        offset: readNonNegativeNumber(args[2], 0),
      };
    case "ta.obv":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "obv",
        source: readSource(args[0], "close"),
      };
    case "ta.pivothigh": {
      const hasSource = args.length >= 3;
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "pivotHigh",
        source: readSource(hasSource ? args[0] : undefined, "high"),
        leftBars: readNumber(hasSource ? args[1] : args[0], 2),
        rightBars: readNumber(hasSource ? args[2] : args[1], 2),
      };
    }
    case "ta.pivotlow": {
      const hasSource = args.length >= 3;
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "pivotLow",
        source: readSource(hasSource ? args[0] : undefined, "low"),
        leftBars: readNumber(hasSource ? args[1] : args[0], 2),
        rightBars: readNumber(hasSource ? args[2] : args[1], 2),
      };
    }
    case "ta.kc":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "keltner",
        source: readSource(args[0], "close"),
        period: readNumber(args[1], 20),
        multiplier: readNumber(args[2], 1.5),
      };
    case "ta.alma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "alma",
        source: readSource(args[0], "close"),
        period: readNumber(args[1], 20),
        offset: readNumber(args[2], 0.85),
        sigma: readNumber(args[3], 6),
      };
    default:
      return null;
  }
}
export function parseRequestSecurityIndicator(args: string[]): Record<string, unknown> | null {
  if (args.length < 3 || args[0]?.trim() !== "syminfo.tickerid") {
    return null;
  }
  const timeframe = normalizePineTimeframe(readPineLiteral(args[1] ?? ""));
  if (timeframe === null) {
    return null;
  }
  const inner = parseIndicatorExpression(args[2] ?? "");
  if (inner === null || !supportsRequestSecurityIndicator(inner.indicatorType)) {
    return null;
  }
  return {
    ...inner,
    timeframe,
  };
}

export function supportsRequestSecurityIndicator(indicatorType: unknown): boolean {
  return typeof indicatorType === "string" && [
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

export function normalizePineTimeframe(value: string): string | null {
  switch (value.trim().toUpperCase()) {
    case "1":
      return "1";
    case "5":
      return "5";
    case "15":
      return "15";
    case "30":
      return "30";
    case "45":
      return "45";
    case "60":
      return "60";
    case "120":
      return "120";
    case "240":
      return "240";
    case "D":
      return "D";
    case "W":
      return "W";
    case "M":
      return "M";
    default:
      return null;
  }
}
