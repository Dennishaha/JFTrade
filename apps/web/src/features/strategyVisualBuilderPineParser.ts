import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";

import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
  type StrategyVisualEdgeBranch,
} from "./strategyVisualBuilderEdges";
import {
  literalExpression,
  parsePineExpressionToVisualExpression,
  sourceExpression,
} from "./strategyVisualBuilderExpressions";
import {
  parseStrategyFlowNodeAnnotationLines,
  type StrategyFlowNodeJsDoc,
} from "./strategyVisualBuilderShared";

export interface StrategyPineParseSuccess {
  ok: true;
  model: StrategyVisualModelDocument;
}

export interface StrategyPineParseFailure {
  ok: false;
  error: string;
}

export type StrategyPineParseResult =
  | StrategyPineParseSuccess
  | StrategyPineParseFailure;

interface ParsedPineEntry {
  lineNumber: number;
  raw: string;
  trimmed: string;
  indent: number;
  start: number;
  end: number;
  annotation: StrategyFlowNodeJsDoc | null;
  annotationStart: number | null;
}

interface ParseState {
  entries: ParsedPineEntry[];
  index: number;
  nodes: StrategyVisualNodeDocument[];
  edges: StrategyVisualEdgeDocument[];
  nodeIds: Set<string>;
  existingNodeById: Map<string, StrategyVisualNodeDocument>;
  aliasByName: Map<string, IndicatorAliasBinding>;
  sequence: number;
  error: string | null;
}

interface IndicatorAliasBinding {
  alias: string;
  nodeId: string;
  indicatorType: string;
}

interface ParsedNodeResult {
  node: StrategyVisualNodeDocument;
  isCondition: boolean;
}

const ROOT_LAYOUT = {
  onInit: { x: 180, y: 120 },
  onKLineClosed: { x: 180, y: 320 },
};

export function buildStrategyVisualModelFromPine(
  script: string,
  existingModel?: StrategyVisualModelDocument | null,
): StrategyPineParseResult {
  if (hasLegacyFlowBlockAnnotation(script)) {
    return {
      ok: false,
      error: "旧 codeBlock / technicalIndicator 流程图注解不再支持，请用 Pine v6 标准图块重建。",
    };
  }
  const entries = tokenizePine(script);
  if (entries.length === 0) {
    return { ok: false, error: "Pine 代码为空，无法转换回流程图。" };
  }

  const state: ParseState = {
    entries,
    index: 0,
    nodes: [],
    edges: [],
    nodeIds: new Set(),
    existingNodeById: new Map(
      (existingModel?.nodes ?? []).map((node) => [node.id, node] as const),
    ),
    aliasByName: new Map(),
    sequence: 0,
    error: null,
  };

  const root = createSyntheticRoot(state);
  addNode(state, root);

  while (state.index < entries.length) {
    const entry = entries[state.index]!;
    if (isMetadataLine(entry.trimmed)) {
      state.index += 1;
      continue;
    }

    parseBlock(state, -1, root.id);
    if (state.error !== null) {
      return { ok: false, error: state.error };
    }
  }

  if (state.nodes.length === 0) {
    return { ok: false, error: "Pine 策略至少需要一个可映射语句。" };
  }

  return {
    ok: true,
    model: {
      engine: "logic-flow",
      version: 1,
      nodes: state.nodes,
      edges: state.edges,
    },
  };
}

function createSyntheticRoot(state: ParseState): StrategyVisualNodeDocument {
  const existing = state.existingNodeById.get("on-kline-root");
  return {
    id: "on-kline-root",
    type: existing?.type ?? "circle",
    x: existing?.x ?? ROOT_LAYOUT.onKLineClosed.x,
    y: existing?.y ?? ROOT_LAYOUT.onKLineClosed.y,
    text: existing?.text ?? "K 线收盘",
    properties: { blockKind: "onKLineClosed" },
  };
}

