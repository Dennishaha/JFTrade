import { buildApiUrl, csrfHeaders } from "./apiClient";
import { isTerminalRunStatus } from "./adkChatPresentation";
import {
  normalizeADKChatResponse,
  normalizeADKRun,
  normalizeADKTimelineEntry,
} from "./adkNormalization";

import type {
  ADKApproval,
  ADKRun,
  ADKSession,
  ADKSessionContextSnapshot,
  ADKTimelineEntry,
} from "@/contracts";

export interface ADKChatStreamResponse {
  reply: string;
  reasoningContent?: string;
  session: ADKSession;
  run: ADKRun;
  pendingApprovals: ADKApproval[];
  timeline: ADKTimelineEntry[];
  context?: ADKSessionContextSnapshot;
}

export interface ADKChatStreamEvent {
  type: "session" | "run" | "timeline" | "context" | "final" | "error";
  timeline?: ADKTimelineEntry;
  response?: ADKChatStreamResponse;
  session?: ADKSession;
  run?: ADKRun;
  context?: ADKSessionContextSnapshot;
  message?: string;
}

export async function streamADKChat(
  payload: {
    agentId?: string;
    sessionId?: string;
    message: string;
    workModeOverride?: string;
    objective?: string;
    runOptions?: {
      loopMaxIterations?: number;
    };
  },
  onEvent: (event: ADKChatStreamEvent) => void | Promise<void>,
): Promise<ADKChatStreamResponse> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...csrfHeaders("POST"),
  };
  const response = await fetch(buildApiUrl("/api/v1/adk/chat/stream"), {
    method: "POST",
    credentials: "include",
    headers,
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error((await response.text()) || "Agents chat failed");
  }
  if (!response.body) {
    throw new Error("流式响应不可用");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let finalResponse: ADKChatStreamResponse | null = null;
  let sawAnyFrame = false;
  let lastSession: ADKSession | null = null;
  let lastRun: ADKRun | null = null;

  const defaultIdleTimeoutMs = 300_000;
  const headerIdleTimeoutMs = Number(
    response.headers?.get?.("X-ADK-Stream-Idle-Timeout-Ms") ?? "",
  );
  const SSE_IDLE_TIMEOUT_MS =
    Number.isFinite(headerIdleTimeoutMs) && headerIdleTimeoutMs > 0
      ? headerIdleTimeoutMs
      : defaultIdleTimeoutMs;
  let idleTimer: ReturnType<typeof setTimeout> | null = null;

  const resetIdleTimer = () => {
    if (idleTimer) clearTimeout(idleTimer);
    idleTimer = setTimeout(() => {
      console.warn(
        "[ADK SSE] Idle timeout - no data for",
        SSE_IDLE_TIMEOUT_MS,
        "ms, aborting stream",
      );
      reader.cancel();
    }, SSE_IDLE_TIMEOUT_MS);
  };
  resetIdleTimer();

  try {
    while (true) {
      const { value, done } = await reader.read();
      resetIdleTimer();
      buffer += decoder.decode(value, { stream: !done });
      const frames = buffer.split("\n\n");
      buffer = frames.pop() ?? "";
      for (const frame of frames) {
        const event = parseSSEFrame(frame);
        if (!event) continue;
        sawAnyFrame = true;
        if (event.session) lastSession = event.session;
        if (event.run) lastRun = event.run;
        if (event.response) {
          lastSession = event.response.session;
          lastRun = event.response.run;
        }
        await onEvent(event);
        if (event.type === "final" && event.response) {
          finalResponse = event.response;
        }
        if (event.type === "error") {
          throw new Error(event.message || "Agents chat failed");
        }
      }
      if (done) break;
    }

    if (buffer.trim() !== "") {
      const event = parseSSEFrame(buffer);
      if (event) {
        sawAnyFrame = true;
        if (event.session) lastSession = event.session;
        if (event.run) lastRun = event.run;
        if (event.response) {
          lastSession = event.response.session;
          lastRun = event.response.run;
        }
        await onEvent(event);
        if (event.type === "final" && event.response) {
          finalResponse = event.response;
        }
        if (event.type === "error") {
          throw new Error(event.message || "Agents chat failed");
        }
      }
    }
  } finally {
    if (idleTimer) clearTimeout(idleTimer);
  }

  if (!finalResponse) {
    if (lastRun && isTerminalRunStatus(lastRun.status)) {
      // The run reached a terminal state (FAILED / TIMED_OUT / CANCELLED /
      // DENIED) before the final SSE frame could be delivered.  Treat this
      // as a valid terminal result rather than a transport error so callers
      // can display failure details from the run / toolCalls and keep the
      // conversation usable.
      const response: ADKChatStreamResponse = {
        reply: lastSession?.title ?? "",
        session: lastSession ?? {
          id: lastRun.sessionId ?? "",
          agentId: lastRun.agentId ?? "",
          title: "",
          createdAt: lastRun.createdAt ?? new Date().toISOString(),
          updatedAt: lastRun.updatedAt ?? new Date().toISOString(),
        },
        run: lastRun,
        pendingApprovals: lastRun.toolCalls
          ?.filter((tc) => tc.status === "PENDING_APPROVAL")
          .map((tc) => ({
            id: tc.id,
            runId: lastRun.id,
            agentId: lastRun.agentId,
            toolName: tc.toolName,
            ...(tc.input !== undefined ? { input: tc.input } : {}),
            status: "PENDING" as const,
            reason: tc.permission ?? "",
            createdAt: tc.createdAt ?? lastRun.createdAt ?? new Date().toISOString(),
            updatedAt: tc.updatedAt ?? lastRun.updatedAt ?? new Date().toISOString(),
          })) ?? [],
        timeline: [],
      };
      return response;
    }
    if (sawAnyFrame) {
      throw new Error("流式连接中断，未收到最终结果。");
    }
    throw new Error("流式连接未返回有效数据。");
  }
  return finalResponse;
}

function parseSSEFrame(frame: string): ADKChatStreamEvent | null {
  const data = frame
    .split("\n")
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trim())
    .join("\n");
  if (data === "") return null;
  try {
    const event = JSON.parse(data) as ADKChatStreamEvent;
    if (event.run) {
      event.run = normalizeADKRun(event.run);
    }
    if (event.timeline) {
      event.timeline = normalizeADKTimelineEntry(event.timeline);
    }
    if (event.response) {
      event.response = normalizeADKChatResponse(event.response);
    }
    return event;
  } catch {
    console.warn(
      "[ADK SSE] Failed to parse frame, skipping:",
      data.slice(0, 200),
    );
    return null;
  }
}
