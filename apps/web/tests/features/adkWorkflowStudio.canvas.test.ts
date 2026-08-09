import { reactive, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import type { ADKWorkflowCanvasGraph, ADKWorkflowTriggerLog } from "@/types";
import {
  addDraftTriggerFlowNode,
  addAgentFlowNode,
  cloneInputRows,
  cloneWorkflowStudioPaneSizes,
  connectWorkflowFlowEdge,
  createWorkflowTemplateForm,
  createWorkflowTemplateTrigger,
  defaultWorkflowStudioPaneSizes,
  filterWorkflowLogs,
  flowFromGraph,
  formatDurationMs,
  formatJson,
  graphFromFlow,
  inputTypeOptions,
  inspectorTitle,
  inputRowsToInputs,
  logTone,
  logStatusOptions,
  marketEdgeOptions,
  marketOperatorOptions,
  nodeRunClass,
  normalizeWorkflowStudioPanePair,
  nodeTypeLabel,
  nodeRunDetails,
  parseDateFilter,
  permissionOptions,
  previewScheduleRuns,
  projectedNodeRuns,
  refreshWorkflowFlowNodeData,
  removeWorkflowFlowNode,
  runDurationLabel,
  runDurationMs,
  statusLabel,
  templateDescription,
  templateName,
  triggerStatusOptions,
  triggerTypeOptions,
  triggerTypeLabel,
  weekdayOptions,
  workflowEditStatusOptions,
  workflowFormToDefinition,
  workflowInvocationMessage,
  workflowNodeRunFor,
  workflowRunStats,
  workflowStatusOptions,
  workflowTemplates,
  workflowTone,
  workModeOptions,
  workModeLabel,
} from "../../src/features/adkWorkflowStudio";
import { useADKWorkflowStudioCanvas } from "@/composables/adk/useADKWorkflowStudioCanvas";
import { useADKWorkflowStudioResources } from "@/composables/adk/useADKWorkflowStudioResources";
import { useADKWorkflowStudioViewModel } from "@/composables/adk/useADKWorkflowStudioViewModel";
import {
  createTriggerForm,
  createWorkflowForm,
  createWorkflowInputRow,
} from "../../src/features/adkWorkflowForms";

describe("adkWorkflowStudio helpers", () => {
  it("normalizes pane sizes only when both panes can keep their minimum widths", () => {
    expect(normalizeWorkflowStudioPanePair([1, 3], [20, 20])).toEqual([25, 75]);
    expect(normalizeWorkflowStudioPanePair([1, 99], [20, 20])).toBeNull();
    expect(normalizeWorkflowStudioPanePair(null, [20, 20])).toBeNull();
    expect(normalizeWorkflowStudioPanePair([1], [20, 20])).toBeNull();
    expect(normalizeWorkflowStudioPanePair(["bad", 2], [20, 20])).toBeNull();
    expect(normalizeWorkflowStudioPanePair([0, 2], [20, 20])).toBeNull();
    expect(
      normalizeWorkflowStudioPanePair([Number.MAX_VALUE, Number.MAX_VALUE], [20, 20]),
    ).toBeNull();

    const cloned = cloneWorkflowStudioPaneSizes();
    cloned.outer[0] = 1;
    expect(defaultWorkflowStudioPaneSizes.outer[0]).toBe(19);
  });

  it("round trips Vue Flow snapshots without losing handles, edge data, or trigger animation", () => {
    const graph = graphFromFlow(
      [
        { id: "trigger:open", type: "trigger", position: { x: 1, y: 2 }, data: { title: "开盘" } },
        { id: "start", type: "start", position: { x: 3, y: 4 }, data: { inputCount: 1 } },
      ],
      [
        {
          id: "trigger:open->start",
          source: "trigger:open",
          target: "start",
          sourceHandle: "out",
          targetHandle: "in",
          type: "smoothstep",
          data: { guarded: true },
        },
      ],
    );

    expect(graph.nodes?.[0]).toMatchObject({
      id: "trigger:open",
      type: "trigger",
      data: { title: "开盘" },
    });
    expect(graph.edges?.[0]).toMatchObject({
      sourceHandle: "out",
      targetHandle: "in",
      data: { guarded: true },
    });

    const flow = flowFromGraph(graph);
    expect(flow.nodes[0]?.data?.label).toBe("开盘");
    expect(flow.edges[0]?.animated).toBe(true);
  });

  it("applies stable defaults to sparse Vue Flow snapshots and legacy graphs", () => {
    const graph = graphFromFlow(
      [{ id: "start", position: { x: 1, y: 2 } }],
      [{ id: "start->agent", source: "start", target: "agent" }],
    );

    expect(graph.nodes[0]).toEqual({
      id: "start",
      type: "default",
      position: { x: 1, y: 2 },
      data: {},
    });
    expect(graph.edges[0]).toEqual({
      id: "start->agent",
      source: "start",
      target: "agent",
      type: "smoothstep",
    });

    const flow = flowFromGraph({
      version: "adk-workflow-canvas/v1",
      nodes: [{ id: "start", type: "start", position: { x: 1, y: 2 } }],
      edges: [{ id: "start->agent", source: "start", target: "agent", type: "" }],
    });
    expect(flow.nodes[0]?.data).toEqual({ label: "start" });
    expect(flow.edges[0]).toMatchObject({
      sourceHandle: null,
      targetHandle: null,
      type: "smoothstep",
      animated: false,
      data: {},
    });
    expect(flowFromGraph({ version: "legacy" } as never)).toEqual({ nodes: [], edges: [] });
  });

  it("orchestrates Studio canvas graph state through the canvas composable", () => {
    const trigger = {
      id: "trigger-1",
      workflowId: "workflow-1",
      type: "schedule" as const,
      title: "开盘复盘",
      status: "ENABLED",
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    };
    const canvas = useADKWorkflowStudioCanvas({
      workflowTriggers: () => [trigger],
      nodeDataContext: () => ({
        workflowName: "每日复盘",
        workflowStatus: "ENABLED",
        workflowWorkMode: "loop",
        workflowInputCount: 1,
        workflowAgentId: "agent-1",
        agentName: "投研智能体",
        agentNameForId: () => "投研智能体",
        logsCount: 0,
        logStatusFilter: "",
        selectedLog: null,
        triggers: [trigger],
        draftTriggerNodeId: "",
        draftTriggerTitle: "",
        draftTriggerType: "schedule",
        draftTriggerStatus: "DISABLED",
      }),
    });

    canvas.loadWorkflowGraph({
      id: "workflow-1",
      name: "每日复盘",
      status: "ENABLED",
      agentId: "agent-1",
      workMode: "loop",
      promptTemplate: "run",
      canvasGraph: {
        version: "adk-workflow-canvas/v1",
        nodes: [
          { id: "start", type: "start", position: { x: 0, y: 0 } },
          { id: "agent:primary", type: "agent", position: { x: 1, y: 0 }, data: { title: "每日复盘", agentId: "agent-1" } },
          { id: "monitor", type: "monitor", position: { x: 2, y: 0 } },
        ],
        edges: [
          { id: "start->agent:primary", source: "start", target: "agent:primary" },
          { id: "agent:primary->monitor", source: "agent:primary", target: "monitor" },
        ],
      },
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    });
    canvas.addTriggerNode({
      id: "trigger:draft-1",
      type: "webhook",
      title: "外部事件",
      status: "DISABLED",
    });
    canvas.connect({ source: "start", target: "agent:primary" });
    canvas.connect({ source: "start", target: "agent:primary" });
    canvas.removeNode("trigger:draft-1");

    expect(canvas.flowNodes.value.some((node) => node.id === "trigger:trigger-1")).toBe(true);
    expect(canvas.flowNodes.value.find((node) => node.id === "agent:primary")?.data).toMatchObject({
      title: "每日复盘",
      subtitle: "投研智能体",
    });
    expect(canvas.flowNodes.value.some((node) => node.id === "trigger:draft-1")).toBe(false);
    expect(canvas.flowEdges.value.filter((edge) => edge.id === "start->agent:primary")).toHaveLength(1);
    expect(canvas.graphFromFlow().nodes.map((node) => node.id)).toContain("agent:primary");
  });

  it("adds canvas agent nodes with node-scoped defaults", () => {
    const flow = addAgentFlowNode(
      [{ id: "agent", type: "agent", position: { x: 1, y: 2 }, data: { title: "默认" } }],
      [],
      { id: "agent:child", title: "研究", agentId: "research-agent" },
    );

    expect(flow.nodes).toHaveLength(2);
    expect(flow.nodes[1]).toMatchObject({
      id: "agent:child",
      type: "agent",
      data: {
        title: "研究",
        agentId: "research-agent",
        promptTemplate: "",
        objectiveTemplate: "",
      },
    });
    expect(flow.edges).toEqual([]);
  });

  it("derives Studio list, inspector, log, variable, and webhook view state", () => {
    const workflowForm = reactive(createWorkflowForm("agent-disabled", "Run {{ .symbol }}"));
    workflowForm.name = "每日复盘";
    workflowForm.inputRows = [createWorkflowInputRow("symbol", "US.AAPL")];
    workflowForm.preservedDefaultInputs = { complex: { nested: true } };
    const triggerForm = reactive(createTriggerForm("webhook"));
    triggerForm.id = "trigger-1";
    triggerForm.preservedConfig = { advanced: true };

    const selectedLogId = ref("");
    const vm = useADKWorkflowStudioViewModel({
      agents: () => [
        { id: "agent-disabled", name: "停用", status: "DISABLED", workMode: "chat" },
        { id: "agent-enabled", name: "启用", status: "ENABLED", workMode: "loop" },
      ],
      providers: () => [
        {
          id: "provider-1",
          displayName: "OpenAI",
          provider: "openai",
          model: "gpt-test",
          baseUrl: "",
          apiKey: "",
          enabled: false,
          timeoutSec: 30,
          contextWindow: 128000,
          default: false,
          createdAt: "",
          updatedAt: "",
        },
        {
          id: "provider-enabled",
          displayName: "Anthropic",
          provider: "anthropic",
          model: "claude-test",
          baseUrl: "",
          apiKey: "",
          enabled: true,
          timeoutSec: 30,
          contextWindow: 200000,
          default: false,
          createdAt: "",
          updatedAt: "",
        },
      ],
      workflows: ref([
        {
          id: "workflow-1",
          name: "每日复盘",
          description: "关注持仓",
          status: "ENABLED",
          agentId: "agent-enabled",
          workMode: "loop",
          promptTemplate: "run",
          tags: ["复盘"],
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
        {
          id: "workflow-2",
          name: "其他",
          status: "DISABLED",
          agentId: "agent-enabled",
          workMode: "chat",
          promptTemplate: "run",
          tags: [],
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
        {
          id: "workflow-3",
          name: "无元数据工作流",
          status: "DISABLED",
          agentId: "agent-enabled",
          workMode: "loop",
          promptTemplate: "run",
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
      ]),
      triggers: ref([
        {
          id: "trigger-1",
          workflowId: "workflow-1",
          type: "webhook",
          title: "外部事件",
          status: "ENABLED",
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
        {
          id: "trigger-untitled",
          workflowId: "workflow-1",
          type: "event",
          title: "",
          status: "DISABLED",
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
      ]),
      logs: ref([
        buildLog({
          id: "log-1",
          triggerId: "trigger-1",
          triggerType: "market_threshold",
          matchedEvent: { instrumentId: "US.AAPL", price: 123.45 },
          result: { markdown: "风险复盘完成" },
        }),
      ]),
      selectedWorkflowId: ref("workflow-1"),
      selectedNodeId: ref("trigger:trigger-1"),
      selectedLogId,
      workflowSearch: ref("持仓"),
      logStatusFilter: ref(""),
      logTriggerFilter: ref(""),
      logKeywordFilter: ref("风险"),
      logFromFilter: ref("2026-07-01"),
      logToFilter: ref("2026-07-01"),
      workflowForm,
      triggerForm,
      formatDateTime: (value) => value,
      origin: () => "http://localhost:5174",
    });

    expect(vm.defaultAgentId.value).toBe("agent-enabled");
    expect(vm.selectedWorkflow.value?.id).toBe("workflow-1");
    expect(vm.visibleWorkflows.value.map((workflow) => workflow.id)).toEqual(["workflow-1"]);
    expect(vm.inspectorKind.value).toBe("trigger");
    expect(vm.agentOptions.value).toContainEqual({
      title: "启用 (agent-enabled)",
      value: "agent-enabled",
    });
    expect(vm.selectedTrigger.value?.title).toBe("外部事件");
    expect(vm.logTriggerOptions.value).toContainEqual({ title: "外部事件", value: "trigger-1" });
    expect(vm.inputVariableOptions.value).toContainEqual({ title: "symbol", value: "{{ .symbol }}" });
    expect(vm.providerOptions.value).toContainEqual({
      title: "Anthropic · claude-test",
      value: "provider-enabled",
    });
    expect(vm.preservedInputCount.value).toBe(1);
    expect(vm.preservedConfigCount.value).toBe(1);
    expect(vm.visibleLogs.value.map((log) => log.id)).toEqual(["log-1"]);
    expect(vm.selectedLog.value?.id).toBe("log-1");
    selectedLogId.value = "log-1";
    expect(vm.selectedLog.value?.id).toBe("log-1");
    expect(vm.workflowStats.value.total).toBe(1);
    expect(vm.selectedNodeRun.value?.nodeType).toBe("trigger");
    expect(vm.triggerRunSummary.value).toMatchObject({ total: 1, failures: 0 });
    expect(vm.webhookEndpoint.value).toBe(
      "http://localhost:5174/api/v1/adk/workflow-webhooks/trigger-1",
    );
    expect(vm.webhookCurlSample.value).toContain("X-JFTrade-Workflow-Secret");
    expect(vm.latestMarketEvent.value).toEqual({ instrumentId: "US.AAPL", price: 123.45 });
    expect(vm.logTriggerOptions.value).toContainEqual({ title: "事件", value: "trigger-untitled" });
  });

  it("derives safe empty-state view values for drafts and unsupported selections", () => {
    const workflowForm = reactive(createWorkflowForm("", "Run"));
    const triggerForm = reactive(createTriggerForm("schedule"));
    const selectedNodeId = ref("unknown");
    const vm = useADKWorkflowStudioViewModel({
      agents: () => [],
      providers: () => [],
      workflows: ref([]),
      triggers: ref([]),
      logs: ref([]),
      selectedWorkflowId: ref("missing"),
      selectedNodeId,
      selectedLogId: ref("missing"),
      workflowSearch: ref(""),
      logStatusFilter: ref(""),
      logTriggerFilter: ref(""),
      logKeywordFilter: ref(""),
      logFromFilter: ref(""),
      logToFilter: ref(""),
      workflowForm,
      triggerForm,
      formatDateTime: (value) => value,
      origin: () => "",
    });

    expect(vm.defaultAgentId.value).toBe("");
    expect(vm.selectedWorkflow.value).toBeNull();
    expect(vm.visibleWorkflows.value).toEqual([]);
    expect(vm.selectedTrigger.value).toBeNull();
    expect(vm.inspectorKind.value).toBe("workflow");
    selectedNodeId.value = "start";
    expect(vm.inspectorKind.value).toBe("start");
    selectedNodeId.value = "agent:primary";
    expect(vm.inspectorKind.value).toBe("agent");
    selectedNodeId.value = "monitor";
    expect(vm.inspectorKind.value).toBe("monitor");
    expect(vm.agentOptions.value).toEqual([]);
    expect(vm.providerOptions.value).toEqual([{ title: "沿用智能体默认模型", value: "" }]);
    workflowForm.inputRows = [createWorkflowInputRow("  ", "ignored")];
    expect(vm.inputVariableOptions.value.map((item) => item.title)).toEqual([
      "当前时间",
      "工作流名称",
      "触发器标题",
    ]);
    expect(vm.selectedLog.value).toBeNull();
    expect(vm.selectedNodeRun.value).toBeNull();
    expect(vm.triggerRunSummary.value).toBeNull();
    expect(vm.schedulePreviewRuns.value.length).toBeGreaterThan(0);
    expect(vm.webhookEndpoint.value).toBe("保存触发器后生成网络回调地址");
    expect(vm.webhookCurlSample.value).toBe("");
    expect(vm.latestMarketEvent.value).toBeNull();

    triggerForm.id = "external/hook";
    triggerForm.type = "webhook";
    expect(vm.webhookEndpoint.value).toBe("/api/v1/adk/workflow-webhooks/external%2Fhook");
    expect(vm.webhookCurlSample.value).toContain("curl -X POST");
    expect(vm.schedulePreviewRuns.value).toEqual([]);
    expect(vm.triggerRunSummary.value).toEqual({ total: 0, latest: null, failures: 0 });
  });
});

function buildWorkflow(id: string) {
  return {
    id,
    name: "每日复盘",
    status: "ENABLED",
    agentId: "agent-1",
    workMode: "loop",
    promptTemplate: "run",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  };
}

function buildLog(overrides: Partial<ADKWorkflowTriggerLog> = {}): ADKWorkflowTriggerLog {
  return {
    id: "log-1",
    workflowId: "workflow-1",
    triggerId: "trigger-1",
    triggerType: "schedule",
    status: "SUCCEEDED",
    inputs: { symbol: "US.AAPL" },
    matchedEvent: { source: "schedule" },
    startedAt: "2026-07-01T00:00:00Z",
    finishedAt: "2026-07-01T00:00:05Z",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:05Z",
    ...overrides,
  };
}

export function buildGraph(): ADKWorkflowCanvasGraph {
  return {
    version: "adk-workflow-canvas/v1",
    nodes: [
      { id: "start", type: "start", position: { x: 0, y: 0 }, data: { title: "开始" } },
      { id: "agent", type: "agent", position: { x: 200, y: 0 }, data: { title: "智能体" } },
    ],
    edges: [{ id: "start->agent", source: "start", target: "agent", type: "smoothstep" }],
  };
}
