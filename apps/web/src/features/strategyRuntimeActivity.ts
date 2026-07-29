import type { StrategyAuditEntryDocument } from "@/contracts";

import { formatLocalDateTime } from "../utils/dateTime";

export type StrategyActivityTab = "logs" | "audit";
export type StrategyActivityLevel = "all" | "error" | "warning" | "info";
type ConcreteActivityLevel = Exclude<StrategyActivityLevel, "all">;

interface StrategyTimestampParts {
  display: string;
  timestampMs: number | null;
}

export interface StrategyLogViewEntry {
  raw: string;
  message: string;
  at: string;
  timestampMs: number | null;
  level: ConcreteActivityLevel;
}

export interface StrategyAuditViewEntry extends StrategyAuditEntryDocument {
  detailText: string;
  label: string;
  level: ConcreteActivityLevel;
  timestampMs: number | null;
}

export interface StrategyActivityDetailView {
  title: string;
  kindLabel: string;
  summary: string;
  detail: string;
  at: string;
  tooltip: string;
  level: ConcreteActivityLevel;
  rawKind?: string;
}

function normalizeText(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

export function formatTimestampParts(value: unknown): StrategyTimestampParts {
  const normalized = normalizeText(value);
  if (normalized === "") return { display: "暂无", timestampMs: null };
  const parsed = new Date(normalized);
  if (Number.isNaN(parsed.getTime())) {
    return {
      display: normalized.replace("T", " ").replace(".000Z", "Z"),
      timestampMs: null,
    };
  }
  return {
    display: formatLocalDateTime(parsed, normalized),
    timestampMs: parsed.getTime(),
  };
}

export const formatTimestamp = (value: unknown): string =>
  formatTimestampParts(value).display;
export const formatTimestampTooltip = formatTimestamp;

export function sortActivityEntriesByTime<
  T extends { timestampMs: number | null },
>(items: T[]): T[] {
  return items
    .map((item, index) => ({ item, index }))
    .sort((left, right) => {
      const leftTime = left.item.timestampMs ?? Number.NEGATIVE_INFINITY;
      const rightTime = right.item.timestampMs ?? Number.NEGATIVE_INFINITY;
      return rightTime !== leftTime ? rightTime - leftTime : right.index - left.index;
    })
    .map(({ item }) => item);
}

export function formatAuditKind(kind: unknown): string {
  const normalized = normalizeText(kind).toLowerCase();
  return ({
    instantiated: "已实例化",
    "binding.updated": "已更新绑定",
    created: "已创建",
    started: "已启动",
    running: "运行中",
    paused: "已暂停",
    stopped: "已停止",
    failed: "执行失败",
    risk_rejected: "风控拒单",
    risk_monitor: "风控观察",
  } as Record<string, string>)[normalized] ?? (normalizeText(kind) || "未知");
}

export function formatStrategyActivityLevel(level: StrategyActivityLevel): string {
  return {
    error: "高优先",
    warning: "需关注",
    info: "常规",
    all: "全部",
  }[level];
}

function classifySignal(
  signal: string,
  errorKeywords: string[],
  warningKeywords: string[],
): ConcreteActivityLevel {
  if (errorKeywords.some((keyword) => signal.includes(keyword))) return "error";
  if (warningKeywords.some((keyword) => signal.includes(keyword))) return "warning";
  return "info";
}

export function classifyStrategyLogLevel(message: string): ConcreteActivityLevel {
  return classifySignal(
    normalizeText(message).toLowerCase(),
    ["panic", "fatal", "error", "failed", "exception", "reject", "denied", "timeout"],
    ["warn", "warning", "paused", "pause", "stopped", "stop", "retry", "skip", "throttle"],
  );
}

export function classifyStrategyAuditLevel(
  entry: StrategyAuditEntryDocument,
): ConcreteActivityLevel {
  return classifySignal(
    `${normalizeText(entry.kind)} ${normalizeText(entry.detail)}`.toLowerCase(),
    ["failed", "panic", "error", "exception", "reject", "denied", "timeout"],
    ["paused", "pause", "stopped", "stop", "retry", "warning", "warn"],
  );
}

export function parseStrategyLogEntry(entry: string): StrategyLogViewEntry {
  const raw = normalizeText(entry);
  const matched = raw.match(
    /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s*(.*)$/,
  );
  const at = matched?.[1] ?? "";
  const message = normalizeText(matched?.[2]) || raw;
  return {
    raw: entry,
    message,
    at,
    timestampMs: at === "" ? null : formatTimestampParts(at).timestampMs,
    level: classifyStrategyLogLevel(message || raw),
  };
}

export function buildLogActivityDetail(
  entry: StrategyLogViewEntry,
): StrategyActivityDetailView {
  return {
    title: "运行日志",
    kindLabel: "日志详情",
    summary: entry.message,
    detail: entry.raw,
    at: formatTimestamp(entry.at),
    tooltip: formatTimestampTooltip(entry.at),
    level: entry.level,
  };
}

export function buildAuditActivityDetail(
  entry: StrategyAuditViewEntry,
): StrategyActivityDetailView {
  return {
    title: entry.label,
    kindLabel: "审计详情",
    summary: entry.detailText,
    detail: [
      `instanceId: ${entry.instanceId}`,
      `kind: ${entry.kind}`,
      `detail: ${entry.detailText}`,
      `at: ${entry.at}`,
    ].join("\n"),
    at: formatTimestamp(entry.at),
    tooltip: formatTimestampTooltip(entry.at),
    level: entry.level,
    rawKind: entry.kind,
  };
}
