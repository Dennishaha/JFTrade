import {
  literalExpression,
  parsePineExpressionToVisualExpression,
  sourceExpression,
} from "./strategyVisualBuilderExpressions";
import { parseIndicatorExpression } from "./strategyVisualBuilderPineParserIndicators";
import {
  parseNamedArgs,
  readAnyNumber,
  readLiteralNumber,
  readPineLiteral,
  readSource,
  splitArguments,
  stripWrappingParens,
} from "./strategyVisualBuilderPineParserSyntax";

export function parseSeriesCondition(condition: string): Record<string, unknown> | null {
  const structured = parseStructuredSeriesCondition(condition);
  if (structured !== null) {
    return structured;
  }

  const compareMatch = condition.match(/^(open|high|low|close|volume|hl2|hlc3|ohlc4)\s*([<>])\s*(-?\d+(?:\.\d+)?)$/i);
  if (compareMatch !== null) {
    return {
      mode: "compare",
      source: compareMatch[1]!.toLowerCase(),
      operator: compareMatch[2]!,
      threshold: Number(compareMatch[3]!),
      leftExpressionAst: sourceExpression(compareMatch[1]!.toLowerCase()),
      rightExpressionAst: literalExpression(Number(compareMatch[3]!)),
    };
  }

  const trendMatch = condition.match(/^ta\.(rising|falling)\((open|high|low|close|volume|hl2|hlc3|ohlc4),\s*(\d+)\)$/i);
  if (trendMatch !== null) {
    return {
      mode: trendMatch[1]!.toLowerCase(),
      source: trendMatch[2]!.toLowerCase(),
      length: Number(trendMatch[3]!),
      sourceExpressionAst: sourceExpression(trendMatch[2]!.toLowerCase()),
    };
  }

  const barssinceMatch = condition.match(/^ta\.barssince\((open|high|low|close|volume|hl2|hlc3|ohlc4)\s*([<>])\s*(-?\d+(?:\.\d+)?)\)\s*([<>])\s*(\d+)$/i);
  if (barssinceMatch !== null) {
    const operator = barssinceMatch[4] === ">" ? ">" : "<";
    return {
      mode: "barssince",
      eventSource: barssinceMatch[1]!.toLowerCase(),
      eventOperator: barssinceMatch[2]!,
      eventThreshold: Number(barssinceMatch[3]!),
      length: Number(barssinceMatch[5]!),
      operator,
      eventExpressionAst: {
        kind: "binary",
        left: sourceExpression(barssinceMatch[1]!.toLowerCase()),
        operator: barssinceMatch[2] as ">" | "<",
        right: literalExpression(Number(barssinceMatch[3]!)),
      },
    };
  }

  const valuewhenMatch = condition.match(/^ta\.valuewhen\((open|high|low|close|volume|hl2|hlc3|ohlc4)\s*([<>])\s*(-?\d+(?:\.\d+)?),\s*(open|high|low|close|volume|hl2|hlc3|ohlc4),\s*(\d+)\)\s*([<>])\s*(-?\d+(?:\.\d+)?)$/i);
  if (valuewhenMatch !== null) {
    return {
      mode: "valuewhen",
      eventSource: valuewhenMatch[1]!.toLowerCase(),
      eventOperator: valuewhenMatch[2]!,
      eventThreshold: Number(valuewhenMatch[3]!),
      valueSource: valuewhenMatch[4]!.toLowerCase(),
      occurrence: Number(valuewhenMatch[5]!),
      operator: valuewhenMatch[6]!,
      threshold: Number(valuewhenMatch[7]!),
      eventExpressionAst: {
        kind: "binary",
        left: sourceExpression(valuewhenMatch[1]!.toLowerCase()),
        operator: valuewhenMatch[2] as ">" | "<",
        right: literalExpression(Number(valuewhenMatch[3]!)),
      },
      valueExpressionAst: sourceExpression(valuewhenMatch[4]!.toLowerCase()),
      rightExpressionAst: literalExpression(Number(valuewhenMatch[7]!)),
    };
  }

  return null;
}

