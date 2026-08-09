import type { Ref } from "vue";

import type {
  ADKChatResponse,
  ADKProvider,
  ADKRun,
  ADKSessionContextSnapshot,
} from "@/types";

import {
  streamADKChat,
  type ADKChatStreamEvent,
} from "@/composables/adk/adkChatStream";
import {
  isGoalPauseAbortError,
  resolveGoalAwareChatResponse,
} from "@/composables/adk/adkChatRuntime";
import { scrollToBottom } from "@/composables/adk/adkThreadScroll";
import {
  createTimelineEntryState,
  replaceAuthoritativeChatResponseTimeline,
  upsertTimelineEntry,
  type ADKTimelineEntryState,
} from "@/composables/adk/adkTimeline";

interface ADKPageChatExecutionInput {
  applySessionContext: (
    context: ADKSessionContextSnapshot | null | undefined,
  ) => void;
  clearSessionRuntimeState: (sessionId: string) => void;
  dispatchQueuedMessagesIfIdle: () => Promise<void>;
  effectiveWorkMode: Ref<string>;
  errorMessage: Ref<string>;
  flushComposerState: () => Promise<void>;
  goalObjectiveDraft: Ref<string>;
  goalPauseRequested: Ref<boolean>;
  permissionModeOverride: Ref<string>;
  refreshAll: () => Promise<void>;
  refreshSessionContext: (sessionId?: string) => Promise<void>;
  reloadSessionTimeline: (sessionId: string) => Promise<void>;
  scrollTarget: Ref<HTMLElement | null>;
  selectedAgentId: Ref<string>;
  selectedProvider: Ref<ADKProvider | null>;
  selectedProviderId: Ref<string>;
  selectedSessionId: Ref<string>;
  sendingChat: Ref<boolean>;
  setSelectedSessionId: (sessionId: string) => void;
  syncActiveRun: (
    run: ADKRun | undefined,
    waitingForContinuation?: boolean,
  ) => ADKRun | undefined;
  timelineEntries: Ref<ADKTimelineEntryState[]>;
  clearWorkflowPlanRun: () => void;
  onStreamEvent: (event: ADKChatStreamEvent) => void | Promise<void>;
}

export function useADKPageChatExecution(input: ADKPageChatExecutionInput) {
  let chatStreamController: AbortController | null = null;
  let chatStreamAbortReason = "";

  async function executeChatMessage(
    text: string,
    options: { forceChat?: boolean } = {},
    clientRequestId: string = crypto.randomUUID(),
  ): Promise<boolean> {
    const payload: Parameters<typeof streamADKChat>[0] = {
      clientRequestId,
      agentId: input.selectedAgentId.value,
      sessionId: input.selectedSessionId.value,
      message: text,
    };
    const providerId = input.selectedProviderId.value.trim();
    if (providerId !== "") payload.providerId = providerId;
    const model = input.selectedProvider.value?.model?.trim() ?? "";
    if (model !== "") payload.model = model;
    if (input.permissionModeOverride.value) {
      payload.permissionModeOverride = input.permissionModeOverride.value;
    }
    const mode = input.effectiveWorkMode.value;
    if (options.forceChat) {
      payload.workModeOverride = "chat";
    } else if (mode) {
      payload.workModeOverride = mode;
      if (mode === "loop") {
        payload.objective = input.goalObjectiveDraft.value.trim() || text;
      }
    }
    const optimisticUserEntry = createTimelineEntryState({
      id: `local-user-${clientRequestId}`,
      sessionId: input.selectedSessionId.value,
      kind: "user_message",
      createdAt: new Date().toISOString(),
      sequence: input.timelineEntries.value.length + 1,
      status: "streaming",
      text,
    });
    if (!options.forceChat) input.clearWorkflowPlanRun();
    input.sendingChat.value = true;
    input.timelineEntries.value = upsertTimelineEntry(
      input.timelineEntries.value,
      optimisticUserEntry,
    );
    await scrollToBottom(input.scrollTarget);
    const controller = new AbortController();
    chatStreamController = controller;
    chatStreamAbortReason = "";
    let streamAbortedForGoalPause = false;

    try {
      const response = await streamADKChat(payload, input.onStreamEvent, {
        signal: controller.signal,
      });
      await finalizeStreamResponse(response);
      await input.flushComposerState();
      return true;
    } catch (error) {
      if (isGoalPauseAbort(controller, error)) {
        streamAbortedForGoalPause = true;
        await input.flushComposerState();
        return true;
      }
      input.errorMessage.value =
        error instanceof Error ? error.message : "Agents chat failed";
      await scrollToBottom(input.scrollTarget);
      return false;
    } finally {
      if (chatStreamController === controller) {
        chatStreamController = null;
        chatStreamAbortReason = "";
      }
      input.sendingChat.value = false;
      if (!streamAbortedForGoalPause || !input.goalPauseRequested.value) {
        await input.dispatchQueuedMessagesIfIdle();
      }
    }
  }

  async function finalizeStreamResponse(
    response: ADKChatResponse,
  ): Promise<void> {
    const resolution = resolveGoalAwareChatResponse(
      response,
      input.syncActiveRun,
    );
    input.setSelectedSessionId(resolution.normalizedResponse.session.id);
    if (resolution.staleTerminalGoalPauseOverride) {
      input.clearSessionRuntimeState(resolution.normalizedResponse.session.id);
      await input.reloadSessionTimeline(resolution.normalizedResponse.session.id);
      return;
    }
    await applyAuthoritativeTimeline(resolution.resolvedResponse);
    if (resolution.normalizedResponse.context) {
      input.applySessionContext(resolution.normalizedResponse.context);
    } else {
      await input.refreshSessionContext(
        resolution.normalizedResponse.session.id,
      );
    }
    await input.refreshAll();
    if (resolution.failMessage) {
      input.errorMessage.value = resolution.failMessage;
    }
    if (resolution.terminal) {
      input.clearSessionRuntimeState(resolution.normalizedResponse.session.id);
    }
    await scrollToBottom(input.scrollTarget);
  }

  async function applyAuthoritativeTimeline(
    response: ADKChatResponse,
  ): Promise<void> {
    input.timelineEntries.value = replaceAuthoritativeChatResponseTimeline(
      response,
      input.timelineEntries.value,
    );
    await scrollToBottom(input.scrollTarget);
  }

  function abortActiveChatStream(reason = ""): void {
    if (!chatStreamController) return;
    chatStreamAbortReason = reason;
    chatStreamController.abort();
  }

  function isGoalPauseAbort(
    controller: AbortController,
    error: unknown,
  ): boolean {
    return isGoalPauseAbortError(controller, error, chatStreamAbortReason);
  }

  return {
    abortActiveChatStream,
    applyAuthoritativeTimeline,
    executeChatMessage,
    finalizeStreamResponse,
  };
}
