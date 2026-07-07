import type {
  ADKApproval,
  ADKRun,
  ADKTimelineEntry,
  ADKToolCall,
  ADKWorkflowStepState,
} from "@/contracts";

import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

const mocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
}));

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => mocks.fetchEnvelope(...args),
}));

import { createTimelineEntryState } from "../src/composables/adkTimeline";
import {
  useADKWorkflowQueueState,
  workflowQueueTone,
} from "../src/composables/useADKWorkflowQueueState";

function buildApproval(
  overrides: Partial<ADKApproval> & { id: string; runId: string },
): ADKApproval {
  return {
    id: overrides.id,
    runId: overrides.runId,
    agentId: overrides.agentId ?? "agent-1",
    toolName: overrides.toolName ?? "approve.order",
    status: overrides.status ?? "PENDING",
    reason: overrides.reason ?? "needs approval",
    createdAt: overrides.createdAt ?? "2026-07-03T10:00:00.000Z",
    updatedAt: overrides.updatedAt ?? "2026-07-03T10:00:00.000Z",
    ...overrides,
  };
}

function buildToolCall(
  overrides: Partial<ADKToolCall> & { id: string; runId: string },
): ADKToolCall {
  return {
    id: overrides.id,
    runId: overrides.runId,
    toolName: overrides.toolName ?? "search",
    permission: overrides.permission ?? "read",
    status: overrides.status ?? "COMPLETED",
    requiresUser: overrides.requiresUser ?? false,
    createdAt: overrides.createdAt ?? "2026-07-03T10:00:00.000Z",
    updatedAt: overrides.updatedAt ?? "2026-07-03T10:00:01.000Z",
    ...overrides,
  };
}

function buildWorkflowStep(
  overrides: Partial<ADKWorkflowStepState> & {
    title: string;
    status: string;
  },
): ADKWorkflowStepState {
  return {
    title: overrides.title,
    status: overrides.status,
    ...overrides,
  };
}

function buildRun(
  overrides: Partial<ADKRun> & {
    id: string;
    sessionId?: string;
    status?: string;
  },
): ADKRun {
  return {
    id: overrides.id,
    sessionId: overrides.sessionId ?? "session-1",
    agentId: overrides.agentId ?? "agent-1",
    status: overrides.status ?? "RUNNING",
    message: overrides.message ?? "",
    toolCalls: overrides.toolCalls ?? [],
    pendingApprovals: overrides.pendingApprovals ?? [],
    createdAt: overrides.createdAt ?? "2026-07-03T10:00:00.000Z",
    updatedAt: overrides.updatedAt ?? "2026-07-03T10:00:01.000Z",
    ...overrides,
  };
}

function buildTimelineEntry(
  overrides: Partial<ADKTimelineEntry> & {
    id: string;
    kind: string;
    sequence: number;
  },
) {
  return createTimelineEntryState({
    id: overrides.id,
    sessionId: overrides.sessionId ?? "session-1",
    kind: overrides.kind,
    createdAt: overrides.createdAt ?? "2026-07-03T10:00:00.000Z",
    sequence: overrides.sequence,
    ...overrides,
  });
}

