import type {
  ADKRun,
  ADKSession,
  ADKSessionComposerState,
} from "@/types";

import { apiGetPath } from "@/composables/shared/apiClient";
import {
  requireADKComposerState,
  requireADKRuns,
  requireADKSession,
  requireADKTimeline,
} from "@/composables/adk/adkApiMappers";
import { normalizeADKTimelineEntries } from "@/composables/adk/adkNormalization";
import {
  replaceTimelineEntries,
  type ADKTimelineEntryState,
} from "@/composables/adk/adkTimeline";

export async function loadSessionChatHistory(sessionId: string): Promise<{
  session: ADKSession;
  timelineEntries: ADKTimelineEntryState[];
  runs: ADKRun[];
  composerState: ADKSessionComposerState;
}> {
  const detail = await apiGetPath(
    "/api/v1/adk/sessions/{sessionId}",
    `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}`,
  );
  const runs = detail.runs === undefined ? [] : requireADKRuns(detail.runs);
  return {
    session: requireADKSession(detail.session),
    composerState: normalizeSessionComposerState(
      sessionId,
      detail.composerState === undefined
        ? undefined
        : requireADKComposerState(detail.composerState),
    ),
    runs,
    timelineEntries: replaceTimelineEntries(
      normalizeADKTimelineEntries(requireADKTimeline(detail.timeline)),
      [],
      runsById(runs),
    ),
  };
}

function runsById(runs: ADKRun[]): Map<string, ADKRun> {
  return new Map(runs.map((run) => [run.id, run]));
}

export function normalizeSessionComposerState(
  sessionId: string,
  state: ADKSessionComposerState | null | undefined,
): ADKSessionComposerState {
  const mode = String(state?.workModeOverride ?? "").trim();
  const permissionMode = String(state?.permissionModeOverride ?? "").trim();
  const reasoningEffort = String(state?.reasoningEffortOverride ?? "").trim();
  return {
    sessionId: String(state?.sessionId || sessionId).trim(),
    chatDraft: state?.chatDraft ?? "",
    providerIdOverride: String(state?.providerIdOverride ?? "").trim(),
    modelOverride: String(state?.modelOverride ?? "").trim(),
    reasoningEffortOverride:
      reasoningEffort === "low" ||
      reasoningEffort === "medium" ||
      reasoningEffort === "high" ||
      reasoningEffort === "xhigh" ||
      reasoningEffort === "max"
        ? reasoningEffort
        : "",
    workModeOverride:
      mode === "chat" || mode === "loop" ? mode : "",
    permissionModeOverride:
      permissionMode === "approval" ||
      permissionMode === "less_approval" ||
      permissionMode === "all"
        ? permissionMode
        : "",
    goalObjectiveDraft: state?.goalObjectiveDraft ?? "",
    goalObjectiveTouched: state?.goalObjectiveTouched === true,
    updatedAt: state?.updatedAt ?? "",
  };
}
