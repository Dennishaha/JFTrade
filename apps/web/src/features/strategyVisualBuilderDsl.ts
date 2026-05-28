import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";

import {
  getStrategyBlockKind,
  normalizeStopLossBlockProperties,
} from "./strategyVisualBuilderCatalog";
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
  normalizeTechnicalIndicatorProperties,
  type GetTechnicalIndicatorBlockProperties,
  type TechnicalIndicatorBlockProperties,
  type TechnicalIndicatorConditionBlockProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import { reconcileStrategyVisualModelIndicatorBindings } from "./strategyVisualBuilderIndicatorReferences";
import {
  normalizeDecimal,
  normalizeEntryPositionPolicy,
  normalizeMessage,
  normalizeOrderSide,
  normalizeOrderType,
  normalizeQuantityModeForSide,
  normalizeThreshold,
  type VisualOrderSide,
} from "./strategyVisualBuilderScriptSupport";
import {
  buildStrategyFlowNodeDslComment,
  cloneStrategyVisualModel,
  type StrategyFlowNodeJsDoc,
} from "./strategyVisualBuilderShared";

export interface StrategyDslContext {
  name: string;
  symbol: string;
  interval: string;
  version?: string;
}

export type StrategyScriptContext = StrategyDslContext;

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

export function buildStrategyDslFromVisualModel(
  model: StrategyVisualModelDocument | null | undefined,
  context: StrategyDslContext,
): string {
  const sourceModel = reconcileStrategyVisualModelIndicatorBindings(
    cloneStrategyVisualModel(model) ?? createEmptyVisualModel(),
  );
  const state = buildRenderState(sourceModel);
  const lines = [
    `strategy ${sanitizeMetadataValue(context.name, "未命名策略")}`,
    `version ${sanitizeMetadataValue(context.version ?? "0.1.0", "0.1.0")}`,
    `symbol ${sanitizeMetadataValue(context.symbol, "00700")}`,
    `interval ${sanitizeMetadataValue(context.interval, "1m")}`,
  ];

  const initRoots = sourceModel.nodes.filter(
    (node) => getStrategyBlockKind(node) === "onInit",
  );
  const klineRoots = sourceModel.nodes.filter(
    (node) => getStrategyBlockKind(node) === "onKLineClosed",
  );

  for (const root of [...initRoots, ...klineRoots]) {
    const hookLines = renderHook(root, state);
    if (hookLines.length === 0) {
      continue;
    }
    lines.push("", ...hookLines);
  }

  if (initRoots.length === 0 && klineRoots.length === 0) {
    lines.push(
      "",
      "on kline_close:",
      "  log \"策略尚未配置入口图块\"",
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
  const kind = getStrategyBlockKind(root);
  const hookHeader = kind === "onInit" ? "on init:" : kind === "onKLineClosed" ? "on kline_close:" : null;
  if (hookHeader === null) {
    return [];
  }

  state.emittedIndicatorNodeIds.clear();
  state.emittedStatementNodeIds.clear();

  const body = renderControlChildren(root.id, state, 1, new Set());
  return [
    ...buildStrategyFlowNodeDslComment(root, 0),
    hookHeader,
    ...(body.length > 0 ? body : [`${indent(1)}log ${toDslStringLiteral("入口图块暂无动作")}`]),
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
        `log ${toDslStringLiteral(normalizeMessage(node.properties.message, "策略事件"))}`,
      );
    case "notify":
      return renderLinearStatement(
        node,
        state,
        depth,
        visited,
        `notify ${toDslStringLiteral(normalizeMessage(node.properties.message, "策略通知"))}`,
      );
    case "getTechnicalIndicator":
      return renderGetTechnicalIndicatorNode(node, state, depth, visited);
    case "technicalIndicatorCondition":
      return renderTechnicalIndicatorConditionNode(node, state, depth, visited);
    case "technicalIndicator":
      return renderUnifiedTechnicalIndicatorNode(node, state, depth, visited);
    case "ifCloseAbove":
    case "ifCloseBelow":
      return renderCloseConditionNode(node, state, depth, visited, kind);
    case "placeOrder":
      return renderLinearStatement(node, state, depth, visited, buildOrderStatement(node));
    case "stopLoss":
      return renderLinearStatement(node, state, depth, visited, buildProtectStatement(node));
    case "codeBlock":
      return renderLinearStatement(
        node,
        state,
        depth,
        visited,
        `log ${toDslStringLiteral("代码块已废弃，请改用 DSL 图块")}`,
      );
    default:
      return renderLinearStatement(
        node,
        state,
        depth,
        visited,
        `log ${toDslStringLiteral(node.text || "未识别图块")}`,
      );
  }
}

function renderLinearStatement(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
  statement: string,
): string[] {
  state.emittedStatementNodeIds.add(node.id);
  return [
    ...buildStrategyFlowNodeDslComment(node, depth),
    `${indent(depth)}${statement}`,
    ...renderControlChildren(node.id, state, depth, visited),
  ];
}

function renderGetTechnicalIndicatorNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  const lines = renderIndicatorDeclaration(node, state, depth, true);
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
  const setupLines = inputs.flatMap((input) => renderIndicatorDeclaration(input.node, state, depth, true));
  const expression = buildTechnicalIndicatorConditionExpression(properties, inputs) ?? "false";
  const trueBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "true");
  const falseBody = renderControlChildren(node.id, state, depth + 1, new Set(visited), "false");

  return [
    ...setupLines,
    ...buildStrategyFlowNodeDslComment(node, depth, readConditionInputAnnotation(inputs)),
    `${indent(depth)}if ${expression}:`,
    ...(trueBody.length > 0 ? trueBody : [`${indent(depth + 1)}log ${toDslStringLiteral("条件命中但未配置动作")}`]),
    ...(falseBody.length > 0
      ? [`${indent(depth)}else:`, ...falseBody]
      : []),
  ];
}

