import type { ADKToolCall, ADKToolDescriptor } from "@/types";

import { toolCallErrorSummary } from "@/composables/adk/adkChatPresentation";
import { deriveToolGroupStatus } from "@/composables/adk/adkTimeline";

export interface ADKToolActionClass {
  category: string;
  verb: string;
  unit: string;
  icon: string;
}

const QUERY_ACTION: ADKToolActionClass = {
  category: "query",
  verb: "查询",
  unit: "项",
  icon: "fa-solid fa-list",
};

const EXACT_TOOL_ACTIONS: Record<string, ADKToolActionClass> = {
  "market.search": {
    category: "search",
    verb: "搜索",
    unit: "个结果",
    icon: "fa-solid fa-magnifying-glass",
  },
  "market.provider.select": {
    category: "settings",
    verb: "修改设置",
    unit: "项",
    icon: "fa-solid fa-gear",
  },
  "strategy.pine_spec": {
    category: "read",
    verb: "读取",
    unit: "个章节",
    icon: "fa-solid fa-book-open",
  },
  "strategy.validate_pine": {
    category: "validate",
    verb: "校验",
    unit: "个脚本",
    icon: "fa-solid fa-circle-check",
  },
  "strategy.research_backtest": {
    category: "backtest",
    verb: "回测",
    unit: "次",
    icon: "fa-solid fa-flask",
  },
  "strategy.optimize": {
    category: "optimize",
    verb: "优化",
    unit: "次",
    icon: "fa-solid fa-sliders",
  },
  "strategy.save_draft": {
    category: "save",
    verb: "保存",
    unit: "个",
    icon: "fa-solid fa-floppy-disk",
  },
  "strategy.save_definition": {
    category: "save",
    verb: "保存",
    unit: "个",
    icon: "fa-solid fa-floppy-disk",
  },
  "strategy.update_instance_mode": {
    category: "update",
    verb: "更新",
    unit: "个实例",
    icon: "fa-solid fa-pen-to-square",
  },
  "strategy.instance_risk.update": {
    category: "risk",
    verb: "更新风控",
    unit: "项",
    icon: "fa-solid fa-shield-halved",
  },
  "strategy.instantiate": {
    category: "lifecycle",
    verb: "实例化",
    unit: "个",
    icon: "fa-solid fa-power-off",
  },
  "strategy.instance_start": {
    category: "lifecycle",
    verb: "启动",
    unit: "个实例",
    icon: "fa-solid fa-power-off",
  },
  "strategy.instance_stop": {
    category: "lifecycle",
    verb: "停止",
    unit: "个实例",
    icon: "fa-solid fa-power-off",
  },
  "strategy.instance_refresh_definition": {
    category: "lifecycle",
    verb: "刷新",
    unit: "个实例",
    icon: "fa-solid fa-rotate",
  },
  "backtest.cancel": {
    category: "cancel",
    verb: "取消",
    unit: "个",
    icon: "fa-solid fa-ban",
  },
  "execution.order_place": {
    category: "order",
    verb: "下单",
    unit: "笔",
    icon: "fa-solid fa-paper-plane",
  },
  "execution.combo_place": {
    category: "order",
    verb: "下单",
    unit: "笔",
    icon: "fa-solid fa-paper-plane",
  },
  "execution.order_cancel": {
    category: "cancel",
    verb: "撤单",
    unit: "笔",
    icon: "fa-solid fa-ban",
  },
  "execution.combo_cancel": {
    category: "cancel",
    verb: "撤单",
    unit: "笔",
    icon: "fa-solid fa-ban",
  },
  "execution.order_preview": {
    category: "preview",
    verb: "预览",
    unit: "个",
    icon: "fa-solid fa-eye",
  },
  "execution.combo_preview": {
    category: "preview",
    verb: "预览",
    unit: "个",
    icon: "fa-solid fa-eye",
  },
  "interaction.request_user": {
    category: "ask",
    verb: "提问",
    unit: "个问题",
    icon: "fa-regular fa-circle-question",
  },
  "tasks.create": {
    category: "task",
    verb: "创建任务",
    unit: "个",
    icon: "fa-solid fa-list-check",
  },
  "tasks.update": {
    category: "task",
    verb: "更新任务",
    unit: "个",
    icon: "fa-solid fa-list-check",
  },
  "tasks.delete": {
    category: "task",
    verb: "删除任务",
    unit: "个",
    icon: "fa-solid fa-list-check",
  },
  "memory.remember": {
    category: "memory",
    verb: "写入记忆",
    unit: "条",
    icon: "fa-solid fa-brain",
  },
  "memory.forget": {
    category: "memory",
    verb: "删除记忆",
    unit: "条",
    icon: "fa-solid fa-brain",
  },
  load_skill: {
    category: "skill",
    verb: "加载技能",
    unit: "个",
    icon: "fa-solid fa-puzzle-piece",
  },
};

