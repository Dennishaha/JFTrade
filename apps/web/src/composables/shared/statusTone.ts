import { formatGenericStatusLabel } from "@/composables/shared/consoleDataFormatting";

/**
 * 「状态词 → Vuetify 颜色语义」的共享映射，是各域状态 chip 配色的唯一事实来源。
 *
 * 只收录跨域配色一致的状态词；域间存在分歧的状态不进此表，由调用方保留
 * 自己的分支，例如：
 * - CANCELLED：ADK 域用 grey，回测域用 warning；
 * - QUEUED：ADK 优化任务用 info，回测域用 warning；
 * - 未收录状态的兜底色（default / "" / error）各域不同。
 */
export interface StatusTone {
  /** Vuetify 颜色名；未收录的状态返回 "default"（与既有调用方的兜底一致）。 */
  color: string;
  /** 通用中文状态文案（与 formatGenericStatusLabel 一致）。 */
  label: string;
}

const STATUS_TONE_COLORS: Record<string, string> = {
  COMPLETED: "success",
  DONE: "success",
  SUCCEEDED: "success",
  APPROVED: "success",
  ENABLED: "success",
  RUNNING: "info",
  IN_PROGRESS: "info",
  PENDING: "info",
  FAILED: "error",
  TIMED_OUT: "error",
  DENIED: "error",
  PENDING_APPROVAL: "warning",
  PENDING_INPUT: "warning",
  BLOCKED: "warning",
  PAUSED: "warning",
};

/** 规范化状态词：去首尾空白、连字符/空白归一为下划线、转大写。 */
export function normalizeStatusWord(status: string | null | undefined): string {
  return (status ?? "")
    .trim()
    .replace(/[\s-]+/g, "_")
    .toUpperCase();
}

export function statusTone(status: string | null | undefined): StatusTone {
  return {
    color: STATUS_TONE_COLORS[normalizeStatusWord(status)] ?? "default",
    label: formatGenericStatusLabel(status),
  };
}
