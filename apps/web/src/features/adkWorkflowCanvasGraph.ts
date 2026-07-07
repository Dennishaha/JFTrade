import type {
  ADKWorkflowCanvasEdge,
  ADKWorkflowCanvasGraph,
  ADKWorkflowCanvasNode,
  ADKWorkflowDefinition,
  ADKWorkflowDefinitionWriteRequest,
  ADKWorkflowTrigger,
} from "@/contracts";

import {
  workflowFormToPayload,
  type WorkflowFormModel,
} from "./adkWorkflowForms";

export const ADK_WORKFLOW_CANVAS_VERSION = "adk-workflow-canvas/v1";

export type ADKWorkflowCanvasNodeKind = "start" | "agent" | "trigger" | "monitor";

export function workflowToCanvasGraph(
  workflow: ADKWorkflowDefinition,
  triggers: ADKWorkflowTrigger[] = [],
): ADKWorkflowCanvasGraph {
  if (validateWorkflowCanvasGraph(workflow.canvasGraph)) {
    return mergeTriggerNodes(workflow.canvasGraph, triggers);
  }
  return emptyWorkflowCanvasGraph();
}

export function defaultWorkflowCanvasGraph(
  workflow: ADKWorkflowDefinition,
  triggers: ADKWorkflowTrigger[] = [],
): ADKWorkflowCanvasGraph {
  const triggerNodes = triggers.map((trigger, index) =>
    createCanvasNode(
      triggerNodeId(trigger.id),
      "trigger",
      { x: 80, y: 42 + index * 104 },
      {
        title: trigger.title || triggerTypeLabel(trigger.type),
        triggerId: trigger.id,
        triggerType: trigger.type,
        status: trigger.status,
      },
    ),
  );

  const nodes: ADKWorkflowCanvasNode[] = [
    createCanvasNode("start", "start", { x: 80, y: 250 }, {
      title: "开始",
      inputCount: Object.keys(workflow.defaultInputs ?? {}).length,
    }),
    ...triggerNodes,
    createCanvasNode("agent:primary", "agent", { x: 385, y: 250 }, {
      title: workflow.name || "智能体",
      agentId: workflow.agentId,
      providerId: workflow.providerId,
      model: workflow.model,
      permissionMode: workflow.permissionMode,
      promptTemplate: workflow.promptTemplate,
      objectiveTemplate: workflow.objectiveTemplate,
    }),
    createCanvasNode("monitor", "monitor", { x: 690, y: 250 }, {
      title: "监控",
    }),
  ];

  const triggerEdges = triggerNodes.map((node) =>
    createCanvasEdge(`${node.id}->start`, node.id, "start"),
  );

  return {
    version: ADK_WORKFLOW_CANVAS_VERSION,
    nodes,
    edges: [
      ...triggerEdges,
      createCanvasEdge("start->agent:primary", "start", "agent:primary"),
      createCanvasEdge("agent:primary->monitor", "agent:primary", "monitor"),
    ],
    viewport: { x: 0, y: 0, zoom: 1 },
  };
}

function emptyWorkflowCanvasGraph(): ADKWorkflowCanvasGraph {
  return {
    version: ADK_WORKFLOW_CANVAS_VERSION,
    nodes: [],
    edges: [],
    viewport: { x: 0, y: 0, zoom: 1 },
  };
}

export function canvasGraphToWorkflowPayload(
  form: WorkflowFormModel,
  graph: ADKWorkflowCanvasGraph,
): ADKWorkflowDefinitionWriteRequest {
  return {
    ...workflowFormToPayload(form),
    canvasGraph: sanitizeWorkflowCanvasGraph(graph),
  };
}

export function validateWorkflowCanvasGraph(
  graph: unknown,
): graph is ADKWorkflowCanvasGraph {
  if (!isRecord(graph)) return false;
  if (!Array.isArray(graph.nodes) || !Array.isArray(graph.edges)) return false;
  return graph.nodes.every(isCanvasNode) && graph.edges.every(isCanvasEdge);
}