function renderUnifiedTechnicalIndicatorNode(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  visited: Set<string>,
): string[] {
  const properties = normalizeTechnicalIndicatorProperties(node.properties ?? {});
  const getterNodes = buildSyntheticGetterNodes(node, properties);
  const setupLines = getterNodes.flatMap((getterNode) =>
    renderIndicatorDeclaration(getterNode, state, depth, false),
  );

  if (properties.conditionMode === "none") {
    return [
      ...setupLines,
      ...renderControlChildren(node.id, state, depth, visited),
    ];
  }

  const inputs: IndicatorInputBinding[] = getterNodes.map((getterNode, index) => ({
    node: getterNode,
    slot: properties.indicatorType === "movingAverage"
      ? index === 0 ? "fast" : "slow"
      : "primary",
    properties: normalizeGetTechnicalIndicatorProperties(getterNode.properties),
  }));
  const conditionProperties: TechnicalIndicatorConditionBlockProperties = {
    blockKind: "technicalIndicatorCondition",
    indicatorType: properties.indicatorType,
    conditionMode: properties.conditionMode,
    ...(properties.operator !== undefined ? { operator: properties.operator } : {}),
    ...(properties.threshold !== undefined ? { threshold: properties.threshold } : {}),
    ...(properties.patternType !== undefined ? { patternType: properties.patternType } : {}),
    ...(properties.lookback !== undefined ? { lookback: properties.lookback } : {}),
  };
  const expression = buildTechnicalIndicatorConditionExpression(conditionProperties, inputs) ?? "false";
  const body = renderControlChildren(node.id, state, depth + 1, new Set(visited));

  return [
    ...setupLines,
    ...buildStrategyFlowNodeDslComment(node, depth),
    `${indent(depth)}if ${expression}:`,
    ...(body.length > 0 ? body : [`${indent(depth + 1)}log ${toDslStringLiteral("指标条件命中但未配置动作")}`]),
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
    ...buildStrategyFlowNodeDslComment(node, depth),
    `${indent(depth)}if close ${operator} ${formatNumber(threshold)}:`,
    ...(trueBody.length > 0 ? trueBody : [`${indent(depth + 1)}log ${toDslStringLiteral("价格条件命中但未配置动作")}`]),
    ...(falseBody.length > 0
      ? [`${indent(depth)}else:`, ...falseBody]
      : []),
  ];
}

