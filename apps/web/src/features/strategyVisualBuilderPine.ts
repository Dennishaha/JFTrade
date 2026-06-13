import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";

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

export type StrategyScriptContext = StrategyPineContext;

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
  const kind = getStrategyBlockKind(root);
  if (kind !== "onInit" && kind !== "onKLineClosed") {
    return [];
  }

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
  if (node.properties.blockKind === "codeBlock" || node.properties.blockKind === "technicalIndicator") {
    return [];
  }
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
    case "technicalIndicatorCondition":
      return renderTechnicalIndicatorConditionNode(node, state, depth, visited);
    case "ifCloseAbove":
    case "ifCloseBelow":
      return renderCloseConditionNode(node, state, depth, visited, kind);
    case "placeOrder":
      return renderLinearStatement(node, state, depth, visited, buildOrderStatement(node));
    case "stopLoss":
      return renderProtectNode(node, state, depth, visited);
    case "pineSnippet":
      return renderLinearStatement(
        node,
        state,
        depth,
        visited,
        readPineSnippetCode(node.properties.code),
      );
    default:
      return renderLinearStatement(
        node,
        state,
        depth,
        visited,
        `log.info(${toPineStringLiteral(node.text || "未识别图块")})`,
      );
  }
}

function readPineSnippetCode(value: unknown): string {
  if (typeof value !== "string" || value.trim() === "") {
    return `log.info(${toPineStringLiteral("Pine 片段为空")})`;
  }
  return value.trimEnd();
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
    ...buildStrategyFlowNodeAnnotation(node, depth, { variableName }),
    `${indent(depth)}${variableName} = ${buildIndicatorExpression(properties)}`,
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
        return `${properties.patternType === "topDivergence" ? "divergence_top" : "divergence_bottom"}(${variableName}, ${properties.lookback ?? 5})`;
      }
      return `ta.${properties.patternType === "deathCross" ? "crossunder" : "crossover"}(${variableName}.k, ${variableName}.d)`;
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
      const expression = buildMovingAverageExpression(movingAverageType, windowSize, properties.source ?? "close");
      const timeframe = pineTimeframeForPeriodUnit(properties.periodUnit ?? "bar");
      return timeframe === null
        ? expression
        : `request.security(syminfo.tickerid, ${toPineStringLiteral(timeframe)}, ${expression})`;
    }
    case "macd":
      return `ta.macd(close, ${properties.fastPeriod ?? 12}, ${properties.slowPeriod ?? 26}, ${properties.signalPeriod ?? 9})`;
    case "kdj":
      return `ta.rsi(close, ${properties.period ?? 9})`;
    case "bollinger":
      return `ta.sma(close, ${properties.period ?? 20})`;
    case "atr":
      return `ta.atr(${properties.period ?? 14})`;
    case "cci":
      return `ta.cci(close, ${properties.period ?? 20})`;
    case "williamsR":
      return `ta.wpr(${properties.period ?? 14})`;
    case "rsi":
    default:
      return `ta.rsi(close, ${properties.period ?? 14})`;
  }
}

