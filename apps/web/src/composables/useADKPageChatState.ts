import { computed, nextTick, onBeforeUnmount, ref, watch, type Ref } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKProvider,
  ADKRun,
  ADKSession,
  ADKSessionComposerState,
  ADKSessionContextSnapshot,
} from "@/contracts";

import {
  resolveADKApprovalBatchOnce,
  type ADKApprovalAction,
} from "./adkApprovalResolution";
import {
  isTerminalRunStatus,
  isUserPausedGoalRun,
  runTerminalMessage,
} from "./adkChatPresentation";
import {
  resumeADKChatStream,
  streamADKChat,
  type ADKChatStreamEvent,
} from "./adkChatStream";
import {
  normalizeADKApprovalResolution,
  normalizeADKRun,
  normalizeADKTimelineEntry,
} from "./adkNormalization";
import {
  buildQueueSessionKey,
  createQueuedChatMessage,
  isActiveGoalParentRun,
  isGoalPauseAbortError,
  isQueueDispatchBlockedByGoalLifecycle,
  isRootRun,
  isResumableTimedOutGoalRun,
  isUserPauseRequestedGoalRun,
  resolveGoalAwareChatResponse,
  selectActiveGoalRun,
  selectPrimaryRootRun,
  isBlockingRunStatus,
  shouldWaitForRunContinuation,
  syncGoalAwareActiveRun,
  waitForGoalAwareRunContinuation,
  type ActiveChatRunState,
  type GoalObjectiveState,
  type QueuedChatMessage,
} from "./adkChatRuntime";
import { monitorADKRunContinuation } from "./adkRunContinuation";
import { scrollToBottom } from "./adkThreadScroll";
import {
  loadSessionChatHistory,
  normalizeSessionComposerState,
} from "./adkPageRunHistory";
import { saveADKSessionComposerState } from "./adkPageSessionApi";
import {
  emptyADKSessionRuntimeState,
  readADKPagePersistentState,
  writeADKPagePersistentState,
  type ADKSessionRuntimeState,
} from "./adkPagePersistence";
import {
  applyApprovalResolutions,
  createTimelineEntryState,
  replaceAuthoritativeChatResponseTimeline,
  upsertTimelineEntry,
  type ADKTimelineEntryState,
} from "./adkTimeline";
import {
  compactADKSessionContext,
  fetchADKSessionContext,
} from "./adkSessionContextApi";
import { fetchEnvelopeWithInit } from "./apiClient";
import { useADKWorkflowQueueState } from "./useADKWorkflowQueueState";

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

export interface SlashCommandItem {
  id: "context" | "compact" | "compact-aggressive";
  command: "/context" | "/compact" | "/compact-aggressive";
  title: string;
  description: string;
  disabled?: boolean;
}

