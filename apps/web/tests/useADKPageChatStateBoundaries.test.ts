// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref, type Ref } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKChatResponse,
  ADKInputAnswer,
  ADKInputRequest,
  ADKProvider,
  ADKRun,
  ADKSession,
  ADKSessionContextSnapshot,
} from "../src/contracts";
import { compactADKSessionContext, fetchADKSessionContext } from "../src/composables/adkSessionContextApi";
import { streamADKChat } from "../src/composables/adkChatStream";
import { fetchEnvelopeWithInit } from "../src/composables/apiClient";
import { PROVISIONAL_SESSION_KEY } from "../src/composables/adkChatRuntime";
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
  fetchADKSessionContext: vi.fn(),
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
  vi.mocked(fetchADKSessionContext).mockResolvedValue(buildContext());
  vi.mocked(loadSessionChatHistory).mockResolvedValue({
    session: buildSession(),
    runs: [],
    timelineEntries: [],
    composerState: {
      sessionId: "session-1",
      chatDraft: "",
      providerIdOverride: "",
      modelOverride: "",
      workModeOverride: "",
      permissionModeOverride: "",
      goalObjectiveDraft: "",
      goalObjectiveTouched: false,
    },
  });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useADKPageChatState boundaries", () => {
  it("rejects context compaction without a selected session", async () => {
    const harness = mountHarness({ selectedSessionId: "" });

    await harness.state.runSlashCommand("compact");

    expect(harness.errorMessage.value).toBe("当前没有可压缩的会话");
    expect(compactADKSessionContext).not.toHaveBeenCalled();
    harness.unmount();
  });

  it("compacts context, refreshes the timeline, and opens details", async () => {
    const compacted = buildContext({
      contextRevisionId: "ctx-2",
      previousContextRevisionId: "ctx-1",
      compactedEventCount: 18,
      lastCompactionMode: "aggressive",
    });
    vi.mocked(compactADKSessionContext).mockResolvedValue(compacted);
    const harness = mountHarness();

    await harness.state.runSlashCommand("compact-aggressive");

    expect(compactADKSessionContext).toHaveBeenCalledWith("session-1", "aggressive");
    expect(harness.state.sessionContext.value?.contextRevisionId).toBe("ctx-2");
    expect(harness.state.contextDetailsOpen.value).toBe(true);
    expect(loadSessionChatHistory).toHaveBeenCalledWith("session-1");
    expect(harness.state.contextBusy.value).toBe(false);
    harness.state.contextDetailsOpen.value = false;
    await nextTick();
    harness.unmount();
  });

  it("keeps the explicit compaction error when timeline recovery also fails", async () => {
    vi.mocked(compactADKSessionContext).mockRejectedValueOnce("backend unavailable");
    vi.mocked(loadSessionChatHistory).mockRejectedValueOnce(new Error("timeline unavailable"));
    const harness = mountHarness();

    await harness.state.runSlashCommand("compact");

    expect(harness.errorMessage.value).toBe("上下文压缩失败");
    expect(harness.state.contextBusy.value).toBe(false);
    harness.unmount();
  });

  it("refreshes context on demand and clears it when the session disappears", async () => {
    const harness = mountHarness();

    await harness.state.initializeSessionContext("session-1");
    expect(harness.state.sessionContext.value?.sessionId).toBe("session-1");
    expect(harness.state.contextBusy.value).toBe(false);

    harness.state.openContextDetails();
    expect(harness.state.contextDetailsOpen.value).toBe(true);
    harness.state.clearSessionContext();
    expect(harness.state.sessionContext.value).toBeNull();
    expect(harness.state.contextDetailsOpen.value).toBe(false);
    harness.unmount();
  });

  it("sends on plain Enter but preserves newline and IME composition", async () => {
    vi.mocked(streamADKChat).mockResolvedValue(buildResponse(buildRun()));
    const harness = mountHarness();
    harness.state.chatDraft.value = "review account risk";
    const enter = new KeyboardEvent("keydown", { key: "Enter", cancelable: true });

    harness.state.handleComposerKeydown(enter);
    await flushAsync();

    expect(enter.defaultPrevented).toBe(true);
    expect(streamADKChat).toHaveBeenCalledOnce();

    harness.state.chatDraft.value = "line two";
    harness.state.handleComposerKeydown(
      new KeyboardEvent("keydown", { key: "Enter", shiftKey: true, cancelable: true }),
    );
    harness.state.handleComposerKeydown(
      new KeyboardEvent("keydown", { key: "Enter", isComposing: true, cancelable: true }),
    );
    await flushAsync();
    expect(streamADKChat).toHaveBeenCalledOnce();
    harness.unmount();
  });

  it("restores a failed send draft so the user can retry", async () => {
    vi.mocked(streamADKChat).mockRejectedValueOnce("transport closed");
    const harness = mountHarness();
    harness.state.chatDraft.value = "do not lose this request";

    await harness.state.sendChat();

    expect(harness.state.chatDraft.value).toBe("do not lose this request");
    expect(harness.errorMessage.value).toBe("Agents chat failed");
    expect(harness.state.sendingChat.value).toBe(false);
    harness.unmount();
  });

  it("updates an active goal objective with an encoded run identifier", async () => {
    const active = buildRun({
      id: "goal/run-1",
      status: "PAUSED",
      workMode: "loop",
      objective: "old objective",
      workflowStatus: "PAUSED",
    });
    vi.mocked(streamADKChat).mockResolvedValue(buildResponse(active));
    vi.mocked(fetchEnvelopeWithInit).mockResolvedValue({
      ...active,
      objective: "new objective",
    });
    const harness = mountHarness();
    harness.state.workModeOverride.value = "loop";
    harness.state.chatDraft.value = "start goal";
    await harness.state.sendChat();

    harness.state.updateGoalObjectiveDraft("  new objective  ");
    expect(harness.state.canSaveGoalObjective.value).toBe(true);
    await harness.state.updateGoalObjective();

    expect(fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/adk/runs/goal%2Frun-1/objective",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ objective: "new objective" }),
      }),
    );
    expect(harness.state.goalObjectiveDraft.value).toBe("new objective");
    expect(harness.state.goalObjectiveError.value).toBe("");
    expect(harness.state.canSaveGoalObjective.value).toBe(false);
    harness.unmount();
  });

  it("keeps an edited goal objective and reports failed persistence", async () => {
    const active = buildRun({
      status: "PAUSED",
      workMode: "loop",
      objective: "old objective",
      workflowStatus: "PAUSED",
    });
    vi.mocked(streamADKChat).mockResolvedValue(buildResponse(active));
    vi.mocked(fetchEnvelopeWithInit).mockRejectedValueOnce("write rejected");
    const harness = mountHarness();
    harness.state.workModeOverride.value = "loop";
    harness.state.chatDraft.value = "start goal";
    await harness.state.sendChat();
    harness.state.updateGoalObjectiveDraft("new objective");

    await harness.state.updateGoalObjective();

    expect(harness.state.goalObjectiveDraft.value).toBe("new objective");
    expect(harness.state.goalObjectiveError.value).toBe("目标保存失败");
    expect(harness.errorMessage.value).toBe("目标保存失败");
    expect(harness.state.goalObjectiveSaving.value).toBe(false);
    harness.unmount();
  });

  it("does not write an empty or unchanged objective", async () => {
    const harness = mountHarness();

    await harness.state.updateGoalObjective();
    harness.state.updateGoalObjectiveDraft("   ");
    await harness.state.updateGoalObjective();

    expect(fetchEnvelopeWithInit).not.toHaveBeenCalled();
    harness.unmount();
  });

  it("resets and flushes composer state using the selected provider override", async () => {
    vi.useFakeTimers();
    const harness = mountHarness();
    harness.state.chatDraft.value = "draft";
    harness.selectedProviderId.value = "provider-2";
    harness.selectedProvider.value = buildProvider({ id: "provider-2", model: "model-2" });
    await nextTick();

    await harness.state.flushComposerState({ keepalive: true });

    expect(saveADKSessionComposerState).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({
        chatDraft: "draft",
        providerIdOverride: "provider-2",
        modelOverride: "model-2",
      }),
      { keepalive: true },
    );

    harness.state.resetComposerState();
    expect(harness.state.chatDraft.value).toBe("");
    await vi.runAllTimersAsync();
    expect(saveADKSessionComposerState).toHaveBeenCalledTimes(2);
    harness.unmount();
  });

  it("retains dirty composer state after a failed save and retries later", async () => {
    vi.mocked(saveADKSessionComposerState)
      .mockRejectedValueOnce(new Error("storage unavailable"))
      .mockResolvedValueOnce(undefined);
    const harness = mountHarness();
    harness.state.chatDraft.value = "retry this draft";
    await nextTick();

    await harness.state.flushComposerState();
    await harness.state.flushComposerState();

    expect(saveADKSessionComposerState).toHaveBeenCalledTimes(2);
    expect(saveADKSessionComposerState).toHaveBeenLastCalledWith(
      "session-1",
      expect.objectContaining({ chatDraft: "retry this draft" }),
      {},
    );
    harness.unmount();
  });

  it("awaits an in-flight composer flush instead of saving the same revision twice", async () => {
    let resolveSave: (() => void) | null = null;
    vi.mocked(saveADKSessionComposerState).mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve;
        }),
    );
    const harness = mountHarness();
    harness.state.chatDraft.value = "flush once";
    await nextTick();

    const firstFlush = harness.state.flushComposerState();
    const secondFlush = harness.state.flushComposerState();
    await flushAsync();

    expect(saveADKSessionComposerState).toHaveBeenCalledTimes(1);

    resolveSave?.();
    await firstFlush;
    await secondFlush;

    expect(saveADKSessionComposerState).toHaveBeenCalledTimes(1);
    harness.unmount();
  });

  it("rebinds queued provisional messages after the first response creates a real session", async () => {
    let resolveFirstSend: ((response: ADKChatResponse) => void) | null = null;
    vi.mocked(streamADKChat)
      .mockImplementationOnce(
        async () =>
          new Promise<ADKChatResponse>((resolve) => {
            resolveFirstSend = resolve;
          }),
      )
      .mockImplementationOnce(async (payload) =>
        ({
          ...buildResponse(
            buildRun({
              id: "run-queued",
              sessionId: payload.sessionId ?? "session-queued",
              message: "queued follow-up sent",
            }),
            buildContext({ sessionId: payload.sessionId ?? "session-queued" }),
          ),
          session: {
            ...buildSession(),
            id: payload.sessionId ?? "session-queued",
          },
        }),
      );
    const harness = mountHarness({ selectedSessionId: "" });
    harness.state.chatDraft.value = "create session";

    const firstSend = harness.state.sendChat();
    await flushAsync();
    expect(harness.state.sendingChat.value).toBe(true);

    harness.state.chatDraft.value = "follow up after create";
    await harness.state.sendChat();

    expect(harness.state.queuedMessages.value).toHaveLength(1);
    expect(vi.mocked(streamADKChat)).toHaveBeenCalledTimes(1);

    resolveFirstSend?.({
      ...buildResponse(
        buildRun({
          id: "run-create",
          sessionId: "session-queued",
          message: "session created",
        }),
        buildContext({ sessionId: "session-queued" }),
      ),
      session: {
        ...buildSession(),
        id: "session-queued",
      },
    });
    await firstSend;
    await flushAsync();

    expect(harness.selectedSessionId.value).toBe("session-queued");
    expect(vi.mocked(streamADKChat)).toHaveBeenCalledTimes(2);
    expect(vi.mocked(streamADKChat).mock.calls[1]?.[0]).toMatchObject({
      sessionId: "session-queued",
      message: "follow up after create",
    });
    expect(harness.state.queuedMessages.value).toEqual([]);
    harness.unmount();
  });

  it("executes exact context slash commands without sending them to the model", async () => {
    const harness = mountHarness();
    harness.state.chatDraft.value = "  /CONTEXT  ";

    await harness.state.sendChat();

    expect(fetchADKSessionContext).toHaveBeenCalledWith("session-1");
    expect(harness.state.contextDetailsOpen.value).toBe(true);
    expect(harness.state.chatDraft.value).toBe("");
    expect(streamADKChat).not.toHaveBeenCalled();
    harness.unmount();
  });

  it("ignores invalid sends and lets interrupt-send fall back to a normal send", async () => {
    const harness = mountHarness();
    await harness.state.sendChat();
    await harness.state.interruptAndQueueChat();
    expect(streamADKChat).not.toHaveBeenCalled();

    vi.mocked(streamADKChat).mockResolvedValue(buildResponse(buildRun()));
    harness.state.chatDraft.value = "send immediately";
    await harness.state.interruptAndQueueChat();

    expect(streamADKChat).toHaveBeenCalledOnce();
    expect(vi.mocked(streamADKChat).mock.calls[0]?.[0].message).toBe("send immediately");
    harness.unmount();
  });

  it("restores an empty composer when a newly selected session has no history yet", async () => {
    vi.mocked(loadSessionChatHistory).mockRejectedValueOnce(new Error("not created yet"));
    const harness = mountHarness({ selectedSessionId: "session-old" });
    harness.state.chatDraft.value = "old session draft";

    await harness.state.selectSession("session-1");

    expect(harness.selectedSessionId.value).toBe("session-1");
    expect(harness.state.chatDraft.value).toBe("");
    expect(harness.state.timelineEntries.value).toEqual([]);
    expect(fetchADKSessionContext).toHaveBeenCalledWith("session-1");
    harness.unmount();
  });

  it("clears provisional composer state without scheduling a save", async () => {
    vi.useFakeTimers();
    const harness = mountHarness({ selectedSessionId: "" });
    harness.state.chatDraft.value = "draft before session exists";
    await nextTick();

    harness.state.resetComposerState("");
    await vi.runAllTimersAsync();

    expect(harness.state.chatDraft.value).toBe("");
    expect(saveADKSessionComposerState).not.toHaveBeenCalled();
    harness.unmount();
  });

  it("preserves the newest context revision across forward and stale responses", async () => {
    const contexts = [
      buildContext({
        contextRevisionId: "ctx-base",
        contextRevisionCreatedAt: "2026-01-01T10:00:00Z",
      }),
      buildContext({
        contextRevisionId: "ctx-forward",
        previousContextRevisionId: "ctx-base",
        contextRevisionCreatedAt: "2026-01-01T10:01:00Z",
      }),
      buildContext({
        contextRevisionId: "ctx-base",
        contextRevisionCreatedAt: "2026-01-01T10:00:00Z",
      }),
      buildContext({
        contextRevisionId: "ctx-stale",
        contextRevisionCreatedAt: "2026-01-01T09:59:00Z",
      }),
      buildContext({
        contextRevisionId: "ctx-latest",
        contextRevisionCreatedAt: "2026-01-01T10:02:00Z",
      }),
    ];
    vi.mocked(streamADKChat).mockImplementation(async () =>
      buildResponse(buildRun(), contexts.shift()),
    );
    const harness = mountHarness();

    for (let index = 0; index < 5; index += 1) {
      harness.state.chatDraft.value = `turn ${index}`;
      await harness.state.sendChat();
    }

    expect(harness.state.sessionContext.value?.contextRevisionId).toBe("ctx-latest");
    harness.unmount();
  });

  it("encodes cancellation requests and reports non-Error service failures", async () => {
    const harness = mountHarness();
    await harness.state.cancelActiveRun();
    expect(fetchEnvelopeWithInit).not.toHaveBeenCalled();

    vi.mocked(fetchEnvelopeWithInit).mockRejectedValueOnce("cancel unavailable");
    await harness.state.cancelActiveRun("run/with space");

    expect(fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/adk/runs/run%2Fwith%20space/cancel",
      { method: "POST" },
    );
    expect(harness.errorMessage.value).toBe("取消运行失败");
    harness.unmount();
  });

  it("skips stale timeline reloads when a cancellation result belongs to another session", async () => {
    vi.mocked(fetchEnvelopeWithInit).mockResolvedValueOnce(
      buildRun({
        id: "run-stale",
        sessionId: "session-2",
        status: "CANCELLED",
      }),
    );
    const harness = mountHarness();

    await harness.state.cancelActiveRun("run-stale");

    expect(loadSessionChatHistory).not.toHaveBeenCalled();
    harness.unmount();
  });

  it("reports pause and resume failures while releasing lifecycle locks", async () => {
    const runningGoal = buildRun({
      id: "goal-running",
      status: "RUNNING",
      workMode: "loop",
      objective: "monitor risk",
      workflowStatus: "RUNNING",
    });
    vi.mocked(streamADKChat).mockResolvedValueOnce(buildResponse(runningGoal));
    const runningHarness = mountHarness();
    runningHarness.state.workModeOverride.value = "loop";
    runningHarness.state.chatDraft.value = "start running goal";
    await runningHarness.state.sendChat();
    vi.mocked(fetchEnvelopeWithInit).mockRejectedValueOnce("pause unavailable");

    await runningHarness.state.pauseGoalRun();

    expect(runningHarness.errorMessage.value).toBe("暂停目标失败");
    expect(runningHarness.state.goalLifecycleBusy.value).toBe(false);
    runningHarness.unmount();

    const pausedGoal = buildRun({
      id: "goal-paused",
      status: "PAUSED",
      workMode: "loop",
      objective: "monitor risk",
      workflowStatus: "PAUSED",
    });
    vi.mocked(streamADKChat).mockResolvedValueOnce(buildResponse(pausedGoal));
    const pausedHarness = mountHarness();
    pausedHarness.state.workModeOverride.value = "loop";
    pausedHarness.state.chatDraft.value = "start paused goal";
    await pausedHarness.state.sendChat();
    vi.mocked(fetchEnvelopeWithInit).mockRejectedValueOnce(new Error("resume rejected"));

    await pausedHarness.state.resumeGoalRun();

    expect(pausedHarness.errorMessage.value).toBe("resume rejected");
    expect(pausedHarness.state.goalLifecycleBusy.value).toBe(false);
    pausedHarness.unmount();
  });

  it("removes persisted runtime state only for meaningful session identifiers", () => {
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: "session-1",
        sessions: {
          "session-1": {
            streamId: "stream-1",
            runId: "run-1",
            sequence: 4,
            activeChildRunId: "",
          },
        },
      }),
    );
    const harness = mountHarness();

    harness.state.removeSessionRuntimeState("   ");
    harness.state.removeSessionRuntimeState("session-1");

    const persisted = JSON.parse(
      String(window.localStorage.getItem("jftrade.adk.page-state.v1")),
    );
    expect(persisted.selectedSessionId).toBe("");
    expect(persisted.sessions).toEqual({});
    harness.unmount();
  });

  it("clears goal draft state when leaving loop mode without an active goal", async () => {
    const harness = mountHarness();
    harness.state.workModeOverride.value = "loop";
    harness.state.chatDraft.value = "draft objective";
    await nextTick();
    harness.state.updateGoalObjectiveDraft("edited objective");
    harness.state.workModeOverride.value = "chat";
    await nextTick();

    expect(harness.state.goalObjectiveDraft.value).toBe("");
    expect(harness.state.goalObjectiveError.value).toBe("");
    expect(harness.state.showGoalObjectiveEditor.value).toBe(false);
    harness.unmount();
  });

  it("includes permission overrides in the streamed request", async () => {
    vi.mocked(streamADKChat).mockResolvedValue(buildResponse(buildRun()));
    const harness = mountHarness();
    harness.state.permissionModeOverride.value = "less_approval";
    harness.state.chatDraft.value = "read portfolio only";

    await harness.state.sendChat();

    expect(vi.mocked(streamADKChat).mock.calls[0]?.[0]).toMatchObject({
      permissionModeOverride: "less_approval",
    });
    harness.unmount();
  });

  it("does not schedule a context refresh for empty session ids in run events", async () => {
    vi.mocked(streamADKChat).mockImplementationOnce(async (_payload, onEvent) => {
      await onEvent({
        type: "run",
        run: buildRun({
          id: "run-empty-session",
          sessionId: "",
          status: "RUNNING",
        }),
      });
      return {
        ...buildResponse(
          buildRun({
            id: "run-empty-session",
            sessionId: "",
            status: "COMPLETED",
          }),
          undefined,
        ),
        session: {
          ...buildSession(),
          id: "",
        },
      };
    });
    const harness = mountHarness({ selectedSessionId: "" });
    harness.state.chatDraft.value = "start without a persisted session";

    await harness.state.sendChat();

    expect(fetchADKSessionContext).not.toHaveBeenCalled();
    harness.unmount();
  });

  it("supports empty approval batches and lifecycle calls without an active goal", async () => {
    const harness = mountHarness();

    await harness.state.denyAllApprovals([]);
    await harness.state.resolveAllApprovals([]);
    await harness.state.pauseGoalRun();
    await harness.state.resumeGoalRun();

    expect(fetchEnvelopeWithInit).not.toHaveBeenCalled();
    harness.unmount();
  });

  it("denies a pending approval and refreshes the authoritative terminal run", async () => {
    const approval = buildApproval();
    vi.mocked(fetchEnvelopeWithInit).mockResolvedValueOnce({
      approval: { ...approval, status: "DENIED" },
      run: buildRun({ id: approval.runId, status: "DENIED" }),
    });
    const harness = mountHarness();

    await harness.state.denyApproval(approval);

    expect(fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/adk/approvals/approval%2F1/deny",
      { method: "POST" },
    );
    expect(harness.state.approvalsBusy.value).toBe(false);
    harness.unmount();
  });

  it("clears context when explicitly initialized with no session", async () => {
    const harness = mountHarness();
    await harness.state.initializeSessionContext("session-1");
    expect(harness.state.sessionContext.value).not.toBeNull();

    await harness.state.initializeSessionContext("");

    expect(harness.state.sessionContext.value).toBeNull();
    harness.state.contextDetailsOpen.value = false;
    await nextTick();
    harness.unmount();
  });

  it("monitors a non-terminal cancellation and clears an interrupt lock", async () => {
    vi.mocked(fetchEnvelopeWithInit).mockResolvedValueOnce(
      buildRun({ id: "run-cancel", status: "RUNNING" }),
    );
    const harness = mountHarness();
    harness.state.interruptingRunId.value = "run-cancel";

    await harness.state.cancelActiveRun("run-cancel");
    await flushAsync();

    expect(loadSessionChatHistory).toHaveBeenCalledWith("session-1");
    expect(harness.state.interruptingRunId.value).toBe("");
    harness.unmount();
  });

  it("aborts the active stream as a successful send when a running goal is paused", async () => {
    const runningGoal = buildRun({
      id: "goal-pause-stream",
      status: "RUNNING",
      workMode: "loop",
      objective: "monitor risk continuously",
      workflowStatus: "RUNNING",
    });
    vi.mocked(streamADKChat).mockImplementationOnce(
      async (_payload, onEvent, options) => {
        await onEvent({ type: "run", run: runningGoal });
        return new Promise<ADKChatResponse>((_resolve, reject) => {
          options?.signal?.addEventListener("abort", () => {
            reject(new DOMException("aborted", "AbortError"));
          });
        });
      },
    );
    vi.mocked(fetchEnvelopeWithInit).mockResolvedValueOnce({
      ...runningGoal,
      status: "PAUSED",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-01-01T00:00:02Z",
      pausedAt: "2026-01-01T00:00:03Z",
    });
    const harness = mountHarness();
    harness.state.workModeOverride.value = "loop";
    harness.state.chatDraft.value = "start long goal";
    const sendPromise = harness.state.sendChat();
    await flushAsync();

    await harness.state.pauseGoalRun();
    await sendPromise;

    expect(harness.state.goalPaused.value).toBe(true);
    expect(harness.state.sendingChat.value).toBe(false);
    expect(harness.errorMessage.value).toBe("");
    harness.unmount();
  });

  it("keeps provisional queue keys unchanged when a response returns the reserved provisional session id", async () => {
    let resolveFirstSend: ((response: ADKChatResponse) => void) | null = null;
    vi.mocked(streamADKChat)
      .mockImplementationOnce(
        async () =>
          new Promise<ADKChatResponse>((resolve) => {
            resolveFirstSend = resolve;
          }),
      )
      .mockImplementationOnce(async (payload) => ({
        ...buildResponse(
          buildRun({
            id: "run-provisional-followup",
            sessionId: payload.sessionId ?? PROVISIONAL_SESSION_KEY,
            message: "queued follow-up sent",
          }),
          buildContext({ sessionId: payload.sessionId ?? PROVISIONAL_SESSION_KEY }),
        ),
        session: {
          ...buildSession(),
          id: payload.sessionId ?? PROVISIONAL_SESSION_KEY,
        },
      }));
    const harness = mountHarness({ selectedSessionId: "" });
    harness.state.chatDraft.value = "create placeholder session";

    const firstSend = harness.state.sendChat();
    await flushAsync();

    harness.state.chatDraft.value = "follow-up on placeholder session";
    await harness.state.sendChat();
    expect(harness.state.queuedMessages.value).toHaveLength(1);

    resolveFirstSend?.({
      ...buildResponse(
        buildRun({
          id: "run-provisional-root",
          sessionId: PROVISIONAL_SESSION_KEY,
          message: "placeholder accepted",
        }),
        buildContext({ sessionId: PROVISIONAL_SESSION_KEY }),
      ),
      session: {
        ...buildSession(),
        id: PROVISIONAL_SESSION_KEY,
      },
    });
    await firstSend;
    await flushAsync();

    expect(harness.selectedSessionId.value).toBe(PROVISIONAL_SESSION_KEY);
    expect(vi.mocked(streamADKChat)).toHaveBeenCalledTimes(2);
    expect(vi.mocked(streamADKChat).mock.calls[1]?.[0]).toMatchObject({
      sessionId: PROVISIONAL_SESSION_KEY,
      message: "follow-up on placeholder session",
    });
    harness.unmount();
  });

  it("submits input answers once, refreshes authoritative state and clears busy state", async () => {
    const run = buildRun({ id: "run-input", status: "RUNNING" });
    vi.mocked(fetchEnvelopeWithInit).mockResolvedValueOnce({ run });
    const harness = mountHarness();
    const request: ADKInputRequest = {
      id: "request/1",
      runId: run.id,
      agentId: "agent-1",
      functionCallId: "call-1",
      status: "PENDING",
      questions: [],
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    const answers: ADKInputAnswer[] = [{ questionId: "risk", optionId: "low" }];

    const submitting = harness.state.submitInputResponse(request, answers);
    expect(harness.state.inputRequestBusy(request.id)).toBe(true);
    await harness.state.submitInputResponse(request, answers);
    await submitting;
    await flushAsync();

    expect(fetchEnvelopeWithInit).toHaveBeenCalledTimes(1);
    expect(fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/adk/runs/run-input/input-response",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ requestId: request.id, answers }),
      }),
    );
    expect(harness.state.inputRequestBusy(request.id)).toBe(false);
    expect(loadSessionChatHistory).toHaveBeenCalledWith("session-1");
    harness.unmount();
  });

  it("surfaces non-Error input submission failures and always releases the request lock", async () => {
    vi.mocked(fetchEnvelopeWithInit).mockRejectedValueOnce("offline");
    const harness = mountHarness();
    const request: ADKInputRequest = {
      id: "request-2",
      runId: "run-2",
      agentId: "agent-1",
      functionCallId: "call-2",
      status: "PENDING",
      questions: [],
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };

    await harness.state.submitInputResponse(request, []);

    expect(harness.errorMessage.value).toBe("提交回答失败");
    expect(harness.state.inputRequestBusy(request.id)).toBe(false);
    harness.unmount();
  });
});