export function sanitizeWorkflowCanvasGraph(
  graph: ADKWorkflowCanvasGraph,
): ADKWorkflowCanvasGraph & {
  nodes: ADKWorkflowCanvasNode[];
  edges: ADKWorkflowCanvasEdge[];
} {
  const nodes = (graph.nodes ?? [])
    .filter(isCanvasNode)
    .map((node) => {
      const next: ADKWorkflowCanvasNode = {
        id: node.id.trim(),
        type: node.type.trim(),
        position: {
          x: Number.isFinite(node.position.x) ? node.position.x : 0,
          y: Number.isFinite(node.position.y) ? node.position.y : 0,
        },
      };
      if (isRecord(node.data)) {
        next.data = { ...node.data };
      }
      return next;
    });

  const nodeIds = new Set(nodes.map((node) => node.id));
  const edges = (graph.edges ?? [])
    .filter(isCanvasEdge)
    .filter((edge) => nodeIds.has(edge.source.trim()) && nodeIds.has(edge.target.trim()))
    .map((edge) => {
      const next: ADKWorkflowCanvasEdge = {
        id: edge.id.trim(),
        source: edge.source.trim(),
        target: edge.target.trim(),
      };
      if (edge.sourceHandle?.trim()) next.sourceHandle = edge.sourceHandle.trim();
      if (edge.targetHandle?.trim()) next.targetHandle = edge.targetHandle.trim();
      if (edge.type?.trim()) next.type = edge.type.trim();
      if (isRecord(edge.data)) next.data = { ...edge.data };
      return next;
    });

  return {
    version: graph.version?.trim() || ADK_WORKFLOW_CANVAS_VERSION,
    nodes,
    edges,
    viewport: isRecord(graph.viewport) ? { ...graph.viewport } : { x: 0, y: 0, zoom: 1 },
  };
}

export function triggerNodeId(triggerId: string): string {
  return `trigger:${triggerId.trim() || "draft"}`;
}

function mergeTriggerNodes(
  graph: ADKWorkflowCanvasGraph,
  triggers: ADKWorkflowTrigger[],
): ADKWorkflowCanvasGraph {
  const sanitized = sanitizeWorkflowCanvasGraph(graph);
  const nodes = [...sanitized.nodes];
  const edges = [...sanitized.edges];
  const nodeIds = new Set(nodes.map((node) => node.id));
  const edgeIds = new Set(edges.map((edge) => edge.id));

  triggers.forEach((trigger, index) => {
    const id = triggerNodeId(trigger.id);
    if (!nodeIds.has(id)) {
      nodes.push(
        createCanvasNode(id, "trigger", { x: 80, y: 42 + index * 104 }, {
          title: trigger.title || triggerTypeLabel(trigger.type),
          triggerId: trigger.id,
          triggerType: trigger.type,
          status: trigger.status,
        }),
      );
      nodeIds.add(id);
    }
    const edgeId = `${id}->start`;
    if (nodeIds.has("start") && !edgeIds.has(edgeId)) {
      edges.push(createCanvasEdge(edgeId, id, "start"));
      edgeIds.add(edgeId);
    }
  });

  return { ...sanitized, nodes, edges };
}

function createCanvasNode(
  id: string,
  type: ADKWorkflowCanvasNodeKind,
  position: { x: number; y: number },
  data: Record<string, unknown>,
): ADKWorkflowCanvasNode {
  return { id, type, position, data };
}

function createCanvasEdge(
  id: string,
  source: string,
  target: string,
): ADKWorkflowCanvasEdge {
  return { id, source, target, type: "smoothstep" };
}

function triggerTypeLabel(type: string): string {
  switch (type) {
    case "schedule":
      return "定时";
    case "webhook":
      return "网络回调";
    case "event":
      return "事件";
    case "market_threshold":
      return "行情阈值";
    default:
      return "手动";
  }
}

function isCanvasNode(value: unknown): value is ADKWorkflowCanvasNode {
  if (!isRecord(value) || typeof value.id !== "string" || typeof value.type !== "string") {
    return false;
  }
  const position = value.position;
  return (
    isRecord(position) &&
    typeof position.x === "number" &&
    typeof position.y === "number"
  );
}

function isCanvasEdge(value: unknown): value is ADKWorkflowCanvasEdge {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.source === "string" &&
    typeof value.target === "string"
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
