// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import type {
  ADKApproval,
  ADKChatResponse,
  ADKRun,
  ADKSessionComposerState,
  ADKSessionContextSnapshot,
  ADKTimelineEntry,
} from "@/contracts";

import { resetADKApprovalInFlightForTest } from "../src/composables/adkApprovalResolution";
import ADKPage from "../src/pages/ADKPage.vue";
import { createResponse, flushRequests } from "./helpers";

const {
  monitorADKRunContinuationMock,
  resumeADKChatStreamMock,
  streamADKChatMock,
} = vi.hoisted(() => ({
  monitorADKRunContinuationMock: vi.fn(),
  resumeADKChatStreamMock: vi.fn(),
  streamADKChatMock: vi.fn(),
}));

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(),
    run: vi.fn(),
  },
}));

vi.mock("../src/composables/adkChatStream", async () => {
  const actual = await vi.importActual<
    typeof import("../src/composables/adkChatStream")
  >("../src/composables/adkChatStream");
  return {
    ...actual,
    resumeADKChatStream: resumeADKChatStreamMock,
    streamADKChat: streamADKChatMock,
  };
});

vi.mock("../src/composables/adkRunContinuation", async () => {
  const actual = await vi.importActual<
    typeof import("../src/composables/adkRunContinuation")
  >("../src/composables/adkRunContinuation");
  return {
    ...actual,
    monitorADKRunContinuation: monitorADKRunContinuationMock,
  };
});

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  resetADKApprovalInFlightForTest();
  streamADKChatMock.mockReset();
  resumeADKChatStreamMock.mockReset();
  resumeADKChatStreamMock.mockResolvedValue(null);
  monitorADKRunContinuationMock.mockReset();
  monitorADKRunContinuationMock.mockImplementation(async (run) => run);
});