function mountHarness(options: { selectedSessionId?: string } = {}) {
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
        initialized: ref(false),
        refreshAll: vi.fn(async () => {}),
        finishSessionSelection: vi.fn(async () => {}),
        selectedProvider,
        selectedAgentId: ref("agent-1"),
        selectedProviderId,
        selectedSessionId,
        sessions: ref([buildSession()]),
      }, ref(""));
      return () => h("div");
    },
  });
  const wrapper = mount(component);
  return {
    state,
    errorMessage,
    selectedProvider,
    selectedProviderId,
    selectedSessionId,
    unmount: () => wrapper.unmount(),
  };
}

function buildSession(): ADKSession {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "Risk review",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
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

function buildProvider(overrides: Partial<ADKProvider> = {}): ADKProvider {
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

function buildApproval(): ADKApproval {
  return {
    id: "approval/1",
    runId: "run-approval",
    agentId: "agent-1",
    toolName: "strategy.save_definition",
    status: "PENDING",
    reason: "Saving a strategy changes persisted state",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

function buildResponse(
  run: ADKRun,
  context: ADKSessionContextSnapshot | undefined = buildContext(),
): ADKChatResponse {
  return {
    reply: run.message,
    session: buildSession(),
    run,
    pendingApprovals: [],
    timeline: [],
    context,
  };
}

function buildContext(
  overrides: Partial<ADKSessionContextSnapshot> = {},
): ADKSessionContextSnapshot {
  return {
    sessionId: "session-1",
    contextRevisionId: "ctx-1",
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

async function flushAsync(): Promise<void> {
  await Promise.resolve();
  await nextTick();
  await new Promise((resolve) => setTimeout(resolve, 0));
}
