// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick, ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ADKProvider, ADKSession } from "@/contracts";

import { flushRequests } from "./helpers";

const {
  createADKPageSessionMock,
  deleteADKPageSessionMock,
  fetchADKPageSessionDataMock,
  renameADKPageSessionMock,
} = vi.hoisted(() => ({
  createADKPageSessionMock: vi.fn(),
  deleteADKPageSessionMock: vi.fn(),
  fetchADKPageSessionDataMock: vi.fn(),
  renameADKPageSessionMock: vi.fn(),
}));

vi.mock("../src/composables/adkPageSessionApi", async () => {
  const actual = await vi.importActual<
    typeof import("../src/composables/adkPageSessionApi")
  >("../src/composables/adkPageSessionApi");
  return {
    ...actual,
    createADKPageSession: createADKPageSessionMock,
    deleteADKPageSession: deleteADKPageSessionMock,
    fetchADKPageSessionData: fetchADKPageSessionDataMock,
    renameADKPageSession: renameADKPageSessionMock,
  };
});

import { useADKPageSessionState } from "../src/composables/useADKPageSessionState";

beforeEach(() => {
  createADKPageSessionMock.mockReset();
  deleteADKPageSessionMock.mockReset();
  fetchADKPageSessionDataMock.mockReset();
  renameADKPageSessionMock.mockReset();
  fetchADKPageSessionDataMock.mockResolvedValue(buildPageData([buildSession()]));
});

