import { parsePineExpressionToVisualExpression } from "./strategyVisualBuilderExpressions";

export function readMessageCallOrLiteral(trimmed: string, kind: "log" | "notify"): string {
  const args = splitArguments(readCallArgs(trimmed));
  return readPineLiteral(args[0] ?? "");
}

export function parsePineOrder(trimmed: string): Record<string, unknown> | null {
  if (trimmed.startsWith("strategy.risk.allow_entry_in")) {
    const args = splitArguments(readCallArgs(trimmed));
    return {
      orderAction: "riskAllowEntryIn",
      side: "BUY",
      orderId: "",
      orderType: "MARKET",
      entryPositionPolicy: "sameDirection",
      quantityMode: "shares",
      quantityValue: 100,
      limitPrice: 0,
      riskAllowedDirection: parsePineRiskAllowEntryDirection(args[0]),
    };
  }
  if (trimmed.startsWith("strategy.close_all")) {
    const args = splitArguments(readCallArgs(trimmed));
    const namedArgs = parseNamedArgs(args);
    const positionalArgs = readPositionalArgs(args);
    return {
      orderAction: "closeAll",
      side: "SELL",
      orderId: "",
      orderType: "MARKET",
      entryPositionPolicy: "sameDirection",
      quantityMode: "shares",
      quantityValue: 100,
      limitPrice: 0,
      pineOrderFunction: "strategy.close_all",
      ...(readPineBooleanArg(namedArgs.get("immediately") ?? positionalArgs[0]) === undefined
        ? {}
        : { immediately: readPineBooleanArg(namedArgs.get("immediately") ?? positionalArgs[0]) }),
      ...(readPineOptionalStringArg(namedArgs.get("comment") ?? positionalArgs[1]) === undefined
        ? {}
        : { comment: readPineOptionalStringArg(namedArgs.get("comment") ?? positionalArgs[1]) }),
      ...(readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[2]) === undefined
        ? {}
        : { alert_message: readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[2]) }),
      ...(readPineBooleanArg(namedArgs.get("disable_alert") ?? positionalArgs[3]) === undefined
        ? {}
        : { disable_alert: readPineBooleanArg(namedArgs.get("disable_alert") ?? positionalArgs[3]) }),
    };
  }
  if (trimmed.startsWith("strategy.cancel_all")) {
    return {
      orderAction: "cancelAll",
      side: "BUY",
      orderId: "",
      orderType: "MARKET",
      entryPositionPolicy: "sameDirection",
      quantityMode: "shares",
      quantityValue: 100,
      limitPrice: 0,
      pineOrderFunction: "strategy.cancel_all",
    };
  }
  if (trimmed.startsWith("strategy.cancel")) {
    const args = splitArguments(readCallArgs(trimmed));
    return {
      orderAction: "cancel",
      side: "BUY",
      orderId: readPineLiteral(args[0] ?? ""),
      orderType: "MARKET",
      entryPositionPolicy: "sameDirection",
      quantityMode: "shares",
      quantityValue: 100,
      limitPrice: 0,
      pineOrderFunction: "strategy.cancel",
    };
  }
  if (trimmed.startsWith("strategy.close")) {
    const args = splitArguments(readCallArgs(trimmed));
    const id = readPineLiteral(args[0] ?? "");
    const orderArgs = args.slice(1);
    const namedArgs = parseNamedArgs(orderArgs);
    const positionalArgs = readPositionalArgs(orderArgs);
    const quantity = parsePineQuantity(namedArgs.get("qty") ?? positionalArgs[0], namedArgs.get("qty_percent"));
    const limit = namedArgs.get("limit");
    const stop = namedArgs.get("stop");
    const limitPriceExpressionAst = limit === undefined ? null : parsePineExpressionToVisualExpression(limit);
    const stopPriceExpressionAst = stop === undefined ? null : parsePineExpressionToVisualExpression(stop);
    return {
      orderAction: "close",
      side: id.toLowerCase().includes("short") ? "BUY_COVER" : "SELL",
      orderId: id,
      orderType: limit === undefined ? "MARKET" : "LIMIT",
      entryPositionPolicy: "sameDirection",
      quantityMode: quantity.mode,
      quantityValue: quantity.value,
      limitPrice: readNumber(limit, 0),
      stopPrice: readNumber(stop, 0),
      ...(limitPriceExpressionAst === null ? {} : { limitPriceExpressionAst }),
      ...(stopPriceExpressionAst === null ? {} : { stopPriceExpressionAst }),
      ...(readPineOptionalStringArg(namedArgs.get("comment")) === undefined ? {} : { comment: readPineOptionalStringArg(namedArgs.get("comment")) }),
      ...(readPineOptionalStringArg(namedArgs.get("alert_message")) === undefined ? {} : { alert_message: readPineOptionalStringArg(namedArgs.get("alert_message")) }),
      ...(readPineBooleanArg(namedArgs.get("immediately")) === undefined ? {} : { immediately: readPineBooleanArg(namedArgs.get("immediately")) }),
      ...(readPineBooleanArg(namedArgs.get("disable_alert")) === undefined ? {} : { disable_alert: readPineBooleanArg(namedArgs.get("disable_alert")) }),
      ...(readPineRawArg(namedArgs.get("when")) === undefined ? {} : { when: readPineRawArg(namedArgs.get("when")) }),
    };
  }
  const isEntry = trimmed.startsWith("strategy.entry");
  const isOrder = trimmed.startsWith("strategy.order");
  if (!isEntry && !isOrder) {
    return null;
  }
  const args = splitArguments(readCallArgs(trimmed));
  const direction = String(args[1] ?? "strategy.long").toLowerCase();
  const orderArgs = args.slice(2);
  const namedArgs = parseNamedArgs(orderArgs);
  const positionalArgs = readPositionalArgs(orderArgs);
  const quantity = parsePineQuantity(namedArgs.get("qty") ?? positionalArgs[0], namedArgs.get("qty_percent"));
  const limit = namedArgs.get("limit");
  const stop = namedArgs.get("stop");
  const limitPrice = readNumber(limit, 0);
  const stopPrice = readNumber(stop, 0);
  const limitPriceExpressionAst = limit === undefined ? null : parsePineExpressionToVisualExpression(limit);
  const stopPriceExpressionAst = stop === undefined ? null : parsePineExpressionToVisualExpression(stop);
  return {
    orderAction: isOrder ? "order" : "entry",
    side: direction.includes("short") ? (isOrder ? "SELL" : "SELL_SHORT") : "BUY",
    orderId: readPineLiteral(args[0] ?? ""),
    orderType: limit === undefined ? "MARKET" : "LIMIT",
    entryPositionPolicy: "sameDirection",
    quantityMode: quantity.mode,
    quantityValue: quantity.value,
    limitPrice,
    stopPrice,
    ...(limitPriceExpressionAst === null ? {} : { limitPriceExpressionAst }),
    ...(stopPriceExpressionAst === null ? {} : { stopPriceExpressionAst }),
    ...(readPineOptionalStringArg(namedArgs.get("comment")) === undefined ? {} : { comment: readPineOptionalStringArg(namedArgs.get("comment")) }),
    ...(readPineOptionalStringArg(namedArgs.get("alert_message")) === undefined ? {} : { alert_message: readPineOptionalStringArg(namedArgs.get("alert_message")) }),
    ...(readPineBooleanArg(namedArgs.get("disable_alert")) === undefined ? {} : { disable_alert: readPineBooleanArg(namedArgs.get("disable_alert")) }),
    ...(readPineRawArg(namedArgs.get("when")) === undefined ? {} : { when: readPineRawArg(namedArgs.get("when")) }),
    pineOrderFunction: isOrder ? "strategy.order" : "strategy.entry",
  };
}

