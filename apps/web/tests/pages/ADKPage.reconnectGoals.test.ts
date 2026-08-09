// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import type {
  ADKApproval,
  ADKChatResponse,
  ADKRun,
  ADKTimelineEntry,
} from "@/types";

import { flushRequests } from "../helpers";
import {
  buildApproval,
  buildComposerState,
  buildProvider,
  buildRun,
  buildSession,
  buildSessionContextSnapshot,
  buildTimelineEntry,
  buildToolCall,
  buildWorkflowStep,
  clickButtonByText,
  countApprovalActionCalls,
  deferred,
  expandQueue,
  findProviderSelect,
  findWorkModeSelect,
  lastComposerStatePatch,
  mountADKPage,
  pendingApprovalTimeline,
  registerADKPageTestLifecycle,
  sendPageMessage,
} from "./adkPageTestkit";

const {
  monitorADKRunContinuationMock,
  resumeADKChatStreamMock,
  streamADKChatMock,
} = vi.hoisted(() => ({
  monitorADKRunContinuationMock: vi.fn(),
  resumeADKChatStreamMock: vi.fn(),
  streamADKChatMock: vi.fn(),
}));

registerADKPageTestLifecycle({
  monitorADKRunContinuationMock,
  resumeADKChatStreamMock,
  streamADKChatMock,
});

vi.mock("mermaid", () => ({
  default: { initialize: vi.fn(), run: vi.fn() },
}));

vi.mock("@/composables/adk/adkChatStream", async () => {
  const actual = await vi.importActual<
    typeof import("@/composables/adk/adkChatStream")
  >("@/composables/adk/adkChatStream");
  return {
    ...actual,
    resumeADKChatStream: resumeADKChatStreamMock,
    streamADKChat: streamADKChatMock,
  };
});

vi.mock("@/composables/adk/adkRunContinuation", async () => {
  const actual = await vi.importActual<
    typeof import("@/composables/adk/adkRunContinuation")
  >("@/composables/adk/adkRunContinuation");
  return { ...actual, monitorADKRunContinuation: monitorADKRunContinuationMock };
});

