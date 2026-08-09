import { onBeforeUnmount, ref, watch, type Ref } from "vue";

import type {
  ADKAgent,
  ADKChatResponse,
  ADKProvider,
  ADKRun,
  ADKSession,
} from "@/types";

import { isTerminalRunStatus } from "@/composables/adk/adkChatPresentation";
import { streamADKChat } from "@/composables/adk/adkChatStream";
import { normalizeADKRun } from "@/composables/adk/adkNormalization";
import {
  isActiveGoalParentRun,
  isGoalPauseAbortError,
  isQueueDispatchBlockedByGoalLifecycle,
  isRootRun,
  resolveGoalAwareChatResponse,
  isBlockingRunStatus,
  syncGoalAwareActiveRun,
  type ActiveChatRunState,
  type GoalObjectiveState,
  type QueuedChatMessage,
} from "@/composables/adk/adkChatRuntime";
import { scrollToBottom } from "@/composables/adk/adkThreadScroll";
import { loadSessionChatHistory } from "@/composables/adk/adkPageRunHistory";
import {
  createTimelineEntryState,
  replaceAuthoritativeChatResponseTimeline,
  upsertTimelineEntry,
  type ADKTimelineEntryState,
} from "@/composables/adk/adkTimeline";
import { useADKWorkflowQueueState } from "@/composables/adk/useADKWorkflowQueueState";
import { useADKComposerPersistence } from "@/composables/adk/useADKComposerPersistence";
import { useADKSessionContextState } from "@/composables/adk/useADKSessionContextState";
import { useADKApprovalRuntime } from "@/composables/adk/useADKApprovalRuntime";
import { useADKRunActions } from "@/composables/adk/useADKRunActions";
import { useADKPageRuntimePersistence } from "@/composables/adk/useADKPageRuntimePersistence";
import { useADKStreamRuntime } from "@/composables/adk/useADKStreamRuntime";
import {
  enqueueChatMessage as enqueueADKChatMessage,
  handleComposerKeydown as handleADKComposerKeydown,
  revokeQueuedMessage as revokeADKQueuedMessage,
  setSelectedSessionId as setADKSelectedSessionId,
} from "@/composables/adk/adkChatQueue";
import { useADKRunProjection } from "@/composables/adk/useADKRunProjection";

interface SessionState {
  agents: Ref<ADKAgent[]>;
  errorMessage: Ref<string>;
  initialized: Ref<boolean>;
  refreshAll: () => Promise<void>;
  finishSessionSelection: (agentId: string | undefined) => Promise<void>;
  selectedProvider: Ref<ADKProvider | null>;
  selectedAgentId: Ref<string>;
  selectedProviderId: Ref<string>;
  selectedSessionId: Ref<string>;
  sessions: Ref<ADKSession[]>;
}

export type { SlashCommandItem } from "@/composables/adk/useADKRunProjection";