export function parsePineRiskAllowEntryDirection(value: string | undefined): "all" | "long" | "short" {
  const normalized = (value ?? "").trim().toLowerCase();
  if (normalized.includes("short")) {
    return "short";
  }
  if (normalized.includes("long")) {
    return "long";
  }
  return "all";
}

export function parsePineRiskRule(trimmed: string): Record<string, unknown> | null {
  const args = splitArguments(readCallArgs(trimmed));
  const namedArgs = parseNamedArgs(args);
  const positionalArgs = readPositionalArgs(args);
  if (trimmed.startsWith("strategy.risk.allow_entry_in")) {
    return {
      riskRuleType: "allowEntryIn",
      riskAllowedDirection: parsePineRiskAllowEntryDirection(args[0]),
    };
  }
  if (trimmed.startsWith("strategy.risk.max_drawdown")) {
    return {
      riskRuleType: "maxDrawdown",
      riskValue: readAnyNumber(namedArgs.get("value") ?? positionalArgs[0], 10),
      riskAmountType: parsePineRiskAmountType(namedArgs.get("type") ?? positionalArgs[1]),
      ...(readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[2]) === undefined
        ? {}
        : { alert_message: readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[2]) }),
    };
  }
  if (trimmed.startsWith("strategy.risk.max_intraday_loss")) {
    return {
      riskRuleType: "maxIntradayLoss",
      riskValue: readAnyNumber(namedArgs.get("value") ?? positionalArgs[0], 5),
      riskAmountType: parsePineRiskAmountType(namedArgs.get("type") ?? positionalArgs[1]),
      ...(readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[2]) === undefined
        ? {}
        : { alert_message: readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[2]) }),
    };
  }
  if (trimmed.startsWith("strategy.risk.max_intraday_filled_orders")) {
    return {
      riskRuleType: "maxIntradayFilledOrders",
      riskCount: readAnyNumber(namedArgs.get("count") ?? positionalArgs[0], 10),
      ...(readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[1]) === undefined
        ? {}
        : { alert_message: readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[1]) }),
    };
  }
  if (trimmed.startsWith("strategy.risk.max_position_size")) {
    return {
      riskRuleType: "maxPositionSize",
      riskContracts: readAnyNumber(namedArgs.get("contracts") ?? positionalArgs[0], 1),
    };
  }
  if (trimmed.startsWith("strategy.risk.max_cons_loss_days")) {
    return {
      riskRuleType: "maxConsLossDays",
      riskCount: readAnyNumber(namedArgs.get("count") ?? positionalArgs[0], 3),
      ...(readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[1]) === undefined
        ? {}
        : { alert_message: readPineOptionalStringArg(namedArgs.get("alert_message") ?? positionalArgs[1]) }),
    };
  }
  return null;
}

