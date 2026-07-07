import type {
  ADKRun,
  ADKSession,
  ADKSessionComposerState,
  ADKTimelineEntry,
} from "@/contracts";

import { fetchEnvelope } from "./apiClient";
import { normalizeADKTimelineEntries } from "./adkNormalization";
import {
  replaceTimelineEntries,
  type ADKTimelineEntryState,
} from "./adkTimeline";

interface SessionDetailResponse {
  session: ADKSession;
  timeline: ADKTimelineEntry[];
  runs?: ADKRun[];
  composerState?: ADKSessionComposerState;
}

export async function loadSessionChatHistory(sessionId: string): Promise<{
  session: ADKSession;
  timelineEntries: ADKTimelineEntryState[];
  runs: ADKRun[];
  composerState: ADKSessionComposerState;
}> {
  const detail = await fetchEnvelope<SessionDetailResponse>(
    `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}`,
  );
  return {
    session: detail.session,
    composerState: normalizeSessionComposerState(
      sessionId,
      detail.composerState,
    ),
    runs: detail.runs ?? [],
    timelineEntries: replaceTimelineEntries(
      normalizeADKTimelineEntries(detail.timeline),
      [],
      runsById(detail.runs ?? []),
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
  return {
    sessionId: String(state?.sessionId || sessionId).trim(),
    chatDraft: state?.chatDraft ?? "",
    providerIdOverride: String(state?.providerIdOverride ?? "").trim(),
    modelOverride: String(state?.modelOverride ?? "").trim(),
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
