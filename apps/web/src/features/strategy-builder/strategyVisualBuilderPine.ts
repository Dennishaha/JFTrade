import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/types";

import {
  getStrategyBlockKind,
  normalizeCollectionStatBlockProperties,
  normalizeDerivedSeriesBlockProperties,
  normalizeMtfSeriesBlockProperties,
  normalizeRiskRuleBlockProperties,
  normalizeSeriesConditionBlockProperties,
  normalizeSessionFilterBlockProperties,
  normalizeStateUpdateBlockProperties,
  normalizeStateVariableBlockProperties,
  normalizeStopLossBlockProperties,
  normalizeStrategyInputBlockProperties,
  normalizeTimeFilterBlockProperties,
  type StrategySeriesSource,
} from "./strategyVisualBuilderCatalog";
import { renderVisualExpressionToPine } from "./strategyVisualBuilderExpressions";
import {
  buildCollectionStatStatement,
  buildDerivedSeriesExpression,
  buildDerivedSeriesStatement,
  buildMtfInnerExpression,
  buildMtfSeriesStatement,
  buildSeriesConditionExpression,
  buildSessionFilterExpression,
  buildStateUpdateStatement,
  buildStateVariableStatement,
  buildStrategyInputExpression,
  buildTimeFilterExpression,
} from "./strategyVisualBuilderPineStatements";
import {
  buildIndicatorExpression,
  buildKDJIndicatorStatements,
  readIndicatorVariableName,
  readSyntheticKDJVariableName,
} from "./strategyVisualBuilderPineIndicatorExpressions";
import {
  buildOrderStatement,
  buildProtectStatements,
  buildRiskRuleStatement,
} from "./strategyVisualBuilderPineOrders";
import {
  formatNumber,
  formatPineValue,
  indent,
  isPineIdentifier,
  sanitizeMetadataValue,
  sanitizePineIdentifier,
  toPineStringLiteral,
} from "./strategyVisualBuilderPineFormat";
import {
  isStrategyVisualControlEdge,
  isStrategyVisualDataEdge,
  readStrategyVisualEdgeBranch,
  readStrategyVisualEdgeInputSlot,
  type StrategyVisualEdgeBranch,
} from "./strategyVisualBuilderEdges";
import {
  normalizeGetTechnicalIndicatorProperties,
  normalizeTechnicalIndicatorConditionProperties,
  type GetTechnicalIndicatorBlockProperties,
  type TechnicalIndicatorConditionBlockProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import { reconcileStrategyVisualModelIndicatorBindings } from "./strategyVisualBuilderIndicatorReferences";
import {
  entryPositionPolicyToSnakeCase,
  normalizeDecimal,
  normalizeEntryPositionPolicy,
  normalizeMessage,
  normalizeOrderSide,
  normalizeOrderType,
  normalizePineOrderAction,
  normalizePineRiskAllowEntryDirection,
  normalizeQuantityModeForSide,
  normalizeThreshold,
} from "./strategyVisualBuilderScriptSupport";
import {
  buildStrategyFlowNodeAnnotation,
  cloneStrategyVisualModel,
  type StrategyFlowNodeJsDoc,
} from "./strategyVisualBuilderShared";

export interface StrategyPineContext {
  name: string;
  version?: string;
}
interface IndicatorInputBinding {
  node: StrategyVisualNodeDocument;
  slot: "primary" | "fast" | "slow";
  properties: GetTechnicalIndicatorBlockProperties;
}

interface RenderState {
  nodeById: Map<string, StrategyVisualNodeDocument>;
  outgoingById: Map<string, StrategyVisualEdgeDocument[]>;
  incomingById: Map<string, StrategyVisualEdgeDocument[]>;
  emittedIndicatorNodeIds: Set<string>;
  emittedStatementNodeIds: Set<string>;
}

export function buildStrategyPineFromVisualModel(
  model: StrategyVisualModelDocument | null | undefined,
  context: StrategyPineContext,
): string {
  const sourceModel = reconcileStrategyVisualModelIndicatorBindings(
    cloneStrategyVisualModel(model) ?? createEmptyVisualModel(),
  );
  const state = buildRenderState(sourceModel);
  const lines = [
    "//@version=6",
    `strategy(${toPineStringLiteral(sanitizeMetadataValue(context.name, "未命名策略"))}, overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)`,
  ];

  const inputLines = sourceModel.nodes
    .filter((node) => getStrategyBlockKind(node) === "strategyInput")
    .flatMap((node) => renderStrategyInputDeclaration(node, 0));
  if (inputLines.length > 0) {
    lines.push("", ...inputLines);
  }

  const initRoots = sourceModel.nodes.filter(
    (node) => getStrategyBlockKind(node) === "onInit",
  );
  const klineRoots = sourceModel.nodes.filter(
    (node) => getStrategyBlockKind(node) === "onKLineClosed",
  );

  for (const root of [...initRoots, ...klineRoots]) {
    lines.push("", ...renderHook(root, state));
  }

  if (initRoots.length === 0 && klineRoots.length === 0) {
    lines.push(
      "",
      `log.info(${toPineStringLiteral("策略尚未配置入口图块")})`,
    );
  }

  return lines.join("\n").trimEnd() + "\n";
}

function createEmptyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [],
    edges: [],
  };
}

