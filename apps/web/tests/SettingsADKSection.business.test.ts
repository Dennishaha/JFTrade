// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import SettingsADKSection from "../src/components/SettingsADKSection.vue";
import type { ADKAgent } from "../src/contracts";
import {
  buttonStub,
  flushRequests,
  inputStub,
  selectStub,
  tabStub,
  tabsStub,
  windowItemStub,
  windowStub,
} from "./helpers";

let currentState: ReturnType<typeof buildState>;

vi.mock("../src/composables/useADKSettingsSectionState", () => ({
  useADKSettingsSectionState: () => currentState,
}));

const alertStub = defineComponent({
  emits: ["click:close"],
  template:
    "<div class='alert-stub'><button type='button' class='close-alert' @click=\"$emit('click:close')\">close</button><slot /></div>",
});

afterEach(() => {
  document.body.innerHTML = "";
});

describe("SettingsADKSection business flows", () => {
  it("renders observation task and memory semantics, closes alerts, and wires child filter updates", async () => {
    currentState = buildState();
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        {
          path: "/settings/:section",
          component: SettingsADKSection,
        },
      ],
    });
    await router.push("/settings/adk?tab=observation&view=workflow");
    await router.isReady();

    const wrapper = mount(SettingsADKSection, {
      attachTo: document.body,
      global: {
        plugins: [router],
        stubs: {
          ADKProvidersPanel: { template: "<div />" },
          ADKAgentsPanel: { template: "<div />" },
          ADKToolsPanel: { template: "<div />" },
          ADKSkillsPanel: {
            emits: ["update:skillUrl"],
            template:
              "<button type='button' class='emit-skill-url' @click=\"$emit('update:skillUrl', 'https://skills.example/new')\">skill</button>",
          },
          ADKRunsPanel: {
            emits: [
              "update:runStatusFilter",
              "update:approvalStatusFilter",
              "update:auditKindFilter",
            ],
            template: `
              <div>
                <button type='button' class='emit-run-filter' @click="$emit('update:runStatusFilter', 'RUNNING')">run</button>
                <button type='button' class='emit-approval-filter' @click="$emit('update:approvalStatusFilter', 'APPROVED')">approval</button>
                <button type='button' class='emit-audit-filter' @click="$emit('update:auditKindFilter', 'memory.saved')">audit</button>
              </div>
            `,
          },
          "v-alert": alertStub,
          "v-btn": buttonStub,
          "v-card": { template: "<section><slot /></section>" },
          "v-card-text": { template: "<div><slot /></div>" },
          "v-card-title": { template: "<div><slot /></div>" },
          "v-chip": {
            props: ["color"],
            template: "<span class='v-chip-stub' :data-color='color'><slot /></span>",
          },
          "v-select": selectStub,
          "v-tab": tabStub,
          "v-tabs": tabsStub,
          "v-text-field": inputStub,
          "v-window": windowStub,
          "v-window-item": windowItemStub,
        },
      },
    });

    expect(wrapper.text()).toContain("智能体任务");
    expect(wrapper.text()).toContain("智能体记忆");
    expect(wrapper.text()).toContain("已完成任务");
    expect(wrapper.text()).toContain("阻塞任务");
    expect(wrapper.text()).toContain("工作区记忆");
    expect(wrapper.text()).toContain("智能体记忆");
    expect(wrapper.text()).toContain("对开启记忆的智能体全局可见");
    expect(wrapper.text()).toContain("仅当前智能体使用");
    expect(wrapper.text()).toContain("智能体：策略助手");

    const taskCards = wrapper
      .findAll("div")
      .filter((node) => node.text().includes("任务"));
    expect(wrapper.findAll(".v-chip-stub").some((chip) => chip.text() === "DONE" && chip.attributes("data-color") === "success")).toBe(true);
    expect(wrapper.findAll(".v-chip-stub").some((chip) => chip.text() === "IN_PROGRESS" && chip.attributes("data-color") === "info")).toBe(true);
    expect(wrapper.findAll(".v-chip-stub").some((chip) => chip.text() === "BLOCKED" && chip.attributes("data-color") === "warning")).toBe(true);
    expect(wrapper.findAll(".v-chip-stub").some((chip) => chip.text() === "CANCELLED" && chip.attributes("data-color") === "grey")).toBe(true);
    expect(wrapper.findAll(".v-chip-stub").some((chip) => chip.text() === "UNKNOWN" && chip.attributes("data-color") === "error")).toBe(true);
    expect(taskCards.length).toBeGreaterThan(0);

    const selects = wrapper.findAll("select");
    await selects[0]!.setValue("BLOCKED");
    await selects[1]!.setValue("agent-1");
    await selects[2]!.setValue("workspace");
    await selects[3]!.setValue("agent-1");
    await wrapper.find("input").setValue("preferred_market");
    expect(currentState.taskStatusFilter.value).toBe("BLOCKED");
    expect(currentState.taskAgentFilter.value).toBe("agent-1");
    expect(currentState.memoryScopeFilter.value).toBe("workspace");
    expect(currentState.memoryAgentFilter.value).toBe("agent-1");
    expect(currentState.memoryKeyFilter.value).toBe("preferred_market");

    const alerts = wrapper.findAll(".alert-stub");
    await alerts.find((alert) => alert.text().includes("配置加载失败"))!.find(".close-alert").trigger("click");
    await alerts.find((alert) => alert.text().includes("已保存"))!.find(".close-alert").trigger("click");
    expect(currentState.errorMessage.value).toBe("");
    expect(currentState.successMessage.value).toBe("");

    await router.push("/settings/adk?tab=observation&view=runs");
    await flushRequests();
    await wrapper.find(".emit-run-filter").trigger("click");
    await wrapper.find(".emit-approval-filter").trigger("click");
    await wrapper.find(".emit-audit-filter").trigger("click");
    expect(currentState.runStatusFilter.value).toBe("RUNNING");
    expect(currentState.approvalStatusFilter.value).toBe("APPROVED");
    expect(currentState.auditKindFilter.value).toBe("memory.saved");

    await router.push("/settings/adk?tab=skills");
    await flushRequests();
    await wrapper.find(".emit-skill-url").trigger("click");
    expect(currentState.skillUrl.value).toBe("https://skills.example/new");
  });

  it("normalizes route state and wires agent/tool tab child events without leaking outside ADK routes", async () => {
    currentState = buildState();
    currentState.tasks.value = [
      {
        id: "task-unbound",
        title: "待分配任务",
        description: "等待选择智能体",
        status: "TODO",
        agentId: "",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
    ];
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        {
          path: "/settings/:section",
          component: SettingsADKSection,
        },
      ],
    });
    await router.push("/settings/profile?tab=agents");
    await router.isReady();
    const replaceSpy = vi.spyOn(router, "replace");

    const wrapper = mount(SettingsADKSection, {
      attachTo: document.body,
      global: {
        plugins: [router],
        stubs: {
          ADKProvidersPanel: { template: "<div class='providers-stub' />" },
          ADKAgentsPanel: {
            emits: ["update:tool-category-filter", "update:tool-risk-filter"],
            template: `
              <div>
                <button type='button' class='emit-agent-category' @click="$emit('update:tool-category-filter', 'market-data')">category</button>
                <button type='button' class='emit-agent-risk' @click="$emit('update:tool-risk-filter', 'high')">risk</button>
              </div>
            `,
          },
          ADKToolsPanel: {
            emits: [
              "update:tool-category-filter",
              "update:tool-risk-filter",
              "update:tool-search-query",
              "update:tool-detail-dialog-open",
            ],
            template: `
              <div>
                <button type='button' class='emit-tool-category' @click="$emit('update:tool-category-filter', 'portfolio')">tool-category</button>
                <button type='button' class='emit-tool-risk' @click="$emit('update:tool-risk-filter', 'medium')">tool-risk</button>
                <button type='button' class='emit-tool-search' @click="$emit('update:tool-search-query', 'backtest')">tool-search</button>
                <button type='button' class='emit-tool-dialog' @click="$emit('update:tool-detail-dialog-open', true)">tool-dialog</button>
              </div>
            `,
          },
          ADKSkillsPanel: { template: "<div />" },
          ADKRunsPanel: { template: "<div />" },
          "v-alert": alertStub,
          "v-btn": buttonStub,
          "v-card": { template: "<section><slot /></section>" },
          "v-card-text": { template: "<div><slot /></div>" },
          "v-card-title": { template: "<div><slot /></div>" },
          "v-chip": {
            props: ["color"],
            template: "<span class='v-chip-stub' :data-color='color'><slot /></span>",
          },
          "v-select": selectStub,
          "v-tab": tabStub,
          "v-tabs": tabsStub,
          "v-text-field": inputStub,
          "v-window": windowStub,
          "v-window-item": windowItemStub,
        },
      },
    });

    currentState.activeTab.value = "agents";
    await flushRequests();
    expect(replaceSpy).not.toHaveBeenCalled();

    await router.push({
      path: "/settings/adk",
      query: {
        tab: ["observation", "agents"],
        view: ["workflow", "runs"],
      },
    });
    await flushRequests();
    expect(wrapper.text()).toContain("未绑定智能体");

    await router.push("/settings/adk?tab=agents");
    await flushRequests();
    await wrapper.find(".emit-agent-category").trigger("click");
    await wrapper.find(".emit-agent-risk").trigger("click");
    expect(currentState.toolCategoryFilter.value).toBe("market-data");
    expect(currentState.toolRiskFilter.value).toBe("high");

    await router.push("/settings/adk?tab=tools");
    await flushRequests();
    await wrapper.find(".emit-tool-category").trigger("click");
    await wrapper.find(".emit-tool-risk").trigger("click");
    await wrapper.find(".emit-tool-search").trigger("click");
    await wrapper.find(".emit-tool-dialog").trigger("click");
    expect(currentState.toolCategoryFilter.value).toBe("portfolio");
    expect(currentState.toolRiskFilter.value).toBe("medium");
    expect(currentState.toolSearchQuery.value).toBe("backtest");
    expect(currentState.toolDetailDialogOpen.value).toBe(true);

    currentState.activeTab.value = "invalid" as never;
    const setup = wrapper.vm.$.setupState as { observationTab: string };
    setup.observationTab = "invalid";
    await flushRequests();
    expect(currentState.activeTab.value).toBe("providers");
    expect(setup.observationTab).toBe("workflow");
  });
});