function parseBlock(
  state: ParseState,
  parentIndent: number,
  parentNodeId: string,
  branch?: StrategyVisualEdgeBranch,
): void {
  if (state.index >= state.entries.length) {
    return;
  }
  const firstEntry = state.entries[state.index]!;
  if (firstEntry.indent <= parentIndent) {
    return;
  }

  const blockIndent = firstEntry.indent;
  let previousNodeId = parentNodeId;
  let firstStatement = true;

  while (state.index < state.entries.length) {
    const entry = state.entries[state.index]!;
    if (entry.indent <= parentIndent) {
      return;
    }
    if (entry.indent < blockIndent || entry.indent > blockIndent) {
      return;
    }
    if (isElseLine(entry.trimmed)) {
      return;
    }

    const result = parseStatementNode(entry, state);
    if (result === null) {
      return;
    }
    addNode(state, result.node);
    addControlEdge(
      state,
      previousNodeId,
      result.node.id,
      firstStatement ? branch : undefined,
    );
    firstStatement = false;
    state.index += 1;

    if (result.isCondition) {
      parseBlock(state, entry.indent, result.node.id, "true");
      if (
        state.index < state.entries.length &&
        state.entries[state.index]!.indent === entry.indent &&
        isElseLine(state.entries[state.index]!.trimmed)
      ) {
        state.index += 1;
        parseBlock(state, entry.indent, result.node.id, "false");
      }
      previousNodeId = result.node.id;
      continue;
    }

    previousNodeId = result.node.id;
  }
}

function parseStatementNode(entry: ParsedPineEntry, state: ParseState): ParsedNodeResult | null {
  const annotation = entry.annotation;
  const explicitKind = effectiveExplicitKind(annotation?.blockKind);

  const expandedKDJ = parseExpandedKDJNode(entry, state, explicitKind);
  if (expandedKDJ !== null) {
    return expandedKDJ;
  }

  if (isAssignmentLine(entry.trimmed)) {
    return parseLetNode(entry, state, explicitKind);
  }
  if (entry.trimmed.startsWith("log.info(")) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "log",
        defaultText: annotation?.nodeText ?? "输出日志",
        defaultType: defaultTypeForKind(explicitKind ?? "log"),
        properties: {
          blockKind: explicitKind ?? "log",
          message: readMessageCallOrLiteral(entry.trimmed, "log"),
        },
      }),
      isCondition: false,
    };
  }
  if (entry.trimmed.startsWith("alert(")) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "notify",
        defaultText: annotation?.nodeText ?? "发送通知",
        defaultType: defaultTypeForKind(explicitKind ?? "notify"),
        properties: {
          blockKind: explicitKind ?? "notify",
          message: readMessageCallOrLiteral(entry.trimmed, "notify"),
        },
      }),
      isCondition: false,
    };
  }
  if (entry.trimmed.startsWith("if ")) {
    return parseIfNode(entry, state, explicitKind);
  }
  if (isOrderLine(entry.trimmed)) {
    return parseOrderNode(entry, state, explicitKind);
  }
  if (entry.trimmed.startsWith("strategy.exit")) {
    return parsePineExitNode(entry, state, explicitKind);
  }

  return failUnsupportedPineStatement(state, entry);
}

function parseExpandedKDJNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult | null {
  const first = entry.trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)_highest\s*=\s*ta\.highest\(high,\s*(\d+)\)$/);
  if (first === null) {
    return null;
  }
  const base = first[1]!;
  const period = Number(first[2]!);
  const expected = state.entries.slice(state.index, state.index + 8);
  if (expected.length < 8 || expected.some((candidate) => candidate.indent !== entry.indent)) {
    return null;
  }
  const [highest, lowest, rsv, kVar, dVar, kUpdate, dUpdate, jLine] = expected;
  if (
    highest?.trimmed !== `${base}_highest = ta.highest(high, ${period})` ||
    lowest?.trimmed !== `${base}_lowest = ta.lowest(low, ${period})` ||
    rsv?.trimmed !== `${base}_rsv = ${base}_highest == ${base}_lowest ? 50 : ((close - ${base}_lowest) / (${base}_highest - ${base}_lowest)) * 100` ||
    kVar?.trimmed !== `var ${base}_k = 50.0` ||
    dVar?.trimmed !== `var ${base}_d = 50.0` ||
    jLine?.trimmed !== `${base}_j = 3 * ${base}_k - 2 * ${base}_d`
  ) {
    return null;
  }
  const kMatch = kUpdate?.trimmed.match(new RegExp(`^${base}_k\\s*:=\\s*\\(\\((\\d+)\\) \\* nz\\(${base}_k\\[1\\], 50\\) \\+ ${base}_rsv\\) / (\\d+)$`)) ?? null;
  const dMatch = dUpdate?.trimmed.match(new RegExp(`^${base}_d\\s*:=\\s*\\(\\((\\d+)\\) \\* nz\\(${base}_d\\[1\\], 50\\) \\+ ${base}_k\\) / (\\d+)$`)) ?? null;
  if (kMatch === null || dMatch === null) {
    return null;
  }
  const m1 = Number(kMatch[2]!);
  const m2 = Number(dMatch[2]!);
  if (Number(kMatch[1]!) !== m1 - 1 || Number(dMatch[1]!) !== m2 - 1) {
    return null;
  }
  const variableName = entry.annotation?.variableName ?? base;
  const node = createNodeFromParts({
    state,
    entry,
    kind: explicitKind ?? "getTechnicalIndicator",
    defaultText: entry.annotation?.nodeText ?? `KDJ ${period}`,
    defaultType: "rect",
    properties: {
      blockKind: explicitKind ?? "getTechnicalIndicator",
      indicatorType: "kdj",
      period,
      m1,
      m2,
      variableName,
    },
  });
  for (const alias of [base, `${base}_k`, `${base}_d`, `${base}_j`]) {
    state.aliasByName.set(alias, {
      alias,
      nodeId: node.id,
      indicatorType: "kdj",
    });
  }
  state.index += 7;
  return { node, isCondition: false };
}

function parseLetNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult | null {
  const match = entry.trimmed.match(/^(var\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::=|=)\s*(.+)$/);
  if (match === null) {
    return failUnsupportedPineStatement(state, entry);
  }

  const isVarDeclaration = match[1] !== undefined;
  const alias = match[2]!;
  const expression = match[3]!.trim();
  const isReassignment = entry.trimmed.includes(":=");

  const inputProperties = parseStrategyInputExpression(expression);
  if (inputProperties !== null) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "strategyInput",
        defaultText: entry.annotation?.nodeText ?? `参数 ${alias}`,
        defaultType: "rect",
        properties: {
          blockKind: explicitKind ?? "strategyInput",
          ...inputProperties,
          variableName: entry.annotation?.variableName ?? alias,
        },
      }),
      isCondition: false,
    };
  }

  if (isVarDeclaration) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "stateVariable",
        defaultText: entry.annotation?.nodeText ?? `状态 ${alias}`,
        defaultType: "rect",
        properties: {
          blockKind: explicitKind ?? "stateVariable",
          variableName: entry.annotation?.variableName ?? alias,
          ...parseStateInitialValue(expression),
        },
      }),
      isCondition: false,
    };
  }

  if (isReassignment) {
    const expressionAst = parsePineExpressionToVisualExpression(expression);
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "stateUpdate",
        defaultText: entry.annotation?.nodeText ?? `更新状态 ${alias}`,
        defaultType: "rect",
        properties: {
          blockKind: explicitKind ?? "stateUpdate",
          variableName: entry.annotation?.variableName ?? alias,
          expression,
          ...(expressionAst === null ? {} : { expressionAst }),
        },
      }),
      isCondition: false,
    };
  }

  const mtfProperties = parseMtfSeriesExpression(expression);
  if (mtfProperties !== null && (explicitKind === "mtfSeries" || mtfProperties.expressionType !== "indicator")) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "mtfSeries",
        defaultText: entry.annotation?.nodeText ?? `MTF ${alias}`,
        defaultType: "rect",
        properties: {
          blockKind: explicitKind ?? "mtfSeries",
          ...mtfProperties,
          variableName: entry.annotation?.variableName ?? alias,
        },
      }),
      isCondition: false,
    };
  }

  const derivedProperties = parseDerivedSeriesExpression(expression);
  if (derivedProperties !== null) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "derivedSeries",
        defaultText: entry.annotation?.nodeText ?? `派生 ${alias}`,
        defaultType: "rect",
        properties: {
          blockKind: explicitKind ?? "derivedSeries",
          ...derivedProperties,
          variableName: entry.annotation?.variableName ?? alias,
        },
      }),
      isCondition: false,
    };
  }

  const collectionStatProperties = parseCollectionStatExpression(expression);
  if (collectionStatProperties !== null && (explicitKind === undefined || explicitKind === "collectionStat")) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "collectionStat",
        defaultText: entry.annotation?.nodeText ?? `集合统计 ${alias}`,
        defaultType: "rect",
        properties: {
          blockKind: explicitKind ?? "collectionStat",
          ...collectionStatProperties,
          variableName: entry.annotation?.variableName ?? alias,
        },
      }),
      isCondition: false,
    };
  }

  const indicatorProperties = parseIndicatorExpression(expression);
  if (indicatorProperties === null) {
    return failUnsupportedPineStatement(state, entry);
  }

  const node = createNodeFromParts({
    state,
    entry,
    kind: explicitKind ?? "getTechnicalIndicator",
    defaultText: entry.annotation?.nodeText ?? defaultIndicatorText(indicatorProperties),
    defaultType: "rect",
      properties: {
        blockKind: explicitKind ?? "getTechnicalIndicator",
        ...indicatorProperties,
      variableName: entry.annotation?.variableName ?? alias,
    },
  });
  state.aliasByName.set(alias, {
    alias,
    nodeId: node.id,
    indicatorType: String(indicatorProperties.indicatorType),
  });
  return { node, isCondition: false };
}

