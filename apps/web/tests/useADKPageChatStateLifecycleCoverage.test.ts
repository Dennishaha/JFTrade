// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref, type Ref } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKChatResponse,
  ADKInputRequest,
  ADKProvider,
  ADKRun,
  ADKSession,
  ADKSessionComposerState,
  ADKSessionContextSnapshot,
  ADKTimelineEntry,
} from "../src/contracts";
import {
  resumeADKChatStream,
  streamADKChat,
} from "../src/composables/adkChatStream";
import { fetchEnvelopeWithInit } from "../src/composables/apiClient";
import { loadSessionChatHistory } from "../src/composables/adkPageRunHistory";
import { saveADKSessionComposerState } from "../src/composables/adkPageSessionApi";
import { useADKPageChatState } from "../src/composables/useADKPageChatState";

vi.mock("../src/composables/adkChatStream", async () => {
  const actual = await vi.importActual<typeof import("../src/composables/adkChatStream")>(
    "../src/composables/adkChatStream",
  );
  return { ...actual, resumeADKChatStream: vi.fn(), streamADKChat: vi.fn() };
});

vi.mock("../src/composables/adkSessionContextApi", () => ({
  compactADKSessionContext: vi.fn(),
  fetchADKSessionContext: vi.fn(async (sessionId: string) => buildContext({ sessionId })),
}));

vi.mock("../src/composables/apiClient", async () => {
  const actual = await vi.importActual<typeof import("../src/composables/apiClient")>(
    "../src/composables/apiClient",
  );
  return { ...actual, fetchEnvelopeWithInit: vi.fn() };
});

vi.mock("../src/composables/adkPageRunHistory", async () => {
  const actual = await vi.importActual<typeof import("../src/composables/adkPageRunHistory")>(
    "../src/composables/adkPageRunHistory",
  );
  return { ...actual, loadSessionChatHistory: vi.fn() };
});

vi.mock("../src/composables/adkPageSessionApi", async () => {
  const actual = await vi.importActual<typeof import("../src/composables/adkPageSessionApi")>(
    "../src/composables/adkPageSessionApi",
  );
  return { ...actual, saveADKSessionComposerState: vi.fn() };
});

vi.mock("../src/composables/adkRunContinuation", () => ({
  monitorADKRunContinuation: vi.fn(async (run: ADKRun) => ({
    ...run,
    status: "COMPLETED",
  })),
}));