export function parseStructuredSeriesCondition(condition: string): Record<string, unknown> | null {
  const expression = parsePineExpressionToVisualExpression(condition);
  if (expression?.kind !== "binary") {
    return null;
  }

  if (expression.left.kind === "call" && expression.left.functionName === "ta.barssince") {
    return {
      mode: "barssince",
      operator: expression.operator === ">" ? ">" : "<",
      length: readLiteralNumber(expression.right, 3),
      eventExpressionAst: expression.left.args[0] ?? sourceExpression("close"),
    };
  }

  if (expression.left.kind === "call" && expression.left.functionName === "ta.valuewhen") {
    return {
      mode: "valuewhen",
      operator: expression.operator === ">" ? ">" : "<",
      threshold: readLiteralNumber(expression.right, 0),
      eventExpressionAst: expression.left.args[0] ?? sourceExpression("close"),
      valueExpressionAst: expression.left.args[1] ?? sourceExpression("close"),
      occurrence: readLiteralNumber(expression.left.args[2], 0),
      rightExpressionAst: expression.right,
    };
  }

  if ([">", "<", ">=", "<=", "==", "!="].includes(expression.operator)) {
    return {
      mode: "compare",
      operator: expression.operator === "<" ? "<" : ">",
      threshold: readLiteralNumber(expression.right, 0),
      leftExpressionAst: expression.left,
      rightExpressionAst: expression.right,
    };
  }

  return null;
}

export function parseTimeFilterCondition(condition: string): Record<string, unknown> | null {
  const normalized = condition.replace(/\s+/g, " ").trim();
  const dayMatch = normalized.match(/^dayofweek\s*==\s*([1-7])$/i);
  if (dayMatch !== null) {
    return {
      mode: "dayOfWeek",
      dayOfWeek: Number(dayMatch[1]),
      startHour: 9,
      startMinute: 30,
      endHour: 16,
      endMinute: 0,
    };
  }

  const minuteExpression = "\\(?\\s*hour\\s*\\*\\s*60\\s*\\+\\s*minute\\s*\\)?";
  const betweenMatch = normalized.match(new RegExp(`^${minuteExpression}\\s*>=\\s*(\\d+)\\s+and\\s+${minuteExpression}\\s*<\\s*(\\d+)$`, "i"));
  if (betweenMatch !== null) {
    return minuteWindowProperties("between", Number(betweenMatch[1]), Number(betweenMatch[2]));
  }
  const afterMatch = normalized.match(new RegExp(`^${minuteExpression}\\s*>=\\s*(\\d+)$`, "i"));
  if (afterMatch !== null) {
    return minuteWindowProperties("after", Number(afterMatch[1]), 960);
  }
  const beforeMatch = normalized.match(new RegExp(`^${minuteExpression}\\s*<\\s*(\\d+)$`, "i"));
  if (beforeMatch !== null) {
    return minuteWindowProperties("before", 570, Number(beforeMatch[1]));
  }
  return null;
}

export function minuteWindowProperties(mode: "after" | "before" | "between", startMinuteOfDay: number, endMinuteOfDay: number): Record<string, unknown> {
  return {
    mode,
    startHour: Math.floor(startMinuteOfDay / 60),
    startMinute: startMinuteOfDay % 60,
    endHour: Math.floor(endMinuteOfDay / 60),
    endMinute: endMinuteOfDay % 60,
    dayOfWeek: 2,
  };
}

export function parseSessionFilterCondition(condition: string): Record<string, unknown> | null {
  const normalized = condition.replace(/\s+/g, "").toLowerCase();
  switch (normalized) {
    case "session.ismarket":
    case "session_ismarket":
      return { scope: "market" };
    case "session.ispremarket":
    case "session_ispremarket":
      return { scope: "premarket" };
    case "session.ispostmarket":
    case "session_ispostmarket":
      return { scope: "postmarket" };
    default:
      return null;
  }
}

export function parseStrategyInputExpression(expression: string): Record<string, unknown> | null {
  const call = expression.match(/^input\.(int|float|source|timeframe|time|color)\((.*)\)$/i);
  if (call === null) {
    return null;
  }
  const inputType = call[1]!.toLowerCase();
  const args = splitArguments(call[2]!);
  const namedArgs = parseNamedArgs(args);
  const rawDefault = namedArgs.get("defval") ?? args[0] ?? "";
  const rawTitle = namedArgs.get("title") ?? args[1] ?? "";
  return {
    inputType,
    title: readPineLiteral(rawTitle) || "Input",
    defaultValue: parseInputDefaultValue(inputType, rawDefault),
  };
}

