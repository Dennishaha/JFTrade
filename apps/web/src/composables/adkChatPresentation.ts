import type { ADKRun } from "@jftrade/ui-contracts";

export interface ADKAssistantMessageState {
  id: string;
  role: "assistant";
  content: string;
  reasoningContent?: string;
  reasoningExpanded?: boolean;
  toolProgress?: string;
  preToolContent?: string | undefined;
  preToolReasoning?: string | undefined;
  run?: ADKRun | undefined;
  toolSummaryExpanded?: boolean | undefined;
  expandedToolCallIds?: string[] | undefined;
}

export interface ADKRunPresentationStateTarget {
  run?: ADKRun | undefined;
  toolSummaryExpanded?: boolean | undefined;
  expandedToolCallIds?: string[] | undefined;
}

export function createAssistantMessageState(id: string): ADKAssistantMessageState {
  return {
    id,
    role: "assistant",
    content: "",
    reasoningContent: "",
    reasoningExpanded: false,
    toolProgress: "",
    toolSummaryExpanded: false,
    expandedToolCallIds: [],
  };
}

export function isActiveRunStatus(status: string | undefined): boolean {
  return status === "RUNNING" || status === "PENDING" || status === "PENDING_APPROVAL";
}

export function isTerminalRunStatus(status: string | undefined): boolean {
  return status === "COMPLETED"
    || status === "FAILED"
    || status === "TIMED_OUT"
    || status === "CANCELLED"
    || status === "DENIED";
}

export function runStatusTone(status: string | undefined): string {
  switch ((status ?? "").toUpperCase()) {
    case "COMPLETED":
    case "SUCCEEDED":
      return "success";
    case "FAILED":
    case "TIMED_OUT":
    case "DENIED":
      return "error";
    case "PENDING_APPROVAL":
      return "warning";
    case "RUNNING":
    case "PENDING":
      return "info";
    case "CANCELLED":
      return "muted";
    default:
      return "muted";
  }
}

export function normalizedDisplayStatus(status: string | undefined): string | undefined {
  if (status === "SUCCEEDED") return "COMPLETED";
  return status;
}

export function runTerminalMessage(run: ADKRun | undefined): string {
  if (!run) return "";
  switch (run.status) {
    case "FAILED":
      return run.failureReason || run.message || "运行失败";
    case "TIMED_OUT":
      return run.failureReason || run.message || "运行超时";
    case "CANCELLED":
      return run.failureReason || run.message || "运行已取消";
    case "DENIED":
      return run.failureReason || run.message || "审批已拒绝，本次运行未执行写操作";
    default:
      return "";
  }
}

export function syncRunPresentationState(
  message: ADKRunPresentationStateTarget,
  run: ADKRun | undefined,
): void {
  message.run = run;
  const toolCalls = run?.toolCalls ?? [];
  const validIds = new Set(toolCalls.map((toolCall) => toolCall.id));
  const expandedToolCallIds = (message.expandedToolCallIds ?? []).filter((id) => validIds.has(id));

  if (toolCalls.length === 0) {
    message.toolSummaryExpanded = false;
    message.expandedToolCallIds = [];
    return;
  }

  if (message.toolSummaryExpanded === undefined) {
    message.toolSummaryExpanded = false;
  }

  message.expandedToolCallIds = expandedToolCallIds;
}
