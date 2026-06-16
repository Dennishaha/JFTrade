import { computed, ref, watch, type Ref } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKRun,
  ADKSessionContextSnapshot,
} from "@/contracts";

import {
  resolveADKApprovalBatchOnce,
  type ADKApprovalAction,
} from "./adkApprovalResolution";
import { isTerminalRunStatus, runTerminalMessage } from "./adkChatPresentation";
import { streamADKChat } from "./adkChatStream";
import {
  normalizeADKApprovalResolution,
  normalizeADKChatResponse,
  normalizeADKRun,
  normalizeADKTimelineEntry,
} from "./adkNormalization";
import {
  buildActiveChatRunState,
  buildQueueSessionKey,
  createQueuedChatMessage,
  hasPendingRunApproval,
  isBlockingRunStatus,
  type ActiveChatRunState,
  type QueuedChatMessage,
} from "./adkChatRuntime";
import { monitorADKRunContinuation } from "./adkRunContinuation";
import { scrollToBottom } from "./adkThreadScroll";
import { loadSessionChatHistory } from "./adkPageRunHistory";
import {
  applyApprovalResolutions,
  createTimelineEntryState,
  replaceTimelineEntries,
  upsertTimelineEntry,
  type ADKTimelineEntryState,
} from "./adkTimeline";
import {
  compactADKSessionContext,
  fetchADKSessionContext,
} from "./adkSessionContextApi";
import { fetchEnvelopeWithInit } from "./apiClient";
import {
  sessionContextFromRunUsage,
  useADKWorkflowQueueState,
} from "./useADKWorkflowQueueState";

interface SessionState {
  agents: Ref<ADKAgent[]>;
  errorMessage: Ref<string>;
  refreshAll: () => Promise<void>;
  finishSessionSelection: (agentId: string | undefined) => Promise<void>;
  selectedAgentId: Ref<string>;
  selectedSessionId: Ref<string>;
}

export interface SlashCommandItem {
  id: "context" | "compact" | "compact-aggressive";
  command: "/context" | "/compact" | "/compact-aggressive";
  title: string;
  description: string;
  disabled?: boolean;
}

