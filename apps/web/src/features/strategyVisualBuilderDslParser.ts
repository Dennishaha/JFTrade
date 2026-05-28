import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";

import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
  type StrategyVisualEdgeBranch,
} from "./strategyVisualBuilderEdges";
import {
  parseStrategyFlowNodeDslCommentLines,
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

interface ParsedDslEntry {
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
  entries: ParsedDslEntry[];
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

export function buildStrategyVisualModelFromDsl(
  script: string,
  existingModel?: StrategyVisualModelDocument | null,
): StrategyScriptParseResult {
  const entries = tokenizeDsl(script);
  if (entries.length === 0) {
    return { ok: false, error: "DSL 代码为空，无法转换回流程图。" };
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

  while (state.index < entries.length) {
    const entry = entries[state.index]!;
    if (entry.indent > 0) {
      return {
        ok: false,
        error: `DSL 第 ${entry.lineNumber} 行缩进语句必须位于 hook 内。`,
      };
    }

    if (isMetadataLine(entry.trimmed)) {
      state.index += 1;
      continue;
    }

    if (entry.trimmed === "on init:" || entry.trimmed === "on kline_close:") {
      parseHook(entry, state);
      continue;
    }

    return {
      ok: false,
      error: `DSL 第 ${entry.lineNumber} 行暂不支持顶层语句：${entry.trimmed}`,
    };
  }

  if (state.nodes.length === 0) {
    return { ok: false, error: "DSL 策略至少需要一个 hook。" };
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

function parseHook(entry: ParsedDslEntry, state: ParseState): void {
  const kind = entry.trimmed === "on init:" ? "onInit" : "onKLineClosed";
  const defaultText = kind === "onInit" ? "策略启动" : "K 线收盘";
  const node = createNodeFromParts({
    state,
    entry,
    kind,
    defaultText,
    defaultType: "circle",
    properties: { blockKind: kind },
    sourceStart: entry.annotationStart ?? entry.start,
  });
  addNode(state, node);
  state.index += 1;
  parseBlock(state, entry.indent, node.id);
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
    if (entry.trimmed === "else:") {
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
        state.entries[state.index]!.trimmed === "else:"
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

function parseStatementNode(entry: ParsedDslEntry, state: ParseState): ParsedNodeResult {
  const annotation = entry.annotation;
  const explicitKind = annotation?.blockKind;

  if (entry.trimmed.startsWith("let ")) {
    return parseLetNode(entry, state, explicitKind);
  }
  if (entry.trimmed.startsWith("log ")) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "log",
        defaultText: annotation?.nodeText ?? "输出日志",
        defaultType: defaultTypeForKind(explicitKind ?? "log"),
        properties: {
          blockKind: explicitKind ?? "log",
          message: readDslLiteral(entry.trimmed.slice(4).trim()),
        },
      }),
      isCondition: false,
    };
  }
  if (entry.trimmed.startsWith("notify ")) {
    return {
      node: createNodeFromParts({
        state,
        entry,
        kind: explicitKind ?? "notify",
        defaultText: annotation?.nodeText ?? "发送通知",
        defaultType: defaultTypeForKind(explicitKind ?? "notify"),
        properties: {
          blockKind: explicitKind ?? "notify",
          message: readDslLiteral(entry.trimmed.slice(7).trim()),
        },
      }),
      isCondition: false,
    };
  }
  if (entry.trimmed.startsWith("if ") && entry.trimmed.endsWith(":")) {
    return parseIfNode(entry, state, explicitKind);
  }
  if (isOrderLine(entry.trimmed)) {
    return parseOrderNode(entry, state, explicitKind);
  }
  if (entry.trimmed.startsWith("protect ")) {
    return parseProtectNode(entry, state, explicitKind);
  }

  state.codeBlockCount += 1;
  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: "codeBlock",
      defaultText: annotation?.nodeText ?? "DSL 片段",
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
  entry: ParsedDslEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult {
  const match = entry.trimmed.match(/^let\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+)$/);
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
  entry: ParsedDslEntry,
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
      defaultText: entry.annotation?.nodeText ?? "DSL 条件",
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
  entry: ParsedDslEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult {
  const parts = entry.trimmed.split(/\s+/);
  const action = parts[0] ?? "buy";
  const quantityMode = parts[1] ?? "shares";
  const quantityValue = Number(parts[2] ?? "100");
  const options = parseOptionPairs(parts.slice(3));
  const side = sideFromAction(action);
  const orderType = String(options.get("type") ?? (options.has("limit") ? "LIMIT" : "MARKET")).toUpperCase() === "LIMIT"
    ? "LIMIT"
    : "MARKET";
  const limitPrice = Number(options.get("limit") ?? 0);

  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: explicitKind ?? "placeOrder",
      defaultText: entry.annotation?.nodeText ?? "下单",
      defaultType: "rect",
      properties: {
        blockKind: explicitKind ?? "placeOrder",
        side,
        orderType,
        entryPositionPolicy: entryPolicyFromDsl(options.get("policy")),
        quantityMode: quantityModeFromDsl(quantityMode),
        quantityValue: Number.isFinite(quantityValue) ? quantityValue : 100,
        limitPrice: Number.isFinite(limitPrice) ? limitPrice : 0,
      },
    }),
    isCondition: false,
  };
}

function parseProtectNode(
  entry: ParsedDslEntry,
  state: ParseState,
  explicitKind: StrategyBlockKind | undefined,
): ParsedNodeResult {
  const parts = entry.trimmed.split(/\s+/);
  const options = parseOptionPairs(parts.slice(6));
  const timeValue = Number(parts[3] ?? "1");
  const percentage = Number(String(parts[5] ?? "2").replace(/%$/, ""));

  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: explicitKind ?? "stopLoss",
      defaultText: entry.annotation?.nodeText ?? "止损",
      defaultType: "rect",
      properties: {
        blockKind: explicitKind ?? "stopLoss",
        direction: protectDirectionFromDsl(parts[1]),
        mode: protectModeFromDsl(parts[2]),
        timeValue: Number.isFinite(timeValue) ? timeValue : 1,
        timeUnit: protectTimeUnitFromDsl(parts[4]),
        percentage: Number.isFinite(percentage) ? percentage : 2,
        windowPolicy: options.get("window") === "session" ? "session" : "continuous",
      },
    }),
    isCondition: false,
  };
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

  const crossMatch = condition.match(/^cross_(over|under)\(([^,]+),\s*([^\)]+)\)$/);
  if (crossMatch !== null) {
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
        patternType: crossMatch[1] === "under" ? "deathCross" : "goldenCross",
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
  const call = expression.match(/^([A-Za-z_][A-Za-z0-9_]*)\((.*)\)$/);
  if (call === null) {
    return null;
  }
  const functionName = call[1]!.toLowerCase();
  const args = splitArguments(call[2] ?? "");

  switch (functionName) {
    case "ma":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: String(args[0] ?? "MA").toUpperCase(),
        windowSize: readNumber(args[1], 20),
        periodUnit: periodUnitFromDsl(args[2]),
      };
    case "rsi":
      return { blockKind: "getTechnicalIndicator", indicatorType: "rsi", period: readNumber(args[0], 14) };
    case "macd":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "macd",
        fastPeriod: readNumber(args[0], 12),
        slowPeriod: readNumber(args[1], 26),
        signalPeriod: readNumber(args[2], 9),
      };
    case "kdj":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "kdj",
        period: readNumber(args[0], 9),
        m1: readNumber(args[1], 3),
        m2: readNumber(args[2], 3),
      };
    case "bollinger":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "bollinger",
        period: readNumber(args[0], 20),
        multiplier: readNumber(args[1], 2),
      };
    case "atr":
      return { blockKind: "getTechnicalIndicator", indicatorType: "atr", period: readNumber(args[0], 14) };
    case "cci":
      return { blockKind: "getTechnicalIndicator", indicatorType: "cci", period: readNumber(args[0], 20) };
    case "williams_r":
    case "williamsr":
      return { blockKind: "getTechnicalIndicator", indicatorType: "williamsR", period: readNumber(args[0], 14) };
    default:
      return null;
  }
}