export function classifyToolAction(toolName: string): ADKToolActionClass {
  const normalized = toolName.trim();
  return EXACT_TOOL_ACTIONS[normalized] ?? QUERY_ACTION;
}

const ARGUMENT_PRIORITY_KEYS = [
  "symbol",
  "symbols",
  "instrumentId",
  "instrumentIds",
  "query",
  "keywords",
  "name",
  "definitionId",
  "instanceId",
  "runId",
  "taskId",
  "accountId",
  "group",
  "market",
  "interval",
  "scope",
  "key",
];

const ARGUMENT_NEVER_KEYS = new Set([
  "script",
  "code",
  "content",
  "source",
  "prompt",
  "objective",
]);

const ARGUMENT_MAX_LENGTH = 60;

export function toolPrimaryArgument(
  input: Record<string, unknown> | undefined,
): string {
  if (!input || typeof input !== "object") return "";
  for (const key of ARGUMENT_PRIORITY_KEYS) {
    if (ARGUMENT_NEVER_KEYS.has(key)) continue;
    const value = input[key];
    const text = formatArgumentValue(value);
    if (text !== "") {
      return truncateTraceText(text, ARGUMENT_MAX_LENGTH);
    }
  }
  return "";
}

function formatArgumentValue(value: unknown): string {
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  if (Array.isArray(value)) {
    const items = value
      .map((item) => formatArgumentValue(item))
      .filter((item) => item !== "");
    if (items.length === 0) return "";
    if (items.length <= 3) return items.join("、");
    return `${items.slice(0, 3).join("、")} 等 ${items.length} 个`;
  }
  return "";
}

const RESULT_ARRAY_KEYS = [
  "items",
  "data",
  "results",
  "candles",
  "bars",
  "snapshots",
  "orders",
  "fills",
  "positions",
  "news",
  "articles",
  "runs",
  "events",
  "accounts",
  "instruments",
  "members",
  "groups",
  "sections",
  "examples",
  "hooks",
  "errors",
  "warnings",
  "candidates",
  "tasks",
  "logs",
  "entries",
  "cashFlows",
  "fees",
  "subscriptions",
  "providers",
  "definitions",
  "instances",
  "versions",
];

export function toolResultMeta(
  toolCall: Pick<ADKToolCall, "status" | "output" | "error">,
): string {
  const status = (toolCall.status ?? "").trim().toUpperCase();
  if (
    status === "FAILED" ||
    status === "TIMED_OUT" ||
    status === "CANCELLED" ||
    status === "DENIED"
  ) {
    return truncateTraceText(toolCallErrorSummary(toolCall as ADKToolCall), 48);
  }
  if (status !== "SUCCEEDED" && status !== "COMPLETED") return "";
  const output = toolCall.output;
  if (Array.isArray(output)) return `${output.length} 条`;
  if (!isPlainRecord(output)) return "";
  if (typeof output.ok === "boolean") return output.ok ? "通过" : "未通过";
  const count = firstResultArrayLength(output);
  return count == null ? "" : `${count} 条`;
}

