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

describe("ADKPage workflow, children, and context", () => {
  it("shows the latest workflow plan directly above the composer and clears it on the next run", async () => {
    const workflowRun = buildRun({
      id: "run-workflow-plan",
      status: "COMPLETED",
      workMode: "loop",
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
      workMode: "loop",
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
    const childTrace = document.querySelector(".adk-child-run-trace");
    expect(childTrace).not.toBeNull();
    const childQueue = document.querySelector('[aria-label="子智能体"]');
    expect(childQueue).not.toBeNull();
    expect(document.querySelector('[aria-label="执行计划"]')).not.toBeNull();
    expect(
      childTrace?.querySelector(".adk-status-pill.is-success"),
    ).not.toBeNull();
    expect(
      childTrace?.querySelector(".adk-status-pill.is-error"),
    ).toBeNull();
    expect(document.body.textContent).toContain("检查子智能体");
    expect(document.body.textContent).toContain("parent visible answer");
    expect(document.body.textContent).not.toContain("启动子智能体 #1");
    expect(document.body.textContent).not.toContain("子智能体 #1 已结束：已完成");
    expect(document.body.textContent).toContain("已完成");
    expect(childTrace?.textContent).not.toContain("运行中");
    expect(
      document.querySelector('[aria-label="执行计划"]')?.textContent,
    ).not.toContain("IN_PROGRESS");
    expect(document.body.textContent).not.toContain("child filtered answer");
    expect(document.body.textContent).not.toContain("strategy.inspect_child");
    expect(document.body.textContent).not.toContain(
      "strategy.parent_copy_child_result",
    );
    expect(document.body.textContent).not.toContain("child-only-success");

    await expandQueue("子智能体");
    clickButtonByText("进入");
    await nextTick();

    expect(document.body.textContent).toContain("子智能体 #1");
    expect(document.body.textContent).toContain("child-run-drilldown");
    expect(document.body.textContent).toContain("42% 正常");
    expect(document.body.textContent).not.toContain("15% 正常");
    expect(document.body.textContent).not.toContain("child filtered answer");

    document.querySelector<HTMLElement>(".adk-turn-trace__header")?.click();
    await nextTick();

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
    expect(document.body.textContent).not.toContain("启动子智能体 #1");
    expect(document.body.textContent).not.toContain("子智能体 #1 已结束：已完成");
    expect(document.body.textContent).toContain("检查子智能体");
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

  it("refreshes the context popover from the live session snapshot while a run is still active", async () => {
    const liveContext = buildSessionContextSnapshot({
      contextRevisionId: "ctx-live-during-run",
      contextRevisionCreatedAt: "2026-06-21T05:00:00Z",
      currentInputTokens: 67874,
      projectedNextTurnTokens: 67874,
      contextWindowTokens: 200000,
      usageRatio: 0.33937,
      activeHandoffCount: 2,
      compactedEventCount: 18,
      autoCompacted: true,
      summaryPreview: "运行中也应显示最新上下文",
    });
    const runningWorkflow = buildRun({
      id: "run-live-context",
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "RUNNING",
      objective: "持续推进中",
    });
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "still running",
        session: buildSession(),
        run: runningWorkflow,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "live-context-answer",
            runId: runningWorkflow.id,
            text: "still running",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "run", run: runningWorkflow });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      sessionContextSequence: [null, liveContext],
    });
    await flushRequests();

    await sendPageMessage("start long task");

    document
      .querySelector<HTMLButtonElement>(".adk-context-pill")
      ?.click();
    await flushRequests();

    expect(document.body.textContent).toContain("34% 正常");
    expect(document.body.textContent).toContain("200,000");
    expect(document.body.textContent).toContain("ctx-live-during-ru...");
    expect(document.body.textContent).toContain("运行中也应显示最新上下文");
  });

  it("marks failed child agent queue items as error instead of success", async () => {
    const workflowRun = buildRun({
      id: "parent-run-child-failed",
      status: "FAILED",
      workMode: "loop",
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

    const childTrace = document.querySelector(".adk-child-run-trace");
    expect(childTrace).not.toBeNull();
    await expandQueue("子智能体");
    const childQueue = document.querySelector('[aria-label="子智能体"]');
    expect(childQueue).not.toBeNull();
    expect(
      childTrace?.querySelector(".adk-status-pill.is-error"),
    ).not.toBeNull();
    expect(
      childTrace?.querySelector(".adk-status-pill.is-success"),
    ).toBeNull();
    expect(childTrace?.textContent).toContain("运行失败");
    expect(childQueue?.textContent).toContain("运行失败");
  });

  it("surfaces workflow provider failures in the conversation alert and child queue", async () => {
    const workflowRun = buildRun({
      id: "parent-run-provider-failed",
      status: "FAILED",
      workMode: "loop",
      workflowStatus: "FAILED",
      message:
        'provider returned 402: {"error":{"message":"Insufficient Balance"}}',
      failureReason:
        'provider returned 402: {"error":{"message":"Insufficient Balance"}}',
      errorCode: "MODEL_CALL_FAILED",
      childRunIds: ["child-run-provider-cancelled"],
      workflowPlan: [
        buildWorkflowStep(
          "step-provider-failed",
          "每日股票盘点",
          "IN_PROGRESS",
          "child-run-provider-cancelled",
        ),
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply:
          'provider returned 402: {"error":{"message":"Insufficient Balance"}}',
        session: buildSession(),
        run: workflowRun,
        pendingApprovals: [],
        timeline: [],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        "child-run-provider-cancelled": buildRun({
          id: "child-run-provider-cancelled",
          parentRunId: workflowRun.id,
          status: "CANCELLED",
          failureReason: "parent workflow parent-run-provider-failed terminated",
          errorCode: "PARENT_RUN_TERMINATED",
        }),
      },
    });
    await flushRequests();

    await sendPageMessage("run failed provider workflow");

    expect(document.body.textContent).toContain(
      "模型调用失败：服务商余额不足",
    );
    expect(document.body.textContent).not.toContain(
      "MODEL_CALL_FAILED",
    );
    document.querySelector<HTMLButtonElement>(".adk-inline-alert__toggle")?.click();
    await nextTick();
    expect(document.body.textContent).toContain(
      "MODEL_CALL_FAILED",
    );
    await expandQueue("子智能体");
    expect(document.querySelector('[aria-label="子智能体"]')).not.toBeNull();
    expect(document.querySelector(".adk-child-run-trace")?.textContent).toContain(
      "父工作流已终止，子智能体已取消",
    );
  });

  it("derives completed workflow state from terminal child snapshots", async () => {
    const workflowRun = buildRun({
      id: "parent-run-stale-running-child-complete",
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "RUNNING",
      childRunIds: ["child-run-complete"],
      workflowPlan: [
        buildWorkflowStep(
          "step-child-complete",
          "完成子智能体",
          "IN_PROGRESS",
          "child-run-complete",
        ),
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "parent still stale",
        session: buildSession(),
        run: workflowRun,
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "child-complete-answer",
            runId: "child-run-complete",
            text: "child completed answer",
            createdAt: "2026-06-06T00:00:03Z",
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
        "child-run-complete": buildRun({
          id: "child-run-complete",
          parentRunId: workflowRun.id,
          status: "COMPLETED",
        }),
      },
    });
    await flushRequests();

    await sendPageMessage("run stale parent workflow");

    const childTrace = document.querySelector(".adk-child-run-trace");
    expect(childTrace?.textContent).toContain("已完成");
    expect(childTrace?.textContent).not.toContain("运行中");
    expect(
      childTrace?.querySelector(".adk-status-pill.is-success"),
    ).not.toBeNull();
    expect(document.querySelector('[aria-label="子智能体"]')).not.toBeNull();
    expect(document.querySelector(".adk-composer-stop")).toBeNull();

    await expandQueue("子智能体");
    clickButtonByText("进入");
    await nextTick();

    expect(document.body.textContent).toContain("child completed answer");
    expect(document.body.textContent).not.toContain("正在运行");
  });

  it("does not show an empty approval queue for a blocked child without approvals", async () => {
    const workflowRun = buildRun({
      id: "parent-run-child-blocked",
      status: "RUNNING",
      workMode: "loop",
      childRunIds: ["child-run-blocked"],
      workflowPlan: [
        buildWorkflowStep(
          "step-child-blocked",
          "等待子智能体",
          "BLOCKED",
          "child-run-blocked",
        ),
      ],
    });
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "child blocked",
        session: buildSession(),
        run: workflowRun,
        pendingApprovals: [],
        timeline: [],
      };
      await onEvent({ type: "final", response });
      return response;
    });

    mountADKPage({
      runById: {
        "child-run-blocked": buildRun({
          id: "child-run-blocked",
          parentRunId: workflowRun.id,
          status: "PENDING_APPROVAL",
          pendingApprovals: [],
        }),
      },
    });
    await flushRequests();
    await sendPageMessage("run blocked child workflow");

    expect(document.querySelector('[aria-label="待审批"]')).toBeNull();
    expect(document.querySelector('[aria-label="子智能体"]')).not.toBeNull();
    expect(document.body.textContent).toContain("已阻断");
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
    const approval = buildApproval(
      "approval-child-pending",
      "child-run-approval",
    );
    const parentRun = buildRun({
      id: "parent-run-approval",
      status: "PENDING_APPROVAL",
      workMode: "loop",
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
    expect(document.body.textContent).not.toContain("启动子智能体 #1");
    expect(document.body.textContent).toContain("等待审批");
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
      workMode: "loop",
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
      workMode: "loop",
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

    expect(
      document.querySelector('[aria-label="执行计划"]')?.textContent,
    ).toContain("子级计划步骤");
    expect(
      document.querySelector('[aria-label="子智能体"]')?.textContent,
    ).toContain("子级计划步骤");
    expect(
      document.querySelector('[aria-label="执行计划"]')?.textContent,
    ).not.toContain("父级入口步骤");
    expect(
      document.querySelector('[aria-label="子智能体"]')?.textContent,
    ).not.toContain("父级入口步骤");
    expect(document.body.textContent).toContain("子级计划步骤");
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
    expect(
      document.querySelector(".adk-workflow-plan-panel")?.textContent,
    ).toContain("等待保存审批");

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();

    const panelText = document.querySelector(
      ".adk-workflow-plan-panel",
    )?.textContent;
    expect(panelText).toContain("保存审批已通过");
    expect(panelText).toContain("执行计划");
    expect(panelText).not.toContain("等待保存审批");
  });
});
