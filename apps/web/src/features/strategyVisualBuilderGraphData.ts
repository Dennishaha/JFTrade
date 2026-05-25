import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  getStrategyBlockDefinition,
  type StrategyBlockKind,
} from "./strategyVisualBuilderCatalog";
import {
  nextTechnicalIndicatorNodeText,
  normalizeTechnicalIndicatorProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import { createDefaultStrategyVisualModel } from "./strategyVisualBuilderModels";
import { cloneStrategyVisualModel } from "./strategyVisualBuilderShared";

export interface StrategyVisualGraphData {
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
    nodes: (graphData.nodes ?? []).map((node) => {
      const properties = normalizeNodeProperties(node.properties ?? {});
      return {
        id: node.id ?? buildBlockId("node"),
        type: node.type ?? "rect",
        x: typeof node.x === "number" ? node.x : 0,
        y: typeof node.y === "number" ? node.y : 0,
        text: normalizeNodeText(normalizeTextValue(node.text), properties),
        properties,
      };
    }),
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

function normalizeNodeProperties(
  rawProperties: Record<string, unknown>,
): Record<string, unknown> {
  const blockKind = readBlockKind(rawProperties.blockKind);
  if (blockKind === null) {
    return { ...rawProperties };
  }

  if (blockKind === "technicalIndicator") {
    return normalizeTechnicalIndicatorProperties(rawProperties) as unknown as Record<string, unknown>;
  }

  const definition = getStrategyBlockDefinition(blockKind);
  if (definition === null) {
    return { ...rawProperties };
  }

  return {
    ...definition.properties,
    ...rawProperties,
  };
}

function normalizeNodeText(
  text: string,
  properties: Record<string, unknown>,
): string {
  if (text.trim() !== "") {
    return text;
  }

  const blockKind = readBlockKind(properties.blockKind);
  if (blockKind === "technicalIndicator") {
    return nextTechnicalIndicatorNodeText(properties);
  }

  return getStrategyBlockDefinition(blockKind)?.text ?? "";
}

function readBlockKind(value: unknown): StrategyBlockKind | null {
  return typeof value === "string"
    ? (value as StrategyBlockKind)
    : null;
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