describe("ADKPage", () => {
  it("shows a composer block when the selected provider has no API key", async () => {
    mountADKPage({ providerHasKey: false });
    await flushRequests();

    expect(document.body.textContent).toContain("API Key");
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
      workflowPlan: [
        buildWorkflowStep("goal-step-1", "推进目标", "DONE"),
      ],
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

    document.querySelector<HTMLButtonElement>(".adk-goal-editor__icon")?.click();
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
    expect(document.querySelector(".adk-goal-editor")?.textContent).not.toContain(
      "补充：更关注回撤",
    );
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

  it("shows the latest workflow plan directly above the composer and clears it on the next run", async () => {
    const workflowRun = buildRun({
      id: "run-workflow-plan",
      status: "COMPLETED",
      workMode: "task",
      objective: "检查账户风险后生成建议",
      workflowStatus: "COMPLETED",
      workflowPlan: [
        buildWorkflowStep("step-1", "读取账户状态", "DONE"),
        buildWorkflowStep("step-2", "生成风险摘要", "DONE"),
      ],
    });
    const chatRun = buildRun({
      id: "run-chat-next",
      status: "COMPLETED",
      workMode: "chat",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "workflow done",
          session: buildSession(),
          run: workflowRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("assistant_message", {
              id: "entry-workflow-answer",
              runId: workflowRun.id,
              text: "workflow done",
              createdAt: "2026-06-06T00:00:02Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "chat done",
          session: buildSession(),
          run: chatRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("assistant_message", {
              id: "entry-chat-answer",
              runId: chatRun.id,
              text: "chat done",
              createdAt: "2026-06-06T00:00:04Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    mountADKPage();
    await flushRequests();

    await sendPageMessage("run workflow");

    const panel = document.querySelector(".adk-workflow-plan-panel");
    const composer = document.querySelector(".adk-composer");
    expect(panel).not.toBeNull();
    expect(panel?.nextElementSibling).toBe(composer);
    expect(panel?.textContent).toContain("执行计划");
    await expandQueue("执行计划");
    expect(panel?.textContent).toContain("读取账户状态");
    expect(panel?.textContent).toContain("检查账户风险后生成建议");

    await sendPageMessage("plain chat");

    expect(document.querySelector(".adk-workflow-plan-panel")).toBeNull();
  });

  it("opens a child agent filtered view from the child run queue", async () => {
    const workflowRun = buildRun({
      id: "parent-run-drilldown",
      status: "COMPLETED",
      workMode: "task",
      childRunIds: ["child-run-drilldown"],
      workflowPlan: [
        buildWorkflowStep(
          "step-child",
          "检查子智能体",
          "DONE",
          "child-run-drilldown",
        ),
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "parent done",
        session: buildSession(),
        run: workflowRun,
        context: buildSessionContextSnapshot(),
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "parent-answer",
            runId: workflowRun.id,
            text: "parent visible answer",
            createdAt: "2026-06-06T00:00:02Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "child-answer",
            runId: "child-run-drilldown",
            text: "child filtered answer",
            createdAt: "2026-06-06T00:00:03Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "child-tools",
            runId: "child-run-drilldown",
            toolCalls: [
              buildToolCall(
                "child-tool",
                "child-run-drilldown",
                "strategy.inspect_child",
                "SUCCEEDED",
              ),
            ],
            createdAt: "2026-06-06T00:00:04Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "parent-copy-child-tools",
            runId: workflowRun.id,
            toolCalls: [
              {
                ...buildToolCall(
                  "parent-copy-child-tool",
                  "child-run-drilldown",
                  "strategy.parent_copy_child_result",
                  "SUCCEEDED",
                ),
                output: { result: "child-only-success" },
              },
            ],
            createdAt: "2026-06-06T00:00:05Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        "child-run-drilldown": buildRun({
          id: "child-run-drilldown",
          parentRunId: workflowRun.id,
          status: "RUNNING",
          usage: { tokensIn: 1200, tokensOut: 300 },
        }),
      },
    });
    await flushRequests();

    await sendPageMessage("run child workflow");
    expect(document.body.textContent).toContain("42% 正常");
    await expandQueue("子智能体");
    const childQueue = document.querySelector('[aria-label="子智能体"]');
    expect(childQueue).not.toBeNull();
    expect(document.querySelector('[aria-label="执行计划"]')).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue__badge.is-success")).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue-status.is-success")).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue__badge.is-error")).toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue-status.is-error")).toBeNull();
    expect(document.body.textContent).toContain("检查子智能体");
    expect(document.body.textContent).toContain("parent visible answer");
    expect(document.body.textContent).toContain("启动子智能体 #1");
    expect(document.body.textContent).toContain("子智能体 #1 已结束：已完成");
    expect(
      document.querySelector('[aria-label="子智能体"]')?.textContent,
    ).not.toContain("运行中");
    expect(
      document.querySelector('[aria-label="执行计划"]')?.textContent,
    ).not.toContain("IN_PROGRESS");
    expect(document.body.textContent).not.toContain("child filtered answer");
    expect(document.body.textContent).not.toContain("strategy.inspect_child");
    expect(document.body.textContent).not.toContain(
      "strategy.parent_copy_child_result",
    );
    expect(document.body.textContent).not.toContain("child-only-success");

    clickButtonByText("进入");
    await nextTick();

    expect(document.body.textContent).toContain("子智能体 #1");
    expect(document.body.textContent).toContain("child-run-drilldown");
    expect(document.body.textContent).toContain("42% 正常");
    expect(document.body.textContent).not.toContain("15% 正常");
    expect(document.body.textContent).toContain("child filtered answer");
    expect(document.body.textContent).toContain("strategy.inspect_child");
    expect(document.body.textContent).not.toContain(
      "strategy.parent_copy_child_result",
    );
    expect(document.body.textContent).not.toContain("parent visible answer");
    expect(document.body.textContent).toContain("子智能体视图仅支持观察和审批");
    expect(document.querySelector('[aria-label="子智能体"]')).toBeNull();
    expect(document.querySelector('[aria-label="执行计划"]')).toBeNull();

    clickButtonByText("返回父对话");
    await nextTick();

    expect(document.body.textContent).toContain("parent visible answer");
    expect(document.body.textContent).toContain("42% 正常");
    expect(document.body.textContent).not.toContain("15% 正常");
    expect(document.body.textContent).toContain("启动子智能体 #1");
    expect(document.body.textContent).toContain("子智能体 #1 已结束：已完成");
    expect(document.body.textContent).not.toContain("child filtered answer");
    expect(document.body.textContent).not.toContain("strategy.inspect_child");
    expect(document.body.textContent).not.toContain(
      "strategy.parent_copy_child_result",
    );
    expect(document.body.textContent).not.toContain("child-only-success");
    expect(document.querySelector('[aria-label="子智能体"]')).not.toBeNull();
    expect(document.querySelector('[aria-label="执行计划"]')).not.toBeNull();
  });

  it("keeps parent session context visible in a child view when child usage is unavailable", async () => {
    const fallbackContext = buildSessionContextSnapshot({
      summaryPreview: "父会话上下文仍然有效",
    });
    const workflowRun = buildRun({
      id: "parent-run-child-context-fallback",
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "PAUSED",
      childRunIds: ["child-run-no-usage"],
      workflowPlan: [
        buildWorkflowStep(
          "step-child-no-usage",
          "等待子智能体上报用量",
          "IN_PROGRESS",
          "child-run-no-usage",
        ),
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "parent running",
        session: buildSession(),
        run: workflowRun,
        context: fallbackContext,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "parent-context-answer",
            runId: workflowRun.id,
            text: "parent context answer",
          }),
          buildTimelineEntry("assistant_message", {
            id: "child-context-answer",
            runId: "child-run-no-usage",
            text: "child context answer",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "run", run: workflowRun });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        "child-run-no-usage": buildRun({
          id: "child-run-no-usage",
          parentRunId: workflowRun.id,
          status: "RUNNING",
          usage: { modelCalls: 0, toolCallsTotal: 0 },
        }),
      },
    });
    await flushRequests();

    await sendPageMessage("run child without usage");
    expect(document.body.textContent).toContain("42% 正常");

    await expandQueue("子智能体");
    clickButtonByText("进入");
    await nextTick();

    expect(document.body.textContent).toContain("子智能体 #1");
    expect(document.body.textContent).toContain("child-run-no-usage");
    expect(document.body.textContent).toContain("42% 正常");
    expect(document.body.textContent).toContain("父会话上下文仍然有效");
    expect(document.body.textContent).not.toContain("子智能体运行用量");
  });

  it("keeps backend session context visible in a child view even when child usage exists", async () => {
    const fallbackContext = buildSessionContextSnapshot({
      contextRevisionId: "ctx-child-visible",
      contextRevisionCreatedAt: "2026-06-18T10:10:00Z",
      contextWindowTokens: 20000,
      currentInputTokens: 6000,
      projectedNextTurnTokens: 6200,
      usageRatio: 0.3,
      activeHandoffCount: 1,
      compactedEventCount: 12,
      autoCompacted: true,
      summaryPreview: "父会话后端上下文快照",
    });
    const workflowRun = buildRun({
      id: "parent-run-child-context-usage",
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "PAUSED",
      childRunIds: ["child-run-with-usage"],
      workflowPlan: [
        buildWorkflowStep(
          "step-child-with-usage",
          "观察子智能体用量",
          "IN_PROGRESS",
          "child-run-with-usage",
        ),
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "parent running",
        session: buildSession(),
        run: workflowRun,
        context: fallbackContext,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "child-usage-answer",
            runId: "child-run-with-usage",
            text: "child usage answer",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "run", run: workflowRun });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        "child-run-with-usage": buildRun({
          id: "child-run-with-usage",
          parentRunId: workflowRun.id,
          status: "RUNNING",
          usage: { tokensIn: 2000, tokensOut: 1000 },
        }),
      },
    });
    await flushRequests();

    await sendPageMessage("run child with usage");
    await expandQueue("子智能体");
    clickButtonByText("进入");
    await nextTick();

    expect(document.body.textContent).toContain("30% 正常");
    expect(document.body.textContent).toContain("模型窗口");
    expect(document.body.textContent).toContain("20,000");
    expect(document.body.textContent).toContain("自动压缩");
    expect(document.body.textContent).toContain("生效 handoff 段数");
    expect(document.body.textContent).toContain("当前上下文版本");
    expect(document.body.textContent).toContain("ctx-child-visible");
    expect(document.body.textContent).toContain("已压缩事件数");
    expect(document.body.textContent).toContain("12");
    expect(document.body.textContent).toContain("版本创建时间");
    expect(document.body.textContent).toContain("2026-06-18T10:10:00Z");
    expect(document.body.textContent).toContain("父会话后端上下文快照");
    expect(document.body.textContent).not.toContain("15% 正常");
    expect(document.body.textContent).not.toContain("子智能体运行用量");
  });

  it("keeps the context tag when refreshing context fails after a valid snapshot", async () => {
    const context = buildSessionContextSnapshot({
      summaryPreview: "已有上下文快照",
    });
    const run = buildRun({ id: "run-context-refresh", status: "COMPLETED" });
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "context ready",
        session: buildSession(),
        run,
        context,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "context-ready-answer",
            runId: run.id,
            text: "context ready",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      sessionContextSequence: [new Error("context unavailable")],
    });
    await flushRequests();

    await sendPageMessage("create context");
    expect(document.body.textContent).toContain("42% 正常");

    await sendPageMessage("/context");

    expect(document.body.textContent).toContain("42% 正常");
    expect(document.body.textContent).toContain("已有上下文快照");
  });

  it("marks failed child agent queue items as error instead of success", async () => {
    const workflowRun = buildRun({
      id: "parent-run-child-failed",
      status: "FAILED",
      workMode: "task",
      childRunIds: ["child-run-failed"],
      workflowPlan: [
        buildWorkflowStep(
          "step-child-failed",
          "失败子智能体",
          "BLOCKED",
          "child-run-failed",
        ),
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "parent failed",
        session: buildSession(),
        run: workflowRun,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "parent-failed-answer",
            runId: workflowRun.id,
            text: "parent failed answer",
            createdAt: "2026-06-06T00:00:02Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        "child-run-failed": buildRun({
          id: "child-run-failed",
          parentRunId: workflowRun.id,
          status: "FAILED",
        }),
      },
    });
    await flushRequests();

    await sendPageMessage("run failed child workflow");
    await expandQueue("子智能体");

    const childQueue = document.querySelector('[aria-label="子智能体"]');
    expect(childQueue).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue__badge.is-error")).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue-status.is-error")).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue__badge.is-success")).toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue-status.is-success")).toBeNull();
  });

  it("clears the token indicator when deleting the selected conversation", async () => {
    mountADKPage({
      sessionContext: buildSessionContextSnapshot(),
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("42% 正常");

    document
      .querySelector<HTMLElement>('.adk-session-close[title="关闭会话"]')
      ?.click();
    await flushRequests();

    expect(document.body.textContent).not.toContain("42% 正常");
  });

  it("summarizes a pending child approval in the parent timeline without child details", async () => {
    const approval = buildApproval("approval-child-pending", "child-run-approval");
    const parentRun = buildRun({
      id: "parent-run-approval",
      status: "PENDING_APPROVAL",
      workMode: "task",
      childRunIds: ["child-run-approval"],
      workflowPlan: [
        buildWorkflowStep(
          "step-child-approval",
          "等待子审批",
          "BLOCKED",
          "child-run-approval",
        ),
      ],
    });
    const childRun = buildRun({
      id: "child-run-approval",
      parentRunId: parentRun.id,
      status: "PENDING_APPROVAL",
      pendingApprovals: [approval],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "parent waiting",
        session: buildSession(),
        run: parentRun,
        pendingApprovals: [approval],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "parent-waiting-answer",
            runId: parentRun.id,
            text: "parent waiting answer",
            createdAt: "2026-06-06T00:00:02Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "child-approval-detail",
            runId: childRun.id,
            text: "child approval detail should stay hidden",
            createdAt: "2026-06-06T00:00:03Z",
          }),
          buildTimelineEntry("approval_group", {
            id: "child-approval-group",
            runId: childRun.id,
            approvals: [approval],
            createdAt: "2026-06-06T00:00:04Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      approvals: [approval],
      runById: {
        "child-run-approval": childRun,
      },
    });
    await flushRequests();

    await sendPageMessage("run child approval workflow");

    expect(document.body.textContent).toContain("parent waiting answer");
    expect(document.body.textContent).toContain("启动子智能体 #1");
    expect(document.body.textContent).toContain("子智能体 #1 等待审批");
    expect(document.body.textContent).not.toContain(
      "child approval detail should stay hidden",
    );
    expect(document.querySelector('[aria-label="待审批"]')).not.toBeNull();

    await expandQueue("子智能体");
    clickButtonByText("进入");
    await nextTick();

    expect(document.body.textContent).toContain(
      "child approval detail should stay hidden",
    );
  });

  it("shows the active child run's own workflow queues instead of the parent queues", async () => {
    const parentRun = buildRun({
      id: "parent-run-nested",
      status: "COMPLETED",
      workMode: "task",
      childRunIds: ["child-run-nested"],
      workflowPlan: [
        buildWorkflowStep(
          "parent-step",
          "父级入口步骤",
          "DONE",
          "child-run-nested",
        ),
      ],
    });
    const childRun = buildRun({
      id: "child-run-nested",
      parentRunId: parentRun.id,
      status: "RUNNING",
      workMode: "task",
      childRunIds: ["grandchild-run"],
      workflowPlan: [
        buildWorkflowStep(
          "child-step",
          "子级计划步骤",
          "IN_PROGRESS",
          "grandchild-run",
        ),
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "parent done",
        session: buildSession(),
        run: parentRun,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "nested-child-answer",
            runId: "child-run-nested",
            text: "nested child answer",
            createdAt: "2026-06-06T00:00:03Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        "child-run-nested": childRun,
      },
    });
    await flushRequests();

    await sendPageMessage("run nested child workflow");
    await expandQueue("子智能体");
    clickButtonByText("进入");
    await nextTick();

    expect(document.body.textContent).toContain("nested child answer");
    await expandQueue("执行计划");
    await expandQueue("子智能体");

    expect(document.querySelector('[aria-label="执行计划"]')?.textContent).toContain(
      "子级计划步骤",
    );
    expect(document.querySelector('[aria-label="子智能体"]')?.textContent).toContain(
      "子级计划步骤",
    );
    expect(document.querySelector('[aria-label="执行计划"]')?.textContent).not.toContain(
      "父级入口步骤",
    );
    expect(document.querySelector('[aria-label="子智能体"]')?.textContent).not.toContain(
      "父级入口步骤",
    );
  });

  it("updates the workflow plan from approval parentRun instead of the child run", async () => {
    const approval = buildApproval("approval-workflow", "child-run-1");
    const parentPendingRun = buildRun({
      id: "parent-run-1",
      status: "PENDING_APPROVAL",
      workMode: "loop",
      workflowStatus: "PAUSED",
      objective: "保存策略草稿",
      childRunIds: ["child-run-1"],
      workflowPlan: [
        buildWorkflowStep("step-1", "生成保存参数", "DONE", "child-run-1"),
        buildWorkflowStep("step-2", "等待保存审批", "BLOCKED", "child-run-1"),
      ],
      pendingApprovals: [approval],
    });
    const childCompletedRun = buildRun({
      id: "child-run-1",
      status: "COMPLETED",
      workMode: "chat",
      pendingApprovals: [],
    });
    const parentCompletedRun = buildRun({
      id: "parent-run-1",
      status: "COMPLETED",
      workMode: "loop",
      workflowStatus: "COMPLETED",
      objective: "保存策略草稿",
      childRunIds: ["child-run-1"],
      workflowPlan: [
        buildWorkflowStep("step-1", "生成保存参数", "DONE", "child-run-1"),
        buildWorkflowStep("step-2", "保存审批已通过", "DONE", "child-run-1"),
      ],
      pendingApprovals: [],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run: parentPendingRun,
        pendingApprovals: [approval],
        timeline: pendingApprovalTimeline(
          parentPendingRun,
          [approval],
          "workflow approval",
        ),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      approvals: [approval],
      approvalResolution: {
        approval: { ...approval, status: "APPROVED" },
        run: childCompletedRun,
        parentRun: parentCompletedRun,
      },
    });
    await flushRequests();

    await sendPageMessage("workflow approval");
    await expandQueue("执行计划");
    expect(document.querySelector(".adk-workflow-plan-panel")?.textContent).toContain(
      "等待保存审批",
    );

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();

    const panelText = document.querySelector(".adk-workflow-plan-panel")?.textContent;
    expect(panelText).toContain("保存审批已通过");
    expect(panelText).toContain("执行计划");
    expect(panelText).not.toContain("等待保存审批");
  });

  it("refreshes approval state to RUNNING, hides the approval bar, and keeps input editable", async () => {
    const pendingApproval = buildApproval("approval-1", "run-approval");
    const pendingRun = buildRun({
      id: "run-approval",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall(
          "tool-1",
          "run-approval",
          "strategy.save_draft",
          "PENDING_APPROVAL",
        ),
      ],
      pendingApprovals: [pendingApproval],
    });
    const runningRun = buildRun({
      id: "run-approval",
      status: "RUNNING",
      toolCalls: [
        buildToolCall(
          "tool-1",
          "run-approval",
          "strategy.save_draft",
          "RUNNING",
        ),
      ],
      pendingApprovals: [],
    });
    const completedRun = buildRun({
      id: "run-approval",
      status: "COMPLETED",
      toolCalls: [
        buildToolCall(
          "tool-1",
          "run-approval",
          "strategy.save_draft",
          "SUCCEEDED",
        ),
      ],
      pendingApprovals: [],
    });

    let finishContinuation!: () => void;
    monitorADKRunContinuationMock.mockImplementationOnce(
      async (run, options) => {
        await options?.onProgress?.(runningRun, run!);
        await new Promise<void>((resolve) => {
          finishContinuation = resolve;
        });
        await options?.onTerminal?.(completedRun);
        return completedRun;
      },
    );

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run: pendingRun,
        pendingApprovals: [pendingApproval],
        timeline: pendingApprovalTimeline(
          pendingRun,
          [pendingApproval],
          "approve this",
        ),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      approvals: [pendingApproval],
      approvalResolution: {
        approval: { ...pendingApproval, status: "APPROVED" },
        run: runningRun,
      },
      sessionDetailSequence: [
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "running-tools",
              runId: runningRun.id,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-06T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "running-tools-2",
              runId: runningRun.id,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-06T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "completed-tools",
              runId: completedRun.id,
              toolCalls: completedRun.toolCalls,
              createdAt: "2026-06-06T00:00:02Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "completed-answer",
              runId: completedRun.id,
              text: "approved and finished",
              createdAt: "2026-06-06T00:00:03Z",
            }),
          ],
        },
      ],
    });
    await flushRequests();

    await sendPageMessage("approve this");

    expect(document.body.textContent).toContain("PENDING_APPROVAL");
    expect(document.querySelector("textarea")?.hasAttribute("disabled")).toBe(
      false,
    );

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();

    expect(document.querySelector(".adk-run-spinner")).not.toBeNull();
    expect(document.querySelector(".adk-approvals-approve-all")).toBeNull();
    expect(document.querySelector("textarea")?.hasAttribute("disabled")).toBe(
      false,
    );
    expect(document.body.textContent).not.toContain("approved and finished");

    finishContinuation();
    await flushRequests();

    expect(document.body.textContent).toContain("approved and finished");
  });

  it("shows a second approval produced during continuation without refreshing", async () => {
    const runId = "run-second-approval";
    const firstApproval = buildApproval("approval-first", runId);
    const firstPendingRun = buildRun({
      id: runId,
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-first", runId, "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [firstApproval],
    });
    const runningRun = buildRun({
      id: runId,
      status: "RUNNING",
      toolCalls: [
        buildToolCall("tool-first", runId, "strategy.save_draft", "RUNNING"),
      ],
      pendingApprovals: [],
    });
    const secondApproval = {
      ...buildApproval("approval-second", runId),
      reason: "second approval required",
      input: { query: "second-draft" },
      createdAt: "2026-06-06T00:00:04Z",
      updatedAt: "2026-06-06T00:00:04Z",
    };
    const secondPendingRun = buildRun({
      id: runId,
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall("tool-first", runId, "strategy.save_draft", "SUCCEEDED"),
        buildToolCall("tool-second", runId, "strategy.save_draft", "PENDING_APPROVAL"),
      ],
      pendingApprovals: [secondApproval],
      resumeState: "waiting_approval",
      updatedAt: "2026-06-06T00:00:04Z",
    });

    monitorADKRunContinuationMock.mockImplementationOnce(
      async (run, options) => {
        await options?.onProgress?.(secondPendingRun, run!);
        return secondPendingRun;
      },
    );
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run: firstPendingRun,
        pendingApprovals: [firstApproval],
        timeline: pendingApprovalTimeline(
          firstPendingRun,
          [firstApproval],
          "first approval request",
        ),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      approvals: [firstApproval],
      approvalResolutionById: {
        "approval-first": {
          approval: { ...firstApproval, status: "APPROVED" },
          run: runningRun,
        },
      },
      sessionDetailSequence: [
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "tools-running",
              runId,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-06T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: pendingApprovalTimeline(
            secondPendingRun,
            [secondApproval],
            "first approval request",
          ),
        },
      ],
    });
    await flushRequests();

    await sendPageMessage("first approval request");
    await sendPageMessage("queued follow-up");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();
    await flushRequests();
    await expandQueue("待审批");

    expect(document.body.textContent).toContain("second approval required");
    expect(document.body.textContent).toContain("second-draft");
    expect(document.body.textContent).toContain("queued follow-up");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);
  });

  it("approves all pending approvals from the inline approval group", async () => {
    const approvalA = buildApproval("approval-1");
    const approvalB = {
      ...approvalA,
      id: "approval-2",
      toolName: "strategy.publish",
      input: { query: "@strategy.publish" },
    };

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run: buildRun({
          status: "PENDING_APPROVAL",
          toolCalls: [],
          pendingApprovals: [approvalA, approvalB],
        }),
        pendingApprovals: [approvalA, approvalB],
        timeline: [
          buildTimelineEntry("user_message", {
            id: "entry-user",
            text: "batch approve",
            createdAt: "2026-06-06T00:00:00Z",
          }),
          buildTimelineEntry("approval_group", {
            id: "entry-approvals",
            approvals: [approvalA, approvalB],
            createdAt: "2026-06-06T00:00:01Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    const fetchMock = mountADKPage({
      approvals: [approvalA, approvalB],
      approvalResolutionById: {
        "approval-1": {
          approval: { ...approvalA, status: "APPROVED" },
          run: buildRun({ id: "run-a", status: "COMPLETED" }),
        },
        "approval-2": {
          approval: { ...approvalB, status: "APPROVED" },
          run: buildRun({ id: "run-b", status: "COMPLETED" }),
        },
      },
    });
    await flushRequests();

    await sendPageMessage("batch approve");
    await expandQueue("待审批");
    clickButtonByText("全部批准");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/approvals/approval-1/approve"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/approvals/approval-2/approve"),
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("deduplicates repeated approval ids before approving all", async () => {
    const approval = buildApproval("approval-duplicate", "run-duplicate");
    const duplicateApproval = { ...approval, reason: "duplicate copy" };
    const run = buildRun({
      id: "run-duplicate",
      status: "PENDING_APPROVAL",
      pendingApprovals: [approval, duplicateApproval],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run,
        pendingApprovals: [approval, duplicateApproval],
        timeline: pendingApprovalTimeline(
          run,
          [approval, duplicateApproval],
          "duplicate approval",
        ),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    const fetchMock = mountADKPage({
      approvals: [approval],
      approvalResolution: {
        approval: { ...approval, status: "APPROVED" },
        run: buildRun({
          id: "run-duplicate",
          status: "COMPLETED",
          pendingApprovals: [],
        }),
      },
    });
    await flushRequests();

    await sendPageMessage("duplicate approval");

    await expandQueue("待审批");
    expect(document.querySelectorAll(".adk-approval-queue__item")).toHaveLength(1);

    clickButtonByText("全部批准");
    await flushRequests();

    expect(
      countApprovalActionCalls(fetchMock, "approval-duplicate", "approve"),
    ).toBe(1);
  });

  it("shows one authoritative approval card when parent and child timeline groups share an approval id", async () => {
    const childApproval = {
      ...buildApproval("approval-workflow-dup", "child-run"),
      reason: "child copy",
    };
    const parentApproval = {
      ...childApproval,
      reason: "parent copy",
    };

    mountADKPage({
      approvals: [childApproval],
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("approval_group", {
            id: "parent-approval-group",
            runId: "parent-run",
            approvals: [parentApproval],
            createdAt: "2026-06-06T00:00:01Z",
          }),
          buildTimelineEntry("approval_group", {
            id: "child-approval-group",
            runId: "child-run",
            approvals: [childApproval],
            createdAt: "2026-06-06T00:00:02Z",
          }),
        ],
      },
    });
    await flushRequests();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    await expandQueue("待审批");
    expect(document.querySelectorAll(".adk-approval-queue__item")).toHaveLength(1);
    expect(document.body.textContent).toContain("parent copy");
    expect(document.body.textContent).not.toContain("child copy");
  });

  it("ignores rapid duplicate clicks on the same approval", async () => {
    const approval = buildApproval("approval-fast-click", "run-fast-click");
    const run = buildRun({
      id: "run-fast-click",
      status: "PENDING_APPROVAL",
      pendingApprovals: [approval],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run,
        pendingApprovals: [approval],
        timeline: pendingApprovalTimeline(run, [approval], "fast approval"),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    const fetchMock = mountADKPage({
      approvals: [approval],
      approvalResolution: {
        approval: { ...approval, status: "APPROVED" },
        run: buildRun({
          id: "run-fast-click",
          status: "COMPLETED",
          pendingApprovals: [],
        }),
      },
    });
    await flushRequests();

    await sendPageMessage("fast approval");

    await expandQueue("待审批");
    const button = Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((candidate) => candidate.textContent?.includes("批准"));
    button?.click();
    button?.click();
    await flushRequests();

    expect(
      countApprovalActionCalls(fetchMock, "approval-fast-click", "approve"),
    ).toBe(1);
  });

  it("queues, revokes, and auto-dispatches messages while a blocking run is active", async () => {
    const pendingApproval = buildApproval("approval-queue", "run-queue");
    const pendingRun = buildRun({
      id: "run-queue",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall(
          "tool-queue",
          "run-queue",
          "strategy.save_draft",
          "PENDING_APPROVAL",
        ),
      ],
      pendingApprovals: [pendingApproval],
    });
    const completedRun = buildRun({
      id: "run-queue",
      status: "COMPLETED",
      toolCalls: [
        buildToolCall(
          "tool-queue",
          "run-queue",
          "strategy.save_draft",
          "SUCCEEDED",
        ),
      ],
      pendingApprovals: [],
    });
    const queuedRun = buildRun({
      id: "run-queue-2",
      status: "COMPLETED",
      userMessage: "queued follow-up",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting",
          session: buildSession(),
          run: pendingRun,
          pendingApprovals: [pendingApproval],
          timeline: pendingApprovalTimeline(
            pendingRun,
            [pendingApproval],
            "first request",
          ),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "queued done",
          session: buildSession(),
          run: queuedRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "queued-user",
              text: String(payload.message),
              createdAt: "2026-06-06T00:00:04Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "queued-answer",
              runId: queuedRun.id,
              text: "queued follow-up completed",
              createdAt: "2026-06-06T00:00:05Z",
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
        run: completedRun,
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("tool_group", {
            id: "entry-tools-done",
            runId: completedRun.id,
            toolCalls: completedRun.toolCalls,
            createdAt: "2026-06-06T00:00:02Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "entry-answer",
            runId: completedRun.id,
            text: "first request done",
            createdAt: "2026-06-06T00:00:03Z",
          }),
        ],
      },
    });
    await flushRequests();

    await sendPageMessage("first request");
    expect(document.querySelector("textarea")?.hasAttribute("disabled")).toBe(
      false,
    );

    await sendPageMessage("revoke me");
    expect(document.body.textContent).toContain("revoke me");
    clickButtonByText("撤回");
    await flushRequests();
    expect(document.body.textContent).not.toContain("revoke me");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);

    await sendPageMessage("queued follow-up");
    expect(document.body.textContent).toContain("queued follow-up");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();

    expect(streamADKChatMock).toHaveBeenCalledTimes(2);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "queued follow-up",
    });
    expect(document.body.textContent).toContain("queued follow-up completed");
  });

  it("interrupts the active run and sends the interrupt message before the rest of the queue", async () => {
    const pendingApproval = buildApproval(
      "approval-interrupt",
      "run-interrupt",
    );
    const pendingRun = buildRun({
      id: "run-interrupt",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall(
          "tool-interrupt",
          "run-interrupt",
          "strategy.save_draft",
          "PENDING_APPROVAL",
        ),
      ],
      pendingApprovals: [pendingApproval],
    });
    const cancelledRun = buildRun({
      id: "run-interrupt",
      status: "CANCELLED",
      pendingApprovals: [],
    });
    const urgentRun = buildRun({
      id: "run-urgent",
      status: "COMPLETED",
      userMessage: "urgent request",
    });
    const normalRun = buildRun({
      id: "run-normal",
      status: "COMPLETED",
      userMessage: "normal queued request",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting",
          session: buildSession(),
          run: pendingRun,
          pendingApprovals: [pendingApproval],
          timeline: pendingApprovalTimeline(
            pendingRun,
            [pendingApproval],
            "first request",
          ),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "urgent done",
          session: buildSession(),
          run: urgentRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "urgent-user",
              text: String(payload.message),
              createdAt: "2026-06-06T00:00:04Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "urgent-answer",
              runId: urgentRun.id,
              text: "urgent completed",
              createdAt: "2026-06-06T00:00:05Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "normal done",
          session: buildSession(),
          run: normalRun,
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "normal-user",
              text: String(payload.message),
              createdAt: "2026-06-06T00:00:06Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "normal-answer",
              runId: normalRun.id,
              text: "normal completed",
              createdAt: "2026-06-06T00:00:07Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    const fetchMock = mountADKPage({
      approvals: [pendingApproval],
      cancelRunById: {
        "run-interrupt": cancelledRun,
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "cancelled-answer",
            runId: cancelledRun.id,
            text: "first request cancelled",
            createdAt: "2026-06-06T00:00:03Z",
          }),
        ],
      },
    });
    await flushRequests();

    await sendPageMessage("first request");
    await sendPageMessage("normal queued request");
    expect(document.body.textContent).toContain("normal queued request");

    const textarea = document.querySelector("textarea")!;
    textarea.value = "urgent request";
    textarea.dispatchEvent(new Event("input"));
    await nextTick();
    clickButtonByText("打断后发送");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/runs/run-interrupt/cancel"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(streamADKChatMock).toHaveBeenCalledTimes(3);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "urgent request",
    });
    expect(streamADKChatMock.mock.calls[2]?.[0]).toMatchObject({
      message: "normal queued request",
    });
    expect(document.body.textContent).toContain("normal completed");
  });

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
    expect(
      document.querySelector<HTMLTextAreaElement>(
        ".adk-composer textarea, .adk-composer input",
      )?.disabled,
    ).toBe(false);
  });

  it("keeps deep reasoning collapsed until the user expands it", async () => {
    const reasoningText = "Detailed  chain of thought preview.\n  Preserve indentation.";
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

    expect(fetchMock.mock.calls.filter(([input]) =>
      String(input).includes(`/api/v1/adk/sessions/${session.id}/composer-state`),
    ).length).toBeGreaterThanOrEqual(2);
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

    document.querySelector<HTMLElement>('.adk-session-close[title="关闭会话"]')?.click();
    await flushRequests();

    const persisted = JSON.parse(
      window.localStorage.getItem("jftrade.adk.page-state.v1") ?? "{}",
    ) as { selectedSessionId?: string; sessions?: Record<string, unknown> };
    expect(persisted.selectedSessionId).toBe("");
    expect(persisted.sessions?.[session.id]).toBeUndefined();
  });

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