const COMPOSER_STATE_SAVE_DELAY_MS = 600;
const CONTEXT_REFRESH_THROTTLE_MS = 1000;

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
  const approvalsBusy = ref(false);
  const resolvingApprovalIds = ref<Set<string>>(new Set());
  const contextBusy = ref(false);
  const contextDetailsOpen = ref(false);
  const sessionContext = ref<ADKSessionContextSnapshot | null>(null);
  const goalObjectiveDraft = ref("");
  const goalObjectiveTouched = ref(false);
  const goalObjectiveSaving = ref(false);
  const goalObjectiveError = ref("");
  const goalLifecycleBusy = ref(false);
  let applyingComposerState = false;
  let composerSaveTimer: ReturnType<typeof window.setTimeout> | null = null;
  let composerDirty = false;
  let composerRevision = 0;
  let composerFlushPromise: Promise<void> | null = null;
  let pageStateRestored = false;
  let streamReconnectController: AbortController | null = null;
  let chatStreamController: AbortController | null = null;
  let chatStreamAbortReason = "";
  let lastContextRefreshAt = 0;
  const pageState = readADKPagePersistentState();
  const flushComposerStateBeforeUnload = () => {
    void flushComposerState({ keepalive: true });
  };
  const workflowQueues = useADKWorkflowQueueState({
    timelineEntries,
    selectedSessionId: sessionState.selectedSessionId,
    resolvingApprovalIds,
  });

  function applySessionContext(
    incoming: ADKSessionContextSnapshot | null | undefined,
  ): void {
    if (!incoming) return;
    const current = sessionContext.value;
    if (!current || current.sessionId !== incoming.sessionId) {
      sessionContext.value = incoming;
      return;
    }
    const currentRevision = current.contextRevisionId?.trim() ?? "";
    const incomingRevision = incoming.contextRevisionId?.trim() ?? "";
    if (currentRevision === incomingRevision) {
      sessionContext.value = incoming;
      return;
    }
    if (
      incoming.previousContextRevisionId?.trim() === currentRevision ||
      currentRevision === ""
    ) {
      sessionContext.value = incoming;
      return;
    }
    if (
      current.previousContextRevisionId?.trim() === incomingRevision ||
      incomingRevision === ""
    ) {
      return;
    }
    const currentCreatedAt = Date.parse(
      current.contextRevisionCreatedAt?.trim() ?? "",
    );
    const incomingCreatedAt = Date.parse(
      incoming.contextRevisionCreatedAt?.trim() ?? "",
    );
    if (
      Number.isFinite(currentCreatedAt) &&
      Number.isFinite(incomingCreatedAt) &&
      incomingCreatedAt < currentCreatedAt
    ) {
      return;
    }
    sessionContext.value = incoming;
  }

  const activeRunId = computed(() => activeRun.value?.runId ?? "");
  const activeRunStatus = computed(() => activeRun.value?.status ?? "");
  const activeGoalRun = computed(() =>
    selectActiveGoalRun({
      activeRunSnapshot: activeRunSnapshot.value,
      activeGoalRunSnapshot: activeGoalRunSnapshot.value,
      workflowRun: workflowQueues.parentWorkflowPlanRun.value,
    }),
  );
  const primaryRootRun = computed(() =>
    selectPrimaryRootRun({
      activeRunSnapshot: activeRunSnapshot.value,
      activeGoalRunSnapshot: activeGoalRunSnapshot.value,
      workflowRun: workflowQueues.parentWorkflowPlanRun.value,
    }),
  );
  const selectedAgentDefaultWorkMode = computed(() => {
    const agent = sessionState.agents.value.find(
      (candidate) => candidate.id === sessionState.selectedAgentId.value,
    );
    const mode = String(agent?.workMode ?? "").trim();
    return mode === "loop" ? mode : "chat";
  });
  const effectiveWorkMode = computed(() => {
    const mode = String(
      workModeOverride.value || selectedAgentDefaultWorkMode.value,
    ).trim();
    return mode === "loop" ? mode : "chat";
  });
  const showGoalObjectiveEditor = computed(
    () => activeGoalRun.value != null || goalObjectiveDraft.value.trim() !== "",
  );
  const canSaveGoalObjective = computed(() => {
    const run = activeGoalRun.value;
    return (
      !!run &&
      !goalObjectiveSaving.value &&
      goalObjectiveDraft.value.trim() !== "" &&
      goalObjectiveDraft.value.trim() !== String(run.objective ?? "").trim()
    );
  });
  const activeGoalRunId = computed(() => activeGoalRun.value?.id ?? "");
  const goalPaused = computed(() =>
    isUserPausedGoalRun(activeGoalRun.value ?? undefined),
  );
  const goalTimedOut = computed(() =>
    isResumableTimedOutGoalRun(activeGoalRun.value ?? undefined),
  );
  const goalPauseRequested = computed(
    () => isUserPauseRequestedGoalRun(activeGoalRun.value ?? undefined),
  );
  const canPauseGoal = computed(() => {
    const run = activeGoalRun.value;
    return (
      !!run &&
      run.status === "RUNNING" &&
      !goalPauseRequested.value &&
      !goalLifecycleBusy.value
    );
  });
  const canResumeGoal = computed(
    () => (goalPaused.value || goalTimedOut.value) && !goalLifecycleBusy.value,
  );
  const hasBlockingRun = computed(() =>
    primaryRootRun.value
      ? isBlockingRunStatus(primaryRootRun.value.status)
      : isBlockingRunStatus(activeRun.value?.status),
  );
  const activeRunControlId = computed(() => {
    if (
      primaryRootRun.value &&
      isBlockingRunStatus(primaryRootRun.value.status)
    ) {
      return primaryRootRun.value.id;
    }
    return activeRun.value?.runId ?? "";
  });
  const currentQueueSessionKey = computed(() =>
    buildQueueSessionKey(sessionState.selectedSessionId.value),
  );
  const queuedMessages = computed(() =>
    queuedChatMessages.value.filter(
      (message) => message.sessionKey === currentQueueSessionKey.value,
    ),
  );
  const canSendChat = computed(
    () =>
      chatDraft.value.trim() !== "" &&
      sessionState.selectedAgentId.value !== "" &&
      composerBlockMessage.value === "",
  );
  const canInterruptChat = computed(
    () => canSendChat.value && hasBlockingRun.value,
  );
  const activityIndicator = computed<"idle" | "typing" | "child_finished">(() => {
    if (!sendingChat.value && !hasBlockingRun.value) return "idle";
    const parent = workflowQueues.parentWorkflowPlanRun.value;
    const children = workflowQueues.parentChildRunItems.value;
    const parentActive =
      !!parent &&
      parent.status !== "COMPLETED" &&
      parent.status !== "FAILED" &&
      parent.status !== "CANCELLED" &&
      parent.status !== "DENIED" &&
      parent.status !== "TIMED_OUT";
    const childrenFinished =
      children.length > 0 &&
      children.every((child) =>
        ["COMPLETED", "DONE", "FAILED", "CANCELLED", "DENIED", "TIMED_OUT"].includes(
          String(child.status).trim().toUpperCase(),
        ),
      );
    return parentActive && childrenFinished ? "child_finished" : "typing";
  });
  const slashCommands = computed<SlashCommandItem[]>(() => {
    const hasSession = sessionState.selectedSessionId.value.trim() !== "";
    return [
      {
        id: "context",
        command: "/context",
        title: "查看上下文占用",
        description: hasSession
          ? "打开当前会话的上下文详情"
          : "需要先创建或选择一个会话",
        disabled: !hasSession,
      },
      {
        id: "compact",
        command: "/compact",
        title: "压缩当前会话",
        description: hasSession
          ? "执行标准上下文压缩"
          : "需要先创建或选择一个会话",
        disabled: !hasSession,
      },
      {
        id: "compact-aggressive",
        command: "/compact-aggressive",
        title: "激进压缩当前会话",
        description: hasSession
          ? "执行更强的摘要压缩"
          : "需要先创建或选择一个会话",
        disabled: !hasSession,
      },
    ];
  });
  const visibleSessionContext = computed(() => sessionContext.value);

  watch(effectiveWorkMode, (mode) => {
    if (applyingComposerState) return;
    if (activeGoalRun.value) return;
    if (mode !== "loop") {
      goalObjectiveTouched.value = false;
      goalObjectiveDraft.value = "";
      goalObjectiveError.value = "";
      return;
    }
    if (!goalObjectiveTouched.value) {
      goalObjectiveDraft.value = chatDraft.value;
    }
  });

  watch(chatDraft, () => {
    if (applyingComposerState) return;
    if (activeGoalRun.value) return;
    if (effectiveWorkMode.value !== "loop") return;
    if (!goalObjectiveTouched.value) {
      goalObjectiveDraft.value = chatDraft.value;
    }
  });

  watch(
    () => [
      chatDraft.value,
      sessionState.selectedProviderId.value,
      workModeOverride.value,
      permissionModeOverride.value,
      goalObjectiveDraft.value,
      goalObjectiveTouched.value,
    ],
    () => {
      if (applyingComposerState) return;
      markComposerStateDirty();
    },
  );

  watch(
    () => sessionState.initialized.value,
    (initialized) => {
      if (!initialized || pageStateRestored) return;
      pageStateRestored = true;
      void restoreADKPageState();
    },
    { immediate: true },
  );

  watch(workflowQueues.activeChildRunId, (runId) => {
    const sessionId = sessionState.selectedSessionId.value.trim();
    if (sessionId === "") return;
    updateSessionRuntimeState(sessionId, { activeChildRunId: runId });
  });

  watch(sessionState.selectedSessionId, (sessionId) => {
    pageState.selectedSessionId = sessionId.trim();
    writeADKPagePersistentState(pageState);
  });

  watch(contextDetailsOpen, (open) => {
    if (!open) return;
    void refreshSessionContext(undefined, true);
  });

  onBeforeUnmount(() => {
    abortActiveChatStream();
    streamReconnectController?.abort();
    streamReconnectController = null;
    window.removeEventListener("pagehide", flushComposerStateBeforeUnload);
    window.removeEventListener("beforeunload", flushComposerStateBeforeUnload);
    if (composerSaveTimer !== null) {
      window.clearTimeout(composerSaveTimer);
      composerSaveTimer = null;
    }
    void flushComposerState();
  });
  window.addEventListener("pagehide", flushComposerStateBeforeUnload);
  window.addEventListener("beforeunload", flushComposerStateBeforeUnload);

  async function selectSession(sessionId: string): Promise<void> {
    if (sessionState.selectedSessionId.value === sessionId) return;
    abortActiveChatStream();
    streamReconnectController?.abort();
    streamReconnectController = null;
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
      await reconnectSessionStream(sessionId, detail.runs);
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
        pageState.selectedSessionId = "";
        writeADKPagePersistentState(pageState);
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

  async function reconnectSessionStream(
    sessionId: string,
    runs: ADKRun[],
  ): Promise<void> {
    const runtimeState = sessionRuntimeState(sessionId);
    const activeRootRun = [...runs]
      .reverse()
      .find((run) => isRootRun(run) && isBlockingRunStatus(run.status));
    const runId = runtimeState.runId || activeRootRun?.id || "";
    if (runtimeState.streamId === "" && runId === "") {
      return;
    }

    streamReconnectController?.abort();
    const controller = new AbortController();
    streamReconnectController = controller;
    try {
      const response = await resumeADKChatStream(
        {
          streamId: runtimeState.streamId,
          runId,
          after: runtimeState.sequence,
          signal: controller.signal,
        },
        handleChatStreamEvent,
      );
      if (response == null) {
        const run =
          activeRootRun ?? runs.find((candidate) => candidate.id === runId);
        if (run && !isTerminalRunStatus(run.status)) {
          await waitForRunContinuation(run);
        }
        return;
      }
      await finalizeStreamResponse(response);
    } catch (error) {
      if (controller.signal.aborted) return;
      const run =
        activeRootRun ?? runs.find((candidate) => candidate.id === runId);
      if (run && !isTerminalRunStatus(run.status)) {
        await waitForRunContinuation(run);
        return;
      }
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "恢复推理流失败";
    } finally {
      if (streamReconnectController === controller) {
        streamReconnectController = null;
      }
    }
  }

  function sessionRuntimeState(sessionId: string): ADKSessionRuntimeState {
    const normalized = sessionId.trim();
    if (normalized === "") {
      return emptyADKSessionRuntimeState();
    }
    pageState.sessions[normalized] ??= emptyADKSessionRuntimeState();
    return pageState.sessions[normalized]!;
  }

  function updateSessionRuntimeState(
    sessionId: string,
    patch: Partial<ADKSessionRuntimeState>,
  ): void {
    const normalized = sessionId.trim();
    if (normalized === "") return;
    pageState.sessions[normalized] = {
      ...sessionRuntimeState(normalized),
      ...patch,
    };
    writeADKPagePersistentState(pageState);
  }

  function clearSessionRuntimeState(sessionId: string): void {
    updateSessionRuntimeState(sessionId, {
      streamId: "",
      runId: "",
      sequence: 0,
    });
  }

  function removeSessionRuntimeState(sessionId: string): void {
    const normalized = sessionId.trim();
    if (normalized === "") return;
    delete pageState.sessions[normalized];
    if (pageState.selectedSessionId === normalized) {
      pageState.selectedSessionId = "";
    }
    writeADKPagePersistentState(pageState);
  }

  async function handleChatStreamEvent(
    event: ADKChatStreamEvent,
  ): Promise<void> {
    const eventSessionId =
      event.session?.id ||
      event.run?.sessionId ||
      event.response?.session.id ||
      sessionState.selectedSessionId.value;
    const selectedSessionId = sessionState.selectedSessionId.value.trim();
    if (
      selectedSessionId !== "" &&
      eventSessionId !== "" &&
      selectedSessionId !== eventSessionId
    ) {
      return;
    }
    if (eventSessionId) {
      setSelectedSessionId(eventSessionId);
    }
    const runId = event.run?.id || event.response?.run.id || event.runId || "";
    if (eventSessionId && (event.streamId || event.sequence || runId)) {
      updateSessionRuntimeState(eventSessionId, {
        streamId:
          event.streamId || sessionRuntimeState(eventSessionId).streamId,
        runId: runId || sessionRuntimeState(eventSessionId).runId,
        sequence: Math.max(
          sessionRuntimeState(eventSessionId).sequence,
          event.sequence ?? 0,
        ),
      });
    }
    if (event.type === "context" && event.context) {
      applySessionContext(event.context);
    }
    if (event.type === "run" && event.run?.id) {
      syncActiveRun(normalizeADKRun(event.run), true);
      scheduleSessionContextRefresh(eventSessionId);
    }
    if (event.type === "timeline" && event.timeline) {
      timelineEntries.value = upsertTimelineEntry(
        timelineEntries.value,
        normalizeADKTimelineEntry(event.timeline),
      );
      await scrollToBottom(threadRef);
    }
    if (event.type === "final" && event.response) {
      const resolution = resolveGoalAwareChatResponse(
        event.response,
        syncActiveRun,
      );
      if (resolution.staleTerminalGoalPauseOverride) {
        clearSessionRuntimeState(resolution.normalizedResponse.session.id);
        await reloadSessionTimeline(resolution.normalizedResponse.session.id);
        return;
      }
      await applyAuthoritativeTimeline(resolution.resolvedResponse);
      if (resolution.normalizedResponse.context) {
        applySessionContext(resolution.normalizedResponse.context);
      }
      if (resolution.failMessage) {
        sessionState.errorMessage.value = resolution.failMessage;
      }
      if (resolution.terminal) {
        clearSessionRuntimeState(resolution.normalizedResponse.session.id);
      }
    }
    if (event.type === "error") {
      if (eventSessionId) {
        clearSessionRuntimeState(eventSessionId);
      }
      throw new Error(event.message || "Agents chat failed");
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
    if (await handleExactSlashCommand(text)) {
      chatDraft.value = "";
      markComposerStateDirty();
      await flushComposerState();
      return;
    }

    if (hasBlockingRun.value || sendingChat.value) {
      enqueueChatMessage(text, "queued", {
        forceChat: shouldSendCurrentDraftAsGoalConversation(),
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
    const sent = await executeChatMessage(text, {
      forceChat: shouldSendCurrentDraftAsGoalConversation(),
    });
    if (!sent) {
      chatDraft.value = draftBeforeSend;
      markComposerStateDirty();
      await flushComposerState();
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

  async function cancelActiveRun(runId = activeRunControlId.value): Promise<void> {
    if (!runId) return;
    try {
      const run = normalizeADKRun(
        await fetchEnvelopeWithInit<ADKRun>(
          `/api/v1/adk/runs/${encodeURIComponent(runId)}/cancel`,
          { method: "POST" },
        ),
      );
      syncActiveRun(run, !isTerminalRunStatus(run.status));
      await reloadSessionTimeline(
        run.sessionId || sessionState.selectedSessionId.value,
      );
      if (isTerminalRunStatus(run.status)) {
        await handleTerminalRun(run);
        return;
      }
      await waitForRunContinuation(run);
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "取消运行失败";
    } finally {
      if (interruptingRunId.value === runId) {
        interruptingRunId.value = "";
      }
    }
  }

  async function pauseGoalRun(): Promise<void> {
    const runId = activeGoalRunId.value;
    if (!runId || goalLifecycleBusy.value) return;
    goalLifecycleBusy.value = true;
    try {
      const run = normalizeADKRun(
        await fetchEnvelopeWithInit<ADKRun>(
          `/api/v1/adk/runs/${encodeURIComponent(runId)}/pause`,
          { method: "POST" },
        ),
      );
      abortActiveChatStream("goal_pause");
      streamReconnectController?.abort();
      clearSessionRuntimeState(run.sessionId || sessionState.selectedSessionId.value);
      syncActiveRun(
        run,
        shouldWaitForRunContinuation(run),
      );
      await reloadSessionTimeline(
        run.sessionId || sessionState.selectedSessionId.value,
      );
      if (shouldWaitForRunContinuation(run)) {
        void waitForRunContinuation(run);
      }
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "暂停目标失败";
    } finally {
      goalLifecycleBusy.value = false;
    }
  }

  async function resumeGoalRun(): Promise<void> {
    const runId = activeGoalRunId.value;
    if (!runId || goalLifecycleBusy.value) return;
    goalLifecycleBusy.value = true;
    try {
      const run = normalizeADKRun(
        await fetchEnvelopeWithInit<ADKRun>(
          `/api/v1/adk/runs/${encodeURIComponent(runId)}/resume`,
          { method: "POST" },
        ),
      );
      syncActiveRun(run, true);
      await reloadSessionTimeline(
        run.sessionId || sessionState.selectedSessionId.value,
      );
      await waitForRunContinuation(run);
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "运行目标失败";
    } finally {
      goalLifecycleBusy.value = false;
    }
  }

  async function resolveApproval(approval: ADKApproval): Promise<void> {
    await resolveApprovalsBatch([approval], "approve");
  }

  async function denyApproval(approval: ADKApproval): Promise<void> {
    await resolveApprovalsBatch([approval], "deny");
  }

  async function resolveAllApprovals(approvals: ADKApproval[]): Promise<void> {
    await resolveApprovalsBatch(approvals, "approve");
  }

  async function denyAllApprovals(approvals: ADKApproval[]): Promise<void> {
    await resolveApprovalsBatch(approvals, "deny");
  }

  function revokeQueuedMessage(messageId: string): void {
    queuedChatMessages.value = queuedChatMessages.value.filter(
      (message) => message.id !== messageId,
    );
  }

  function handleComposerKeydown(event: KeyboardEvent): void {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      void sendChat();
    }
  }

  async function runSlashCommand(
    command: SlashCommandItem["id"],
  ): Promise<void> {
    switch (command) {
      case "context":
        await refreshSessionContext();
        contextDetailsOpen.value = true;
        return;
      case "compact":
        await compactContext("normal");
        return;
      case "compact-aggressive":
        await compactContext("aggressive");
        return;
    }
  }

  function openContextDetails(): void {
    contextDetailsOpen.value = true;
  }

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
    selectSession,
    sendChat,
    sendingChat,
    activityIndicator,
    timelineEntries,
    updateGoalObjective,
    updateGoalObjectiveDraft,
    workModeOverride,
    permissionModeOverride,
  };

  async function executeChatMessage(
    text: string,
    options: { forceChat?: boolean } = {},
  ): Promise<boolean> {
    const payload: Parameters<typeof streamADKChat>[0] = {
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
      id: `local-user-${Date.now()}`,
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
    timelineEntries.value = [...timelineEntries.value, optimisticUserEntry];
    await scrollToBottom(threadRef);
    sendingChat.value = true;
    const controller = new AbortController();
    chatStreamController = controller;
    chatStreamAbortReason = "";
    let streamAbortedForGoalPause = false;

    try {
      const response = await streamADKChat(payload, handleChatStreamEvent, {
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
    setSelectedSessionId(resolution.normalizedResponse.session.id);
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
    if (resolution.failMessage) {
      sessionState.errorMessage.value = resolution.failMessage;
    }
    if (resolution.terminal) {
      clearSessionRuntimeState(resolution.normalizedResponse.session.id);
    }
    await sessionState.refreshAll();
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

  async function submitApproval(
    approval: ADKApproval,
    action: ADKApprovalAction,
  ): Promise<ADKApprovalResolution> {
    return normalizeADKApprovalResolution(
      await fetchEnvelopeWithInit<ADKApprovalResolution>(
        `/api/v1/adk/approvals/${encodeURIComponent(approval.id)}/${action}`,
        { method: "POST" },
      ),
    );
  }

  async function resolveApprovalsBatch(
    approvals: ADKApproval[],
    action: ADKApprovalAction,
  ): Promise<void> {
    if (approvals.length === 0 || approvalsBusy.value) return;
    const resolvingIds = approvals
      .map((approval) => String(approval.id ?? "").trim())
      .filter((id) => id !== "");
    resolvingApprovalIds.value = new Set([
      ...resolvingApprovalIds.value,
      ...resolvingIds,
    ]);
    approvalsBusy.value = true;
    try {
      const { resolutions, errors } = await resolveADKApprovalBatchOnce({
        approvals,
        action,
        submit: submitApproval,
        onResolution: (resolution) => {
          timelineEntries.value = applyApprovalResolutions(
            timelineEntries.value,
            [resolution],
          );
          if (resolution.run) {
            const run = resolution.parentRun ?? resolution.run;
            void workflowQueues.syncWorkflowRun(resolution.run);
            void workflowQueues.syncWorkflowRun(resolution.parentRun);
            syncActiveRun(run, !isTerminalRunStatus(run.status));
          }
        },
      });
      if (resolutions.length > 0) {
        await finalizeApprovalBatch(resolutions);
      }
      if (errors.length > 0) {
        sessionState.errorMessage.value =
          errors.length === 1 ? errors[0]! : `批量审批部分失败：${errors[0]}`;
      }
    } finally {
      const remaining = new Set(resolvingApprovalIds.value);
      resolvingIds.forEach((id) => remaining.delete(id));
      resolvingApprovalIds.value = remaining;
      approvalsBusy.value = false;
    }
  }

  async function finalizeApprovalBatch(
    resolutions: ADKApprovalResolution[],
  ): Promise<void> {
    await sessionState.refreshAll();
    await refreshSessionContext();
    await reloadSessionTimeline(sessionState.selectedSessionId.value);
    const runs = Array.from(
      new Map(
        resolutions
          .map((resolution) => resolution.parentRun ?? resolution.run)
          .filter((run): run is ADKRun => run != null)
          .map((run) => [run.id, run]),
      ).values(),
    );
    for (const run of runs) {
      syncActiveRun(run, !isTerminalRunStatus(run.status));
      if (isTerminalRunStatus(run.status)) {
        await handleTerminalRun(run);
        continue;
      }
      // Approval controls should only be busy while the approval request and
      // authoritative refresh are in flight. A continuation may run for minutes
      // and can itself produce another approval round; monitor it in the
      // background so those new controls remain clickable.
      void waitForRunContinuation(run);
    }
  }

  async function waitForRunContinuation(
    run: ADKRun | undefined,
  ): Promise<void> {
    if (!run) {
      return;
    }
    const sessionId = run.sessionId || sessionState.selectedSessionId.value;
    if (!sessionId) {
      return;
    }
    await waitForGoalAwareRunContinuation({
      run,
      monitorRun: monitorADKRunContinuation,
      syncActiveRun,
      reloadTimeline: () => reloadSessionTimeline(sessionId),
      handleTerminalRun,
      setErrorMessage: (message) => {
        sessionState.errorMessage.value = message;
      },
    });
  }

  async function handleTerminalRun(run: ADKRun): Promise<void> {
    syncActiveRun(run);
    const failMsg = runTerminalMessage(run);
    if (failMsg) {
      sessionState.errorMessage.value = failMsg;
    }
    if (interruptingRunId.value === run.id) {
      interruptingRunId.value = "";
    }
    await dispatchQueuedMessagesIfIdle();
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

  async function refreshSessionContext(
    sessionId = sessionState.selectedSessionId.value,
    showBusy = false,
  ): Promise<void> {
    if (!sessionId) {
      sessionContext.value = null;
      return;
    }
    lastContextRefreshAt = Date.now();
    if (showBusy) contextBusy.value = true;
    try {
      const context = await fetchADKSessionContext(sessionId);
      if (sessionState.selectedSessionId.value === sessionId) {
        applySessionContext(context);
      }
    } catch {
      // Keep the last non-empty snapshot so the context tag does not blink away.
    } finally {
      if (showBusy) contextBusy.value = false;
    }
  }

  function scheduleSessionContextRefresh(
    sessionId = sessionState.selectedSessionId.value,
  ): void {
    const normalized = sessionId.trim();
    if (normalized === "") {
      return;
    }
    if (Date.now() - lastContextRefreshAt < CONTEXT_REFRESH_THROTTLE_MS) {
      return;
    }
    void refreshSessionContext(normalized);
  }

  function markComposerStateDirty(): void {
    composerDirty = true;
    composerRevision += 1;
    scheduleComposerStateSave();
  }

  function scheduleComposerStateSave(): void {
    if (composerSaveTimer !== null) {
      window.clearTimeout(composerSaveTimer);
    }
    composerSaveTimer = window.setTimeout(() => {
      composerSaveTimer = null;
      void flushComposerState();
    }, COMPOSER_STATE_SAVE_DELAY_MS);
  }

  async function flushComposerState(
    options: { keepalive?: boolean } = {},
  ): Promise<void> {
    if (composerSaveTimer !== null) {
      window.clearTimeout(composerSaveTimer);
      composerSaveTimer = null;
    }
    const sessionId = sessionState.selectedSessionId.value.trim();
    if (sessionId === "") {
      return;
    }
    if (!composerDirty) {
      return;
    }
    if (composerFlushPromise !== null) {
      await composerFlushPromise;
      if (!composerDirty) {
        return;
      }
    }
    while (composerDirty) {
      const activeSessionId = sessionState.selectedSessionId.value.trim();
      if (activeSessionId === "") return;
      const revision = composerRevision;
      const state = currentComposerState(activeSessionId);
      const savePromise = saveADKSessionComposerState(
        activeSessionId,
        {
          chatDraft: state.chatDraft,
          providerIdOverride: state.providerIdOverride,
          modelOverride: state.modelOverride,
          workModeOverride: state.workModeOverride,
          permissionModeOverride: state.permissionModeOverride,
          goalObjectiveDraft: state.goalObjectiveDraft,
          goalObjectiveTouched: state.goalObjectiveTouched,
        },
        options,
      );
      const trackedPromise = savePromise.then(
        () => undefined,
        () => undefined,
      );
      composerFlushPromise = trackedPromise;
      try {
        await savePromise;
        if (revision === composerRevision) {
          composerDirty = false;
        } else {
          composerDirty = true;
        }
      } catch {
        composerDirty = true;
        return;
      } finally {
        if (composerFlushPromise === trackedPromise) {
          composerFlushPromise = null;
        }
      }
    }
  }

  function currentComposerState(sessionId: string): ADKSessionComposerState {
    const providerOverride = currentProviderOverride();
    return normalizeSessionComposerState(sessionId, {
      sessionId,
      chatDraft: chatDraft.value,
      providerIdOverride: providerOverride.providerId,
      modelOverride: providerOverride.model,
      workModeOverride: workModeOverride.value,
      permissionModeOverride: permissionModeOverride.value,
      goalObjectiveDraft: goalObjectiveDraft.value,
      goalObjectiveTouched: goalObjectiveTouched.value,
    });
  }

  function currentProviderOverride(): { providerId: string; model: string } {
    const selectedProviderId = sessionState.selectedProviderId.value.trim();
    if (selectedProviderId === "" || selectedProviderId === defaultProviderIdForSelectedAgent()) {
      return { providerId: "", model: "" };
    }
    return {
      providerId: selectedProviderId,
      model: sessionState.selectedProvider.value?.model?.trim() ?? "",
    };
  }

  function defaultProviderIdForSelectedAgent(): string {
    return (
      sessionState.agents.value.find(
        (agent) => agent.id === sessionState.selectedAgentId.value,
      )?.providerId ?? ""
    ).trim();
  }

  function applyComposerState(state: ADKSessionComposerState): void {
    const sessionId = sessionState.selectedSessionId.value.trim();
    const normalized = normalizeSessionComposerState(sessionId, state);
    applyingComposerState = true;
    sessionState.selectedProviderId.value =
      normalized.providerIdOverride || defaultProviderIdForSelectedAgent();
    workModeOverride.value = normalized.workModeOverride;
    permissionModeOverride.value = normalized.permissionModeOverride;
    chatDraft.value = normalized.chatDraft;
    goalObjectiveTouched.value = normalized.goalObjectiveTouched;
    goalObjectiveDraft.value =
      !normalized.goalObjectiveTouched && activeGoalRun.value?.objective
        ? activeGoalRun.value.objective
        : normalized.goalObjectiveDraft;
    goalObjectiveError.value = "";
    composerDirty = false;
    if (composerSaveTimer !== null) {
      window.clearTimeout(composerSaveTimer);
      composerSaveTimer = null;
    }
    void nextTick(() => {
      applyingComposerState = false;
    });
  }

  function resetComposerState(
    sessionId = sessionState.selectedSessionId.value,
  ): void {
    applyComposerState(emptyComposerState(sessionId));
    if (sessionId.trim() === "") {
      composerDirty = false;
      return;
    }
    composerDirty = true;
    composerRevision += 1;
    scheduleComposerStateSave();
  }

  function emptyComposerState(sessionId: string): ADKSessionComposerState {
    return {
      sessionId: sessionId.trim(),
      chatDraft: "",
      providerIdOverride: "",
      modelOverride: "",
      workModeOverride: "",
      permissionModeOverride: "",
      goalObjectiveDraft: "",
      goalObjectiveTouched: false,
      updatedAt: "",
    };
  }

  async function compactContext(mode: "normal" | "aggressive"): Promise<void> {
    const sessionId = sessionState.selectedSessionId.value.trim();
    if (sessionId === "") {
      sessionState.errorMessage.value = "当前没有可压缩的会话";
      return;
    }
    contextBusy.value = true;
    try {
      applySessionContext(await compactADKSessionContext(sessionId, mode));
      contextDetailsOpen.value = true;
      await reloadSessionTimeline(sessionId);
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "上下文压缩失败";
      try {
        await reloadSessionTimeline(sessionId);
      } catch {
        // Keep the explicit error message if the timeline refresh also fails.
      }
    } finally {
      contextBusy.value = false;
    }
  }

  function updateGoalObjectiveDraft(value: string): void {
    goalObjectiveTouched.value = true;
    goalObjectiveDraft.value = value;
    goalObjectiveError.value = "";
  }

  async function updateGoalObjective(): Promise<void> {
    const run = activeGoalRun.value;
    const objective = goalObjectiveDraft.value.trim();
    if (!run || objective === "" || goalObjectiveSaving.value) {
      return;
    }
    goalObjectiveSaving.value = true;
    goalObjectiveError.value = "";
    try {
      const updated = normalizeADKRun(
        await fetchEnvelopeWithInit<ADKRun>(
          `/api/v1/adk/runs/${encodeURIComponent(run.id)}/objective`,
          {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ objective }),
          },
        ),
      );
      syncActiveRun(updated, !isTerminalRunStatus(updated.status));
      goalObjectiveDraft.value = updated.objective ?? objective;
      goalObjectiveTouched.value = false;
    } catch (error) {
      goalObjectiveError.value =
        error instanceof Error ? error.message : "目标保存失败";
      sessionState.errorMessage.value = goalObjectiveError.value;
    } finally {
      goalObjectiveSaving.value = false;
    }
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

  function setSelectedSessionId(sessionId: string): void {
    const normalized = sessionId.trim();
    if (normalized === "") return;
    const previousSessionId = sessionState.selectedSessionId.value.trim();
    sessionState.selectedSessionId.value = normalized;
    if (previousSessionId !== "") {
      return;
    }
    const previousQueueKey = buildQueueSessionKey(previousSessionId);
    const nextQueueKey = buildQueueSessionKey(normalized);
    if (previousQueueKey === nextQueueKey) {
      return;
    }
    queuedChatMessages.value = queuedChatMessages.value.map((message) =>
      message.sessionKey === previousQueueKey
        ? { ...message, sessionKey: nextQueueKey }
        : message,
    );
  }

  function enqueueChatMessage(
    text: string,
    mode: "queued" | "interrupt",
    options: { forceChat?: boolean } = {},
  ): QueuedChatMessage {
    const message = createQueuedChatMessage(
      text,
      currentQueueSessionKey.value,
      mode,
      options,
    );
    queuedChatMessages.value =
      mode === "interrupt"
        ? [message, ...queuedChatMessages.value]
        : [...queuedChatMessages.value, message];
    return message;
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
    });
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

  function clearSessionContext(): void {
    sessionContext.value = null;
    contextDetailsOpen.value = false;
  }

  async function initializeSessionContext(sessionId: string): Promise<void> {
    await refreshSessionContext(sessionId, true);
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
