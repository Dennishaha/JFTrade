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
  parseStrategyFlowNodeAnnotationLines,
  type StrategyFlowNodeJsDoc,
} from "./strategyVisualBuilderShared";

export interface StrategyScriptParseSuccess {
  ok: true;
  model: StrategyVisualModelDocument;
  codeBlockCount: number;
}

export interface StrategyScriptParseFailure {
  ok: false;
  error: string;
}

export type StrategyScriptParseResult =
  | StrategyScriptParseSuccess
  | StrategyScriptParseFailure;

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
  codeBlockCount: number;
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
): StrategyScriptParseResult {
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
    codeBlockCount: 0,
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
    codeBlockCount: state.codeBlockCount,
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

function parseStatementNode(entry: ParsedPineEntry, state: ParseState): ParsedNodeResult {
  const annotation = entry.annotation;
  const explicitKind = annotation?.blockKind;

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

  state.codeBlockCount += 1;
  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: "codeBlock",
      defaultText: annotation?.nodeText ?? "Pine 片段",
      defaultType: "rect",
      properties: {
        blockKind: "codeBlock",
        code: entry.trimmed,
      },
    }),
    isCondition: false,
  };
}

function parseLetNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult {
  const match = entry.trimmed.match(/^(?:var\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::=|=)\s*(.+)$/);
  if (match === null) {
    state.codeBlockCount += 1;
    return createCodeBlockResult(entry, state);
  }

  const alias = match[1]!;
  const expression = match[2]!.trim();
  const indicatorProperties = parseIndicatorExpression(expression);
  if (indicatorProperties === null) {
    state.codeBlockCount += 1;
    return createCodeBlockResult(entry, state);
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

function parseIfNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult {
  const condition = entry.trimmed.replace(/^if\s+/, "").replace(/:\s*$/, "").trim();
  const closeCondition = parseCloseCondition(condition, explicitKind);
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
    const node = createNodeFromParts({
      state,
      entry,
      kind: explicitKind ?? "technicalIndicatorCondition",
      defaultText: entry.annotation?.nodeText ?? "指标条件判断",
      defaultType: "diamond",
      properties: {
        blockKind: explicitKind ?? "technicalIndicatorCondition",
        ...indicatorCondition.properties,
      },
    });
    for (const input of indicatorCondition.inputs) {
      addDataEdge(state, input.nodeId, node.id, input.slot);
    }
    return { node, isCondition: true };
  }

  state.codeBlockCount += 1;
  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: explicitKind ?? "codeBlock",
      defaultText: entry.annotation?.nodeText ?? "Pine 条件",
      defaultType: defaultTypeForKind(explicitKind ?? "codeBlock"),
      properties: {
        blockKind: explicitKind ?? "codeBlock",
        code: entry.trimmed,
      },
    }),
    isCondition: true,
  };
}

function parseOrderNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult {
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
    state.codeBlockCount += 1;
    return createCodeBlockResult(entry, state);
  }
  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: explicitKind ?? "codeBlock",
      defaultText: entry.annotation?.nodeText ?? "Pine 片段",
      defaultType: "rect",
      properties: {
        blockKind: explicitKind ?? "codeBlock",
        code: entry.trimmed,
      },
    }),
    isCondition: false,
  };
}

