// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import type {
  ADKApproval,
  ADKApprovalResolution,
  ADKAgent,
  ADKChatResponse,
  ADKRun,
  ADKSessionComposerState,
  ADKSessionContextSnapshot,
  ADKTimelineEntry,
  ADKToolDescriptor,
} from "@/contracts";

import { resetADKApprovalInFlightForTest } from "../src/composables/adkApprovalResolution";
import AiAssistantPanel from "../src/layout/AiAssistantPanel.vue";
import { createResponse, flushRequests } from "./helpers";

const { monitorADKRunContinuationMock, streamADKChatMock } = vi.hoisted(() => ({
  monitorADKRunContinuationMock: vi.fn(),
  streamADKChatMock: vi.fn(),
}));

vi.mock("../src/composables/adkChatStream", async () => {
  const actual = await vi.importActual<
    typeof import("../src/composables/adkChatStream")
  >("../src/composables/adkChatStream");
  return {
    ...actual,
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
  monitorADKRunContinuationMock.mockReset();
  monitorADKRunContinuationMock.mockImplementation(async (run) => run);
});

describe("AiAssistantPanel", () => {
  it("renders the shared mobile workspace shell with a collapsed composer summary", async () => {
    mountPanel();
    await flushRequests();

    expect(document.querySelector(".adk-shell--mobile")).not.toBeNull();
    expect(document.querySelector(".adk-mobile-toolbar")).not.toBeNull();
    expect(document.querySelector(".adk-thread--mobile")).not.toBeNull();
    expect(document.querySelector(".adk-chat-thread--mobile")).not.toBeNull();
    expect(document.querySelector(".adk-empty--mobile")).not.toBeNull();
    expect(document.querySelector(".adk-composer--mobile")).not.toBeNull();
    expect(
      document.querySelector("[data-testid='adk-mobile-composer-summary']"),
    ).not.toBeNull();
    expect(document.querySelector(".adk-agent-select")).toBeNull();
    expect(document.querySelector(".adk-provider-select")).toBeNull();
    expect(document.body.textContent).toContain("开始与智能体对话");

    document
      .querySelector<HTMLButtonElement>("[data-testid='adk-mobile-composer-toggle']")
      ?.click();
    await nextTick();

    expect(document.querySelector(".adk-agent-select")).not.toBeNull();
    expect(document.querySelector(".adk-provider-select")).not.toBeNull();
  });

  it("opens the session list as an in-panel drawer and closes after selecting a session", async () => {
    mountPanel();
    await flushRequests();

    expect(
      document.querySelector("[data-testid='adk-mobile-session-drawer']"),
    ).toBeNull();
    expect(document.querySelector(".adk-mobile-sheet")).toBeNull();
    expect(document.querySelector(".adk-mobile-sheet-backdrop")).toBeNull();

    document
      .querySelector<HTMLButtonElement>("[data-testid='adk-mobile-sessions-toggle']")
      ?.click();
    await nextTick();

    expect(
      document.querySelector("[data-testid='adk-mobile-session-drawer']"),
    ).not.toBeNull();

    document.querySelector<HTMLElement>(".adk-session-item")?.click();
    await flushRequests();

    expect(
      document.querySelector("[data-testid='adk-mobile-session-drawer']"),
    ).toBeNull();
  });

  it("shows the run goal command for a user-paused dock goal", async () => {
    const pausedRun = buildRun({
      id: "run-dock-paused-goal",
      status: "PAUSED",
      workMode: "loop",
      objective: "侧栏暂停目标",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-09T00:00:10Z",
      pausedAt: "2026-06-09T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
      workflowPlan: [
        {
          title: "侧栏暂停目标",
          message: "等待继续",
          status: "TODO",
        },
      ],
    });
    const response: ADKChatResponse = {
      reply: "目标已暂停。",
      run: pausedRun,
      session: buildSession(),
      pendingApprovals: [],
      timeline: [
        buildTimelineEntry("assistant_message", {
          id: "dock-paused-answer",
          runId: pausedRun.id,
          text: "目标已暂停。",
        }),
      ],
    };
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });
    mountPanel();
    await flushRequests();

    await sendDockMessage("dock paused goal");

    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "侧栏暂停目标",
    );
    expect(
      document.querySelector<HTMLButtonElement>(
        ".adk-goal-editor__action[title='运行目标']",
      ),
    ).not.toBeNull();
  });

  it("resumes a dock user-paused goal without sending a chat message", async () => {
    const pausedRun = buildRun({
      id: "run-dock-resume-goal",
      status: "PAUSED",
      workMode: "loop",
      objective: "侧栏恢复目标",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-09T00:00:10Z",
      pausedAt: "2026-06-09T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
      workflowPlan: [
        {
          title: "侧栏恢复目标",
          message: "等待继续",
          status: "TODO",
        },
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
    const response: ADKChatResponse = {
      reply: "目标已暂停。",
      run: pausedRun,
      session: buildSession(),
      pendingApprovals: [],
      timeline: [],
    };
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });
    const fetchMock = mountPanel({
      resumeRunById: { [pausedRun.id]: runningRun },
    });
    await flushRequests();
    await sendDockMessage("dock resume goal");
    streamADKChatMock.mockClear();

    document
      .querySelector<HTMLButtonElement>(
        ".adk-goal-editor__action[title='运行目标']",
      )
      ?.click();
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

  it("clears the dock goal editor after a resumed loop goal completes", async () => {
    const pausedRun = buildRun({
      id: "run-dock-complete-goal",
      status: "PAUSED",
      workMode: "loop",
      objective: "侧栏目标完成后清空",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-09T00:00:10Z",
      pausedAt: "2026-06-09T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
      workflowPlan: [
        {
          title: "侧栏目标完成后清空",
          message: "等待继续",
          status: "TODO",
        },
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
    const completedRun = buildRun({
      ...runningRun,
      status: "COMPLETED",
      workflowStatus: "COMPLETED",
      workflowPlan: [
        {
          title: "侧栏目标完成后清空",
          message: "目标已完成",
          status: "DONE",
        },
      ],
    });
    const response: ADKChatResponse = {
      reply: "目标已暂停。",
      run: pausedRun,
      session: buildSession(),
      pendingApprovals: [],
      timeline: [],
    };
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });
    monitorADKRunContinuationMock.mockImplementationOnce(
      async (run, options) => {
        await options?.onTerminal?.(completedRun);
        return completedRun;
      },
    );
    mountPanel({
      resumeRunById: { [pausedRun.id]: runningRun },
    });
    await flushRequests();

    await sendDockMessage("dock complete goal");

    expect(document.querySelector(".adk-goal-editor")?.textContent).toContain(
      "侧栏目标完成后清空",
    );

    document
      .querySelector<HTMLButtonElement>(
        ".adk-goal-editor__action[title='运行目标']",
      )
      ?.click();
    await flushRequests();

    expect(document.querySelector(".adk-goal-editor")).toBeNull();
  });

  it("refreshes dock approval state to RUNNING and keeps the composer editable", async () => {
    const approval = buildApproval("approval-1", "run-dock");
    const pendingRun = buildRun({
      id: "run-dock",
      status: "PENDING_APPROVAL",
      toolCalls: [
        buildToolCall(
          "tool-dock",
          "run-dock",
          "strategy.save_draft",
          "PENDING_APPROVAL",
        ),
      ],
      pendingApprovals: [approval],
    });
    const runningRun = buildRun({
      id: "run-dock",
      status: "RUNNING",
      toolCalls: [
        buildToolCall(
          "tool-dock",
          "run-dock",
          "strategy.save_draft",
          "RUNNING",
        ),
      ],
      pendingApprovals: [],
    });
    const completedRun = buildRun({
      id: "run-dock",
      status: "COMPLETED",
      toolCalls: [
        buildToolCall(
          "tool-dock",
          "run-dock",
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
        run: pendingRun,
        session: buildSession(),
        pendingApprovals: [approval],
        timeline: pendingApprovalTimeline(
          pendingRun,
          [approval],
          "dock approval",
        ),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountPanel({
      approvals: [approval],
      approvalResolutionById: {
        "approval-1": {
          approval: { ...approval, status: "APPROVED" },
          run: runningRun,
        },
      },
      sessionDetailSequence: [
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "dock-running-tools",
              runId: runningRun.id,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-09T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "dock-running-tools-2",
              runId: runningRun.id,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-09T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: [
            buildTimelineEntry("tool_group", {
              id: "dock-completed-tools",
              runId: completedRun.id,
              toolCalls: completedRun.toolCalls,
              createdAt: "2026-06-09T00:00:02Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "dock-answer",
              runId: completedRun.id,
              text: "dock approval completed",
              createdAt: "2026-06-09T00:00:03Z",
            }),
          ],
        },
      ],
    });
    await flushRequests();

    await sendDockMessage("dock approval");

    expect(document.body.textContent).toContain("PENDING_APPROVAL");
    const composerInput =
      document.querySelector("textarea") ??
      document.querySelector("input");
    expect(composerInput?.hasAttribute("disabled")).toBe(false);

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();

    expect(document.querySelector(".adk-run-spinner")).not.toBeNull();
    expect(document.querySelector(".adk-approvals-approve-all")).toBeNull();
    expect(composerInput?.hasAttribute("disabled")).toBe(false);
    expect(document.body.textContent).not.toContain("dock approval completed");

    finishContinuation();
    await flushRequests();

    expect(document.body.textContent).toContain("dock approval completed");
  });

  it("shows a second dock approval produced during continuation without refreshing", async () => {
    const runId = "run-dock-second-approval";
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
      reason: "dock second approval required",
      input: { prompt: "dock-second-draft" },
      createdAt: "2026-06-09T00:00:04Z",
      updatedAt: "2026-06-09T00:00:04Z",
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
      updatedAt: "2026-06-09T00:00:04Z",
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
        run: firstPendingRun,
        session: buildSession(),
        pendingApprovals: [firstApproval],
        timeline: pendingApprovalTimeline(
          firstPendingRun,
          [firstApproval],
          "dock first approval",
        ),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountPanel({
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
              id: "dock-tools-running",
              runId,
              toolCalls: runningRun.toolCalls,
              createdAt: "2026-06-09T00:00:02Z",
            }),
          ],
        },
        {
          session: buildSession(),
          timeline: pendingApprovalTimeline(
            secondPendingRun,
            [secondApproval],
            "dock first approval",
          ),
        },
      ],
    });
    await flushRequests();

    await sendDockMessage("dock first approval");
    await sendDockMessage("dock queued follow-up");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();
    await flushRequests();
    await expandQueue("待审批");

    expect(document.body.textContent).toContain("dock second approval required");
    expect(document.body.textContent).toContain("dock-second-draft");
    expect(document.body.textContent).toContain("dock queued follow-up");
    expect(streamADKChatMock).toHaveBeenCalledTimes(1);
  });

  it("opens a dock child agent filtered view from the child run queue", async () => {
    const workflowRun = buildRun({
      id: "dock-parent-run",
      status: "COMPLETED",
      workMode: "task",
      childRunIds: ["dock-child-run"],
      workflowPlan: [
        {
          taskId: "dock-step-child",
          title: "侧栏子智能体",
          status: "DONE",
          childRunId: "dock-child-run",
        },
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "dock parent done",
        run: workflowRun,
        session: buildSession(),
        context: buildSessionContextSnapshot(),
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "dock-parent-answer",
            runId: workflowRun.id,
            text: "dock parent visible answer",
            createdAt: "2026-06-09T00:00:02Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "dock-child-answer",
            runId: "dock-child-run",
            text: "dock child filtered answer",
            createdAt: "2026-06-09T00:00:03Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "dock-child-tools",
            runId: "dock-child-run",
            toolCalls: [
              buildToolCall(
                "dock-child-tool",
                "dock-child-run",
                "strategy.inspect_dock_child",
                "SUCCEEDED",
              ),
            ],
            createdAt: "2026-06-09T00:00:04Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "dock-parent-copy-child-tools",
            runId: workflowRun.id,
            toolCalls: [
              {
                ...buildToolCall(
                  "dock-parent-copy-child-tool",
                  "dock-child-run",
                  "strategy.inspect_dock_parent_copy_child",
                  "SUCCEEDED",
                ),
                output: { result: "dock-child-only-success" },
              },
            ],
            createdAt: "2026-06-09T00:00:05Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountPanel({
      runById: {
        "dock-child-run": buildRun({
          id: "dock-child-run",
          parentRunId: workflowRun.id,
          status: "RUNNING",
          usage: { tokensIn: 1200, tokensOut: 300 },
        }),
      },
    });
    await flushRequests();

    await sendDockMessage("dock child workflow");
    expect(document.body.textContent).toContain("42% 正常");
    await expandQueue("子智能体");
    const childQueue = document.querySelector('[aria-label="子智能体"]');
    expect(childQueue).not.toBeNull();
    expect(document.querySelector('[aria-label="执行计划"]')).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue__badge.is-success")).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue-status.is-success")).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue__badge.is-error")).toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue-status.is-error")).toBeNull();
    expect(document.body.textContent).toContain("侧栏子智能体");
    expect(document.body.textContent).toContain("dock parent visible answer");
    expect(document.body.textContent).toContain("启动子智能体 #1");
    expect(document.body.textContent).toContain("子智能体 #1 已结束：已完成");
    expect(
      document.querySelector('[aria-label="子智能体"]')?.textContent,
    ).not.toContain("运行中");
    expect(
      document.querySelector('[aria-label="执行计划"]')?.textContent,
    ).not.toContain("IN_PROGRESS");
    expect(document.body.textContent).not.toContain("dock child filtered answer");
    expect(document.body.textContent).not.toContain("strategy.inspect_dock_child");
    expect(document.body.textContent).not.toContain(
      "strategy.inspect_dock_parent_copy_child",
    );
    expect(document.body.textContent).not.toContain("dock-child-only-success");

    clickButtonByText("进入");
    await nextTick();

    expect(document.body.textContent).toContain("子智能体 #1");
    expect(document.body.textContent).toContain("dock-child-run");
    expect(document.body.textContent).toContain("42% 正常");
    expect(document.body.textContent).not.toContain("15% 正常");
    expect(document.body.textContent).toContain("dock child filtered answer");
    expect(document.body.textContent).toContain("strategy.inspect_dock_child");
    expect(document.body.textContent).not.toContain(
      "strategy.inspect_dock_parent_copy_child",
    );
    expect(document.body.textContent).not.toContain("dock parent visible answer");
    expect(document.querySelector('[aria-label="子智能体"]')).toBeNull();
    expect(document.querySelector('[aria-label="执行计划"]')).toBeNull();

    clickButtonByText("返回父对话");
    await nextTick();

    expect(document.body.textContent).toContain("dock parent visible answer");
    expect(document.body.textContent).toContain("42% 正常");
    expect(document.body.textContent).not.toContain("15% 正常");
    expect(document.body.textContent).toContain("启动子智能体 #1");
    expect(document.body.textContent).toContain("子智能体 #1 已结束：已完成");
    expect(document.body.textContent).not.toContain("dock child filtered answer");
    expect(document.body.textContent).not.toContain("strategy.inspect_dock_child");
    expect(document.body.textContent).not.toContain(
      "strategy.inspect_dock_parent_copy_child",
    );
    expect(document.body.textContent).not.toContain("dock-child-only-success");
    expect(document.querySelector('[aria-label="子智能体"]')).not.toBeNull();
    expect(document.querySelector('[aria-label="执行计划"]')).not.toBeNull();
  });

  it("marks failed dock child agent queue items as error instead of success", async () => {
    const workflowRun = buildRun({
      id: "dock-parent-child-failed",
      status: "FAILED",
      workMode: "task",
      childRunIds: ["dock-child-failed"],
      workflowPlan: [
        {
          taskId: "dock-step-child-failed",
          title: "侧栏失败子智能体",
          status: "BLOCKED",
          childRunId: "dock-child-failed",
        },
      ],
    });

    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "dock parent failed",
        run: workflowRun,
        session: buildSession(),
        pendingApprovals: [],
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "dock-parent-failed-answer",
            runId: workflowRun.id,
            text: "dock parent failed answer",
            createdAt: "2026-06-09T00:00:02Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountPanel({
      runById: {
        "dock-child-failed": buildRun({
          id: "dock-child-failed",
          parentRunId: workflowRun.id,
          status: "FAILED",
        }),
      },
    });
    await flushRequests();

    await sendDockMessage("dock failed child workflow");
    await expandQueue("子智能体");

    const childQueue = document.querySelector('[aria-label="子智能体"]');
    expect(childQueue).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue__badge.is-error")).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue-status.is-error")).not.toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue__badge.is-success")).toBeNull();
    expect(childQueue?.querySelector(".adk-workspace-queue-status.is-success")).toBeNull();
  });

  it("queues dock messages and auto-dispatches them after the blocking run completes", async () => {
    const approval = buildApproval("approval-queue", "run-queue");
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
      pendingApprovals: [approval],
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
      id: "run-queue-next",
      status: "COMPLETED",
      userMessage: "dock follow-up",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting",
          run: pendingRun,
          session: buildSession(),
          pendingApprovals: [approval],
          timeline: pendingApprovalTimeline(
            pendingRun,
            [approval],
            "dock first",
          ),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "queued sent",
          run: queuedRun,
          session: buildSession(),
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "dock-queued-user",
              text: String(payload.message),
              createdAt: "2026-06-09T00:00:04Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "dock-queued-answer",
              runId: queuedRun.id,
              text: "dock follow-up completed",
              createdAt: "2026-06-09T00:00:05Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    mountPanel({
      approvals: [approval],
      approvalResolutionById: {
        "approval-queue": {
          approval: { ...approval, status: "APPROVED" },
          run: completedRun,
        },
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "dock-complete",
            runId: completedRun.id,
            text: "dock first completed",
            createdAt: "2026-06-09T00:00:03Z",
          }),
        ],
      },
    });
    await flushRequests();

    await sendDockMessage("dock first");
    await sendDockMessage("dock follow-up");

    expect(streamADKChatMock).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("dock follow-up");

    await expandQueue("待审批");
    clickButtonByText("批准");
    await flushRequests();

    expect(streamADKChatMock).toHaveBeenCalledTimes(2);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "dock follow-up",
    });
    expect(document.body.textContent).toContain("dock follow-up completed");
  });

  it("cancels the current dock run and sends the interrupt message before the rest of the queue", async () => {
    const approval = buildApproval("approval-interrupt", "run-interrupt");
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
      pendingApprovals: [approval],
    });
    const cancelledRun = buildRun({
      id: "run-interrupt",
      status: "CANCELLED",
      pendingApprovals: [],
    });
    const urgentRun = buildRun({
      id: "run-urgent",
      status: "COMPLETED",
      userMessage: "dock urgent",
    });
    const normalRun = buildRun({
      id: "run-normal",
      status: "COMPLETED",
      userMessage: "dock normal",
    });

    streamADKChatMock
      .mockImplementationOnce(async (_payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "waiting",
          run: pendingRun,
          session: buildSession(),
          pendingApprovals: [approval],
          timeline: pendingApprovalTimeline(
            pendingRun,
            [approval],
            "dock first",
          ),
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      })
      .mockImplementationOnce(async (payload, onEvent) => {
        const response: ADKChatResponse = {
          reply: "urgent done",
          run: urgentRun,
          session: buildSession(),
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "dock-urgent-user",
              text: String(payload.message),
              createdAt: "2026-06-09T00:00:04Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "dock-urgent-answer",
              runId: urgentRun.id,
              text: "dock urgent completed",
              createdAt: "2026-06-09T00:00:05Z",
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
          run: normalRun,
          session: buildSession(),
          pendingApprovals: [],
          timeline: [
            buildTimelineEntry("user_message", {
              id: "dock-normal-user",
              text: String(payload.message),
              createdAt: "2026-06-09T00:00:06Z",
            }),
            buildTimelineEntry("assistant_message", {
              id: "dock-normal-answer",
              runId: normalRun.id,
              text: "dock normal completed",
              createdAt: "2026-06-09T00:00:07Z",
            }),
          ],
        };
        await onEvent({ type: "session", session: response.session });
        await onEvent({ type: "final", response });
        return response;
      });

    const fetchMock = mountPanel({
      approvals: [approval],
      cancelRunById: {
        "run-interrupt": cancelledRun,
      },
      sessionDetail: {
        session: buildSession(),
        timeline: [
          buildTimelineEntry("assistant_message", {
            id: "dock-cancelled",
            runId: cancelledRun.id,
            text: "dock first cancelled",
            createdAt: "2026-06-09T00:00:03Z",
          }),
        ],
      },
    });
    await flushRequests();

    await sendDockMessage("dock first");
    await sendDockMessage("dock normal");

    const input = (document.querySelector("textarea") ??
      document.querySelector("input")) as
      | HTMLTextAreaElement
      | HTMLInputElement
      | null;
    if (!input) {
      throw new Error("chat input not found");
    }
    input.value = "dock urgent";
    input.dispatchEvent(new Event("input"));
    await nextTick();
    clickButtonByText("打断后发送");
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/adk/runs/run-interrupt/cancel"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(streamADKChatMock).toHaveBeenCalledTimes(3);
    expect(streamADKChatMock.mock.calls[1]?.[0]).toMatchObject({
      message: "dock urgent",
    });
    expect(streamADKChatMock.mock.calls[2]?.[0]).toMatchObject({
      message: "dock normal",
    });
    expect(document.body.textContent).toContain("dock normal completed");
  });

  it("keeps rendering when the final response contains null ADK arrays", async () => {
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "你好！我是JFTrade投资分析助手。",
        session: buildSession(),
        run: {
          ...buildRun({ id: "run-null-arrays", status: "COMPLETED" }),
          toolCalls: null as unknown as ADKRun["toolCalls"],
          pendingApprovals: null as unknown as ADKRun["pendingApprovals"],
        },
        pendingApprovals: null as unknown as ADKApproval[],
        timeline: [
          buildTimelineEntry("assistant_reasoning", {
            id: "dock-reasoning",
            text: "深度思考内容",
            createdAt: "2026-06-09T00:00:01Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "dock-null-tools",
            runId: "run-null-arrays",
            toolCalls: null as unknown as ADKTimelineEntry["toolCalls"],
            createdAt: "2026-06-09T00:00:02Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "dock-null-answer",
            runId: "run-null-arrays",
            text: "你好！我是JFTrade投资分析助手。",
            createdAt: "2026-06-09T00:00:03Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountPanel();
    await flushRequests();

    await sendDockMessage("你好");

    expect(document.body.textContent).toContain("查看深度思考");
    expect(document.body.textContent).toContain(
      "你好！我是JFTrade投资分析助手。",
    );
    expect(document.body.textContent).not.toContain(
      "Cannot read properties of null",
    );
  });

  it("treats failed final runs as terminal responses instead of stream errors", async () => {
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "侧栏本地兜底回复。",
        session: buildSession(),
        run: buildRun({
          id: "run-dock-failed-final",
          status: "FAILED",
          message: "disk full",
          failureReason: "disk full",
          errorCode: "TOOL_EXECUTION_FAILED",
          toolCalls: [
            {
              ...buildToolCall(
                "tool-dock-failed",
                "run-dock-failed-final",
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
            id: "dock-user-failed",
            runId: "run-dock-failed-final",
            text: "dock failed run",
            createdAt: "2026-06-09T00:00:00Z",
          }),
          buildTimelineEntry("tool_group", {
            id: "dock-tool-failed",
            runId: "run-dock-failed-final",
            toolCalls: [
              {
                ...buildToolCall(
                  "tool-dock-failed",
                  "run-dock-failed-final",
                  "strategy.save_draft",
                  "FAILED",
                ),
                error: "disk full",
              },
            ],
            createdAt: "2026-06-09T00:00:01Z",
          }),
          buildTimelineEntry("assistant_message", {
            id: "dock-answer-failed",
            runId: "run-dock-failed-final",
            text: "侧栏本地兜底回复。",
            createdAt: "2026-06-09T00:00:02Z",
          }),
        ],
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    mountPanel();
    await flushRequests();

    await sendDockMessage("dock failed run");

    expect(document.body.textContent).toContain("侧栏本地兜底回复。");
    expect(document.body.textContent).toContain("disk full");
    expect(
      document.querySelector<HTMLInputElement>("input, textarea")?.disabled,
    ).toBe(false);
  });
});

function mountPanel(
  options: {
    approvals?: ADKApproval[];
    approvalResolutionById?: Record<string, ADKApprovalResolution>;
    cancelRunById?: Record<string, ADKRun>;
    pauseRunById?: Record<string, ADKRun>;
    resumeRunById?: Record<string, ADKRun>;
    runById?: Record<string, ADKRun>;
    sessionDetail?: {
      session: ReturnType<typeof buildSession>;
      timeline: ADKTimelineEntry[];
    };
    sessionDetailSequence?: Array<{
      session: ReturnType<typeof buildSession>;
      timeline: ADKTimelineEntry[];
    }>;
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
    composerStateBySession: {} as Record<string, ADKSessionComposerState>,
  };

  const fetchMock = vi.fn(
    async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/api/v1/adk/agents")) {
        return createResponse({ agents: [buildAgent()] });
      }
      if (url.includes("/api/v1/adk/providers")) {
        return createResponse({ providers: [buildProvider()] });
      }
      if (url.includes("/api/v1/adk/tools")) {
        return createResponse({ tools: [buildToolDescriptor()] });
      }
      if (/\/api\/v1\/adk\/sessions\/[^/]+\/context$/.test(url)) {
        return createResponse(null);
      }
      const composerStateMatch = url.match(
        /\/api\/v1\/adk\/sessions\/([^/]+)\/composer-state$/,
      );
      if (composerStateMatch) {
        const sessionId = decodeURIComponent(composerStateMatch[1]!);
        const patch = JSON.parse(
          String(init?.body ?? "{}"),
        ) as Partial<ADKSessionComposerState>;
        state.composerStateBySession[sessionId] = buildComposerState(sessionId, {
          ...(state.composerStateBySession[sessionId] ?? {}),
          ...patch,
          updatedAt: "2026-06-09T00:00:10Z",
        });
        return createResponse(state.composerStateBySession[sessionId]);
      }
      const approvalActionMatch = url.match(
        /\/api\/v1\/adk\/approvals\/([^/]+)\/(approve|deny)$/,
      );
      if (approvalActionMatch) {
        const approvalId = approvalActionMatch[1]!;
        state.approvals = state.approvals.filter(
          (approval) => approval.id !== approvalId,
        );
        return createResponse(
          options.approvalResolutionById?.[approvalId] ?? {
            approval: {
              ...buildApproval(approvalId, "run-1"),
              status: init?.method === "POST" ? "APPROVED" : "DENIED",
            },
            run: buildRun({ status: "COMPLETED", pendingApprovals: [] }),
          },
        );
      }
      if (url.includes("/api/v1/adk/approvals")) {
        return createResponse({ approvals: state.approvals });
      }
      const cancelRunMatch = url.match(
        /\/api\/v1\/adk\/runs\/([^/]+)\/cancel$/,
      );
      if (cancelRunMatch) {
        const runId = decodeURIComponent(cancelRunMatch[1]!);
        return createResponse(
          options.cancelRunById?.[runId] ??
            buildRun({ id: runId, status: "CANCELLED", pendingApprovals: [] }),
        );
      }
      const pauseRunMatch = url.match(/\/api\/v1\/adk\/runs\/([^/]+)\/pause$/);
      if (pauseRunMatch) {
        const runId = decodeURIComponent(pauseRunMatch[1]!);
        return createResponse(
          options.pauseRunById?.[runId] ??
            buildRun({
              id: runId,
              status: "RUNNING",
              pauseRequestedAt: "2026-06-09T00:00:10Z",
            }),
        );
      }
      const resumeRunMatch = url.match(
        /\/api\/v1\/adk\/runs\/([^/]+)\/resume$/,
      );
      if (resumeRunMatch) {
        const runId = decodeURIComponent(resumeRunMatch[1]!);
        return createResponse(
          options.resumeRunById?.[runId] ??
            buildRun({
              id: runId,
              status: "RUNNING",
              resumeState: "user_resuming",
            }),
        );
      }
      const runDetailMatch = url.match(/\/api\/v1\/adk\/runs\/([^/]+)$/);
      if (runDetailMatch) {
        const runId = decodeURIComponent(runDetailMatch[1]!);
        return createResponse(options.runById?.[runId] ?? {});
      }
      if (url.includes("/api/v1/adk/sessions")) {
        if (/\/api\/v1\/adk\/sessions\/[^/]+$/.test(url)) {
          const detail =
            state.sessionDetailSequence.length > 1
              ? state.sessionDetailSequence.shift()!
              : state.sessionDetailSequence[0]!;
          return createResponse({
            ...detail,
            composerState:
              state.composerStateBySession[detail.session.id] ??
              buildComposerState(detail.session.id),
          });
        }
        return createResponse({
          sessions: [buildSession()],
        });
      }
      return createResponse({});
    },
  );
  vi.stubGlobal("fetch", fetchMock);

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { template: "<div />" } }],
  });

  mount(AiAssistantPanel, {
    attachTo: "#root",
    global: {
      plugins: [router],
      stubs: vuetifyStubs(),
    },
  });

  return fetchMock;
}