beforeEach(() => {
  window.localStorage.clear();
  vi.clearAllMocks();
  vi.mocked(saveADKSessionComposerState).mockResolvedValue(undefined);
  vi.mocked(loadSessionChatHistory).mockImplementation(async (sessionId) =>
    buildHistory(sessionId),
  );
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useADKPageChatState workflow recovery coverage", () => {
  it("keeps a selected session isolated from stale timeline events from another session", async () => {
    const staleEntry = buildTimeline({ id: "foreign-entry", sessionId: "session-foreign" });
    vi.mocked(streamADKChat).mockImplementationOnce(async (_payload, onEvent) => {
      await onEvent({ type: "timeline", timeline: staleEntry });
      return buildResponse(buildRun());
    });
    const harness = mountHarness();
    harness.state.chatDraft.value = "review the current account";

    await harness.state.sendChat();

    expect(harness.state.timelineEntries.value.some((entry) => entry.id === staleEntry.id)).toBe(false);
    expect(harness.selectedSessionId.value).toBe("session-1");
    harness.unmount();
  });

  it("clears blocking run state before switching to a different session", async () => {
    const running = buildRun({ id: "running-session-1", status: "RUNNING" });
    vi.mocked(streamADKChat).mockResolvedValueOnce(buildResponse(running));
    const harness = mountHarness({ sessions: [buildSession(), buildSession({ id: "session-2" })] });
    harness.state.chatDraft.value = "start a long review";

    await harness.state.sendChat();
    expect(harness.state.activeRunId.value).toBe(running.id);

    await harness.state.selectSession("session-2");

    expect(harness.selectedSessionId.value).toBe("session-2");
    expect(harness.state.activeRunId.value).toBe("");
    expect(harness.state.hasBlockingRun.value).toBe(false);
    harness.unmount();
  });

  it("reconnects an in-progress persisted run after a page reload and falls back to polling", async () => {
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: "session-1",
        sessions: {
          "session-1": {
            streamId: "stream-reload",
            runId: "run-reload",
            sequence: 12,
            activeChildRunId: "",
          },
        },
      }),
    );
    vi.mocked(loadSessionChatHistory).mockResolvedValueOnce(
      buildHistory("session-1", [buildRun({ id: "run-reload", status: "RUNNING" })]),
    );
    vi.mocked(resumeADKChatStream).mockResolvedValueOnce(null);
    const harness = mountHarness({
      initialized: true,
      selectedSessionId: "session-2",
      sessions: [buildSession(), buildSession({ id: "session-2" })],
    });

    await flushAsync();

    expect(harness.selectedSessionId.value).toBe("session-1");
    expect(resumeADKChatStream).toHaveBeenCalledWith(
      expect.objectContaining({ streamId: "stream-reload", runId: "run-reload", after: 12 }),
      expect.any(Function),
    );
    harness.unmount();
  });

  it("reports a reconnect failure when persistence has no recoverable run", async () => {
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: "session-1",
        sessions: {
          "session-1": {
            streamId: "stream-orphaned",
            runId: "",
            sequence: 4,
            activeChildRunId: "",
          },
        },
      }),
    );
    vi.mocked(loadSessionChatHistory).mockResolvedValueOnce(buildHistory("session-1"));
    vi.mocked(resumeADKChatStream).mockRejectedValueOnce("network unavailable");
    const harness = mountHarness({
      initialized: true,
      selectedSessionId: "session-2",
      sessions: [buildSession(), buildSession({ id: "session-2" })],
    });

    await flushAsync();

    expect(harness.errorMessage.value).toBe("恢复推理流失败");
    harness.unmount();
  });

  it("keeps the parent run authoritative when an input response resumes a workflow child", async () => {
    const child = buildRun({ id: "child-run", parentRunId: "parent-run", status: "RUNNING" });
    const parent = buildRun({ id: "parent-run", status: "RUNNING" });
    vi.mocked(fetchEnvelopeWithInit).mockResolvedValueOnce({
      request: buildInputRequest(),
      run: child,
      parentRun: parent,
    });
    const harness = mountHarness();

    await harness.state.submitInputResponse(buildInputRequest(), []);

    expect(harness.state.activeRunId.value).toBe(parent.id);
    expect(fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/adk/runs/run-input/input-response",
      expect.objectContaining({ method: "POST" }),
    );
    harness.unmount();
  });

  it("retains successful approval results while reporting a partial batch failure", async () => {
    const first = buildApproval({ id: "approval-1" });
    const second = buildApproval({ id: "approval-2" });
    const third = buildApproval({ id: "approval-3" });
    vi.mocked(fetchEnvelopeWithInit)
      .mockResolvedValueOnce({
        approval: { ...first, status: "APPROVED" },
        run: buildRun({ id: first.runId, status: "COMPLETED" }),
      })
      .mockRejectedValueOnce("approval service unavailable")
      .mockRejectedValueOnce(new Error("second approval failure"));
    const harness = mountHarness();

    await harness.state.resolveAllApprovals([first, second, third]);

    expect(harness.errorMessage.value).toBe("批量审批部分失败：审批处理失败");
    expect(harness.state.approvalsBusy.value).toBe(false);
    expect(fetchEnvelopeWithInit).toHaveBeenCalledTimes(3);
    harness.unmount();
  });

  it("keeps a live goal objective through mode changes and clears it on session handoff", async () => {
    const goalRun = buildRun({
      id: "goal-session-1",
      status: "RUNNING",
      workMode: "loop",
      objective: "review exposure before opening",
    });
    vi.mocked(streamADKChat).mockResolvedValueOnce(buildResponse(goalRun));
    const harness = mountHarness({
      sessions: [buildSession(), buildSession({ id: "session-2" })],
    });
    harness.state.workModeOverride.value = "loop";
    harness.state.chatDraft.value = "review exposure before opening";

    await harness.state.sendChat();
    expect(harness.state.showGoalObjectiveEditor.value).toBe(true);
    expect(harness.state.goalObjectiveDraft.value).toBe(
      "review exposure before opening",
    );

    // The composer may switch back to chat for a side question, but that must
    // not erase the active goal's persisted objective.
    harness.state.workModeOverride.value = "chat";
    await nextTick();
    expect(harness.state.goalObjectiveDraft.value).toBe(
      "review exposure before opening",
    );

    await harness.state.selectSession("session-2");
    expect(harness.state.showGoalObjectiveEditor.value).toBe(false);
    harness.unmount();
  });

  it("does not persist a composer draft before any session is selected", async () => {
    const harness = mountHarness({ selectedSessionId: "", sessions: [] });
    harness.state.chatDraft.value = "draft without a session";
    await nextTick();

    await harness.state.flushComposerState();

    expect(saveADKSessionComposerState).not.toHaveBeenCalled();
    harness.unmount();
  });

  it("drops a persisted selection that no longer belongs to the restored session list", async () => {
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: "removed-session",
        sessions: { "removed-session": { streamId: "", runId: "", sequence: 0, activeChildRunId: "" } },
      }),
    );
    const harness = mountHarness({ initialized: true, sessions: [buildSession()] });

    await flushAsync();

    expect(JSON.parse(window.localStorage.getItem("jftrade.adk.page-state.v1") ?? "{}")).toMatchObject({
      selectedSessionId: "",
    });
    expect(harness.selectedSessionId.value).toBe("session-1");
    harness.unmount();
  });

  it("does not attach a child-run selection to an empty session", async () => {
    const harness = mountHarness({ selectedSessionId: "", sessions: [] });

    harness.state.setActiveChildRunId("child-before-session");
    await nextTick();

    const persisted = JSON.parse(window.localStorage.getItem("jftrade.adk.page-state.v1") ?? "{}");
    expect(harness.selectedSessionId.value).toBe("");
    expect(persisted.sessions?.[""]).toBeUndefined();
    harness.unmount();
  });

  it("stops a queued composer flush when its session disappears during the save", async () => {
    let resolveSave: (() => void) | null = null;
    vi.mocked(saveADKSessionComposerState).mockImplementationOnce(
      () => new Promise<void>((resolve) => { resolveSave = resolve; }),
    );
    const harness = mountHarness({
      sessions: [buildSession(), buildSession({ id: "session-2" })],
    });
    harness.state.chatDraft.value = "persist only while this session still exists";
    await nextTick();

    const flush = harness.state.flushComposerState();
    await vi.waitFor(() => expect(saveADKSessionComposerState).toHaveBeenCalledOnce());
    harness.selectedSessionId.value = "";
    resolveSave?.();
    await flush;

    expect(saveADKSessionComposerState).toHaveBeenCalledTimes(1);
    harness.unmount();
  });
});

