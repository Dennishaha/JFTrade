import type { ADKRun } from "@/contracts";

import { isActiveRunStatus, isTerminalRunStatus } from "./adkChatPresentation";

export const PROVISIONAL_SESSION_KEY = "__adk_provisional_session__";

export type QueuedChatMessageMode = "queued" | "interrupt";

export interface QueuedChatMessage {
  id: string;
  sessionKey: string;
  text: string;
  mode: QueuedChatMessageMode;
  createdAt: string;
}

export interface ActiveChatRunState {
  runId: string;
  sessionId: string;
  status: string;
  lastObservedToolSignature: string;
  waitingForContinuation: boolean;
}

export function buildQueueSessionKey(sessionId: string | undefined): string {
  return sessionId?.trim() ? sessionId.trim() : PROVISIONAL_SESSION_KEY;
}

export function createQueuedChatMessage(
  text: string,
  sessionKey: string,
  mode: QueuedChatMessageMode,
): QueuedChatMessage {
  return {
    id: `queued-chat-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    sessionKey,
    text,
    mode,
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

export function buildRunObservationSignature(run: ADKRun | undefined): string {
  if (!run) return "";
  return JSON.stringify({
    status: run.status,
    resumeState: run.resumeState ?? "",
    updatedAt: run.updatedAt ?? "",
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
    const status = String(approval.status ?? "").trim().toUpperCase();
    return status === "" || status === "PENDING" || status === "PENDING_APPROVAL";
  });
}
