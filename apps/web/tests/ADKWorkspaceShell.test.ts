// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { computed, defineComponent, h, nextTick, ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import type { ADKAgent, ADKProvider, ADKTimelineEntry } from "@/contracts";

import ADKWorkspaceShell from "../src/components/adk-page/ADKWorkspaceShell.vue";
import { flushRequests } from "./helpers";

const { mermaidInitializeMock, mermaidRunMock } = vi.hoisted(() => ({
  mermaidInitializeMock: vi.fn(),
  mermaidRunMock: vi.fn(),
}));

vi.mock("mermaid", () => ({
  default: {
    initialize: mermaidInitializeMock,
    run: mermaidRunMock,
  },
}));

let currentControllerState: ReturnType<typeof buildControllerState>;

vi.mock("../src/composables/useADKMarkdownRenderer", () => ({
  useADKMarkdownRenderer: () => ({
    renderMarkdown: (value: string) => value,
  }),
}));

vi.mock("../src/composables/useADKPageController", () => ({
  useADKPageController: () => currentControllerState,
}));

const sessionSidebarStub = defineComponent({
  props: ["createNewSession", "selectSession"],
  template: `
    <div class="adk-sidebar">
      <button type="button" class="sidebar-create" @click="void createNewSession()">create</button>
      <button type="button" class="adk-session-item" @click="void selectSession('session-2')">session</button>
    </div>
  `,
});

const chatThreadStub = defineComponent({
  props: [
    "layout",
    "timelineEntries",
    "timelineTotal",
    "timelineWindowStart",
    "timelineWindowEnd",
    "timelineAtLatest",
    "errorMessage",
    "clearErrorMessage",
  ],
  emits: ["show-older-timeline", "show-newer-timeline", "show-latest-timeline"],
  template: `
    <div :class="layout === 'mobile' ? 'adk-chat-thread--mobile' : 'adk-chat-thread--desktop'">
      <div class="timeline-window">{{ timelineWindowStart }}-{{ timelineWindowEnd }} / {{ timelineTotal }} / {{ timelineAtLatest ? 'latest' : 'older' }}</div>
      <button type="button" class="older-window" @click="$emit('show-older-timeline')">older</button>
      <button type="button" class="newer-window" @click="$emit('show-newer-timeline')">newer</button>
      <button type="button" class="latest-window" @click="$emit('show-latest-timeline')">latest</button>
      <button type="button" class="clear-error" @click="clearErrorMessage()">clear</button>
      <div class="error-message">{{ errorMessage }}</div>
      <div
        v-for="entry in timelineEntries"
        :key="entry.id"
        class="timeline-entry"
      >
        <div
          v-if="String(entry.text).includes('\`\`\`mermaid')"
          class="mermaid"
        >
          graph TD;A-->B;
        </div>
        <div v-else v-html="entry.text" />
      </div>
    </div>
  `,
});

const composerStub = defineComponent({
  emits: [
    "update:chatDraft",
    "update:contextDetailsOpen",
    "update:selectedAgentId",
    "update:selectedProviderId",
    "update:permissionModeOverride",
    "update:workModeOverride",
  ],
  template: `
    <div class="composer-stub">
      <button type="button" class="set-draft" @click="$emit('update:chatDraft', 'updated draft')">draft</button>
      <button type="button" class="set-context-open" @click="$emit('update:contextDetailsOpen', true)">context</button>
      <button type="button" class="set-agent" @click="$emit('update:selectedAgentId', 'agent-2')">agent</button>
      <button type="button" class="set-provider" @click="$emit('update:selectedProviderId', 'provider-2')">provider</button>
      <button type="button" class="set-permission" @click="$emit('update:permissionModeOverride', 'all')">permission</button>
      <button type="button" class="set-work-mode" @click="$emit('update:workModeOverride', 'loop')">work</button>
    </div>
  `,
});

const approvalQueueStub = {
  template: "<div class='approval-queue' />",
};

const workflowPlanStub = {
  template: "<div class='workflow-plan' />",
};

let originalMatchMedia: typeof window.matchMedia;
let originalRequestAnimationFrame: typeof window.requestAnimationFrame;
let originalCancelAnimationFrame: typeof window.cancelAnimationFrame;