function createCodeBlockResult(entry: ParsedDslEntry, state: ParseState): ParsedNodeResult {
  return {
    node: createNodeFromParts({
      state,
      entry,
      kind: "codeBlock",
      defaultText: entry.annotation?.nodeText ?? "DSL 片段",
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
  entry: ParsedDslEntry;
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
  const base = preferredId.trim() === "" ? "dsl-node" : preferredId.trim();
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

function tokenizeDsl(script: string): ParsedDslEntry[] {
  const normalized = script.replace(/\r\n/g, "\n");
  const lines = normalized.split("\n");
  const entries: ParsedDslEntry[] = [];
  let offset = 0;
  let pendingComments: string[] = [];
  let pendingStart: number | null = null;

  for (let index = 0; index < lines.length; index += 1) {
    const raw = lines[index] ?? "";
    const start = offset;
    const end = start + raw.length;
    const trimmed = raw.trim();

    if (trimmed.startsWith("#")) {
      if (pendingComments.length === 0) {
        pendingStart = start;
      }
      pendingComments.push(trimmed);
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
          ? parseStrategyFlowNodeDslCommentLines(pendingComments)
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
  return /^(strategy|version|symbol|interval)\s+/.test(trimmed);
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
  return /^(buy|sell|short|cover)\s+/.test(trimmed);
}

function parseOptionPairs(parts: string[]): Map<string, string> {
  const result = new Map<string, string>();
  for (let index = 0; index + 1 < parts.length; index += 2) {
    result.set(parts[index]!, parts[index + 1]!);
  }
  return result;
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

function readDslLiteral(value: string): string {
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

function periodUnitFromDsl(value: string | undefined): string {
  switch ((value ?? "bar").toLowerCase()) {
    case "minute":
    case "hour":
    case "day":
    case "week":
    case "month":
      return value!.toLowerCase();
    default:
      return "bar";
  }
}

function sideFromAction(action: string): string {
  switch (action) {
    case "sell":
      return "SELL";
    case "short":
      return "SELL_SHORT";
    case "cover":
      return "BUY_COVER";
    default:
      return "BUY";
  }
}

function quantityModeFromDsl(value: string): string {
  switch (value) {
    case "account_position_percent":
      return "accountPositionPercent";
    case "symbol_position_percent":
    case "position_percent":
      return "symbolPositionPercent";
    case "cash_percent":
      return "cashPercent";
    case "margin_buying_power_percent":
      return "marginBuyingPowerPercent";
    case "short_selling_power_percent":
      return "shortSellingPowerPercent";
    default:
      return value;
  }
}

function entryPolicyFromDsl(value: string | undefined): string {
  switch (value) {
    case "flat_only":
    case "flatOnly":
      return "flatOnly";
    case "allow":
      return "allow";
    default:
      return "sameDirection";
  }
}

function protectDirectionFromDsl(value: string | undefined): string {
  return value === "long" || value === "short" ? value : "auto";
}

function protectModeFromDsl(value: string | undefined): string {
  switch (value) {
    case "takeProfit":
    case "take_profit":
      return "takeProfit";
    case "trailingStop":
    case "trailing_stop":
      return "trailingStop";
    default:
      return "stopLoss";
  }
}

function protectTimeUnitFromDsl(value: string | undefined): string {
  switch (value) {
    case "minute":
    case "hour":
    case "day":
    case "week":
    case "month":
      return value;
    default:
      return "day";
  }
}

export const buildStrategyVisualModelFromScript = buildStrategyVisualModelFromDsl;
