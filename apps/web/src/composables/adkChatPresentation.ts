import type { ADKRun } from "@jftrade/ui-contracts";

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