type SessionContextTestResponse =
  | ADKSessionContextSnapshot
  | null
  | Error
  | Promise<ADKSessionContextSnapshot | null>;

function mountADKPage(
  options: {
    providerHasKey?: boolean;
    agent?: Partial<ReturnType<typeof buildAgentBase>>;
    approvals?: ADKApproval[];
    approvalResolution?: unknown;
    approvalResolutionById?: Record<string, unknown>;
    cancelRunById?: Record<string, ADKRun>;
    runById?: Record<string, ADKRun>;
    sessions?: Array<ReturnType<typeof buildSession>>;
    composerStateBySession?: Record<string, ADKSessionComposerState>;
    composerStateSave?: (
      sessionId: string,
      patch: Partial<ADKSessionComposerState>,
    ) => Promise<ADKSessionComposerState>;
    sessionDetail?: {
      session: ReturnType<typeof buildSession>;
      timeline: ADKTimelineEntry[];
      runs?: ADKRun[];
      composerState?: ADKSessionComposerState;
    };
    sessionDetailSequence?: Array<{
      session: ReturnType<typeof buildSession>;
      timeline: ADKTimelineEntry[];
      runs?: ADKRun[];
      composerState?: ADKSessionComposerState;
    }>;
    sessionContext?: ADKSessionContextSnapshot | null;
    sessionContextSequence?: SessionContextTestResponse[];
  } = {},
) {
  document.body.innerHTML = "<div id='root'></div>";
  const state = {
    approvals: [...(options.approvals ?? [])],
    sessionDetailSequence: [
      ...(options.sessionDetailSequence ?? [
        options.sessionDetail ?? { session: buildSession(), timeline: [] },
      ]),
    ],
    sessionContextSequence: [
      ...(options.sessionContextSequence ?? [options.sessionContext ?? null]),
    ],
    composerStateBySession: { ...(options.composerStateBySession ?? {}) },
  };

  const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/api/v1/adk/agents")) {
      return createResponse({ agents: [buildAgent(options.agent)] });
    }
    if (url.includes("/api/v1/adk/providers")) {
      return createResponse({
        providers: [buildProvider(options.providerHasKey ?? true)],
      });
    }
    if (/\/api\/v1\/adk\/sessions\/[^/]+\/context$/.test(url)) {
      const context =
        state.sessionContextSequence.length > 1
          ? state.sessionContextSequence.shift()!
          : state.sessionContextSequence[0]!;
      const resolvedContext = await context;
      if (resolvedContext instanceof Error) {
        throw resolvedContext;
      }
      return createResponse(resolvedContext ?? null);
    }
    const composerStateMatch = url.match(
      /\/api\/v1\/adk\/sessions\/([^/]+)\/composer-state$/,
    );
    if (composerStateMatch) {
      const sessionId = decodeURIComponent(composerStateMatch[1]!);
      const patch = JSON.parse(String(init?.body ?? "{}")) as Partial<ADKSessionComposerState>;
      if (options.composerStateSave) {
        return createResponse(await options.composerStateSave(sessionId, patch));
      }
      state.composerStateBySession[sessionId] = buildComposerState(sessionId, {
        ...(state.composerStateBySession[sessionId] ?? {}),
        ...patch,
        updatedAt: "2026-06-06T00:00:10Z",
      });
      return createResponse(state.composerStateBySession[sessionId]);
    }
    if (url.includes("/api/v1/adk/sessions")) {
      if (init?.method === "DELETE") {
        return createResponse({});
      }
      if (/\/api\/v1\/adk\/sessions\/[^/]+$/.test(url)) {
        const detail =
          state.sessionDetailSequence.length > 1
            ? state.sessionDetailSequence.shift()!
            : state.sessionDetailSequence[0]!;
        return createResponse({
          ...detail,
          composerState:
            detail.composerState ??
            state.composerStateBySession[detail.session.id] ??
            buildComposerState(detail.session.id),
        });
      }
      return createResponse({ sessions: options.sessions ?? [buildSession()] });
    }
    const cancelRunMatch = url.match(/\/api\/v1\/adk\/runs\/([^/]+)\/cancel$/);
    if (cancelRunMatch) {
      const runId = decodeURIComponent(cancelRunMatch[1]!);
      return createResponse(
        options.cancelRunById?.[runId] ??
          buildRun({ id: runId, status: "CANCELLED", pendingApprovals: [] }),
      );
    }
    const runDetailMatch = url.match(/\/api\/v1\/adk\/runs\/([^/]+)$/);
    if (runDetailMatch) {
      const runId = decodeURIComponent(runDetailMatch[1]!);
      return createResponse(options.runById?.[runId] ?? {});
    }
    const approvalActionMatch = url.match(
      /\/api\/v1\/adk\/approvals\/([^/]+)\/(approve|deny)$/,
    );
    if (approvalActionMatch) {
      const approvalId = approvalActionMatch[1]!;
      state.approvals = state.approvals.filter(
        (approval) => approval.id !== approvalId,
      );
      if (options.approvalResolutionById?.[approvalId] !== undefined) {
        return createResponse(options.approvalResolutionById[approvalId]);
      }
      return createResponse(options.approvalResolution);
    }
    if (url.includes("/api/v1/adk/approvals")) {
      return createResponse({ approvals: state.approvals });
    }
    return createResponse({});
  });
  vi.stubGlobal("fetch", fetchMock);

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { template: "<div />" } }],
  });
  mount(ADKPage, {
    attachTo: "#root",
    global: {
      plugins: [router],
      stubs: vuetifyStubs(),
    },
  });
  return fetchMock;
}

