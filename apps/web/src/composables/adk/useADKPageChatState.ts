import { onBeforeUnmount, ref, watch, type Ref } from "vue";

import type {
  ADKAgent,
  ADKProvider,
  ADKRun,
  ADKSession,
} from "@/types";

import { isTerminalRunStatus } from "@/composables/adk/adkChatPresentation";
import {
  isActiveGoalParentRun,
  isQueueDispatchBlockedByGoalLifecycle,
  isRootRun,
  isBlockingRunStatus,
  syncGoalAwareActiveRun,
  type ActiveChatRunState,
  type GoalObjectiveState,
  type QueuedChatMessage,
} from "@/composables/adk/adkChatRuntime";
import { scrollToBottom } from "@/composables/adk/adkThreadScroll";
import { loadSessionChatHistory } from "@/composables/adk/adkPageRunHistory";
import { type ADKTimelineEntryState } from "@/composables/adk/adkTimeline";
import { useADKWorkflowQueueState } from "@/composables/adk/useADKWorkflowQueueState";
import { useADKComposerPersistence } from "@/composables/adk/useADKComposerPersistence";
import { useADKSessionContextState } from "@/composables/adk/useADKSessionContextState";
import { useADKApprovalRuntime } from "@/composables/adk/useADKApprovalRuntime";
import { useADKRunActions } from "@/composables/adk/useADKRunActions";
import { useADKPageRuntimePersistence } from "@/composables/adk/useADKPageRuntimePersistence";
import { useADKStreamRuntime } from "@/composables/adk/useADKStreamRuntime";
import { useADKPageChatExecution } from "@/composables/adk/useADKPageChatExecution";
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
  let streamRuntime!: ReturnType<typeof useADKStreamRuntime>;
  const chatExecution = useADKPageChatExecution({
    applySessionContext,
    clearSessionRuntimeState,
    dispatchQueuedMessagesIfIdle,
    effectiveWorkMode,
    errorMessage: sessionState.errorMessage,
    flushComposerState,
    goalObjectiveDraft,
    goalPauseRequested,
    permissionModeOverride,
    refreshAll: sessionState.refreshAll,
    refreshSessionContext,
    reloadSessionTimeline,
    scrollTarget: threadRef,
    selectedAgentId: sessionState.selectedAgentId,
    selectedProvider: sessionState.selectedProvider,
    selectedProviderId: sessionState.selectedProviderId,
    selectedSessionId: sessionState.selectedSessionId,
    sendingChat,
    setSelectedSessionId: (sessionId) =>
      setADKSelectedSessionId(
        sessionState.selectedSessionId,
        queuedChatMessages,
        sessionId,
      ),
    syncActiveRun,
    timelineEntries,
    clearWorkflowPlanRun,
    onStreamEvent: (event) => streamRuntime.handleChatStreamEvent(event),
  });
  streamRuntime = useADKStreamRuntime({
    activeRunSnapshot,
    activeGoalRunSnapshot,
    applyAuthoritativeTimeline: chatExecution.applyAuthoritativeTimeline,
    applySessionContext,
    clearSessionRuntimeState,
    errorMessage: sessionState.errorMessage,
    finalizeStreamResponse: chatExecution.finalizeStreamResponse,
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
  const { abortActiveChatStream, executeChatMessage } = chatExecution;
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