beforeEach(() => {
  currentControllerState = buildControllerState();
  mermaidInitializeMock.mockReset();
  mermaidRunMock.mockReset();

  originalMatchMedia = window.matchMedia;
  originalRequestAnimationFrame = window.requestAnimationFrame;
  originalCancelAnimationFrame = window.cancelAnimationFrame;
});

afterEach(() => {
  window.matchMedia = originalMatchMedia;
  window.requestAnimationFrame = originalRequestAnimationFrame;
  window.cancelAnimationFrame = originalCancelAnimationFrame;
});

describe("ADKWorkspaceShell", () => {
  it("renders mermaid timelines, paginates windows, clears errors, and leaves child views", async () => {
    currentControllerState = buildControllerState({
      errorMessage: "stream failed",
      selectedSessionId: "session-1",
      childViewContext: {
        runId: "child-1",
        title: "子任务",
        message: "等待继续",
      },
      visibleTimelineEntries: buildTimelineEntries(260),
    });

    window.requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      callback(16);
      return 1;
    });
    window.cancelAnimationFrame = vi.fn();
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    } as unknown as MediaQueryList);

    const wrapper = await mountShell();

    expect(mermaidInitializeMock).toHaveBeenCalledWith({
      startOnLoad: false,
      securityLevel: "strict",
    });
    expect(mermaidRunMock).toHaveBeenCalled();
    expect(wrapper.find(".timeline-window").text()).toContain("20-260 / 260 / latest");

    await wrapper.find(".older-window").trigger("click");
    expect(wrapper.find(".timeline-window").text()).toContain("0-240 / 260 / older");
    await wrapper.find(".newer-window").trigger("click");
    expect(wrapper.find(".timeline-window").text()).toContain("20-260 / 260 / latest");

    currentControllerState.visibleTimelineEntries.value = buildTimelineEntries(10);
    await flushRequests();
    expect(wrapper.find(".timeline-window").text()).toContain("0-10 / 10 / latest");

    await wrapper.find(".clear-error").trigger("click");
    expect(currentControllerState.errorMessage.value).toBe("");

    await wrapper.find(".set-draft").trigger("click");
    await wrapper.find(".set-context-open").trigger("click");
    await wrapper.find(".set-agent").trigger("click");
    await wrapper.find(".set-provider").trigger("click");
    await wrapper.find(".set-permission").trigger("click");
    await wrapper.find(".set-work-mode").trigger("click");
    expect(currentControllerState.chatDraft.value).toBe("updated draft");
    expect(currentControllerState.contextDetailsOpen.value).toBe(true);
    expect(currentControllerState.selectedAgentId.value).toBe("agent-2");
    expect(currentControllerState.selectedProviderId.value).toBe("provider-2");
    expect(currentControllerState.permissionModeOverride.value).toBe("all");
    expect(currentControllerState.workModeOverride.value).toBe("loop");

    const thread = wrapper.find(".adk-thread").element as HTMLDivElement;
    const header = wrapper.find(".adk-child-view-header").element as HTMLElement;
    Object.defineProperty(thread, "scrollTop", {
      configurable: true,
      writable: true,
      value: 50,
    });
    Object.defineProperty(header, "offsetTop", { configurable: true, value: 10 });
    Object.defineProperty(header, "offsetHeight", { configurable: true, value: 20 });
    await wrapper.find(".adk-thread").trigger("scroll");
    expect(wrapper.find(".adk-child-view-sticky").exists()).toBe(true);

    await wrapper.find(".adk-child-view-sticky button").trigger("click");
    expect(currentControllerState.setActiveChildRunId).toHaveBeenCalledWith("");
  });

  it("switches with matchMedia, opens the mobile drawer, creates/selects sessions, and cleans up listeners", async () => {
    const addEventListener = vi.fn();
    const removeEventListener = vi.fn();
    let changeListener: ((event: MediaQueryListEvent) => void) | undefined;
    addEventListener.mockImplementation(
      (_type: string, listener: (event: MediaQueryListEvent) => void) => {
        changeListener = listener;
      },
    );
    currentControllerState = buildControllerState({
      selectedAgentId: "agent-1",
      selectedSessionId: "session-1",
      sessions: [
        { id: "session-1", title: "已有会话" },
        { id: "session-2", title: "第二个会话" },
      ],
    });
    window.matchMedia = vi.fn().mockReturnValue({
      matches: true,
      addEventListener,
      removeEventListener,
    } as unknown as MediaQueryList);
    window.requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      callback(16);
      return 1;
    });
    window.cancelAnimationFrame = vi.fn();

    const wrapper = await mountShell();

    expect(wrapper.find(".adk-shell--mobile").exists()).toBe(true);
    expect(wrapper.find(".adk-mobile-toolbar").exists()).toBe(true);
    expect(wrapper.find(".adk-chat-thread--mobile").exists()).toBe(true);

    await wrapper.find("[data-testid='adk-mobile-sessions-toggle']").trigger("click");
    expect(wrapper.find("[data-testid='adk-mobile-session-drawer']").exists()).toBe(true);

    await wrapper.find(".sidebar-create").trigger("click");
    expect(currentControllerState.createNewSession).toHaveBeenCalled();
    expect(wrapper.find("[data-testid='adk-mobile-session-drawer']").exists()).toBe(false);

    await wrapper.find("[data-testid='adk-mobile-sessions-toggle']").trigger("click");
    await wrapper.find(".adk-session-item").trigger("click");
    expect(currentControllerState.selectSession).toHaveBeenCalledWith("session-2");
    expect(wrapper.find("[data-testid='adk-mobile-session-drawer']").exists()).toBe(false);

    await wrapper.find("[data-testid='adk-mobile-sessions-toggle']").trigger("click");
    await wrapper.find("[data-testid='adk-mobile-sessions-close']").trigger("click");
    expect(wrapper.find("[data-testid='adk-mobile-session-drawer']").exists()).toBe(false);

    changeListener?.({ matches: false } as MediaQueryListEvent);
    await nextTick();
    expect(wrapper.find(".adk-shell--mobile").exists()).toBe(false);

    wrapper.unmount();
    expect(removeEventListener).toHaveBeenCalled();
  });
});

