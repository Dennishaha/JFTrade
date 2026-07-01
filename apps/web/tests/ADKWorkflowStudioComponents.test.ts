// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h, type App, type Component } from "vue";

vi.mock("@vue-flow/core", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    VueFlow: defineComponent({
      emits: ["connect", "node-click", "update:nodes", "update:edges"],
      setup(_, { emit, slots }) {
        return () =>
          h("div", { class: "vue-flow-stub" }, [
            h(
              "button",
              {
                type: "button",
                "data-testid": "connect-edge",
                onClick: () => emit("connect", { source: "start", target: "agent" }),
              },
              "connect",
            ),
            slots["node-start"]?.({
              data: { title: "开始", subtitle: "1 个输入项", status: "ENABLED" },
              selected: false,
            }),
            slots["node-trigger"]?.({
              id: "trigger:trigger-1",
              data: { title: "开盘复盘", subtitle: "定时", status: "FAILED", runStatus: "FAILED" },
              selected: false,
            }),
            slots["node-agent"]?.({
              data: { title: "每日复盘", subtitle: "Agent", status: "task" },
              selected: false,
            }),
            slots["node-monitor"]?.({
              data: { title: "监控", subtitle: "1 条日志", status: "ALL" },
              selected: false,
            }),
            slots.default?.(),
          ]);
      },
    }),
  };
});

vi.mock("@vue-flow/background", async () => {
  const { defineComponent, h } = await import("vue");
  return { Background: defineComponent({ setup: () => () => h("div") }) };
});

vi.mock("@vue-flow/controls", async () => {
  const { defineComponent, h } = await import("vue");
  return { Controls: defineComponent({ setup: () => () => h("div") }) };
});

vi.mock("@vue-flow/minimap", async () => {
  const { defineComponent, h } = await import("vue");
  return { MiniMap: defineComponent({ setup: () => () => h("div") }) };
});

import ADKWorkflowCanvas from "../src/components/adk-page/ADKWorkflowCanvas.vue";
import ADKWorkflowAgentInspector from "../src/components/adk-page/ADKWorkflowAgentInspector.vue";
import ADKWorkflowDebugPanel from "../src/components/adk-page/ADKWorkflowDebugPanel.vue";
import ADKWorkflowEventTriggerPanel from "../src/components/adk-page/ADKWorkflowEventTriggerPanel.vue";
import ADKWorkflowMarketTriggerPanel from "../src/components/adk-page/ADKWorkflowMarketTriggerPanel.vue";
import ADKWorkflowMonitorPanel from "../src/components/adk-page/ADKWorkflowMonitorPanel.vue";
import ADKWorkflowNodeRunPreview from "../src/components/adk-page/ADKWorkflowNodeRunPreview.vue";
import ADKWorkflowNoticeStack from "../src/components/adk-page/ADKWorkflowNoticeStack.vue";
import ADKWorkflowPlanPanel from "../src/components/adk-page/ADKWorkflowPlanPanel.vue";
import ADKQueuePanel from "../src/components/adk-page/ADKQueuePanel.vue";
import ADKWorkflowScheduleTriggerPanel from "../src/components/adk-page/ADKWorkflowScheduleTriggerPanel.vue";
import ADKWorkflowSecretDialog from "../src/components/adk-page/ADKWorkflowSecretDialog.vue";
import ADKWorkflowStartInspector from "../src/components/adk-page/ADKWorkflowStartInspector.vue";
import ADKWorkflowStudioInspector from "../src/components/adk-page/ADKWorkflowStudioInspector.vue";
import ADKWorkflowStudioSidebar from "../src/components/adk-page/ADKWorkflowStudioSidebar.vue";
import ADKWorkflowStudioTopbar from "../src/components/adk-page/ADKWorkflowStudioTopbar.vue";
import ADKWorkflowTriggerInspector from "../src/components/adk-page/ADKWorkflowTriggerInspector.vue";
import ADKWorkflowWebhookTriggerPanel from "../src/components/adk-page/ADKWorkflowWebhookTriggerPanel.vue";
import { createTriggerForm, createWorkflowForm } from "../src/features/adkWorkflowForms";

