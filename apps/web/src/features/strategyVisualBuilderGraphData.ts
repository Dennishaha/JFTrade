import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";

import {
  getStrategyBlockDefinition,
  getStrategyBlockKind,
  type StrategyBlockKind,
} from "./strategyVisualBuilderCatalog";
import {
  buildStrategyVisualControlEdgeProperties,
  isStrategyVisualControlEdge,
  isStrategyVisualDataEdge,
  normalizeStrategyVisualEdgeProperties,
  readStrategyVisualEdgeBranch,
} from "./strategyVisualBuilderEdges";
import {
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  normalizeGetTechnicalIndicatorProperties,
  normalizeTechnicalIndicatorConditionProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import { createDefaultStrategyVisualModel } from "./strategyVisualBuilderModels";
import { reconcileStrategyVisualModelIndicatorBindings } from "./strategyVisualBuilderIndicatorReferences";
import { fromStrategyLogicFlowDisplayNodeType, toStrategyLogicFlowDisplayNodeType } from "./strategyVisualBuilderNodePresentation";
import { cloneStrategyVisualModel } from "./strategyVisualBuilderShared";

const STRATEGY_CANVAS_BRIDGE_EDGE_ID_PREFIX = "__strategy-canvas-bridge__::";

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
  const normalizedModel = reconcileStrategyVisualModelIndicatorBindings(
    cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel(),
  );

  return {
    nodes: normalizedModel.nodes.map((node) => ({
      id: node.id,
      type: toStrategyLogicFlowDisplayNodeType(node.type),
      x: node.x,
      y: node.y,
      text: node.text,
      properties: { ...node.properties },
    })),
    edges: normalizedModel.edges.filter((edge) => !isStrategyVisualDataEdge(edge)).map((edge) => ({
      id: edge.id,
      type: edge.type,
      sourceNodeId: edge.sourceNodeId,
      targetNodeId: edge.targetNodeId,
      text: edge.text,
      properties: edge.properties === undefined ? undefined : { ...edge.properties },
    })),
  };
}