function buildRenderState(model: StrategyVisualModelDocument): RenderState {
  const nodeById = new Map(model.nodes.map((node) => [node.id, node] as const));
  const outgoingById = new Map<string, StrategyVisualEdgeDocument[]>();
  const incomingById = new Map<string, StrategyVisualEdgeDocument[]>();

  for (const edge of model.edges) {
    const outgoing = outgoingById.get(edge.sourceNodeId) ?? [];
    outgoing.push(edge);
    outgoingById.set(edge.sourceNodeId, outgoing);

    const incoming = incomingById.get(edge.targetNodeId) ?? [];
    incoming.push(edge);
    incomingById.set(edge.targetNodeId, incoming);
  }

  return {
    nodeById,
    outgoingById,
    incomingById,
    emittedIndicatorNodeIds: new Set(),
    emittedStatementNodeIds: new Set(),
  };
}

function renderHook(
  root: StrategyVisualNodeDocument,
  state: RenderState,
): string[] {
  state.emittedIndicatorNodeIds.clear();
  state.emittedStatementNodeIds.clear();

  const body = renderControlChildren(root.id, state, 1, new Set());
  return [
    ...buildStrategyFlowNodeAnnotation(root, 0),
    ...(body.length > 0 ? body : [`log.info(${toPineStringLiteral("入口图块暂无动作")})`]),
  ];
}
function renderControlChildren(
  nodeId: string,
  state: RenderState,
  depth: number,
  visited: Set<string>,
  branch?: StrategyVisualEdgeBranch,
): string[] {
  const lines: string[] = [];
  const edges = controlOutgoingEdges(state, nodeId, branch);

  for (const edge of edges) {
    if (visited.has(edge.targetNodeId)) {
      continue;
    }
    const child = state.nodeById.get(edge.targetNodeId);
    if (child === undefined) {
      continue;
    }
    lines.push(...renderNode(child, state, depth, new Set(visited)));
  }

  return lines;
}

function renderNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  if (visited.has(node.id)) {
    return [];
  }
  visited.add(node.id);

  const kind = getStrategyBlockKind(node);
  assertSupportedVisualBlockKind(node, kind);

  switch (kind) {
    case "onInit":
    case "onKLineClosed":
      return renderControlChildren(node.id, state, depth, visited);
    case "log":
      return renderLinearStatement(
        node,
        state,
        depth,
        visited,
        `log.info(${toPineStringLiteral(normalizeMessage(node.properties.message, "策略事件"))})`,
      );
    case "notify":
      return renderLinearStatement(
        node,
        state,
        depth,
        visited,
        `alert(${toPineStringLiteral(normalizeMessage(node.properties.message, "策略通知"))})`,
      );
    case "getTechnicalIndicator":
      return renderGetTechnicalIndicatorNode(node, state, depth, visited);
    case "strategyInput":
      return renderControlChildren(node.id, state, depth, visited);
    case "derivedSeries":
      return renderLinearStatement(node, state, depth, visited, buildDerivedSeriesStatement(node));
    case "mtfSeries":
      return renderLinearStatement(node, state, depth, visited, buildMtfSeriesStatement(node));
    case "stateVariable":
      return renderLinearStatement(node, state, depth, visited, buildStateVariableStatement(node));
    case "stateUpdate":
      return renderLinearStatement(node, state, depth, visited, buildStateUpdateStatement(node));
    case "collectionStat":
      return renderLinearStatement(node, state, depth, visited, buildCollectionStatStatement(node));
    case "timeFilter":
      return renderTimeFilterNode(node, state, depth, visited);
    case "sessionFilter":
      return renderSessionFilterNode(node, state, depth, visited);
    case "technicalIndicatorCondition":
      return renderTechnicalIndicatorConditionNode(node, state, depth, visited);
    case "seriesCondition":
      return renderSeriesConditionNode(node, state, depth, visited);
    case "ifCloseAbove":
    case "ifCloseBelow":
      return renderCloseConditionNode(node, state, depth, visited, kind);
    case "placeOrder":
      return renderLinearStatement(node, state, depth, visited, buildOrderStatement(node));
    case "riskRule":
      return renderLinearStatement(node, state, depth, visited, buildRiskRuleStatement(node));
    case "stopLoss":
      return renderProtectNode(node, state, depth, visited);
    default:
      throw new Error(`不支持的流程图块：${String(kind)}`);
  }
}