function firstResultArrayLength(output: Record<string, unknown>): number | null {
  for (const key of RESULT_ARRAY_KEYS) {
    const value = output[key];
    if (Array.isArray(value)) return value.length;
  }
  for (const value of Object.values(output)) {
    if (!isPlainRecord(value)) continue;
    for (const key of RESULT_ARRAY_KEYS) {
      const nested = value[key];
      if (Array.isArray(nested)) return nested.length;
    }
  }
  return null;
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export interface ADKToolGroupSummary {
  parts: string[];
  total: number;
  status: string;
  durationMs?: number;
}

export function summarizeToolGroup(
  toolCalls: ADKToolCall[],
): ADKToolGroupSummary {
  const counts = new Map<string, { action: ADKToolActionClass; count: number }>();
  for (const toolCall of toolCalls) {
    const action = classifyToolAction(toolCall.toolName);
    const existing = counts.get(action.category);
    if (existing) {
      existing.count += 1;
    } else {
      counts.set(action.category, { action, count: 1 });
    }
  }
  const parts = [...counts.values()].map(
    ({ action, count }) => `已${action.verb}了 ${count} ${action.unit}`,
  );
  const summary: ADKToolGroupSummary = {
    parts,
    total: toolCalls.length,
    status: deriveToolGroupStatus(toolCalls),
  };
  const durationMs = toolGroupElapsedMs(toolCalls);
  if (durationMs != null) summary.durationMs = durationMs;
  return summary;
}

function toolGroupElapsedMs(toolCalls: ADKToolCall[]): number | undefined {
  let start = Number.POSITIVE_INFINITY;
  let end = Number.NEGATIVE_INFINITY;
  for (const toolCall of toolCalls) {
    const startMs = parseTraceTime(toolCall.startedAt ?? toolCall.createdAt);
    const endMs = parseTraceTime(toolCall.completedAt ?? toolCall.updatedAt);
    if (startMs != null) start = Math.min(start, startMs);
    if (endMs != null) end = Math.max(end, endMs);
  }
  if (Number.isFinite(start) && Number.isFinite(end) && end >= start) {
    return end - start;
  }
  const durations = toolCalls
    .map((toolCall) => toolCall.durationMs)
    .filter((value): value is number => value != null && Number.isFinite(value));
  if (durations.length === 0) return undefined;
  return durations.reduce((total, value) => total + value, 0);
}

export function parseTraceTime(value: string | undefined): number | null {
  const trimmed = (value ?? "").trim();
  if (trimmed === "") return null;
  const parsed = Date.parse(trimmed);
  return Number.isNaN(parsed) ? null : parsed;
}

export function formatTraceDuration(durationMs: number | undefined): string {
  if (durationMs == null || Number.isNaN(durationMs) || durationMs < 0) return "";
  if (durationMs < 1000) return `${Math.round(durationMs)}ms`;
  const totalSeconds = durationMs / 1000;
  if (totalSeconds < 10) {
    const fixed = totalSeconds.toFixed(1);
    return fixed.endsWith(".0") ? `${Math.round(totalSeconds)}s` : `${fixed}s`;
  }
  if (totalSeconds < 60) return `${Math.round(totalSeconds)}s`;
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = Math.round(totalSeconds % 60);
  if (minutes < 60) return `${minutes}m${seconds}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h${minutes % 60}m`;
}

export function toolTraceRowLabel(
  toolCall: Pick<ADKToolCall, "toolName" | "input">,
  descriptor: ADKToolDescriptor | undefined,
): { label: string; argument: string } {
  const displayName = (descriptor?.displayName ?? "").trim();
  const argument = toolPrimaryArgument(toolCall.input);
  if (displayName !== "") {
    return { label: displayName, argument };
  }
  return {
    label: classifyToolAction(toolCall.toolName).verb,
    argument: argument === "" ? toolCall.toolName : argument,
  };
}

export function truncateTraceText(value: string, maxLength: number): string {
  const trimmed = value.trim();
  if (trimmed.length <= maxLength) return trimmed;
  return `${trimmed.slice(0, Math.max(0, maxLength - 1))}…`;
}
