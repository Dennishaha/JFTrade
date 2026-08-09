import type { StrategyVisualNodeDocument } from "@/types";

import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
  type StrategyVisualEdgeBranch,
} from "./strategyVisualBuilderEdges";
import type { ParseState, ParsedPineEntry } from "./strategyVisualBuilderPineParserTypes";

export function failUnsupportedPineStatement(
  state: ParseState,
  entry: ParsedPineEntry,
): null {
  state.error = `第 ${entry.lineNumber} 行无法同步为流程图：${entry.trimmed}`;
  return null;
}

export function hasLegacyFlowBlockAnnotation(script: string): boolean {
  return /@jftradeFlowBlockKind\s+(?:codeBlock|technicalIndicator)(?:\s|$)/.test(script);
}

export function createNodeFromParts(options: {
  state: ParseState;
  entry: ParsedPineEntry;
  kind: StrategyBlockKind;
  defaultText: string;
  defaultType: StrategyVisualNodeDocument["type"];
  properties: Record<string, unknown>;
  sourceStart?: number;
}): StrategyVisualNodeDocument {
  const { state, entry, kind, defaultText, defaultType, properties } = options;
  const id = ensureUniqueNodeId(
    state,
    entry.annotation?.nodeId ?? `${kind}-${entry.lineNumber}`,
  );
  const existing = state.existingNodeById.get(id);
  const layout = existing ?? nextLayoutNode(state, kind);
  return {
    id,
    type: existing?.type ?? defaultType,
    x: layout.x,
    y: layout.y,
    text: entry.annotation?.nodeText ?? existing?.text ?? defaultText,
    properties: {
      ...properties,
      sourceRange: {
        start: options.sourceStart ?? entry.annotationStart ?? entry.start,
        end: entry.end,
      },
    },
  };
}

export function nextLayoutNode(
  state: ParseState,
  _kind: StrategyBlockKind,
): Pick<StrategyVisualNodeDocument, "x" | "y"> {
  const index = state.sequence++;
  return {
    x: 440 + (index % 4) * 240,
    y: 120 + Math.floor(index / 4) * 120,
  };
}

export function addNode(state: ParseState, node: StrategyVisualNodeDocument): void {
  if (state.nodeIds.has(node.id)) {
    return;
  }
  state.nodes.push(node);
  state.nodeIds.add(node.id);
}

export function addControlEdge(
  state: ParseState,
  sourceNodeId: string,
  targetNodeId: string,
  branch?: StrategyVisualEdgeBranch,
): void {
  if (sourceNodeId === targetNodeId || hasEdge(state, sourceNodeId, targetNodeId, branch, "control")) {
    return;
  }
  state.edges.push({
    id: buildEdgeId(sourceNodeId, targetNodeId, branch ?? "control"),
    type: "polyline",
    sourceNodeId,
    targetNodeId,
    properties: buildStrategyVisualControlEdgeProperties(branch),
  });
}

export function addDataEdge(
  state: ParseState,
  sourceNodeId: string,
  targetNodeId: string,
  slot: "primary" | "fast" | "slow",
): void {
  if (sourceNodeId === targetNodeId || hasEdge(state, sourceNodeId, targetNodeId, slot, "data")) {
    return;
  }
  state.edges.push({
    id: buildEdgeId(sourceNodeId, targetNodeId, `data-${slot}`),
    type: "polyline",
    sourceNodeId,
    targetNodeId,
    properties: buildStrategyVisualDataEdgeProperties(slot),
  });
}

export function hasEdge(
  state: ParseState,
  sourceNodeId: string,
  targetNodeId: string,
  discriminator: string | undefined,
  role: "control" | "data",
): boolean {
  return state.edges.some((edge) => {
    if (edge.sourceNodeId !== sourceNodeId || edge.targetNodeId !== targetNodeId) {
      return false;
    }
    if (role === "data") {
      return edge.properties?.role === "data" && edge.properties.slot === discriminator;
    }
    return (edge.properties?.role ?? "control") !== "data" && (edge.properties?.branch ?? undefined) === discriminator;
  });
}

export function buildEdgeId(sourceNodeId: string, targetNodeId: string, suffix: string): string {
  return `edge-${sourceNodeId}-${targetNodeId}-${suffix}`.replace(/[^A-Za-z0-9_-]+/g, "-");
}

export function ensureUniqueNodeId(state: ParseState, preferredId: string): string {
  const base = preferredId.trim() === "" ? "pine-node" : preferredId.trim();
  if (!state.nodeIds.has(base)) {
    return base;
  }
  for (let index = 2; ; index += 1) {
    const candidate = `${base}-${index}`;
    if (!state.nodeIds.has(candidate)) {
      return candidate;
    }
  }
}


