import { describe, expect, it } from "vitest";

import type { ADKRun } from "@/contracts";

import {
  resolveGoalAwareChatResponse,
  selectActiveGoalRun,
  selectPrimaryRootRun,
  isStaleTerminalGoalPauseOverride,
  mergeADKRunLifecycleSnapshot,
  syncGoalAwareActiveRun,
} from "../src/composables/adkChatRuntime";

describe("mergeADKRunLifecycleSnapshot", () => {
  it("keeps a user-paused goal over a stale completed snapshot", () => {
    const pausedRun = buildRun({
      status: "PAUSED",
      workMode: "loop",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      pausedAt: "2026-06-19T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
    });
    const staleCompleted = buildRun({
      status: "COMPLETED",
      workMode: "loop",
      workflowStatus: "COMPLETED",
      message: "goal completed",
      completedAt: "2026-06-19T00:00:20Z",
    });

    const merged = mergeADKRunLifecycleSnapshot(pausedRun, staleCompleted);

    expect(merged?.status).toBe("PAUSED");
    expect(merged?.resumeState).toBe("user_paused");
    expect(merged?.pausedReason).toBe("user");
    expect(isStaleTerminalGoalPauseOverride(staleCompleted, merged)).toBe(true);
  });

  it("keeps a pause-requested goal over a stale completed snapshot", () => {
    const pauseRequested = buildRun({
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "RUNNING",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      resumeState: "user_pause_requested",
      message: "目标将在当前轮结束后暂停。",
    });
    const staleCompleted = buildRun({
      status: "COMPLETED",
      workMode: "loop",
      workflowStatus: "COMPLETED",
      message: "goal completed",
      completedAt: "2026-06-19T00:00:20Z",
    });

    const merged = mergeADKRunLifecycleSnapshot(
      pauseRequested,
      staleCompleted,
    );

    expect(merged?.status).toBe("RUNNING");
    expect(merged?.pauseRequestedAt).toBe("2026-06-19T00:00:10Z");
    expect(merged?.resumeState).toBe("user_pause_requested");
    expect(isStaleTerminalGoalPauseOverride(staleCompleted, merged)).toBe(true);
  });

  it("allows a resumed goal to replace a paused snapshot", () => {
    const pausedRun = buildRun({
      status: "PAUSED",
      workMode: "loop",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      pausedAt: "2026-06-19T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
    });
    const resumedRun = buildRun({
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "RUNNING",
      resumeState: "user_resuming",
    });

    const merged = mergeADKRunLifecycleSnapshot(pausedRun, resumedRun);

    expect(merged).toEqual(resumedRun);
    expect(isStaleTerminalGoalPauseOverride(resumedRun, merged)).toBe(false);
  });

  it("clears the goal objective state when a root loop run completes", () => {
    const completedRun = buildRun({
      status: "COMPLETED",
      workMode: "loop",
      workflowStatus: "COMPLETED",
      objective: "完成目标",
    });

    const result = syncGoalAwareActiveRun({
      incomingRun: completedRun,
      activeRunSnapshot: completedRun,
      activeGoalRunSnapshot: completedRun,
      activeRunState: {
        runId: completedRun.id,
        sessionId: completedRun.sessionId,
        status: completedRun.status,
        lastObservedToolSignature: "",
        waitingForContinuation: false,
      },
      goalObjectiveState: {
        draft: "完成目标",
        touched: true,
        error: "保存失败",
      },
      goalObjectiveSaving: false,
    });

    expect(result.goalObjectiveCleared).toBe(true);
    expect(result.goalObjectiveState).toEqual({
      draft: "",
      touched: false,
      error: "",
    });
    expect(result.activeRunSnapshot).toBeNull();
    expect(result.activeGoalRunSnapshot).toBeNull();
    expect(result.activeRunState).toBeNull();
  });

  it("keeps a completed/running workflow parent visible until backend reconciliation", () => {
    const pseudoCompletedGoal = buildRun({
      status: "COMPLETED",
      workMode: "loop",
      workflowStatus: "RUNNING",
      objective: "等待子智能体审批",
      message: "running",
    });

    const result = syncGoalAwareActiveRun({
      incomingRun: pseudoCompletedGoal,
      activeRunSnapshot: null,
      activeGoalRunSnapshot: null,
      activeRunState: null,
      goalObjectiveState: {
        draft: "",
        touched: false,
        error: "",
      },
      goalObjectiveSaving: false,
    });

    expect(result.goalObjectiveCleared).toBe(false);
    expect(result.goalObjectiveState.draft).toBe("等待子智能体审批");
    expect(result.activeRunSnapshot?.id).toBe(pseudoCompletedGoal.id);
    expect(result.activeGoalRunSnapshot?.id).toBe(pseudoCompletedGoal.id);
  });

  it("hydrates the goal objective from an untouched active loop run", () => {
    const runningGoal = buildRun({
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "RUNNING",
      objective: "推进目标",
    });

    const result = syncGoalAwareActiveRun({
      incomingRun: runningGoal,
      activeRunSnapshot: null,
      activeGoalRunSnapshot: null,
      activeRunState: null,
      goalObjectiveState: {
        draft: "",
        touched: false,
        error: "旧错误",
      },
      goalObjectiveSaving: false,
    });

    expect(result.goalObjectiveCleared).toBe(false);
    expect(result.goalObjectiveState).toEqual({
      draft: "推进目标",
      touched: false,
      error: "",
    });
    expect(result.activeRunSnapshot?.id).toBe(runningGoal.id);
    expect(result.activeGoalRunSnapshot?.id).toBe(runningGoal.id);
    expect(result.activeRunState?.runId).toBe(runningGoal.id);
  });

  it("prefers a user-paused root goal over other active goal snapshots", () => {
    const pausedGoal = buildRun({
      id: "run-paused-goal",
      status: "PAUSED",
      workMode: "loop",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      pausedAt: "2026-06-19T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
    });
    const runningGoal = buildRun({
      id: "run-running-goal",
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "RUNNING",
    });

    const selected = selectActiveGoalRun({
      activeRunSnapshot: runningGoal,
      activeGoalRunSnapshot: pausedGoal,
      workflowRun: null,
    });

    expect(selected?.id).toBe(pausedGoal.id);
  });

  it("prefers the workflow parent as the controlling root run", () => {
    const workflowRun = buildRun({
      id: "run-workflow-parent",
      status: "RUNNING",
      workMode: "loop",
      workflowStatus: "RUNNING",
    });
    const otherRootRun = buildRun({
      id: "run-other-root",
      status: "RUNNING",
    });

    const selected = selectPrimaryRootRun({
      activeRunSnapshot: otherRootRun,
      activeGoalRunSnapshot: null,
      workflowRun,
    });

    expect(selected?.id).toBe(workflowRun.id);
  });

  it("keeps a terminal root run authoritative over stale child activity", () => {
    const cancelledRootRun = buildRun({
      id: "run-root-cancelled",
      status: "CANCELLED",
      workMode: "loop",
      workflowStatus: "CANCELLED",
    });

    const selected = selectPrimaryRootRun({
      activeRunSnapshot: cancelledRootRun,
      activeGoalRunSnapshot: null,
      workflowRun: null,
    });

    expect(selected?.id).toBe(cancelledRootRun.id);
    expect(selected?.status).toBe("CANCELLED");
  });

  it("resolves a stale terminal pause override into a normalized response", () => {
    const pausedRun = buildRun({
      status: "PAUSED",
      workMode: "loop",
      workflowStatus: "PAUSED",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      pausedAt: "2026-06-19T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
    });
    const response = {
      reply: "goal done",
      session: {
        id: "session-1",
        agentId: "agent-1",
        title: "Session",
        createdAt: "2026-06-19T00:00:00Z",
        updatedAt: "2026-06-19T00:00:00Z",
      },
      run: buildRun({
        status: "COMPLETED",
        workMode: "loop",
        workflowStatus: "COMPLETED",
        message: "goal completed",
        completedAt: "2026-06-19T00:00:20Z",
      }),
      pendingApprovals: [],
      timeline: [],
    };

    const resolution = resolveGoalAwareChatResponse(response, () => pausedRun);

    expect(resolution.staleTerminalGoalPauseOverride).toBe(true);
    expect(resolution.resolvedRun.status).toBe("PAUSED");
    expect(resolution.resolvedResponse.run.resumeState).toBe("user_paused");
  });
});

function buildRun(overrides: Partial<ADKRun>): ADKRun {
  return {
    id: "run-goal-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "RUNNING",
    message: "",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-19T00:00:00Z",
    updatedAt: "2026-06-19T00:00:00Z",
    ...overrides,
  };
}