export function parsePineRiskAmountType(value: string | undefined): "strategy.percent_of_equity" | "strategy.cash" {
  return (value ?? "").trim().toLowerCase() === "strategy.cash"
    ? "strategy.cash"
    : "strategy.percent_of_equity";
}

export function parsePineQuantity(
  qty: string | undefined,
  qtyPercent?: string | undefined,
): { mode: "shares" | "amount" | "equityPercent"; value: number } {
  if (qtyPercent !== undefined) {
    return { mode: "equityPercent", value: readNumber(qtyPercent, 100) };
  }
  const normalized = stripWrappingParens(qty ?? "").replace(/\s+/g, " ");
  if (normalized === "") {
    return { mode: "shares", value: 100 };
  }
  const equityMatch = normalized.match(/^strategy\.equity\s*\*\s*(-?\d+(?:\.\d+)?)\s*\/\s*100\s*\/\s*close$/i)
    ?? normalized.match(/^\(?\s*strategy\.equity\s*\*\s*(-?\d+(?:\.\d+)?)\s*\/\s*100\s*\)?\s*\/\s*close$/i);
  if (equityMatch !== null) {
    return { mode: "equityPercent", value: readNumber(equityMatch[1], 100) };
  }
  const amountMatch = normalized.match(/^(-?\d+(?:\.\d+)?)\s*\/\s*close$/i);
  if (amountMatch !== null) {
    return { mode: "amount", value: readNumber(amountMatch[1], 100) };
  }
  return { mode: "shares", value: readNumber(normalized, 100) };
}

export function stripWrappingParens(value: string): string {
  let result = value.trim();
  while (result.startsWith("(") && result.endsWith(")") && wrappingParensCoverExpression(result)) {
    result = result.slice(1, -1).trim();
  }
  return result;
}

export function wrappingParensCoverExpression(value: string): boolean {
  let depth = 0;
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index];
    if (char === "(") {
      depth += 1;
    } else if (char === ")") {
      depth -= 1;
      if (depth === 0 && index < value.length - 1) {
        return false;
      }
    }
  }
  return depth === 0;
}

