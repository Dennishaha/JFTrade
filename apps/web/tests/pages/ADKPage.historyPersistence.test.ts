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

describe("ADKPage history and composer persistence", () => {
  it("restores persisted timeline entries for saved sessions", async () => {
    const savedRun = buildRun({
      id: "run-restored",
      status: "COMPLETED",
      toolCalls: [
        buildToolCall(
          "tool-restored",
          "run-restored",
          "portfolio.summary",
          "SUCCEEDED",
        ),
      ],
    });
    mountADKPage({
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("user_message", {
            id: "msg-user",
            text: "查看系统状态",
            createdAt: "2026-06-06T00:00:01Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-pre",
            runId: savedRun.id,
            text: "先查组合，再整理系统状态。",
            createdAt: "2026-06-06T00:00:02Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "entry-tools",
            runId: savedRun.id,
            toolCalls: savedRun.toolCalls,
            createdAt: "2026-06-06T00:00:03Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-final",
            runId: savedRun.id,
            text: "最终结论已经整理完成。",
            createdAt: "2026-06-06T00:00:04Z",
          }),
        ],
      },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("先查组合，再整理系统状态。");
    expect(document.body.textContent).toContain("最终结论已经整理完成。");
    expect(document.body.textContent).toContain("portfolio.summary");
  });

  it("restores processed goal prompts as observable details without replacing the user prompt", async () => {
    mountADKPage({
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("user_message", {
            id: "msg-user-goal",
            runId: "run-goal",
            text: "编写个适合nvda的策略",
            originalText: "编写个适合nvda的策略",
            processedText:
              "请推进这个目标。你可以使用 workflow.task.* 工具维护 TODO DAG，并在本轮完成可见回复后等待系统追问再裁决目标是否完成。\n总体目标：编写个适合nvda的策略\n用户请求：编写个适合nvda的策略",
            createdAt: "2026-06-18T00:00:01Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-goal-answer",
            runId: "run-goal",
            text: "策略草案已生成。",
            createdAt: "2026-06-18T00:00:02Z",
          }),
        ],
      },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("编写个适合nvda的策略");
    expect(document.body.textContent).toContain("策略草案已生成。");
    expect(document.body.textContent).toContain("可观测");
    expect(document.body.textContent).not.toContain("请推进这个目标");
  });

  it("renders chat alerts inside the chat thread", async () => {
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      await onEvent({ type: "session", session: buildSession() });
      await onEvent({ type: "error", message: "stream exploded" });
      throw new Error("stream exploded");
    });

    mountADKPage();
    await flushRequests();

    await sendPageMessage("check failed run");

    expect(document.querySelector(".adk-thread")?.textContent).toContain(
      "stream exploded",
    );
    expect(document.querySelector(".adk-inline-alert")?.textContent).toContain(
      "stream exploded",
    );
    expect(document.querySelector(".adk-composer")?.textContent).not.toContain(
      "stream exploded",
    );
  });

  it("treats failed final runs as terminal responses instead of stream errors", async () => {
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "本地兜底回复。",
        session: buildSession(),
        run: buildRun({
          id: "run-failed-final",
          status: "FAILED",
          message: "disk full",
          failureReason: "disk full",
          errorCode: "TOOL_EXECUTION_FAILED",
          toolCalls: [
            {
              ...buildToolCall(
                "tool-failed",
                "run-failed-final",
                "strategy.save_draft",
                "FAILED",
              ),
              error: "disk full",
            },
          ],
        }),
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("user_message", {
            id: "entry-user-failed",
            runId: "run-failed-final",
            text: "保存失败草稿",
            createdAt: "2026-06-06T00:00:00Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "entry-tool-failed",
            runId: "run-failed-final",
            toolCalls: [
              {
                ...buildToolCall(
                  "tool-failed",
                  "run-failed-final",
                  "strategy.save_draft",
                  "FAILED",
                ),
                error: "disk full",
              },
            ],
            createdAt: "2026-06-06T00:00:01Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-answer-failed",
            runId: "run-failed-final",
            text: "本地兜底回复。",
            createdAt: "2026-06-06T00:00:02Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage();
    await flushRequests();

    await sendPageMessage("保存失败草稿");

    expect(document.querySelector(".adk-thread")?.textContent).toContain(
      "本地兜底回复。",
    );
    expect(document.body.textContent).toContain("disk full");
    expect(document.body.textContent).toContain(
      "运行失败",
    );
    expect(document.body.textContent).not.toContain(
      "TOOL_EXECUTION_FAILED",
    );
    document.querySelector<HTMLButtonElement>(".adk-inline-alert__toggle")?.click();
    await nextTick();
    expect(document.body.textContent).toContain(
      "TOOL_EXECUTION_FAILED",
    );
    expect(document.body.textContent).toContain(
      "run-failed-final",
    );
    expect(
      document.querySelector<HTMLTextAreaElement>(
        ".adk-composer textarea, .adk-composer input",
      )?.disabled,
    ).toBe(false);
  });

  it("keeps deep reasoning collapsed until the user expands it", async () => {
    const reasoningText =
      "Detailed  chain of thought preview.\n  Preserve indentation.";
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "Final answer.",
        reasoningContent: reasoningText,
        session: buildSession(),
        run: buildRun({ status: "COMPLETED" }),
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("user_message", {
            id: "entry-user",
            text: "show reasoning",
            createdAt: "2026-06-06T00:00:00Z",
          }),
          buildTimelineEntry("assistant_reasoning", {
            id: "entry-reasoning",
            text: reasoningText,
            createdAt: "2026-06-06T00:00:01Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-answer",
            text: "Final answer.",
            createdAt: "2026-06-06T00:00:02Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage();
    await flushRequests();

    await sendPageMessage("show reasoning");

    expect(document.body.textContent).toContain("查看深度思考");
    expect(document.body.textContent).not.toContain(
      "Detailed  chain of thought preview.",
    );

    clickButtonByText("查看深度思考");
    await nextTick();

    expect(document.body.textContent).toContain("隐藏深度思考");
    expect(document.body.textContent).toContain(
      "Detailed  chain of thought preview.",
    );
    expect(document.body.textContent).toContain("  Preserve indentation.");
  });

  it("does not let an older session context response replace the selected session tag", async () => {
    const firstContext = deferred<ADKSessionContextSnapshot | null>();
    const secondContext = deferred<ADKSessionContextSnapshot | null>();
    mountADKPage({
      sessions: [
        buildSession({ id: "session-old", title: "旧会话" }),
        buildSession({ id: "session-current", title: "当前会话" }),
      ],
      sessionDetailSequence: [
        {
          session: buildSession({ id: "session-old", title: "旧会话" }),
          timeline: [],
        },
        {
          session: buildSession({ id: "session-current", title: "当前会话" }),
          timeline: [],
        },
      ],
      sessionContextSequence: [firstContext.promise, secondContext.promise],
    });
    await flushRequests();

    const sessions = Array.from(
      document.querySelectorAll<HTMLElement>(".adk-session-item"),
    );
    sessions[0]?.click();
    await flushRequests();
    sessions[1]?.click();
    await flushRequests();

    secondContext.resolve(
      buildSessionContextSnapshot({
        sessionId: "session-current",
        currentInputTokens: 2400,
        projectedNextTurnTokens: 2500,
        usageRatio: 0.24,
        status: "healthy",
      }),
    );
    await flushRequests();

    expect(document.body.textContent).toContain("24% 正常");

    firstContext.resolve(
      buildSessionContextSnapshot({
        sessionId: "session-old",
        currentInputTokens: 9900,
        projectedNextTurnTokens: 9900,
        usageRatio: 0.99,
        status: "critical",
      }),
    );
    await flushRequests();

    expect(document.body.textContent).toContain("24% 正常");
    expect(document.body.textContent).not.toContain("99% 危险");
  });

  it("does not let an older context revision overwrite an auto-compacted snapshot", async () => {
    const session = buildSession({ id: "session-context-revision" });
    const newer = buildSessionContextSnapshot({
      sessionId: session.id,
      contextRevisionId: "ctx-new",
      previousContextRevisionId: "ctx-old",
      contextRevisionCreatedAt: "2026-06-21T03:18:54Z",
      currentInputTokens: 1200,
      projectedNextTurnTokens: 1300,
      usageRatio: 0.12,
      activeHandoffCount: 1,
      autoCompacted: true,
      lastCompactedAt: "2026-06-21T03:18:54Z",
      lastCompactionMode: "auto",
    });
    const older = buildSessionContextSnapshot({
      sessionId: session.id,
      contextRevisionId: "ctx-old",
      contextRevisionCreatedAt: "2026-06-21T03:14:17Z",
      currentInputTokens: 9200,
      projectedNextTurnTokens: 9300,
      usageRatio: 0.92,
      autoCompacted: false,
    });
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      await onEvent({ type: "context", context: newer });
      const response: ADKChatResponse = {
        reply: "done",
        session,
        run: buildRun({ id: "run-context-revision", status: "COMPLETED" }),
        context: older,
        pendingApprovals: [],
        timeline: [],
      };
      await onEvent({ type: "final", response });
      return response;
    });
    mountADKPage({
      sessions: [session],
      sessionContext: older,
    });
    await flushRequests();

    await sendPageMessage("触发上下文更新");

    expect(document.body.textContent).toContain("12% 正常");
    expect(document.body.textContent).not.toContain("92% 正常");
  });

  it("persists composer state per session when switching history", async () => {
    const sessionA = buildSession({ id: "session-a", title: "会话 A" });
    const sessionB = buildSession({ id: "session-b", title: "会话 B" });
    const fetchMock = mountADKPage({
      sessions: [sessionA, sessionB],
      composerStateBySession: {
        [sessionA.id]: buildComposerState(sessionA.id, {
          chatDraft: "A 原始草稿",
          workModeOverride: "loop",
          goalObjectiveDraft: "A 目标草稿",
          goalObjectiveTouched: true,
        }),
        [sessionB.id]: buildComposerState(sessionB.id, {
          chatDraft: "B 草稿",
          workModeOverride: "chat",
        }),
      },
      sessionDetailSequence: [
        { session: sessionA, timeline: [] },
        { session: sessionB, timeline: [] },
        { session: sessionA, timeline: [] },
      ],
    });
    await flushRequests();

    const sessions = Array.from(
      document.querySelectorAll<HTMLElement>(".adk-session-item"),
    );
    sessions[0]?.click();
    await flushRequests();

    const textarea = document.querySelector<HTMLTextAreaElement>(
      ".adk-composer-input",
    )!;
    expect(textarea.value).toBe("A 原始草稿");
    expect(findWorkModeSelect()?.value).toBe("loop");
    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "A 目标草稿",
    );

    textarea.value = "A 编辑后草稿";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();

    sessions[1]?.click();
    await flushRequests();

    expect(lastComposerStatePatch(fetchMock, sessionA.id)).toMatchObject({
      chatDraft: "A 编辑后草稿",
      workModeOverride: "loop",
      goalObjectiveDraft: "A 目标草稿",
      goalObjectiveTouched: true,
    });
    expect(
      document.querySelector<HTMLTextAreaElement>(".adk-composer-input")?.value,
    ).toBe("B 草稿");
    expect(findWorkModeSelect()?.value).toBe("chat");

    sessions[0]?.click();
    await flushRequests();

    expect(
      document.querySelector<HTMLTextAreaElement>(".adk-composer-input")?.value,
    ).toBe("A 编辑后草稿");
    expect(findWorkModeSelect()?.value).toBe("loop");
  });

  it("does not mark a newer draft as saved when an older composer save resolves later", async () => {
    const session = buildSession({ id: "session-save-race" });
    const firstSave = deferred<ADKSessionComposerState>();
    const savedPatches: Partial<ADKSessionComposerState>[] = [];
    const fetchMock = mountADKPage({
      sessions: [session],
      sessionDetail: { session, timeline: [] },
      composerStateSave: async (sessionId, patch) => {
        savedPatches.push(patch);
        if (savedPatches.length === 1) {
          return firstSave.promise;
        }
        return buildComposerState(sessionId, patch);
      },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();
    const textarea = document.querySelector<HTMLTextAreaElement>(
      ".adk-composer-input",
    )!;
    textarea.value = "旧草稿";
    textarea.dispatchEvent(new Event("input"));
    await flushRequests();

    window.dispatchEvent(new Event("pagehide"));
    await nextTick();
    textarea.value = "新草稿";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();
    firstSave.resolve(buildComposerState(session.id, savedPatches[0]));
    await flushRequests();

    expect(
      fetchMock.mock.calls.filter(([input]) =>
        String(input).includes(
          `/api/v1/adk/sessions/${session.id}/composer-state`,
        ),
      ).length,
    ).toBeGreaterThanOrEqual(2);
    expect(lastComposerStatePatch(fetchMock, session.id)).toMatchObject({
      chatDraft: "新草稿",
    });
  });

  it("uses the agent default loop mode when sending without an explicit override", async () => {
    const session = buildSession({ id: "session-default-loop" });
    const run = buildRun({
      id: "run-default-loop",
      sessionId: session.id,
      status: "COMPLETED",
      workMode: "loop",
    });
    streamADKChatMock.mockResolvedValueOnce({
      reply: "done",
      session,
      run,
      pendingApprovals: [],
      timeline: [],
    });
    mountADKPage({
      agent: { workMode: "loop", loopMaxIterations: 5 },
      sessions: [session],
      sessionDetail: { session, timeline: [] },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();
    await sendPageMessage("默认目标");

    expect(streamADKChatMock).toHaveBeenCalledWith(
      expect.objectContaining({
        message: "默认目标",
        workModeOverride: "loop",
        objective: "默认目标",
      }),
      expect.any(Function),
      expect.any(Object),
    );
  });

  it("clears persisted stream cursor when deleting a session", async () => {
    const session = buildSession({ id: "session-delete-cursor" });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {
          [session.id]: {
            streamId: "stream-delete",
            runId: "run-delete",
            sequence: 3,
            activeChildRunId: "",
          },
        },
      }),
    );
    mountADKPage({
      sessions: [session],
      sessionDetail: { session, timeline: [] },
    });
    await flushRequests();

    document
      .querySelector<HTMLElement>('.adk-session-close[title="关闭会话"]')
      ?.click();
    await flushRequests();

    const persisted = JSON.parse(
      window.localStorage.getItem("jftrade.adk.page-state.v1") ?? "{}",
    ) as { selectedSessionId?: string; sessions?: Record<string, unknown> };
    expect(persisted.selectedSessionId).toBe("");
    expect(persisted.sessions?.[session.id]).toBeUndefined();
  });
});
