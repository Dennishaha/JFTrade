import type { ADKChatResponse, ADKRun } from "@/contracts";

import {
  isActiveRunStatus,
  isTerminalRunStatus,
  isUserPausedGoalRun,
  runErrorDisplayMessage,
} from "./adkChatPresentation";
import { normalizeADKChatResponse } from "./adkNormalization";

export const PROVISIONAL_SESSION_KEY = "__adk_provisional_session__";

export type QueuedChatMessageMode = "queued" | "interrupt";

export interface QueuedChatMessage {
  id: string;
  sessionKey: string;
  text: string;
  mode: QueuedChatMessageMode;
  forceChat?: boolean;
  createdAt: string;
}

export interface ActiveChatRunState {
  runId: string;
  sessionId: string;
  status: string;
  lastObservedToolSignature: string;
  waitingForContinuation: boolean;
}

export interface GoalObjectiveState {
  draft: string;
  touched: boolean;
  error: string;
}

export interface GoalAwareRunSyncOptions {
  incomingRun: ADKRun | undefined;
  waitingForContinuation?: boolean;
  activeRunSnapshot: ADKRun | null;
  activeGoalRunSnapshot: ADKRun | null;
  activeRunState: ActiveChatRunState | null;
  goalObjectiveState: GoalObjectiveState;
  goalObjectiveSaving: boolean;
  syncWorkflowRun?: (run: ADKRun) => void | Promise<void>;
}

export interface GoalAwareRunSyncResult {
  run: ADKRun | undefined;
  activeRunSnapshot: ADKRun | null;
  activeGoalRunSnapshot: ADKRun | null;
  activeRunState: ActiveChatRunState | null;
  goalObjectiveState: GoalObjectiveState;
  goalObjectiveCleared: boolean;
}

export interface GoalAwareChatResponseResolution {
  normalizedResponse: ADKChatResponse;
  resolvedResponse: ADKChatResponse;
  resolvedRun: ADKRun;
  staleTerminalGoalPauseOverride: boolean;
  failMessage: string;
  terminal: boolean;
}

export interface GoalAwareRunContinuationOptions {
  run: ADKRun | undefined;
  monitorRun: (
    run: ADKRun | undefined,
    options?: {
      onProgress?: (
        latestRun: ADKRun,
        previousRun: ADKRun,
      ) => void | Promise<void>;
      onTerminal?: (latestRun: ADKRun) => void | Promise<void>;
    },
  ) => Promise<ADKRun | undefined>;
  syncActiveRun: (
    run: ADKRun | undefined,
    waitingForContinuation?: boolean,
  ) => ADKRun | undefined;
  reloadTimeline: () => Promise<void>;
  handleTerminalRun: (run: ADKRun) => Promise<void>;
  setErrorMessage?: (message: string) => void;
}

export function buildQueueSessionKey(sessionId: string | undefined): string {
  return sessionId?.trim() ? sessionId.trim() : PROVISIONAL_SESSION_KEY;
}