describe("ADK workflow Studio components", () => {
  it("groups toolbar actions as icon buttons and emits workflow commands", async () => {
    const wrapper = mount(ADKWorkflowStudioTopbar, {
      props: {
        title: "每日复盘",
        description: "开盘前生成观察计划",
        status: "ENABLED",
        statusTone: "is-success",
        statusLabel: "已启用",
        loading: false,
        saving: false,
        runningWorkflow: false,
        logLoading: false,
        hasWorkflow: true,
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("每日复盘");
    expect(wrapper.find("[aria-label='运行']").classes()).toContain("is-success");
    expect(wrapper.find("[aria-label='保存']").classes()).toContain("is-primary");

    await wrapper.find("[aria-label='运行']").trigger("click");
    await wrapper.find("[aria-label='删除工作流']").trigger("click");

    expect(wrapper.emitted("run")).toHaveLength(1);
    expect(wrapper.emitted("remove")).toHaveLength(1);
  });

  it("keeps unavailable toolbar actions disabled while still exposing navigation controls", async () => {
    const wrapper = mount(ADKWorkflowStudioTopbar, {
      props: {
        title: "",
        description: "",
        status: "DISABLED",
        statusTone: "is-muted",
        statusLabel: "未启用",
        loading: false,
        saving: true,
        runningWorkflow: false,
        logLoading: false,
        hasWorkflow: false,
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("新的工作流");
    expect(wrapper.text()).toContain("开始 -> 智能体 -> 监控");
    expect(wrapper.find("[aria-label='删除工作流']").exists()).toBe(false);
    expect(wrapper.find("[aria-label='复制']").attributes("disabled")).toBeDefined();
    expect(wrapper.find("[aria-label='存为模板']").attributes("disabled")).toBeDefined();
    expect(wrapper.find("[aria-label='运行']").attributes("disabled")).toBeDefined();

    await wrapper.find("[aria-label='刷新']").trigger("click");
    await wrapper.find("[aria-label='显示右栏']").trigger("click");
    await wrapper.find("[aria-label='添加触发器']").trigger("click");
    await wrapper.find("[aria-label='触发日志']").trigger("click");
    await wrapper.find("[aria-label='调试']").trigger("click");
    await wrapper.find("[aria-label='保存']").trigger("click");

    expect(wrapper.emitted("refresh")).toHaveLength(1);
    expect(wrapper.emitted("showInspector")).toHaveLength(1);
    expect(wrapper.emitted("addTrigger")).toHaveLength(1);
    expect(wrapper.emitted("openLogs")).toHaveLength(1);
    expect(wrapper.emitted("debug")).toBeUndefined();
    expect(wrapper.emitted("save")).toBeUndefined();
    expect(wrapper.emitted("remove")).toBeUndefined();
  });

  it("lets users search, choose a template, and select an existing workflow from the sidebar", async () => {
    const workflow = {
      id: "workflow-1",
      name: "每日复盘",
      description: "",
      status: "ENABLED",
      agentId: "agent-1",
      workMode: "task",
      promptTemplate: "run",
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    };
    const wrapper = mount(ADKWorkflowStudioSidebar, {
      props: {
        workflows: [workflow],
        selectedWorkflowId: "",
        templates: [
          {
            value: "schedule",
            title: "定时复盘",
            description: "周一到五开盘前",
            icon: "fa-solid fa-clock",
          },
        ],
        showTemplatePicker: true,
        search: "",
        statusFilter: "",
        statusOptions: [{ title: "全部工作流", value: "" }],
        loading: false,
        page: { limit: 20, offset: 0, total: 1, returned: 1, hasMore: false },
        pageSummary: "1-1 / 1",
        agentName: () => "投研智能体",
        workModeLabel: () => "任务",
        workflowTone: () => "is-success",
        statusLabel: () => "已启用",
      },
      global: workflowMountGlobal(),
    });

    await wrapper.find(".adk-workflow-template").trigger("click");
    await wrapper.find(".adk-workflow-resource").trigger("click");

    expect(wrapper.find("input[placeholder='搜索工作流']").exists()).toBe(true);
    expect(wrapper.emitted("start-template")?.[0]).toEqual(["schedule"]);
    expect(wrapper.emitted("select-workflow")?.[0]).toEqual([workflow]);
  });

  it("renders canvas nodes and emits node selection plus new connections", async () => {
    const wrapper = mount(ADKWorkflowCanvas, {
      props: {
        nodes: [
          { id: "start", type: "start", position: { x: 0, y: 0 }, data: {} },
          { id: "agent", type: "agent", position: { x: 200, y: 0 }, data: {} },
        ],
        edges: [],
        selectedNodeId: "start",
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("开始");
    expect(wrapper.find(".adk-flow-node.is-trigger").classes()).toContain("is-run-failed");

    await wrapper.find(".adk-flow-node.is-start").trigger("click");
    await wrapper.find("[data-testid='connect-edge']").trigger("click");

    expect(wrapper.emitted("selectNode")?.[0]).toEqual(["start"]);
    expect(wrapper.emitted("connect")?.[0]).toEqual([{ source: "start", target: "agent" }]);
  });

  it("keeps debug inputs in the Studio flow and emits run controls", async () => {
    const wrapper = mount(ADKWorkflowDebugPanel, {
      props: {
        inputRows: [
          { key: "symbol", type: "string", value: "US.AAPL", booleanValue: false },
          { key: "dryRun", type: "boolean", value: "", booleanValue: true },
        ],
        running: false,
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("调试输入");
    expect(wrapper.findAll(".adk-workflow-input-row")).toHaveLength(2);

    const buttons = wrapper.findAll("button");
    await buttons.find((button) => button.text().includes("添加输入"))!.trigger("click");
    await buttons.find((button) => button.text().includes("开始调试"))!.trigger("click");
    await wrapper.find(".adk-workflow-input-row button").trigger("click");

    expect(wrapper.emitted("addInput")).toHaveLength(1);
    expect(wrapper.emitted("run")).toHaveLength(1);
    expect(wrapper.emitted("removeInput")?.[0]).toEqual([0]);
  });

  it("shows run notices with a chat run link and dismiss actions", async () => {
    const wrapper = mount(ADKWorkflowNoticeStack, {
      props: {
        errorMessage: "请先启用工作流后运行",
        successMessage: "工作流已启动：run-1",
        runHref: "/adk/agents?sessionId=session-1&runId=run-1",
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("请先启用工作流后运行");
    expect(wrapper.text()).toContain("工作流已启动：run-1");
    expect(wrapper.find("a").attributes("href")).toBe(
      "/adk/agents?sessionId=session-1&runId=run-1",
    );

    const closeButtons = wrapper.findAll(".adk-workflow-notice__close");
    await closeButtons[0]!.trigger("click");
    await closeButtons[1]!.trigger("click");

    expect(wrapper.emitted("dismissError")).toHaveLength(1);
    expect(wrapper.emitted("dismissSuccess")).toHaveLength(1);
  });

  it("shows the one-time webhook secret and closes the dialog", async () => {
    const wrapper = mount(ADKWorkflowSecretDialog, {
      props: {
        modelValue: true,
        secret: "secret-once",
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("网络回调密钥");
    expect(wrapper.text()).toContain("secret-once");

    await wrapper.findAll("button").find((button) => button.text().includes("关闭"))!.trigger("click");

    expect(wrapper.emitted("update:modelValue")?.[0]).toEqual([false]);
  });

  it("updates Start node form fields and emits refresh/add/remove events from the inspector", async () => {
    const workflowForm = createWorkflowForm("agent-1", "Run {{ .symbol }}");
    workflowForm.name = "每日复盘";
    workflowForm.inputRows = [
      { key: "symbol", type: "string", value: "US.AAPL", booleanValue: false },
    ];
    workflowForm.preservedDefaultInputs = { complex: { nested: true } };

    const wrapper = mountInspector({
      inspectorKind: "start",
      workflowForm,
      preservedInputCount: 1,
    });

    await wrapper.findAll("button").find((button) => button.text().includes("添加"))!.trigger("click");
    await wrapper.find(".adk-workflow-input-row button").trigger("click");

    expect(wrapper.text()).toContain("已保留 1 个复杂输入字段");
    expect(wrapper.emitted("addInputRow")).toHaveLength(1);
    expect(wrapper.emitted("removeInputRow")?.[0]).toEqual([0]);
  });

  it("keeps Start inspector metadata and input actions isolated", async () => {
    const workflowForm = createWorkflowForm("agent-1", "Run");
    workflowForm.inputRows = [];

    const wrapper = mount(ADKWorkflowStartInspector, {
      props: {
        workflowForm,
        selectedNodeRun: null,
        preservedInputCount: 0,
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("暂无输入项");

    await wrapper.findAll("button").find((button) => button.text().includes("添加"))!.trigger("click");

    expect(wrapper.emitted("addInputRow")).toHaveLength(1);
  });

  it("shows agent prompt variables and forwards insertion requests from the inspector", async () => {
    const workflowForm = createWorkflowForm("agent-1", "Run");
    workflowForm.providerId = "provider-1";

    const wrapper = mountInspector({
      inspectorKind: "agent",
      workflowForm,
      inputVariableOptions: [{ title: "symbol", value: "{{ .symbol }}" }],
    });

    await wrapper.findAll("button").find((button) => button.text().includes("symbol"))!.trigger("click");

    expect(wrapper.text()).toContain("默认模型 provider-1");
    expect(wrapper.emitted("insertPromptVariable")?.[0]).toEqual(["{{ .symbol }}"]);
  });

  it("keeps Agent inspector prompt variable actions explicit", async () => {
    const workflowForm = createWorkflowForm("agent-1", "Run");

    const wrapper = mount(ADKWorkflowAgentInspector, {
      props: {
        workflowForm,
        selectedNodeRun: null,
        agentOptions: [
          { title: "Agent 1", value: "agent-1" },
          { title: "Agent 2", value: "agent-2" },
        ],
        providerOptions: [{ title: "默认模型", value: "" }],
        inputVariableOptions: [{ title: "当前时间", value: "{{ .now }}" }],
        providerName: (providerId: string) => providerId || "默认模型",
      },
      global: workflowMountGlobal(),
    });

    await wrapper.findAll("button").find((button) => button.text().includes("当前时间"))!.trigger("click");

    expect(wrapper.emitted("insertPromptVariable")?.[0]).toEqual(["{{ .now }}"]);
  });

  it("renders webhook trigger health and emits run/delete actions", async () => {
    const triggerForm = createTriggerForm("webhook");
    triggerForm.id = "trigger-1";
    triggerForm.title = "外部事件";

    const wrapper = mountInspector({
      inspectorKind: "trigger",
      triggerForm,
      selectedTrigger: {
        id: "trigger-1",
        workflowId: "workflow-1",
        type: "webhook",
        title: "外部事件",
        status: "ENABLED",
        hasSecret: true,
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
      webhookEndpoint: "http://localhost/api/v1/adk/workflow-webhooks/trigger-1",
      webhookCurlSample: "curl -X POST",
    });

    expect(wrapper.text()).toContain("网络回调");
    expect(wrapper.text()).toContain("密钥已生成");
    expect(wrapper.text()).toContain("curl -X POST");

    const buttons = wrapper.findAll("button");
    await buttons.find((button) => button.text().includes("运行"))!.trigger("click");
    await buttons.find((button) => button.text().includes("删除"))!.trigger("click");

    expect(wrapper.emitted("runSelectedTrigger")).toHaveLength(1);
    expect(wrapper.emitted("removeSelectedTrigger")).toHaveLength(1);
  });

  it("edits schedule trigger presets and surfaces scheduler health", async () => {
    const triggerForm = createTriggerForm("schedule");
    triggerForm.schedule.frequency = "weekly";
    triggerForm.schedule.time = "08:00";
    triggerForm.schedule.timezone = "Asia/Shanghai";

    const wrapper = mount(ADKWorkflowScheduleTriggerPanel, {
      props: {
        triggerForm,
        selectedTrigger: {
          id: "trigger-1",
          workflowId: "workflow-1",
          type: "schedule",
          title: "开盘前",
          status: "ENABLED",
          nextRunAt: "2026-07-02T00:00:00Z",
          lastError: "cron 表达式不可用",
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
        schedulePreviewRuns: ["2026-07-02 08:00", "2026-07-03 08:00"],
        formatDateTime: (value: string) => `格式化 ${value}`,
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("定时规则");
    expect(wrapper.find("select[label='星期']").exists()).toBe(true);
    expect(wrapper.text()).toContain("2026-07-02 08:00");
    expect(wrapper.text()).toContain("下一次：格式化 2026-07-02T00:00:00Z");
    expect(wrapper.text()).toContain("最近错误：cron 表达式不可用");

    expect(wrapper.find("input[label='时间']").exists()).toBe(true);
    expect(wrapper.find("input[label='时区']").exists()).toBe(true);

    const customTriggerForm = createTriggerForm("schedule");
    customTriggerForm.schedule.frequency = "custom";
    const customWrapper = mount(ADKWorkflowScheduleTriggerPanel, {
      props: {
        triggerForm: customTriggerForm,
        selectedTrigger: null,
        schedulePreviewRuns: [],
        formatDateTime: (value: string) => value,
      },
      global: workflowMountGlobal(),
    });
    expect(customWrapper.find("input[label='自定义定时表达式']").exists()).toBe(true);
  });

  it("edits event trigger matching fields inline", async () => {
    const triggerForm = createTriggerForm("event");
    const wrapper = mount(ADKWorkflowEventTriggerPanel, {
      props: { triggerForm },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("事件匹配");
    expect(wrapper.find("input[label='来源']").exists()).toBe(true);
    expect(wrapper.find("input[label='事件类型']").exists()).toBe(true);
    expect(wrapper.find("input[label='实体标识']").exists()).toBe(true);
    expect(wrapper.find("input[label='分类']").exists()).toBe(true);
    expect(wrapper.find("input[label='级别']").exists()).toBe(true);
    expect(wrapper.find("input[label='冷却秒数']").exists()).toBe(true);
  });

  it("shows market trigger observability without losing editable threshold fields", async () => {
    const triggerForm = createTriggerForm("market_threshold");
    triggerForm.market.cooldownSec = "";

    const wrapper = mount(ADKWorkflowTriggerInspector, {
      props: {
        triggerForm,
        selectedTrigger: {
          id: "trigger-1",
          workflowId: "workflow-1",
          type: "market_threshold",
          title: "价格阈值",
          status: "ENABLED",
          lastRunAt: "2026-07-01T01:00:00Z",
          lastError: "价格字段缺失",
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
        selectedNodeRun: null,
        triggerRunSummary: null,
        schedulePreviewRuns: [],
        webhookEndpoint: "",
        webhookCurlSample: "",
        latestMarketEvent: { instrumentId: "US.AAPL", price: 123.45 },
        triggerLoading: false,
        runningTrigger: false,
        saving: false,
        preservedConfigCount: 2,
        formatDateTime: (value: string) => value,
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("行情阈值");
    expect(wrapper.text()).toContain("冷却时间：900 秒");
    expect(wrapper.text()).toContain("价格字段缺失");
    expect(wrapper.text()).toContain("US.AAPL");
    expect(wrapper.text()).toContain("已保留 2 个高级配置字段");

    await wrapper.findAll("button").find((button) => button.text().includes("运行"))!.trigger("click");

    expect(wrapper.emitted("runSelectedTrigger")).toHaveLength(1);
  });

  it("edits market threshold fields and displays latest market evidence", async () => {
    const triggerForm = createTriggerForm("market_threshold");
    const wrapper = mount(ADKWorkflowMarketTriggerPanel, {
      props: {
        triggerForm,
        selectedTrigger: {
          id: "trigger-1",
          workflowId: "workflow-1",
          type: "market_threshold",
          title: "价格突破",
          status: "ENABLED",
          lastRunAt: "2026-07-01T01:00:00Z",
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
        latestMarketEvent: { instrumentId: "US.TSLA", snapshot: { price: 260 } },
        formatDateTime: (value: string) => `格式化 ${value}`,
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("行情观测");
    expect(wrapper.text()).toContain("上次触发：格式化 2026-07-01T01:00:00Z");
    expect(wrapper.text()).toContain("US.TSLA");
    expect(wrapper.find("input[label='标的列表']").exists()).toBe(true);
    expect(wrapper.find("input[label='指标路径']").exists()).toBe(true);
    expect(wrapper.find("select[label='比较符']").exists()).toBe(true);
    expect(wrapper.find("input[label='阈值']").exists()).toBe(true);
    expect(wrapper.find("select[label='触发边沿']").exists()).toBe(true);
    expect(wrapper.find("input[label='冷却秒数']").exists()).toBe(true);
  });

  it("shows webhook setup states and lets users request a secret reset", async () => {
    const triggerForm = createTriggerForm("webhook");
    const unsaved = mount(ADKWorkflowWebhookTriggerPanel, {
      props: {
        triggerForm,
        selectedTrigger: null,
        triggerRunSummary: null,
        webhookEndpoint: "保存触发器后生成网络回调地址",
        webhookCurlSample: "",
        formatDateTime: (value: string) => value,
      },
      global: workflowMountGlobal(),
    });

    expect(unsaved.text()).toContain("保存后生成回调地址和密钥");

    triggerForm.id = "trigger-1";
    const wrapper = mount(ADKWorkflowWebhookTriggerPanel, {
      props: {
        triggerForm,
        selectedTrigger: {
          id: "trigger-1",
          workflowId: "workflow-1",
          type: "webhook",
          title: "外部回调",
          status: "ENABLED",
          hasSecret: false,
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
        triggerRunSummary: {
          total: 3,
          failures: 1,
          latest: {
            id: "log-2",
            workflowId: "workflow-1",
            triggerId: "trigger-1",
            triggerType: "webhook",
            status: "FAILED",
            errorMessage: "签名错误",
            createdAt: "2026-07-01T02:00:00Z",
            updatedAt: "2026-07-01T02:00:01Z",
          },
        },
        webhookEndpoint: "/api/v1/adk/workflow-webhooks/trigger-1",
        webhookCurlSample: "curl -X POST /api",
        formatDateTime: (value: string) => `格式化 ${value}`,
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("密钥未生成");
    expect(wrapper.text()).toContain("最近请求：失败 · 格式化 2026-07-01T02:00:00Z");
    expect(wrapper.text()).toContain("curl -X POST /api");

    expect(wrapper.find("label[data-vuetify='v-switch'][data-label='重置回调密钥']").exists()).toBe(true);
  });

  it("propagates trigger inspector edits and lets users hide the Inspector rail", async () => {
    const triggerForm = createTriggerForm("event");
    const wrapper = mountInspector({
      inspectorKind: "trigger",
      triggerForm,
      selectedTrigger: {
        id: "trigger-1",
        workflowId: "workflow-1",
        type: "event",
        title: "风控事件",
        status: "ENABLED",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
    });

    await wrapper.find("[data-testid='adk-workflow-inspector-hide']").trigger("click");

    expect(wrapper.emitted("hideInspector")).toHaveLength(1);
    expect(wrapper.find("select[label='状态']").exists()).toBe(true);
    expect(wrapper.find("input[label='标题']").exists()).toBe(true);
    expect(wrapper.text()).toContain("事件匹配");
  });

  it("shows monitor statistics, run result, and emits log/node/result actions", async () => {
    const log = successfulWorkflowLog();
    const wrapper = mountInspector({
      inspectorKind: "monitor",
      selectedLog: log,
      visibleLogs: [log],
      workflowStats: { total: 3, successRate: 67, avgMs: 1500, recent: 2 },
      runLink: () => "/adk/agents?sessionId=session-1&runId=run-1",
    });

    expect(wrapper.text()).toContain("运行次数");
    expect(wrapper.text()).toContain("67%");
    expect(wrapper.text()).toContain("1.5 秒");
    expect(wrapper.text()).toContain("复盘完成");
    expect(wrapper.find("a").attributes("href")).toBe("/adk/agents?sessionId=session-1&runId=run-1");

    await wrapper.find(".adk-log-item").trigger("click");
    await wrapper.find(".adk-node-trace__item").trigger("click");
    await wrapper.findAll("button").find((button) => button.text().includes("复制结果"))!.trigger("click");

    expect(wrapper.emitted("selectLog")?.[0]).toEqual(["log-1"]);
    expect(wrapper.emitted("selectNode")?.[0]).toEqual(["trigger:trigger-1"]);
    expect(wrapper.emitted("copyResultMarkdown")).toHaveLength(1);
  });

  it("lets Monitor users refresh logs and page through run history", async () => {
    const log = successfulWorkflowLog();
    const wrapper = mount(ADKWorkflowMonitorPanel, {
      props: {
        workflowName: "每日复盘",
        selectedNodeId: "monitor",
        selectedLog: log,
        visibleLogs: [log],
        workflowStats: { total: 8, successRate: 75, avgMs: 2300, recent: 4 },
        logTriggerOptions: [{ title: "全部触发器", value: "" }],
        logStatusFilter: "",
        logTriggerFilter: "",
        logKeywordFilter: "",
        logFromFilter: "",
        logToFilter: "",
        logLoading: false,
        logPage: { limit: 20, offset: 20, total: 41, returned: 20, hasMore: true },
        logPageSummary: "21-40 / 41",
        formatDateTime: (value: string) => value,
        runLink: () => "/adk/agents?sessionId=session-1&runId=run-1",
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("运行监控");
    expect(wrapper.text()).toContain("75%");
    expect(wrapper.text()).toContain("21-40 / 41");

    const buttons = wrapper.findAll("button");
    await buttons.find((button) => button.text().includes("刷新"))!.trigger("click");
    await buttons.find((button) => button.text().includes("上一页"))!.trigger("click");
    await buttons.find((button) => button.text().includes("下一页"))!.trigger("click");

    expect(wrapper.emitted("refreshLogs")).toHaveLength(1);
    expect(wrapper.emitted("previousLogPage")).toHaveLength(1);
    expect(wrapper.emitted("nextLogPage")).toHaveLength(1);
  });

  it("renders node run details for debugging selected workflow steps", () => {
    const wrapper = mount(ADKWorkflowNodeRunPreview, {
      props: {
        run: {
          nodeId: "agent",
          nodeType: "agent",
          title: "每日复盘",
          status: "SUCCEEDED",
          startedAt: "2026-07-01T00:00:00Z",
          finishedAt: "2026-07-01T00:00:02Z",
          inputs: { symbol: "US.AAPL" },
          outputs: { markdown: "完成" },
        },
      },
      global: workflowMountGlobal(),
    });

    expect(wrapper.text()).toContain("最近节点运行");
    expect(wrapper.text()).toContain("成功");
    expect(wrapper.text()).toContain("2.0 秒");
    expect(wrapper.text()).toContain("US.AAPL");
  });

  it("toggles generic queue content while preserving summary and status cues", async () => {
    const wrapper = mount(ADKQueuePanel, {
      props: {
        title: "审批队列",
        count: 2,
        status: "PENDING_APPROVAL",
        statusLabel: "等待审批",
        summary: "需要确认交易计划",
        defaultExpanded: false,
      },
    });

    expect(wrapper.text()).toContain("审批队列");
    expect(wrapper.text()).toContain("等待审批");
    expect(wrapper.text()).toContain("需要确认交易计划");
    await wrapper.setProps({ count: 3 });
    expect(wrapper.text()).toContain("3");
  });

  it("summarizes non-chat workflow plans and normalizes completed child runs", async () => {
    const wrapper = mount(ADKWorkflowPlanPanel, {
      props: {
        run: {
          id: "run-1",
          status: "COMPLETED",
          workflowStatus: "COMPLETED",
          workMode: "task",
          objective: "完成盘前复盘",
          iteration: 2,
          childRunIds: ["child-1", "child-2"],
          workflowPlan: [
            {
              taskId: "step-1",
              title: "收集持仓",
              description: "读取当前持仓",
              status: "RUNNING",
              childRunId: "child-1",
            },
            {
              title: "",
              message: "生成风险提示",
              status: "PENDING_APPROVAL",
              iteration: 2,
            },
          ],
        },
      },
    });

    expect(wrapper.text()).toContain("执行计划");
    expect(wrapper.text()).toContain("收集持仓");
    expect(wrapper.text()).toContain("收集持仓 · 已完成");
    expect(wrapper.text()).toContain("PENDING_APPROVAL");
    expect(wrapper.text()).toContain("完成");
  });

  it("keeps workflow plan hidden for chat runs and empty plans", () => {
    const chatWrapper = mount(ADKWorkflowPlanPanel, {
      props: {
        run: {
          id: "chat-run",
          status: "RUNNING",
          workMode: "chat",
          workflowPlan: [{ title: "聊天步骤", status: "RUNNING" }],
        },
      },
    });
    const emptyWrapper = mount(ADKWorkflowPlanPanel, {
      props: {
        run: {
          id: "empty-run",
          status: "RUNNING",
          workMode: "task",
          workflowPlan: [],
        },
      },
    });

    expect(chatWrapper.find(".adk-workspace-queue").exists()).toBe(false);
    expect(emptyWrapper.find(".adk-workspace-queue").exists()).toBe(false);
  });
});

function successfulWorkflowLog() {
  return {
    id: "log-1",
    workflowId: "workflow-1",
    triggerId: "trigger-1",
    triggerType: "schedule",
    status: "SUCCEEDED",
    sessionId: "session-1",
    runId: "run-1",
    result: { markdown: "复盘完成" },
    startedAt: "2026-07-01T00:00:00Z",
    finishedAt: "2026-07-01T00:00:05Z",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:05Z",
  };
}

function mountInspector(overrides: Partial<InstanceType<typeof ADKWorkflowStudioInspector>["$props"]> = {}) {
  const workflowForm = createWorkflowForm("agent-1", "Run {{ .symbol }}");
  workflowForm.name = "每日复盘";
  const triggerForm = createTriggerForm("schedule");
  return mount(ADKWorkflowStudioInspector, {
    props: {
      inspectorKind: "start",
      workflowForm,
      triggerForm,
      selectedTrigger: null,
      selectedNodeRun: null,
      selectedLog: null,
      visibleLogs: [],
      selectedNodeId: "start",
      workflowStats: { total: 0, successRate: 0, avgMs: 0, recent: 0 },
      triggerRunSummary: null,
      schedulePreviewRuns: [],
      webhookEndpoint: "保存触发器后生成网络回调地址",
      webhookCurlSample: "",
      latestMarketEvent: null,
      logTriggerOptions: [{ title: "全部触发器", value: "" }],
      logStatusFilter: "",
      logTriggerFilter: "",
      logKeywordFilter: "",
      logFromFilter: "",
      logToFilter: "",
      logLoading: false,
      triggerLoading: false,
      runningTrigger: false,
      saving: false,
      logPage: { limit: 20, offset: 0, total: 0, returned: 0, hasMore: false },
      logPageSummary: "0 / 0",
      preservedInputCount: 0,
      preservedConfigCount: 0,
      agentOptions: [{ title: "Agent", value: "agent-1" }],
      providerOptions: [{ title: "默认模型 provider-1", value: "provider-1" }],
      inputVariableOptions: [{ title: "symbol", value: "{{ .symbol }}" }],
      providerName: (providerId: string) => `默认模型 ${providerId}`,
      formatDateTime: (value: string) => value,
      runLink: () => "",
      ...overrides,
    },
    global: workflowMountGlobal(),
  });
}

function workflowMountGlobal() {
  const vuetifyComponents = workflowVuetifyComponents();
  return {
    stubs: vuetifyComponents,
    plugins: [
      {
        install(app: App) {
          Object.entries(vuetifyComponents).forEach(([name, component]) => {
            app.component(name, component);
          });
        },
      },
    ],
  };
}

function workflowVuetifyComponents(): Record<string, Component> {
  const button = defineComponent({
    props: ["disabled", "loading"],
    emits: ["click"],
    setup(props, { attrs, emit, slots }) {
      return () =>
        h(
          "button",
          {
            ...attrs,
            type: "button",
            disabled: Boolean(props.disabled || props.loading),
            onClick: (event: MouseEvent) => emit("click", event),
          },
          slots.default?.(),
        );
    },
  });
  const icon = defineComponent({
    setup(_, { attrs, slots }) {
      return () => h("span", attrs, slots.default?.());
    },
  });
  const textField = defineComponent({
    props: ["modelValue", "disabled"],
    emits: ["update:modelValue"],
    setup(props, { attrs, emit }) {
      return () =>
        h("input", {
          ...attrs,
          value: props.modelValue ?? "",
          disabled: Boolean(props.disabled),
          onInput: (event: Event) =>
            emit("update:modelValue", (event.target as HTMLInputElement).value),
        });
    },
  });
  const textarea = defineComponent({
    props: ["modelValue", "disabled"],
    emits: ["update:modelValue"],
    setup(props, { attrs, emit }) {
      return () =>
        h("textarea", {
          ...attrs,
          value: props.modelValue ?? "",
          disabled: Boolean(props.disabled),
          onInput: (event: Event) =>
            emit("update:modelValue", (event.target as HTMLTextAreaElement).value),
        });
    },
  });
  const select = defineComponent({
    props: ["modelValue", "items", "itemTitle", "itemValue"],
    emits: ["update:modelValue"],
    setup(props, { attrs, emit, slots }) {
      return () => {
        const items = (props.items as Array<string | Record<string, unknown>>) ?? [];
        const titleKey = (props.itemTitle as string) ?? "title";
        const valueKey = (props.itemValue as string) ?? "value";
        const options = items.map((item, index) => {
          const value = typeof item === "string" ? item : String(item[valueKey] ?? index);
          const label = typeof item === "string" ? item : String(item[titleKey] ?? value);
          return h("option", { key: value, value }, label);
        });
        return h(
          "select",
          {
            ...attrs,
            value: props.modelValue ?? "",
            onChange: (event: Event) =>
              emit("update:modelValue", (event.target as HTMLSelectElement).value),
          },
          options.length > 0 ? options : slots.default?.(),
        );
      };
    },
  });
  const toggle = defineComponent({
    props: ["modelValue", "label", "activeText"],
    emits: ["update:modelValue"],
    setup(props, { attrs, emit }) {
      return () =>
        h("label", { ...attrs, "data-vuetify": "v-switch", "data-label": props.label }, [
          h("input", {
            type: "checkbox",
            checked: Boolean(props.modelValue),
            onChange: (event: Event) =>
              emit("update:modelValue", (event.target as HTMLInputElement).checked),
          }),
          h("span", String(props.label ?? props.activeText ?? "")),
        ]);
    },
  });
  const dialog = defineComponent({
    props: ["modelValue"],
    emits: ["update:modelValue"],
    setup(props, { attrs, slots }) {
      return () => (props.modelValue ? h("div", attrs, slots.default?.()) : null);
    },
  });
  const passthrough = defineComponent({
    setup(_, { attrs, slots }) {
      return () => h("div", attrs, slots.default?.());
    },
  });

  return {
    "v-btn": button,
    "v-card": passthrough,
    "v-card-actions": passthrough,
    "v-card-text": passthrough,
    "v-card-title": passthrough,
    "v-dialog": dialog,
    "v-icon": icon,
    "v-select": select,
    "v-switch": toggle,
    "v-text-field": textField,
    "v-textarea": textarea,
    VBtn: button,
    VCard: passthrough,
    VCardActions: passthrough,
    VCardText: passthrough,
    VCardTitle: passthrough,
    VDialog: dialog,
    VIcon: icon,
    VSelect: select,
    VSwitch: toggle,
    VTextField: textField,
    VTextarea: textarea,
  };
}
