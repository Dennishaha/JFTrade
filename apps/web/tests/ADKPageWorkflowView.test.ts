// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@vue-flow/core", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    VueFlow: defineComponent({
      name: "VueFlow",
      setup(_, { slots }) {
        return () =>
          h("div", { class: "vue-flow-stub" }, [
            slots["node-start"]?.({
              data: { title: "开始", subtitle: "1 个输入项", status: "ENABLED" },
              selected: false,
            }),
            slots["node-trigger"]?.({
              id: "trigger:trigger-1",
              data: { title: "开盘复盘", subtitle: "定时", status: "ENABLED" },
              selected: false,
            }),
            slots["node-agent"]?.({
              data: { title: "Daily Review", subtitle: "智能体", status: "loop" },
              selected: false,
            }),
            slots["node-monitor"]?.({
              data: { title: "监控", subtitle: "0 条日志", status: "ALL" },
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

vi.mock("../src/components/shared/SplitPane.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      name: "SplitPane",
      emits: ["resized"],
      setup(_, { slots }) {
        return () => h("div", { class: "split-pane-stub" }, slots.default?.());
      },
    }),
  };
});

vi.mock("../src/components/shared/SplitPaneItem.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      name: "SplitPaneItem",
      props: ["size", "minSize", "maxSize"],
      setup(_, { slots }) {
        return () => h("div", { class: "split-pane-item-stub" }, slots.default?.());
      },
    }),
  };
});

import ADKWorkflowStudio from "../src/components/adk-page/ADKWorkflowStudio.vue";
import {
  dialogStub,
  inputStub,
  passthroughStub,
  selectStub,
  switchStub,
  tabStub,
  tabsStub,
  textareaStub,
  createResponse,
  flushRequests,
} from "./helpers";