describe("useADKWorkflowQueueState", () => {
  it("builds child queues, filtered parent timelines, and approval views for workflow runs", async () => {
    const parentApproval = buildApproval({
      id: "approval-parent",
      runId: "run-parent",
    });
    const resolvingApproval = buildApproval({
      id: "approval-resolving",
      runId: "run-parent",
    });
    const childApproval = buildApproval({
      id: "approval-child",
      runId: "child-2",
      status: "PENDING_APPROVAL",
    });

    const timelineEntries = ref([
      buildTimelineEntry({
        id: "entry-parent",
        kind: "assistant_message",
        sequence: 1,
        runId: "run-parent",
        text: "workflow started",
      }),
      buildTimelineEntry({
        id: "entry-child",
        kind: "assistant_message",
        sequence: 2,
        runId: "child-1",
        text: "child detail",
      }),
      buildTimelineEntry({
        id: "entry-tools",
        kind: "tool_group",
        sequence: 3,
        runId: "run-parent",
        toolCalls: [
          buildToolCall({ id: "tool-parent", runId: "run-parent" }),
          buildToolCall({ id: "tool-child", runId: "child-1" }),
        ],
      }),
      buildTimelineEntry({
        id: "entry-approvals",
        kind: "approval_group",
        sequence: 4,
        approvals: [parentApproval, childApproval],
      }),
    ]);

    const state = useADKWorkflowQueueState({
      timelineEntries,
      selectedSessionId: ref("session-1"),
      resolvingApprovalIds: ref(new Set(["approval-resolving"])),
    });

    mocks.fetchEnvelope.mockImplementation(async (url: string) => {
      if (url.endsWith("/child-1")) {
        return buildRun({
          id: "child-1",
          parentRunId: "run-parent",
          status: "COMPLETED",
          message: "quotes collected",
          userMessage: "collect quotes",
        });
      }
      if (url.endsWith("/child-2")) {
        return buildRun({
          id: "child-2",
          parentRunId: "run-parent",
          status: "PENDING_APPROVAL",
          message: "waiting approval",
          pendingApprovals: [childApproval],
        });
      }
      throw new Error(`Unexpected workflow run fetch: ${url}`);
    });

    await state.syncWorkflowRun(
      buildRun({
        id: "run-parent",
        workMode: "loop",
        workflowStatus: "RUNNING",
        childRunIds: ["child-1", " child-2 ", "child-2"],
        pendingApprovals: [parentApproval, resolvingApproval],
        workflowPlan: [
          buildWorkflowStep({
            title: "Collect quotes",
            message: "query market data",
            status: "DONE",
            childRunId: "child-1",
          }),
          buildWorkflowStep({
            title: "Place order",
            description: "submit a limit order",
            status: "BLOCKED",
            childRunId: "child-2",
          }),
        ],
      }),
    );

    expect(state.parentWorkflowPlanRun.value?.id).toBe("run-parent");
    expect(state.parentChildRunItems.value).toEqual([
      expect.objectContaining({
        id: "child-1",
        index: 1,
        stepTitle: "Collect quotes",
        status: "COMPLETED",
        pendingApprovalCount: 0,
      }),
      expect.objectContaining({
        id: "child-2",
        index: 2,
        stepTitle: "Place order",
        status: "PENDING_APPROVAL",
        pendingApprovalCount: 1,
      }),
    ]);
    expect(state.parentApprovalQueue.value.map((item) => item.approval.id)).toEqual(
      ["approval-parent", "approval-child"],
    );
    expect(
      state.parentTimelineEntries.value.some((entry) => entry.id === "entry-child"),
    ).toBe(false);
    expect(
      state.parentTimelineEntries.value.find((entry) => entry.id === "entry-tools")
        ?.toolCalls,
    ).toEqual([expect.objectContaining({ id: "tool-parent" })]);
    expect(
      state.parentTimelineEntries.value.map((entry) => entry.text).filter(Boolean),
    ).toEqual(
      expect.arrayContaining([
        "启动子智能体 #1：Collect quotes（child-1）",
        "子智能体 #2 等待审批：Place order（child-2）",
      ]),
    );

    state.setActiveChildRunId("child-2");
    expect(state.activeChildRunId.value).toBe("child-2");
    expect(state.selectedApprovalQueue.value.map((item) => item.approval.id)).toEqual(
      ["approval-child"],
    );
    expect(state.childViewContext.value).toEqual({
      title: "子智能体 #2",
      runId: "child-2",
      message: "submit a limit order",
    });
    expect(state.childTimelineEntries.value).toEqual([]);

    state.setActiveChildRunId("missing-child");
    expect(state.activeChildRunId.value).toBe("");
  });

  it("derives terminal workflow status from child completion and clears queues when another root run takes over", async () => {
    const timelineEntries = ref([
      buildTimelineEntry({
        id: "entry-parent",
        kind: "assistant_message",
        sequence: 1,
        runId: "run-parent",
        text: "workflow root",
      }),
    ]);
    const state = useADKWorkflowQueueState({
      timelineEntries,
      selectedSessionId: ref("session-1"),
    });

    mocks.fetchEnvelope.mockImplementation(async (url: string) => {
      if (url.endsWith("/child-ok")) {
        return buildRun({
          id: "child-ok",
          parentRunId: "run-parent",
          status: "COMPLETED",
        });
      }
      if (url.endsWith("/child-failed")) {
        return buildRun({
          id: "child-failed",
          parentRunId: "run-parent",
          status: "FAILED",
        });
      }
      throw new Error(`Unexpected workflow run fetch: ${url}`);
    });

    await state.syncWorkflowRun(
      buildRun({
        id: "run-parent",
        workMode: "loop",
        status: "RUNNING",
        workflowStatus: "RUNNING",
        childRunIds: ["child-ok", "child-failed"],
        workflowPlan: [
          buildWorkflowStep({
            title: "Collect",
            status: "DONE",
            childRunId: "child-ok",
          }),
          buildWorkflowStep({
            title: "Trade",
            status: "FAILED",
            childRunId: "child-failed",
          }),
        ],
      }),
    );

    expect(state.parentWorkflowPlanRun.value?.status).toBe("FAILED");
    expect(state.parentWorkflowPlanRun.value?.workflowStatus).toBe("FAILED");
    expect(state.parentWorkflowPlanRun.value?.message).toBe("workflow failed");

    await state.syncWorkflowRun(
      buildRun({
        id: "replacement-run",
        sessionId: "session-1",
        workMode: "chat",
        status: "RUNNING",
      }),
    );

    expect(state.parentWorkflowPlanRun.value).toBeNull();
    expect(state.parentChildRunItems.value).toEqual([]);
    expect(state.childRunSnapshots.value).toEqual({});
  });

  it("merges child snapshot updates, handles empty child plans, and exposes tone helpers", async () => {
    const state = useADKWorkflowQueueState({
      timelineEntries: ref([]),
      selectedSessionId: ref("session-1"),
    });

    mocks.fetchEnvelope.mockResolvedValueOnce(
      buildRun({
        id: "child-1",
        parentRunId: "run-parent",
        status: "RUNNING",
        message: "initial child",
      }),
    );

    await state.syncWorkflowRun(
      buildRun({
        id: "run-parent",
        workMode: "loop",
        childRunIds: ["child-1"],
        workflowPlan: [
          buildWorkflowStep({
            title: "Step 1",
            status: "IN_PROGRESS",
            childRunId: "child-1",
          }),
        ],
      }),
    );

    await state.syncWorkflowRun(
      buildRun({
        id: "child-1",
        parentRunId: "run-parent",
        status: "COMPLETED",
        message: "child finished",
        completedAt: "2026-07-03T10:05:00.000Z",
      }),
    );

    expect(state.childRunSnapshots.value["child-1"]).toEqual(
      expect.objectContaining({
        id: "child-1",
        status: "COMPLETED",
        message: "child finished",
        completedAt: "2026-07-03T10:05:00.000Z",
      }),
    );

    await state.syncWorkflowRun(
      buildRun({
        id: "run-empty",
        workMode: "loop",
        childRunIds: [],
        workflowPlan: [
          buildWorkflowStep({
            title: "Step without child",
            status: "TODO",
          }),
        ],
      }),
    );

    expect(state.childRunSnapshots.value).toEqual({});
    expect(state.parentChildRunItems.value).toEqual([]);

    expect(workflowQueueTone("PENDING_APPROVAL")).toBe("warning");
    expect(workflowQueueTone("RUNNING")).toBe("info");
    expect(workflowQueueTone("FAILED")).toBe("error");
    expect(workflowQueueTone("CANCELLED")).toBe("muted");
    expect(workflowQueueTone("COMPLETED")).toBe("success");
  });

  it("renders child lifecycle text for blocked, failed, cancelled, pending, and external states", async () => {
    const timelineEntries = ref([
      buildTimelineEntry({
        id: "entry-parent",
        kind: "assistant_message",
        sequence: 1,
        runId: "run-parent",
        text: "workflow root",
      }),
    ]);
    const state = useADKWorkflowQueueState({
      timelineEntries,
      selectedSessionId: ref("session-1"),
    });

    const childRuns = {
      "child-blocked": buildRun({
        id: "child-blocked",
        parentRunId: "run-parent",
        status: "BLOCKED",
      }),
      "child-failed": buildRun({
        id: "child-failed",
        parentRunId: "run-parent",
        status: "FAILED",
      }),
      "child-cancelled": buildRun({
        id: "child-cancelled",
        parentRunId: "run-parent",
        status: "CANCELLED",
      }),
      "child-pending": buildRun({
        id: "child-pending",
        parentRunId: "run-parent",
        status: "PENDING",
      }),
      "child-external": buildRun({
        id: "child-external",
        parentRunId: "run-parent",
        status: "QUEUED_EXTERNALLY",
      }),
    } as const;

    mocks.fetchEnvelope.mockImplementation(async (url: string) => {
      const runId = url.split("/").at(-1) ?? "";
      const run = childRuns[runId as keyof typeof childRuns];
      if (run) {
        return run;
      }
      throw new Error(`Unexpected workflow run fetch: ${url}`);
    });

    await state.syncWorkflowRun(
      buildRun({
        id: "run-parent",
        workMode: "loop",
        workflowStatus: "RUNNING",
        childRunIds: Object.keys(childRuns),
        workflowPlan: [
          buildWorkflowStep({
            title: "Blocked child",
            status: "BLOCKED",
            childRunId: "child-blocked",
          }),
          buildWorkflowStep({
            title: "Failed child",
            status: "FAILED",
            childRunId: "child-failed",
          }),
          buildWorkflowStep({
            title: "Cancelled child",
            status: "CANCELLED",
            childRunId: "child-cancelled",
          }),
          buildWorkflowStep({
            title: "Pending child",
            status: "PENDING",
            childRunId: "child-pending",
          }),
          buildWorkflowStep({
            title: "External child",
            status: "RUNNING",
            childRunId: "child-external",
          }),
        ],
      }),
    );

    const texts = state.parentTimelineEntries.value
      .map((entry) => entry.text)
      .filter(Boolean);

    expect(texts).toEqual(
      expect.arrayContaining([
        "子智能体 #1 已阻断：Blocked child（child-blocked）",
        "子智能体 #2 已结束：失败 · Failed child（child-failed）",
        "子智能体 #3 已结束：已取消 · Cancelled child（child-cancelled）",
        "子智能体 #4 等待启动：Pending child（child-pending）",
        "子智能体 #5 状态 QUEUED_EXTERNALLY：External child（child-external）",
      ]),
    );
  });

  it("keeps child lifecycle rows ordered by child creation time", async () => {
    const state = useADKWorkflowQueueState({
      timelineEntries: ref([]),
      selectedSessionId: ref("session-1"),
    });

    mocks.fetchEnvelope.mockImplementation(async (url: string) => {
      if (url.endsWith("/child-older")) {
        return buildRun({
          id: "child-older",
          parentRunId: "run-parent",
          status: "RUNNING",
          createdAt: "2026-07-03T10:00:00.000Z",
          updatedAt: "2026-07-03T10:10:00.000Z",
        });
      }
      if (url.endsWith("/child-newer")) {
        return buildRun({
          id: "child-newer",
          parentRunId: "run-parent",
          status: "RUNNING",
          createdAt: "2026-07-03T10:05:00.000Z",
          updatedAt: "2026-07-03T10:06:00.000Z",
        });
      }
      throw new Error(`Unexpected workflow run fetch: ${url}`);
    });

    await state.syncWorkflowRun(
      buildRun({
        id: "run-parent",
        workMode: "loop",
        workflowStatus: "RUNNING",
        childRunIds: ["child-older", "child-newer"],
        workflowPlan: [
          buildWorkflowStep({
            title: "Older child",
            status: "IN_PROGRESS",
            childRunId: "child-older",
          }),
          buildWorkflowStep({
            title: "Newer child",
            status: "IN_PROGRESS",
            childRunId: "child-newer",
          }),
        ],
      }),
    );

    expect(
      state.parentTimelineEntries.value.map((entry) => entry.text).filter(Boolean),
    ).toEqual([
      "启动子智能体 #1：Older child（child-older）",
      "子智能体 #1 正在运行：Older child（child-older）",
      "启动子智能体 #2：Newer child（child-newer）",
      "子智能体 #2 正在运行：Newer child（child-newer）",
    ]);
  });
});