describe("useADKPageSessionState", () => {
  it("shows a newly created session first and adjusts filters", async () => {
    const created = buildSession({
      id: "session-created",
      title: "新会话",
      agentId: "agent-1",
    });
    createADKPageSessionMock.mockResolvedValue(created);
    const { state } = await mountSessionState();
    state.sessionSearch.value = "不匹配";
    state.sessionAgentFilter.value = "agent-other";

    const reset = vi.fn();
    await state.createNewSession(reset);

    expect(state.sessions.value[0]).toEqual(created);
    expect(state.visibleSessions.value[0]).toEqual(created);
    expect(state.selectedSessionId.value).toBe(created.id);
    expect(state.sessionSearch.value).toBe("");
    expect(state.sessionAgentFilter.value).toBe(created.agentId);
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it("keeps a local create when an older refresh completes later", async () => {
    const existing = buildSession();
    const created = buildSession({
      id: "session-created-during-refresh",
      title: "并发创建",
    });
    const staleRefresh = deferred<ReturnType<typeof buildPageData>>();
    const { state } = await mountSessionState();
    fetchADKPageSessionDataMock.mockImplementationOnce(
      () => staleRefresh.promise,
    );
    createADKPageSessionMock.mockResolvedValue(created);

    const refreshPromise = state.refreshAll();
    await nextTick();
    await state.createNewSession(vi.fn());
    staleRefresh.resolve(buildPageData([existing]));
    await refreshPromise;

    expect(state.sessions.value.map((session) => session.id)).toEqual([
      created.id,
      existing.id,
    ]);

    fetchADKPageSessionDataMock.mockResolvedValueOnce(
      buildPageData([created, existing]),
    );
    await state.refreshAll();
    expect(state.sessions.value.map((session) => session.id)).toEqual([
      created.id,
      existing.id,
    ]);
  });

  it("ignores an older refresh after a newer refresh has already converged", async () => {
    const existing = buildSession();
    const created = buildSession({
      id: "session-created-before-newer-refresh",
      title: "新响应已包含",
    });
    const staleRefresh = deferred<ReturnType<typeof buildPageData>>();
    const { state } = await mountSessionState();
    fetchADKPageSessionDataMock.mockImplementationOnce(
      () => staleRefresh.promise,
    );
    createADKPageSessionMock.mockResolvedValue(created);

    const stalePromise = state.refreshAll();
    await nextTick();
    await state.createNewSession(vi.fn());
    fetchADKPageSessionDataMock.mockResolvedValueOnce(
      buildPageData([created, existing]),
    );
    await state.refreshAll();
    staleRefresh.resolve(buildPageData([existing]));
    await stalePromise;

    expect(state.sessions.value.map((session) => session.id)).toEqual([
      created.id,
      existing.id,
    ]);
  });

  it("restores filters and leaves the list unchanged when create fails", async () => {
    createADKPageSessionMock.mockRejectedValue(new Error("create failed"));
    const { state } = await mountSessionState();
    const before = [...state.sessions.value];
    state.sessionSearch.value = "保留搜索";
    state.sessionAgentFilter.value = "agent-other";

    await state.createNewSession(vi.fn());

    expect(state.sessions.value).toEqual(before);
    expect(state.sessionSearch.value).toBe("保留搜索");
    expect(state.sessionAgentFilter.value).toBe("agent-other");
    expect(state.errorMessage.value).toBe("create failed");
  });

  it("prevents duplicate create requests while one is pending", async () => {
    const create = deferred<ADKSession>();
    createADKPageSessionMock.mockImplementation(() => create.promise);
    const { state } = await mountSessionState();

    const first = state.createNewSession(vi.fn());
    const second = state.createNewSession(vi.fn());
    expect(state.creatingSession.value).toBe(true);
    expect(createADKPageSessionMock).toHaveBeenCalledTimes(1);

    create.resolve(buildSession({ id: "session-created-once" }));
    await Promise.all([first, second]);
    expect(state.creatingSession.value).toBe(false);
  });

  it("shows the default provider first without duplicating it", async () => {
    fetchADKPageSessionDataMock.mockResolvedValueOnce(
      buildPageData([buildSession()], [
        buildProvider({ id: "provider-other", displayName: "Other", model: "claude", default: false }),
        buildProvider({ id: "provider-default", displayName: "Default", model: "gpt-4o", default: true }),
      ]),
    );

    const { state } = await mountSessionState();

    expect(state.providerOptions.value).toHaveLength(2);
    expect(state.providerOptions.value[0]).toMatchObject({
      value: "",
      providerId: "provider-default",
      model: "gpt-4o",
      isDefault: true,
    });
    expect(state.providerOptions.value[1]).toMatchObject({
      value: "provider-other",
      providerId: "provider-other",
      isDefault: false,
    });
    expect(
      state.providerOptions.value.filter((option) => option.model === "gpt-4o"),
    ).toHaveLength(1);
  });

  it("deletes the selected session and resets the thread state", async () => {
    deleteADKPageSessionMock.mockResolvedValue(undefined);
    const { state } = await mountSessionState();
    state.selectedSessionId.value = "session-existing";

    const reset = vi.fn();
    await expect(state.deleteSession("session-existing", reset)).resolves.toBe(true);

    expect(state.sessions.value).toEqual([]);
    expect(state.selectedSessionId.value).toBe("");
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it("surfaces delete failures and keeps the current selection", async () => {
    deleteADKPageSessionMock.mockRejectedValue(new Error("delete failed"));
    const { state } = await mountSessionState();
    state.selectedSessionId.value = "session-existing";

    await expect(state.deleteSession("session-existing", vi.fn())).resolves.toBe(
      false,
    );

    expect(state.selectedSessionId.value).toBe("session-existing");
    expect(state.sessions.value).toHaveLength(1);
    expect(state.errorMessage.value).toBe("delete failed");
  });

  it("renames sessions only when the prompt returns a new title", async () => {
    const originalPrompt = window.prompt;
    const { state } = await mountSessionState();
    const session = state.sessions.value[0]!;

    window.prompt = vi.fn()
      .mockReturnValueOnce("   ")
      .mockReturnValueOnce(session.title)
      .mockReturnValueOnce("改名后的会话");
    renameADKPageSessionMock.mockResolvedValue({
      ...session,
      title: "改名后的会话",
    });

    await state.renameSession(session);
    await state.renameSession(session);
    expect(renameADKPageSessionMock).not.toHaveBeenCalled();

    await state.renameSession(session);
    expect(renameADKPageSessionMock).toHaveBeenCalledWith(
      session.id,
      "改名后的会话",
    );
    expect(state.sessions.value[0]?.title).toBe("改名后的会话");

    renameADKPageSessionMock.mockRejectedValueOnce(new Error("rename failed"));
    window.prompt = vi.fn().mockReturnValueOnce("再次改名");
    await state.renameSession(state.sessions.value[0]!);
    expect(state.errorMessage.value).toBe("rename failed");

    window.prompt = originalPrompt;
  });

  it("syncs provider selection, filter helpers, and utility labels from live state", async () => {
    fetchADKPageSessionDataMock.mockResolvedValueOnce({
      agents: [
        {
          id: "agent-1",
          name: "Agent One",
          instruction: "",
          providerId: "provider-default",
          model: "gpt-4o",
          tools: [],
          skills: [],
          permissionMode: "approval",
          memoryEnabled: true,
          recentUserWindow: 6,
          workMode: "chat",
          loopMaxIterations: 5,
          status: "ENABLED",
          createdAt: "2026-06-18T00:00:00Z",
          updatedAt: "2026-06-18T00:00:00Z",
        },
        {
          id: "agent-2",
          name: "Agent Two",
          instruction: "",
          providerId: "provider-other",
          model: "claude-sonnet",
          tools: [],
          skills: [],
          permissionMode: "all",
          memoryEnabled: true,
          recentUserWindow: 6,
          workMode: "loop",
          loopMaxIterations: 5,
          status: "ENABLED",
          createdAt: "2026-06-18T00:00:00Z",
          updatedAt: "2026-06-18T00:00:00Z",
        },
      ],
      providers: [
        buildProvider({
          id: "provider-other",
          displayName: "Claude",
          model: "claude-sonnet",
          default: false,
        }),
        buildProvider({
          id: "provider-default",
          displayName: "OpenAI",
          model: "gpt-4o",
          default: true,
        }),
      ],
      sessions: [
        buildSession({
          title: "新会话",
        }),
        buildSession({
          id: "session-blank",
          title: "Blank Session",
          agentId: "agent-2",
          createdAt: "2026-06-19T00:00:00Z",
          updatedAt: "2026-06-19T00:00:00Z",
        }),
        buildSession({
          id: "session-fallback-title",
          title: "   ",
          agentId: "agent-2",
          createdAt: "2026-06-20T00:00:00Z",
          updatedAt: "2026-06-20T00:00:00Z",
        }),
      ],
      approvals: [
        {
          id: "approval-pending",
          runId: "run-1",
          agentId: "agent-1",
          toolName: "tool.approve",
          status: "PENDING",
          reason: "needs confirmation",
          createdAt: "2026-06-18T00:00:00Z",
          updatedAt: "2026-06-18T00:00:00Z",
        },
        {
          id: "approval-approved",
          runId: "run-2",
          agentId: "agent-1",
          toolName: "tool.ignore",
          status: "APPROVED",
          reason: "done",
          createdAt: "2026-06-18T00:00:00Z",
          updatedAt: "2026-06-18T00:00:00Z",
        },
      ],
      tools: [
        {
          name: "tool.approve",
          displayName: "Approve Tool",
          description: "requires approval",
          category: "control",
          permission: "approval",
          allowedModes: ["approval"],
          requiresApprovalIn: ["approval"],
        },
      ],
    });

    const { state } = await mountSessionState();
    state.sessionSearch.value = "blank";
    state.sessionAgentFilter.value = "agent-2";

    expect(state.pendingApprovals.value.map((approval) => approval.id)).toEqual([
      "approval-pending",
    ]);
    expect(state.approvalTool(state.pendingApprovals.value[0]!)).toMatchObject({
      name: "tool.approve",
    });
    expect(state.visibleSessions.value.map((session) => session.id)).toEqual([
      "session-blank",
    ]);
    expect(state.selectedProvider.value?.id).toBe("provider-default");

    await state.handleProviderChange("provider-other");
    expect(state.selectedProviderId.value).toBe("provider-other");
    expect(state.selectedProvider.value?.id).toBe("provider-other");

    state.selectedProviderId.value = "missing-provider";
    state.syncSelectedProviderFromAgent();
    expect(state.selectedProviderId.value).toBe("provider-default");

    state.selectedAgentId.value = "agent-2";
    state.handleAgentChange();
    expect(state.selectedProviderId.value).toBe("provider-other");
    expect(state.agentName("agent-2")).toBe("Agent Two");
    expect(state.agentName("missing-agent")).toBe("missing-agent");
    expect(state.sessionTitle(state.sessions.value[0]!)).not.toBe("新会话");
    expect(state.sessionTitle(state.sessions.value[2]!)).not.toBe("   ");
  });

  it("groups visible sessions by workflow source and keeps default-only lists flat", async () => {
    fetchADKPageSessionDataMock.mockResolvedValueOnce(
      buildPageData([
        buildSession({
          id: "session-default",
          title: "普通对话",
          updatedAt: "2026-06-21T00:00:00Z",
        }),
        buildSession({
          id: "session-daily-new",
          title: "每日盘点新会话",
          workflowId: "workflow-daily",
          workflowName: "每日盘点",
          updatedAt: "2026-06-23T00:00:00Z",
        }),
        buildSession({
          id: "session-risk",
          title: "风险巡检",
          workflowId: "workflow-risk",
          workflowName: "风险巡检",
          updatedAt: "2026-06-24T00:00:00Z",
        }),
        buildSession({
          id: "session-daily-old",
          title: "每日盘点旧会话",
          workflowId: "workflow-daily",
          workflowName: "每日盘点",
          updatedAt: "2026-06-20T00:00:00Z",
        }),
        buildSession({
          id: "session-agent-2",
          agentId: "agent-2",
          title: "其他 Agent",
          workflowId: "workflow-other",
          workflowName: "其他工作流",
          updatedAt: "2026-06-25T00:00:00Z",
        }),
      ]),
    );

    const { state } = await mountSessionState();

    expect(state.showSessionGroups.value).toBe(true);
    expect(state.visibleSessionGroups.value.map((group) => group.title)).toEqual([
      "对话",
      "其他工作流",
      "风险巡检",
      "每日盘点",
    ]);
    expect(state.visibleSessionGroups.value[3]?.sessions.map((session) => session.id)).toEqual([
      "session-daily-new",
      "session-daily-old",
    ]);

    state.sessionSearch.value = "普通";
    expect(state.showSessionGroups.value).toBe(false);
    expect(state.visibleSessionGroups.value).toMatchObject([
      {
        title: "对话",
        isDefault: true,
        sessions: [{ id: "session-default" }],
      },
    ]);

    state.sessionSearch.value = "";
    state.sessionAgentFilter.value = "agent-2";
    expect(state.visibleSessionGroups.value.map((group) => group.title)).toEqual([
      "其他工作流",
    ]);
    expect(state.showSessionGroups.value).toBe(true);
  });

  it("opens provider settings and preserves invalid session-selection agent ids", async () => {
    const { state, router } = await mountSessionState();

    state.openProviderSettings();
    await router.isReady();
    await nextTick();
    expect(router.currentRoute.value.path).toBe("/settings/adk");
    expect(state.selectedAgentId.value).toBe("agent-1");

    await state.finishSessionSelection("missing-agent");
    expect(state.selectedAgentId.value).toBe("agent-1");
  });
});

async function mountSessionState() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: { template: "<div />" } },
      { path: "/settings/adk", component: { template: "<div />" } },
    ],
  });
  let state!: ReturnType<typeof useADKPageSessionState>;
  mount(
    defineComponent({
      setup() {
        state = useADKPageSessionState(router, ref<HTMLElement | null>(null));
        return () => h("div");
      },
    }),
  );
  await flushRequests();
  return { state, router };
}