function buildMovingAverageExpression(
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

function pineMovingAverageSource(source: string): string {
  switch (source) {
    case "open":
    case "high":
    case "low":
    case "volume":
      return source;
    case "close":
    default:
      return "close";
  }
}

function pineTimeframeForPeriodUnit(periodUnit: string): string | null {
  switch (periodUnit) {
    case "minute":
      return "1";
    case "hour":
      return "60";
    case "day":
      return "D";
    case "week":
      return "W";
    case "month":
      return "M";
    case "bar":
    default:
      return null;
  }
}

function readIndicatorVariableName(
  node: StrategyVisualNodeDocument,
  properties: GetTechnicalIndicatorBlockProperties,
): string {
  const fromProperties = typeof properties.variableName === "string" ? properties.variableName : "";
  if (isPineIdentifier(fromProperties)) {
    return fromProperties;
  }
  return sanitizePineIdentifier(node.id, "indicator");
}

function buildOrderStatement(node: StrategyVisualNodeDocument): string {
  const side = normalizeOrderSide(node.properties.side);
  const quantityMode = normalizeQuantityModeForSide(node.properties.quantityMode, side);
  const quantityValue = normalizeDecimal(node.properties.quantityValue, 100);
  const quantityOption = `qty=${buildPineQuantityExpression(quantityMode, quantityValue)}`;
  const orderType = String(node.properties.orderType ?? "MARKET").toUpperCase();
  const limitPrice = normalizeDecimal(node.properties.limitPrice, 0);
  const limitOption = orderType === "LIMIT" && limitPrice > 0
    ? `, limit=${formatNumber(limitPrice)}`
    : "";

  const entryPolicy = normalizeEntryPositionPolicy(node.properties.entryPositionPolicy);
  const entryPolicyAnnotation = entryPolicy !== "sameDirection"
    ? `// @entry_policy ${entryPositionPolicyToSnakeCase(entryPolicy)}\n`
    : "";

  switch (side) {
    case "SELL":
      return `strategy.close("Long", ${quantityOption}${limitOption})`;
    case "SELL_SHORT":
      return `${entryPolicyAnnotation}strategy.entry("Short", strategy.short, ${quantityOption}${limitOption})`;
    case "BUY_COVER":
      return `strategy.close("Short", ${quantityOption}${limitOption})`;
    case "BUY":
    default:
      return `${entryPolicyAnnotation}strategy.entry("Long", strategy.long, ${quantityOption}${limitOption})`;
  }
}

function buildPineQuantityExpression(
  quantityMode: ReturnType<typeof normalizeQuantityModeForSide>,
  quantityValue: number,
): string {
  const value = formatNumber(quantityValue);
  switch (quantityMode) {
    case "amount":
      return `(${value} / close)`;
    case "equityPercent":
      return `((strategy.equity * ${value} / 100) / close)`;
    case "shares":
    default:
      return value;
  }
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

function buildProtectStatements(node: StrategyVisualNodeDocument): string[] {
  const properties = normalizeStopLossBlockProperties(node.properties ?? {});
  if (
    (properties.windowPolicy ?? "continuous") !== "continuous" ||
    (properties.timeUnit ?? "day") !== "bar" ||
    (properties.timeValue ?? 1) !== 1
  ) {
    return [
      `runtime.error(${toPineStringLiteral("JFTrade Pine 暂不支持带时间窗口或交易时段感知的自动退出图块")})`,
    ];
  }

  const percentage = formatNumber(properties.percentage ?? 2);
  const directions = properties.direction === "long"
    ? ["long"]
    : properties.direction === "short"
      ? ["short"]
      : ["long", "short"];
  return directions.map((direction) => {
    const entryId = direction === "short" ? "Short" : "Long";
    const exitId = `${entryId} ${properties.mode ?? "stopLoss"}`;
    switch (properties.mode) {
      case "takeProfit":
        return direction === "short"
          ? `strategy.exit(${toPineStringLiteral(exitId)}, ${toPineStringLiteral(entryId)}, limit=close * (1 - ${percentage} / 100))`
          : `strategy.exit(${toPineStringLiteral(exitId)}, ${toPineStringLiteral(entryId)}, limit=close * (1 + ${percentage} / 100))`;
      case "trailingStop":
        return `strategy.exit(${toPineStringLiteral(exitId)}, ${toPineStringLiteral(entryId)}, trail_points=close * ${percentage} / 100, trail_offset=close * ${percentage} / 100)`;
      case "stopLoss":
      default:
        return direction === "short"
          ? `strategy.exit(${toPineStringLiteral(exitId)}, ${toPineStringLiteral(entryId)}, stop=close * (1 + ${percentage} / 100))`
          : `strategy.exit(${toPineStringLiteral(exitId)}, ${toPineStringLiteral(entryId)}, stop=close * (1 - ${percentage} / 100))`;
    }
  });
}

function sanitizeMetadataValue(value: string, fallback: string): string {
  const normalized = value.replace(/[\r\n]+/g, " ").trim();
  return normalized === "" ? fallback : normalized;
}

function sanitizePineIdentifier(value: string, fallback: string): string {
  const normalized = value
    .trim()
    .replace(/[^A-Za-z0-9_]+/g, "_")
    .replace(/^([0-9])/, "_$1")
    .replace(/^_+|_+$/g, "");
  return normalized === "" ? fallback : normalized;
}

function isPineIdentifier(value: string): boolean {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(value);
}

function toPineStringLiteral(value: string): string {
  return JSON.stringify(value.replace(/\r?\n/g, " ").trim());
}

function formatNumber(value: number): string {
  return Number.isFinite(value) ? String(value) : "0";
}

function indent(depth: number): string {
  return "  ".repeat(depth);
}

export const buildStrategyScriptFromVisualModel = buildStrategyPineFromVisualModel;