function buildApproval(id: string, runId: string): ADKApproval {
  return {
    id,
    runId,
    agentId: "agent-1",
    toolName: "strategy.save_draft",
    input: { prompt: "save" },
    status: "PENDING",
    reason: "Needs review",
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
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
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
}

function buildRun(overrides: Partial<ADKRun>): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "completed",
    userMessage: "dock message",
    toolSummaries: [],
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
    ...overrides,
  };
}

function buildAgent(
  overrides: Partial<ADKAgent> = {},
): ADKAgent {
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
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
    ...overrides,
  };
}

function buildProvider() {
  return {
    id: "provider-1",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    enabled: true,
    hasApiKey: true,
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
}

function buildSession() {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "Dock Session",
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
}

function buildComposerState(
  sessionId: string,
  overrides: Partial<ADKSessionComposerState> = {},
): ADKSessionComposerState {
  return {
    sessionId,
    chatDraft: "",
    providerIdOverride: "",
    modelOverride: "",
    workModeOverride: "",
    permissionModeOverride: "",
    goalObjectiveDraft: "",
    goalObjectiveTouched: false,
    updatedAt: "2026-06-09T00:00:00Z",
    ...overrides,
  };
}

function buildToolDescriptor(): ADKToolDescriptor {
  return {
    name: "strategy.save_draft",
    displayName: "Save Draft",
    description: "save draft",
    category: "strategy",
    permission: "write_strategy",
    allowedModes: ["approval", "less_approval", "all"],
    requiresApprovalIn: ["approval"],
    inputSchema: {},
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

function buildTimelineEntry(
  kind: ADKTimelineEntry["kind"],
  overrides: Partial<ADKTimelineEntry> = {},
): ADKTimelineEntry {
  return {
    id: overrides.id ?? `entry-${kind}`,
    sessionId: overrides.sessionId ?? "session-1",
    kind,
    createdAt: overrides.createdAt ?? "2026-06-09T00:00:00Z",
    sequence: overrides.sequence ?? 1,
    status: overrides.status ?? "final",
    runId: overrides.runId,
    text: overrides.text,
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
      createdAt: "2026-06-09T00:00:00Z",
    }),
    buildTimelineEntry("tool_group", {
      id: `tools-${run.id}`,
      runId: run.id,
      toolCalls: run.toolCalls,
      createdAt: "2026-06-09T00:00:01Z",
    }),
    buildTimelineEntry("approval_group", {
      id: `approvals-${run.id}`,
      runId: run.id,
      approvals,
      createdAt: "2026-06-09T00:00:02Z",
    }),
  ];
}

async function sendDockMessage(text: string): Promise<void> {
  const input =
    document.querySelector("textarea") ??
    document.querySelector("input");
  if (!(input instanceof HTMLTextAreaElement || input instanceof HTMLInputElement)) {
    throw new Error("chat input not found");
  }
  input.value = text;
  input.dispatchEvent(new Event("input"));
  await nextTick();
  document
    .querySelector<HTMLButtonElement>(".adk-composer-send")
    ?.click();
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

function vuetifyStubs() {
  return {
    "v-alert": {
      emits: ["click:close"],
      template: "<div :class='$attrs.class'><slot /></div>",
    },
    "v-btn": {
      props: ["disabled", "loading"],
      emits: ["click"],
      template:
        "<button type='button' :disabled='disabled' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
    },
    "v-chip": {
      emits: ["click"],
      template:
        "<button type='button' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
    },
    "v-card": { template: "<div><slot /></div>" },
    "v-card-text": { template: "<div><slot /></div>" },
    "v-card-title": { template: "<div><slot /></div>" },
    "v-icon": { template: "<span><slot /></span>" },
    "v-menu": {
      template: "<div><slot name='activator' :props='{}' /><slot /></div>",
    },
    "v-progress-circular": { template: "<span />" },
    "v-progress-linear": { template: "<span />" },
    "v-list-item": { template: "<div><slot /></div>" },
    "v-select": {
      props: ["modelValue", "items"],
      emits: ["update:modelValue"],
      template:
        "<select :value='modelValue' :class='$attrs.class' @change=\"$emit('update:modelValue', $event.target.value)\"><option v-for='item in items' :key='item.value' :value='item.value'>{{ item.title }}</option></select>",
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
        "<input :value='modelValue' :disabled='disabled' :class='$attrs.class' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
  };
}