export function parseInputDefaultValue(inputType: string, rawValue: string): number | string {
  switch (inputType) {
    case "int":
      return Math.round(readAnyNumber(rawValue, 20));
    case "float":
      return readAnyNumber(rawValue, 2);
    case "source":
    case "timeframe":
      return readPineLiteral(rawValue) || rawValue.trim() || (inputType === "source" ? "close" : "D");
    case "time":
    case "color":
    default:
      return rawValue.trim() || (inputType === "color" ? "color.green" : "timestamp(2026, 1, 1)");
  }
}

export function parseDerivedSeriesExpression(expression: string): Record<string, unknown> | null {
  const historyMatch = expression.match(/^(open|high|low|close|volume|hl2|hlc3|ohlc4)\[(\d+)\]$/i);
  if (historyMatch !== null) {
    return {
      mode: "history",
      source: historyMatch[1]!.toLowerCase(),
      historyOffset: Number(historyMatch[2]!),
      sourceExpressionAst: sourceExpression(historyMatch[1]!.toLowerCase()),
    };
  }

  const nzMatch = expression.match(/^nz\((open|high|low|close|volume|hl2|hlc3|ohlc4)(?:,\s*(-?\d+(?:\.\d+)?))?\)$/i);
  if (nzMatch !== null) {
    return {
      mode: "nz",
      source: nzMatch[1]!.toLowerCase(),
      fallbackValue: readAnyNumber(nzMatch[2], 0),
      sourceExpressionAst: sourceExpression(nzMatch[1]!.toLowerCase()),
      fallbackExpressionAst: literalExpression(readAnyNumber(nzMatch[2], 0)),
    };
  }

  const mathMatch = expression.match(/^math\.(min|max|abs|round|floor|ceil)\((.*)\)$/i);
  if (mathMatch !== null) {
    const args = splitArguments(mathMatch[2]!);
    return {
      mode: "math",
      mathFunction: mathMatch[1]!.toLowerCase(),
      leftExpression: args[0] ?? "close",
      rightExpression: args[1] ?? "open",
      leftExpressionAst: parsePineExpressionToVisualExpression(args[0] ?? "close") ?? sourceExpression("close"),
      rightExpressionAst: parsePineExpressionToVisualExpression(args[1] ?? "open") ?? sourceExpression("open"),
    };
  }

  const arithmeticMatch = stripWrappingParens(expression).match(/^([A-Za-z_][A-Za-z0-9_.]*(?:\[\d+\])?|-?\d+(?:\.\d+)?)\s*([+\-*/])\s*([A-Za-z_][A-Za-z0-9_.]*(?:\[\d+\])?|-?\d+(?:\.\d+)?)$/);
  if (arithmeticMatch !== null) {
    return {
      mode: "arithmetic",
      leftExpression: arithmeticMatch[1]!,
      operator: arithmeticMatch[2]!,
      rightExpression: arithmeticMatch[3]!,
      leftExpressionAst: parsePineExpressionToVisualExpression(arithmeticMatch[1]!)!,
      rightExpressionAst: parsePineExpressionToVisualExpression(arithmeticMatch[3]!)!,
    };
  }

  const crossMatch = expression.match(/^ta\.(crossover|crossunder|cross)\(([^,]+),\s*([^)]+)\)$/i);
  if (crossMatch !== null) {
    return {
      mode: "cross",
      crossFunction: crossMatch[1]!.toLowerCase(),
      leftExpression: crossMatch[2]!.trim(),
      rightExpression: crossMatch[3]!.trim(),
      leftExpressionAst: parsePineExpressionToVisualExpression(crossMatch[2]!.trim()) ?? sourceExpression("close"),
      rightExpressionAst: parsePineExpressionToVisualExpression(crossMatch[3]!.trim()) ?? sourceExpression("open"),
    };
  }

  return null;
}