export function toStrategyCanvasGraphData(
  model: StrategyVisualModelDocument | null | undefined,
): StrategyVisualGraphData {
  const normalizedModel = reconcileStrategyVisualModelIndicatorBindings(
    cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel(),
  );
  const hiddenNodeIds = new Set(
    normalizedModel.nodes
      .filter(isHiddenStrategyCanvasNode)
      .map((node) => node.id),
  );
  const visibleEdges = normalizedModel.edges
    .filter((edge) => !isStrategyVisualDataEdge(edge))
    .filter(
      (edge) =>
        !hiddenNodeIds.has(edge.sourceNodeId) &&
        !hiddenNodeIds.has(edge.targetNodeId),
    );
  const bridgedEdges = buildStrategyCanvasBridgeEdges(
    normalizedModel,
    hiddenNodeIds,
    visibleEdges,
  );

  return {
    nodes: normalizedModel.nodes
      .filter((node) => !hiddenNodeIds.has(node.id))
      .map((node) => ({
        id: node.id,
        type: toStrategyLogicFlowDisplayNodeType(node.type),
        x: node.x,
        y: node.y,
        text: node.text,
        properties: { ...node.properties },
      })),
    edges: [...visibleEdges, ...bridgedEdges]
      .map((edge) => ({
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
  return reconcileStrategyVisualModelIndicatorBindings({
    engine: "logic-flow",
    version: 1,
    nodes: (graphData.nodes ?? []).map((node) => {
      const properties = normalizeNodeProperties(node.properties ?? {});
      return {
        id: node.id ?? buildBlockId("node"),
        type: fromStrategyLogicFlowDisplayNodeType(node.type),
        x: typeof node.x === "number" ? node.x : 0,
        y: typeof node.y === "number" ? node.y : 0,
        text: normalizeNodeText(normalizeTextValue(node.text), properties),
        properties,
      };
    }),
    edges: (graphData.edges ?? [])
      .filter((edge) => {
        const sourceNodeId = typeof edge.sourceNodeId === "string" ? edge.sourceNodeId : "";
        const targetNodeId = typeof edge.targetNodeId === "string" ? edge.targetNodeId : "";
        return sourceNodeId !== "" && targetNodeId !== "";
      })
      .map((edge) => ({
        ...(edge.id === undefined ? {} : { id: edge.id }),
        type: edge.type ?? "polyline",
        sourceNodeId: edge.sourceNodeId ?? "",
        targetNodeId: edge.targetNodeId ?? "",
        text: normalizeTextValue(edge.text),
        properties: normalizeStrategyVisualEdgeProperties(edge.properties),
      })),
  });
}

export function fromStrategyCanvasGraphData(
  graphData: StrategyVisualGraphData,
  baseModel: StrategyVisualModelDocument | null | undefined,
): StrategyVisualModelDocument {
  const retainedBridgeKeys = collectStrategyCanvasBridgeKeys(graphData.edges ?? []);
  const visibleModel = fromLogicFlowGraphData({
    ...graphData,
    edges: (graphData.edges ?? []).filter((edge) => !isStrategyCanvasBridgeEdgeId(edge.id)),
  });
  const hiddenArtifacts = collectHiddenCanvasArtifacts(baseModel);
  const hiddenBridgeMembership = collectHiddenCanvasBridgeMembership(baseModel);
  const visibleNodeIds = new Set(visibleModel.nodes.map((node) => node.id));
  const hiddenNodeIds = new Set(hiddenArtifacts.nodes.map((node) => node.id));

  return reconcileStrategyVisualModelIndicatorBindings({
    engine: visibleModel.engine,
    version: visibleModel.version,
    nodes: [...visibleModel.nodes, ...hiddenArtifacts.nodes],
    edges: [
      ...visibleModel.edges,
      ...hiddenArtifacts.edges.filter((edge) => {
        const sourceKnown =
          visibleNodeIds.has(edge.sourceNodeId) || hiddenNodeIds.has(edge.sourceNodeId);
        const targetKnown =
          visibleNodeIds.has(edge.targetNodeId) || hiddenNodeIds.has(edge.targetNodeId);
        return sourceKnown
          && targetKnown
          && shouldRetainHiddenCanvasEdge(edge, hiddenBridgeMembership, retainedBridgeKeys);
      }),
    ],
  });
}

function normalizeNodeProperties(
  rawProperties: Record<string, unknown>,
): Record<string, unknown> {
  const blockKind = readBlockKind(rawProperties.blockKind);
  if (blockKind === null) {
    return { ...rawProperties };
  }

  if (blockKind === "getTechnicalIndicator") {
    return normalizeGetTechnicalIndicatorProperties(rawProperties) as unknown as Record<string, unknown>;
  }

  if (blockKind === "technicalIndicatorCondition") {
    return normalizeTechnicalIndicatorConditionProperties(rawProperties) as unknown as Record<string, unknown>;
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
  if (blockKind === "getTechnicalIndicator") {
    return nextGetTechnicalIndicatorNodeText(properties);
  }

  if (blockKind === "technicalIndicatorCondition") {
    return nextTechnicalIndicatorConditionNodeText(properties);
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

function isHiddenStrategyCanvasNode(
  node: StrategyVisualNodeDocument,
): boolean {
  return getStrategyBlockKind(node) === "getTechnicalIndicator";
}

function collectHiddenCanvasArtifacts(
  model: StrategyVisualModelDocument | null | undefined,
): {
  nodes: StrategyVisualNodeDocument[];
  edges: StrategyVisualEdgeDocument[];
} {
  const normalizedModel = reconcileStrategyVisualModelIndicatorBindings(
    cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel(),
  );
  const hiddenNodes = normalizedModel.nodes
    .filter(isHiddenStrategyCanvasNode)
    .map((node) => ({
      ...node,
      properties: { ...node.properties },
    }));
  const hiddenNodeIds = new Set(hiddenNodes.map((node) => node.id));

  return {
    nodes: hiddenNodes,
    edges: normalizedModel.edges
      .filter(
        (edge) =>
          isStrategyVisualDataEdge(edge)
          || hiddenNodeIds.has(edge.sourceNodeId)
          || hiddenNodeIds.has(edge.targetNodeId),
      )
      .map((edge) => ({
        ...(edge.id === undefined ? {} : { id: edge.id }),
        type: edge.type ?? "polyline",
        sourceNodeId: edge.sourceNodeId,
        targetNodeId: edge.targetNodeId,
        ...(edge.text === undefined ? {} : { text: edge.text }),
        ...(edge.properties === undefined
          ? {}
          : { properties: { ...edge.properties } }),
      })),
  };
}

function buildStrategyCanvasBridgeEdges(
  model: StrategyVisualModelDocument,
  hiddenNodeIds: Set<string>,
  visibleEdges: StrategyVisualEdgeDocument[],
): StrategyVisualEdgeDocument[] {
  const outgoingControlEdges = new Map<string, StrategyVisualEdgeDocument[]>();
  const visibleEdgeKeys = new Set(
    visibleEdges.map((edge) =>
      buildStrategyCanvasBridgeEdgeKey(
        edge.sourceNodeId,
        edge.targetNodeId,
        readStrategyVisualEdgeBranch(edge),
      ),
    ),
  );
  const bridgeEdges: StrategyVisualEdgeDocument[] = [];

  for (const edge of model.edges) {
    if (!isStrategyVisualControlEdge(edge)) {
      continue;
    }
    const bucket = outgoingControlEdges.get(edge.sourceNodeId) ?? [];
    bucket.push(edge);
    outgoingControlEdges.set(edge.sourceNodeId, bucket);
  }

  for (const edge of visibleEdges) {
    visibleEdgeKeys.add(
      buildStrategyCanvasBridgeEdgeKey(
        edge.sourceNodeId,
        edge.targetNodeId,
        readStrategyVisualEdgeBranch(edge),
      ),
    );
  }

  for (const node of model.nodes) {
    if (hiddenNodeIds.has(node.id)) {
      continue;
    }

    for (const edge of outgoingControlEdges.get(node.id) ?? []) {
      if (!hiddenNodeIds.has(edge.targetNodeId)) {
        continue;
      }

      const bridgedTargets = collectBridgedVisibleTargetPaths(
        edge.targetNodeId,
        hiddenNodeIds,
        outgoingControlEdges,
        new Set<string>(),
        readStrategyVisualEdgeBranch(edge),
        [buildStrategyVisualEdgeIdentity(edge)],
      );

      for (const target of bridgedTargets) {
        const bridgeKey = buildStrategyCanvasBridgeEdgeKey(
          node.id,
          target.targetNodeId,
          target.branch,
        );
        if (visibleEdgeKeys.has(bridgeKey)) {
          continue;
        }

        visibleEdgeKeys.add(bridgeKey);
        bridgeEdges.push({
          id: `${STRATEGY_CANVAS_BRIDGE_EDGE_ID_PREFIX}${bridgeKey}`,
          type: "polyline",
          sourceNodeId: node.id,
          targetNodeId: target.targetNodeId,
          properties: buildStrategyVisualControlEdgeProperties(target.branch ?? undefined),
        });
      }
    }
  }

  return bridgeEdges;
}

function collectBridgedVisibleTargets(
  nodeId: string,
  hiddenNodeIds: Set<string>,
  outgoingControlEdges: Map<string, StrategyVisualEdgeDocument[]>,
  visitedHiddenNodeIds: Set<string>,
  inheritedBranch: "true" | "false" | null,
): Array<{ targetNodeId: string; branch: "true" | "false" | null }> {
  return collectBridgedVisibleTargetPaths(
    nodeId,
    hiddenNodeIds,
    outgoingControlEdges,
    visitedHiddenNodeIds,
    inheritedBranch,
    [],
  ).map((target) => ({
    targetNodeId: target.targetNodeId,
    branch: target.branch,
  }));
}

function collectBridgedVisibleTargetPaths(
  nodeId: string,
  hiddenNodeIds: Set<string>,
  outgoingControlEdges: Map<string, StrategyVisualEdgeDocument[]>,
  visitedHiddenNodeIds: Set<string>,
  inheritedBranch: "true" | "false" | null,
  traversedEdgeKeys: string[],
): Array<{
  targetNodeId: string;
  branch: "true" | "false" | null;
  edgeKeys: string[];
}> {
  if (visitedHiddenNodeIds.has(nodeId)) {
    return [];
  }

  visitedHiddenNodeIds.add(nodeId);

  const targets: Array<{
    targetNodeId: string;
    branch: "true" | "false" | null;
    edgeKeys: string[];
  }> = [];

  for (const edge of outgoingControlEdges.get(nodeId) ?? []) {
    const nextBranch = inheritedBranch ?? readStrategyVisualEdgeBranch(edge);
    const nextEdgeKeys = [...traversedEdgeKeys, buildStrategyVisualEdgeIdentity(edge)];

    if (hiddenNodeIds.has(edge.targetNodeId)) {
      targets.push(
        ...collectBridgedVisibleTargetPaths(
          edge.targetNodeId,
          hiddenNodeIds,
          outgoingControlEdges,
          new Set(visitedHiddenNodeIds),
          nextBranch,
          nextEdgeKeys,
        ),
      );
      continue;
    }

    targets.push({
      targetNodeId: edge.targetNodeId,
      branch: nextBranch,
      edgeKeys: nextEdgeKeys,
    });
  }

  return targets;
}

function collectHiddenCanvasBridgeMembership(
  model: StrategyVisualModelDocument | null | undefined,
): Map<string, Set<string>> {
  const normalizedModel = reconcileStrategyVisualModelIndicatorBindings(
    cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel(),
  );
  const hiddenNodeIds = new Set(
    normalizedModel.nodes
      .filter(isHiddenStrategyCanvasNode)
      .map((node) => node.id),
  );
  const outgoingControlEdges = new Map<string, StrategyVisualEdgeDocument[]>();
  const membership = new Map<string, Set<string>>();

  for (const edge of normalizedModel.edges) {
    if (!isStrategyVisualControlEdge(edge)) {
      continue;
    }
    const bucket = outgoingControlEdges.get(edge.sourceNodeId) ?? [];
    bucket.push(edge);
    outgoingControlEdges.set(edge.sourceNodeId, bucket);
  }

  for (const node of normalizedModel.nodes) {
    if (hiddenNodeIds.has(node.id)) {
      continue;
    }

    for (const edge of outgoingControlEdges.get(node.id) ?? []) {
      if (!hiddenNodeIds.has(edge.targetNodeId)) {
        continue;
      }

      const bridgedTargets = collectBridgedVisibleTargetPaths(
        edge.targetNodeId,
        hiddenNodeIds,
        outgoingControlEdges,
        new Set<string>(),
        readStrategyVisualEdgeBranch(edge),
        [buildStrategyVisualEdgeIdentity(edge)],
      );

      for (const target of bridgedTargets) {
        const bridgeKey = buildStrategyCanvasBridgeEdgeKey(
          node.id,
          target.targetNodeId,
          target.branch,
        );

        for (const edgeKey of target.edgeKeys) {
          const bridgeKeys = membership.get(edgeKey) ?? new Set<string>();
          bridgeKeys.add(bridgeKey);
          membership.set(edgeKey, bridgeKeys);
        }
      }
    }
  }

  return membership;
}

function collectStrategyCanvasBridgeKeys(
  edges: NonNullable<StrategyVisualGraphData["edges"]>,
): Set<string> {
  const bridgeKeys = new Set<string>();

  for (const edge of edges) {
    const edgeId = typeof edge.id === "string" ? edge.id : null;
    if (edgeId === null || !isStrategyCanvasBridgeEdgeId(edgeId)) {
      continue;
    }
    bridgeKeys.add(edgeId.slice(STRATEGY_CANVAS_BRIDGE_EDGE_ID_PREFIX.length));
  }

  return bridgeKeys;
}

function shouldRetainHiddenCanvasEdge(
  edge: StrategyVisualEdgeDocument,
  hiddenBridgeMembership: Map<string, Set<string>>,
  retainedBridgeKeys: Set<string>,
): boolean {
  if (isStrategyVisualDataEdge(edge)) {
    return true;
  }

  const bridgeKeys = hiddenBridgeMembership.get(buildStrategyVisualEdgeIdentity(edge));
  if (bridgeKeys === undefined || bridgeKeys.size === 0) {
    return true;
  }

  for (const bridgeKey of bridgeKeys) {
    if (retainedBridgeKeys.has(bridgeKey)) {
      return true;
    }
  }

  return false;
}

function buildStrategyVisualEdgeIdentity(
  edge: Pick<StrategyVisualEdgeDocument, "id" | "sourceNodeId" | "targetNodeId" | "properties">,
): string {
  if (typeof edge.id === "string" && edge.id.trim() !== "") {
    return edge.id;
  }

  return buildStrategyCanvasBridgeEdgeKey(
    edge.sourceNodeId,
    edge.targetNodeId,
    readStrategyVisualEdgeBranch(edge as StrategyVisualEdgeDocument),
  );
}

function buildStrategyCanvasBridgeEdgeKey(
  sourceNodeId: string,
  targetNodeId: string,
  branch: "true" | "false" | null,
): string {
  return `${sourceNodeId}->${targetNodeId}:${branch ?? "main"}`;
}

function isStrategyCanvasBridgeEdgeId(value: unknown): boolean {
  return typeof value === "string" && value.startsWith(STRATEGY_CANVAS_BRIDGE_EDGE_ID_PREFIX);
}