function buildProvider(hasApiKey: boolean) {
  return {
    id: "provider-1",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    enabled: true,
    hasApiKey,
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildApproval(id: string, runId = "run-1"): ADKApproval {
  return {
    id,
    runId,
    agentId: "agent-1",
    toolName: "strategy.save_draft",
    input: { query: "@strategy.save_draft" },
    status: "PENDING",
    reason: "needs approval",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildAgent(
  overrides: Partial<ReturnType<typeof buildAgentBase>> = {},
) {
  return {
    ...buildAgentBase(),
    ...overrides,
  };
}

function buildAgentBase() {
  return {
    id: "agent-1",
    name: "投资分析助手",
    instruction: "test",
    providerId: "provider-1",
    model: "gpt-4o-mini",
    tools: ["strategy.save_draft"],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat",
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildSession(
  overrides: Partial<{
    id: string;
    agentId: string;
    title: string;
    createdAt: string;
    updatedAt: string;
  }> = {},
) {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "测试会话",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
    ...overrides,
  };
}

function buildComposerState(
  sessionId: string,
  overrides: Partial<ADKSessionComposerState> = {},
): ADKSessionComposerState {
  return {
    sessionId,
    chatDraft: "",
    workModeOverride: "",
    goalObjectiveDraft: "",
    goalObjectiveTouched: false,
    updatedAt: "2026-06-06T00:00:00Z",
    ...overrides,
  };
}

function buildSessionContextSnapshot(
  overrides: Partial<ADKSessionContextSnapshot> = {},
): ADKSessionContextSnapshot {
  const breakdown = {
    instructionTokens: 900,
    handoffTokens: 0,
    recentUserTokens: 1200,
    protectedTailTokens: 0,
    otherVisibleTokens: 1600,
    pendingUserTokens: 200,
    toolDeclarationTokens: 300,
  };
  return {
    sessionId: "session-1",
    currentInputTokens: 4200,
    projectedNextTurnTokens: 4300,
    rawCurrentInputTokens: 4200,
    rawProjectedNextTurnTokens: 4300,
    contextWindowTokens: 10000,
    usageRatio: 0.42,
    status: "healthy",
    recentUserWindow: 6,
    retainedRecentUserCount: 1,
    activeHandoffCount: 0,
    breakdown,
    rawBreakdown: breakdown,
    trimmedToolResponseCount: 0,
    autoCompacted: false,
    degradedSummary: false,
    ...overrides,
  };
}

function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
} {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function buildToolCall(
  id: string,
  runId: string,
  toolName: string,
  status: string,
) {
  return {
    id,
    runId,
    toolName,
    permission: "write_strategy",
    status,
    input: { toolName },
    requiresUser: status === "PENDING_APPROVAL",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

function buildRun(overrides: Partial<ADKRun>): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "completed",
    userMessage: "@strategy.save_draft 保存策略",
    toolSummaries: [],
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
    ...overrides,
  };
}

function buildWorkflowStep(
  taskId: string,
  title: string,
  status: string,
  childRunId?: string,
): NonNullable<ADKRun["workflowPlan"]>[number] {
  return {
    taskId,
    title,
    status,
    childRunId,
  };
}

function buildTimelineEntry(
  kind: ADKTimelineEntry["kind"],
  overrides: Partial<ADKTimelineEntry> = {},
): ADKTimelineEntry {
	return {
		id: overrides.id ?? `entry-${kind}`,
		sessionId: overrides.sessionId ?? "session-1",
		kind,
		createdAt: overrides.createdAt ?? "2026-06-06T00:00:00Z",
		updatedAt: overrides.updatedAt,
		sequence: overrides.sequence ?? 1,
		status: overrides.status ?? "final",
		runId: overrides.runId,
		text: overrides.text,
		originalText: overrides.originalText,
		processedText: overrides.processedText,
		toolCalls: overrides.toolCalls,
		approvals: overrides.approvals,
	};
}

function pendingApprovalTimeline(
  run: ADKRun,
  approvals: ADKApproval[],
  userText: string,
): ADKTimelineEntry[] {
  return [
    buildTimelineEntry("user_message", {
      id: `user-${run.id}`,
      text: userText,
      createdAt: "2026-06-06T00:00:00Z",
    }),
    buildTimelineEntry("tool_group", {
      id: `tools-${run.id}`,
      runId: run.id,
      toolCalls: run.toolCalls,
      createdAt: "2026-06-06T00:00:01Z",
    }),
    buildTimelineEntry("approval_group", {
      id: `approvals-${run.id}`,
      runId: run.id,
      approvals,
      createdAt: "2026-06-06T00:00:02Z",
    }),
  ];
}

async function sendPageMessage(text: string): Promise<void> {
  const textarea = document.querySelector("textarea")!;
  textarea.value = text;
  textarea.dispatchEvent(new Event("input"));
  await nextTick();
  document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
  await flushRequests();
}

function clickButtonByText(text: string): void {
  Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
    .find((button) => button.textContent?.includes(text))
    ?.click();
}

async function expandQueue(title: string): Promise<void> {
  const queue =
    document.querySelector<HTMLElement>(`[aria-label="${title}"]`) ??
    Array.from(
      document.querySelectorAll<HTMLElement>(".adk-workspace-queue"),
    ).find((candidate) => candidate.textContent?.includes(title));
  if (!queue || queue.querySelector(".adk-workspace-queue__body")) return;
  queue.querySelector<HTMLButtonElement>(".adk-workspace-queue__header")?.click();
  await nextTick();
}

function findWorkModeSelect(): HTMLSelectElement | undefined {
  return Array.from(document.querySelectorAll<HTMLSelectElement>("select")).find(
    (select) =>
      Array.from(select.options).some((option) => option.value === "loop"),
  );
}

function lastComposerStatePatch(
  fetchMock: ReturnType<typeof vi.fn>,
  sessionId: string,
): Record<string, unknown> | undefined {
  const calls = fetchMock.mock.calls.filter(([input]) =>
    String(input).includes(
      `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}/composer-state`,
    ),
  );
  const body = calls.at(-1)?.[1]?.body;
  return body == null ? undefined : JSON.parse(String(body));
}

function countApprovalActionCalls(
  fetchMock: ReturnType<typeof vi.fn>,
  approvalId: string,
  action: "approve" | "deny",
): number {
  return fetchMock.mock.calls.filter(([input]) =>
    String(input).includes(`/api/v1/adk/approvals/${approvalId}/${action}`),
  ).length;
}

function vuetifyStubs() {
  return {
    "v-alert": { template: "<div><slot /></div>" },
    "v-btn": {
      props: ["disabled", "loading"],
      emits: ["click"],
      template:
        "<button type='button' :disabled='disabled' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
    },
    "v-card": { template: "<div><slot /></div>" },
    "v-card-text": { template: "<div><slot /></div>" },
    "v-card-title": { template: "<div><slot /></div>" },
    "v-chip": { template: "<span><slot /></span>" },
    "v-expansion-panel": { template: "<div><slot /></div>" },
    "v-expansion-panel-text": { template: "<div><slot /></div>" },
    "v-expansion-panel-title": { template: "<div><slot /></div>" },
    "v-expansion-panels": { template: "<div><slot /></div>" },
    "v-icon": { template: "<span><slot /></span>" },
    "v-menu": {
      template: "<div><slot name='activator' :props='{}' /><slot /></div>",
    },
    "v-progress-circular": { template: "<span />" },
    "v-select": {
      props: ["modelValue", "items"],
      emits: ["update:modelValue"],
      template:
        "<select :value='modelValue' @change=\"$emit('update:modelValue', $event.target.value)\"><option v-for='item in items' :key='item.value' :value='item.value'>{{ item.title }}</option></select>",
    },
    "v-textarea": {
      props: ["modelValue", "disabled"],
      emits: ["update:modelValue"],
      template:
        "<textarea :value='modelValue' :disabled='disabled' :class='$attrs.class' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
    "v-text-field": {
      props: ["modelValue", "disabled"],
      emits: ["update:modelValue"],
      template:
        "<input :value='modelValue' :disabled='disabled' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
  };
}