export function parseMtfSeriesExpression(expression: string): Record<string, unknown> | null {
  const call = expression.match(/^request\.security\((.*)\)$/i);
  if (call === null) {
    return null;
  }
  const args = splitArguments(call[1]!);
  if (args.length < 3 || args[0]?.trim() !== "syminfo.tickerid") {
    return null;
  }
  const timeframe = readPineLiteral(args[1]!) || "D";
  const inner = args[2]!.trim();
  const innerAst = parsePineExpressionToVisualExpression(inner);
  const historyMatch = inner.match(/^(open|high|low|close|volume|hl2|hlc3|ohlc4)\[(\d+)\]$/i);
  if (historyMatch !== null) {
    return {
      timeframe,
      expressionType: "history",
      source: historyMatch[1]!.toLowerCase(),
      historyOffset: Number(historyMatch[2]!),
      indicatorExpressionAst: innerAst!,
    };
  }
  const sourceMatch = inner.match(/^(open|high|low|close|volume|hl2|hlc3|ohlc4)$/i);
  if (sourceMatch !== null) {
    return {
      timeframe,
      expressionType: "source",
      source: sourceMatch[1]!.toLowerCase(),
      indicatorExpressionAst: innerAst!,
    };
  }
  const indicatorWithField = splitIndicatorField(inner);
  if (parseIndicatorExpression(indicatorWithField.expression) !== null) {
    const indicatorExpressionAst = parsePineExpressionToVisualExpression(indicatorWithField.expression);
    return {
      timeframe,
      expressionType: "indicator",
      indicatorExpression: indicatorWithField.expression,
      ...(indicatorExpressionAst === null ? {} : { indicatorExpressionAst }),
      ...(indicatorWithField.field === undefined ? {} : { mtfField: indicatorWithField.field }),
    };
  }
  return null;
}

export function parseCollectionStatExpression(expression: string): Record<string, unknown> | null {
  const statMatch = expression.match(/^array\.from\((.*)\)\.(min|max|avg|sum|median|stdev|variance)\(\)$/i);
  const percentileMatch = expression.match(/^array\.from\((.*)\)\.percentile_linear_interpolation\(([^)]*)\)$/i);
  const argsExpression = statMatch?.[1] ?? percentileMatch?.[1];
  if (argsExpression === undefined) {
    return null;
  }
  const args = splitArguments(argsExpression);
  if (args.length < 1 || args.length > 3) {
    return null;
  }
  const expressionA = parsePineExpressionToVisualExpression(args[0]!);
  const expressionB = parsePineExpressionToVisualExpression(args[1] ?? "open");
  const expressionC = parsePineExpressionToVisualExpression(args[2] ?? "high");
  if (expressionA === null || expressionB === null || expressionC === null) {
    return null;
  }
  return {
    statFunction: percentileMatch === null ? statMatch?.[2]?.toLowerCase() : "percentile",
    sourceA: readSource(args[0], "close"),
    sourceB: readSource(args[1], "open"),
    sourceC: readSource(args[2], "high"),
    sourceAExpressionAst: expressionA,
    sourceBExpressionAst: expressionB,
    sourceCExpressionAst: expressionC,
    ...(percentileMatch === null ? {} : { percentile: readAnyNumber(percentileMatch[2], 50) }),
  };
}

export function splitIndicatorField(expression: string): { expression: string; field?: string } {
  const fieldMatch = expression.match(/^(.+)\.([A-Za-z_][A-Za-z0-9_]*)$/);
  if (fieldMatch === null) {
    return { expression };
  }
  const baseExpression = fieldMatch[1]!.trim();
  if (parseIndicatorExpression(baseExpression) === null) {
    return { expression };
  }
  const field = fieldMatch[2]!;
  return { expression: baseExpression, field };
}

export function parseStateInitialValue(expression: string): Record<string, unknown> {
  const trimmed = expression.trim();
  if (trimmed === "true" || trimmed === "false") {
    return { valueType: "bool", initialValue: trimmed === "true" };
  }
  const numberValue = Number(trimmed);
  if (Number.isFinite(numberValue)) {
    return { valueType: "number", initialValue: numberValue };
  }
  return { valueType: "string", initialValue: readPineLiteral(trimmed) };
}