async function mountShell() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { template: "<div />" } }],
  });
  const wrapper = mount(ADKWorkspaceShell, {
    global: {
      plugins: [router],
      stubs: {
        ADKSessionSidebar: sessionSidebarStub,
        ADKChatThread: chatThreadStub,
        ADKApprovalQueuePanel: approvalQueueStub,
        ADKWorkflowPlanPanel: workflowPlanStub,
        ADKChatComposer: composerStub,
      },
    },
  });
  await router.isReady();
  await flushRequests();
  return wrapper;
}

function buildControllerState(
  overrides: Partial<{
    errorMessage: string;
    selectedSessionId: string;
    selectedAgentId: string;
    childViewContext: { runId: string; title: string; message: string } | null;
    visibleTimelineEntries: ADKTimelineEntry[];
    sessions: Array<{ id: string; title: string }>;
  }> = {},
) {
  const selectedAgentId = ref(overrides.selectedAgentId ?? "agent-1");
  const sessions = ref(
    (overrides.sessions ?? [{ id: "session-1", title: "已有会话" }]).map(
      (session) => ({
        id: session.id,
        title: session.title,
        agentId: "agent-1",
        createdAt: "2026-07-01T00:00:00Z",
        updatedAt: "2026-07-01T00:00:00Z",
      }),
    ),
  );
  const selectedSessionId = ref(overrides.selectedSessionId ?? "");
  const providers = ref<ADKProvider[]>([
    {
      id: "provider-1",
      displayName: "OpenAI",
      baseUrl: "https://api.openai.com/v1",
      model: "gpt-4.1",
      requestTimeoutMs: 180_000,
      enabled: true,
      default: true,
      hasApiKey: true,
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    },
  ]);
  const selectedProviderId = ref("provider-1");
  const selectedAgent = computed<ADKAgent | null>(() => ({
    id: selectedAgentId.value,
    name: selectedAgentId.value === "agent-2" ? "目标助手" : "交易助手",
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
  }));
  const selectedProvider = computed<ADKProvider | null>(
    () =>
      providers.value.find((provider) => provider.id === selectedProviderId.value) ??
      providers.value[0] ??
      null,
  );
  return {
    activeRunId: ref("run-1"),
    activeRunStatus: ref("RUNNING"),
    activeChildRunId: ref("child-1"),
    agentName: (agentId: string) => agentId || "未绑定智能体",
    agentOptions: ref([{ title: "交易助手", value: "agent-1" }]),
    approvalTool: computed(() => null),
    approvalsBusy: ref(false),
    canInterruptChat: ref(false),
    canSendChat: ref(true),
    childRunItems: ref([]),
    childViewContext: ref(overrides.childViewContext ?? null),
    chatDraft: ref("draft"),
    composerBlockMessage: ref(""),
    cancelActiveRun: vi.fn(),
    contextBusy: ref(false),
    contextDetailsOpen: ref(false),
    creatingSession: ref(false),
    createNewSession: vi.fn(async () => {
      selectedSessionId.value = "session-created";
    }),
    deleteSession: vi.fn(),
    errorMessage: ref(overrides.errorMessage ?? ""),
    formatPermission: (mode: string) => mode,
    goalObjectiveDraft: ref(""),
    goalObjectiveError: ref(""),
    goalObjectiveSaving: ref(false),
    goalLifecycleBusy: ref(false),
    goalPaused: ref(false),
    goalTimedOut: ref(false),
    goalPauseRequested: ref(false),
    showGoalObjectiveEditor: ref(false),
    canSaveGoalObjective: ref(false),
    canPauseGoal: ref(false),
    canResumeGoal: ref(false),
    hasBlockingRun: ref(false),
    handleAgentChange: vi.fn(),
    handleComposerKeydown: vi.fn(),
    handleProviderChange: vi.fn(),
    interruptAndQueueChat: vi.fn(),
    interruptingRunId: ref(""),
    loading: ref(false),
    openProviderSettings: vi.fn(),
    pauseGoalRun: vi.fn(),
    preview: (value: unknown) => JSON.stringify(value),
    providerOptions: ref([{ title: "OpenAI", value: "provider-1" }]),
    providers,
    queueDispatchingId: ref(""),
    queuedMessages: ref([]),
    revokeQueuedMessage: vi.fn(),
    resumeGoalRun: vi.fn(),
    runSlashCommand: vi.fn(),
    savingProviderSelection: ref(false),
    selectedAgent,
    selectedApprovalQueue: ref([]),
    selectedAgentId,
    selectedProvider,
    selectedProviderId,
    selectedSessionId,
    sendingChat: ref(false),
    sessionContext: ref(null),
    sessionAgentFilter: ref(""),
    sessionSearch: ref(""),
    sessions,
    sessionTitle: (session: { title?: string; id: string }) =>
      session.title || session.id,
    showSessionGroups: computed(() => false),
    activityIndicator: ref("idle"),
    suggestions: ref<string[]>([]),
    composerPlaceholder: ref("输入问题或任务..."),
    emptyStateHint: ref("选择会话开始"),
    slashCommands: ref([]),
    selectSession: vi.fn(async (sessionId: string) => {
      selectedSessionId.value = sessionId;
    }),
    sendChat: vi.fn(),
    setActiveChildRunId: vi.fn(),
    updateGoalObjective: vi.fn(),
    updateGoalObjectiveDraft: vi.fn(),
    visibleSessionGroups: computed(() => [
      {
        id: "__default_conversation__",
        title: "对话",
        sessions: sessions.value,
        isDefault: true,
      },
    ]),
    visibleSessions: computed(() => sessions.value),
    visibleTimelineEntries: ref(
      overrides.visibleTimelineEntries ?? buildTimelineEntries(3),
    ),
    visibleWorkflowPlanRun: ref(null),
    workModeOverride: ref(""),
    permissionModeOverride: ref(""),
    openContextDetails: vi.fn(),
  };
}

function buildTimelineEntries(count: number): ADKTimelineEntry[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `entry-${index + 1}`,
    type: "assistant_message",
    text:
      index === count - 1
        ? "```mermaid\ngraph TD;A-->B;\n```"
        : `entry-${index + 1}`,
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  })) as ADKTimelineEntry[];
}