describe("ADKPage reconnect and goal controls", () => {
  it("restores the selected session and reconnects its active stream after remount", async () => {
    const session = buildSession({ id: "session-resume" });
    const runningRun = buildRun({
      id: "run-resume",
      sessionId: session.id,
      status: "RUNNING",
    });
    const completedRun = buildRun({
      ...runningRun,
      status: "COMPLETED",
      message: "completed",
    });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {
          [session.id]: {
            streamId: "stream-resume",
            runId: runningRun.id,
            sequence: 7,
            activeChildRunId: "",
          },
        },
      }),
    );
    resumeADKChatStreamMock.mockImplementationOnce(async (_cursor, onEvent) => {
      const response: ADKChatResponse = {
        reply: "刷新后完成",
        session,
        run: completedRun,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "resume-answer",
            runId: completedRun.id,
            text: "刷新后完成",
          }),
        ],
      };
      await onEvent({
        type: "final",
        streamId: "stream-resume",
        sequence: 8,
        runId: completedRun.id,
        replay: true,
        response,
      });
      return response;
    });

    mountADKPage({
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [runningRun],
        composerState: buildComposerState(session.id, {
          chatDraft: "刷新保留草稿",
          workModeOverride: "loop",
          goalObjectiveDraft: "刷新保留目标",
          goalObjectiveTouched: true,
        }),
      },
    });
    await flushRequests();

    expect(resumeADKChatStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({
        streamId: "stream-resume",
        runId: runningRun.id,
        after: 7,
      }),
      expect.any(Function),
    );
    expect(
      document.querySelector<HTMLTextAreaElement>(".adk-composer-input")?.value,
    ).toBe("刷新保留草稿");
    expect(document.body.textContent).toContain("刷新后完成");
  });

  it("keeps a restored paused goal when reconnect replay delivers a stale completed final", async () => {
    const session = buildSession({ id: "session-resume-paused-goal" });
    const pausedRun = buildRun({
      id: "run-resume-paused-goal",
      sessionId: session.id,
      status: "PAUSED",
      workMode: "loop",
      objective: "刷新后继续暂停目标",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-06T00:00:10Z",
      pausedAt: "2026-06-06T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
      workflowPlan: [
        buildWorkflowStep("step-resume-paused-goal", "刷新后继续暂停目标", "TODO"),
      ],
    });
    const staleCompleted = buildRun({
      ...pausedRun,
      status: "COMPLETED",
      workflowStatus: "COMPLETED",
      pausedAt: undefined,
      pausedReason: undefined,
      resumeState: "",
      message: "goal completed",
      completedAt: "2026-06-06T00:00:30Z",
    });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {
          [session.id]: {
            streamId: "stream-resume-paused-goal",
            runId: pausedRun.id,
            sequence: 7,
            activeChildRunId: "",
          },
        },
      }),
    );
    resumeADKChatStreamMock.mockImplementationOnce(async (_cursor, onEvent) => {
      const response: ADKChatResponse = {
        reply: "刷新后完成",
        session,
        run: staleCompleted,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "resume-stale-answer",
            runId: staleCompleted.id,
            text: "刷新后完成",
          }),
        ],
      };
      await onEvent({
        type: "final",
        streamId: "stream-resume-paused-goal",
        sequence: 8,
        runId: staleCompleted.id,
        replay: true,
        response,
      });
      return response;
    });

    mountADKPage({
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "resume-paused-answer",
            runId: pausedRun.id,
            text: "目标已暂停。",
          }),
        ],
        runs: [pausedRun],
        composerState: buildComposerState(session.id),
      },
    });
    await flushRequests();

    expect(
      document.querySelector<HTMLButtonElement>(
        ".adk-goal-editor__action[title='运行目标']",
      ),
    ).not.toBeNull();
    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "已暂停",
    );
    expect(document.body.textContent).not.toContain("刷新后完成");
  });

  it("keeps a restored goal visible when editing the chat draft after refresh", async () => {
    const session = buildSession({ id: "session-restored-goal-draft" });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {},
      }),
    );
    mountADKPage({
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [],
        composerState: buildComposerState(session.id, {
          chatDraft: "",
          workModeOverride: "chat",
          goalObjectiveDraft: "刷新后已有目标",
          goalObjectiveTouched: true,
        }),
      },
    });
    await flushRequests();

    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "刷新后已有目标",
    );

    const textarea = document.querySelector<HTMLTextAreaElement>(
      ".adk-composer-input",
    )!;
    textarea.value = "重新输入的内容";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();

    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "刷新后已有目标",
    );
    expect(
      document.querySelector<HTMLTextAreaElement>(".adk-composer-input")?.value,
    ).toBe("重新输入的内容");
  });

  it("shows pause goal for an active goal and preserves the goal editor after clicking", async () => {
    const session = buildSession({ id: "session-goal-pause-button" });
    const runningRun = buildRun({
      id: "run-goal-pause-button",
      sessionId: session.id,
      status: "RUNNING",
      workMode: "loop",
      objective: "持续检查风险",
      workflowStatus: "RUNNING",
      workflowPlan: [
        buildWorkflowStep("step-goal-pause", "持续检查风险", "TODO"),
      ],
    });
    const pauseRequestedRun = buildRun({
      ...runningRun,
      pauseRequestedAt: "2026-06-06T00:00:10Z",
      resumeState: "user_pause_requested",
    });
    const pausedRun = buildRun({
      ...pauseRequestedRun,
      status: "PAUSED",
      workflowStatus: "PAUSED",
      pausedAt: "2026-06-06T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
    });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {},
      }),
    );
    const fetchMock = mountADKPage({
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [runningRun],
        composerState: buildComposerState(session.id),
      },
      pauseRunById: { [runningRun.id]: pauseRequestedRun },
    });
    await flushRequests();

    const pauseButton = document.querySelector<HTMLButtonElement>(
      ".adk-goal-editor__action[title='暂停目标']",
    );
    expect(pauseButton).not.toBeNull();
    monitorADKRunContinuationMock.mockReset();
    monitorADKRunContinuationMock.mockImplementationOnce(
      async (_run, options) => {
        await options?.onProgress?.(pausedRun, pauseRequestedRun);
        return pausedRun;
      },
    );
    pauseButton?.click();
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/adk/runs/${runningRun.id}/pause`,
      expect.objectContaining({ method: "POST" }),
    );
    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "持续检查风险",
    );
    expect(monitorADKRunContinuationMock).toHaveBeenCalledWith(
      expect.objectContaining({
        id: runningRun.id,
        pauseRequestedAt: "2026-06-06T00:00:10Z",
      }),
      expect.any(Object),
    );
    expect(
      document.querySelector<HTMLButtonElement>(
        ".adk-goal-editor__action[title='运行目标']",
      ),
    ).not.toBeNull();
    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "已暂停",
    );
  });

  it("resumes a user-paused goal without sending a chat message", async () => {
    const session = buildSession({ id: "session-goal-resume-button" });
    const pausedRun = buildRun({
      id: "run-goal-resume-button",
      sessionId: session.id,
      status: "PAUSED",
      workMode: "loop",
      objective: "恢复已有目标",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-06T00:00:10Z",
      pausedAt: "2026-06-06T00:00:12Z",
      resumeState: "user_paused",
      workflowPlan: [
        buildWorkflowStep("step-goal-resume", "恢复已有目标", "TODO"),
      ],
    });
    const runningRun = buildRun({
      ...pausedRun,
      status: "RUNNING",
      workflowStatus: "RUNNING",
      pauseRequestedAt: undefined,
      pausedAt: undefined,
      pausedReason: undefined,
      resumeState: "user_resuming",
    });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {},
      }),
    );
    const fetchMock = mountADKPage({
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [pausedRun],
        composerState: buildComposerState(session.id),
      },
      resumeRunById: { [pausedRun.id]: runningRun },
    });
    await flushRequests();

    const resumeButton = document.querySelector<HTMLButtonElement>(
      ".adk-goal-editor__action[title='运行目标']",
    );
    expect(resumeButton).not.toBeNull();
    resumeButton?.click();
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/adk/runs/${pausedRun.id}/resume`,
      expect.objectContaining({ method: "POST" }),
    );
    expect(streamADKChatMock).not.toHaveBeenCalled();
    expect(monitorADKRunContinuationMock).toHaveBeenCalledWith(
      expect.objectContaining({ id: pausedRun.id, status: "RUNNING" }),
      expect.any(Object),
    );
  });

  it("keeps the run goal command after a stale running snapshot arrives", async () => {
    const session = buildSession({ id: "session-goal-stale-running" });
    const runningRun = buildRun({
      id: "run-goal-stale-running",
      sessionId: session.id,
      status: "RUNNING",
      workMode: "loop",
      objective: "防止暂停态回退",
      workflowStatus: "RUNNING",
      workflowPlan: [
        buildWorkflowStep("step-goal-stale-running", "防止暂停态回退", "TODO"),
      ],
    });
    const pauseRequestedRun = buildRun({
      ...runningRun,
      pauseRequestedAt: "2026-06-06T00:00:10Z",
      resumeState: "user_pause_requested",
    });
    const pausedRun = buildRun({
      ...pauseRequestedRun,
      status: "PAUSED",
      workflowStatus: "PAUSED",
      pausedAt: "2026-06-06T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
    });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {},
      }),
    );
    mountADKPage({
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [runningRun],
        composerState: buildComposerState(session.id),
      },
      pauseRunById: { [runningRun.id]: pauseRequestedRun },
    });
    await flushRequests();

    monitorADKRunContinuationMock.mockReset();
    monitorADKRunContinuationMock.mockImplementationOnce(
      async (_run, options) => {
        await options?.onProgress?.(pausedRun, pauseRequestedRun);
        await options?.onProgress?.(pauseRequestedRun, pausedRun);
        return pauseRequestedRun;
      },
    );
    document
      .querySelector<HTMLButtonElement>(
        ".adk-goal-editor__action[title='暂停目标']",
      )
      ?.click();
    await flushRequests();

    expect(
      document.querySelector<HTMLButtonElement>(
        ".adk-goal-editor__action[title='运行目标']",
      ),
    ).not.toBeNull();
    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "已暂停",
    );
  });

  it("restores a paused goal after refresh with the run goal command visible", async () => {
    const session = buildSession({ id: "session-restored-paused-goal" });
    const pausedRun = buildRun({
      id: "run-restored-paused-goal",
      sessionId: session.id,
      status: "PAUSED",
      workMode: "loop",
      objective: "刷新后暂停目标",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-06T00:00:10Z",
      pausedAt: "2026-06-06T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
      workflowPlan: [
        buildWorkflowStep("step-paused-restored", "刷新后暂停目标", "TODO"),
      ],
    });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {},
      }),
    );
    mountADKPage({
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [pausedRun],
        composerState: buildComposerState(session.id),
      },
    });
    await flushRequests();

    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "刷新后暂停目标",
    );
    expect(
      document.querySelector<HTMLButtonElement>(
        ".adk-goal-editor__action[title='运行目标']",
      ),
    ).not.toBeNull();
  });

  it("keeps and persists the current draft when sending fails", async () => {
    const session = buildSession({ id: "session-send-fails" });
    streamADKChatMock.mockRejectedValueOnce(new Error("provider unavailable"));
    const fetchMock = mountADKPage({
      sessions: [session],
      sessionDetail: { session, timeline: [] },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();
    await sendPageMessage("失败后还在的草稿");

    expect(
      document.querySelector<HTMLTextAreaElement>(".adk-composer-input")?.value,
    ).toBe("失败后还在的草稿");
    expect(lastComposerStatePatch(fetchMock, session.id)).toMatchObject({
      chatDraft: "失败后还在的草稿",
    });
  });

  it("restores persisted timeline entries even when tool and approval arrays are null", async () => {
    mountADKPage({
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("user_message", {
            id: "msg-user-null",
            text: "你好",
            createdAt: "2026-06-06T00:00:01Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "entry-tools-null",
            runId: "run-null-history",
            toolCalls: null as unknown as ADKTimelineEntry["toolCalls"],
            createdAt: "2026-06-06T00:00:02Z",
          }),
          buildTimelineEntry("approval_group", {
            id: "entry-approvals-null",
            runId: "run-null-history",
            approvals: null as unknown as ADKTimelineEntry["approvals"],
            createdAt: "2026-06-06T00:00:03Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-answer-null",
            runId: "run-null-history",
            text: "历史记录已恢复。",
            createdAt: "2026-06-06T00:00:04Z",
          }),
        ],
      },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("历史记录已恢复。");
    expect(document.body.textContent).not.toContain(
      "Cannot read properties of null",
    );
  });
});
