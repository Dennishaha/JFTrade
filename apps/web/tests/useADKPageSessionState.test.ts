// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick, ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ADKProvider, ADKSession } from "@/contracts";

import { flushRequests } from "./helpers";

const {
  createADKPageSessionMock,
  fetchADKPageSessionDataMock,
} = vi.hoisted(() => ({
  createADKPageSessionMock: vi.fn(),
  fetchADKPageSessionDataMock: vi.fn(),
}));

vi.mock("../src/composables/adkPageSessionApi", async () => {
  const actual = await vi.importActual<
    typeof import("../src/composables/adkPageSessionApi")
  >("../src/composables/adkPageSessionApi");
  return {
    ...actual,
    createADKPageSession: createADKPageSessionMock,
    fetchADKPageSessionData: fetchADKPageSessionDataMock,
  };
});

import { useADKPageSessionState } from "../src/composables/useADKPageSessionState";

beforeEach(() => {
  createADKPageSessionMock.mockReset();
  fetchADKPageSessionDataMock.mockReset();
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
    const state = await mountSessionState();
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
    const state = await mountSessionState();
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
    const state = await mountSessionState();
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
    const state = await mountSessionState();
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
    const state = await mountSessionState();

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

    const state = await mountSessionState();

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
});

async function mountSessionState() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { template: "<div />" } }],
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
  return state;
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
