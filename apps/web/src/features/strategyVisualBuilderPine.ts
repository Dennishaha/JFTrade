import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";

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
  assertSupportedVisualBlockKind(node, kind);

  if (kind === null) {
    throw new Error(`不支持的流程图块：${node.properties.blockKind ?? node.id}`);
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

function buildStrategyInputExpression(
  properties: ReturnType<typeof normalizeStrategyInputBlockProperties>,
): string {
  const title = toPineStringLiteral(properties.title ?? properties.variableName ?? "Input");
  switch (properties.inputType) {
    case "float":
      return `input.float(defval=${formatPineValue(properties.defaultValue)}, title=${title})`;
    case "source":
      return `input.source(${pineIndicatorSource(String(properties.defaultValue ?? "close"))}, ${title})`;
    case "timeframe":
      return `input.timeframe(${toPineStringLiteral(String(properties.defaultValue ?? "D"))}, ${title})`;
    case "time":
      return `input.time(${formatPineValue(properties.defaultValue)}, ${title})`;
    case "color":
      return `input.color(${formatPineValue(properties.defaultValue)}, ${title})`;
    case "int":
    default:
      return `input.int(${formatPineValue(properties.defaultValue)}, ${title})`;
  }
}

function buildDerivedSeriesStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeDerivedSeriesBlockProperties(node.properties ?? {});
  return `${properties.variableName} = ${buildDerivedSeriesExpression(properties)}`;
}

function buildDerivedSeriesExpression(
  properties: ReturnType<typeof normalizeDerivedSeriesBlockProperties>,
): string {
  const source = pineSeriesSource(properties.source ?? "close");
  switch (properties.mode) {
    case "nz":
      return `nz(${renderVisualExpressionToPine(properties.sourceExpressionAst, source)}, ${renderVisualExpressionToPine(properties.fallbackExpressionAst, formatNumber(properties.fallbackValue ?? 0))})`;
    case "math":
      if (properties.mathFunction === "min" || properties.mathFunction === "max") {
        return `math.${properties.mathFunction}(${renderVisualExpressionToPine(properties.leftExpressionAst, properties.leftExpression ?? "close")}, ${renderVisualExpressionToPine(properties.rightExpressionAst, properties.rightExpression ?? "open")})`;
      }
      return `math.${properties.mathFunction ?? "abs"}(${renderVisualExpressionToPine(properties.leftExpressionAst, properties.leftExpression ?? "close")})`;
    case "arithmetic":
      return `(${renderVisualExpressionToPine(properties.leftExpressionAst, properties.leftExpression ?? "close")} ${properties.operator ?? "-"} ${renderVisualExpressionToPine(properties.rightExpressionAst, properties.rightExpression ?? "open")})`;
    case "cross":
      return `ta.${properties.crossFunction ?? "crossover"}(${renderVisualExpressionToPine(properties.leftExpressionAst, properties.leftExpression ?? "close")}, ${renderVisualExpressionToPine(properties.rightExpressionAst, properties.rightExpression ?? "open")})`;
    case "history":
    default:
      return `${renderVisualExpressionToPine(properties.sourceExpressionAst, source)}[${properties.historyOffset ?? 1}]`;
  }
}

function buildMtfSeriesStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeMtfSeriesBlockProperties(node.properties ?? {});
  const expression = buildMtfInnerExpression(properties);
  return `${properties.variableName} = request.security(syminfo.tickerid, ${toPineStringLiteral(properties.timeframe ?? "D")}, ${expression})`;
}

function buildMtfInnerExpression(
  properties: ReturnType<typeof normalizeMtfSeriesBlockProperties>,
): string {
  const source = pineSeriesSource(properties.source ?? "close");
  switch (properties.expressionType) {
    case "history":
      return `${source}[${properties.historyOffset ?? 1}]`;
    case "indicator":
      return appendMtfField(
        properties.indicatorExpressionAst === undefined
          ? properties.indicatorExpression ?? "ta.ema(close, 20)"
          : renderVisualExpressionToPine(properties.indicatorExpressionAst, properties.indicatorExpression ?? "ta.ema(close, 20)"),
        properties.mtfField,
      );
    case "source":
    default:
      return source;
  }
}

function buildStateVariableStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeStateVariableBlockProperties(node.properties ?? {});
  return `var ${properties.variableName} = ${formatPineValue(properties.initialValue)}`;
}

function buildStateUpdateStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeStateUpdateBlockProperties(node.properties ?? {});
  const expression = properties.expressionAst === undefined
    ? properties.expression ?? "close > open"
    : renderVisualExpressionToPine(properties.expressionAst, properties.expression ?? "close > open");
  return `${properties.variableName} := ${expression}`;
}

function buildCollectionStatStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeCollectionStatBlockProperties(node.properties ?? {});
  const values = [
    renderVisualExpressionToPine(properties.sourceAExpressionAst, pineSeriesSource(properties.sourceA ?? "close")),
    renderVisualExpressionToPine(properties.sourceBExpressionAst, pineSeriesSource(properties.sourceB ?? "open")),
    renderVisualExpressionToPine(properties.sourceCExpressionAst, pineSeriesSource(properties.sourceC ?? "high")),
  ].join(", ");
  const receiver = `array.from(${values})`;
  if (properties.statFunction === "percentile") {
    return `${properties.variableName} = ${receiver}.percentile_linear_interpolation(${formatNumber(properties.percentile ?? 50)})`;
  }
  return `${properties.variableName} = ${receiver}.${properties.statFunction ?? "median"}()`;
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

function buildTimeFilterExpression(rawProperties: Record<string, unknown>): string {
  const properties = normalizeTimeFilterBlockProperties(rawProperties);
  const startMinute = (properties.startHour ?? 9) * 60 + (properties.startMinute ?? 30);
  const endMinute = (properties.endHour ?? 16) * 60 + (properties.endMinute ?? 0);
  const currentMinute = "(hour * 60 + minute)";
  switch (properties.mode) {
    case "after":
      return `${currentMinute} >= ${formatNumber(startMinute)}`;
    case "before":
      return `${currentMinute} < ${formatNumber(endMinute)}`;
    case "dayOfWeek":
      return `dayofweek == ${formatNumber(properties.dayOfWeek ?? 2)}`;
    case "between":
    default:
      return `${currentMinute} >= ${formatNumber(startMinute)} and ${currentMinute} < ${formatNumber(endMinute)}`;
  }
}

function buildSessionFilterExpression(rawProperties: Record<string, unknown>): string {
  const properties = normalizeSessionFilterBlockProperties(rawProperties);
  switch (properties.scope) {
    case "premarket":
      return "session.ispremarket";
    case "postmarket":
      return "session.ispostmarket";
    case "market":
    default:
      return "session.ismarket";
  }
}