export function useADKPageChatState(
  threadRef: Ref<HTMLElement | null>,
  sessionState: SessionState,
  composerBlockMessage: Ref<string>,
) {
  const timelineEntries = ref<ADKTimelineEntryState[]>([]);
  const chatDraft = ref("");
  const workModeOverride = ref("");
  const sendingChat = ref(false);
  const activeRun = ref<ActiveChatRunState | null>(null);
  const activeRunSnapshot = ref<ADKRun | null>(null);
  const queuedChatMessages = ref<QueuedChatMessage[]>([]);
  const queueDispatchingId = ref("");
  const interruptingRunId = ref("");
  const approvalsBusy = ref(false);
  const contextBusy = ref(false);
  const contextDetailsOpen = ref(false);
  const sessionContext = ref<ADKSessionContextSnapshot | null>(null);
  const goalObjectiveDraft = ref("");
  const goalObjectiveTouched = ref(false);
  const goalObjectiveSaving = ref(false);
  const goalObjectiveError = ref("");
  const workflowQueues = useADKWorkflowQueueState({
    timelineEntries,
    selectedSessionId: sessionState.selectedSessionId,
  });

  const activeRunId = computed(() => activeRun.value?.runId ?? "");
  const activeRunStatus = computed(() => activeRun.value?.status ?? "");
  const activeGoalRun = computed(() => {
    const run = activeRunSnapshot.value;
    if (!run || isTerminalRunStatus(run.status)) return null;
    return String(run.workMode ?? "").trim().toLowerCase() === "loop" &&
      !run.parentRunId
      ? run
      : null;
  });
  const showGoalObjectiveEditor = computed(
    () =>
      activeGoalRun.value != null ||
      (workModeOverride.value === "loop" &&
        goalObjectiveDraft.value.trim() !== ""),
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
  const hasBlockingRun = computed(() =>
    isBlockingRunStatus(activeRun.value?.status),
  );
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
  const showTypingIndicator = computed(() => {
    return sendingChat.value || hasBlockingRun.value;
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
  const visibleSessionContext = computed(() =>
    workflowQueues.activeChildRunId.value
      ? sessionContextFromRunUsage(
          workflowQueues.childRunSnapshots.value[
            workflowQueues.activeChildRunId.value
          ],
          sessionContext.value,
        )
      : sessionContext.value,
  );

  watch([workModeOverride, chatDraft], () => {
    if (activeGoalRun.value) return;
    if (workModeOverride.value !== "loop") {
      goalObjectiveTouched.value = false;
      goalObjectiveDraft.value = "";
      goalObjectiveError.value = "";
      return;
    }
    if (!goalObjectiveTouched.value) {
      goalObjectiveDraft.value = chatDraft.value;
    }
  });

  async function selectSession(sessionId: string): Promise<void> {
    if (sessionState.selectedSessionId.value === sessionId) return;
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
    try {
      const detail = await loadSessionChatHistory(sessionId);
      timelineEntries.value = detail.timelineEntries;
      await sessionState.finishSessionSelection(detail.session.agentId);
    } catch {
      // Session may not have timeline entries yet.
    }
    await refreshSessionContext(sessionId);
    await dispatchQueuedMessagesIfIdle();
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
      return;
    }

    if (hasBlockingRun.value || sendingChat.value) {
      enqueueChatMessage(text, "queued");
      chatDraft.value = "";
      await scrollToBottom(threadRef);
      return;
    }

    chatDraft.value = "";
    await executeChatMessage(text);
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

    const currentRunId = activeRunId.value;
    enqueueChatMessage(text, "interrupt");
    chatDraft.value = "";
    await scrollToBottom(threadRef);
    if (!currentRunId || interruptingRunId.value === currentRunId) {
      return;
    }
    interruptingRunId.value = currentRunId;
    await cancelActiveRun(currentRunId);
  }

  async function cancelActiveRun(runId = activeRunId.value): Promise<void> {
    if (!runId) return;
    try {
      const run = normalizeADKRun(await fetchEnvelopeWithInit<ADKRun>(
        `/api/v1/adk/runs/${encodeURIComponent(runId)}/cancel`,
        { method: "POST" },
      ));
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
    showGoalObjectiveEditor,
    canSaveGoalObjective,
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
    clearWorkflowPlanRun,
    handleComposerKeydown,
    interruptAndQueueChat,
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
    showTypingIndicator,
    timelineEntries,
    updateGoalObjective,
    updateGoalObjectiveDraft,
    workModeOverride,
  };

  async function executeChatMessage(text: string): Promise<boolean> {
    const payload: Parameters<typeof streamADKChat>[0] = {
      agentId: sessionState.selectedAgentId.value,
      sessionId: sessionState.selectedSessionId.value,
      message: text,
    };
    if (workModeOverride.value) {
      payload.workModeOverride = workModeOverride.value;
      if (workModeOverride.value === "loop") {
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
    clearWorkflowPlanRun();
    timelineEntries.value = [...timelineEntries.value, optimisticUserEntry];
    await scrollToBottom(threadRef);
    sendingChat.value = true;

    try {
      const response = await streamADKChat(
        payload,
        async (event) => {
          if (event.type === "session" && event.session?.id) {
            setSelectedSessionId(event.session.id);
          }
          if (event.type === "context" && event.context) {
            sessionContext.value = event.context;
          }
          if (event.type === "run" && event.run?.id) {
            syncActiveRun(normalizeADKRun(event.run));
          }
          if (event.type === "timeline" && event.timeline) {
            timelineEntries.value = upsertTimelineEntry(
              timelineEntries.value,
              normalizeADKTimelineEntry(event.timeline),
            );
            await scrollToBottom(threadRef);
          }
          if (event.type === "final" && event.response) {
            const response = normalizeADKChatResponse(event.response);
            await applyAuthoritativeTimeline(response);
            syncActiveRun(
              response.run,
              !isTerminalRunStatus(response.run.status),
            );
            if (response.context) {
              sessionContext.value = response.context;
            }
            const failMsg = runTerminalMessage(response.run);
            if (failMsg) {
              sessionState.errorMessage.value = failMsg;
            }
          }
          if (event.type === "error") {
            throw new Error(event.message || "Agents chat failed");
          }
        },
      );

      const normalizedResponse = normalizeADKChatResponse(response);
      setSelectedSessionId(normalizedResponse.session.id);
      await applyAuthoritativeTimeline(normalizedResponse);
      syncActiveRun(
        normalizedResponse.run,
        !isTerminalRunStatus(normalizedResponse.run.status),
      );
      if (normalizedResponse.context) {
        sessionContext.value = normalizedResponse.context;
      } else {
        await refreshSessionContext(normalizedResponse.session.id);
      }
      const failMsg = runTerminalMessage(normalizedResponse.run);
      if (failMsg) {
        sessionState.errorMessage.value = failMsg;
      }
      await sessionState.refreshAll();
      await scrollToBottom(threadRef);
      return true;
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "Agents chat failed";
      await scrollToBottom(threadRef);
      return false;
    } finally {
      sendingChat.value = false;
      await dispatchQueuedMessagesIfIdle();
    }
  }

  async function applyAuthoritativeTimeline(
    response: ADKChatResponse,
  ): Promise<void> {
    const normalizedResponse = normalizeADKChatResponse(response);
    timelineEntries.value = replaceTimelineEntries(
      normalizedResponse.timeline,
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
            syncActiveRun(
              run,
              !isTerminalRunStatus(run.status),
            );
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
      await waitForRunContinuation(run);
    }
  }

  async function waitForRunContinuation(
    run: ADKRun | undefined,
  ): Promise<void> {
    if (!run || isTerminalRunStatus(run.status)) {
      return;
    }
    const sessionId = run.sessionId || sessionState.selectedSessionId.value;
    if (!sessionId) {
      return;
    }
    try {
      const latestRun = await monitorADKRunContinuation(run, {
        onProgress: async (nextRun) => {
          syncActiveRun(nextRun, true);
          const failMsg = runTerminalMessage(nextRun);
          if (failMsg) {
            sessionState.errorMessage.value = failMsg;
          }
          await reloadSessionTimeline(sessionId);
        },
        onTerminal: async (terminalRun) => {
          await reloadSessionTimeline(sessionId);
          await handleTerminalRun(terminalRun);
        },
      });
      if (latestRun && !isTerminalRunStatus(latestRun.status)) {
        syncActiveRun(latestRun, true);
        const failMsg = runTerminalMessage(latestRun);
        if (failMsg) {
          sessionState.errorMessage.value = failMsg;
        }
        if (hasPendingRunApproval(latestRun)) {
          await reloadSessionTimeline(sessionId);
        }
      }
    } catch {
      // Ignore polling failures and keep the latest visible state.
    }
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
  ): Promise<void> {
    if (!sessionId) {
      sessionContext.value = null;
      return;
    }
    try {
      sessionContext.value = await fetchADKSessionContext(sessionId);
    } catch {
      sessionContext.value = null;
    }
  }

  async function compactContext(mode: "normal" | "aggressive"): Promise<void> {
    const sessionId = sessionState.selectedSessionId.value.trim();
    if (sessionId === "") {
      sessionState.errorMessage.value = "当前没有可压缩的会话";
      return;
    }
    contextBusy.value = true;
    try {
      sessionContext.value = await compactADKSessionContext(sessionId, mode);
      contextDetailsOpen.value = true;
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "上下文压缩失败";
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
  ): QueuedChatMessage {
    const message = createQueuedChatMessage(
      text,
      currentQueueSessionKey.value,
      mode,
    );
    queuedChatMessages.value =
      mode === "interrupt"
        ? [message, ...queuedChatMessages.value]
        : [...queuedChatMessages.value, message];
    return message;
  }

  async function dispatchQueuedMessagesIfIdle(): Promise<void> {
    if (
      sendingChat.value ||
      hasBlockingRun.value ||
      queueDispatchingId.value !== "" ||
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
    const sent = await executeChatMessage(nextMessage.text);
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
    run: ADKRun | undefined,
    waitingForContinuation = false,
  ): void {
    if (!run) {
      return;
    }
    void workflowQueues.syncWorkflowRun(run);
    if (isTerminalRunStatus(run.status)) {
      if (!activeRun.value || activeRun.value.runId === run.id) {
        activeRun.value = null;
        activeRunSnapshot.value = null;
        if (workModeOverride.value === "loop") {
          goalObjectiveTouched.value = false;
          goalObjectiveDraft.value = chatDraft.value;
        }
      }
      return;
    }
    activeRunSnapshot.value = run;
    if (String(run.workMode ?? "").trim().toLowerCase() === "loop" && !goalObjectiveSaving.value) {
      goalObjectiveDraft.value = run.objective ?? goalObjectiveDraft.value;
      goalObjectiveTouched.value = false;
      goalObjectiveError.value = "";
    }
    activeRun.value = buildActiveChatRunState(run, waitingForContinuation);
  }

  function clearWorkflowPlanRun(): void {
    workflowQueues.clearWorkflowQueues();
  }

  function clearSessionContext(): void {
    sessionContext.value = null;
    contextDetailsOpen.value = false;
  }
}
