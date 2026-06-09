import type {
  ADKApproval,
  ADKRun,
  ADKTimelineEntry,
} from "@jftrade/ui-contracts";

export interface ADKTimelineEntryState extends ADKTimelineEntry {
  reasoningExpanded?: boolean;
  toolSummaryExpanded?: boolean;
  expandedToolCallIds?: string[];
}

export function createTimelineEntryState(
  entry: ADKTimelineEntry,
): ADKTimelineEntryState {
  const state: ADKTimelineEntryState = { ...entry };
  if (entry.kind === "assistant_reasoning") {
    state.reasoningExpanded = false;
  }
  if (entry.kind === "tool_group") {
    state.toolSummaryExpanded = false;
    state.expandedToolCallIds = [];
  }
  return state;
}

export function replaceTimelineEntries(
  entries: ADKTimelineEntry[] | undefined,
  previous: ADKTimelineEntryState[] = [],
): ADKTimelineEntryState[] {
  const previousById = new Map(previous.map((entry) => [entry.id, entry]));
  return sortTimelineEntries(
    (entries ?? []).map((entry) =>
      mergeTimelineEntry(previousById.get(entry.id), entry),
    ),
  );
}

export function upsertTimelineEntry(
  entries: ADKTimelineEntryState[],
  incoming: ADKTimelineEntry,
): ADKTimelineEntryState[] {
  const index = entries.findIndex((entry) => entry.id === incoming.id);
  if (index < 0) {
    return sortTimelineEntries([...entries, createTimelineEntryState(incoming)]);
  }
  const next = [...entries];
  next[index] = mergeTimelineEntry(next[index], incoming);
  return sortTimelineEntries(next);
}

export function sortTimelineEntries(
  entries: ADKTimelineEntryState[],
): ADKTimelineEntryState[] {
  return [...entries].sort((left, right) => {
    const leftTime = Date.parse(left.createdAt ?? "");
    const rightTime = Date.parse(right.createdAt ?? "");
    if (!Number.isNaN(leftTime) && !Number.isNaN(rightTime) && leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    if ((left.sequence ?? 0) !== (right.sequence ?? 0)) {
      return (left.sequence ?? 0) - (right.sequence ?? 0);
    }
    if ((left.createdAt ?? "") !== (right.createdAt ?? "")) {
      return (left.createdAt ?? "").localeCompare(right.createdAt ?? "");
    }
    return left.id.localeCompare(right.id);
  });
}

export function buildTimelineRun(entry: ADKTimelineEntryState): ADKRun {
  const toolCalls = [...(entry.toolCalls ?? [])];
  return {
    id: entry.runId ?? entry.id,
    sessionId: entry.sessionId,
    agentId: "",
    status: deriveToolGroupStatus(toolCalls),
    message: "",
    toolCalls,
    pendingApprovals: [],
    createdAt: entry.createdAt,
    updatedAt: entry.createdAt,
  };
}

export function approvalsForGroup(
  entry: ADKTimelineEntryState,
): ADKApproval[] {
  return [...(entry.approvals ?? [])];
}

function mergeTimelineEntry(
  existing: ADKTimelineEntryState | undefined,
  incoming: ADKTimelineEntry,
): ADKTimelineEntryState {
  const base = existing ?? createTimelineEntryState(incoming);
  const next: ADKTimelineEntryState = { ...base, ...incoming };
  if (incoming.toolCalls) {
    next.toolCalls = [...incoming.toolCalls];
  } else if (base.toolCalls) {
    next.toolCalls = [...base.toolCalls];
  }
  if (incoming.approvals) {
    next.approvals = [...incoming.approvals];
  } else if (base.approvals) {
    next.approvals = [...base.approvals];
  }
  return next;
}

function deriveToolGroupStatus(toolCalls: ADKRun["toolCalls"]): string {
  if (toolCalls.some((toolCall) => toolCall.status === "PENDING_APPROVAL")) {
    return "PENDING_APPROVAL";
  }
  if (toolCalls.some((toolCall) => toolCall.status === "RUNNING" || toolCall.status === "PENDING")) {
    return "RUNNING";
  }
  if (toolCalls.some((toolCall) => toolCall.status === "FAILED" || toolCall.status === "TIMED_OUT")) {
    return "FAILED";
  }
  if (toolCalls.some((toolCall) => toolCall.status === "DENIED")) {
    return "DENIED";
  }
  if (toolCalls.some((toolCall) => toolCall.status === "CANCELLED")) {
    return "CANCELLED";
  }
  return "COMPLETED";
}
