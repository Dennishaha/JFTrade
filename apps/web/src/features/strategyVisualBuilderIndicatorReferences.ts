import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";

import { getStrategyBlockKind } from "./strategyVisualBuilderCatalog";
import {
  buildStrategyVisualDataEdgeProperties,
  isStrategyVisualDataEdge,
  normalizeStrategyVisualEdgeProperties,
  readStrategyVisualEdgeInputSlot,
} from "./strategyVisualBuilderEdges";
import {
  getTechnicalIndicatorDefinition,
  getTechnicalIndicatorInputSlots,
  nextGetTechnicalIndicatorNodeText,
  normalizeGetTechnicalIndicatorProperties,
  normalizeStrategyIndicatorReferenceNodeId,
  normalizeStrategyIndicatorVariableName,
  normalizeTechnicalIndicatorConditionProperties,
  type TechnicalIndicatorInputSlot,
} from "./strategyVisualBuilderIndicatorBlock";
import { cloneStrategyVisualModel } from "./strategyVisualBuilderShared";

export interface StrategyIndicatorInputBindings {
  primary?: string;
  fast?: string;
  slow?: string;
}

export interface StrategyIndicatorGetterOption {
  value: string;
  label: string;
}

export function suggestStrategyIndicatorVariableName(
  rawProperties: Record<string, unknown>,
): string {
  const properties = normalizeGetTechnicalIndicatorProperties(rawProperties);

  switch (properties.indicatorType) {
    case "movingAverage":
      return `${properties.movingAverageType ?? "MA"}${properties.windowSize ?? 20}`;
    case "macd":
      return `MACD${properties.fastPeriod ?? 12}_${properties.slowPeriod ?? 26}_${properties.signalPeriod ?? 9}`;
    case "kdj":
      return `KDJ${properties.period ?? 9}_${properties.m1 ?? 3}_${properties.m2 ?? 3}`;
    case "bollinger":
      return `BOLL${properties.period ?? 20}x${properties.multiplier ?? 2}`;
    default:
      return `${properties.indicatorType.toUpperCase()}${properties.period ?? 14}`;
  }
}

export function resolveStrategyIndicatorGetterLabel(
  node: StrategyVisualNodeDocument,
): string {
  const properties = normalizeGetTechnicalIndicatorProperties(node.properties ?? {});
  const variableName = properties.variableName;
  const summary = nextGetTechnicalIndicatorNodeText(properties as unknown as Record<string, unknown>);
  return variableName === undefined ? summary : `${variableName} · ${summary}`;
}

export function listStrategyIndicatorGetterOptions(
  model: StrategyVisualModelDocument | null | undefined,
  indicatorType?: ReturnType<typeof normalizeTechnicalIndicatorConditionProperties>["indicatorType"],
): StrategyIndicatorGetterOption[] {
  if (model === null || model === undefined) {
    return [];
  }

  return model.nodes
    .filter((node) => getStrategyBlockKind(node) === "getTechnicalIndicator")
    .filter((node) => {
      if (indicatorType === undefined) {
        return true;
      }
      return normalizeGetTechnicalIndicatorProperties(node.properties ?? {}).indicatorType === indicatorType;
    })
    .map((node) => ({
      value: node.id,
      label: resolveStrategyIndicatorGetterLabel(node),
    }));
}

export function readTechnicalIndicatorConditionInputBindings(
  properties: Record<string, unknown> | null | undefined,
): StrategyIndicatorInputBindings {
  const normalized = normalizeTechnicalIndicatorConditionProperties(properties ?? {});
  return {
    ...(normalized.inputPrimaryNodeId === undefined ? {} : { primary: normalized.inputPrimaryNodeId }),
    ...(normalized.inputFastNodeId === undefined ? {} : { fast: normalized.inputFastNodeId }),
    ...(normalized.inputSlowNodeId === undefined ? {} : { slow: normalized.inputSlowNodeId }),
  };
}

