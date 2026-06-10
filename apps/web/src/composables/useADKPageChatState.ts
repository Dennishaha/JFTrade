import { computed, ref, type Ref } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKRun,
  ADKSessionContextSnapshot,
} from "@/contracts";

import { isTerminalRunStatus, runTerminalMessage } from "./adkChatPresentation";
import { streamADKChat } from "./adkChatStream";
import {
  buildActiveChatRunState,
  buildQueueSessionKey,
  createQueuedChatMessage,
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
  const sendingChat = ref(false);
  const activeRun = ref<ActiveChatRunState | null>(null);
  const queuedChatMessages = ref<QueuedChatMessage[]>([]);
  const queueDispatchingId = ref("");
  const interruptingRunId = ref("");
  const approvalsBusy = ref(false);
  const contextBusy = ref(false);
  const contextDetailsOpen = ref(false);
  const sessionContext = ref<ADKSessionContextSnapshot | null>(null);

  const activeRunId = computed(() => activeRun.value?.runId ?? "");
  const activeRunStatus = computed(() => activeRun.value?.status ?? "");
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
    if (!sendingChat.value) return false;
    const lastEntry = timelineEntries.value.at(-1);
    if (!lastEntry) return true;
    if (lastEntry.kind === "tool_group") return false;
    if ((lastEntry.text ?? "").trim() !== "") return false;
    return true;
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
      const run = await fetchEnvelopeWithInit<ADKRun>(
        `/api/v1/adk/runs/${encodeURIComponent(runId)}/cancel`,
        { method: "POST" },
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
    hasBlockingRun,
    interruptingRunId,
    queuedMessages,
    queueDispatchingId,
    revokeQueuedMessage,
    sessionContext,
    slashCommands,
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
  };

  async function executeChatMessage(text: string): Promise<boolean> {
    const optimisticUserEntry = createTimelineEntryState({
      id: `local-user-${Date.now()}`,
      sessionId: sessionState.selectedSessionId.value,
      kind: "user_message",
      createdAt: new Date().toISOString(),
      sequence: timelineEntries.value.length + 1,
      status: "streaming",
      text,
    });
    timelineEntries.value = [...timelineEntries.value, optimisticUserEntry];
    await scrollToBottom(threadRef);
    sendingChat.value = true;

    try {
      const response = await streamADKChat(
        {
          agentId: sessionState.selectedAgentId.value,
          sessionId: sessionState.selectedSessionId.value,
          message: text,
        },
        async (event) => {
          if (event.type === "session" && event.session?.id) {
            setSelectedSessionId(event.session.id);
          }
          if (event.type === "context" && event.context) {
            sessionContext.value = event.context;
          }
          if (event.type === "run" && event.run?.id) {
            syncActiveRun(event.run);
          }
          if (event.type === "timeline" && event.timeline) {
            timelineEntries.value = upsertTimelineEntry(
              timelineEntries.value,
              event.timeline,
            );
            await scrollToBottom(threadRef);
          }
          if (event.type === "final" && event.response) {
            await applyAuthoritativeTimeline(event.response);
            syncActiveRun(
              event.response.run,
              !isTerminalRunStatus(event.response.run.status),
            );
            if (event.response.context) {
              sessionContext.value = event.response.context;
            }
            const failMsg = runTerminalMessage(event.response.run);
            if (failMsg) {
              sessionState.errorMessage.value = failMsg;
            }
          }
          if (event.type === "error") {
            throw new Error(event.message || "Agents chat failed");
          }
        },
      );

      setSelectedSessionId(response.session.id);
      await applyAuthoritativeTimeline(response);
      syncActiveRun(response.run, !isTerminalRunStatus(response.run.status));
      if (response.context) {
        sessionContext.value = response.context;
      } else {
        await refreshSessionContext(response.session.id);
      }
      const failMsg = runTerminalMessage(response.run);
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
    timelineEntries.value = replaceTimelineEntries(
      response.timeline,
      timelineEntries.value,
    );
    await scrollToBottom(threadRef);
  }

  async function submitApproval(
    approval: ADKApproval,
    action: "approve" | "deny",
  ): Promise<ADKApprovalResolution> {
    return fetchEnvelopeWithInit<ADKApprovalResolution>(
      `/api/v1/adk/approvals/${encodeURIComponent(approval.id)}/${action}`,
      { method: "POST" },
    );
  }

  async function resolveApprovalsBatch(
    approvals: ADKApproval[],
    action: "approve" | "deny",
  ): Promise<void> {
    if (approvals.length === 0 || approvalsBusy.value) return;
    approvalsBusy.value = true;
    const resolutions: ADKApprovalResolution[] = [];
    const errors: string[] = [];
    try {
      for (const approval of approvals) {
        try {
          const resolution = await submitApproval(approval, action);
          resolutions.push(resolution);
          timelineEntries.value = applyApprovalResolutions(
            timelineEntries.value,
            [resolution],
          );
          if (resolution.run) {
            syncActiveRun(
              resolution.run,
              !isTerminalRunStatus(resolution.run.status),
            );
          }
        } catch (error) {
          errors.push(error instanceof Error ? error.message : "审批处理失败");
        }
      }
      await finalizeApprovalBatch(resolutions);
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
          .map((resolution) => resolution.run)
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
    if (isTerminalRunStatus(run.status)) {
      if (!activeRun.value || activeRun.value.runId === run.id) {
        activeRun.value = null;
      }
      return;
    }
    activeRun.value = buildActiveChatRunState(run, waitingForContinuation);
  }
}
