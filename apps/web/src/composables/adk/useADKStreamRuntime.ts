import { ref, type Ref } from "vue";

import type { ADKChatResponse, ADKRun } from "@/types";
import {
  isBlockingRunStatus,
  isRootRun,
  resolveGoalAwareChatResponse,
} from "./adkChatRuntime";
import { isTerminalRunStatus } from "./adkChatPresentation";
import {
  resumeADKChatStream,
  type ADKChatStreamEvent,
} from "./adkChatStream";
import { normalizeADKRun, normalizeADKTimelineEntry } from "./adkNormalization";
import { scrollToBottom } from "./adkThreadScroll";
import { upsertTimelineEntry, type ADKTimelineEntryState } from "./adkTimeline";
import type { ADKSessionRuntimeState } from "./adkPagePersistence";

interface StreamRuntimeInput {
  activeRunSnapshot: Ref<ADKRun | null>;
  activeGoalRunSnapshot: Ref<ADKRun | null>;
  applyAuthoritativeTimeline: (response: ADKChatResponse) => Promise<void>;
  applySessionContext: (context: ADKChatStreamEvent["context"]) => void;
  clearSessionRuntimeState: (sessionId: string) => void;
  errorMessage: Ref<string>;
  finalizeStreamResponse: (response: ADKChatResponse) => Promise<void>;
  handleRunContinuation: (run: ADKRun) => Promise<void>;
  reloadSessionTimeline: (sessionId: string) => Promise<void>;
  scheduleSessionContextRefresh: (sessionId: string) => void;
  scrollTarget: Ref<HTMLElement | null>;
  selectedSessionId: Ref<string>;
  setSelectedSessionId: (sessionId: string) => void;
  syncActiveRun: (run: ADKRun | undefined, waiting?: boolean) => ADKRun | undefined;
  timelineEntries: Ref<ADKTimelineEntryState[]>;
  sessionRuntimeState: (sessionId: string) => ADKSessionRuntimeState;
  updateSessionRuntimeState: (
    sessionId: string,
    patch: Partial<ADKSessionRuntimeState>,
  ) => void;
}

export function useADKStreamRuntime(input: StreamRuntimeInput) {
  const streamReconnectController = ref<AbortController | null>(null);

  async function handleChatStreamEvent(event: ADKChatStreamEvent): Promise<void> {
    const eventSessionId =
      event.session?.id ||
      event.run?.sessionId ||
      event.response?.session.id ||
      input.selectedSessionId.value;
    const selectedSessionId = input.selectedSessionId.value.trim();
    if (
      selectedSessionId !== "" &&
      eventSessionId !== "" &&
      selectedSessionId !== eventSessionId
    ) {
      return;
    }
    if (eventSessionId) input.setSelectedSessionId(eventSessionId);
    const runId = event.run?.id || event.response?.run.id || event.runId || "";
    if (eventSessionId && (event.streamId || event.sequence || runId)) {
      const state = input.sessionRuntimeState(eventSessionId);
      input.updateSessionRuntimeState(eventSessionId, {
        streamId: event.streamId || state.streamId,
        runId: runId || state.runId,
        sequence: Math.max(state.sequence, event.sequence ?? 0),
      });
    }
    if (event.type === "context" && event.context) {
      input.applySessionContext(event.context);
    }
    if (event.type === "run" && event.run?.id) {
      input.syncActiveRun(normalizeADKRun(event.run), true);
      input.scheduleSessionContextRefresh(eventSessionId);
    }
    if (event.type === "timeline" && event.timeline) {
      input.timelineEntries.value = upsertTimelineEntry(
        input.timelineEntries.value,
        normalizeADKTimelineEntry(event.timeline),
      );
      await scrollToBottom(input.scrollTarget);
    }
    if (event.type === "final" && event.response) {
      const resolution = resolveGoalAwareChatResponse(
        event.response,
        input.syncActiveRun,
      );
      if (resolution.staleTerminalGoalPauseOverride) {
        input.clearSessionRuntimeState(resolution.normalizedResponse.session.id);
        await input.reloadSessionTimeline(resolution.normalizedResponse.session.id);
        return;
      }
      await input.applyAuthoritativeTimeline(resolution.resolvedResponse);
      if (resolution.normalizedResponse.context) {
        input.applySessionContext(resolution.normalizedResponse.context);
      }
      if (resolution.failMessage) input.errorMessage.value = resolution.failMessage;
      if (resolution.terminal) {
        input.clearSessionRuntimeState(resolution.normalizedResponse.session.id);
      }
    }
    if (event.type === "error") {
      if (eventSessionId) input.clearSessionRuntimeState(eventSessionId);
      throw new Error(event.message || "Agents chat failed");
    }
  }

  async function reconnectSessionStream(
    sessionId: string,
    runs: ADKRun[],
  ): Promise<void> {
    const state = input.sessionRuntimeState(sessionId);
    const activeRootRun = [...runs]
      .reverse()
      .find((run) => isRootRun(run) && isBlockingRunStatus(run.status));
    const runId = state.runId || activeRootRun?.id || "";
    if (state.streamId === "" && runId === "") return;
    streamReconnectController.value?.abort();
    const controller = new AbortController();
    streamReconnectController.value = controller;
    try {
      const response = await resumeADKChatStream(
        {
          streamId: state.streamId,
          runId,
          after: state.sequence,
          signal: controller.signal,
        },
        handleChatStreamEvent,
      );
      if (response == null) {
        const run = activeRootRun ?? runs.find((candidate) => candidate.id === runId);
        if (run && !isTerminalRunStatus(run.status)) {
          await input.handleRunContinuation(run);
        }
      } else {
        await input.finalizeStreamResponse(response);
      }
    } catch (error) {
      if (controller.signal.aborted) return;
      const run = activeRootRun ?? runs.find((candidate) => candidate.id === runId);
      if (run && !isTerminalRunStatus(run.status)) {
        await input.handleRunContinuation(run);
        return;
      }
      input.errorMessage.value =
        error instanceof Error ? error.message : "恢复推理流失败";
    } finally {
      if (streamReconnectController.value === controller) {
        streamReconnectController.value = null;
      }
    }
  }

  function abortReconnectStream(): void {
    streamReconnectController.value?.abort();
    streamReconnectController.value = null;
  }

  return {
    abortReconnectStream,
    handleChatStreamEvent,
    reconnectSessionStream,
    streamReconnectController,
  };
}