function buildState() {
  const agents = ref<ADKAgent[]>([
    buildAgent({ id: "agent-1", name: "策略助手" }),
  ]);
  return {
    activeTab: ref("providers"),
    agents,
    agentForm: ref({
      id: "",
      name: "新 Agent",
      instruction: "",
      providerId: "",
      model: "",
      tools: [],
      skills: [],
      permissionMode: "approval",
      memoryEnabled: true,
      recentUserWindow: 6,
      workMode: "chat",
      loopMaxIterations: 5,
      status: "ENABLED",
    }),
    agentTemplateNotice: ref(""),
    agentTemplates: ref([]),
    applyAgentTemplate: vi.fn(),
    approvalPage: ref({ limit: 10, offset: 0, total: 0, returned: 0, hasMore: false }),
    approvals: ref([]),
    approvalStatusFilter: ref("PENDING"),
    auditEvents: ref([]),
    auditKindFilter: ref(""),
    auditPage: ref({ limit: 12, offset: 0, total: 0, returned: 0, hasMore: false }),
    cancelOptimizationTask: vi.fn(),
    cancelRun: vi.fn(),
    deleteAgent: vi.fn(),
    deleteProvider: vi.fn(),
    duplicateAgent: vi.fn(),
    editAgent: vi.fn(),
    editProvider: vi.fn(),
    errorMessage: ref("配置加载失败"),
    filteredRuns: ref([]),
    formatDateTime: (value: string) => value,
    formatGenericStatusLabel: (status: string) => status,
    formatPermission: (value: string) => value,
    installSkill: vi.fn(),
    isInternalSkill: vi.fn(() => false),
    loading: ref(false),
    metrics: ref(null),
    memoryAgentFilter: ref(""),
    memoryEntries: ref([
      {
        id: "memory-workspace",
        scope: "workspace",
        key: "preferred_market",
        value: "HK",
        agentId: "",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
      {
        id: "memory-agent",
        scope: "agent",
        key: "risk_profile",
        value: "balanced",
        agentId: "agent-1",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
    ]),
    memoryKeyFilter: ref(""),
    memoryScopeFilter: ref(""),
    newAgentForm: vi.fn(),
    newProviderForm: vi.fn(),
    nextApprovalsPage: vi.fn(),
    nextAuditPage: vi.fn(),
    nextRunsPage: vi.fn(),
    optimizationTasks: ref([]),
    pageSummary: () => "0/0",
    pendingApprovals: ref([]),
    permissionModes: ref([{ title: "审批制", value: "approval" }]),
    preview: (value: unknown) => JSON.stringify(value),
    previousApprovalsPage: vi.fn(),
    previousAuditPage: vi.fn(),
    previousRunsPage: vi.fn(),
    providerForm: ref({
      id: "",
      displayName: "",
      baseUrl: "",
      model: "",
      contextWindowTokens: 0,
      requestTimeoutSeconds: 180,
      apiKey: "",
      enabled: true,
    }),
    runtimeSettingsForm: ref({
      runTimeoutSeconds: 1800,
      streamIdleTimeoutSeconds: 300,
    }),
    providerOptions: ref([{ title: "OpenAI", value: "provider-1" }]),
    providers: ref([]),
    resumeRun: vi.fn(),
    riskColor: vi.fn(() => "default"),
    riskLabel: vi.fn(() => "默认"),
    runPage: ref({ limit: 20, offset: 0, total: 0, returned: 0, hasMore: false }),
    runStatusFilter: ref("attention"),
    runTerminalMessage: vi.fn(() => ""),
    saveAgent: vi.fn(),
    saveProvider: vi.fn(),
    saveRuntimeSettings: vi.fn(),
    setDefaultProvider: vi.fn(),
    skillOptions: ref([]),
    skills: ref([]),
    skillUrl: ref(""),
    successMessage: ref("已保存"),
    taskAgentFilter: ref(""),
    taskStatusFilter: ref(""),
    tasks: ref([
      {
        id: "task-done",
        title: "已完成任务",
        description: "完成",
        status: "DONE",
        agentId: "agent-1",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
      {
        id: "task-progress",
        title: "进行中任务",
        description: "处理中",
        status: "IN_PROGRESS",
        agentId: "agent-1",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
      {
        id: "task-blocked",
        title: "阻塞任务",
        description: "等待外部依赖",
        status: "BLOCKED",
        agentId: "agent-1",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
      {
        id: "task-cancelled",
        title: "取消任务",
        description: "已取消",
        status: "CANCELLED",
        agentId: "agent-1",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
      {
        id: "task-todo",
        title: "待办任务",
        description: "待处理",
        status: "TODO",
        agentId: "agent-1",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
      {
        id: "task-unknown",
        title: "异常任务",
        description: "未知状态",
        status: "UNKNOWN",
        agentId: "agent-1",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      },
    ]),
    testProvider: vi.fn(),
    toolDetailDialogOpen: ref(false),
    toolCallStatusColor: vi.fn(() => "default"),
    toolCategoryFilter: ref(""),
    toolCategoryOptions: ref([]),
    toolSearchQuery: ref(""),
    toolOptions: ref([]),
    toolRiskFilter: ref(""),
    toolRiskOptions: ref([]),
    filteredTools: ref([]),
    openToolDetail: vi.fn(),
    closeToolDetail: vi.fn(),
    selectedTool: ref(null),
    tools: ref([]),
    uninstallSkill: vi.fn(),
  };
}

function buildAgent(overrides: Partial<ADKAgent> = {}): ADKAgent {
  return {
    id: "agent-1",
    name: "策略助手",
    instruction: "",
    providerId: "provider-1",
    model: "gpt-4.1",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat",
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}