function renderIndicatorDeclaration(
  node: StrategyVisualNodeDocument,
  state: RenderState,
  depth: number,
  includeChildren: boolean,
): string[] {
  if (state.emittedIndicatorNodeIds.has(node.id)) {
    return [];
  }

  const properties = normalizeGetTechnicalIndicatorProperties(node.properties ?? {});
  const variableName = readIndicatorVariableName(node, properties);
  state.emittedIndicatorNodeIds.add(node.id);
  state.emittedStatementNodeIds.add(node.id);

  const lines = [
    ...buildStrategyFlowNodeDslComment(node, depth, { variableName }),
    `${indent(depth)}let ${variableName} = ${buildIndicatorExpression(properties)}`,
  ];

  if (!includeChildren) {
    return lines;
  }

  return lines;
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
      return `${properties.patternType === "deathCross" ? "cross_under" : "cross_over"}(${readIndicatorVariableName(fast.node, fast.properties)}, ${readIndicatorVariableName(slow.node, slow.properties)})`;
    }
    case "macd": {
      if (primary === undefined) {
        return null;
      }
      const variableName = readIndicatorVariableName(primary.node, primary.properties);
      if (properties.patternType === "topDivergence" || properties.patternType === "bottomDivergence") {
        return `${properties.patternType === "topDivergence" ? "divergence_top" : "divergence_bottom"}(${variableName}, ${properties.lookback ?? 5})`;
      }
      return `${properties.patternType === "deathCross" ? "cross_under" : "cross_over"}(${variableName}.diff, ${variableName}.signal)`;
    }
    case "kdj": {
      if (primary === undefined) {
        return null;
      }
      const variableName = readIndicatorVariableName(primary.node, primary.properties);
      if (properties.patternType === "topDivergence" || properties.patternType === "bottomDivergence") {
        return `${properties.patternType === "topDivergence" ? "divergence_top" : "divergence_bottom"}(${variableName}, ${properties.lookback ?? 5})`;
      }
      return `${properties.patternType === "deathCross" ? "cross_under" : "cross_over"}(${variableName}.k, ${variableName}.d)`;
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
      return `${variableName}.j`;
    default:
      return variableName;
  }
}

function buildSyntheticGetterNodes(
  node: StrategyVisualNodeDocument,
  properties: TechnicalIndicatorBlockProperties,
): StrategyVisualNodeDocument[] {
  if (properties.indicatorType === "movingAverage") {
    return [
      {
        ...node,
        id: `${node.id}-fast`,
        properties: {
          blockKind: "getTechnicalIndicator",
          indicatorType: "movingAverage",
          movingAverageType: properties.movingAverageType,
          windowSize: properties.fastPeriod,
        },
      },
      {
        ...node,
        id: `${node.id}-slow`,
        properties: {
          blockKind: "getTechnicalIndicator",
          indicatorType: "movingAverage",
          movingAverageType: properties.movingAverageType,
          windowSize: properties.slowPeriod,
        },
      },
    ];
  }

  return [
    {
      ...node,
      properties: {
        blockKind: "getTechnicalIndicator",
        indicatorType: properties.indicatorType,
        ...(properties.period !== undefined ? { period: properties.period } : {}),
        ...(properties.fastPeriod !== undefined ? { fastPeriod: properties.fastPeriod } : {}),
        ...(properties.slowPeriod !== undefined ? { slowPeriod: properties.slowPeriod } : {}),
        ...(properties.signalPeriod !== undefined ? { signalPeriod: properties.signalPeriod } : {}),
        ...(properties.m1 !== undefined ? { m1: properties.m1 } : {}),
        ...(properties.m2 !== undefined ? { m2: properties.m2 } : {}),
        ...(properties.multiplier !== undefined ? { multiplier: properties.multiplier } : {}),
      },
    },
  ];
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

function buildIndicatorExpression(properties: GetTechnicalIndicatorBlockProperties): string {
  switch (properties.indicatorType) {
    case "movingAverage": {
      const movingAverageType = properties.movingAverageType ?? "MA";
      const windowSize = properties.windowSize ?? properties.period ?? 20;
      const periodUnit = properties.periodUnit ?? "bar";
      return periodUnit === "bar"
        ? `ma(${movingAverageType}, ${windowSize})`
        : `ma(${movingAverageType}, ${windowSize}, ${periodUnit})`;
    }
    case "macd":
      return `macd(${properties.fastPeriod ?? 12}, ${properties.slowPeriod ?? 26}, ${properties.signalPeriod ?? 9})`;
    case "kdj":
      return `kdj(${properties.period ?? 9}, ${properties.m1 ?? 3}, ${properties.m2 ?? 3})`;
    case "bollinger":
      return `bollinger(${properties.period ?? 20}, ${formatNumber(properties.multiplier ?? 2)})`;
    case "atr":
      return `atr(${properties.period ?? 14})`;
    case "cci":
      return `cci(${properties.period ?? 20})`;
    case "williamsR":
      return `williams_r(${properties.period ?? 14})`;
    case "rsi":
    default:
      return `rsi(${properties.period ?? 14})`;
  }
}

function readIndicatorVariableName(
  node: StrategyVisualNodeDocument,
  properties: GetTechnicalIndicatorBlockProperties,
): string {
  const fromProperties = typeof properties.variableName === "string" ? properties.variableName : "";
  if (isDslIdentifier(fromProperties)) {
    return fromProperties;
  }
  return sanitizeDslIdentifier(node.id, "indicator");
}

function buildOrderStatement(node: StrategyVisualNodeDocument): string {
  const side = normalizeOrderSide(node.properties.side);
  const action = orderActionForSide(side);
  const quantityMode = normalizeQuantityModeForSide(node.properties.quantityMode, side);
  const quantityValue = normalizeDecimal(node.properties.quantityValue, 100);
  const entryPolicy = normalizeEntryPositionPolicy(node.properties.entryPositionPolicy);
  const orderType = normalizeOrderType(node.properties.orderType);
  const limitPrice = normalizeDecimal(node.properties.limitPrice, 0);
  const options = [`policy ${entryPolicyToDsl(entryPolicy)}`, `type ${orderType}`];

  if (orderType === "LIMIT" && limitPrice > 0) {
    options.push(`limit ${formatNumber(limitPrice)}`);
  }

  return [
    action,
    quantityModeToDsl(quantityMode),
    formatNumber(quantityValue),
    ...options,
  ].join(" ");
}

function buildProtectStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeStopLossBlockProperties(node.properties ?? {});
  return [
    "protect",
    properties.direction ?? "auto",
    properties.mode ?? "stopLoss",
    formatNumber(properties.timeValue ?? 1),
    properties.timeUnit ?? "day",
    formatNumber(properties.percentage ?? 2),
    "window",
    properties.windowPolicy ?? "continuous",
  ].join(" ");
}