export function createQueuedChatMessage(
  text: string,
  sessionKey: string,
  mode: QueuedChatMessageMode,
  options: { forceChat?: boolean } = {},
): QueuedChatMessage {
  return {
    id: `queued-chat-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    sessionKey,
    text,
    mode,
    forceChat: options.forceChat === true,
    createdAt: new Date().toISOString(),
  };
}

export function buildActiveChatRunState(
  run: ADKRun,
  waitingForContinuation = false,
): ActiveChatRunState {
  return {
    runId: run.id,
    sessionId: run.sessionId ?? "",
    status: run.status,
    lastObservedToolSignature: buildRunObservationSignature(run),
    waitingForContinuation,
  };
}

export function syncGoalAwareActiveRun(
  options: GoalAwareRunSyncOptions,
): GoalAwareRunSyncResult {
  const {
    incomingRun,
    waitingForContinuation = false,
    activeRunSnapshot,
    activeGoalRunSnapshot,
    activeRunState,
    goalObjectiveSaving,
    syncWorkflowRun,
  } = options;
  if (!incomingRun) {
    return {
      run: undefined,
      activeRunSnapshot,
      activeGoalRunSnapshot,
      activeRunState,
      goalObjectiveState: options.goalObjectiveState,
      goalObjectiveCleared: false,
    };
  }
  const currentRun =
    activeRunSnapshot?.id === incomingRun.id
      ? activeRunSnapshot
      : activeGoalRunSnapshot?.id === incomingRun.id
        ? activeGoalRunSnapshot
        : null;
  const run =
    mergeADKRunLifecycleSnapshot(currentRun, incomingRun) ?? incomingRun;
  void syncWorkflowRun?.(run);

  let nextActiveRunSnapshot = activeRunSnapshot;
  let nextActiveGoalRunSnapshot = activeGoalRunSnapshot;
  let nextActiveRunState = activeRunState;
  let nextGoalObjectiveState = options.goalObjectiveState;
  let goalObjectiveCleared = false;

  if (
    isTerminalRunStatus(run.status) &&
    !isCompletedRunningWorkflowGoal(run) &&
    !isResumableTimedOutGoalRun(run)
  ) {
    if (isRootLoopRun(run)) {
      goalObjectiveCleared =
        nextGoalObjectiveState.draft !== "" ||
        nextGoalObjectiveState.touched ||
        nextGoalObjectiveState.error !== "";
      nextGoalObjectiveState = {
        draft: "",
        touched: false,
        error: "",
      };
    }
    if (isRootRun(run) && nextActiveRunSnapshot?.id === run.id) {
      nextActiveRunSnapshot = null;
    }
    if (isRootRun(run) && nextActiveGoalRunSnapshot?.id === run.id) {
      nextActiveGoalRunSnapshot = null;
    }
    if (!nextActiveRunState || nextActiveRunState.runId === run.id) {
      nextActiveRunState = null;
    }
    return {
      run,
      activeRunSnapshot: nextActiveRunSnapshot,
      activeGoalRunSnapshot: nextActiveGoalRunSnapshot,
      activeRunState: nextActiveRunState,
      goalObjectiveState: nextGoalObjectiveState,
      goalObjectiveCleared,
    };
  }

  if (isRootRun(run)) {
    nextActiveRunSnapshot = run;
  }
  if (
    isActiveGoalParentRun(run) &&
    !goalObjectiveSaving &&
    !nextGoalObjectiveState.touched
  ) {
    nextActiveGoalRunSnapshot = run;
    nextGoalObjectiveState = {
      draft: run.objective ?? nextGoalObjectiveState.draft,
      touched: false,
      error: "",
    };
  } else if (isActiveGoalParentRun(run)) {
    nextActiveGoalRunSnapshot = run;
  }

  nextActiveRunState = buildActiveChatRunState(
    run,
    waitingForContinuation && shouldWaitForRunContinuation(run),
  );
  return {
    run,
    activeRunSnapshot: nextActiveRunSnapshot,
    activeGoalRunSnapshot: nextActiveGoalRunSnapshot,
    activeRunState: nextActiveRunState,
    goalObjectiveState: nextGoalObjectiveState,
    goalObjectiveCleared,
  };
}

export function selectActiveGoalRun(options: {
  activeRunSnapshot: ADKRun | null;
  activeGoalRunSnapshot: ADKRun | null;
  workflowRun: ADKRun | null | undefined;
}): ADKRun | null {
  if (
    isActiveGoalParentRun(options.activeRunSnapshot) &&
    isUserPausedGoalRun(options.activeRunSnapshot)
  ) {
    return options.activeRunSnapshot;
  }
  if (
    isActiveGoalParentRun(options.activeGoalRunSnapshot) &&
    isUserPausedGoalRun(options.activeGoalRunSnapshot)
  ) {
    return options.activeGoalRunSnapshot;
  }
  if (isActiveGoalParentRun(options.workflowRun)) {
    return options.workflowRun;
  }
  if (isActiveGoalParentRun(options.activeRunSnapshot)) {
    return options.activeRunSnapshot;
  }
  return isActiveGoalParentRun(options.activeGoalRunSnapshot)
    ? options.activeGoalRunSnapshot
    : null;
}

export function selectPrimaryRootRun(options: {
  activeRunSnapshot: ADKRun | null;
  activeGoalRunSnapshot: ADKRun | null;
  workflowRun: ADKRun | null | undefined;
}): ADKRun | null {
  if (isRootRun(options.workflowRun)) {
    return options.workflowRun;
  }
  if (isRootRun(options.activeRunSnapshot)) {
    return options.activeRunSnapshot;
  }
  return isRootRun(options.activeGoalRunSnapshot)
    ? options.activeGoalRunSnapshot
    : null;
}

export function resolveGoalAwareChatResponse(
  response: ADKChatResponse,
  syncActiveRun: (
    run: ADKRun | undefined,
    waitingForContinuation?: boolean,
  ) => ADKRun | undefined,
): GoalAwareChatResponseResolution {
  const normalizedResponse = normalizeADKChatResponse(response);
  const resolvedRun =
    syncActiveRun(
      normalizedResponse.run,
      !isTerminalRunStatus(normalizedResponse.run.status),
    ) ?? normalizedResponse.run;
  const staleTerminalGoalPauseOverride = isStaleTerminalGoalPauseOverride(
    normalizedResponse.run,
    resolvedRun,
  );
  return {
    normalizedResponse,
    resolvedResponse: {
      ...normalizedResponse,
      run: resolvedRun,
    },
    resolvedRun,
    staleTerminalGoalPauseOverride,
    failMessage:
      runErrorDisplayMessage(resolvedRun) ||
      runErrorDisplayMessage(normalizedResponse.run),
    terminal: isTerminalRunStatus(resolvedRun.status),
  };
}

export async function waitForGoalAwareRunContinuation(
  options: GoalAwareRunContinuationOptions,
): Promise<void> {
  if (!shouldWaitForRunContinuation(options.run)) {
    return;
  }
  try {
    const latestRun = await options.monitorRun(options.run, {
      onProgress: async (nextRun) => {
        syncGoalAwareContinuationProgress(nextRun, options);
        await options.reloadTimeline();
      },
      onTerminal: async (terminalRun) => {
        await options.reloadTimeline();
        await options.handleTerminalRun(terminalRun);
      },
    });
    if (latestRun && !isTerminalRunStatus(latestRun.status)) {
      syncGoalAwareContinuationProgress(latestRun, options);
      if (hasPendingRunApproval(latestRun)) {
        await options.reloadTimeline();
      }
    }
  } catch {
    // Ignore polling failures and keep the latest visible state.
  }
}

export function mergeADKRunLifecycleSnapshot(
  current: ADKRun | null | undefined,
  incoming: ADKRun | undefined,
): ADKRun | undefined {
  if (!incoming) return current ?? undefined;
  if (!current || current.id !== incoming.id) return incoming;
  if (incoming.resumeState === "user_resuming") return incoming;
  const currentPaused = isUserPausedGoalRun(current);
  const currentPauseRequested = isUserPauseRequestedGoalRun(current);
  const incomingPaused = isUserPausedGoalRun(incoming);
  const incomingPauseRequested = isUserPauseRequestedGoalRun(incoming);
  const currentGoalPaused = currentPaused || currentPauseRequested;
  const incomingGoalPaused = incomingPaused || incomingPauseRequested;

  if (currentGoalPaused && !incomingGoalPaused) {
    if (isTerminalRunStatus(incoming.status) && incoming.status === "CANCELLED") {
      return incoming;
    }
    const merged: ADKRun = {
      ...current,
      ...incoming,
      status: current.status,
      resumeState:
        current.resumeState ||
        (currentPaused ? "user_paused" : "user_pause_requested"),
    };
    copyOptionalRunField(merged, "workflowStatus", current.workflowStatus);
    copyOptionalRunField(merged, "pauseRequestedAt", current.pauseRequestedAt);
    copyOptionalRunField(merged, "pausedAt", current.pausedAt);
    copyOptionalRunField(merged, "pausedReason", current.pausedReason);
    if (currentPaused) {
      copyOptionalRunField(merged, "completedAt", current.completedAt);
      copyOptionalRunField(merged, "cancelledAt", current.cancelledAt);
    }
    return merged;
  }

  if (currentPaused && incomingGoalPaused) {
    const merged: ADKRun = {
      ...current,
      ...incoming,
      status: current.status,
    };
    copyOptionalRunField(
      merged,
      "pauseRequestedAt",
      incoming.pauseRequestedAt ?? current.pauseRequestedAt,
    );
    copyOptionalRunField(
      merged,
      "pausedAt",
      incoming.pausedAt ?? current.pausedAt,
    );
    copyOptionalRunField(
      merged,
      "pausedReason",
      incoming.pausedReason ?? current.pausedReason,
    );
    copyOptionalRunField(
      merged,
      "resumeState",
      incoming.resumeState ?? current.resumeState,
    );
    return merged;
  }

  if (currentPauseRequested && incomingPauseRequested) {
    const merged: ADKRun = {
      ...current,
      ...incoming,
      status: incoming.status,
    };
    copyOptionalRunField(
      merged,
      "pauseRequestedAt",
      incoming.pauseRequestedAt ?? current.pauseRequestedAt,
    );
    copyOptionalRunField(
      merged,
      "resumeState",
      incoming.resumeState ?? current.resumeState,
    );
    return merged;
  }

  if (isTerminalRunStatus(incoming.status) && incoming.status === "CANCELLED") {
    return incoming;
  }

  if (isTerminalRunStatus(incoming.status)) return incoming;

  if (
    current.pauseRequestedAt &&
    !incoming.pauseRequestedAt &&
    !isUserPausedGoalRun(incoming)
  ) {
    const merged: ADKRun = {
      ...current,
      ...incoming,
      pauseRequestedAt: current.pauseRequestedAt,
    };
    copyOptionalRunField(
      merged,
      "resumeState",
      incoming.resumeState || current.resumeState,
    );
    return merged;
  }

  return incoming;
}

export function isUserPauseRequestedGoalRun(
  run: ADKRun | undefined,
): boolean {
  return (
    !!run &&
    String(run.workMode ?? "")
      .trim()
      .toLowerCase() === "loop" &&
    !isTerminalRunStatus(run.status) &&
    !isUserPausedGoalRun(run) &&
    String(run.pauseRequestedAt ?? "").trim() !== ""
  );
}

export function isStaleTerminalGoalPauseOverride(
  incoming: ADKRun | undefined,
  resolved: ADKRun | undefined,
): boolean {
  if (!incoming || !resolved || incoming.id !== resolved.id) {
    return false;
  }
  if (!isTerminalRunStatus(incoming.status) || isTerminalRunStatus(resolved.status)) {
    return false;
  }
  return isUserPausedGoalRun(resolved) || isUserPauseRequestedGoalRun(resolved);
}

export function isGoalPauseAbortError(
  controller: AbortController,
  error: unknown,
  abortReason: string,
): boolean {
  if (!controller.signal.aborted || abortReason !== "goal_pause") {
    return false;
  }
  if (error instanceof DOMException) {
    return error.name === "AbortError";
  }
  return error instanceof Error && error.name === "AbortError";
}

export function isQueueDispatchBlockedByGoalLifecycle(options: {
  sendingChat: boolean;
  hasBlockingRun: boolean;
  goalPauseRequested: boolean;
  goalPaused: boolean;
  queueDispatchingId: string;
}): boolean {
  return (
    options.sendingChat ||
    options.hasBlockingRun ||
    options.goalPauseRequested ||
    options.goalPaused ||
    options.queueDispatchingId !== ""
  );
}

export function shouldWaitForRunContinuation(
  run: ADKRun | undefined,
): run is ADKRun {
  return !!run && !isTerminalRunStatus(run.status) && !isUserPausedGoalRun(run);
}

export function isRootRun(run: ADKRun | null | undefined): run is ADKRun {
  return !!run && String(run.parentRunId ?? "").trim() === "";
}

export function isRootLoopRun(run: ADKRun | null | undefined): run is ADKRun {
  return (
    isRootRun(run) &&
    String(run.workMode ?? "")
      .trim()
      .toLowerCase() === "loop"
  );
}

export function isCompletedRunningWorkflowGoal(
  run: ADKRun | null | undefined,
): boolean {
  return (
    isRootLoopRun(run) &&
    String(run.status ?? "")
      .trim()
      .toUpperCase() === "COMPLETED" &&
    String(run.workflowStatus ?? "")
      .trim()
      .toUpperCase() === "RUNNING"
  );
}

export function isResumableTimedOutGoalRun(
  run: ADKRun | null | undefined,
): boolean {
  return (
    isRootLoopRun(run) &&
    String(run.status ?? "")
      .trim()
      .toUpperCase() === "TIMED_OUT" &&
    String(run.workflowStatus ?? "").trim() !== ""
  );
}

export function isActiveGoalParentRun(
  run: ADKRun | null | undefined,
): run is ADKRun {
  if (!isRootLoopRun(run)) return false;
  return (
    !isTerminalRunStatus(run.status) ||
    isCompletedRunningWorkflowGoal(run) ||
    isResumableTimedOutGoalRun(run)
  );
}

function copyOptionalRunField<
  Key extends
    | "workflowStatus"
    | "pauseRequestedAt"
    | "pausedAt"
    | "pausedReason"
    | "resumeState"
    | "completedAt"
    | "cancelledAt",
>(run: ADKRun, key: Key, value: ADKRun[Key]): void {
  if (value === undefined) {
    delete run[key];
    return;
  }
  run[key] = value;
}

function syncGoalAwareContinuationProgress(
  run: ADKRun,
  options: GoalAwareRunContinuationOptions,
): void {
  options.syncActiveRun(run, shouldWaitForRunContinuation(run));
  const failMessage = runErrorDisplayMessage(run);
  if (failMessage) {
    options.setErrorMessage?.(failMessage);
  }
}

export function buildRunObservationSignature(run: ADKRun | undefined): string {
  if (!run) return "";
  return JSON.stringify({
    status: run.status,
    resumeState: run.resumeState ?? "",
    pauseRequestedAt: run.pauseRequestedAt ?? "",
    pausedAt: run.pausedAt ?? "",
    pausedReason: run.pausedReason ?? "",
    updatedAt: run.updatedAt ?? "",
    workMode: run.workMode ?? "",
    objective: run.objective ?? "",
    workflowStatus: run.workflowStatus ?? "",
    workflowCursor: run.workflowCursor ?? 0,
    workflowPlan: (run.workflowPlan ?? []).map((step) => ({
      taskId: step.taskId ?? "",
      title: step.title,
      status: step.status,
      childRunId: step.childRunId ?? "",
      iteration: step.iteration ?? 0,
    })),
    toolCalls: (run.toolCalls ?? []).map((toolCall) => ({
      id: toolCall.id,
      status: toolCall.status,
      updatedAt: toolCall.updatedAt ?? "",
      completedAt: toolCall.completedAt ?? "",
    })),
    pendingApprovals: (run.pendingApprovals ?? []).map((approval) => ({
      id: approval.id,
      toolName: approval.toolName,
      status: approval.status,
      updatedAt: approval.updatedAt ?? "",
    })),
  });
}

export function isBlockingRunStatus(status: string | undefined): boolean {
  return isActiveRunStatus(status) && !isTerminalRunStatus(status);
}

export function hasPendingRunApproval(run: ADKRun | undefined): boolean {
  if (!run) return false;
  if (run.status === "PENDING_APPROVAL") {
    return true;
  }
  return (run.pendingApprovals ?? []).some((approval) => {
    const status = String(approval.status ?? "")
      .trim()
      .toUpperCase();
    return (
      status === "" || status === "PENDING" || status === "PENDING_APPROVAL"
    );
  });
}
