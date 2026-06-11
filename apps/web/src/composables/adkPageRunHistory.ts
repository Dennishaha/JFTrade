import type { ADKSession, ADKTimelineEntry } from "@/contracts";

import { fetchEnvelope } from "./apiClient";
import { normalizeADKTimelineEntries } from "./adkNormalization";
import {
  replaceTimelineEntries,
  type ADKTimelineEntryState,
} from "./adkTimeline";

interface SessionDetailResponse {
  session: ADKSession;
  timeline: ADKTimelineEntry[];
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
    ),
  };
}