function orderActionForSide(side: VisualOrderSide): "buy" | "sell" | "short" | "cover" {
  switch (side) {
    case "SELL":
      return "sell";
    case "SELL_SHORT":
      return "short";
    case "BUY_COVER":
      return "cover";
    case "BUY":
    default:
      return "buy";
  }
}

function quantityModeToDsl(mode: ReturnType<typeof normalizeQuantityModeForSide>): string {
  switch (mode) {
    case "accountPositionPercent":
      return "account_position_percent";
    case "symbolPositionPercent":
      return "symbol_position_percent";
    case "cashPercent":
      return "cash_percent";
    case "marginBuyingPowerPercent":
      return "margin_buying_power_percent";
    case "shortSellingPowerPercent":
      return "short_selling_power_percent";
    default:
      return mode;
  }
}

function entryPolicyToDsl(policy: ReturnType<typeof normalizeEntryPositionPolicy>): string {
  switch (policy) {
    case "flatOnly":
      return "flat_only";
    case "allow":
      return "allow";
    default:
      return "same_direction";
  }
}

function sanitizeMetadataValue(value: string, fallback: string): string {
  const normalized = value.replace(/[\r\n]+/g, " ").trim();
  return normalized === "" ? fallback : normalized;
}

function sanitizeDslIdentifier(value: string, fallback: string): string {
  const normalized = value
    .trim()
    .replace(/[^A-Za-z0-9_]+/g, "_")
    .replace(/^([0-9])/, "_$1")
    .replace(/^_+|_+$/g, "");
  return normalized === "" ? fallback : normalized;
}

function isDslIdentifier(value: string): boolean {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(value);
}

function toDslStringLiteral(value: string): string {
  return JSON.stringify(value.replace(/\r?\n/g, " ").trim());
}

function formatNumber(value: number): string {
  return Number.isFinite(value) ? String(value) : "0";
}

function indent(depth: number): string {
  return "  ".repeat(depth);
}

export const buildStrategyScriptFromVisualModel = buildStrategyDslFromVisualModel;
