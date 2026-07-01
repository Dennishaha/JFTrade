import type { ADKWorkflowCanvasGraph } from "@/contracts";

export type FlowNodeData = Record<string, unknown>;
export type FlowNodeSnapshot = {
  id: string;
  type?: string;
  position: { x: number; y: number };
  data?: FlowNodeData;
};
export type FlowEdgeSnapshot = {
  id: string;
  source: string;
  target: string;
  sourceHandle?: string | null;
  targetHandle?: string | null;
  type?: string;
  data?: FlowNodeData;
  animated?: boolean;
};
export type WorkflowStudioPanePair = [number, number];
export type WorkflowStudioPaneSizes = {
  outer: WorkflowStudioPanePair;
  workbench: WorkflowStudioPanePair;
};

export const defaultWorkflowStudioPaneSizes: WorkflowStudioPaneSizes = {
  outer: [19, 81],
  workbench: [69.14, 30.86],
};
export const workflowStudioOuterPaneMinSizes: WorkflowStudioPanePair = [18, 64];
export const workflowStudioWorkbenchPaneMinSizes: WorkflowStudioPanePair = [48, 22];

export function cloneWorkflowStudioPaneSizes(
  sizes = defaultWorkflowStudioPaneSizes,
): WorkflowStudioPaneSizes {
  return {
    outer: [...sizes.outer],
    workbench: [...sizes.workbench],
  };
}

export function normalizeWorkflowStudioPanePair(
  input: unknown,
  minSizes: WorkflowStudioPanePair,
): WorkflowStudioPanePair | null {
  if (!Array.isArray(input) || input.length !== 2) return null;
  const values = input.map((value) => Number(value));
  if (!values.every((value) => Number.isFinite(value) && value > 0)) return null;
  const total = values.reduce((sum, value) => sum + value, 0);
  if (!Number.isFinite(total) || total <= 0) return null;
  const normalized = values.map((value) => (value / total) * 100);
  if (normalized.some((value, index) => value < minSizes[index]!)) {
    return null;
  }
  return [
    Number(normalized[0]!.toFixed(2)),
    Number(normalized[1]!.toFixed(2)),
  ];
}

export function graphFromFlow(
  nodes: FlowNodeSnapshot[],
  edges: FlowEdgeSnapshot[],
): ADKWorkflowCanvasGraph {
  return {
    version: "adk-workflow-canvas/v1",
    nodes: nodes.map((node) => ({
      id: node.id,
      type: String(node.type ?? "default"),
      position: {
        x: node.position.x,
        y: node.position.y,
      },
      data: { ...(node.data ?? {}) },
    })),
    edges: edges.map((edge) => {
      const next: {
        id: string;
        source: string;
        target: string;
        sourceHandle?: string;
        targetHandle?: string;
        type?: string;
        data?: FlowNodeData;
      } = {
        id: edge.id,
        source: edge.source,
        target: edge.target,
        type: String(edge.type ?? "smoothstep"),
      };
      if (edge.sourceHandle) next.sourceHandle = edge.sourceHandle;
      if (edge.targetHandle) next.targetHandle = edge.targetHandle;
      if (edge.data) next.data = { ...edge.data };
      return next;
    }),
    viewport: { x: 0, y: 0, zoom: 1 },
  };
}

export function flowFromGraph(graph: ADKWorkflowCanvasGraph): {
  nodes: FlowNodeSnapshot[];
  edges: FlowEdgeSnapshot[];
} {
  return {
    nodes: (graph.nodes ?? []).map((node) => ({
      id: node.id,
      type: node.type,
      position: node.position,
      data: {
        ...(node.data ?? {}),
        label: node.data?.title ?? node.id,
      },
    })),
    edges: (graph.edges ?? []).map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      sourceHandle: edge.sourceHandle ?? null,
      targetHandle: edge.targetHandle ?? null,
      type: edge.type || "smoothstep",
      animated: edge.source.startsWith("trigger:"),
      data: edge.data ?? {},
    })),
  };
}
