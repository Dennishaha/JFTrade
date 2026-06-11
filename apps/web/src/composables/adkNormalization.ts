import type {
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKRun,
  ADKTimelineEntry,
} from "@/contracts";

function ensureArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function normalizeApprovals(
  approvals: ADKApproval[] | null | undefined,
): ADKApproval[] {
  return [...ensureArray(approvals)];
}

function normalizeTimeline(
  timeline: ADKTimelineEntry[] | null | undefined,
): ADKTimelineEntry[] {
  return ensureArray(timeline).map((entry) => normalizeADKTimelineEntry(entry));
}

export function normalizeADKRun(run: ADKRun): ADKRun {
  return {
    ...run,
    toolCalls: [...ensureArray(run.toolCalls as ADKRun["toolCalls"] | null)],
    pendingApprovals: normalizeApprovals(
      run.pendingApprovals as ADKApproval[] | null,
    ),
  };
}

export function normalizeADKTimelineEntry(
  entry: ADKTimelineEntry,
): ADKTimelineEntry {
  const normalized: ADKTimelineEntry = { ...entry };
  if (entry.toolCalls !== undefined) {
    normalized.toolCalls = [
      ...ensureArray(entry.toolCalls as ADKTimelineEntry["toolCalls"] | null),
    ];
  }
  if (entry.approvals !== undefined) {
    normalized.approvals = normalizeApprovals(
      entry.approvals as ADKApproval[] | null,
    );
  }
  return normalized;
}

export function normalizeADKChatResponse(
  response: ADKChatResponse,
): ADKChatResponse {
  return {
    ...response,
    run: normalizeADKRun(response.run),
    pendingApprovals: normalizeApprovals(
      response.pendingApprovals as ADKApproval[] | null,
    ),
    timeline: normalizeTimeline(response.timeline as ADKTimelineEntry[] | null),
  };
}

export function normalizeADKApprovalResolution(
  resolution: ADKApprovalResolution,
): ADKApprovalResolution {
  const normalized: ADKApprovalResolution = { ...resolution };
  if (resolution.run) {
    normalized.run = normalizeADKRun(resolution.run);
  }
  return normalized;
}

export function normalizeADKRunList(runs: ADKRun[] | null | undefined): ADKRun[] {
  return ensureArray(runs).map((run) => normalizeADKRun(run));
}

export function normalizeADKTimelineEntries(
  entries: ADKTimelineEntry[] | null | undefined,
): ADKTimelineEntry[] {
  return normalizeTimeline(entries);
}