function buildPageData(sessions: ADKSession[], providers: ADKProvider[] = []) {
  return {
    agents: [
      {
        id: "agent-1",
        name: "Agent",
        instruction: "",
        providerId: "provider-1",
        model: "model",
        tools: [],
        skills: [],
        permissionMode: "approval",
        memoryEnabled: true,
        recentUserWindow: 6,
        workMode: "chat",
        loopMaxIterations: 5,
        status: "ENABLED",
        createdAt: "2026-06-18T00:00:00Z",
        updatedAt: "2026-06-18T00:00:00Z",
      },
    ],
    providers,
    sessions,
    approvals: [],
    tools: [],
  };
}

function buildProvider(overrides: Partial<ADKProvider> = {}): ADKProvider {
  return {
    id: "provider-1",
    displayName: "Provider",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    requestTimeoutMs: 180_000,
    enabled: true,
    default: false,
    hasApiKey: true,
    createdAt: "2026-06-24T00:00:00Z",
    updatedAt: "2026-06-24T00:00:00Z",
    ...overrides,
  };
}

function buildSession(overrides: Partial<ADKSession> = {}): ADKSession {
  return {
    id: "session-existing",
    agentId: "agent-1",
    title: "已有会话",
    createdAt: "2026-06-18T00:00:00Z",
    updatedAt: "2026-06-18T00:00:00Z",
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}