export function reconcileStrategyVisualModelIndicatorBindings(
  model: StrategyVisualModelDocument,
): StrategyVisualModelDocument {
  const cloned = cloneStrategyVisualModel(model) ?? model;
  const fallbackBindings = collectConditionBindingsFromDataEdges(cloned);

  const nextNodes = cloned.nodes.map((node) => {
    const kind = getStrategyBlockKind(node);
    const rawProperties = node.properties ?? {};

    if (kind === "getTechnicalIndicator") {
      const nextProperties = buildGetterNodeProperties(rawProperties);
      return {
        ...node,
        text: nextGetterNodeText(node.text, nextProperties),
        properties: nextProperties,
      };
    }

    if (kind === "technicalIndicatorCondition") {
      return {
        ...node,
        properties: buildConditionNodeProperties(
          rawProperties,
          fallbackBindings.get(node.id),
          cloned.nodes,
        ),
      };
    }

    return {
      ...node,
      properties: { ...rawProperties },
    };
  });

  const existingDataEdgeIds = buildDataEdgeIdMap(cloned.edges);
  const nextEdges: StrategyVisualEdgeDocument[] = [];

  for (const edge of cloned.edges) {
    if (isStrategyVisualDataEdge(edge)) {
      continue;
    }

    const normalized = buildVisibleEdge(edge);
    if (normalized !== null) {
      nextEdges.push(normalized);
    }
  }

  const getterNodeIds = new Set(
    nextNodes
      .filter((node) => getStrategyBlockKind(node) === "getTechnicalIndicator")
      .map((node) => node.id),
  );

  for (const node of nextNodes) {
    if (getStrategyBlockKind(node) !== "technicalIndicatorCondition") {
      continue;
    }

    const bindings = readTechnicalIndicatorConditionInputBindings(node.properties ?? {});
    for (const slot of ["primary", "fast", "slow"] as const) {
      const sourceNodeId = bindings[slot];
      if (sourceNodeId === undefined || !getterNodeIds.has(sourceNodeId)) {
        continue;
      }

      const edgeId = existingDataEdgeIds.get(buildDataEdgeKey(sourceNodeId, node.id, slot))
        ?? `edge-data-${sourceNodeId}-${node.id}-${slot}`;

      nextEdges.push({
        id: edgeId,
        type: "polyline",
        sourceNodeId,
        targetNodeId: node.id,
        properties: buildStrategyVisualDataEdgeProperties(slot),
      });
    }
  }

  return {
    engine: cloned.engine,
    version: cloned.version,
    nodes: nextNodes,
    edges: nextEdges,
  };
}

function nextGetterNodeText(
  rawText: StrategyVisualNodeDocument["text"],
  rawProperties: Record<string, unknown>,
): string {
  const nextText = nextGetTechnicalIndicatorNodeText(rawProperties);
  const normalized = typeof rawText === "string" ? rawText.trim() : "";
  if (normalized === "") {
    return nextText;
  }

  const legacyText = nextLegacyGetterNodeText(rawProperties, nextText);
  if (normalized === nextText || normalized === legacyText) {
    return nextText;
  }

  return typeof rawText === "string" ? rawText : nextText;
}

function nextLegacyGetterNodeText(
  rawProperties: Record<string, unknown>,
  nextText: string,
): string {
  const normalized = normalizeGetTechnicalIndicatorProperties(rawProperties);
  const legacyLabel = getTechnicalIndicatorDefinition(normalized.indicatorType).label;
  const currentPrefix = `获取 `;
  if (!nextText.startsWith(currentPrefix)) {
    return nextText;
  }

  const firstSpaceIndex = nextText.indexOf(" ", currentPrefix.length);
  if (firstSpaceIndex < 0) {
    return nextText;
  }

  return `${currentPrefix}${legacyLabel}${nextText.slice(firstSpaceIndex)}`;
}

function buildGetterNodeProperties(
  rawProperties: Record<string, unknown>,
): Record<string, unknown> {
  const normalized = normalizeGetTechnicalIndicatorProperties(rawProperties);
  const variableName = normalizeStrategyIndicatorVariableName(rawProperties.variableName);

  return {
    ...normalized,
    ...(variableName === undefined ? {} : { variableName }),
    ...pickSourceRange(rawProperties),
  };
}

function buildConditionNodeProperties(
  rawProperties: Record<string, unknown>,
  fallbackBindings: StrategyIndicatorInputBindings | undefined,
  nodes: StrategyVisualNodeDocument[],
): Record<string, unknown> {
  const normalized = normalizeTechnicalIndicatorConditionProperties(rawProperties);
  const getterIndicatorTypes = new Map(
    nodes
      .filter((node) => getStrategyBlockKind(node) === "getTechnicalIndicator")
      .map((node) => [
        node.id,
        normalizeGetTechnicalIndicatorProperties(node.properties ?? {}).indicatorType,
      ] as const),
  );
  const propertyBindings = readTechnicalIndicatorConditionInputBindings(rawProperties);
  const bindings = sanitizeConditionBindings(
    normalized.indicatorType,
    mergeConditionBindings(fallbackBindings, propertyBindings),
    getterIndicatorTypes,
  );

  return {
    ...normalized,
    ...serializeConditionBindings(bindings),
    ...pickSourceRange(rawProperties),
  };
}

