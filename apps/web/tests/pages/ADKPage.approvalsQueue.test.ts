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

describe("ADKPage approvals and queued messages", () => {
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
        buildToolCall(
          "tool-first",
          runId,
          "strategy.save_draft",
          "PENDING_APPROVAL",
        ),
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
        buildToolCall(
          "tool-second",
          runId,
          "strategy.save_draft",
          "PENDING_APPROVAL",
        ),
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
    expect(document.querySelectorAll(".adk-approval-queue__item")).toHaveLength(
      1,
    );

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
    expect(document.querySelectorAll(".adk-approval-queue__item")).toHaveLength(
      1,
    );
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
    const button = Array.from(
      document.querySelectorAll<HTMLButtonElement>("button"),
    ).find((candidate) => candidate.textContent?.includes("批准"));
    button?.click();
    button?.click();
    await flushRequests();

    expect(
      countApprovalActionCalls(fetchMock, "approval-fast-click", "approve"),
    ).toBe(1);
  });

  it("hides an approval immediately while the request is in flight", async () => {
    const approval = buildApproval("approval-optimistic", "run-optimistic");
    const run = buildRun({
      id: "run-optimistic",
      status: "PENDING_APPROVAL",
      pendingApprovals: [approval],
    });
    streamADKChatMock.mockImplementationOnce(async (_payload, onEvent) => {
      const response: ADKChatResponse = {
        reply: "waiting",
        session: buildSession(),
        run,
        pendingApprovals: [approval],
        timeline: pendingApprovalTimeline(run, [approval], "optimistic approval"),
      };
      await onEvent({ type: "session", session: response.session });
      await onEvent({ type: "final", response });
      return response;
    });

    let finishApproval!: (value: unknown) => void;
    const approvalResponse = new Promise<unknown>((resolve) => {
      finishApproval = resolve;
    });
    mountADKPage({ approvals: [approval], approvalResolution: approvalResponse });
    await flushRequests();
    await sendPageMessage("optimistic approval");
    await expandQueue("待审批");
    expect(document.querySelectorAll(".adk-approval-queue__item")).toHaveLength(1);

    const approve = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
      (candidate) => candidate.textContent?.trim() === "批准",
    );
    approve?.click();
    await nextTick();
    expect(document.querySelectorAll(".adk-approval-queue__item")).toHaveLength(0);

    finishApproval({
      approval: { ...approval, status: "APPROVED" },
      run: buildRun({ id: run.id, status: "COMPLETED", pendingApprovals: [] }),
    });
    await flushRequests();
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
});
