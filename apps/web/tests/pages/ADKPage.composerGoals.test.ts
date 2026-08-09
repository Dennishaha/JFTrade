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

describe("ADKPage composer and goals", () => {
  it("loads context immediately after creating an empty session", async () => {
    const session = buildSession({ id: "session-new-empty" });
    const context = buildSessionContextSnapshot({
      sessionId: session.id,
      currentInputTokens: 0,
      projectedNextTurnTokens: 0,
      usageRatio: 0,
      status: "healthy",
    });
    const fetchMock = mountADKPage({
      sessions: [],
      createSession: session,
      sessionContext: context,
    });
    await flushRequests();

    document.querySelector<HTMLButtonElement>('[title="新建会话"]')?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("0% 正常");
    expect(
      fetchMock.mock.calls.some(([input]) =>
        String(input).includes(`/api/v1/adk/sessions/${session.id}/context`),
      ),
    ).toBe(true);
  });

  it("shows a composer block when the selected provider has no API key", async () => {
    mountADKPage({ providerHasKey: false });
    await flushRequests();

    expect(document.body.textContent).toContain("API Key");
  });

  it("keeps session provider overrides without editing the selected agent", async () => {
    const session = buildSession({ id: "session-provider-override" });
    const providers = [
      buildProvider(true),
      buildProvider(true, {
        id: "provider-2",
        displayName: "Claude",
        model: "claude-sonnet",
      }),
    ];
    const fetchMock = mountADKPage({
      providers,
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [],
        composerState: buildComposerState(session.id),
      },
    });
    await flushRequests();
    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    const providerSelect = findProviderSelect("provider-2");
    expect(providerSelect).toBeDefined();
    providerSelect!.value = "provider-2";
    providerSelect!.dispatchEvent(new Event("change"));
    await nextTick();

    streamADKChatMock.mockImplementationOnce(async (payload) => ({
      reply: "ok",
      session,
      run: buildRun({
        id: "run-provider-override",
        sessionId: session.id,
        providerId: String(payload.providerId ?? ""),
        model: String(payload.model ?? ""),
        userMessage: String(payload.message ?? ""),
      }),
      pendingApprovals: [],
      timeline: [],
    }));
    await sendPageMessage("使用临时模型");

    expect(streamADKChatMock.mock.calls[0]?.[0]).toMatchObject({
      providerId: "provider-2",
      model: "claude-sonnet",
    });
    expect(
      fetchMock.mock.calls.some(
        ([input, init]) =>
          String(input).includes("/api/v1/adk/agents") &&
          String(init?.method ?? "GET").toUpperCase() !== "GET",
      ),
    ).toBe(false);
    expect(lastComposerStatePatch(fetchMock, session.id)).toMatchObject({
      providerIdOverride: "provider-2",
      modelOverride: "claude-sonnet",
    });

    mountADKPage({
      providers,
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [],
        composerState: buildComposerState(session.id, {
          providerIdOverride: "provider-2",
          modelOverride: "claude-sonnet",
        }),
      },
    });
    await flushRequests();
    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(findProviderSelect("provider-2")?.value).toBe("provider-2");
  });

  it("switches to the shared mobile shell on narrow viewports", async () => {
    const originalMatchMedia = window.matchMedia;
    const matchMediaMock = vi.fn().mockImplementation((query: string) => ({
      matches: query === "(max-width: 768px)",
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      writable: true,
      value: matchMediaMock,
    });

    try {
      mountADKPage();
      await flushRequests();

      expect(document.querySelector(".adk-shell--mobile")).not.toBeNull();
      expect(
        document.querySelector("[data-testid='adk-mobile-composer-summary']"),
      ).not.toBeNull();
      expect(document.querySelector(".adk-agent-select")).toBeNull();

      document
        .querySelector<HTMLButtonElement>(
          "[data-testid='adk-mobile-composer-toggle']",
        )
        ?.click();
      await nextTick();

      expect(document.querySelector(".adk-agent-select")).not.toBeNull();
      expect(document.querySelector(".adk-provider-select")).not.toBeNull();
    } finally {
      Object.defineProperty(window, "matchMedia", {
        configurable: true,
        writable: true,
        value: originalMatchMedia,
      });
    }
  });

  it("keeps generic hints even when the selected agent exposes strategy Pine tools", async () => {
    mountADKPage({
      agent: {
        tools: [
          "strategy.pine_spec",
          "strategy.validate_pine",
          "strategy.research_backtest",
          "backtest.result_view",
        ],
        skills: ["jftrade-strategy-research"],
      },
    });
    await flushRequests();

    expect(document.body.textContent).toContain("查看系统状态");
    expect(document.body.textContent).toContain("当前行情订阅");
    expect(document.body.textContent).not.toContain(
      "解释当前 JFTrade Pine Script v6 定义",
    );
    expect(
      document.querySelector("textarea")?.getAttribute("placeholder"),
    ).toBe("输入问题或任务...");
  });

  it("sends the edited goal objective when goal mode is selected", async () => {
    const goalRun = buildRun({
      id: "run-goal-payload",
      status: "COMPLETED",
      workMode: "loop",
      objective: "检查风险后给出交易计划",
      workflowStatus: "COMPLETED",
      workflowPlan: [buildWorkflowStep("goal-step-1", "推进目标", "DONE")],
    });
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "goal done",
        session: buildSession(),
        run: goalRun,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "goal-answer",
            runId: goalRun.id,
            text: "goal done",
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

    const workModeSelect = Array.from(
      document.querySelectorAll<HTMLSelectElement>("select"),
    ).find((select) =>
      Array.from(select.options).some((option) => option.value === "loop"),
    )!;
    workModeSelect.value = "loop";
    workModeSelect.dispatchEvent(new Event("change"));
    await nextTick();
    expect(document.querySelector(".adk-goal-editor")).toBeNull();

    const messageInput = document.querySelector<HTMLTextAreaElement>(
      ".adk-composer-input",
    )!;
    messageInput.value = "生成交易计划";
    messageInput.dispatchEvent(new Event("input"));
    await nextTick();
    expect(document.querySelector(".adk-goal-editor")).not.toBeNull();
    expect(document.querySelector(".adk-goal-editor__input")).toBeNull();

    document
      .querySelector<HTMLButtonElement>(".adk-goal-editor__icon")
      ?.click();
    await nextTick();

    const goalInput = document.querySelector<HTMLTextAreaElement>(
      ".adk-goal-editor__input",
    )!;
    goalInput.value = "检查风险后给出交易计划";
    goalInput.dispatchEvent(new Event("input"));
    await nextTick();

    document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
    await flushRequests();

    expect(streamADKChatMock).toHaveBeenCalledTimes(1);
    expect(streamADKChatMock.mock.calls[0]?.[0]).toMatchObject({
      message: "生成交易计划",
      workModeOverride: "loop",
      objective: "检查风险后给出交易计划",
    });
  });

  it("clears the goal editor after a loop goal completes", async () => {
    const session = buildSession({ id: "session-goal-complete-clear" });
    const completedGoalRun = buildRun({
      id: "run-goal-complete-clear",
      sessionId: session.id,
      status: "COMPLETED",
      workMode: "loop",
      objective: "完成后应清空目标栏",
      workflowStatus: "COMPLETED",
      workflowPlan: [
        buildWorkflowStep(
          "step-goal-complete-clear",
          "完成后应清空目标栏",
          "DONE",
        ),
      ],
    });
    window.localStorage.setItem(
      "jftrade.adk.page-state.v1",
      JSON.stringify({
        selectedSessionId: session.id,
        sessions: {},
      }),
    );
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "goal done",
        session,
        run: completedGoalRun,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "goal-complete-clear-answer",
            runId: completedGoalRun.id,
            text: "goal done",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      sessions: [session],
      sessionDetail: {
        session,
        timeline: [],
        runs: [],
        composerState: buildComposerState(session.id),
      },
    });
    await flushRequests();

    const workModeSelect = Array.from(
      document.querySelectorAll<HTMLSelectElement>("select"),
    ).find((select) =>
      Array.from(select.options).some((option) => option.value === "loop"),
    )!;
    workModeSelect.value = "loop";
    workModeSelect.dispatchEvent(new Event("change"));
    await nextTick();

    const messageInput = document.querySelector<HTMLTextAreaElement>(
      ".adk-composer-input",
    )!;
    messageInput.value = "开始执行";
    messageInput.dispatchEvent(new Event("input"));
    await nextTick();
    expect(document.querySelector(".adk-goal-editor")).not.toBeNull();

    document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
    await flushRequests();

    expect(document.querySelector(".adk-goal-editor")).toBeNull();
  });

  it("keeps the active goal editor when a child run becomes the latest active run", async () => {
    const parentRun = buildRun({
      id: "run-goal-parent-stable",
      status: "RUNNING",
      workMode: "loop",
      objective: "持续优化 TME 策略",
      workflowStatus: "PAUSED",
      workflowPlan: [
        buildWorkflowStep(
          "step-goal-child",
          "子智能体设计策略",
          "IN_PROGRESS",
          "run-goal-child-active",
        ),
      ],
    });
    const childRun = buildRun({
      id: "run-goal-child-active",
      parentRunId: parentRun.id,
      status: "RUNNING",
      workMode: "chat",
      objective: parentRun.objective,
      userMessage: "设计策略",
      usage: { modelCalls: 0, toolCallsTotal: 0 },
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "child still running",
        session: buildSession(),
        run: childRun,
        context: buildSessionContextSnapshot(),
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "goal-child-progress",
            runId: childRun.id,
            text: "child still running",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "run", run: parentRun });
      await onEvent({ type: "run", run: childRun });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        [childRun.id]: childRun,
      },
    });
    await flushRequests();

    await sendPageMessage("run goal with child");

    const editor = document.querySelector(".adk-goal-editor");
    expect(editor).not.toBeNull();
    expect(editor?.textContent).toContain("目标");
    expect(editor?.textContent).toContain("持续优化 TME 策略");
  });

  it("keeps the active goal editor after a child run reaches terminal state while the parent remains paused", async () => {
    const parentRun = buildRun({
      id: "run-goal-parent-paused",
      status: "RUNNING",
      workMode: "loop",
      objective: "继续完善 TME 目标",
      workflowStatus: "PAUSED",
      workflowPlan: [
        buildWorkflowStep(
          "step-goal-child-terminal",
          "子智能体策略设计",
          "IN_PROGRESS",
          "run-goal-child-terminal",
        ),
      ],
    });
    const childRunning = buildRun({
      id: "run-goal-child-terminal",
      parentRunId: parentRun.id,
      status: "RUNNING",
      workMode: "chat",
      objective: parentRun.objective,
    });
    const childCompleted = {
      ...childRunning,
      status: "COMPLETED",
      message: "completed",
    };

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "child completed",
        session: buildSession(),
        run: childCompleted,
        context: buildSessionContextSnapshot(),
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "goal-child-completed",
            runId: childCompleted.id,
            text: "child completed",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "run", run: parentRun });
      await onEvent({ type: "run", run: childRunning });
      await onEvent({ type: "run", run: childCompleted });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        [childCompleted.id]: childCompleted,
      },
    });
    await flushRequests();

    await sendPageMessage("run goal child terminal");

    const editor = document.querySelector(".adk-goal-editor");
    expect(editor).not.toBeNull();
    expect(editor?.textContent).toContain("继续完善 TME 目标");
  });

  it("sends follow-up text as chat while an active goal exists", async () => {
    const goalRun = buildRun({
      id: "run-active-goal-follow-up",
      status: "PAUSED",
      workMode: "loop",
      objective: "持续跟踪 TME 策略",
      workflowStatus: "PAUSED",
      workflowPlan: [
        buildWorkflowStep("step-active-goal", "推进目标", "IN_PROGRESS"),
      ],
    });
    const followUpRun = buildRun({
      id: "run-goal-follow-up-chat",
      status: "COMPLETED",
      workMode: "chat",
      userMessage: "补充：更关注回撤",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "goal paused",
          session: buildSession(),
          run: goalRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("assistant_message", {
              id: "goal-paused-answer",
              runId: goalRun.id,
              text: "goal paused",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "follow-up noted",
          session: buildSession(),
          run: followUpRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "follow-up-user",
              text: String(payload.message),
            }),
            buildTimelineEntry("assistant_message", {
              id: "follow-up-answer",
              runId: followUpRun.id,
              text: "follow-up noted",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    mountADKPage();
    await flushRequests();

    const workModeSelect = Array.from(
      document.querySelectorAll<HTMLSelectElement>("select"),
    ).find((select) =>
      Array.from(select.options).some((option) => option.value === "loop"),
    )!;
    workModeSelect.value = "loop";
    workModeSelect.dispatchEvent(new Event("change"));
    await nextTick();

    await sendPageMessage("建立 TME 目标");
    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "持续跟踪 TME 策略",
    );

    await sendPageMessage("补充：更关注回撤");

    expect(streamADKChatMock).toHaveBeenCalledTimes(2);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "补充：更关注回撤",
      workModeOverride: "chat",
    });
    expect(streamADKChatMock.mock.calls[1]?.[0]).not.toHaveProperty(
      "objective",
    );
    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "持续跟踪 TME 策略",
    );
    expect(
      document.querySelector(".adk-goal-editor")?.textContent,
    ).not.toContain("补充：更关注回撤");
  });

  it("keeps queued text as chat when it was entered during an active goal", async () => {
    const pendingApproval = buildApproval(
      "approval-active-goal-queued",
      "run-active-goal-queued",
    );
    const pendingGoalRun = buildRun({
      id: "run-active-goal-queued",
      status: "PENDING_APPROVAL",
      workMode: "loop",
      objective: "稳定推进 TME 目标",
      workflowStatus: "PAUSED",
      workflowPlan: [
        buildWorkflowStep("step-active-goal-queued", "等待审批", "BLOCKED"),
      ],
      toolCalls: [
        buildToolCall(
          "tool-active-goal-queued",
          "run-active-goal-queued",
          "strategy.save_draft",
          "PENDING_APPROVAL",
        ),
      ],
      pendingApprovals: [pendingApproval],
    });
    const completedGoalRun = {
      ...pendingGoalRun,
      status: "COMPLETED",
      workflowStatus: "COMPLETED",
      pendingApprovals: [],
      toolCalls: [
        buildToolCall(
          "tool-active-goal-queued",
          "run-active-goal-queued",
          "strategy.save_draft",
          "SUCCEEDED",
        ),
      ],
    };
    const queuedChatRun = buildRun({
      id: "run-active-goal-queued-chat",
      status: "COMPLETED",
      workMode: "chat",
      userMessage: "目标期间的补充说明",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting approval",
          session: buildSession(),
          run: pendingGoalRun,
          pendingApprovals: [pendingApproval],
          timeline: pendingApprovalTimeline(
            pendingGoalRun,
            [pendingApproval],
            "建立审批目标",
          ),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "queued chat done",
          session: buildSession(),
          run: queuedChatRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "queued-goal-chat-user",
              text: String(payload.message),
            }),
            buildTimelineEntry("assistant_message", {
              id: "queued-goal-chat-answer",
              runId: queuedChatRun.id,
              text: "queued chat done",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    mountADKPage({
      approvals: [pendingApproval],
      approvalResolution: {
        approval: { ...pendingApproval, status: "APPROVED" },
        run: completedGoalRun,
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "completed-goal-answer",
            runId: completedGoalRun.id,
            text: "goal completed",
          }),
        ],
      },
    });
    await flushRequests();

    const workModeSelect = Array.from(
      document.querySelectorAll<HTMLSelectElement>("select"),
    ).find((select) =>
      Array.from(select.options).some((option) => option.value === "loop"),
    )!;
    workModeSelect.value = "loop";
    workModeSelect.dispatchEvent(new Event("change"));
    await nextTick();

    await sendPageMessage("建立审批目标");
    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "稳定推进 TME 目标",
    );

    await sendPageMessage("目标期间的补充说明");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("目标期间的补充说明");

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();

    expect(streamADKChatMock).toHaveBeenCalledTimes(2);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "目标期间的补充说明",
      workModeOverride: "chat",
    });
    expect(streamADKChatMock.mock.calls[1]?.[0]).not.toHaveProperty(
      "objective",
    );
  });
});
