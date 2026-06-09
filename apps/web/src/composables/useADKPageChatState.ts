import { computed, ref, type Ref } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKRun,
  ADKSessionContextSnapshot,
} from "@jftrade/ui-contracts";

import {
  isTerminalRunStatus,
  runTerminalMessage,
} from "./adkChatPresentation";
import { streamADKChat } from "./adkChatStream";
import { scrollToBottom } from "./adkThreadScroll";
import { loadSessionChatHistory } from "./adkPageRunHistory";
import {
  createTimelineEntryState,
  replaceTimelineEntries,
  upsertTimelineEntry,
  type ADKTimelineEntryState,
} from "./adkTimeline";
import {
  compactADKSessionContext,
  fetchADKSessionContext,
} from "./adkSessionContextApi";
import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";

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
  const activeRunId = ref("");
  const approvalsBusy = ref(false);
  const contextBusy = ref(false);
  const contextDetailsOpen = ref(false);
  const sessionContext = ref<ADKSessionContextSnapshot | null>(null);

  const canSendChat = computed(
    () =>
      chatDraft.value.trim() !== "" &&
      sessionState.selectedAgentId.value !== "" &&
      !sendingChat.value &&
      composerBlockMessage.value === "",
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
    sessionState.selectedSessionId.value = sessionId;
    timelineEntries.value = [];
    try {
      const detail = await loadSessionChatHistory(sessionId);
      timelineEntries.value = detail.timelineEntries;
      await sessionState.finishSessionSelection(detail.session.agentId);
    } catch {
      // session may not have timeline entries yet
    }
    await refreshSessionContext(sessionId);
  }

  async function sendChat(): Promise<void> {
    const text = chatDraft.value.trim();
    if (
      text === "" ||
      sessionState.selectedAgentId.value === "" ||
      sendingChat.value ||
      composerBlockMessage.value !== ""
    ) {
      return;
    }
    if (await handleExactSlashCommand(text)) {
      chatDraft.value = "";
      return;
    }

    chatDraft.value = "";
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
            sessionState.selectedSessionId.value = event.session.id;
          }
          if (event.type === "context" && event.context) {
            sessionContext.value = event.context;
          }
          if (event.type === "run" && event.run?.id) {
            activeRunId.value = event.run.id;
          }
          if (event.type === "timeline" && event.timeline) {
            timelineEntries.value = upsertTimelineEntry(
              timelineEntries.value,
              event.timeline,
            );
            await scrollToBottom(threadRef);
          }
          if (event.type === "final" && event.response) {
            await applyAuthoritativeTimeline(event.response, event.response.session.id);
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

      sessionState.selectedSessionId.value = response.session.id;
      await applyAuthoritativeTimeline(response, response.session.id);
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
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "Agents chat failed";
      await scrollToBottom(threadRef);
    } finally {
      sendingChat.value = false;
      activeRunId.value = "";
    }
  }

  async function cancelActiveRun(): Promise<void> {
    if (!activeRunId.value) return;
    try {
      await fetchEnvelopeWithInit<ADKRun>(
        `/api/v1/adk/runs/${encodeURIComponent(activeRunId.value)}/cancel`,
        { method: "POST" },
      );
      await reloadSessionTimeline(sessionState.selectedSessionId.value);
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "取消运行失败";
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
    approvalsBusy,
    canSendChat,
    chatDraft,
    contextBusy,
    contextDetailsOpen,
    sessionContext,
    slashCommands,
    handleComposerKeydown,
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

  async function applyAuthoritativeTimeline(
    response: ADKChatResponse,
    sessionId: string,
  ): Promise<void> {
    if (response.timeline && response.timeline.length > 0) {
      timelineEntries.value = replaceTimelineEntries(response.timeline, timelineEntries.value);
    } else if (sessionId) {
      await reloadSessionTimeline(sessionId);
      return;
    }
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
          resolutions.push(await submitApproval(approval, action));
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
    const runs = Array.from(
      new Map(
        resolutions
          .map((resolution) => resolution.run)
          .filter((run): run is ADKRun => run != null)
          .map((run) => [run.id, run]),
      ).values(),
    );
    for (const run of runs) {
      if (isTerminalRunStatus(run.status)) {
        await reloadSessionTimeline(run.sessionId || sessionState.selectedSessionId.value);
        const failMsg = runTerminalMessage(run);
        if (failMsg) {
          sessionState.errorMessage.value = failMsg;
        }
        continue;
      }
      await waitForApprovalContinuation(run);
    }
  }

  async function waitForApprovalContinuation(
    run: ADKRun | undefined,
  ): Promise<void> {
    if (!run || isTerminalRunStatus(run.status)) {
      return;
    }
    const sessionId = run.sessionId || sessionState.selectedSessionId.value;
    if (!sessionId) {
      return;
    }
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      await delay(900);
      try {
        const latestRun = await fetchEnvelope<ADKRun>(
          `/api/v1/adk/runs/${encodeURIComponent(run.id)}`,
        );
        if (isTerminalRunStatus(latestRun.status)) {
          await reloadSessionTimeline(sessionId);
          const failMsg = runTerminalMessage(latestRun);
          if (failMsg) {
            sessionState.errorMessage.value = failMsg;
          }
          return;
        }
      } catch {
        return;
      }
    }
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
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
