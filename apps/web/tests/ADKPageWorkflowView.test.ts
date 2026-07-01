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
              data: { title: "Daily Review", subtitle: "智能体", status: "task" },
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

});

function readSetupValue(value: unknown): unknown {
  if (value != null && typeof value === "object" && "value" in value) {
    return (value as { value: unknown }).value;
  }
  return value;
}

function setupStateOf(wrapper: { vm: { $: { setupState: Record<string, unknown> } } }): Record<string, unknown> {
  return wrapper.vm.$.setupState;
}

function stubWorkflowFetch(options: { logs?: unknown[] } = {}) {
  const logs = options.logs ?? [];
  const fetchMock = vi.fn(async (input: string | URL | Request, _init?: RequestInit) => {
    const url = String(input);
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
    if (url.includes("/api/v1/adk/workflows/workflow-1/triggers")) {
      return createResponse({ triggers: [buildTrigger()] });
    }
    if (url.includes("/api/v1/adk/workflow-trigger-logs")) {
      return createResponse({
        logs,
        page: { limit: 20, offset: 0, total: logs.length, returned: logs.length, hasMore: false },
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
    workMode: "task",
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
