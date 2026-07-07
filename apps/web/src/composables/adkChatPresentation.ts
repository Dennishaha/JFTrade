import type { ADKRun } from "@/contracts";

export interface ADKRunErrorSummary {
  title: string;
  detail: string;
  code?: string;
  runId?: string;
  tone: "error" | "warning" | "info" | "muted";
}

const RUN_ERROR_DETAIL_MAX_LENGTH = 360;

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

export function isUserPausedGoalRun(run: ADKRun | undefined): boolean {
  return (
    run?.status === "PAUSED" &&
    String(run.workMode ?? "")
      .trim()
      .toLowerCase() === "loop" &&
    (run.pausedReason === "user" ||
      run.resumeState === "user_paused" ||
      String(run.pausedAt ?? "").trim() !== "")
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
    case "PAUSED":
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

export function runErrorSummary(run: ADKRun | undefined): ADKRunErrorSummary | null {
  if (!run) return null;
  const status = String(run.status ?? "").trim().toUpperCase();
  if (status === "COMPLETED" || status === "SUCCEEDED" || status === "DONE") {
    return null;
  }
  const code = String(run.errorCode ?? "").trim();
  const failureReason = String(run.failureReason ?? "").trim();
  const failedToolError = toolCallErrorSummary(firstFailedToolCall(run));
  const terminalErrorStatus = isTerminalErrorStatus(status);
  if (!terminalErrorStatus && !code && !failureReason && !failedToolError) {
    return null;
  }
  const detail = truncateRunErrorDetail(
    firstNonEmpty(
      failureReason,
      terminalErrorStatus || code ? run.message : undefined,
      failedToolError,
      fallbackRunErrorDetail(status),
    ),
  );
  return {
    title: localizedRunErrorTitle(status, code, detail),
    detail,
    ...(code ? { code } : {}),
    ...(String(run.id ?? "").trim()
      ? { runId: String(run.id ?? "").trim() }
      : {}),
    tone: runErrorTone(status),
  };
}

export function runErrorDisplayMessage(run: ADKRun | undefined): string {
  const summary = runErrorSummary(run);
  if (!summary) return "";
  const lines = [summary.title];
  if (summary.detail && summary.detail !== summary.title) {
    lines.push(summary.detail);
  }
  const meta = [
    summary.code ? `错误码：${summary.code}` : "",
    summary.runId ? `Run：${summary.runId}` : "",
  ].filter(Boolean);
  if (meta.length > 0) {
    lines.push(meta.join(" · "));
  }
  return lines.join("\n");
}

export function runTerminalMessage(run: ADKRun | undefined): string {
  return runErrorSummary(run)?.detail ?? "";
}

function isTerminalErrorStatus(status: string): boolean {
  return (
    status === "FAILED" ||
    status === "TIMED_OUT" ||
    status === "DENIED" ||
    status === "CANCELLED" ||
    status === "CANCELED"
  );
}

function localizedRunErrorTitle(status: string, code: string, detail: string): string {
  const normalizedDetail = detail.toLowerCase();
  const normalizedCode = code.toUpperCase();
  if (
    normalizedCode === "MODEL_CALL_FAILED" &&
    (normalizedDetail.includes("insufficient balance") ||
      normalizedDetail.includes("402"))
  ) {
    return "模型调用失败：服务商余额不足";
  }
  if (normalizedCode === "PARENT_RUN_TERMINATED") {
    return "父工作流已终止，子智能体已取消";
  }
  if (status === "TIMED_OUT" || normalizedCode === "RUN_TIMED_OUT") {
    return "运行超时";
  }
  if (status === "DENIED") {
    return "审批被拒绝，运行未继续";
  }
  if (status === "CANCELLED" || status === "CANCELED") {
    return "运行已取消";
  }
  if (status === "FAILED") {
    return "运行失败";
  }
  return "运行异常";
}

function runErrorTone(status: string): ADKRunErrorSummary["tone"] {
  switch (status) {
    case "FAILED":
    case "TIMED_OUT":
    case "DENIED":
      return "error";
    case "CANCELLED":
    case "CANCELED":
      return "warning";
    default:
      return "muted";
  }
}

function fallbackRunErrorDetail(status: string): string {
  switch (status) {
    case "FAILED":
      return "Run failed";
    case "TIMED_OUT":
      return "Run timed out";
    case "CANCELLED":
    case "CANCELED":
      return "Run cancelled";
    case "DENIED":
      return "Approval denied and the run did not continue";
    default:
      return "";
  }
}

function firstNonEmpty(...values: Array<string | undefined>): string {
  for (const value of values) {
    const trimmed = String(value ?? "").trim();
    if (trimmed !== "") return trimmed;
  }
  return "";
}

function truncateRunErrorDetail(value: string): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= RUN_ERROR_DETAIL_MAX_LENGTH) return normalized;
  return `${normalized.slice(0, RUN_ERROR_DETAIL_MAX_LENGTH - 3)}...`;
}
