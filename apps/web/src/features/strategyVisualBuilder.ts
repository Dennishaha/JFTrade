import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  createDefaultStrategyVisualModel,
} from "./strategyVisualBuilderModels";
import { cloneStrategyVisualModel } from "./strategyVisualBuilderShared";

export {
  createStrategyPaletteItems,
  getStrategyBlockCatalog,
  getStrategyBlockDefinition,
  getStrategyBlockKind,
} from "./strategyVisualBuilderCatalog";
export type {
  StrategyBlockDefinition,
  StrategyBlockKind,
} from "./strategyVisualBuilderCatalog";
export {
  buildStrategyVisualModelFromScript,
} from "./strategyVisualBuilderParser";
export type {
  StrategyScriptParseFailure,
  StrategyScriptParseResult,
  StrategyScriptParseSuccess,
} from "./strategyVisualBuilderParser";
export { buildStrategyScriptFromVisualModel } from "./strategyVisualBuilderScript";
export type { StrategyScriptContext } from "./strategyVisualBuilderScript";
export { getStrategyAuthoringTemplates } from "./strategyVisualBuilderTemplates";
export type { StrategyAuthoringTemplate } from "./strategyVisualBuilderTemplates";
export { createBollingerReversionStrategyVisualModel, createBreakoutStrategyVisualModel, createDefaultStrategyVisualModel, createDoubleMovingAverageStrategyVisualModel, createMACDMomentumStrategyVisualModel, createMeanReversionStrategyVisualModel, createRSIReversionStrategyVisualModel, } from "./strategyVisualBuilderModels";
export { cloneStrategyVisualModel } from "./strategyVisualBuilderShared";

interface StrategyVisualGraphData {
  nodes?: Array<{
    id?: string | undefined;
    type?: string | undefined;
    x?: number | undefined;
    y?: number | undefined;
    text?: string | { value?: string } | undefined;
    properties?: Record<string, unknown> | undefined;
  }>;
  edges?: Array<{
    id?: string | undefined;
    type?: string | undefined;
    sourceNodeId?: string | undefined;
    targetNodeId?: string | undefined;
    text?: string | { value?: string } | undefined;
    properties?: Record<string, unknown> | undefined;
  }>;
}

export function toLogicFlowGraphData(
  model: StrategyVisualModelDocument | null | undefined,
): StrategyVisualGraphData {
  const normalizedModel =
    cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel();

  return {
    nodes: normalizedModel.nodes.map((node) => ({
      id: node.id,
      type: node.type,
      x: node.x,
      y: node.y,
      text: node.text,
      properties: { ...node.properties },
    })),
    edges: normalizedModel.edges.map((edge) => ({
      id: edge.id,
      type: edge.type,
      sourceNodeId: edge.sourceNodeId,
      targetNodeId: edge.targetNodeId,
      text: edge.text,
      properties: edge.properties === undefined ? undefined : { ...edge.properties },
    })),
  };
}

export function fromLogicFlowGraphData(
  graphData: StrategyVisualGraphData,
): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: (graphData.nodes ?? []).map((node) => ({
      id: node.id ?? buildBlockId("node"),
      type: node.type ?? "rect",
      x: typeof node.x === "number" ? node.x : 0,
      y: typeof node.y === "number" ? node.y : 0,
      text: normalizeTextValue(node.text),
      properties: { ...(node.properties ?? {}) },
    })),
    edges: (graphData.edges ?? []).map((edge) => ({
      id: edge.id,
      type: edge.type ?? "polyline",
      sourceNodeId: edge.sourceNodeId ?? "",
      targetNodeId: edge.targetNodeId ?? "",
      text: normalizeTextValue(edge.text),
      properties:
        edge.properties === undefined ? undefined : { ...edge.properties },
    })),
  };
}

function buildBlockId(prefix: string): string {
  return `${prefix}-${Math.random().toString(36).slice(2, 10)}`;
}

function normalizeTextValue(value: string | { value?: string } | undefined): string {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value?.value === "string") {
    return value.value;
  }
  return "";
}