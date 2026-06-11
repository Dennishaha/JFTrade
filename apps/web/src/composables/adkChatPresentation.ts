import type { ADKRun } from "@/contracts";

export function isActiveRunStatus(status: string | undefined): boolean {
  return (
    status === "RUNNING" ||
    status === "PENDING" ||
    status === "PENDING_APPROVAL"
  );
}

export function isTerminalRunStatus(status: string | undefined): boolean {
  return (
    status === "COMPLETED" ||
    status === "FAILED" ||
    status === "TIMED_OUT" ||
    status === "CANCELLED" ||
    status === "DENIED"
  );
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

export function normalizedDisplayStatus(
  status: string | undefined,
): string | undefined {
  if (status === "SUCCEEDED") return "COMPLETED";
  return status;
}

export function firstFailedToolCall(
  run: ADKRun | undefined,
): ADKRun["toolCalls"][number] | undefined {
  if (!run) return undefined;
  return (run.toolCalls ?? []).find(
    (toolCall) =>
      toolCall.status === "TIMED_OUT" ||
      toolCall.status === "FAILED" ||
      toolCall.status === "CANCELLED",
  );
}

export function toolCallErrorSummary(
  toolCall: ADKRun["toolCalls"][number] | undefined,
): string {
  if (!toolCall) return "";
  if (toolCall.error?.trim()) return toolCall.error.trim();
  switch (toolCall.status) {
    case "TIMED_OUT":
      return "Tool execution timed out";
    case "CANCELLED":
      return "Tool execution was cancelled";
    case "FAILED":
      return "Tool execution failed";
    default:
      return "";
  }
}

export function runTerminalMessage(run: ADKRun | undefined): string {
  if (!run) return "";
  const toolFailureMessage = toolCallErrorSummary(firstFailedToolCall(run));
  switch (run.status) {
    case "FAILED":
      return (
        run.failureReason || run.message || toolFailureMessage || "Run failed"
      );
    case "TIMED_OUT":
      return (
        run.failureReason ||
        run.message ||
        toolFailureMessage ||
        "Run timed out"
      );
    case "CANCELLED":
      return (
        run.failureReason ||
        run.message ||
        toolFailureMessage ||
        "Run cancelled"
      );
    case "DENIED":
      return (
        run.failureReason ||
        run.message ||
        "Approval denied and the run did not continue"
      );
    default:
      return toolFailureMessage;
  }
}