function effectiveExplicitKind(kind: StrategyBlockKind | undefined): StrategyBlockKind | undefined {
  return kind === "onInit" || kind === "onKLineClosed" ? undefined : kind;
}

function parseIfNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult | null {
  const condition = entry.trimmed.replace(/^if\s+/, "").replace(/:\s*$/, "").trim();
  const timeFilter = parseTimeFilterCondition(condition);
  if (timeFilter !== null && (explicitKind === undefined || explicitKind === "timeFilter")) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "timeFilter",
        defaultText: entry.annotation?.nodeText ?? "时间过滤",
        defaultType: "diamond",
        properties: {
          blockKind: explicitKind ?? "timeFilter",
          ...timeFilter,
        },
      }),
      isCondition: true,
    };
  }

  const sessionFilter = parseSessionFilterCondition(condition);
  if (sessionFilter !== null && (explicitKind === undefined || explicitKind === "sessionFilter")) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "sessionFilter",
        defaultText: entry.annotation?.nodeText ?? "交易时段过滤",
        defaultType: "diamond",
        properties: {
          blockKind: explicitKind ?? "sessionFilter",
          ...sessionFilter,
        },
      }),
      isCondition: true,
    };
  }

  const closeCondition = explicitKind === "seriesCondition"
    ? null
    : parseCloseCondition(condition, explicitKind);
  if (closeCondition !== null) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: closeCondition.kind,
        defaultText: entry.annotation?.nodeText ?? closeCondition.text,
        defaultType: "diamond",
        properties: {
          blockKind: closeCondition.kind,
          threshold: closeCondition.threshold,
        },
      }),
      isCondition: true,
    };
  }

  const indicatorCondition = parseIndicatorCondition(condition, state, entry.annotation);
  if (indicatorCondition !== null) {
    const indicatorConditionKind = explicitKind ?? "technicalIndicatorCondition";
    const node = createNodeFromParts({
      state,
      entry,
      kind: indicatorConditionKind,
      defaultText: entry.annotation?.nodeText ?? "指标条件判断",
      defaultType: "diamond",
      properties: {
        blockKind: indicatorConditionKind,
        ...indicatorCondition.properties,
      },
    });
    for (const input of indicatorCondition.inputs) {
      addDataEdge(state, input.nodeId, node.id, input.slot);
    }
    return { node, isCondition: true };
  }

  const seriesCondition = parseSeriesCondition(condition);
  if (seriesCondition !== null) {
    const kind = explicitKind ?? "seriesCondition";
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind,
        defaultText: entry.annotation?.nodeText ?? "序列条件判断",
        defaultType: "diamond",
        properties: {
          blockKind: kind,
          ...seriesCondition,
        },
      }),
      isCondition: true,
    };
  }

  return failUnsupportedPineStatement(state, entry);
}