function collectConditionBindingsFromDataEdges(
  model: StrategyVisualModelDocument,
): Map<string, StrategyIndicatorInputBindings> {
  const kinds = new Map(model.nodes.map((node) => [node.id, getStrategyBlockKind(node)] as const));
  const bindingsByTarget = new Map<string, StrategyIndicatorInputBindings>();

  for (const edge of model.edges) {
    if (!isStrategyVisualDataEdge(edge)) {
      continue;
    }
    if (kinds.get(edge.sourceNodeId) !== "getTechnicalIndicator") {
      continue;
    }
    if (kinds.get(edge.targetNodeId) !== "technicalIndicatorCondition") {
      continue;
    }

    const slot = readStrategyVisualEdgeInputSlot(edge) ?? "primary";
    const current = bindingsByTarget.get(edge.targetNodeId) ?? {};
    current[slot] = edge.sourceNodeId;
    bindingsByTarget.set(edge.targetNodeId, current);
  }

  return bindingsByTarget;
}

function sanitizeConditionBindings(
  indicatorType: ReturnType<typeof normalizeTechnicalIndicatorConditionProperties>["indicatorType"],
  bindings: StrategyIndicatorInputBindings,
  getterIndicatorTypes: Map<string, ReturnType<typeof normalizeTechnicalIndicatorConditionProperties>["indicatorType"]>,
): StrategyIndicatorInputBindings {
  const allowedSlots = new Set(getTechnicalIndicatorInputSlots(indicatorType));
  const nextBindings: StrategyIndicatorInputBindings = {};

  for (const slot of ["primary", "fast", "slow"] as const) {
    if (!allowedSlots.has(slot)) {
      continue;
    }

    const nodeId = normalizeStrategyIndicatorReferenceNodeId(bindings[slot]);
    if (nodeId !== undefined && getterIndicatorTypes.get(nodeId) === indicatorType) {
      nextBindings[slot] = nodeId;
    }
  }

  return nextBindings;
}

function mergeConditionBindings(
  fallbackBindings: StrategyIndicatorInputBindings | undefined,
  propertyBindings: StrategyIndicatorInputBindings,
): StrategyIndicatorInputBindings {
  const merged: StrategyIndicatorInputBindings = {
    ...(fallbackBindings?.primary === undefined ? {} : { primary: fallbackBindings.primary }),
    ...(fallbackBindings?.fast === undefined ? {} : { fast: fallbackBindings.fast }),
    ...(fallbackBindings?.slow === undefined ? {} : { slow: fallbackBindings.slow }),
  };

  for (const slot of ["primary", "fast", "slow"] as const) {
    const value = propertyBindings[slot];
    if (value !== undefined) {
      merged[slot] = value;
    }
  }

  return merged;
}

function serializeConditionBindings(
  bindings: StrategyIndicatorInputBindings,
): Record<string, string> {
  return {
    ...(bindings.primary === undefined ? {} : { inputPrimaryNodeId: bindings.primary }),
    ...(bindings.fast === undefined ? {} : { inputFastNodeId: bindings.fast }),
    ...(bindings.slow === undefined ? {} : { inputSlowNodeId: bindings.slow }),
  };
}

function buildVisibleEdge(edge: StrategyVisualEdgeDocument): StrategyVisualEdgeDocument | null {
  const sourceNodeId = typeof edge.sourceNodeId === "string" ? edge.sourceNodeId : "";
  const targetNodeId = typeof edge.targetNodeId === "string" ? edge.targetNodeId : "";
  if (sourceNodeId === "" || targetNodeId === "") {
    return null;
  }

  const properties = normalizeStrategyVisualEdgeProperties(edge.properties);
  return {
    ...(edge.id === undefined ? {} : { id: edge.id }),
    type: edge.type ?? "polyline",
    sourceNodeId,
    targetNodeId,
    ...(edge.text === undefined ? {} : { text: edge.text }),
    ...(properties === undefined ? {} : { properties }),
  };
}

function buildDataEdgeIdMap(
  edges: StrategyVisualEdgeDocument[],
): Map<string, string> {
  const ids = new Map<string, string>();

  for (const edge of edges) {
    if (!isStrategyVisualDataEdge(edge)) {
      continue;
    }
    const slot = readStrategyVisualEdgeInputSlot(edge) ?? "primary";
    if (typeof edge.id !== "string" || edge.id === "") {
      continue;
    }
    ids.set(buildDataEdgeKey(edge.sourceNodeId, edge.targetNodeId, slot), edge.id);
  }

  return ids;
}

function buildDataEdgeKey(
  sourceNodeId: string,
  targetNodeId: string,
  slot: TechnicalIndicatorInputSlot,
): string {
  return `${sourceNodeId}->${targetNodeId}:${slot}`;
}

function pickSourceRange(
  rawProperties: Record<string, unknown>,
): Record<string, unknown> {
  const sourceRange = rawProperties.sourceRange;
  if (!isRecord(sourceRange)) {
    return {};
  }

  return {
    sourceRange: { ...sourceRange },
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}