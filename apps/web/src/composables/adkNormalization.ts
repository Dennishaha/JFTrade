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
  const normalized: ADKRun = {
    ...run,
    toolCalls: [...ensureArray(run.toolCalls as ADKRun["toolCalls"] | null)],
    pendingApprovals: normalizeApprovals(
      run.pendingApprovals as ADKApproval[] | null,
    ),
  };
  if (run.inputRequest) {
    normalized.inputRequest = {
      ...run.inputRequest,
      questions: (run.inputRequest.questions ?? []).map((question) => ({
        ...question,
        options: [...(question.options ?? [])],
      })),
      answers: [...(run.inputRequest.answers ?? [])],
    };
  }
  if (run.inputRequests !== undefined) {
    normalized.inputRequests = (run.inputRequests ?? []).map((request) => ({
      ...request,
      questions: (request.questions ?? []).map((question) => ({
        ...question,
        options: [...(question.options ?? [])],
      })),
      answers: [...(request.answers ?? [])],
    }));
  }
  if (run.childRunIds !== undefined) {
    normalized.childRunIds = [...ensureArray(run.childRunIds as string[] | null)];
  }
  if (run.workflowPlan !== undefined) {
    normalized.workflowPlan = [
      ...ensureArray(run.workflowPlan as ADKRun["workflowPlan"] | null),
    ].map((step) => ({
      ...step,
      dependsOn: [...ensureArray(step.dependsOn as string[] | null)],
      routes: [...ensureArray(step.routes as string[] | null)],
    }));
  }
  return normalized;
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
  if (entry.inputRequest) {
    normalized.inputRequest = {
      ...entry.inputRequest,
      questions: (entry.inputRequest.questions ?? []).map((question) => ({
        ...question,
        options: [...(question.options ?? [])],
      })),
      answers: [...(entry.inputRequest.answers ?? [])],
    };
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
  if (resolution.parentRun) {
    normalized.parentRun = normalizeADKRun(resolution.parentRun);
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