function parseSeriesCondition(condition: string): Record<string, unknown> | null {
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

function parseStructuredSeriesCondition(condition: string): Record<string, unknown> | null {
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

function parseTimeFilterCondition(condition: string): Record<string, unknown> | null {
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

function minuteWindowProperties(mode: "after" | "before" | "between", startMinuteOfDay: number, endMinuteOfDay: number): Record<string, unknown> {
  return {
    mode,
    startHour: Math.floor(startMinuteOfDay / 60),
    startMinute: startMinuteOfDay % 60,
    endHour: Math.floor(endMinuteOfDay / 60),
    endMinute: endMinuteOfDay % 60,
    dayOfWeek: 2,
  };
}

function parseSessionFilterCondition(condition: string): Record<string, unknown> | null {
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

function parseStrategyInputExpression(expression: string): Record<string, unknown> | null {
  const call = expression.match(/^input\.(int|float|source|timeframe|time|color)\((.*)\)$/i);
  if (call === null) {
    return null;
  }
  const inputType = call[1]!.toLowerCase();
  const args = splitArguments(call[2] ?? "");
  const namedArgs = parseNamedArgs(args);
  const rawDefault = namedArgs.get("defval") ?? args[0] ?? "";
  const rawTitle = namedArgs.get("title") ?? args[1] ?? "";
  return {
    inputType,
    title: readPineLiteral(rawTitle) || "Input",
    defaultValue: parseInputDefaultValue(inputType, rawDefault),
  };
}

function parseInputDefaultValue(inputType: string, rawValue: string): number | string {
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

function parseDerivedSeriesExpression(expression: string): Record<string, unknown> | null {
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
    const args = splitArguments(mathMatch[2] ?? "");
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
      leftExpressionAst: parsePineExpressionToVisualExpression(arithmeticMatch[1]!) ?? sourceExpression("close"),
      rightExpressionAst: parsePineExpressionToVisualExpression(arithmeticMatch[3]!) ?? sourceExpression("open"),
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

function parseMtfSeriesExpression(expression: string): Record<string, unknown> | null {
  const call = expression.match(/^request\.security\((.*)\)$/i);
  if (call === null) {
    return null;
  }
  const args = splitArguments(call[1] ?? "");
  if (args.length < 3 || args[0]?.trim() !== "syminfo.tickerid") {
    return null;
  }
  const timeframe = readPineLiteral(args[1] ?? "") || "D";
  const inner = args[2]?.trim() ?? "close";
  const innerAst = parsePineExpressionToVisualExpression(inner);
  const historyMatch = inner.match(/^(open|high|low|close|volume|hl2|hlc3|ohlc4)\[(\d+)\]$/i);
  if (historyMatch !== null) {
    return {
      timeframe,
      expressionType: "history",
      source: historyMatch[1]!.toLowerCase(),
      historyOffset: Number(historyMatch[2]!),
      ...(innerAst === null ? {} : { indicatorExpressionAst: innerAst }),
    };
  }
  const sourceMatch = inner.match(/^(open|high|low|close|volume|hl2|hlc3|ohlc4)$/i);
  if (sourceMatch !== null) {
    return {
      timeframe,
      expressionType: "source",
      source: sourceMatch[1]!.toLowerCase(),
      ...(innerAst === null ? {} : { indicatorExpressionAst: innerAst }),
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

function parseCollectionStatExpression(expression: string): Record<string, unknown> | null {
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
  const expressionA = parsePineExpressionToVisualExpression(args[0] ?? "close");
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

function splitIndicatorField(expression: string): { expression: string; field?: string } {
  const fieldMatch = expression.match(/^(.+)\.([A-Za-z_][A-Za-z0-9_]*)$/);
  if (fieldMatch === null) {
    return { expression };
  }
  const baseExpression = fieldMatch[1]?.trim() ?? expression;
  if (parseIndicatorExpression(baseExpression) === null) {
    return { expression };
  }
  const field = fieldMatch[2];
  return field === undefined
    ? { expression: baseExpression }
    : { expression: baseExpression, field };
}

function parseStateInitialValue(expression: string): Record<string, unknown> {
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

function parseOrderNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult | null {
  if (explicitKind !== "placeOrder") {
    const riskRule = parsePineRiskRule(entry.trimmed);
    if (riskRule !== null) {
      return {
        node: createNodeFromParts({
          state,
          entry,
          kind: explicitKind ?? "riskRule",
          defaultText: entry.annotation?.nodeText ?? "策略风控",
          defaultType: "rect",
          properties: {
            blockKind: explicitKind ?? "riskRule",
            ...riskRule,
          },
        }),
        isCondition: false,
      };
    }
  }
  const pineOrder = parsePineOrder(entry.trimmed);
  if (pineOrder !== null) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "placeOrder",
        defaultText: entry.annotation?.nodeText ?? "下单",
        defaultType: "rect",
        properties: {
          blockKind: explicitKind ?? "placeOrder",
          ...pineOrder,
        },
      }),
      isCondition: false,
    };
  }
  if (entry.trimmed.startsWith("strategy.")) {
    return failUnsupportedPineStatement(state, entry);
  }
  return failUnsupportedPineStatement(state, entry);
}

function parsePineExitNode(
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

function parsePineExit(trimmed: string): Record<string, unknown> | null {
  const args = splitArguments(readCallArgs(trimmed));
  if (args.length < 1) {
    return null;
  }
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
      fromEntryMode,
      ...metadata,
      trailingPriceMode: trailPrice === undefined ? "points" : "price",
      ...(trailExpressionAst === null ? {} : { trailingPriceExpressionAst: trailExpressionAst }),
      ...(trailOffsetExpressionAst === null ? {} : { trailingOffsetExpressionAst: trailOffsetExpressionAst }),
    };
  }
  return null;
}

function parsePineExitMetadata(namedArgs: Map<string, string>): Record<string, unknown> {
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

function pineExitProperties(
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

function parsePineExitPricePercent(value: string): number | null {
  const normalized = stripWrappingParens(value).replace(/\s+/g, " ");
  const match = normalized.match(/^close \* \(?1 [+-] (-?\d+(?:\.\d+)?) \/ 100\)?$/i);
  if (match === null) {
    return null;
  }
  const parsed = Number(match[1]);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

function parsePineExitTrailPercent(value: string): number | null {
  const normalized = stripWrappingParens(value).replace(/\s+/g, " ");
  const match = normalized.match(/^close \* (-?\d+(?:\.\d+)?) \/ 100$/i);
  if (match === null) {
    return null;
  }
  const parsed = Number(match[1]);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

function parseIndicatorCondition(
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

function parseCloseCondition(
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

function parseIndicatorExpression(expression: string): Record<string, unknown> | null {
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

function parseRequestSecurityIndicator(args: string[]): Record<string, unknown> | null {
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

function supportsRequestSecurityIndicator(indicatorType: unknown): boolean {
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

function normalizePineTimeframe(value: string): string | null {
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

function failUnsupportedPineStatement(
  state: ParseState,
  entry: ParsedPineEntry,
): null {
  state.error = `第 ${entry.lineNumber} 行无法同步为流程图：${entry.trimmed}`;
  return null;
}

function hasLegacyFlowBlockAnnotation(script: string): boolean {
  return /@jftradeFlowBlockKind\s+(?:codeBlock|technicalIndicator)(?:\s|$)/.test(script);
}

function createNodeFromParts(options: {
  state: ParseState;
  entry: ParsedPineEntry;
  kind: StrategyBlockKind;
  defaultText: string;
  defaultType: StrategyVisualNodeDocument["type"];
  properties: Record<string, unknown>;
  sourceStart?: number;
}): StrategyVisualNodeDocument {
  const { state, entry, kind, defaultText, defaultType, properties } = options;
  const id = ensureUniqueNodeId(
    state,
    entry.annotation?.nodeId ?? `${kind}-${entry.lineNumber}`,
  );
  const existing = state.existingNodeById.get(id);
  const layout = existing ?? nextLayoutNode(state, kind);
  return {
    id,
    type: existing?.type ?? defaultType,
    x: layout.x,
    y: layout.y,
    text: entry.annotation?.nodeText ?? existing?.text ?? defaultText,
    properties: {
      ...properties,
      sourceRange: {
        start: options.sourceStart ?? entry.annotationStart ?? entry.start,
        end: entry.end,
      },
    },
  };
}

function nextLayoutNode(
  state: ParseState,
  kind: StrategyBlockKind,
): Pick<StrategyVisualNodeDocument, "x" | "y"> {
  if (kind === "onInit") {
    return ROOT_LAYOUT.onInit;
  }
  if (kind === "onKLineClosed") {
    return ROOT_LAYOUT.onKLineClosed;
  }
  const index = state.sequence++;
  return {
    x: 440 + (index % 4) * 240,
    y: 120 + Math.floor(index / 4) * 120,
  };
}

function addNode(state: ParseState, node: StrategyVisualNodeDocument): void {
  if (state.nodeIds.has(node.id)) {
    return;
  }
  state.nodes.push(node);
  state.nodeIds.add(node.id);
}

function addControlEdge(
  state: ParseState,
  sourceNodeId: string,
  targetNodeId: string,
  branch?: StrategyVisualEdgeBranch,
): void {
  if (sourceNodeId === targetNodeId || hasEdge(state, sourceNodeId, targetNodeId, branch, "control")) {
    return;
  }
  state.edges.push({
    id: buildEdgeId(sourceNodeId, targetNodeId, branch ?? "control"),
    type: "polyline",
    sourceNodeId,
    targetNodeId,
    properties: buildStrategyVisualControlEdgeProperties(branch),
  });
}

function addDataEdge(
  state: ParseState,
  sourceNodeId: string,
  targetNodeId: string,
  slot: "primary" | "fast" | "slow",
): void {
  if (sourceNodeId === targetNodeId || hasEdge(state, sourceNodeId, targetNodeId, slot, "data")) {
    return;
  }
  state.edges.push({
    id: buildEdgeId(sourceNodeId, targetNodeId, `data-${slot}`),
    type: "polyline",
    sourceNodeId,
    targetNodeId,
    properties: buildStrategyVisualDataEdgeProperties(slot),
  });
}

function hasEdge(
  state: ParseState,
  sourceNodeId: string,
  targetNodeId: string,
  discriminator: string | undefined,
  role: "control" | "data",
): boolean {
  return state.edges.some((edge) => {
    if (edge.sourceNodeId !== sourceNodeId || edge.targetNodeId !== targetNodeId) {
      return false;
    }
    if (role === "data") {
      return edge.properties?.role === "data" && edge.properties.slot === discriminator;
    }
    return (edge.properties?.role ?? "control") !== "data" && (edge.properties?.branch ?? undefined) === discriminator;
  });
}

function buildEdgeId(sourceNodeId: string, targetNodeId: string, suffix: string): string {
  return `edge-${sourceNodeId}-${targetNodeId}-${suffix}`.replace(/[^A-Za-z0-9_-]+/g, "-");
}

function ensureUniqueNodeId(state: ParseState, preferredId: string): string {
  const base = preferredId.trim() === "" ? "pine-node" : preferredId.trim();
  if (!state.nodeIds.has(base)) {
    return base;
  }
  for (let index = 2; ; index += 1) {
    const candidate = `${base}-${index}`;
    if (!state.nodeIds.has(candidate)) {
      return candidate;
    }
  }
}

function tokenizePine(script: string): ParsedPineEntry[] {
  const normalized = script.replace(/\r\n/g, "\n");
  const lines = normalized.split("\n");
  const entries: ParsedPineEntry[] = [];
  let offset = 0;
  let pendingComments: string[] = [];
  let pendingStart: number | null = null;

  for (let index = 0; index < lines.length; index += 1) {
    const raw = lines[index] ?? "";
    const start = offset;
    const end = start + raw.length;
    const trimmed = raw.trim();

    if (trimmed.startsWith("#") || trimmed.startsWith("// @jftradeFlow")) {
      if (pendingComments.length === 0) {
        pendingStart = start;
      }
      pendingComments.push(trimmed);
      offset = end + 1;
      continue;
    }

    if (trimmed.startsWith("//")) {
      offset = end + 1;
      continue;
    }

    if (trimmed !== "") {
      entries.push({
        lineNumber: index + 1,
        raw,
        trimmed,
        indent: raw.length - raw.trimStart().length,
        start,
        end,
        annotation: pendingComments.length > 0
          ? parseStrategyFlowNodeAnnotationLines(pendingComments)
          : null,
        annotationStart: pendingStart,
      });
      pendingComments = [];
      pendingStart = null;
    }

    offset = end + 1;
  }

  return entries;
}

function isMetadataLine(trimmed: string): boolean {
  return /^\/\/@version\s*=/.test(trimmed)
    || /^strategy\s*\(/.test(trimmed);
}

function defaultTypeForKind(kind: StrategyBlockKind): StrategyVisualNodeDocument["type"] {
  switch (kind) {
    case "onInit":
    case "onKLineClosed":
      return "circle";
    case "technicalIndicatorCondition":
    case "seriesCondition":
    case "timeFilter":
    case "sessionFilter":
    case "ifCloseAbove":
    case "ifCloseBelow":
      return "diamond";
    default:
      return "rect";
  }
}

function isOrderLine(trimmed: string): boolean {
  return /^strategy\.(entry|order|close|close_all|cancel|cancel_all)\s*\(/.test(trimmed)
    || /^strategy\.risk\.(allow_entry_in|max_drawdown|max_intraday_loss|max_intraday_filled_orders|max_position_size|max_cons_loss_days)\s*\(/.test(trimmed);
}

function isAssignmentLine(trimmed: string): boolean {
  return /^(?:var\s+)?[A-Za-z_][A-Za-z0-9_]*\s*(?::=|=)\s*/.test(trimmed);
}

function isElseLine(trimmed: string): boolean {
  return trimmed === "else" || trimmed === "else:";
}

function readMessageCallOrLiteral(trimmed: string, kind: "log" | "notify"): string {
  if (trimmed.includes("(")) {
    const args = splitArguments(readCallArgs(trimmed));
    return readPineLiteral(args[0] ?? "");
  }
  return readPineLiteral(kind === "log" ? trimmed.slice(4).trim() : trimmed.slice(7).trim());
}

function parsePineOrder(trimmed: string): Record<string, unknown> | null {
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

function parsePineRiskAllowEntryDirection(value: string | undefined): "all" | "long" | "short" {
  const normalized = (value ?? "").trim().toLowerCase();
  if (normalized.includes("short")) {
    return "short";
  }
  if (normalized.includes("long")) {
    return "long";
  }
  return "all";
}

function parsePineRiskRule(trimmed: string): Record<string, unknown> | null {
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

function parsePineRiskAmountType(value: string | undefined): "strategy.percent_of_equity" | "strategy.cash" {
  return (value ?? "").trim().toLowerCase() === "strategy.cash"
    ? "strategy.cash"
    : "strategy.percent_of_equity";
}

function parsePineQuantity(
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

function stripWrappingParens(value: string): string {
  let result = value.trim();
  while (result.startsWith("(") && result.endsWith(")") && wrappingParensCoverExpression(result)) {
    result = result.slice(1, -1).trim();
  }
  return result;
}

function wrappingParensCoverExpression(value: string): boolean {
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

function parseNamedArgs(args: string[]): Map<string, string> {
  const result = new Map<string, string>();
  for (const arg of args) {
    const [key, ...rest] = arg.split("=");
    if (key !== undefined && rest.length > 0) {
      result.set(key.trim(), rest.join("=").trim());
    }
  }
  return result;
}

function readPositionalArgs(args: string[]): string[] {
  return args.filter((arg) => !isNamedArg(arg));
}

function isNamedArg(value: string): boolean {
  return value.includes("=");
}

function readCallArgs(value: string): string {
  const open = value.indexOf("(");
  const close = value.lastIndexOf(")");
  if (open < 0 || close <= open) {
    return "";
  }
  return value.slice(open + 1, close);
}

function splitArguments(value: string): string[] {
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

function readNumber(value: string | undefined, fallback: number): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function readAnyNumber(value: string | undefined, fallback: number): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) ? parsed : fallback;
}

function readLiteralNumber(value: unknown, fallback: number): number {
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

function readNonNegativeNumber(value: string | undefined, fallback: number): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}

function readSource(
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

function isSeriesSourceLiteral(value: string | undefined): boolean {
  switch ((value ?? "").trim().toLowerCase()) {
    case "open":
    case "high":
    case "low":
    case "close":
    case "volume":
    case "hl2":
    case "hlc3":
    case "ohlc4":
      return true;
    default:
      return false;
  }
}

function readPineLiteral(value: string): string {
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

function readPineBooleanArg(value: string | undefined): boolean | undefined {
  switch ((value ?? "").trim().toLowerCase()) {
    case "true":
      return true;
    case "false":
      return false;
    default:
      return undefined;
  }
}

function readPineOptionalStringArg(value: string | undefined): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  const literal = readPineLiteral(value);
  return literal === "" ? undefined : literal;
}

function readPineRawArg(value: string | undefined): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  const normalized = value.trim();
  return normalized === "" ? undefined : normalized;
}

function defaultIndicatorText(properties: Record<string, unknown>): string {
  const type = properties.indicatorType;
  switch (type) {
    case "movingAverage":
      return `获取 ${properties.movingAverageType ?? "MA"} ${properties.windowSize ?? 20}`;
    case "macd":
      return `获取 MACD ${properties.fastPeriod ?? 12}/${properties.slowPeriod ?? 26}/${properties.signalPeriod ?? 9}`;
    case "kdj":
      return `获取 KDJ ${properties.period ?? 9}`;
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

function indicatorTypeForCondition(value: string): string {
  return value === "ma" ? "movingAverage" : value;
}
