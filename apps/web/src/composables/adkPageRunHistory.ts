import type { ADKRun, ADKSession, ADKTranscriptEntry } from "@jftrade/ui-contracts";

import { fetchEnvelope } from "./apiClient";
import { applyPersistedRunState, toChatMessage, type ChatMessage } from "./adkPageMessages";

interface SessionDetailResponse {
  session: ADKSession;
  transcriptEntries: ADKTranscriptEntry[];
  messages?: ADKTranscriptEntry[];
}

interface PageEnvelope {
  limit: number;
  offset: number;
  total: number;
  returned: number;
  hasMore: boolean;
}

interface RunsResponse {
  runs: ADKRun[];
  page?: PageEnvelope;
}

export async function loadSessionChatHistory(sessionId: string): Promise<{
  session: ADKSession;
  chatMessages: ChatMessage[];
}> {
  const detail = await fetchEnvelope<SessionDetailResponse>(`/api/v1/adk/sessions/${encodeURIComponent(sessionId)}`);
  const runs = await fetchAllRunsForSession(sessionId);
  const runsByMessageId = new Map(
    runs
      .filter((run) => run.finalMessageId)
      .map((run) => [run.finalMessageId!, run]),
  );

  return {
    session: detail.session,
    chatMessages: (detail.transcriptEntries ?? detail.messages ?? [])
      .filter((entry) => !entry.kind || entry.kind === "message")
      .map((entry) => {
        const chatMessage = toChatMessage(entry);
        applyPersistedRunState(chatMessage, runsByMessageId.get(entry.id));
        return chatMessage;
      }),
  };
}

async function fetchAllRunsForSession(sessionId: string): Promise<ADKRun[]> {
  const runs: ADKRun[] = [];
  let offset = 0;
  const limit = 100;
  try {
    while (true) {
      const response = await fetchEnvelope<RunsResponse>(
        `/api/v1/adk/runs?sessionId=${encodeURIComponent(sessionId)}&limit=${limit}&offset=${offset}`,
      );
      runs.push(...(response.runs ?? []));
      const returned = response.page?.returned ?? response.runs?.length ?? 0;
      if (!response.page?.hasMore || returned === 0) {
        break;
      }
      offset += returned;
    }
  } catch {
    return [];
  }
  return runs;
}
