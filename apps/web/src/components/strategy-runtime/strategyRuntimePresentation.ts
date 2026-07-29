import type {
  StrategyDefinitionSyncStatus,
  StrategyInstanceBindingDocument,
  StrategyInstanceItem,
} from "@/types";

import { ApiClientError } from "../../composables/apiClient";
import { formatLocalDateTime } from "../../utils/dateTime";
import { normalizeText } from "./strategyRuntimeInstanceBinding";

export type StrategyAction = "start" | "pause" | "stop";

function formatTimestampParts(value: unknown): string {
  const normalized = normalizeText(value);
  if (normalized === "") return "暂无";
  const parsed = new Date(normalized);
  if (Number.isNaN(parsed.getTime())) {
    return normalized.replace("T", " ").replace(".000Z", "Z");
  }
  return formatLocalDateTime(parsed, normalized);
}

export function formatTimestamp(value: unknown): string {
  return formatTimestampParts(value);
}

export function formatTimestampTooltip(value: unknown): string {
  return formatTimestampParts(value);
}

export function formatStrategyStatus(status: StrategyInstanceItem["status"] | string): string {
  switch (status) {
    case "RUNNING": return "运行中";
    case "PAUSED": return "已暂停";
    case "STOPPED": return "已停止";
    default: return normalizeText(status) || "未知";
  }
}

export function displayStrategyStatus(strategy: StrategyInstanceItem): StrategyInstanceItem["status"] {
  return strategy.runtimeObservation?.actualStatus ?? strategy.status;
}

export function formatStrategyDefinitionSyncSummary(
  sync: StrategyDefinitionSyncStatus | null | undefined,
): string {
  if (sync == null) return "";
  return sync.isLatest
    ? `已同步至 v${sync.latestVersion}`
    : `待刷新 v${sync.appliedVersion} -> v${sync.latestVersion}`;
}

export function formatStrategyExecutionMode(
  mode: StrategyInstanceBindingDocument["executionMode"] | string | null | undefined,
): string {
  return normalizeText(mode) === "notify_only" ? "仅通知" : "确认执行";
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (value === null || typeof value !== "object" || Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

export function readCompiledIndicatorCount(strategy: StrategyInstanceItem): number | null {
  const compiledRequirements = asRecord(strategy.params.compiledRequirements);
  if (compiledRequirements === null) return null;
  return Array.isArray(compiledRequirements.indicators)
    ? compiledRequirements.indicators.length
    : null;
}

export function readCompiledHookCount(strategy: StrategyInstanceItem): number | null {
  return Array.isArray(strategy.params.compiledHooks) ? strategy.params.compiledHooks.length : null;
}

function formatActionLabel(action: StrategyAction): string {
  switch (action) {
    case "start": return "启动";
    case "pause": return "暂停";
    case "stop": return "停止";
    default: return action;
  }
}

export function formatStrategyActionError(action: StrategyAction, error: unknown): string {
  if (
    action === "start" &&
    error instanceof ApiClientError &&
    error.code === "BAD_REQUEST" &&
    error.message.includes("运行实例 PineTS Worker 已达到上限")
  ) {
    return "运行实例 PineTS Worker 已达到上限。请停止其他运行实例，或打开“设置 > PineTS Worker”调高“运行实例 Worker 最大值”后再启动。";
  }
  return error instanceof Error ? error.message : `执行${formatActionLabel(action)}失败。`;
}