export function useADKPageChatState(
  threadRef: Ref<HTMLElement | null>,
  sessionState: SessionState,
  composerBlockMessage: Ref<string>,
) {
  const timelineEntries = ref<ADKTimelineEntryState[]>([]);
  const chatDraft = ref("");
  const workModeOverride = ref("");
  const permissionModeOverride = ref("");
  const sendingChat = ref(false);
  const activeRun = ref<ActiveChatRunState | null>(null);
  const activeRunSnapshot = ref<ADKRun | null>(null);
  const activeGoalRunSnapshot = ref<ADKRun | null>(null);
  const queuedChatMessages = ref<QueuedChatMessage[]>([]);
  const queueDispatchingId = ref("");
  const interruptingRunId = ref("");
  const resolvingApprovalIds = ref<Set<string>>(new Set());
  const goalObjectiveDraft = ref("");
  const goalObjectiveTouched = ref(false);
  const goalObjectiveSaving = ref(false);
  const goalObjectiveError = ref("");
  const goalLifecycleBusy = ref(false);
  let pageStateRestored = false;
  let chatStreamController: AbortController | null = null;
  let chatStreamAbortReason = "";
  let sendAdmissionLocked = false;
  let retryDraftRequest: {
    text: string;
    clientRequestId: string;
    draftRevision: number;
  } | null = null;
  const workflowQueues = useADKWorkflowQueueState({
    timelineEntries,
    selectedSessionId: sessionState.selectedSessionId,
    resolvingApprovalIds,
  });
  const {
    clearSessionRuntimeState,
    pageState,
    removeSessionRuntimeState,
    sessionRuntimeState,
    updateSessionRuntimeState,
  } = useADKPageRuntimePersistence({
    activeChildRunId: workflowQueues.activeChildRunId,
    selectedSessionId: sessionState.selectedSessionId,
  });
  const {
    applySessionContext,
    clearSessionContext,
    contextBusy,
    contextDetailsOpen,
    initializeSessionContext,
    openContextDetails,
    refreshSessionContext,
    runSlashCommand,
    scheduleSessionContextRefresh,
    sessionContext,
    visibleSessionContext,
  } = useADKSessionContextState({
    errorMessage: sessionState.errorMessage,
    reloadTimeline: reloadSessionTimeline,
    selectedSessionId: sessionState.selectedSessionId,
  });
  const streamRuntime = useADKStreamRuntime({
    activeRunSnapshot,
    activeGoalRunSnapshot,
    applyAuthoritativeTimeline,
    applySessionContext,
    clearSessionRuntimeState,
    errorMessage: sessionState.errorMessage,
    finalizeStreamResponse,
    handleRunContinuation: (run) => waitForRunContinuation(run),
    reloadSessionTimeline,
    scheduleSessionContextRefresh,
    scrollTarget: threadRef,
    selectedSessionId: sessionState.selectedSessionId,
    setSelectedSessionId: (sessionId) =>
      setADKSelectedSessionId(
        sessionState.selectedSessionId,
        queuedChatMessages,
        sessionId,
      ),
    syncActiveRun,
    timelineEntries,
    sessionRuntimeState,
    updateSessionRuntimeState,
  });
  const {
    approvalsBusy,
    denyAllApprovals,
    denyApproval,
    handleTerminalRun,
    inputRequestBusy,
    resolveAllApprovals,
    resolveApproval,
    submitInputResponse,
    waitForRunContinuation,
  } = useADKApprovalRuntime({
    dispatchQueuedMessagesIfIdle,
    errorMessage: sessionState.errorMessage,
    interruptingRunId,
    refreshAll: sessionState.refreshAll,
    refreshSessionContext,
    reloadSessionTimeline,
    resolvingApprovalIds,
    selectedSessionId: sessionState.selectedSessionId,
    syncActiveRun,
    timelineEntries,
    workflowQueues,
  });

  const {
    activeGoalRun,
    activeGoalRunId,
    activeRunControlId,
    activeRunId,
    activeRunStatus,
    activityIndicator,
    canInterruptChat,
    canPauseGoal,
    canResumeGoal,
    canSaveGoalObjective,
    canSendChat,
    currentQueueSessionKey,
    effectiveWorkMode,
    goalPaused,
    goalPauseRequested,
    goalTimedOut,
    hasBlockingRun,
    pendingInputRequest,
    primaryRootRun,
    queuedMessages,
    showGoalObjectiveEditor,
    slashCommands,
  } = useADKRunProjection({
    activeGoalRunSnapshot,
    activeRun,
    activeRunSnapshot,
    agents: sessionState.agents,
    chatDraft,
    composerBlockMessage,
    goalLifecycleBusy,
    goalObjectiveDraft,
    goalObjectiveSaving,
    queuedChatMessages,
    selectedAgentId: sessionState.selectedAgentId,
    selectedSessionId: sessionState.selectedSessionId,
    sendingChat,
    workModeOverride,
    workflowQueues,
  });
  const {
    applyComposerState,
    draftRevision,
    emptyComposerState,
    flushComposerState,
    markComposerStateDirty,
    resetComposerState,
  } = useADKComposerPersistence({
    activeGoalRun,
    agents: sessionState.agents,
    chatDraft,
    effectiveWorkMode,
    goalObjectiveDraft,
    goalObjectiveError,
    goalObjectiveTouched,
    permissionModeOverride,
    selectedAgentId: sessionState.selectedAgentId,
    selectedProvider: sessionState.selectedProvider,
    selectedProviderId: sessionState.selectedProviderId,
    selectedSessionId: sessionState.selectedSessionId,
    workModeOverride,
  });
  const {
    cancelActiveRun,
    pauseGoalRun,
    resumeGoalRun,
    updateGoalObjective,
    updateGoalObjectiveDraft,
  } = useADKRunActions({
    abortActiveChatStream,
    abortReconnectStream: streamRuntime.abortReconnectStream,
    activeGoalRun,
    activeGoalRunId,
    activeRunControlId,
    clearSessionRuntimeState,
    errorMessage: sessionState.errorMessage,
    goalLifecycleBusy,
    goalObjectiveDraft,
    goalObjectiveError,
    goalObjectiveSaving,
    goalObjectiveTouched,
    handleTerminalRun,
    interruptingRunId,
    reloadSessionTimeline,
    selectedSessionId: sessionState.selectedSessionId,
    syncActiveRun,
    waitForRunContinuation,
  });

  watch(
    () => sessionState.initialized.value,
    (initialized) => {
      if (!initialized || pageStateRestored) return;
      pageStateRestored = true;
      void restoreADKPageState();
    },
    { immediate: true },
  );

  onBeforeUnmount(() => {
    abortActiveChatStream();
    streamRuntime.abortReconnectStream();
  });

  async function selectSession(sessionId: string): Promise<void> {
    if (sessionState.selectedSessionId.value === sessionId) return;
    abortActiveChatStream();
    streamRuntime.abortReconnectStream();
    await flushComposerState();
    if (
      activeRun.value &&
      activeRun.value.sessionId &&
      activeRun.value.sessionId !== sessionId
    ) {
      activeRun.value = null;
    }
    sessionState.selectedSessionId.value = sessionId;
    timelineEntries.value = [];
    clearWorkflowPlanRun();
    clearSessionContext();
    if (
      activeRunSnapshot.value &&
      activeRunSnapshot.value.sessionId !== sessionId
    ) {
      activeRunSnapshot.value = null;
    }
    if (
      activeGoalRunSnapshot.value &&
      activeGoalRunSnapshot.value.sessionId !== sessionId
    ) {
      activeGoalRunSnapshot.value = null;
    }
    const detail = await loadSessionChatHistory(sessionId).catch(() => null);
    if (detail == null) {
      // Session may not have timeline entries yet.
      applyComposerState(emptyComposerState(sessionId));
    } else {
      timelineEntries.value = detail.timelineEntries;
      await restoreSessionRuns(detail.runs);
      await sessionState.finishSessionSelection(detail.session.agentId);
      applyComposerState(detail.composerState);
      const runtimeState = sessionRuntimeState(sessionId);
      const savedChildRun = detail.runs.find(
        (run) => run.id === runtimeState.activeChildRunId,
      );
      workflowQueues.setActiveChildRunId(
        savedChildRun && !isTerminalRunStatus(savedChildRun.status)
          ? savedChildRun.id
          : "",
      );
      await streamRuntime.reconnectSessionStream(sessionId, detail.runs);
    }
    await refreshSessionContext(sessionId);
    await dispatchQueuedMessagesIfIdle();
  }

  async function restoreADKPageState(): Promise<void> {
    const sessionId = pageState.selectedSessionId;
    if (
      sessionId === "" ||
      !sessionState.sessions.value.some((session) => session.id === sessionId)
    ) {
      if (sessionId !== "") {
        removeSessionRuntimeState(sessionId);
      }
      return;
    }
    await selectSession(sessionId);
  }

  async function restoreSessionRuns(runs: ADKRun[]): Promise<void> {
    for (const run of runs) {
      await workflowQueues.syncWorkflowRun(run);
    }
    const activeRootRun = [...runs]
      .reverse()
      .find((run) => isRootRun(run) && isBlockingRunStatus(run.status));
    if (activeRootRun) {
      syncActiveRun(activeRootRun, true);
      updateSessionRuntimeState(activeRootRun.sessionId ?? "", {
        runId: activeRootRun.id,
      });
      return;
    }
    const activeGoalRootRun = [...runs]
      .reverse()
      .find((run) => isActiveGoalParentRun(run));
    if (activeGoalRootRun) {
      syncActiveRun(activeGoalRootRun, false);
    }
  }

  async function sendChat(): Promise<void> {
    const text = chatDraft.value.trim();
    if (
      text === "" ||
      sessionState.selectedAgentId.value === "" ||
      composerBlockMessage.value !== ""
    ) {
      return;
    }
    if (sendAdmissionLocked) {
      return;
    }
    sendAdmissionLocked = true;
    let handedOff = false;
    try {
      if (await handleExactSlashCommand(text)) {
        chatDraft.value = "";
        markComposerStateDirty();
        await flushComposerState();
        return;
      }

      const clientRequestId =
        retryDraftRequest?.text === text &&
        retryDraftRequest.draftRevision === draftRevision.value
          ? retryDraftRequest.clientRequestId
          : crypto.randomUUID();
      retryDraftRequest = null;

      if (hasBlockingRun.value || sendingChat.value) {
        enqueueChatMessage(text, "queued", {
          forceChat: shouldSendCurrentDraftAsGoalConversation(),
          clientRequestId,
        });
        chatDraft.value = "";
        markComposerStateDirty();
        await flushComposerState();
        await scrollToBottom(threadRef);
        return;
      }

      const draftBeforeSend = chatDraft.value;
      chatDraft.value = "";
      markComposerStateDirty();
      await flushComposerState();
      const execution = executeChatMessage(
        text,
        {
          forceChat: shouldSendCurrentDraftAsGoalConversation(),
        },
        clientRequestId,
      );
      handedOff = true;
      sendAdmissionLocked = false;
      const sent = await execution;
      if (!sent) {
        chatDraft.value = draftBeforeSend;
        retryDraftRequest = {
          text,
          clientRequestId,
          draftRevision: draftRevision.value,
        };
        markComposerStateDirty();
        await flushComposerState();
      }
    } finally {
      if (!handedOff) {
        sendAdmissionLocked = false;
      }
    }
  }

  async function interruptAndQueueChat(): Promise<void> {
    const text = chatDraft.value.trim();
    if (
      text === "" ||
      sessionState.selectedAgentId.value === "" ||
      composerBlockMessage.value !== ""
    ) {
      return;
    }
    if (!hasBlockingRun.value && !sendingChat.value) {
      await sendChat();
      return;
    }

    const currentRunId = activeRunControlId.value;
    enqueueChatMessage(text, "interrupt", {
      forceChat: shouldSendCurrentDraftAsGoalConversation(),
    });
    chatDraft.value = "";
    markComposerStateDirty();
    await flushComposerState();
    await scrollToBottom(threadRef);
    if (!currentRunId || interruptingRunId.value === currentRunId) {
      return;
    }
    interruptingRunId.value = currentRunId;
    await cancelActiveRun(currentRunId);
  }

  const revokeQueuedMessage = (messageId: string) =>
    revokeADKQueuedMessage(queuedChatMessages, messageId);

  const handleComposerKeydown = (event: KeyboardEvent) =>
    handleADKComposerKeydown(event, sendChat);

  return {
    activeRunId,
    activeRunStatus,
    approvalsBusy,
    canInterruptChat,
    canSendChat,
    chatDraft,
    contextBusy,
    contextDetailsOpen,
    goalObjectiveDraft,
    goalObjectiveError,
    goalObjectiveSaving,
    goalLifecycleBusy,
    goalPaused,
    goalTimedOut,
    goalPauseRequested,
    showGoalObjectiveEditor,
    canSaveGoalObjective,
    canPauseGoal,
    canResumeGoal,
    hasBlockingRun,
    interruptingRunId,
    queuedMessages,
    queueDispatchingId,
    revokeQueuedMessage,
    sessionContext,
    visibleSessionContext,
    slashCommands,
    activeChildRunId: workflowQueues.activeChildRunId,
    childRunItems: workflowQueues.childRunItems,
    childTimelineEntries: workflowQueues.childTimelineEntries,
    childViewContext: workflowQueues.childViewContext,
    parentTimelineEntries: workflowQueues.parentTimelineEntries,
    parentApprovalQueue: workflowQueues.parentApprovalQueue,
    selectedApprovalQueue: workflowQueues.selectedApprovalQueue,
    setActiveChildRunId: workflowQueues.setActiveChildRunId,
    visibleTimelineEntries: workflowQueues.visibleTimelineEntries,
    visibleWorkflowPlanRun: workflowQueues.visibleWorkflowPlanRun,
    clearSessionContext,
    initializeSessionContext,
    clearWorkflowPlanRun,
    flushComposerState,
    resetComposerState,
    removeSessionRuntimeState,
    handleComposerKeydown,
    interruptAndQueueChat,
    pauseGoalRun,
    resumeGoalRun,
    openContextDetails,
    runSlashCommand,
    cancelActiveRun,
    denyAllApprovals,
    denyApproval,
    resolveAllApprovals,
    resolveApproval,
    inputRequestBusy,
    submitInputResponse,
    selectSession,
    sendChat,
    sendingChat,
    activityIndicator,
    timelineEntries,
    updateGoalObjective,
    updateGoalObjectiveDraft,
    workModeOverride,
    permissionModeOverride,
    pendingInputRequest,
  };

  async function executeChatMessage(
    text: string,
    options: { forceChat?: boolean } = {},
    clientRequestId: string = crypto.randomUUID(),
  ): Promise<boolean> {
    const payload: Parameters<typeof streamADKChat>[0] = {
      clientRequestId,
      agentId: sessionState.selectedAgentId.value,
      sessionId: sessionState.selectedSessionId.value,
      message: text,
    };
    const providerId = sessionState.selectedProviderId.value.trim();
    if (providerId !== "") {
      payload.providerId = providerId;
    }
    const model = sessionState.selectedProvider.value?.model?.trim() ?? "";
    if (model !== "") {
      payload.model = model;
    }
    if (permissionModeOverride.value) {
      payload.permissionModeOverride = permissionModeOverride.value;
    }
    const mode = effectiveWorkMode.value;
    if (options.forceChat) {
      payload.workModeOverride = "chat";
    } else if (mode) {
      payload.workModeOverride = mode;
      if (mode === "loop") {
        payload.objective = goalObjectiveDraft.value.trim() || text;
      }
    }
    const optimisticUserEntry = createTimelineEntryState({
      id: `local-user-${clientRequestId}`,
      sessionId: sessionState.selectedSessionId.value,
      kind: "user_message",
      createdAt: new Date().toISOString(),
      sequence: timelineEntries.value.length + 1,
      status: "streaming",
      text,
    });
    if (!options.forceChat) {
      clearWorkflowPlanRun();
    }
    sendingChat.value = true;
    timelineEntries.value = upsertTimelineEntry(
      timelineEntries.value,
      optimisticUserEntry,
    );
    await scrollToBottom(threadRef);
    const controller = new AbortController();
    chatStreamController = controller;
    chatStreamAbortReason = "";
    let streamAbortedForGoalPause = false;

    try {
      const response = await streamADKChat(payload, streamRuntime.handleChatStreamEvent, {
        signal: controller.signal,
      });
      await finalizeStreamResponse(response);
      await flushComposerState();
      return true;
    } catch (error) {
      if (isGoalPauseAbort(controller, error)) {
        streamAbortedForGoalPause = true;
        await flushComposerState();
        return true;
      }
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "Agents chat failed";
      await scrollToBottom(threadRef);
      return false;
    } finally {
      if (chatStreamController === controller) {
        chatStreamController = null;
        chatStreamAbortReason = "";
      }
      sendingChat.value = false;
      if (!streamAbortedForGoalPause || !goalPauseRequested.value) {
        await dispatchQueuedMessagesIfIdle();
      }
    }
  }

  async function finalizeStreamResponse(
    response: ADKChatResponse,
  ): Promise<void> {
    const resolution = resolveGoalAwareChatResponse(response, syncActiveRun);
    setADKSelectedSessionId(
      sessionState.selectedSessionId,
      queuedChatMessages,
      resolution.normalizedResponse.session.id,
    );
    if (resolution.staleTerminalGoalPauseOverride) {
      clearSessionRuntimeState(resolution.normalizedResponse.session.id);
      await reloadSessionTimeline(resolution.normalizedResponse.session.id);
      return;
    }
    await applyAuthoritativeTimeline(resolution.resolvedResponse);
    if (resolution.normalizedResponse.context) {
      applySessionContext(resolution.normalizedResponse.context);
    } else {
      await refreshSessionContext(resolution.normalizedResponse.session.id);
    }
    await sessionState.refreshAll();
    if (resolution.failMessage) {
      sessionState.errorMessage.value = resolution.failMessage;
    }
    if (resolution.terminal) {
      clearSessionRuntimeState(resolution.normalizedResponse.session.id);
    }
    await scrollToBottom(threadRef);
  }

  async function applyAuthoritativeTimeline(
    response: ADKChatResponse,
  ): Promise<void> {
    timelineEntries.value = replaceAuthoritativeChatResponseTimeline(
      response,
      timelineEntries.value,
    );
    await scrollToBottom(threadRef);
  }

  async function reloadSessionTimeline(sessionId: string): Promise<void> {
    if (!sessionId || sessionState.selectedSessionId.value !== sessionId) {
      return;
    }
    const detail = await loadSessionChatHistory(sessionId);
    timelineEntries.value = detail.timelineEntries;
    await refreshSessionContext(sessionId);
    await scrollToBottom(threadRef);
  }

  async function handleExactSlashCommand(text: string): Promise<boolean> {
    const normalized = text.trim().toLowerCase();
    const match = slashCommands.value.find(
      (item) => item.command === normalized,
    );
    if (!match || match.disabled) {
      return false;
    }
    await runSlashCommand(match.id);
    return true;
  }

  function enqueueChatMessage(
    text: string,
    mode: "queued" | "interrupt",
    options: { forceChat?: boolean; clientRequestId?: string } = {},
  ): QueuedChatMessage {
    return enqueueADKChatMessage(
      queuedChatMessages,
      currentQueueSessionKey.value,
      text,
      mode,
      options,
    );
  }

  async function dispatchQueuedMessagesIfIdle(): Promise<void> {
    if (
      isQueueDispatchBlockedByGoalLifecycle({
        sendingChat: sendingChat.value,
        hasBlockingRun: hasBlockingRun.value,
        goalPauseRequested: goalPauseRequested.value,
        goalPaused: goalPaused.value,
        queueDispatchingId: queueDispatchingId.value,
      }) ||
      composerBlockMessage.value !== "" ||
      sessionState.selectedAgentId.value === ""
    ) {
      return;
    }
    const nextMessage = queuedMessages.value[0];
    if (!nextMessage) {
      return;
    }
    queueDispatchingId.value = nextMessage.id;
    const sent = await executeChatMessage(nextMessage.text, {
      forceChat: nextMessage.forceChat === true,
    }, nextMessage.clientRequestId);
    if (sent) {
      queuedChatMessages.value = queuedChatMessages.value.filter(
        (message) => message.id !== nextMessage.id,
      );
    }
    queueDispatchingId.value = "";
    if (sent) {
      await dispatchQueuedMessagesIfIdle();
    }
  }

  function syncActiveRun(
    incomingRun: ADKRun | undefined,
    waitingForContinuation = false,
  ): ADKRun | undefined {
    const result = syncGoalAwareActiveRun({
      incomingRun,
      waitingForContinuation,
      activeRunSnapshot: activeRunSnapshot.value,
      activeGoalRunSnapshot: activeGoalRunSnapshot.value,
      activeRunState: activeRun.value,
      goalObjectiveState: currentGoalObjectiveState(),
      goalObjectiveSaving: goalObjectiveSaving.value,
      syncWorkflowRun: workflowQueues.syncWorkflowRun,
    });
    activeRunSnapshot.value = result.activeRunSnapshot;
    activeGoalRunSnapshot.value = result.activeGoalRunSnapshot;
    activeRun.value = result.activeRunState;
    applyGoalObjectiveState(result.goalObjectiveState);
    if (result.goalObjectiveCleared) {
      markComposerStateDirty();
    }
    return result.run;
  }

  function clearWorkflowPlanRun(): void {
    workflowQueues.clearWorkflowQueues();
  }

  function shouldSendCurrentDraftAsGoalConversation(): boolean {
    return effectiveWorkMode.value === "loop" && activeGoalRun.value != null;
  }

  function abortActiveChatStream(reason = ""): void {
    if (!chatStreamController) {
      return;
    }
    chatStreamAbortReason = reason;
    chatStreamController.abort();
  }

  function isGoalPauseAbort(
    controller: AbortController,
    error: unknown,
  ): boolean {
    return isGoalPauseAbortError(controller, error, chatStreamAbortReason);
  }

  function currentGoalObjectiveState(): GoalObjectiveState {
    return {
      draft: goalObjectiveDraft.value,
      touched: goalObjectiveTouched.value,
      error: goalObjectiveError.value,
    };
  }

  function applyGoalObjectiveState(state: GoalObjectiveState): void {
    goalObjectiveDraft.value = state.draft;
    goalObjectiveTouched.value = state.touched;
    goalObjectiveError.value = state.error;
  }
}
