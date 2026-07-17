import { describe, expect, it, vi } from "vitest";

import type { ADKRun } from "@/contracts";

import {
  resolveGoalAwareChatResponse,
  selectActiveGoalRun,
  selectPrimaryRootRun,
  isStaleTerminalGoalPauseOverride,
  buildRunObservationSignature,
  hasPendingRunApproval,
  isGoalPauseAbortError,
  mergeADKRunLifecycleSnapshot,
  syncGoalAwareActiveRun,
  waitForGoalAwareRunContinuation,
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

  it("keeps local state stable when an API refresh has no run payload", () => {
    const active = buildRun({ id: "run-active", status: "RUNNING" });
    const state = {
      runId: active.id,
      sessionId: active.sessionId,
      status: active.status,
      lastObservedToolSignature: "stable",
      waitingForContinuation: true,
    };

    const result = syncGoalAwareActiveRun({
      incomingRun: undefined,
      activeRunSnapshot: active,
      activeGoalRunSnapshot: null,
      activeRunState: state,
      goalObjectiveState: { draft: "持续观察", touched: true, error: "" },
      goalObjectiveSaving: false,
    });

    expect(result).toMatchObject({
      run: undefined,
      activeRunSnapshot: active,
      activeGoalRunSnapshot: null,
      activeRunState: state,
      goalObjectiveState: { draft: "持续观察", touched: true, error: "" },
      goalObjectiveCleared: false,
    });
  });

  it("merges same-goal pause snapshots without discarding pause metadata", () => {
    const currentPaused = buildRun({
      status: "PAUSED",
      workMode: "loop",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      pausedAt: "2026-06-19T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
    });
    const incomingPaused = buildRun({
      status: "PAUSED",
      workMode: "loop",
      workflowStatus: "PAUSED",
      pauseRequestedAt: undefined,
      pausedAt: undefined,
      pausedReason: undefined,
      resumeState: undefined,
    });
    const currentRequested = buildRun({
      status: "RUNNING",
      workMode: "loop",
      pauseRequestedAt: "2026-06-19T00:00:20Z",
      resumeState: "user_pause_requested",
    });
    const incomingRequested = buildRun({
      status: "PENDING_APPROVAL",
      workMode: "loop",
      pauseRequestedAt: "2026-06-19T00:00:21Z",
      resumeState: "user_pause_requested",
    });

    const mergedPaused = mergeADKRunLifecycleSnapshot(currentPaused, incomingPaused);
    const mergedRequested = mergeADKRunLifecycleSnapshot(
      currentRequested,
      incomingRequested,
    );

    expect(mergedPaused).toMatchObject({
      status: "PAUSED",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      pausedAt: "2026-06-19T00:00:12Z",
      pausedReason: "user",
      resumeState: "user_paused",
    });
    expect(mergedRequested).toMatchObject({
      status: "PENDING_APPROVAL",
      pauseRequestedAt: "2026-06-19T00:00:21Z",
      resumeState: "user_pause_requested",
    });
  });

  it("distinguishes deliberate goal-pause aborts and records continuation progress", async () => {
    const controller = new AbortController();
    controller.abort();
    expect(
      isGoalPauseAbortError(controller, new DOMException("stopped", "AbortError"), "goal_pause"),
    ).toBe(true);
    expect(isGoalPauseAbortError(controller, new Error("stopped"), "goal_pause")).toBe(false);
    expect(isGoalPauseAbortError(controller, new DOMException("stopped", "AbortError"), "manual_stop")).toBe(false);

    const monitorRun = vi.fn(async (_run, callbacks) => {
      await callbacks?.onProgress?.(
        buildRun({ status: "FAILED", failureReason: "provider rejected the request" }),
        buildRun({ status: "RUNNING" }),
      );
      await callbacks?.onTerminal?.(buildRun({ status: "COMPLETED" }));
      return buildRun({ status: "PENDING_APPROVAL" });
    });
    const syncActiveRun = vi.fn();
    const reloadTimeline = vi.fn().mockResolvedValue(undefined);
    const handleTerminalRun = vi.fn().mockResolvedValue(undefined);
    const setErrorMessage = vi.fn();

    await waitForGoalAwareRunContinuation({
      run: buildRun({ status: "RUNNING" }),
      monitorRun,
      syncActiveRun,
      reloadTimeline,
      handleTerminalRun,
      setErrorMessage,
    });
    await waitForGoalAwareRunContinuation({
      run: buildRun({ status: "COMPLETED" }),
      monitorRun,
      syncActiveRun,
      reloadTimeline,
      handleTerminalRun,
    });

    expect(monitorRun).toHaveBeenCalledOnce();
    expect(syncActiveRun).toHaveBeenCalledWith(
      expect.objectContaining({ status: "FAILED" }),
      false,
    );
    expect(syncActiveRun).toHaveBeenCalledWith(
      expect.objectContaining({ status: "PENDING_APPROVAL" }),
      true,
    );
    expect(reloadTimeline).toHaveBeenCalledTimes(3);
    expect(handleTerminalRun).toHaveBeenCalledWith(expect.objectContaining({ status: "COMPLETED" }));
    expect(setErrorMessage).toHaveBeenCalledWith(expect.stringContaining("运行失败"));
  });

  it("signs every observable run detail and recognizes approval blockers", () => {
    const run = buildRun({
      status: "PENDING_APPROVAL",
      resumeState: "user_pause_requested",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      workflowPlan: [{ taskId: "task-1", title: "复核订单", status: "PENDING", childRunId: "child-1", iteration: 2 }],
      toolCalls: [{ id: "tool-1", toolName: "order.preview", status: "COMPLETED", updatedAt: "2026-06-19T00:00:11Z" }],
      pendingApprovals: [{ id: "approval-1", toolName: "order.submit", status: "PENDING", updatedAt: "2026-06-19T00:00:12Z" }],
      inputRequest: { id: "input-1", status: "PENDING", updatedAt: "2026-06-19T00:00:13Z" },
      inputRequests: [{ id: "input-2", status: "PENDING", updatedAt: "2026-06-19T00:00:14Z" }],
    });
    const signature = JSON.parse(buildRunObservationSignature(run));

    expect(signature.workflowPlan).toEqual([
      expect.objectContaining({ taskId: "task-1", childRunId: "child-1", iteration: 2 }),
    ]);
    expect(signature.inputRequests).toEqual([
      { id: "input-2", status: "PENDING", updatedAt: "2026-06-19T00:00:14Z" },
    ]);
    expect(hasPendingRunApproval(run)).toBe(true);
    expect(hasPendingRunApproval(buildRun({ status: "RUNNING", pendingApprovals: [{ id: "resolved", toolName: "order.submit", status: "APPROVED" }] }))).toBe(false);
    expect(hasPendingRunApproval(undefined)).toBe(false);
  });

  it("keeps a cached non-goal pause request stable, but lets cancellation win", () => {
    const cachedNonGoalRun = buildRun({
      workMode: "chat",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      resumeState: "manual_pause_requested",
    });
    const staleRefresh = buildRun({
      status: "RUNNING",
      pauseRequestedAt: undefined,
      resumeState: undefined,
    });
    const preserved = mergeADKRunLifecycleSnapshot(
      cachedNonGoalRun,
      staleRefresh,
    );

    expect(preserved).toMatchObject({
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      resumeState: "manual_pause_requested",
    });
    expect(mergeADKRunLifecycleSnapshot(cachedNonGoalRun, undefined)).toBe(
      cachedNonGoalRun,
    );

    const pausedGoal = buildRun({
      status: "PAUSED",
      workMode: "loop",
      pauseRequestedAt: "2026-06-19T00:00:10Z",
      pausedAt: "2026-06-19T00:00:12Z",
      resumeState: "user_paused",
    });
    const cancelled = buildRun({ status: "CANCELLED", cancelledAt: "2026-06-19T00:01:00Z" });
    expect(mergeADKRunLifecycleSnapshot(pausedGoal, cancelled)).toEqual(cancelled);
    expect(isStaleTerminalGoalPauseOverride(cancelled, buildRun({ id: "other-run" }))).toBe(false);
    expect(buildRunObservationSignature(undefined)).toBe("");
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
