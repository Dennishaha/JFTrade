import type { StrategyVisualModelDocument, StrategyVisualNodeDocument } from "@/types";

import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import type { StrategyVisualEdgeBranch } from "./strategyVisualBuilderEdges";
import { parsePineExpressionToVisualExpression } from "./strategyVisualBuilderExpressions";
import {
  parseCollectionStatExpression,
  parseDerivedSeriesExpression,
  parseMtfSeriesExpression,
  parseSeriesCondition,
  parseSessionFilterCondition,
  parseStateInitialValue,
  parseStrategyInputExpression,
  parseTimeFilterCondition,
} from "./strategyVisualBuilderPineParserExpressions";
import {
  parseCloseCondition,
  parseIndicatorCondition,
  parseIndicatorExpression,
  parsePineExitNode,
  parsePineExit,
} from "./strategyVisualBuilderPineParserIndicators";
import {
  addControlEdge,
  addDataEdge,
  addNode,
  createNodeFromParts,
  failUnsupportedPineStatement,
  hasLegacyFlowBlockAnnotation,
} from "./strategyVisualBuilderPineParserModel";
import {
  defaultIndicatorText,
  parsePineOrder,
  parsePineRiskRule,
  readMessageCallOrLiteral,
} from "./strategyVisualBuilderPineParserSyntax";
import { tokenizePine } from "./strategyVisualBuilderPineParserTokenizer";
import type {
  ParseState,
  ParsedNodeResult,
  ParsedPineEntry,
} from "./strategyVisualBuilderPineParserTypes";

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
      state.error = `第 ${entry.lineNumber} 行无法同步为流程图：缺少对应的 if 条件`;
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
  return failUnsupportedPineStatement(state, entry);
}

function isMetadataLine(trimmed: string): boolean {
  return /^\/\/@version\s*=/.test(trimmed)
    || /^strategy\s*\(/.test(trimmed);
}

function defaultTypeForKind(kind: StrategyBlockKind): StrategyVisualNodeDocument["type"] {
  switch (kind) {
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
