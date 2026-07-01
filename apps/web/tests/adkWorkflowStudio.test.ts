import { reactive, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import type { ADKWorkflowCanvasGraph, ADKWorkflowTriggerLog } from "@/contracts";
import {
  addDraftTriggerFlowNode,
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
} from "../src/features/adkWorkflowStudio";
import { useADKWorkflowStudioCanvas } from "../src/composables/useADKWorkflowStudioCanvas";
import { useADKWorkflowStudioResources } from "../src/composables/useADKWorkflowStudioResources";
import { useADKWorkflowStudioViewModel } from "../src/composables/useADKWorkflowStudioViewModel";
import {
  createTriggerForm,
  createWorkflowForm,
  createWorkflowInputRow,
} from "../src/features/adkWorkflowForms";

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
        workflowWorkMode: "task",
        workflowInputCount: 1,
        agentName: "投研智能体",
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
      workMode: "task",
      promptTemplate: "run",
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    });
    canvas.addTriggerNode({
      id: "trigger:draft-1",
      type: "webhook",
      title: "外部事件",
      status: "DISABLED",
    });
    canvas.connect({ source: "start", target: "agent" });
    canvas.connect({ source: "start", target: "agent" });
    canvas.removeNode("trigger:draft-1");

    expect(canvas.flowNodes.value.some((node) => node.id === "trigger:trigger-1")).toBe(true);
    expect(canvas.flowNodes.value.find((node) => node.id === "agent")?.data).toMatchObject({
      title: "每日复盘",
      subtitle: "投研智能体",
    });
    expect(canvas.flowNodes.value.some((node) => node.id === "trigger:draft-1")).toBe(false);
    expect(canvas.flowEdges.value.filter((edge) => edge.id === "start->agent")).toHaveLength(1);
    expect(canvas.graphFromFlow().nodes.map((node) => node.id)).toContain("agent");
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
        { id: "agent-enabled", name: "启用", status: "ENABLED", workMode: "task" },
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
          workMode: "task",
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
          workMode: "task",
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
    selectedNodeId.value = "agent";
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

  it("loads workflow resources, preserves selection, and keeps resource mutations local", async () => {
    const workflow = {
      id: "workflow-1",
      name: "每日复盘",
      status: "ENABLED",
      agentId: "agent-1",
      workMode: "task",
      promptTemplate: "run",
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    };
    const trigger = {
      id: "trigger-1",
      workflowId: "workflow-1",
      type: "schedule" as const,
      title: "开盘复盘",
      status: "ENABLED",
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    };
    const selectedTriggers: string[] = [];
    let emptyDrafts = 0;
    let refreshCount = 0;
    const api = {
      fetchWorkflows: vi.fn(async () => ({
        workflows: [workflow],
        page: { limit: 20, offset: 0, total: 1, returned: 1, hasMore: false },
      })),
      fetchTriggers: vi.fn(async () => [trigger]),
      fetchLogs: vi.fn(async (_page, filters) => ({
        logs: [buildLog({ id: "log-resource", triggerId: filters.triggerId ?? "trigger-1" })],
        page: { limit: 20, offset: 0, total: 1, returned: 1, hasMore: false },
      })),
    };
    const resources = useADKWorkflowStudioResources({
      api,
      getSelectedNodeId: () => "trigger:trigger-1",
      onSelectedTrigger: (item) => selectedTriggers.push(item.id),
      onRefreshNodeData: () => {
        refreshCount += 1;
      },
      onEmptyWorkflows: () => {
        emptyDrafts += 1;
      },
    });

    await resources.refreshWorkflows();
    resources.triggers.value = [
      {
        id: "other-trigger",
        workflowId: "workflow-other",
        type: "event",
        title: "其他工作流事件",
        status: "ENABLED",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
    ];
    await resources.refreshTriggers();
    resources.logTriggerFilter.value = "trigger-1";
    resources.logStatusFilter.value = "FAILED";
    await resources.refreshLogs();

    expect(resources.selectedWorkflowId.value).toBe("workflow-1");
    expect(resources.workflows.value).toEqual([workflow]);
    expect(resources.triggers.value).toEqual([
      {
        id: "other-trigger",
        workflowId: "workflow-other",
        type: "event",
        title: "其他工作流事件",
        status: "ENABLED",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
      trigger,
    ]);
    expect(selectedTriggers).toEqual(["trigger-1"]);
    expect(api.fetchLogs.mock.calls[0]?.[1]).toEqual({
      workflowId: "workflow-1",
      triggerId: "trigger-1",
      status: "FAILED",
    });
    expect(resources.selectedLogId.value).toBe("log-resource");
    expect(refreshCount).toBe(2);

    resources.upsertWorkflow({ ...workflow, name: "更新后的复盘" });
    expect(resources.workflows.value[0]?.name).toBe("更新后的复盘");
    resources.removeTrigger("trigger-1");
    expect(resources.triggers.value.map((item) => item.id)).toEqual(["other-trigger"]);
    resources.upsertWorkflow({ ...workflow, id: "workflow-new", name: "新增复盘" });
    expect(resources.workflows.value[0]?.id).toBe("workflow-new");
    resources.removeWorkflow("workflow-1");
    expect(resources.selectedWorkflowId.value).toBe("workflow-new");
    resources.removeWorkflow("workflow-new");
    expect(resources.selectedWorkflowId.value).toBe("");
    expect(emptyDrafts).toBe(1);
  });

  it("surfaces resource failures and applies pagination boundaries", async () => {
    const errors: string[] = [];
    const api = {
      fetchWorkflows: vi
        .fn()
        .mockRejectedValueOnce("network")
        .mockResolvedValueOnce({
          workflows: [],
          page: { limit: 20, offset: 0, total: 0, returned: 0, hasMore: false },
        })
        .mockImplementation(async (page) => ({
          workflows: [buildWorkflow(`workflow-${page.offset}`)],
          page: { ...page, total: page.offset + 1, returned: 1, hasMore: false },
        })),
      fetchTriggers: vi.fn().mockRejectedValue(new Error("trigger failed")),
      fetchLogs: vi
        .fn()
        .mockRejectedValueOnce("log failed")
        .mockImplementation(async (page) => ({
          logs: [],
          page: { ...page, total: page.offset + 1, returned: 0, hasMore: false },
        })),
    };
    const resources = useADKWorkflowStudioResources({
      api,
      onError: (message) => errors.push(message),
    });

    await resources.refreshWorkflows();
    expect(errors).toContain("加载工作流失败");
    expect(resources.loading.value).toBe(false);

    await resources.refreshWorkflows();
    expect(errors.at(-1)).toBe("");
    expect(resources.selectedWorkflowId.value).toBe("");

    resources.triggers.value = [
      {
        id: "trigger-1",
        workflowId: "workflow-1",
        type: "schedule",
        title: "旧触发器",
        status: "ENABLED",
        createdAt: "",
        updatedAt: "",
      },
    ];
    await resources.refreshTriggers("");
    expect(resources.triggers.value).toEqual([]);

    resources.selectedWorkflowId.value = "workflow-1";
    await resources.refreshTriggers();
    expect(errors).toContain("trigger failed");
    expect(resources.triggerLoading.value).toBe(false);

    await resources.refreshLogs();
    expect(errors).toContain("加载触发日志失败");
    expect(resources.logLoading.value).toBe(false);

    resources.workflowPage.value = { limit: 20, offset: 20, total: 40, returned: 20, hasMore: false };
    resources.previousWorkflowPage();
    await Promise.resolve();
    expect(resources.workflowPage.value.offset).toBe(0);
    resources.nextWorkflowPage();
    expect(resources.workflowPage.value.offset).toBe(0);

    resources.workflowPage.value = { limit: 20, offset: 0, total: 21, returned: 20, hasMore: true };
    resources.nextWorkflowPage();
    await Promise.resolve();
    expect(resources.workflowPage.value.offset).toBe(20);

    resources.logPage.value = { limit: 20, offset: 20, total: 40, returned: 20, hasMore: false };
    resources.previousLogPage();
    await Promise.resolve();
    expect(resources.logPage.value.offset).toBe(0);
    resources.nextLogPage();
    expect(resources.logPage.value.offset).toBe(0);

    resources.logPage.value = { limit: 20, offset: 0, total: 21, returned: 20, hasMore: true };
    resources.nextLogPage();
    await Promise.resolve();
    expect(resources.logPage.value.offset).toBe(20);
  });

  it("recovers from partial resource envelopes and preserves callback safety", async () => {
    expect(useADKWorkflowStudioResources().workflows.value).toEqual([]);

    const errors: string[] = [];
    let emptyDrafts = 0;
    const api = {
      fetchWorkflows: vi
        .fn()
        .mockResolvedValueOnce({})
        .mockRejectedValueOnce(new Error("workflow unavailable")),
      fetchTriggers: vi.fn(async () => []),
      fetchLogs: vi
        .fn()
        .mockResolvedValueOnce({})
        .mockRejectedValueOnce(new Error("logs unavailable")),
    };
    const resources = useADKWorkflowStudioResources({
      api: api as never,
      onError: (message) => errors.push(message),
      onEmptyWorkflows: () => {
        emptyDrafts += 1;
      },
    });

    await resources.refreshWorkflows();
    expect(resources.workflows.value).toEqual([]);
    expect(resources.workflowPage.value).toMatchObject({ total: 0, returned: 0, hasMore: false });
    expect(emptyDrafts).toBe(1);
    await resources.refreshTriggers("workflow-1");
    expect(resources.triggers.value).toEqual([]);
    await resources.refreshLogs();
    expect(resources.logs.value).toEqual([]);
    expect(resources.logPage.value).toMatchObject({ total: 0, returned: 0, hasMore: false });

    await resources.refreshWorkflows();
    await resources.refreshLogs();
    expect(errors).toContain("workflow unavailable");
    expect(errors).toContain("logs unavailable");

    const missingSelection = useADKWorkflowStudioResources({
      api: {
        fetchWorkflows: vi.fn(),
        fetchTriggers: vi.fn(async () => [{
          id: "trigger-1",
          workflowId: "workflow-1",
          type: "schedule",
          title: "开盘复盘",
          status: "ENABLED",
          createdAt: "",
          updatedAt: "",
        }]),
        fetchLogs: vi.fn(),
      } as never,
      getSelectedNodeId: () => "trigger:missing",
      onSelectedTrigger: vi.fn(),
    });
    await missingSelection.refreshTriggers("workflow-1");
    expect(missingSelection.triggers.value).toHaveLength(1);

    const nonErrorTriggerFailure = useADKWorkflowStudioResources({
      api: {
        fetchWorkflows: vi.fn(),
        fetchTriggers: vi.fn().mockRejectedValue("offline"),
        fetchLogs: vi.fn(),
      } as never,
      onError: (message) => errors.push(message),
    });
    await nonErrorTriggerFailure.refreshTriggers("workflow-1");
    expect(errors).toContain("加载触发器失败");
  });

  it("converts input rows for invocation and clones rows before debug editing", () => {
    const rows = [
      { key: "symbol", type: "string" as const, value: "US.AAPL", booleanValue: false },
      { key: "threshold", type: "number" as const, value: "189.5", booleanValue: false },
      { key: "fallback", type: "number" as const, value: "not-a-number", booleanValue: false },
      { key: "dryRun", type: "boolean" as const, value: "", booleanValue: true },
      { key: "   ", type: "string" as const, value: "ignored", booleanValue: false },
    ];

    const cloned = cloneInputRows(rows);
    cloned[0]!.value = "US.MSFT";

    expect(rows[0]?.value).toBe("US.AAPL");
    expect(inputRowsToInputs(cloned)).toEqual({
      symbol: "US.MSFT",
      threshold: 189.5,
      fallback: "not-a-number",
      dryRun: true,
    });
  });

  it("projects legacy run logs into trigger, start, agent, and monitor node runs", () => {
    const log = buildLog({
      status: "FAILED",
      error: "planner failed",
      result: { markdown: "复盘结果" },
    });

    const runs = projectedNodeRuns(log, "每日复盘");

    expect(runs.map((run) => run.nodeId)).toEqual([
      "trigger:trigger-1",
      "start",
      "agent",
      "monitor",
    ]);
    expect(runs[2]).toMatchObject({
      nodeId: "agent",
      title: "每日复盘",
      status: "FAILED",
      error: "planner failed",
      outputs: { reply: "复盘结果" },
    });
    expect(workflowNodeRunFor(log, "trigger:unknown", "每日复盘")?.nodeType).toBe("trigger");
    expect(nodeRunDetails(runs[2]!)).toContain("planner failed");
  });

  it("handles manual and pre-run failures in projected node traces", () => {
    const log = buildLog({
      triggerId: "",
      triggerType: "webhook",
      status: "FAILED",
      runId: "",
      inputs: { source: "external" },
      matchedEvent: { body: true },
      error: "缺少输入",
      result: undefined,
    });

    const runs = projectedNodeRuns(log);

    expect(runs[0]).toMatchObject({
      nodeId: "trigger:manual",
      title: "网络回调",
      outputs: { body: true },
      error: "缺少输入",
    });
    expect(runs[1]).toMatchObject({
      nodeId: "start",
      status: "FAILED",
      error: "缺少输入",
    });
    expect(workflowNodeRunFor(log, "unknown")).toBeNull();
  });

  it("keeps backend supplied node runs as the authoritative monitor trace", () => {
    const log = buildLog({
      nodeRuns: [
        {
          nodeId: "agent",
          nodeType: "agent",
          title: "后端智能体",
          status: "SUCCEEDED",
          startedAt: "2026-07-01T00:00:01Z",
          finishedAt: "2026-07-01T00:00:04Z",
        },
      ],
    });

    expect(projectedNodeRuns(log)).toEqual(log.nodeRuns);
    expect(workflowNodeRunFor(log, "agent")).toBe(log.nodeRuns?.[0]);
  });

  it("refreshes canvas node metadata from workflow, trigger, draft, and monitor state", () => {
    const nodes = refreshWorkflowFlowNodeData(
      [
        { id: "start", type: "start", position: { x: 0, y: 0 }, data: { stale: true } },
        { id: "agent", type: "agent", position: { x: 1, y: 0 }, data: {} },
        { id: "monitor", type: "monitor", position: { x: 2, y: 0 }, data: {} },
        { id: "trigger:trigger-1", type: "trigger", position: { x: 3, y: 0 }, data: {} },
        { id: "trigger:draft", type: "trigger", position: { x: 4, y: 0 }, data: {} },
        { id: "custom", type: "custom", position: { x: 5, y: 0 }, data: { keep: true } },
      ],
      {
        workflowName: "每日复盘",
        workflowStatus: "ENABLED",
        workflowWorkMode: "task",
        workflowInputCount: 2,
        agentName: "投研智能体",
        logsCount: 7,
        logStatusFilter: "FAILED",
        selectedLog: null,
        triggers: [
          {
            id: "trigger-1",
            workflowId: "workflow-1",
            type: "schedule",
            title: "开盘前",
            status: "ENABLED",
            createdAt: "2026-07-01T00:00:00Z",
            updatedAt: "2026-07-01T00:00:00Z",
          },
        ],
        draftTriggerNodeId: "trigger:draft",
        draftTriggerTitle: "外部事件",
        draftTriggerType: "webhook",
        draftTriggerStatus: "DISABLED",
      },
    );

    expect(nodes[0]?.data).toMatchObject({
      stale: true,
      title: "开始",
      subtitle: "2 个输入项",
      status: "ENABLED",
    });
    expect(nodes[1]?.data).toMatchObject({
      title: "每日复盘",
      subtitle: "投研智能体",
      status: "task",
    });
    expect(nodes[2]?.data).toMatchObject({
      title: "监控",
      subtitle: "7 条日志",
      status: "FAILED",
    });
    expect(nodes[3]?.data).toMatchObject({
      title: "开盘前",
      subtitle: "定时",
      status: "ENABLED",
    });
    expect(nodes[4]?.data).toMatchObject({
      title: "外部事件",
      subtitle: "网络回调",
      status: "DISABLED",
    });
    expect(nodes[5]?.data).toEqual({ keep: true });
  });

  it("refreshes sparse canvas nodes from a selected legacy run", () => {
    const selectedLog = buildLog({
      startedAt: "",
      finishedAt: "",
      createdAt: "2026-07-01T00:00:01Z",
      updatedAt: "2026-07-01T00:00:03Z",
    });
    const nodes = refreshWorkflowFlowNodeData(
      [
        { id: "start", type: "start", position: { x: 0, y: 0 } },
        { id: "agent", type: "agent", position: { x: 1, y: 0 } },
        { id: "monitor", type: "monitor", position: { x: 2, y: 0 } },
        { id: "trigger:missing", type: "trigger", position: { x: 3, y: 0 } },
      ],
      {
        workflowName: "",
        workflowStatus: "DISABLED",
        workflowWorkMode: "task",
        workflowInputCount: 0,
        agentName: "默认智能体",
        logsCount: 1,
        logStatusFilter: "",
        selectedLog,
        triggers: [],
        draftTriggerNodeId: "trigger:draft",
        draftTriggerTitle: "",
        draftTriggerType: "manual",
        draftTriggerStatus: "DISABLED",
      },
    );

    expect(nodes.map((node) => node.data?.runStatus)).toEqual([
      "SUCCEEDED",
      "SUCCEEDED",
      "SUCCEEDED",
      "SUCCEEDED",
    ]);
    expect(nodes[1]?.data?.title).toBe("智能体");
    expect(nodes[3]?.data?.title).toBe("触发器");
    expect(projectedNodeRuns(selectedLog)[0]).toMatchObject({
      startedAt: "2026-07-01T00:00:01Z",
      finishedAt: "2026-07-01T00:00:03Z",
    });
    expect(projectedNodeRuns(buildLog({ matchedEvent: undefined }))[0]?.outputs).toBeUndefined();

    const backendOnlyAgent = buildLog({
      nodeRuns: [{
        nodeId: "agent",
        nodeType: "agent",
        title: "后端智能体",
        status: "SUCCEEDED",
      }],
    });
    expect(workflowNodeRunFor(backendOnlyAgent, "trigger:missing")).toBeNull();
  });

  it("adds, connects, de-duplicates, and removes workflow flow nodes", () => {
    const added = addDraftTriggerFlowNode(
      [{ id: "start", type: "start", position: { x: 0, y: 0 }, data: {} }],
      [],
      {
        id: "trigger:draft-1",
        type: "event",
        title: "事件",
        status: "DISABLED",
      },
    );

    expect(added.nodes[1]).toMatchObject({
      id: "trigger:draft-1",
      type: "trigger",
      position: { x: 80, y: 116 },
      data: { title: "事件", subtitle: "事件", status: "DISABLED" },
    });
    expect(added.edges).toEqual([
      {
        id: "trigger:draft-1->start",
        source: "trigger:draft-1",
        target: "start",
        type: "smoothstep",
        animated: true,
      },
    ]);

    const connected = connectWorkflowFlowEdge(added.edges, {
      source: "start",
      target: "agent",
      sourceHandle: "out",
    });
    const deDuplicated = connectWorkflowFlowEdge(connected, {
      source: "start",
      target: "agent",
    });
    const missingSource = connectWorkflowFlowEdge(deDuplicated, {
      source: "",
      target: "monitor",
    });
    const removed = removeWorkflowFlowNode(added.nodes, deDuplicated, "trigger:draft-1");

    expect(connected).toHaveLength(2);
    expect(deDuplicated).toHaveLength(2);
    expect(missingSource).toBe(deDuplicated);
    expect(connected[1]).toMatchObject({
      id: "start->agent",
      source: "start",
      target: "agent",
      sourceHandle: "out",
      targetHandle: null,
    });
    expect(removed.nodes.map((node) => node.id)).toEqual(["start"]);
    expect(removed.edges.map((edge) => edge.id)).toEqual(["start->agent"]);
    expect(connectWorkflowFlowEdge([], { source: "start", target: "agent" })[0]).toMatchObject({
      sourceHandle: null,
      targetHandle: null,
    });
  });

  it("summarizes monitor statistics from workflow trigger logs", () => {
    vi.setSystemTime(new Date("2026-07-01T00:00:00Z"));
    const stats = workflowRunStats([
      buildLog({
        id: "success",
        status: "SUCCEEDED",
        startedAt: "2026-06-30T23:59:00Z",
        finishedAt: "2026-07-01T00:00:00Z",
      }),
      buildLog({
        id: "failed",
        status: "FAILED",
        startedAt: "2026-06-28T00:00:00Z",
        finishedAt: "2026-06-28T00:00:02Z",
      }),
    ]);
    vi.useRealTimers();

    expect(stats).toEqual({
      total: 2,
      succeeded: 1,
      failed: 1,
      recent: 1,
      successRate: 50,
      avgMs: 31_000,
    });
    expect(formatDurationMs(stats.avgMs)).toBe("31.0 秒");
    expect(workflowRunStats([])).toEqual({
      total: 0,
      succeeded: 0,
      failed: 0,
      recent: 0,
      successRate: 0,
      avgMs: 0,
    });
    expect(workflowRunStats([buildLog({ startedAt: "", createdAt: "" })]).recent).toBe(0);
  });

  it("formats run duration, node class, json, and invalid dates for monitor details", () => {
    const valid = buildLog({
      startedAt: "2026-07-01T00:00:00Z",
      finishedAt: "2026-07-01T00:00:00.500Z",
    });
    const invalid = buildLog({
      startedAt: "bad",
      finishedAt: "2026-07-01T00:00:00Z",
    });
    const circular: Record<string, unknown> = {};
    circular.self = circular;

    expect(runDurationMs(valid)).toBe(500);
    expect(runDurationMs({
      ...valid,
      startedAt: "",
      finishedAt: "",
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:01Z",
    })).toBe(1000);
    expect(runDurationMs({
      ...valid,
      startedAt: "",
      createdAt: "",
      finishedAt: "",
      updatedAt: "",
    })).toBeNull();
    expect(runDurationMs({ ...valid, finishedAt: "2026-06-30T23:59:00Z" })).toBeNull();
    expect(runDurationLabel(valid)).toBe("500 毫秒");
    expect(runDurationLabel(invalid)).toBe("-");
    expect(runDurationLabel(null)).toBe("-");
    expect(runDurationLabel({ nodeId: "agent", nodeType: "agent", title: "智能体", status: "RUNNING" })).toBe("-");
    expect(formatDurationMs(0)).toBe("-");
    expect(nodeRunClass("PENDING_APPROVAL")).toBe("is-run-pending-approval");
    expect(nodeRunClass(" ")).toBe("");
    expect(nodeRunClass(null)).toBe("");
    expect(formatJson(null)).toBe("-");
    expect(formatJson(circular)).toBe("[object Object]");
    expect(parseDateFilter("")).toBeNull();
    expect(parseDateFilter("not-a-date")).toBeNull();
    expect(parseDateFilter("2026-07-01")).toBe(Date.parse("2026-07-01"));
  });

  it("labels statuses and custom cron previews in Chinese", () => {
    vi.setSystemTime(new Date("2026-07-01T00:00:00Z"));
    expect(statusLabel("ENABLED")).toBe("已启用");
    expect(statusLabel("DISABLED")).toBe("已停用");
    expect(statusLabel("QUEUED")).toBe("排队中");
    expect(statusLabel("RUNNING")).toBe("运行中");
    expect(statusLabel("PENDING_APPROVAL")).toBe("等待审批");
    expect(statusLabel("SUCCEEDED")).toBe("成功");
    expect(statusLabel("FAILED")).toBe("失败");
    expect(statusLabel("SKIPPED")).toBe("已跳过");
    expect(statusLabel("CANCELLED")).toBe("已取消");
    expect(statusLabel("ALL")).toBe("全部");
    expect(statusLabel("chat")).toBe("对话");
    expect(statusLabel("task")).toBe("任务");
    expect(statusLabel("loop")).toBe("目标");
    expect(statusLabel("unknown_status")).toBe("unknown_status");
    expect(statusLabel("")).toBe("未知");
    expect(workflowTone("ENABLED")).toBe("is-success");
    expect(workflowTone("DISABLED")).toBe("is-muted");
    expect(logTone("SUCCEEDED")).toBe("is-success");
    expect(logTone("FAILED")).toBe("is-error");
    expect(logTone("RUNNING")).toBe("is-info");
    expect(logTone("QUEUED")).toBe("is-info");
    expect(logTone("PENDING_APPROVAL")).toBe("is-warning");
    expect(logTone("CANCELLED")).toBe("is-muted");
    expect(workModeLabel("loop")).toBe("目标");
    expect(workModeLabel("unknown")).toBe("unknown");
    expect(nodeTypeLabel("agent")).toBe("智能体");
    expect(nodeTypeLabel("start")).toBe("开始");
    expect(nodeTypeLabel("trigger")).toBe("触发器");
    expect(nodeTypeLabel("workflow")).toBe("工作流");
    expect(nodeTypeLabel("unknown")).toBe("unknown");
    expect(inspectorTitle("monitor")).toBe("监控");
    expect(triggerTypeLabel("market_threshold")).toBe("行情阈值");
    expect(triggerTypeLabel("manual")).toBe("手动");
    expect(templateName("webhook")).toBe("网络回调工作流");
    expect(templateName("blank")).toBe("新的工作流");
    expect(templateName("schedule")).toBe("定时市场复盘");
    expect(templateName("risk")).toBe("持仓风险扫描");
    expect(templateName("market")).toBe("行情阈值提醒");
    expect(templateDescription("risk")).toContain("持仓风险");
    expect(templateDescription("blank")).toContain("开始节点");
    expect(templateDescription("schedule")).toContain("指定时间");
    expect(templateDescription("webhook")).toContain("外部系统");
    expect(templateDescription("market")).toContain("行情达到阈值");
    expect(workflowTemplates.map((template) => template.value)).toEqual([
      "blank",
      "schedule",
      "risk",
      "webhook",
      "market",
    ]);
    expect(workflowStatusOptions.map((item) => item.title)).toEqual([
      "全部工作流",
      "已启用",
      "已停用",
    ]);
    expect(workflowEditStatusOptions.map((item) => item.value)).toEqual(["ENABLED", "DISABLED"]);
    expect(triggerStatusOptions.map((item) => item.title)).toEqual(["启用", "停用"]);
    expect(workModeOptions.map((item) => item.value)).toEqual(["chat", "task", "loop"]);
    expect(permissionOptions.map((item) => item.value)).toEqual([
      "approval",
      "less_approval",
      "all",
    ]);
    expect(triggerTypeOptions.map((item) => item.value)).toEqual([
      "schedule",
      "webhook",
      "event",
      "market_threshold",
    ]);
    expect(inputTypeOptions.map((item) => item.value)).toEqual(["string", "number", "boolean"]);
    expect(weekdayOptions[1]).toEqual({ title: "周一", value: "1" });
    expect(marketEdgeOptions).toEqual([
      { title: "向上穿越", value: "cross_up" },
      { title: "向下穿越", value: "cross_down" },
      { title: "持续高于", value: "above" },
      { title: "持续低于", value: "below" },
    ]);
    expect(marketOperatorOptions).toEqual([">", ">=", "<", "<="]);
    expect(logStatusOptions.map((item) => item.value)).toContain("PENDING_APPROVAL");
    expect(previewScheduleRuns({
      frequency: "custom",
      time: "08:00",
      weekdays: ["1"],
      timezone: "Asia/Shanghai",
      customCron: "0 8 * * 1-5",
    }, 3, (value) => value)).toEqual(["自定义定时表达式：0 8 * * 1-5"]);
    expect(previewScheduleRuns({
      frequency: "custom",
      time: "08:00",
      weekdays: [],
      timezone: "Asia/Shanghai",
      customCron: "",
    }, 3, (value) => value)).toEqual(["自定义定时表达式：-"]);
    expect(previewScheduleRuns({
      frequency: "daily",
      time: "bad",
      weekdays: [],
      timezone: "Asia/Shanghai",
      customCron: "",
    }, 3, (value) => value)).toEqual([]);
    expect(previewScheduleRuns({
      frequency: "daily",
      time: "08:00",
      weekdays: [],
      timezone: "Asia/Shanghai",
      customCron: "",
    }, 1, (value) => value)).toHaveLength(1);
    expect(previewScheduleRuns({
      frequency: "weekly",
      time: "08:00",
      weekdays: ["3"],
      timezone: "Asia/Shanghai",
      customCron: "",
    }, 1, (value) => value)).toHaveLength(1);
    vi.useRealTimers();
  });

  it("builds template draft forms and trigger defaults for Studio creation", () => {
    const schedule = createWorkflowTemplateForm("schedule", "agent-1");
    const blank = createWorkflowTemplateForm("blank", "agent-1");
    const risk = createWorkflowTemplateForm("risk", "agent-1");
    const market = createWorkflowTemplateForm("market", "agent-1");
    const webhook = createWorkflowTemplateForm("webhook", "agent-1");
    const webhookTrigger = createWorkflowTemplateTrigger("webhook");
    const marketTrigger = createWorkflowTemplateTrigger("market");
    const riskTrigger = createWorkflowTemplateTrigger("risk");
    const scheduleTrigger = createWorkflowTemplateTrigger("schedule");
    const blankTrigger = createWorkflowTemplateTrigger("blank");
    const draft = workflowFormToDefinition(schedule, schedule.inputRows);

    expect(schedule).toMatchObject({
      agentId: "agent-1",
      name: "定时市场复盘",
      tagsText: "工作流, ADK",
    });
    expect(schedule.inputRows.map((row) => row.key)).toEqual(["symbol"]);
    expect(blank.tagsText).toBe("");
    expect(risk.inputRows.map((row) => row.key)).toEqual(["portfolio", "market"]);
    expect(market.promptTemplate).toContain("行情阈值已触发");
    expect(webhook.promptTemplate).toContain("网络回调事件");
    expect(webhookTrigger).toMatchObject({ type: "webhook", title: "网络回调" });
    expect(marketTrigger).toMatchObject({ type: "market_threshold", title: "价格阈值" });
    expect(riskTrigger).toMatchObject({ type: "schedule", title: "风险扫描" });
    expect(scheduleTrigger).toMatchObject({ type: "schedule", title: "开盘复盘" });
    expect(blankTrigger).toBeNull();
    expect(draft).toMatchObject({
      id: "draft-workflow",
      defaultInputs: { symbol: "US.AAPL" },
    });
    expect(workflowFormToDefinition(createWorkflowForm(), [])).toMatchObject({
      id: "draft-workflow",
      name: "未命名工作流",
    });
  });

  it("filters monitor logs by keyword and inclusive local date range", () => {
    const logs = [
      buildLog({
        id: "before",
        startedAt: "2026-06-30T23:59:59Z",
        inputs: { symbol: "US.TSLA" },
      }),
      buildLog({
        id: "target",
        startedAt: "2026-07-01T12:00:00Z",
        inputs: { symbol: "US.AAPL" },
        matchedEvent: { category: "broker.connection" },
        result: { markdown: "风险复盘完成" },
      }),
      buildLog({
        id: "after",
        startedAt: "2026-07-02T00:00:00Z",
        inputs: { symbol: "US.MSFT" },
      }),
    ];

    expect(
      filterWorkflowLogs(logs, {
        keyword: "broker.connection",
        from: "2026-07-01",
        to: "2026-07-01",
      }).map((log) => log.id),
    ).toEqual(["target"]);
    expect(filterWorkflowLogs(logs, { keyword: "us.msft" }).map((log) => log.id)).toEqual([
      "after",
    ]);
    expect(filterWorkflowLogs(logs).map((log) => log.id)).toEqual(["before", "target", "after"]);
    expect(filterWorkflowLogs([
      buildLog({
        id: "metadata-fallback",
        startedAt: "",
        createdAt: "2026-07-01T12:00:00Z",
        inputs: undefined,
        matchedEvent: undefined,
      }),
    ], { keyword: "succeeded", from: "2026-07-01" }).map((log) => log.id)).toEqual([
      "metadata-fallback",
    ]);
    expect(filterWorkflowLogs([
      buildLog({ id: "undated", startedAt: "", createdAt: "" }),
    ], { from: "2026-07-01" }).map((log) => log.id)).toEqual(["undated"]);
  });

  it("summarizes workflow and trigger invocation outcomes for user notices", () => {
    expect(workflowInvocationMessage("工作流", { response: { run: { id: "run-1" } } })).toBe(
      "工作流已启动：run-1",
    );
    expect(workflowInvocationMessage("工作流", { log: { runId: "run-log" } })).toBe(
      "工作流已启动：run-log",
    );
    expect(workflowInvocationMessage("触发器", { log: { status: "QUEUED" } })).toBe(
      "触发器已进入队列",
    );
    expect(
      workflowInvocationMessage("触发器", {
        log: { status: "SKIPPED", error: "冷却中" },
      }),
    ).toBe("触发器本次已跳过：冷却中");
    expect(
      workflowInvocationMessage("触发器", {
        log: { status: "SKIPPED" },
      }),
    ).toBe("触发器本次已跳过");
    expect(workflowInvocationMessage("工作流", {})).toBe("工作流已提交");
  });
});

function buildWorkflow(id: string) {
  return {
    id,
    name: "每日复盘",
    status: "ENABLED",
    agentId: "agent-1",
    workMode: "task",
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
