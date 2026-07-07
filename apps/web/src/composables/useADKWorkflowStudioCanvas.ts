import { ref } from "vue";

import type {
  ADKWorkflowCanvasGraph,
  ADKWorkflowDefinition,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerType,
} from "@/contracts";
import {
  workflowToCanvasGraph,
} from "@/features/adkWorkflowCanvasGraph";
import {
  addDraftTriggerFlowNode,
  addAgentFlowNode,
  connectWorkflowFlowEdge,
  flowFromGraph,
  graphFromFlow as graphFromFlowSnapshots,
  refreshWorkflowFlowNodeData,
  removeWorkflowFlowNode,
  type FlowEdgeSnapshot,
  type FlowNodeSnapshot,
  type WorkflowFlowConnection,
  type WorkflowFlowNodeDataContext,
} from "@/features/adkWorkflowStudio";

export function useADKWorkflowStudioCanvas(options: {
  workflowTriggers: () => ADKWorkflowTrigger[];
  nodeDataContext: () => WorkflowFlowNodeDataContext;
}) {
  const flowNodes = ref<FlowNodeSnapshot[]>([]);
  const flowEdges = ref<FlowEdgeSnapshot[]>([]);

  function graphFromFlow(): ADKWorkflowCanvasGraph {
    return graphFromFlowSnapshots(flowNodes.value, flowEdges.value);
  }

  function setFlowGraph(graph: ADKWorkflowCanvasGraph): void {
    const flow = flowFromGraph(graph);
    flowNodes.value = flow.nodes;
    flowEdges.value = flow.edges;
  }

  function refreshNodeData(): void {
    flowNodes.value = refreshWorkflowFlowNodeData(flowNodes.value, options.nodeDataContext());
  }

  function loadWorkflowGraph(workflow: ADKWorkflowDefinition): void {
    const graph = workflowToCanvasGraph(workflow, options.workflowTriggers());
    setFlowGraph(graph);
    refreshNodeData();
  }

  function addTriggerNode(options: {
    id: string;
    type: ADKWorkflowTriggerType;
    title: string;
    status: string;
  }): void {
    const flow = addDraftTriggerFlowNode(flowNodes.value, flowEdges.value, options);
    flowNodes.value = flow.nodes;
    flowEdges.value = flow.edges;
  }

  function addAgentNode(options: {
    id: string;
    title: string;
    agentId: string;
  }): void {
    const flow = addAgentFlowNode(flowNodes.value, flowEdges.value, options);
    flowNodes.value = flow.nodes;
    flowEdges.value = flow.edges;
  }

  function removeNode(nodeId: string): void {
    const flow = removeWorkflowFlowNode(flowNodes.value, flowEdges.value, nodeId);
    flowNodes.value = flow.nodes;
    flowEdges.value = flow.edges;
  }

  function connect(connection: WorkflowFlowConnection): void {
    flowEdges.value = connectWorkflowFlowEdge(flowEdges.value, connection);
  }

  return {
    flowNodes,
    flowEdges,
    graphFromFlow,
    setFlowGraph,
    loadWorkflowGraph,
    refreshNodeData,
    addTriggerNode,
    addAgentNode,
    removeNode,
    connect,
  };
}
