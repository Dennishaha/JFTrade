import type {
  StrategyVisualEdgeDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";

import {
  buildStrategyVisualControlEdgeProperties,
} from "./strategyVisualBuilderEdges";
import {
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  normalizeTechnicalIndicatorConditionProperties,
  normalizeTechnicalIndicatorProperties,
  type TechnicalIndicatorBlockProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import {
  suggestStrategyIndicatorVariableName,
} from "./strategyVisualBuilderIndicatorReferences";

export const STRATEGY_TECHNICAL_INDICATOR_SHORTCUT_CREATION_MODE = "shortcut";

interface StrategyShortcutSourceNode {
  id?: string;
  x?: number;
  y?: number;
  properties?: Record<string, unknown>;
}

export interface StrategyTechnicalIndicatorShortcutExpansion {
  nodes: StrategyVisualNodeDocument[];
  edges: StrategyVisualEdgeDocument[];
  focusNodeId: string;
}

export function isTechnicalIndicatorShortcutCreation(
  properties: Record<string, unknown> | null | undefined,
): boolean {
  return properties?.blockKind === "technicalIndicator"
    && properties.creationMode === STRATEGY_TECHNICAL_INDICATOR_SHORTCUT_CREATION_MODE;
}

export function expandTechnicalIndicatorShortcutNode(
  sourceNode: StrategyShortcutSourceNode,
): StrategyTechnicalIndicatorShortcutExpansion {
  const baseId = readShortcutBaseId(sourceNode.id);
  const x = typeof sourceNode.x === "number" ? sourceNode.x : 0;
  const y = typeof sourceNode.y === "number" ? sourceNode.y : 0;
  const properties = normalizeTechnicalIndicatorProperties(sourceNode.properties ?? {});

  if (properties.indicatorType === "movingAverage") {
    return buildMovingAverageShortcutExpansion(baseId, x, y, properties);
  }

  return buildSingleIndicatorShortcutExpansion(baseId, x, y, properties);
}

function buildSingleIndicatorShortcutExpansion(
  baseId: string,
  x: number,
  y: number,
  properties: TechnicalIndicatorBlockProperties,
): StrategyTechnicalIndicatorShortcutExpansion {
  const getterId = `${baseId}-getter`;
  const conditionId = `${baseId}-condition`;
  const getterProperties = buildGetterProperties(properties);
  const conditionProperties = buildConditionProperties(properties, {
    primary: getterId,
  });

  return {
    nodes: [
      {
        id: getterId,
        type: "rect",
        x,
        y,
        text: nextGetTechnicalIndicatorNodeText(getterProperties),
        properties: getterProperties,
      },
      {
        id: conditionId,
        type: "diamond",
        x: x + 300,
        y,
        text: nextTechnicalIndicatorConditionNodeText(conditionProperties),
        properties: conditionProperties,
      },
    ],
    edges: [
      {
        id: `${baseId}-control`,
        type: "polyline",
        sourceNodeId: getterId,
        targetNodeId: conditionId,
        properties: buildStrategyVisualControlEdgeProperties(),
      },
    ],
    focusNodeId: conditionId,
  };
}

function buildMovingAverageShortcutExpansion(
  baseId: string,
  x: number,
  y: number,
  properties: TechnicalIndicatorBlockProperties,
): StrategyTechnicalIndicatorShortcutExpansion {
  const fastGetterId = `${baseId}-fast`;
  const slowGetterId = `${baseId}-slow`;
  const conditionId = `${baseId}-condition`;
  const movingAverageType = properties.movingAverageType ?? "MA";
  const fastGetterProperties: Record<string, unknown> = {
    blockKind: "getTechnicalIndicator",
    indicatorType: "movingAverage",
    movingAverageType,
    windowSize: properties.fastPeriod ?? 5,
    variableName: suggestStrategyIndicatorVariableName({
      blockKind: "getTechnicalIndicator",
      indicatorType: "movingAverage",
      movingAverageType,
      windowSize: properties.fastPeriod ?? 5,
    }),
  };
  const slowGetterProperties: Record<string, unknown> = {
    blockKind: "getTechnicalIndicator",
    indicatorType: "movingAverage",
    movingAverageType,
    windowSize: properties.slowPeriod ?? 20,
    variableName: suggestStrategyIndicatorVariableName({
      blockKind: "getTechnicalIndicator",
      indicatorType: "movingAverage",
      movingAverageType,
      windowSize: properties.slowPeriod ?? 20,
    }),
  };
  const conditionProperties = buildConditionProperties(properties, {
    fast: fastGetterId,
    slow: slowGetterId,
  });

  return {
    nodes: [
      {
        id: fastGetterId,
        type: "rect",
        x,
        y: y - 76,
        text: nextGetTechnicalIndicatorNodeText(fastGetterProperties),
        properties: fastGetterProperties,
      },
      {
        id: slowGetterId,
        type: "rect",
        x,
        y: y + 76,
        text: nextGetTechnicalIndicatorNodeText(slowGetterProperties),
        properties: slowGetterProperties,
      },
      {
        id: conditionId,
        type: "diamond",
        x: x + 340,
        y,
        text: nextTechnicalIndicatorConditionNodeText(conditionProperties),
        properties: conditionProperties,
      },
    ],
    edges: [
      {
        id: `${baseId}-control-fast-slow`,
        type: "polyline",
        sourceNodeId: fastGetterId,
        targetNodeId: slowGetterId,
        properties: buildStrategyVisualControlEdgeProperties(),
      },
      {
        id: `${baseId}-control-slow-condition`,
        type: "polyline",
        sourceNodeId: slowGetterId,
        targetNodeId: conditionId,
        properties: buildStrategyVisualControlEdgeProperties(),
      },
    ],
    focusNodeId: conditionId,
  };
}

function buildGetterProperties(
  properties: TechnicalIndicatorBlockProperties,
): Record<string, unknown> {
  switch (properties.indicatorType) {
    case "macd":
      return withVariableName({
        blockKind: "getTechnicalIndicator",
        indicatorType: "macd",
        ...(properties.fastPeriod === undefined ? {} : { fastPeriod: properties.fastPeriod }),
        ...(properties.slowPeriod === undefined ? {} : { slowPeriod: properties.slowPeriod }),
        ...(properties.signalPeriod === undefined ? {} : { signalPeriod: properties.signalPeriod }),
      });
    case "kdj":
      return withVariableName({
        blockKind: "getTechnicalIndicator",
        indicatorType: "kdj",
        ...(properties.period === undefined ? {} : { period: properties.period }),
        ...(properties.m1 === undefined ? {} : { m1: properties.m1 }),
        ...(properties.m2 === undefined ? {} : { m2: properties.m2 }),
      });
    case "bollinger":
      return withVariableName({
        blockKind: "getTechnicalIndicator",
        indicatorType: "bollinger",
        ...(properties.period === undefined ? {} : { period: properties.period }),
        ...(properties.multiplier === undefined ? {} : { multiplier: properties.multiplier }),
      });
    case "movingAverage":
      return withVariableName({
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        ...(properties.movingAverageType === undefined ? {} : { movingAverageType: properties.movingAverageType }),
        ...(properties.fastPeriod === undefined ? {} : { windowSize: properties.fastPeriod }),
      });
    default:
      return withVariableName({
        blockKind: "getTechnicalIndicator",
        indicatorType: properties.indicatorType,
        ...(properties.period === undefined ? {} : { period: properties.period }),
      });
  }
}

function buildConditionProperties(
  properties: TechnicalIndicatorBlockProperties,
  bindings: {
    primary?: string;
    fast?: string;
    slow?: string;
  },
): Record<string, unknown> {
  return {
    ...normalizeTechnicalIndicatorConditionProperties({
      blockKind: "technicalIndicatorCondition",
      indicatorType: properties.indicatorType,
      conditionMode: properties.conditionMode,
      operator: properties.operator,
      threshold: properties.threshold,
      patternType: properties.patternType,
      lookback: properties.lookback,
      ...(bindings.primary === undefined ? {} : { inputPrimaryNodeId: bindings.primary }),
      ...(bindings.fast === undefined ? {} : { inputFastNodeId: bindings.fast }),
      ...(bindings.slow === undefined ? {} : { inputSlowNodeId: bindings.slow }),
    }),
  };
}

function withVariableName(
  properties: Record<string, unknown>,
): Record<string, unknown> {
  return {
    ...properties,
    variableName: suggestStrategyIndicatorVariableName(properties),
  };
}

function readShortcutBaseId(nodeId: string | undefined): string {
  if (typeof nodeId === "string" && nodeId.trim() !== "") {
    return nodeId;
  }

  return `indicator-shortcut-${Math.random().toString(36).slice(2, 10)}`;
}