const buttonWithEventStub = {
  props: ["title", "icon", "disabled"],
  emits: ["click"],
  template:
    "<button type='button' :title='title' :disabled='disabled' @click=\"$emit('click')\"><span v-if='icon'>{{ icon }}</span><slot /></button>",
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ADKPage workflow view", () => {
  it("shows the workflow Studio under /adk/workflows with canvas nodes and structured editors", async () => {
    const fetchMock = stubWorkflowFetch();

    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflows",
      },
      global: {
        stubs: workflowVuetifyStubs(),
      },
    });
    await flushRequests();
    await flushRequests();

    expect(wrapper.text()).toContain("工作流中心");
    expect(wrapper.text()).toContain("Daily Review");
    expect(wrapper.text()).toContain("开始");
    expect(wrapper.text()).toContain("智能体");
    expect(wrapper.text()).toContain("监控");
    expect(wrapper.text()).toContain("已启用");
    expect(wrapper.text()).toContain("触发器");
    expect(wrapper.text()).toContain("触发日志");
    expect(wrapper.text()).toContain("输入项");
    expect(wrapper.text()).not.toContain("默认输入 JSON");
    expect(wrapper.text()).not.toContain("触发器配置 JSON");

    const toolbarActions = wrapper.findAll(".adk-workflow-actions .tv-toolbar-action");
    expect(toolbarActions.length).toBeGreaterThanOrEqual(9);
    expect(wrapper.find(".tv-toolbar-action[aria-label='添加触发器']").classes()).toContain("is-info");
    expect(wrapper.find(".tv-toolbar-action[aria-label='运行']").classes()).toContain("is-success");
    expect(wrapper.find(".tv-toolbar-action[aria-label='存为模板']").classes()).toContain("is-violet");
    expect(wrapper.find(".tv-toolbar-action[aria-label='保存']").classes()).toContain("is-primary");

    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("/api/v1/adk/workflows"))).toBe(true);
  });

  it("runs the selected workflow from the Studio toolbar and shows the run link", async () => {
    const fetchMock = stubWorkflowFetch();

    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflows",
      },
      global: {
        stubs: workflowVuetifyStubs(),
      },
    });
    await flushRequests();
    await flushRequests();
    await wrapper.vm.$nextTick();

    const runButton = wrapper.find("[data-testid='adk-workflow-run-button']");
    expect(runButton.exists()).toBe(true);
    await runButton.trigger("click");
    await flushRequests();
    await flushRequests();
    await flushRequests();
    await flushRequests();
    await wrapper.vm.$nextTick();

    expect(
      fetchMock.mock.calls.some(([input]) =>
        String(input).endsWith("/api/v1/adk/workflows/workflow-1/run"),
      ),
    ).toBe(true);
    const setupState = (wrapper.vm as unknown as { $: { setupState: Record<string, unknown> } }).$.setupState;
    expect(readSetupValue(setupState.successMessage)).toBe("工作流已启动：run-workflow");
    expect(readSetupValue(setupState.lastRunHref)).toBe("/adk/agents?sessionId=session-workflow&runId=run-workflow");
  });

  it("hides and restores the inspector from borderless icon actions", async () => {
    stubWorkflowFetch();

    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflows",
      },
      global: {
        stubs: workflowVuetifyStubs(),
      },
    });
    await flushRequests();

    const hideButton = wrapper.find("[data-testid='adk-workflow-inspector-hide']");
    expect(hideButton.exists()).toBe(true);
    expect(hideButton.classes()).toContain("adk-workflow-inspector__hide");
    const paneSizesBeforeHide = JSON.parse(
      JSON.stringify(readSetupValue(setupStateOf(wrapper).studioPaneSizes)),
    );
    await hideButton.trigger("click");
    await wrapper.vm.$nextTick();
    expect(readSetupValue(setupStateOf(wrapper).inspectorHidden)).toBe(true);
    expect(readSetupValue(setupStateOf(wrapper).studioPaneSizes)).toEqual(paneSizesBeforeHide);

    const showButton = wrapper.find("[data-testid='adk-workflow-inspector-show']");
    expect(showButton.exists()).toBe(true);
    await showButton.trigger("click");
    await wrapper.vm.$nextTick();

    expect(readSetupValue(setupStateOf(wrapper).inspectorHidden)).toBe(false);
  });

  it("runs debug mode with temporary input overrides", async () => {
    const fetchMock = stubWorkflowFetch();

    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflows",
      },
      global: {
        stubs: workflowVuetifyStubs(),
      },
    });
    await flushRequests();
    await flushRequests();

    const setupState = setupStateOf(wrapper);
    (setupState.openDebugPanel as () => void)();
    const debugRows = readSetupValue(setupState.debugInputRows) as Array<{ key: string; value: string }>;
    debugRows[0]!.value = "US.MSFT";
    await (setupState.runDebugWorkflow as () => Promise<void>)();
    await flushRequests();

    const runCall = fetchMock.mock.calls.find(([input]) =>
      String(input).endsWith("/api/v1/adk/workflows/workflow-1/run"),
    );
    expect(runCall).toBeTruthy();
    const init = runCall?.[1] as RequestInit | undefined;
    expect(JSON.parse(String(init?.body))).toEqual({ inputs: { symbol: "US.MSFT" } });
  });

  it("edits graph inputs, trigger drafts, prompt variables and persisted pane sizes", async () => {
    stubWorkflowFetch();
    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflows",
      },
      global: { stubs: workflowVuetifyStubs() },
    });
    await flushRequests();
    await flushRequests();
    const setup = setupStateOf(wrapper);
    const workflowForm = readSetupValue(setup.workflowForm) as {
      inputRows: unknown[];
      promptTemplate: string;
    };

    const originalInputs = workflowForm.inputRows.length;
    (setup.addInputRow as () => void)();
    expect(workflowForm.inputRows).toHaveLength(originalInputs + 1);
    (setup.removeInputRow as (index: number) => void)(originalInputs);
    expect(workflowForm.inputRows).toHaveLength(originalInputs);

    (setup.openDebugPanel as () => void)();
    const debugRows = readSetupValue(setup.debugInputRows) as unknown[];
    (setup.addDebugInputRow as () => void)();
    expect(debugRows).toHaveLength(originalInputs + 1);
    (setup.removeDebugInputRow as (index: number) => void)(originalInputs);
    expect(debugRows).toHaveLength(originalInputs);

    (setup.insertPromptVariable as (value: string) => void)("{{symbol}}");
    (setup.insertPromptVariable as (value: string) => void)("{{date}}");
    expect(workflowForm.promptTemplate).toContain("{{symbol}}\n{{date}}");

    (setup.addTriggerNode as (type: string) => void)("webhook");
    expect(readSetupValue(setup.selectedNodeId)).toMatch(/^trigger:draft-/);
    expect(readSetupValue(setup.draftTriggerPending)).toBe(true);
    (setup.selectNode as (id: string) => void)("trigger:unknown");
    expect(readSetupValue(setup.draftTriggerNodeId)).toBe("trigger:unknown");
    await (setup.removeSelectedTrigger as () => Promise<void>)();
    expect(readSetupValue(setup.selectedNodeId)).toBe("start");

    (setup.onNodeClick as (event: { node: { id: string } }) => void)({ node: { id: "monitor" } });
    expect(readSetupValue(setup.selectedNodeId)).toBe("monitor");
    (setup.onConnect as (connection: { source: string; target: string }) => void)({
      source: "start",
      target: "monitor",
    });

    const beforeSizes = JSON.parse(JSON.stringify(readSetupValue(setup.studioPaneSizes)));
    (setup.handleStudioOuterPaneResized as (payload: unknown) => void)({ panes: [{ size: 20 }, { size: 80 }] });
    (setup.handleStudioWorkbenchPaneResized as (payload: unknown) => void)({ panes: [{ size: 60 }, { size: 40 }] });
    expect(readSetupValue(setup.studioPaneSizes)).not.toEqual(beforeSizes);
    const normalizedSizes = JSON.parse(JSON.stringify(readSetupValue(setup.studioPaneSizes)));
    (setup.handleStudioOuterPaneResized as (payload: unknown) => void)({ panes: [{ size: 100 }] });
    expect(readSetupValue(setup.studioPaneSizes)).toEqual(normalizedSizes);

    expect((setup.agentName as (id: string) => string)("agent-1")).toBe("Agent");
    expect((setup.agentName as (id: string) => string)("missing")).toBe("missing");
    expect((setup.providerName as (id: string) => string)("")).toBe("默认模型");
    expect((setup.providerName as (id: string) => string)("provider-1")).toBe("OpenAI");
    expect((setup.providerName as (id: string) => string)("missing")).toBe("missing");

    const sidebar = wrapper.getComponent({ name: "ADKWorkflowStudioSidebar" });
    sidebar.vm.$emit("update:showTemplatePicker", true);
    sidebar.vm.$emit("update:search", "review");
    sidebar.vm.$emit("update:statusFilter", "ENABLED");
    const noticeStack = wrapper.getComponent({ name: "ADKWorkflowNoticeStack" });
    noticeStack.vm.$emit("dismiss-error");
    noticeStack.vm.$emit("dismiss-success");
    const canvas = wrapper.getComponent({ name: "ADKWorkflowCanvas" });
    canvas.vm.$emit("update:nodes", readSetupValue(setup.flowNodes));
    canvas.vm.$emit("update:edges", readSetupValue(setup.flowEdges));
    const inspector = wrapper.getComponent({ name: "ADKWorkflowStudioInspector" });
    inspector.vm.$emit("update:logKeywordFilter", "risk");
    inspector.vm.$emit("update:logFromFilter", "2026-07-01");
    inspector.vm.$emit("update:logToFilter", "2026-07-02");
    await wrapper.vm.$nextTick();

  });

  it("saves, duplicates, runs and deletes workflow resources through the Studio", async () => {
    const clipboardWrite = vi.fn(async () => {});
    vi.stubGlobal("navigator", {
      ...navigator,
      clipboard: { writeText: clipboardWrite },
    });
    const fetchMock = stubWorkflowFetch({ logs: [buildResultLog()] });
    vi.stubGlobal("confirm", vi.fn(() => true));
    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflows",
      },
      global: { stubs: workflowVuetifyStubs() },
    });
    await flushRequests();
    await flushRequests();
    const setup = setupStateOf(wrapper);

    (setup.selectNode as (id: string) => void)("trigger:trigger-1");
    await (setup.saveStudio as () => Promise<void>)();
    expect(readSetupValue(setup.successMessage)).toBe("工作流已保存");
    expect(fetchMock.mock.calls.some(([input, init]) =>
      String(input).endsWith("/api/v1/adk/workflows/workflow-1") && init?.method === "PUT",
    )).toBe(true);

    await (setup.duplicateWorkflow as (asTemplate?: boolean) => Promise<void>)(false);
    expect(readSetupValue(setup.successMessage)).toBe("工作流已复制");
    await (setup.duplicateWorkflow as (asTemplate?: boolean) => Promise<void>)(true);
    expect(readSetupValue(setup.successMessage)).toBe("已保存为模板副本");

    (setup.selectNode as (id: string) => void)("trigger:trigger-1");
    await (setup.saveCurrentTrigger as () => Promise<void>)();
    expect(readSetupValue(setup.secretDialogOpen)).toBe(true);
    expect(readSetupValue(setup.webhookSecret)).toBe("webhook-secret");
    await (setup.runSelectedTrigger as () => Promise<void>)();
    expect(readSetupValue(setup.successMessage)).toBe("触发器已启动：run-trigger");
    expect(readSetupValue(setup.lastRunHref)).toBe("/adk/agents?sessionId=session-trigger&runId=run-trigger");

    (setup.selectLog as (id: string) => void)("log-result");
    await (setup.copyResultMarkdown as () => Promise<void>)();
    expect(clipboardWrite).toHaveBeenCalledWith("# Workflow result");
    expect(readSetupValue(setup.successMessage)).toBe("结果已复制");
    expect((setup.runLink as (log: ReturnType<typeof buildResultLog>) => string)(buildResultLog()))
      .toBe("/adk/agents?sessionId=session-result&runId=run-result");

    await (setup.removeSelectedTrigger as () => Promise<void>)();
    expect(window.confirm).toHaveBeenCalled();
    await (setup.removeSelectedWorkflow as () => Promise<void>)();
    expect(fetchMock.mock.calls.some(([input, init]) =>
      String(input).endsWith("/api/v1/adk/workflows/workflow-copy") && init?.method === "DELETE",
    )).toBe(true);

  });

  it("guards unsaved and disabled workflow or trigger execution", async () => {
    stubWorkflowFetch();
    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflows",
      },
      global: { stubs: workflowVuetifyStubs() },
    });
    await flushRequests();
    const setup = setupStateOf(wrapper);
    const workflowForm = readSetupValue(setup.workflowForm) as { id: string; status: string };
    const triggerForm = readSetupValue(setup.triggerForm) as { id: string; status: string };

    workflowForm.id = "";
    await (setup.runWorkflowNow as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("请先保存工作流后运行");
    workflowForm.id = "workflow-1";
    workflowForm.status = "DISABLED";
    await (setup.runWorkflowNow as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("请先启用工作流后运行");

    triggerForm.id = "";
    await (setup.runSelectedTrigger as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("请先保存触发器");
    triggerForm.id = "trigger-1";
    triggerForm.status = "DISABLED";
    await (setup.runSelectedTrigger as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("请先启用触发器后运行");

  });

  it("resets paged filters, starts templates and ignores duplicate or cancelled actions", async () => {
    stubWorkflowFetch();
    vi.stubGlobal("confirm", vi.fn(() => false));
    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflow-logs",
      },
      global: { stubs: workflowVuetifyStubs() },
    });
    await flushRequests();
    await flushRequests();
    const setup = setupStateOf(wrapper);

    expect(readSetupValue(setup.selectedNodeId)).toBe("monitor");
    writeSetupValue(setup, "workflowStatusFilter", "DISABLED");
    writeSetupValue(setup, "logStatusFilter", "FAILED");
    writeSetupValue(setup, "logTriggerFilter", "trigger-1");
    await flushRequests();
    expect((readSetupValue(setup.workflowPage) as { offset: number }).offset).toBe(0);
    expect((readSetupValue(setup.logPage) as { offset: number }).offset).toBe(0);

    await wrapper.setProps({ viewMode: "workflows" });
    const templates = readSetupValue(setup.templates) as Array<{ value: string }>;
    (setup.startDraftWorkflow as (template: string) => void)(templates.find((item) => item.value === "blank")!.value);
    expect(readSetupValue(setup.selectedNodeId)).toBe("start");
    expect(readSetupValue(setup.draftTriggerPending)).toBe(false);
    (setup.startDraftWorkflow as (template: string) => void)(templates.find((item) => item.value === "schedule")!.value);
    expect(readSetupValue(setup.selectedNodeId)).toBe("trigger:draft");
    expect(readSetupValue(setup.draftTriggerPending)).toBe(true);

    const workflowForm = readSetupValue(setup.workflowForm) as { id: string; name: string; status: string };
    const triggerForm = readSetupValue(setup.triggerForm) as { id: string; title: string; status: string };
    workflowForm.id = "";
    await (setup.saveCurrentTrigger as (id?: string) => Promise<void>)("");
    expect(readSetupValue(setup.errorMessage)).toBe("请先保存工作流");
    await (setup.duplicateWorkflow as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("请先保存工作流");

    workflowForm.id = "workflow-1";
    workflowForm.name = "Daily Review";
    workflowForm.status = "ENABLED";
    writeSetupValue(setup, "runningWorkflow", true);
    await (setup.runWorkflowNow as () => Promise<void>)();
    writeSetupValue(setup, "runningWorkflow", false);
    triggerForm.id = "trigger-1";
    triggerForm.title = "Market open";
    triggerForm.status = "ENABLED";
    writeSetupValue(setup, "runningTrigger", true);
    await (setup.runSelectedTrigger as () => Promise<void>)();
    writeSetupValue(setup, "runningTrigger", false);

    await (setup.removeSelectedWorkflow as () => Promise<void>)();
    await (setup.removeSelectedTrigger as () => Promise<void>)();
    expect(window.confirm).toHaveBeenCalledTimes(2);

    (setup.selectWorkflow as (workflow: ReturnType<typeof buildWorkflow>) => void)(buildWorkflow());
    (setup.openWorkflowLogs as () => void)();
    expect(readSetupValue(setup.selectedNodeId)).toBe("monitor");
    (setup.handleStudioWorkbenchPaneResized as (payload: unknown) => void)({ panes: [{ size: 100 }] });

    writeSetupValue(setup, "selectedLogId", "");
    await (setup.copyResultMarkdown as () => Promise<void>)();
  });

  it("keeps Studio state recoverable across save, run and delete API failures", async () => {
    const baseFetch = stubWorkflowFetch();
    let failedOperation = "save";
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const method = input instanceof Request ? input.method : (init?.method ?? "GET");
      const shouldFail =
        (failedOperation === "save" && url.endsWith("/adk/workflows/workflow-1") && method === "PUT")
        || (failedOperation === "duplicate" && url.endsWith("/adk/workflows") && method === "POST")
        || (failedOperation === "workflow-run" && url.endsWith("/workflows/workflow-1/run"))
        || (failedOperation === "trigger-run" && url.endsWith("/workflow-triggers/trigger-1/run"))
        || (failedOperation === "workflow-delete" && url.endsWith("/adk/workflows/workflow-1") && method === "DELETE")
        || (failedOperation === "trigger-delete" && url.includes("/triggers/trigger-1") && method === "DELETE");
      if (shouldFail) return workflowErrorResponse(`${failedOperation} failed`);
      return baseFetch(input, init);
    }));
    vi.stubGlobal("confirm", vi.fn(() => true));
    const wrapper = mount(ADKWorkflowStudio, {
      props: {
        agents: [buildAgent()],
        providers: [buildProvider()],
        formatDateTime: (value: string) => value,
        viewMode: "workflows",
      },
      global: { stubs: workflowVuetifyStubs() },
    });
    await flushRequests();
    await flushRequests();
    const setup = setupStateOf(wrapper);
    const workflowForm = readSetupValue(setup.workflowForm) as { id: string; status: string };

    writeSetupValue(setup, "selectedWorkflowId", "missing");
    await wrapper.vm.$nextTick();
    (setup.selectWorkflow as (workflow: ReturnType<typeof buildWorkflow>) => void)(buildWorkflow());
    await flushRequests();
    (setup.selectNode as (id: string) => void)("trigger:trigger-1");
    expect((readSetupValue(setup.triggerForm) as { id: string }).id).toBe("trigger-1");

    await (setup.saveStudio as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("save failed");
    failedOperation = "duplicate";
    await (setup.duplicateWorkflow as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("duplicate failed");
    failedOperation = "workflow-run";
    await (setup.runWorkflowNow as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("workflow-run failed");
    failedOperation = "trigger-run";
    await (setup.runSelectedTrigger as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("trigger-run failed");

    workflowForm.id = "";
    await (setup.removeSelectedWorkflow as () => Promise<void>)();
    workflowForm.id = "workflow-1";
    failedOperation = "workflow-delete";
    await (setup.removeSelectedWorkflow as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("workflow-delete failed");
    failedOperation = "trigger-delete";
    await (setup.removeSelectedTrigger as () => Promise<void>)();
    expect(readSetupValue(setup.errorMessage)).toBe("trigger-delete failed");

  });
});

function readSetupValue(value: unknown): unknown {
  return value != null && typeof value === "object" && "value" in value ? (value as { value: unknown }).value : value;
}

function setupStateOf(wrapper: { vm: { $: { setupState: Record<string, unknown> } } }): Record<string, unknown> { return wrapper.vm.$.setupState; }

function writeSetupValue(setup: Record<string, unknown>, key: string, value: unknown): void {
  const current = setup[key];
  if (current != null && typeof current === "object" && "value" in current) {
    (current as { value: unknown }).value = value;
  } else {
    setup[key] = value;
  }
}

function stubWorkflowFetch(options: { logs?: unknown[] } = {}) {
  const logs = options.logs ?? [];
  let copyIndex = 0;
  const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const method = input instanceof Request ? input.method : (init?.method ?? "GET");
    if (url.includes("/api/v1/adk/agents")) {
      return createResponse({ agents: [buildAgent()] });
    }
    if (url.includes("/api/v1/adk/providers")) {
      return createResponse({ providers: [buildProvider()] });
    }
    if (url.includes("/api/v1/adk/sessions")) {
      return createResponse({ sessions: [] });
    }
    if (url.includes("/api/v1/adk/approvals")) {
      return createResponse({ approvals: [] });
    }
    if (url.includes("/api/v1/adk/tools")) {
      return createResponse({ tools: [] });
    }
    if (url.endsWith("/api/v1/adk/workflows/workflow-1/run")) {
      return createResponse({
        workflow: buildWorkflow(),
        log: {
          id: "log-run",
          workflowId: "workflow-1",
          triggerType: "manual",
          status: "SUCCEEDED",
          sessionId: "session-workflow",
          runId: "run-workflow",
          createdAt: "2026-07-01T00:00:00Z",
          updatedAt: "2026-07-01T00:00:00Z",
        },
      });
    }
    if (url.endsWith("/api/v1/adk/workflow-triggers/trigger-1/run")) {
      return createResponse({
        workflow: buildWorkflow(),
        log: {
          ...buildResultLog(),
          id: "log-trigger",
          sessionId: "session-trigger",
          runId: "run-trigger",
        },
      });
    }
    if (url.match(/\/api\/v1\/adk\/workflows\/[^/]+\/triggers\/[^/]+$/) && method === "DELETE") {
      return createResponse({ deleted: true, trigger: buildTrigger() });
    }
    if (url.match(/\/api\/v1\/adk\/workflows\/[^/]+\/triggers(?:\/[^/]+)?$/) && (method === "POST" || method === "PUT")) {
      return createResponse({ trigger: buildTrigger(), secret: "webhook-secret" });
    }
    if (url.includes("/api/v1/adk/workflows/workflow-1/triggers")) {
      return createResponse({ triggers: [buildTrigger()] });
    }
    if (url.includes("/api/v1/adk/workflow-trigger-logs")) {
      return createResponse({
        logs,
        page: { limit: 20, offset: 0, total: logs.length, returned: logs.length, hasMore: false },
      });
    }
    if (url.match(/\/api\/v1\/adk\/workflows\/[^/?]+$/) && method === "DELETE") {
      return createResponse({ deleted: true, workflow: buildWorkflow() });
    }
    if (url.endsWith("/api/v1/adk/workflows/workflow-1") && method === "PUT") {
      return createResponse(buildWorkflow());
    }
    if (url.endsWith("/api/v1/adk/workflows") && method === "POST") {
      copyIndex += 1;
      return createResponse({
        ...buildWorkflow(),
        id: "workflow-copy",
        name: copyIndex === 1 ? "Daily Review 副本" : "Daily Review 模板",
      });
    }
    if (url.includes("/api/v1/adk/workflows")) {
      return createResponse({
        workflows: [buildWorkflow()],
        page: { limit: 20, offset: 0, total: 1, returned: 1, hasMore: false },
      });
    }
    return createResponse({});
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function workflowVuetifyStubs() {
  const stubs = {
    "v-alert": { template: "<div><slot /></div>" },
    "v-btn": buttonWithEventStub,
    "v-card": { template: "<section><slot /></section>" },
    "v-card-actions": passthroughStub,
    "v-card-title": { template: "<div><slot /></div>" },
    "v-card-text": { template: "<div><slot /></div>" },
    "v-chip": { template: "<span><slot /></span>" },
    "v-dialog": dialogStub,
    Background: { template: "<div />" },
    Controls: { template: "<div />" },
    MiniMap: { template: "<div />" },
    SplitPane: {
      emits: ["resized"],
      template: "<div class='split-pane-stub'><slot /></div>",
    },
    SplitPaneItem: {
      props: ["size", "minSize", "maxSize"],
      template: "<div class='split-pane-item-stub'><slot /></div>",
    },
    VueFlow: {
      template: `
        <div class="vue-flow-stub">
          <slot name="node-start" :data="{ title: '开始', subtitle: '1 个输入项', status: 'ENABLED' }" :selected="false" />
          <slot name="node-trigger" id="trigger:trigger-1" :data="{ title: '开盘复盘', subtitle: '定时', status: 'ENABLED' }" :selected="false" />
          <slot name="node-agent" :data="{ title: 'Daily Review', subtitle: '智能体', status: 'task' }" :selected="false" />
          <slot name="node-monitor" :data="{ title: '监控', subtitle: '0 条日志', status: 'ALL' }" :selected="false" />
          <slot />
        </div>
      `,
    },
    "v-progress-linear": { template: "<div />" },
    "v-select": selectStub,
    "v-switch": switchStub,
    "v-tab": tabStub,
    "v-tabs": tabsStub,
    "v-text-field": inputStub,
    "v-textarea": textareaStub,
  };
  return {
    ...stubs,
    VAlert: stubs["v-alert"],
    VBtn: buttonWithEventStub,
    VCard: stubs["v-card"],
    VCardActions: stubs["v-card-actions"],
    VCardTitle: stubs["v-card-title"],
    VCardText: stubs["v-card-text"],
    VChip: stubs["v-chip"],
    VDialog: stubs["v-dialog"],
    VIcon: { template: "<span><slot /></span>" },
    VProgressLinear: stubs["v-progress-linear"],
    VSelect: stubs["v-select"],
    VSwitch: stubs["v-switch"],
    VTextField: stubs["v-text-field"],
    VTextarea: stubs["v-textarea"],
  };
}

function buildAgent() {
  return {
    id: "agent-1",
    name: "Agent",
    instruction: "test",
    providerId: "provider-1",
    model: "gpt-4o-mini",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    status: "ENABLED",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  };
}

function buildProvider() {
  return {
    id: "provider-1",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    enabled: true,
    default: true,
    hasApiKey: true,
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  };
}

function buildWorkflow() {
  return {
    id: "workflow-1",
    name: "Daily Review",
    description: "Review positions",
    status: "ENABLED",
    agentId: "agent-1",
    workMode: "loop",
    permissionMode: "approval",
    promptTemplate: "Run review",
    defaultInputs: { symbol: "US.AAPL" },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  };
}

function buildTrigger() {
  return {
    id: "trigger-1",
    workflowId: "workflow-1",
    type: "schedule",
    title: "Market open",
    status: "ENABLED",
    config: { cron: "30 9 * * 1-5", timezone: "Asia/Shanghai" },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  };
}

function buildResultLog() {
  return {
    id: "log-result",
    workflowId: "workflow-1",
    triggerId: "trigger-1",
    triggerType: "manual",
    status: "SUCCEEDED",
    sessionId: "session-result",
    runId: "run-result",
    result: { markdown: "# Workflow result" },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  };
}

function workflowErrorResponse(message: string): Response {
  return {
    ok: false,
    status: 500,
    json: async () => ({
      ok: false,
      error: { code: "INTERNAL", message },
      timestamp: "2026-07-01T00:00:00Z",
    }),
  } as Response;
}