function assertSupportedVisualBlockKind(
  node: StrategyVisualNodeDocument,
  kind: ReturnType<typeof getStrategyBlockKind>,
): void {
  const rawKind = String(node.properties.blockKind ?? "");
  if (
    rawKind === "codeBlock"
    || rawKind === "technicalIndicator"
    || kind === null
  ) {
    throw new Error(`旧流程图块 ${rawKind || node.id} 不再支持，请改用 Pine v6 标准图块。`);
  }
}

function renderStrategyInputDeclaration(
  node: StrategyVisualNodeDocument,
  depth: number,
): string[] {
  const properties = normalizeStrategyInputBlockProperties(node.properties ?? {});
  return [
    ...buildStrategyFlowNodeAnnotation(node, depth, { variableName: properties.variableName ?? "input_value" }),
    `${indent(depth)}${properties.variableName} = ${buildStrategyInputExpression(properties)}`,
  ];
}

function renderTimeFilterNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  return renderConditionNode(
    node,
    state,
    depth,
    visited,
    buildTimeFilterExpression(node.properties ?? {}),
    "时间过滤命中但未配置动作",
  );
}

function renderSessionFilterNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  return renderConditionNode(
    node,
    state,
    depth,
    visited,
    buildSessionFilterExpression(node.properties ?? {}),
    "交易时段过滤命中但未配置动作",
  );
}

function renderConditionNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
  expression: string,
  emptyMessage: string,
): string[] {
  const trueBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "true");
  const falseBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "false");
  return [
    ...buildStrategyFlowNodeAnnotation(node, depth),
    `${indent(depth)}if ${expression}`,
    ...(trueBody.length > 0 ? trueBody : [`${indent(depth + 1)}log.info(${toPineStringLiteral(emptyMessage)})`]),
    ...(falseBody.length > 0 ? [`${indent(depth)}else`, ...falseBody] : []),
  ];
}

function renderSeriesConditionNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  const expression = buildSeriesConditionExpression(node.properties ?? {});
  const trueBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "true");
  const falseBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "false");

  return [
    ...buildStrategyFlowNodeAnnotation(node, depth),
    `${indent(depth)}if ${expression}`,
    ...(trueBody.length > 0 ? trueBody : [`${indent(depth + 1)}log.info(${toPineStringLiteral("序列条件命中但未配置动作")})`]),
    ...(falseBody.length > 0
      ? [`${indent(depth)}else`, ...falseBody]
      : []),
  ];
}

function renderLinearStatement(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
  statement: string,
): string[] {
  state.emittedStatementNodeIds.add(node.id);
  const indented = statement
    .split("\n")
    .map((line) => `${indent(depth)}${line}`);
  return [
    ...buildStrategyFlowNodeAnnotation(node, depth),
    ...indented,
    ...renderControlChildren(node.id, state, depth, visited),
  ];
}

function renderGetTechnicalIndicatorNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  const lines = renderIndicatorDeclaration(node, state, depth);
  return [
    ...lines,
    ...renderControlChildren(node.id, state, depth, visited),
  ];
}

function renderTechnicalIndicatorConditionNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  const properties = normalizeTechnicalIndicatorConditionProperties(node.properties ?? {});
  const inputs = incomingIndicatorInputs(node.id, state, properties);
  const setupLines = inputs.flatMap((input) => renderIndicatorDeclaration(input.node, state, depth));
  const expression = buildTechnicalIndicatorConditionExpression(properties, inputs) ?? "false";
  const trueBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "true");
  const falseBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "false");

  return [
    ...setupLines,
    ...buildStrategyFlowNodeAnnotation(node, depth, readConditionInputAnnotation(inputs)),
    `${indent(depth)}if ${expression}`,
    ...(trueBody.length > 0 ? trueBody : [`${indent(depth + 1)}log.info(${toPineStringLiteral("条件命中但未配置动作")})`]),
    ...(falseBody.length > 0
      ? [`${indent(depth)}else`, ...falseBody]
      : []),
  ];
}

function renderCloseConditionNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
  kind: "ifCloseAbove" | "ifCloseBelow",
): string[] {
  const threshold = normalizeThreshold(node.properties.threshold, kind === "ifCloseAbove" ? 520 : 480);
  const operator = kind === "ifCloseAbove" ? ">" : "<";
  const trueBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "true");
  const falseBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "false");

  return [
    ...buildStrategyFlowNodeAnnotation(node, depth),
    `${indent(depth)}if close ${operator} ${formatNumber(threshold)}`,
    ...(trueBody.length > 0 ? trueBody : [`${indent(depth + 1)}log.info(${toPineStringLiteral("价格条件命中但未配置动作")})`]),
    ...(falseBody.length > 0
      ? [`${indent(depth)}else`, ...falseBody]
      : []),
  ];
}

function renderIndicatorDeclaration(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
): string[] {
  if (state.emittedIndicatorNodeIds.has(node.id)) {
    return [];
  }

  const properties = normalizeGetTechnicalIndicatorProperties(node.properties ?? {});
  const variableName = readIndicatorVariableName(node, properties);
  state.emittedIndicatorNodeIds.add(node.id);
  state.emittedStatementNodeIds.add(node.id);

  if (properties.indicatorType === "kdj") {
    return [
      ...buildStrategyFlowNodeAnnotation(node, depth, { variableName }),
      ...buildKDJIndicatorStatements(variableName, properties).map((line) => `${indent(depth)}${line}`),
    ];
  }

  return [
    ...buildStrategyFlowNodeAnnotation(node, depth, { variableName }),
    `${indent(depth)}${variableName} = ${buildIndicatorExpression(properties)}`,
  ];
}

function incomingIndicatorInputs(
  nodeId: string,
  state: RenderState,
  properties: TechnicalIndicatorConditionBlockProperties,
): IndicatorInputBinding[] {
  const bySlot = new Map<"primary" | "fast" | "slow", IndicatorInputBinding>();

  for (const edge of state.incomingById.get(nodeId) ?? []) {
    if (!isStrategyVisualDataEdge(edge)) {
      continue;
    }
    const slot = readStrategyVisualEdgeInputSlot(edge) ?? "primary";
    const inputNode = state.nodeById.get(edge.sourceNodeId);
    if (inputNode === undefined || getStrategyBlockKind(inputNode) !== "getTechnicalIndicator") {
      continue;
    }
    bySlot.set(slot, {
      node: inputNode,
      slot,
      properties: normalizeGetTechnicalIndicatorProperties(inputNode.properties ?? {}),
    });
  }

  const propertyReferences: Array<["primary" | "fast" | "slow", unknown]> = [
    ["primary", properties.inputPrimaryNodeId],
    ["fast", properties.inputFastNodeId],
    ["slow", properties.inputSlowNodeId],
  ];
  for (const [slot, rawNodeId] of propertyReferences) {
    if (typeof rawNodeId !== "string" || rawNodeId.trim() === "" || bySlot.has(slot)) {
      continue;
    }
    const inputNode = state.nodeById.get(rawNodeId.trim());
    if (inputNode === undefined || getStrategyBlockKind(inputNode) !== "getTechnicalIndicator") {
      continue;
    }
    bySlot.set(slot, {
      node: inputNode,
      slot,
      properties: normalizeGetTechnicalIndicatorProperties(inputNode.properties ?? {}),
    });
  }

  const orderedSlots: Array<"primary" | "fast" | "slow"> =
    properties.indicatorType === "movingAverage" ? ["fast", "slow"] : ["primary", "fast", "slow"];
  return orderedSlots
    .map((slot) => bySlot.get(slot))
    .filter((input): input is IndicatorInputBinding => input !== undefined);
}

function readConditionInputAnnotation(
  inputs: IndicatorInputBinding[],
): Partial<Pick<
  StrategyFlowNodeJsDoc,
  "inputPrimaryNodeId" | "inputFastNodeId" | "inputSlowNodeId"
>> {
  const annotation: {
    inputPrimaryNodeId?: string;
    inputFastNodeId?: string;
    inputSlowNodeId?: string;
  } = {};
  for (const input of inputs) {
    if (input.slot === "primary") {
      annotation.inputPrimaryNodeId = input.node.id;
    } else if (input.slot === "fast") {
      annotation.inputFastNodeId = input.node.id;
    } else {
      annotation.inputSlowNodeId = input.node.id;
    }
  }
  return annotation;
}