function parsePineExitNode(
  entry: ParsedPineEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult {
  const properties = parsePineExit(entry.trimmed);
  if (properties === null) {
    state.codeBlockCount += 1;
    return createCodeBlockResult(entry, state);
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
  if (args.length < 2) {
    return null;
  }
  const fromEntry = readPineLiteral(args[1] ?? "").toLowerCase();
  const direction = fromEntry.includes("short") ? "short" : "long";
  const namedArgs = parseNamedArgs(args.slice(2));
  const stop = namedArgs.get("stop");
  if (stop !== undefined) {
    const percentage = parsePineExitPricePercent(stop);
    return percentage === null ? null : pineExitProperties(direction, "stopLoss", percentage);
  }
  const limit = namedArgs.get("limit");
  if (limit !== undefined) {
    const percentage = parsePineExitPricePercent(limit);
    return percentage === null ? null : pineExitProperties(direction, "takeProfit", percentage);
  }
  const trailPoints = namedArgs.get("trail_points");
  const trailOffset = namedArgs.get("trail_offset");
  if (trailPoints !== undefined && trailOffset !== undefined) {
    const percentage = parsePineExitTrailPercent(trailPoints);
    const offsetPercentage = parsePineExitTrailPercent(trailOffset);
    return percentage === null || offsetPercentage === null || percentage !== offsetPercentage
      ? null
      : pineExitProperties(direction, "trailingStop", percentage);
  }
  return null;
}

function pineExitProperties(
  direction: "long" | "short",
  mode: "stopLoss" | "takeProfit" | "trailingStop",
  percentage: number,
): Record<string, unknown> {
  return {
    direction,
    mode,
    timeValue: 1,
    timeUnit: "bar",
    percentage,
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

  const crossMatch = condition.match(/^(?:cross_(over|under)|ta\.cross(over|under))\(([^,]+),\s*([^\)]+)\)$/);
  if (crossMatch !== null) {
    const direction = crossMatch[1] ?? crossMatch[2];
    const leftAlias = crossMatch[3]!.trim().split(".")[0]!;
    const rightAlias = crossMatch[4]!.trim().split(".")[0]!;
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
        windowSize: readNumber(args[1] ?? args[0], 20),
        periodUnit: "bar",
      };
    case "ta.rma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "SMMA",
        windowSize: readNumber(args[1] ?? args[0], 20),
        periodUnit: "bar",
      };
    case "ta.wma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "LWMA",
        windowSize: readNumber(args[1] ?? args[0], 20),
        periodUnit: "bar",
      };
    case "ta.hma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "HMA",
        windowSize: readNumber(args[1] ?? args[0], 20),
        periodUnit: "bar",
      };
    case "ta.vwma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "VWMA",
        windowSize: readNumber(args[1] ?? args[0], 20),
        periodUnit: "bar",
      };
    case "ta.sma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "SMA",
        windowSize: readNumber(args[1] ?? args[0], 20),
        periodUnit: "bar",
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
      return { blockKind: "getTechnicalIndicator", indicatorType: "cci", period: readNumber(args[1] ?? args[0], 20) };
    default:
      return null;
  }
}

function parseRequestSecurityIndicator(args: string[]): Record<string, unknown> | null {
  if (args.length < 3 || args[0]?.trim() !== "syminfo.tickerid") {
    return null;
  }
  const periodUnit = periodUnitFromPineTimeframe(readPineLiteral(args[1] ?? ""));
  if (periodUnit === null) {
    return null;
  }
  const inner = parseIndicatorExpression(args[2] ?? "");
  if (inner === null || inner.indicatorType !== "movingAverage") {
    return null;
  }
  return {
    ...inner,
    periodUnit,
  };
}

function periodUnitFromPineTimeframe(value: string): string | null {
  switch (value.trim().toUpperCase()) {
    case "1":
      return "minute";
    case "60":
      return "hour";
    case "D":
    case "1D":
      return "day";
    case "W":
    case "1W":
      return "week";
    case "M":
    case "1M":
      return "month";
    default:
      return null;
  }
}

function createCodeBlockResult(entry: ParsedPineEntry, state: ParseState): ParsedNodeResult {
  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: "codeBlock",
      defaultText: entry.annotation?.nodeText ?? "Pine 片段",
      defaultType: "rect",
      properties: {
        blockKind: "codeBlock",
        code: entry.trimmed,
      },
    }),
    isCondition: false,
  };
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
    case "ifCloseAbove":
    case "ifCloseBelow":
      return "diamond";
    default:
      return "rect";
  }
}

function isOrderLine(trimmed: string): boolean {
  return /^strategy\.(entry|close)\s*\(/.test(trimmed);
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
  if (trimmed.startsWith("strategy.close")) {
    const args = splitArguments(readCallArgs(trimmed));
    const id = readPineLiteral(args[0] ?? "");
    return {
      side: id.toLowerCase().includes("short") ? "BUY_COVER" : "SELL",
      orderType: "MARKET",
      entryPositionPolicy: "sameDirection",
      quantityMode: "shares",
      quantityValue: 100,
      limitPrice: 0,
    };
  }
  if (!trimmed.startsWith("strategy.entry")) {
    return null;
  }
  const args = splitArguments(readCallArgs(trimmed));
  const direction = String(args[1] ?? "strategy.long").toLowerCase();
  const namedArgs = parseNamedArgs(args.slice(2));
  if (namedArgs.has("qty_percent")) {
    return null;
  }
  const quantity = parsePineQuantity(namedArgs.get("qty"));
  const limitPrice = readNumber(namedArgs.get("limit"), 0);
  return {
    side: direction.includes("short") ? "SELL_SHORT" : "BUY",
    orderType: limitPrice > 0 ? "LIMIT" : "MARKET",
    entryPositionPolicy: "sameDirection",
    quantityMode: quantity.mode,
    quantityValue: quantity.value,
    limitPrice,
  };
}

function parsePineQuantity(
  qty: string | undefined,
): { mode: "shares" | "amount" | "equityPercent"; value: number } {
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

export const buildStrategyVisualModelFromScript = buildStrategyVisualModelFromPine;
