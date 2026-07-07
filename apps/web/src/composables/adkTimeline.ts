import type {
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKRun,
  ADKTimelineEntry,
} from "@/contracts";

import { uniqueADKApprovalsById } from "./adkApprovalResolution";
import { normalizeADKChatResponse } from "./adkNormalization";
import type { ADKChildRunQueueItem } from "./useADKWorkflowQueueState";

export interface ADKTimelineEntryState extends ADKTimelineEntry {
  run?: ADKRun;
  childRunItem?: ADKChildRunQueueItem;
  reasoningExpanded?: boolean;
  userPromptVariant?: "original" | "processed";
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
  if (entry.kind === "user_message") {
    state.userPromptVariant = "original";
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
  runsById: Map<string, ADKRun> = new Map(),
): ADKTimelineEntryState[] {
  const previousById = new Map(previous.map((entry) => [entry.id, entry]));
  return dedupeTimelineApprovalEntries(
    sortTimelineEntries(
      (entries ?? []).map((entry) =>
        mergeTimelineEntry(previousById.get(entry.id), entry, runsById),
      ),
    ),
  );
}

export function replaceAuthoritativeChatResponseTimeline(
  response: ADKChatResponse,
  previous: ADKTimelineEntryState[] = [],
): ADKTimelineEntryState[] {
  const normalizedResponse = normalizeADKChatResponse(response);
  return replaceTimelineEntries(
    normalizedResponse.timeline,
    previous,
    new Map([[normalizedResponse.run.id, normalizedResponse.run]]),
  );
}

export function upsertTimelineEntry(
  entries: ADKTimelineEntryState[],
  incoming: ADKTimelineEntry,
): ADKTimelineEntryState[] {
  const index = entries.findIndex((entry) => entry.id === incoming.id);
  if (index < 0) {
    return dedupeTimelineApprovalEntries(
      sortTimelineEntries([...entries, createTimelineEntryState(incoming)]),
    );
  }
  const next = [...entries];
  next[index] = mergeTimelineEntry(next[index], incoming);
  return dedupeTimelineApprovalEntries(sortTimelineEntries(next));
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
  if (entry.run) {
    return {
      ...entry.run,
      toolCalls: toolCalls.length > 0 ? toolCalls : [...(entry.run.toolCalls ?? [])],
      pendingApprovals: [...(entry.run.pendingApprovals ?? [])],
    };
  }
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
  return uniqueADKApprovalsById(
    [...(entry.approvals ?? [])].filter(isPendingApproval),
  );
}

export function applyApprovalResolutions(
  entries: ADKTimelineEntryState[],
  resolutions: ADKApprovalResolution[],
): ADKTimelineEntryState[] {
  if (resolutions.length === 0) {
    return entries;
  }
  const resolvedApprovalIds = new Set<string>();
  const pendingApprovalIdsByRun = new Map<string, Set<string>>();
  for (const resolution of resolutions) {
    if (resolution.approval?.id) {
      resolvedApprovalIds.add(resolution.approval.id);
    }
    if (resolution.run?.id) {
      pendingApprovalIdsByRun.set(
        resolution.run.id,
        new Set(
          (resolution.run.pendingApprovals ?? [])
            .filter(isPendingApproval)
            .map((approval) => approval.id),
        ),
      );
    }
    if (resolution.parentRun?.id) {
      pendingApprovalIdsByRun.set(
        resolution.parentRun.id,
        new Set(
          (resolution.parentRun.pendingApprovals ?? [])
            .filter(isPendingApproval)
            .map((approval) => approval.id),
        ),
      );
    }
  }
  return dedupeTimelineApprovalEntries(
    sortTimelineEntries(
      entries.flatMap((entry) => {
        if (entry.kind !== "approval_group" || !entry.approvals?.length) {
          return [entry];
        }
        const pendingApprovalIds = entry.runId
          ? pendingApprovalIdsByRun.get(entry.runId)
          : undefined;
        const approvals = uniqueADKApprovalsById(entry.approvals).filter(
          (approval) => {
            if (pendingApprovalIds) {
              return pendingApprovalIds.has(approval.id);
            }
            return (
              !resolvedApprovalIds.has(approval.id) &&
              isPendingApproval(approval)
            );
          },
        );
        if (approvals.length === 0) {
          return [];
        }
        return [{ ...entry, approvals }];
      }),
    ),
  );
}

function mergeTimelineEntry(
  existing: ADKTimelineEntryState | undefined,
  incoming: ADKTimelineEntry,
  runsById: Map<string, ADKRun> = new Map(),
): ADKTimelineEntryState {
  const base = existing ?? createTimelineEntryState(incoming);
  const next: ADKTimelineEntryState = { ...base, ...incoming };
  const runId = String(incoming.runId ?? base.runId ?? "").trim();
  const run = runId === "" ? undefined : runsById.get(runId);
  if (run) {
    next.run = run;
  } else if (base.run) {
    next.run = base.run;
  }
  if (incoming.toolCalls !== undefined) {
    next.toolCalls = [...incoming.toolCalls];
  } else if (base.toolCalls) {
    next.toolCalls = [...base.toolCalls];
  }
  if (incoming.approvals !== undefined) {
    next.approvals = [...incoming.approvals];
  } else if (base.approvals) {
    next.approvals = [...base.approvals];
  }
  return next;
}

function isPendingApproval(approval: Pick<ADKApproval, "status">): boolean {
  const status = String(approval.status ?? "").trim().toUpperCase();
  return status === "" || status === "PENDING" || status === "PENDING_APPROVAL";
}

function dedupeTimelineApprovalEntries(
  entries: ADKTimelineEntryState[],
): ADKTimelineEntryState[] {
  const ownerByApprovalId = new Map<string, ApprovalOwner>();
  entries.forEach((entry, index) => {
    if (entry.kind !== "approval_group" || !entry.approvals?.length) {
      return;
    }
    for (const approval of approvalsForGroup(entry)) {
      const id = approval.id.trim();
      if (id === "") {
        continue;
      }
      const candidate: ApprovalOwner = {
        entryKey: timelineEntryKey(entry, index),
        parentCopy: isParentWorkflowApprovalCopy(entry, approval),
      };
      const current = ownerByApprovalId.get(id);
      if (!current || shouldPreferApprovalOwner(candidate, current)) {
        ownerByApprovalId.set(id, candidate);
      }
    }
  });

  return entries.flatMap((entry, index) => {
    if (entry.kind !== "approval_group" || !entry.approvals?.length) {
      return [entry];
    }
    const entryKey = timelineEntryKey(entry, index);
    const approvals = approvalsForGroup(entry).filter((approval) => {
      const id = approval.id.trim();
      return id === "" || ownerByApprovalId.get(id)?.entryKey === entryKey;
    });
    if (approvals.length === 0) {
      return [];
    }
    return [{ ...entry, approvals }];
  });
}

interface ApprovalOwner {
  entryKey: string;
  parentCopy: boolean;
}

function shouldPreferApprovalOwner(
  candidate: ApprovalOwner,
  current: ApprovalOwner,
): boolean {
  return candidate.parentCopy && !current.parentCopy;
}

function isParentWorkflowApprovalCopy(
  entry: ADKTimelineEntryState,
  approval: ADKApproval,
): boolean {
  const entryRunId = String(entry.runId ?? "").trim();
  const approvalRunId = String(approval.runId ?? "").trim();
  return (
    entryRunId !== "" &&
    approvalRunId !== "" &&
    entryRunId !== approvalRunId
  );
}

function timelineEntryKey(entry: ADKTimelineEntryState, index: number): string {
  return `${entry.id}::${entry.runId ?? ""}::${index}`;
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