function appendMtfField(expression: string, field: string | undefined): string {
  if (field === undefined || field.trim() === "") {
    return expression;
  }
  return `${expression}.${field}`;
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

function buildSeriesConditionExpression(rawProperties: Record<string, unknown>): string {
  const properties = normalizeSeriesConditionBlockProperties(rawProperties);
  const source = renderVisualExpressionToPine(properties.sourceExpressionAst, pineSeriesSource(properties.source ?? "close"));
  const operator = properties.operator ?? ">";
  const threshold = renderVisualExpressionToPine(properties.rightExpressionAst, formatNumber(properties.threshold ?? 0));
  const leftExpression = renderVisualExpressionToPine(properties.leftExpressionAst, source);
  const eventExpression = renderVisualExpressionToPine(
    properties.eventExpressionAst,
    `${pineSeriesSource(properties.eventSource ?? "close")} ${properties.eventOperator ?? ">"} ${formatNumber(properties.eventThreshold ?? 520)}`,
  );
  switch (properties.mode) {
    case "rising":
      return `ta.rising(${source}, ${properties.length ?? 3})`;
    case "falling":
      return `ta.falling(${source}, ${properties.length ?? 3})`;
    case "barssince":
      return `ta.barssince(${eventExpression}) < ${properties.length ?? 3}`;
    case "valuewhen":
      return `ta.valuewhen(${eventExpression}, ${renderVisualExpressionToPine(properties.valueExpressionAst, pineSeriesSource(properties.valueSource ?? "close"))}, ${properties.occurrence ?? 0}) ${operator} ${threshold}`;
    case "compare":
    default:
      return `${leftExpression} ${operator} ${threshold}`;
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

  if (properties.indicatorType === "kdj") {
    const lines = [
      ...buildStrategyFlowNodeAnnotation(node, depth, { variableName }),
      ...buildKDJIndicatorStatements(variableName, properties).map((line) => `${indent(depth)}${line}`),
    ];
    if (!includeChildren) {
      return lines;
    }
    return lines;
  }

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

function buildKDJIndicatorStatements(
  variableName: string,
  properties: GetTechnicalIndicatorBlockProperties,
): string[] {
  const period = Math.max(1, Math.round(properties.period ?? 9));
  const m1 = Math.max(1, Math.round(properties.m1 ?? 3));
  const m2 = Math.max(1, Math.round(properties.m2 ?? 3));
  const highName = `${variableName}_highest`;
  const lowName = `${variableName}_lowest`;
  const rsvName = `${variableName}_rsv`;
  const kName = `${variableName}_k`;
  const dName = `${variableName}_d`;
  const jName = `${variableName}_j`;
  return [
    `${highName} = ta.highest(high, ${period})`,
    `${lowName} = ta.lowest(low, ${period})`,
    `${rsvName} = ${highName} == ${lowName} ? 50 : ((close - ${lowName}) / (${highName} - ${lowName})) * 100`,
    `var ${kName} = 50.0`,
    `var ${dName} = 50.0`,
    `${kName} := ((${m1 - 1}) * nz(${kName}[1], 50) + ${rsvName}) / ${m1}`,
    `${dName} := ((${m2 - 1}) * nz(${dName}[1], 50) + ${kName}) / ${m2}`,
    `${jName} = 3 * ${kName} - 2 * ${dName}`,
  ];
}

function readSyntheticKDJVariableName(value: unknown, field: "k" | "d" | "j"): string {
  const base = typeof value === "string" && isPineIdentifier(value)
    ? value
    : "kdj";
  return `${base}_${field}`;
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
  const wrapTimeframe = (expression: string): string =>
    wrapIndicatorTimeframe(properties, expression);

  switch (properties.indicatorType) {
    case "movingAverage": {
      const movingAverageType = properties.movingAverageType ?? "MA";
      const windowSize = properties.windowSize ?? properties.period ?? 20;
      const expression = buildMovingAverageExpression(movingAverageType, windowSize, properties.source ?? "close");
      return wrapTimeframe(expression);
    }
    case "macd":
      return wrapTimeframe(`ta.macd(close, ${properties.fastPeriod ?? 12}, ${properties.slowPeriod ?? 26}, ${properties.signalPeriod ?? 9})`);
    case "kdj":
      return `${readSyntheticKDJVariableName(properties.variableName, "j")}`;
    case "bollinger":
      return wrapTimeframe(`ta.bb(close, ${properties.period ?? 20}, ${properties.multiplier ?? 2})`);
    case "atr":
      return wrapTimeframe(`ta.atr(${properties.period ?? 14})`);
    case "cci":
      return wrapTimeframe(`ta.cci(${pineIndicatorSource(properties.source ?? "hlc3")}, ${properties.period ?? 20})`);
    case "williamsR":
      return `ta.wpr(${properties.period ?? 14})`;
    case "stdev":
      return wrapTimeframe(`ta.stdev(${pineIndicatorSource(properties.source ?? "close")}, ${properties.period ?? 20})`);
    case "variance":
      return wrapTimeframe(`ta.variance(${pineIndicatorSource(properties.source ?? "close")}, ${properties.period ?? 20})`);
    case "highest":
      return wrapTimeframe(`ta.highest(${pineIndicatorSource(properties.source ?? "high")}, ${properties.period ?? 20})`);
    case "lowest":
      return wrapTimeframe(`ta.lowest(${pineIndicatorSource(properties.source ?? "low")}, ${properties.period ?? 20})`);
    case "sum":
      return wrapTimeframe(`ta.sum(${pineIndicatorSource(properties.source ?? "volume")}, ${properties.period ?? 20})`);
    case "vwap":
      return `ta.vwap(${pineIndicatorSource(properties.source ?? "hlc3")})`;
    case "mfi":
      return wrapTimeframe(`ta.mfi(${pineIndicatorSource(properties.source ?? "hlc3")}, ${properties.period ?? 14})`);
    case "dmi":
      return `ta.dmi(${properties.period ?? 14}, ${properties.adxSmoothing ?? 14})`;
    case "supertrend":
      return wrapTimeframe(`ta.supertrend(${properties.factor ?? 3}, ${properties.period ?? 10})`);
    case "sar":
      return `ta.sar(${properties.start ?? 0.02}, ${properties.increment ?? 0.02}, ${properties.maximum ?? 0.2})`;
    case "linreg":
      return wrapTimeframe(`ta.linreg(${pineIndicatorSource(properties.source ?? "close")}, ${properties.period ?? 5}, ${properties.offset ?? 0})`);
    case "obv":
      return wrapTimeframe("ta.obv");
    case "pivotHigh":
      return wrapTimeframe(`ta.pivothigh(${pineIndicatorSource(properties.source ?? "high")}, ${properties.leftBars ?? 2}, ${properties.rightBars ?? 2})`);
    case "pivotLow":
      return wrapTimeframe(`ta.pivotlow(${pineIndicatorSource(properties.source ?? "low")}, ${properties.leftBars ?? 2}, ${properties.rightBars ?? 2})`);
    case "keltner":
      return wrapTimeframe(`ta.kc(${pineIndicatorSource(properties.source ?? "close")}, ${properties.period ?? 20}, ${properties.multiplier ?? 1.5}, true)`);
    case "alma":
      return wrapTimeframe(`ta.alma(${pineIndicatorSource(properties.source ?? "close")}, ${properties.period ?? 20}, ${properties.offset ?? 0.85}, ${properties.sigma ?? 6})`);
    case "rsi":
    default:
      return wrapTimeframe(`ta.rsi(close, ${properties.period ?? 14})`);
  }
}

function wrapIndicatorTimeframe(
  properties: GetTechnicalIndicatorBlockProperties,
  expression: string,
): string {
  if (!supportsIndicatorRequestSecurity(properties.indicatorType)) {
    return expression;
  }
  const timeframe = properties.timeframe?.trim();
  return timeframe == null || timeframe === ""
    ? expression
    : `request.security(syminfo.tickerid, ${toPineStringLiteral(timeframe)}, ${expression})`;
}

function supportsIndicatorRequestSecurity(
  indicatorType: GetTechnicalIndicatorBlockProperties["indicatorType"],
): boolean {
  return [
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
  return pineIndicatorSource(source);
}

function pineIndicatorSource(source: string): string {
  switch (source) {
    case "open":
    case "high":
    case "low":
    case "volume":
    case "hl2":
    case "hlc3":
    case "ohlc4":
      return source;
    case "close":
    default:
      return "close";
  }
}

function pineSeriesSource(source: StrategySeriesSource): string {
  return pineIndicatorSource(source);
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
  const action = inferPineOrderAction(node.properties.orderAction, side);
  const orderId = normalizePineOrderId(node.properties.orderId);
  if (action === "closeAll") {
    const closeAllArgs = [
      renderBooleanOrderArg("immediately", node.properties.immediately),
      renderStringOrderArg("comment", node.properties.comment),
      renderStringOrderArg("alert_message", node.properties.alert_message),
      renderBooleanOrderArg("disable_alert", node.properties.disable_alert),
    ].filter((arg) => arg !== "");
    return closeAllArgs.length === 0
      ? "strategy.close_all()"
      : `strategy.close_all(${closeAllArgs.join(", ")})`;
  }
  if (action === "cancel") {
    return `strategy.cancel(${toPineStringLiteral(orderId ?? "Long")})`;
  }
  if (action === "cancelAll") {
    return "strategy.cancel_all()";
  }
  if (action === "riskAllowEntryIn") {
    const direction = normalizePineRiskAllowEntryDirection(node.properties.riskAllowedDirection);
    const pineDirection = direction === "long"
      ? "strategy.direction.long"
      : direction === "short"
        ? "strategy.direction.short"
        : "strategy.direction.all";
    return `strategy.risk.allow_entry_in(${pineDirection})`;
  }

  const quantityMode = normalizeQuantityModeForSide(node.properties.quantityMode, side);
  const quantityValue = normalizeDecimal(node.properties.quantityValue, 100);
  const quantityArgumentName = quantityMode === "equityPercent"
    ? "qty_percent"
    : "qty";
  const quantityOption = quantityArgumentName === "qty_percent"
    ? `qty_percent=${formatNumber(quantityValue)}`
    : `qty=${buildPineQuantityExpression(quantityMode, quantityValue)}`;
  const orderType = normalizeOrderType(node.properties.orderType);
  const limitPrice = normalizeDecimal(node.properties.limitPrice, 0);
  const stopPrice = normalizeDecimal(node.properties.stopPrice, 0);
  const limitOption = orderType === "LIMIT" && (limitPrice > 0 || node.properties.limitPriceExpressionAst !== undefined)
    ? `, limit=${renderVisualExpressionToPine(node.properties.limitPriceExpressionAst, formatNumber(limitPrice))}`
    : "";
  const stopOption = stopPrice > 0 || node.properties.stopPriceExpressionAst !== undefined
    ? `, stop=${renderVisualExpressionToPine(node.properties.stopPriceExpressionAst, formatNumber(stopPrice))}`
    : "";

  const entryPolicy = normalizeEntryPositionPolicy(node.properties.entryPositionPolicy);
  const entryPolicyAnnotation = action === "entry" && entryPolicy !== "sameDirection"
    ? `// @entry_policy ${entryPositionPolicyToSnakeCase(entryPolicy)}\n`
    : "";

  if (action === "close") {
    const closeId = orderId ?? (side === "BUY_COVER" ? "Short" : "Long");
    const closeArgs = [
      quantityOption,
      renderOrderExpressionArg("limit", node.properties.limitPriceExpressionAst, limitPrice),
      renderOrderExpressionArg("stop", node.properties.stopPriceExpressionAst, stopPrice),
      renderStringOrderArg("comment", node.properties.comment),
      renderStringOrderArg("alert_message", node.properties.alert_message),
      renderBooleanOrderArg("immediately", node.properties.immediately),
      renderBooleanOrderArg("disable_alert", node.properties.disable_alert),
      renderRawOrderArg("when", node.properties.when),
    ].filter((arg) => arg !== "");
    return `strategy.close(${toPineStringLiteral(closeId)}, ${closeArgs.join(", ")})`;
  }
  const direction = side === "SELL_SHORT" || side === "SELL"
    ? "strategy.short"
    : "strategy.long";
  const defaultOrderId = direction === "strategy.short" ? "Short" : "Long";
  const functionName = action === "order" ? "strategy.order" : "strategy.entry";
  const orderArgs = [
    quantityOption,
    limitOption.slice(2),
    stopOption.slice(2),
    renderStringOrderArg("comment", node.properties.comment),
    renderStringOrderArg("alert_message", node.properties.alert_message),
    renderBooleanOrderArg("disable_alert", node.properties.disable_alert),
    renderRawOrderArg("when", node.properties.when),
  ].filter((arg) => arg !== "");
  return `${entryPolicyAnnotation}${functionName}(${[
    toPineStringLiteral(orderId ?? defaultOrderId),
    direction,
    ...orderArgs,
  ].join(", ")})`;
}

function buildRiskRuleStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeRiskRuleBlockProperties(node.properties ?? {});
  switch (properties.riskRuleType) {
    case "allowEntryIn": {
      const direction = normalizePineRiskAllowEntryDirection(properties.riskAllowedDirection);
      const pineDirection = direction === "long"
        ? "strategy.direction.long"
        : direction === "short"
          ? "strategy.direction.short"
          : "strategy.direction.all";
      return `strategy.risk.allow_entry_in(${pineDirection})`;
    }
    case "maxIntradayLoss":
    case "maxDrawdown": {
      const functionName = properties.riskRuleType === "maxIntradayLoss"
        ? "strategy.risk.max_intraday_loss"
        : "strategy.risk.max_drawdown";
      const args = [
        formatNumber(properties.riskValue ?? 10),
        properties.riskAmountType ?? "strategy.percent_of_equity",
        renderStringOrderArg("alert_message", properties.alert_message),
      ].filter((arg) => arg !== "");
      return `${functionName}(${args.join(", ")})`;
    }
    case "maxIntradayFilledOrders":
    case "maxConsLossDays": {
      const functionName = properties.riskRuleType === "maxIntradayFilledOrders"
        ? "strategy.risk.max_intraday_filled_orders"
        : "strategy.risk.max_cons_loss_days";
      const args = [
        formatNumber(properties.riskCount ?? 3),
        renderStringOrderArg("alert_message", properties.alert_message),
      ].filter((arg) => arg !== "");
      return `${functionName}(${args.join(", ")})`;
    }
    case "maxPositionSize":
      return `strategy.risk.max_position_size(${formatNumber(properties.riskContracts ?? 10)})`;
    default:
      return "strategy.risk.max_drawdown(10, strategy.percent_of_equity)";
  }
}

function inferPineOrderAction(
  value: unknown,
  side: ReturnType<typeof normalizeOrderSide>,
): ReturnType<typeof normalizePineOrderAction> {
  if (typeof value === "string" && value.trim() !== "") {
    return normalizePineOrderAction(value);
  }
  return side === "SELL" || side === "BUY_COVER" ? "close" : "entry";
}

function normalizePineOrderId(value: unknown): string | null {
  if (typeof value !== "string") {
    return null;
  }
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized === "" ? null : normalized;
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

function renderOrderExpressionArg(
  name: string,
  expressionAst: unknown,
  fallbackNumber: number,
): string {
  if (fallbackNumber <= 0 && expressionAst === undefined) {
    return "";
  }
  return `${name}=${renderVisualExpressionToPine(expressionAst, formatNumber(fallbackNumber))}`;
}

function renderStringOrderArg(name: string, value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  const normalized = value.replace(/[\r\n]+/g, " ").trim();
  return normalized === "" ? "" : `${name}=${toPineStringLiteral(normalized)}`;
}

function renderRawOrderArg(name: string, value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  const normalized = value.trim();
  return normalized === "" ? "" : `${name}=${normalized}`;
}

function renderBooleanOrderArg(name: string, value: unknown): string {
  if (typeof value === "boolean") {
    return `${name}=${value ? "true" : "false"}`;
  }
  if (typeof value !== "string") {
    return "";
  }
  const normalized = value.trim().toLowerCase();
  if (normalized !== "true" && normalized !== "false") {
    return "";
  }
  return `${name}=${normalized}`;
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
  const profitTicks = properties.profitTicks === undefined ? null : formatNumber(properties.profitTicks);
  const lossTicks = properties.lossTicks === undefined ? null : formatNumber(properties.lossTicks);
  const quantityPercentage = properties.quantityPercentage ?? 100;
  const quantityOption = quantityPercentage > 0 && quantityPercentage < 100
    ? `, qty_percent=${formatNumber(quantityPercentage)}`
    : "";
  const explicitStopPrice = properties.stopPriceExpressionAst === undefined
    ? null
    : renderVisualExpressionToPine(properties.stopPriceExpressionAst, "close");
  const explicitTakeProfitPrice = properties.takeProfitPriceExpressionAst === undefined
    ? null
    : renderVisualExpressionToPine(properties.takeProfitPriceExpressionAst, "close");
  const explicitTrailingPrice = properties.trailingPriceExpressionAst === undefined
    ? null
    : renderVisualExpressionToPine(properties.trailingPriceExpressionAst, "close");
  const explicitTrailingOffset = properties.trailingOffsetExpressionAst === undefined
    ? explicitTrailingPrice
    : renderVisualExpressionToPine(properties.trailingOffsetExpressionAst, explicitTrailingPrice ?? "close");
  const directions = properties.fromEntryMode === "auto"
    ? ["auto"]
    : properties.direction === "long"
    ? ["long"]
    : properties.direction === "short"
      ? ["short"]
      : ["long", "short"];
  return directions.map((direction) => {
    const entryId = direction === "short" ? "Short" : "Long";
    const exitId = direction === "auto" ? `Auto ${properties.mode ?? "stopLoss"}` : `${entryId} ${properties.mode ?? "stopLoss"}`;
    const fromEntryArg = direction === "auto" ? "" : `, ${toPineStringLiteral(entryId)}`;
    const metadataArgs = buildProtectMetadataArgs(properties);
    switch (properties.mode) {
      case "takeProfit":
        if (explicitTakeProfitPrice !== null) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, limit=${explicitTakeProfitPrice}${quantityOption}${metadataArgs})`;
        }
        if (profitTicks !== null) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, profit=${profitTicks}${quantityOption}${metadataArgs})`;
        }
        return direction === "short"
          ? `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, limit=close * (1 - ${percentage} / 100)${quantityOption}${metadataArgs})`
          : `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, limit=close * (1 + ${percentage} / 100)${quantityOption}${metadataArgs})`;
      case "trailingStop":
        if (explicitTrailingPrice !== null) {
          const trailingArg = properties.trailingPriceMode === "price" ? "trail_price" : "trail_points";
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, ${trailingArg}=${explicitTrailingPrice}, trail_offset=${explicitTrailingOffset ?? explicitTrailingPrice}${quantityOption}${metadataArgs})`;
        }
        return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, trail_points=close * ${percentage} / 100, trail_offset=close * ${percentage} / 100${quantityOption}${metadataArgs})`;
      case "bracketExit": {
        const takeProfitPercentage = formatNumber(properties.takeProfitPercentage ?? 4);
        const bracketArgs = [
          explicitStopPrice === null ? "" : `stop=${explicitStopPrice}`,
          explicitTakeProfitPrice === null ? "" : `limit=${explicitTakeProfitPrice}`,
          lossTicks === null ? "" : `loss=${lossTicks}`,
          profitTicks === null ? "" : `profit=${profitTicks}`,
        ].filter((arg) => arg !== "");
        if (bracketArgs.length > 0) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, ${bracketArgs.join(", ")}${quantityOption}${metadataArgs})`;
        }
        return direction === "short"
          ? `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=close * (1 + ${percentage} / 100), limit=close * (1 - ${takeProfitPercentage} / 100)${quantityOption}${metadataArgs})`
          : `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=close * (1 - ${percentage} / 100), limit=close * (1 + ${takeProfitPercentage} / 100)${quantityOption}${metadataArgs})`;
      }
      case "stopLoss":
      default:
        if (explicitStopPrice !== null) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=${explicitStopPrice}${quantityOption}${metadataArgs})`;
        }
        if (lossTicks !== null) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, loss=${lossTicks}${quantityOption}${metadataArgs})`;
        }
        return direction === "short"
          ? `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=close * (1 + ${percentage} / 100)${quantityOption}${metadataArgs})`
          : `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=close * (1 - ${percentage} / 100)${quantityOption}${metadataArgs})`;
    }
  });
}

function buildProtectMetadataArgs(
  properties: ReturnType<typeof normalizeStopLossBlockProperties>,
): string {
  const args = [
    renderStringOrderArg("comment", properties.comment),
    renderStringOrderArg("comment_profit", properties.comment_profit),
    renderStringOrderArg("comment_loss", properties.comment_loss),
    renderStringOrderArg("comment_trailing", properties.comment_trailing),
    renderStringOrderArg("alert_message", properties.alert_message),
    renderStringOrderArg("alert_profit", properties.alert_profit),
    renderStringOrderArg("alert_loss", properties.alert_loss),
    renderStringOrderArg("alert_trailing", properties.alert_trailing),
    renderBooleanOrderArg("disable_alert", properties.disable_alert),
    renderRawOrderArg("when", properties.when),
  ].filter((arg) => arg !== "");
  return args.length === 0 ? "" : `, ${args.join(", ")}`;
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

function formatPineValue(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (/^(?:timestamp|color\.[A-Za-z_][A-Za-z0-9_]*|#[0-9A-Fa-f]{6}|open|high|low|close|volume|hl2|hlc3|ohlc4)\b/.test(trimmed)) {
      return trimmed;
    }
    return toPineStringLiteral(trimmed);
  }
  return "0";
}

function indent(depth: number): string {
  return "  ".repeat(depth);
}