export function parseNamedArgs(args: string[]): Map<string, string> {
  const result = new Map<string, string>();
  for (const arg of args) {
    const [key, ...rest] = arg.split("=");
    if (key !== undefined && rest.length > 0) {
      result.set(key.trim(), rest.join("=").trim());
    }
  }
  return result;
}

export function readPositionalArgs(args: string[]): string[] {
  return args.filter((arg) => !isNamedArg(arg));
}

export function isNamedArg(value: string): boolean {
  return value.includes("=");
}

export function readCallArgs(value: string): string {
  const open = value.indexOf("(");
  const close = value.lastIndexOf(")");
  if (open < 0 || close <= open) {
    return "";
  }
  return value.slice(open + 1, close);
}

export function splitArguments(value: string): string[] {
  const parts: string[] = [];
  let start = 0;
  let depth = 0;
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index];
    if (char === "(") {
      depth += 1;
    } else if (char === ")") {
      depth = Math.max(0, depth - 1);
    } else if (char === "," && depth === 0) {
      parts.push(value.slice(start, index).trim());
      start = index + 1;
    }
  }
  const tail = value.slice(start).trim();
  if (tail !== "") {
    parts.push(tail);
  }
  return parts;
}

export function readNumber(value: string | undefined, fallback: number): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

export function readAnyNumber(value: string | undefined, fallback: number): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) ? parsed : fallback;
}

export function readLiteralNumber(value: unknown, fallback: number): number {
  if (
    typeof value === "object"
    && value !== null
    && !Array.isArray(value)
    && "kind" in value
    && value.kind === "literal"
    && "value" in value
    && typeof value.value === "number"
  ) {
    return value.value;
  }
  return fallback;
}

export function readNonNegativeNumber(value: string | undefined, fallback: number): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}

export function readSource(
  value: string | undefined,
  fallback: "open" | "high" | "low" | "close" | "volume" | "hl2" | "hlc3" | "ohlc4" = "close",
): "open" | "high" | "low" | "close" | "volume" | "hl2" | "hlc3" | "ohlc4" {
	switch ((value ?? "").trim().toLowerCase()) {
		case "open":
		case "high":
		case "low":
		case "volume":
		case "hl2":
		case "hlc3":
		case "ohlc4":
			return (value ?? "").trim().toLowerCase() as "open" | "high" | "low" | "volume" | "hl2" | "hlc3" | "ohlc4";
		case "close":
			return "close";
		default:
			return fallback;
	}
}

export function readPineLiteral(value: string): string {
  const trimmed = value.trim();
  if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
    try {
      return JSON.parse(trimmed) as string;
    } catch {
      return trimmed.slice(1, -1);
    }
  }
  if (
    (trimmed.startsWith("'") && trimmed.endsWith("'")) ||
    (trimmed.startsWith("`") && trimmed.endsWith("`"))
  ) {
    return trimmed.slice(1, -1);
  }
  return trimmed;
}

export function readPineBooleanArg(value: string | undefined): boolean | undefined {
  switch ((value ?? "").trim().toLowerCase()) {
    case "true":
      return true;
    case "false":
      return false;
    default:
      return undefined;
  }
}

export function readPineOptionalStringArg(value: string | undefined): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  const literal = readPineLiteral(value);
  return literal === "" ? undefined : literal;
}

export function readPineRawArg(value: string | undefined): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  const normalized = value.trim();
  return normalized === "" ? undefined : normalized;
}

export function defaultIndicatorText(properties: Record<string, unknown>): string {
  const type = properties.indicatorType;
  switch (type) {
    case "movingAverage":
      return `获取 ${properties.movingAverageType ?? "MA"} ${properties.windowSize ?? 20}`;
    case "macd":
      return `获取 MACD ${properties.fastPeriod ?? 12}/${properties.slowPeriod ?? 26}/${properties.signalPeriod ?? 9}`;
    case "bollinger":
      return `获取 Bollinger ${properties.period ?? 20}`;
    case "atr":
      return `获取 ATR ${properties.period ?? 14}`;
    case "cci":
      return `获取 CCI ${properties.period ?? 20}`;
    case "williamsR":
      return `获取 Williams %R ${properties.period ?? 14}`;
    default:
      return `获取 RSI ${properties.period ?? 14}`;
  }
}

export function indicatorTypeForCondition(value: string): string {
  return value === "ma" ? "movingAverage" : value;
}