function buildTechnicalIndicatorConditionExpression(
  properties: TechnicalIndicatorConditionBlockProperties,
  inputs: IndicatorInputBinding[],
): string | null {
  const primary = inputs.find((input) => input.slot === "primary") ?? inputs[0];

  if (properties.conditionMode === "numeric") {
    if (primary === undefined) {
      return null;
    }
    const target = numericConditionTargetExpression(primary);
    return `${target} ${properties.operator ?? "<"} ${formatNumber(properties.threshold ?? 0)}`;
  }

  switch (properties.indicatorType) {
    case "movingAverage": {
      const fast = inputs.find((input) => input.slot === "fast") ?? inputs[0];
      const slow = inputs.find((input) => input.slot === "slow") ?? inputs[1];
      if (fast === undefined || slow === undefined) {
        return null;
      }
      return `ta.${properties.patternType === "deathCross" ? "crossunder" : "crossover"}(${readIndicatorVariableName(fast.node, fast.properties)}, ${readIndicatorVariableName(slow.node, slow.properties)})`;
    }
    case "macd": {
      if (primary === undefined) {
        return null;
      }
      const variableName = readIndicatorVariableName(primary.node, primary.properties);
      if (properties.patternType === "topDivergence" || properties.patternType === "bottomDivergence") {
        return `${properties.patternType === "topDivergence" ? "divergence_top" : "divergence_bottom"}(${variableName}, ${properties.lookback ?? 5})`;
      }
      return `ta.${properties.patternType === "deathCross" ? "crossunder" : "crossover"}(${variableName}.diff, ${variableName}.signal)`;
    }
    case "kdj": {
      if (primary === undefined) {
        return null;
      }
      const variableName = readIndicatorVariableName(primary.node, primary.properties);
      if (properties.patternType === "topDivergence" || properties.patternType === "bottomDivergence") {
        return "false";
      }
      return `ta.${properties.patternType === "deathCross" ? "crossunder" : "crossover"}(${variableName}_k, ${variableName}_d)`;
    }
    case "rsi": {
      if (primary === undefined || (properties.patternType !== "topDivergence" && properties.patternType !== "bottomDivergence")) {
        return null;
      }
      const variableName = readIndicatorVariableName(primary.node, primary.properties);
      return `${properties.patternType === "topDivergence" ? "divergence_top" : "divergence_bottom"}(${variableName}, ${properties.lookback ?? 5})`;
    }
    case "bollinger": {
      if (primary === undefined) {
        return null;
      }
      const variableName = readIndicatorVariableName(primary.node, primary.properties);
      return properties.patternType === "closeAboveUpperBand"
        ? `close > ${variableName}.upper`
        : `close < ${variableName}.lower`;
    }
    default:
      return null;
  }
}

function numericConditionTargetExpression(input: IndicatorInputBinding): string {
  const variableName = readIndicatorVariableName(input.node, input.properties);
  switch (input.properties.indicatorType) {
    case "macd":
      return `${variableName}.histogram`;
    case "kdj":
      return `${variableName}_j`;
    case "dmi":
      return `${variableName}.adx`;
    case "supertrend":
      return `${variableName}.direction`;
    case "keltner":
      return `${variableName}.upper`;
    default:
      return variableName;
  }
}

function controlOutgoingEdges(
  state: RenderState,
  nodeId: string,
  branch?: StrategyVisualEdgeBranch,
): StrategyVisualEdgeDocument[] {
  const edges = (state.outgoingById.get(nodeId) ?? []).filter(isStrategyVisualControlEdge);
  if (branch === undefined) {
    return edges.filter((edge) => readStrategyVisualEdgeBranch(edge) === null);
  }

  const branchEdges = edges.filter((edge) => readStrategyVisualEdgeBranch(edge) === branch);
  if (branchEdges.length > 0) {
    return branchEdges;
  }
  return branch === "true"
    ? edges.filter((edge) => readStrategyVisualEdgeBranch(edge) === null)
    : [];
}

function renderProtectNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  state.emittedStatementNodeIds.add(node.id);
  return [
    ...buildStrategyFlowNodeAnnotation(node, depth),
    ...buildProtectStatements(node).map((statement) => `${indent(depth)}${statement}`),
    ...renderControlChildren(node.id, state, depth, visited),
  ];
}
