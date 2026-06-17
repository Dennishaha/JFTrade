import type { ADKRun, ADKSession, ADKTimelineEntry } from "@/contracts";

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
}

export async function loadSessionChatHistory(sessionId: string): Promise<{
  session: ADKSession;
  timelineEntries: ADKTimelineEntryState[];
}> {
  const detail = await fetchEnvelope<SessionDetailResponse>(
    `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}`,
  );
  return {
    session: detail.session,
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