function mountHarness(options: {
  initialized?: boolean;
  selectedSessionId?: string;
  sessions?: ADKSession[];
} = {}) {
  const agents = ref([buildAgent()]);
  const errorMessage = ref("");
  const selectedProvider = ref<ADKProvider | null>(buildProvider());
  const selectedProviderId = ref("provider-1");
  const selectedSessionId = ref(options.selectedSessionId ?? "session-1");
  let state!: ReturnType<typeof useADKPageChatState>;
  const component = defineComponent({
    setup() {
      state = useADKPageChatState(ref(null), {
        agents,
        errorMessage,
        initialized: ref(options.initialized ?? false),
        refreshAll: vi.fn(async () => {}),
        finishSessionSelection: vi.fn(async () => {}),
        selectedProvider,
        selectedAgentId: ref("agent-1"),
        selectedProviderId,
        selectedSessionId,
        sessions: ref(options.sessions ?? [buildSession()]),
      }, ref(""));
      return () => h("div");
    },
  });
  const wrapper = mount(component);
  return {
    state,
    errorMessage,
    selectedSessionId,
    unmount: () => wrapper.unmount(),
  };
}

function buildAgent(): ADKAgent {
  return {
    id: "agent-1",
    name: "Risk Agent",
    instruction: "Review account risk",
    providerId: "provider-1",
    model: "model-1",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat",
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

function buildProvider(): ADKProvider {
  return {
    id: "provider-1",
    displayName: "Provider",
    baseUrl: "https://llm.example/v1",
    model: "model-1",
    requestTimeoutMs: 180_000,
    enabled: true,
    default: true,
    hasApiKey: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

function buildSession(overrides: Partial<ADKSession> = {}): ADKSession {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "Risk review",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function buildRun(overrides: Partial<ADKRun> = {}): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "done",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:01Z",
    ...overrides,
  };
}

function buildApproval(overrides: Partial<ADKApproval> = {}): ADKApproval {
  return {
    id: "approval-1",
    runId: "run-approval",
    agentId: "agent-1",
    toolName: "strategy.save_definition",
    status: "PENDING",
    reason: "Saving a strategy changes persisted state",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function buildInputRequest(): ADKInputRequest {
  return {
    id: "request-1",
    runId: "run-input",
    agentId: "agent-1",
    functionCallId: "call-1",
    status: "PENDING",
    questions: [],
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

function buildTimeline(overrides: Partial<ADKTimelineEntry> = {}): ADKTimelineEntry {
  return {
    id: "timeline-1",
    sessionId: "session-1",
    kind: "assistant_message",
    createdAt: "2026-01-01T00:00:00Z",
    sequence: 1,
    status: "final",
    text: "done",
    ...overrides,
  };
}

function buildResponse(run: ADKRun): ADKChatResponse {
  return {
    reply: run.message,
    session: buildSession({ id: run.sessionId }),
    run,
    pendingApprovals: [],
    timeline: [],
    context: buildContext({ sessionId: run.sessionId }),
  };
}

function buildContext(
  overrides: Partial<ADKSessionContextSnapshot> = {},
): ADKSessionContextSnapshot {
  return {
    sessionId: "session-1",
    currentInputTokens: 1_000,
    projectedNextTurnTokens: 1_200,
    contextWindowTokens: 10_000,
    usageRatio: 0.12,
    status: "healthy",
    recentUserWindow: 6,
    retainedRecentUserCount: 2,
    activeHandoffCount: 0,
    breakdown: {
      instructionTokens: 100,
      handoffTokens: 0,
      recentUserTokens: 500,
      protectedTailTokens: 100,
      otherVisibleTokens: 200,
      pendingUserTokens: 100,
      toolDeclarationTokens: 200,
    },
    autoCompacted: false,
    degradedSummary: false,
    ...overrides,
  };
}

function buildComposerState(sessionId: string): ADKSessionComposerState {
  return {
    sessionId,
    chatDraft: "",
    providerIdOverride: "",
    modelOverride: "",
    workModeOverride: "",
    permissionModeOverride: "",
    goalObjectiveDraft: "",
    goalObjectiveTouched: false,
  };
}

function buildHistory(sessionId: string, runs: ADKRun[] = []) {
  return {
    session: buildSession({ id: sessionId }),
    runs,
    timelineEntries: [],
    composerState: buildComposerState(sessionId),
  };
}

async function flushAsync(): Promise<void> {
  await Promise.resolve();
  await nextTick();
  await new Promise((resolve) => setTimeout(resolve, 0));
  await nextTick();
}
