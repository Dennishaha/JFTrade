import { computed, reactive, ref, type Ref } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKApprovalResolution,
  ADKRun,
  ADKSessionContextSnapshot,
} from "@jftrade/ui-contracts";

import {
  createAssistantMessageState,
  isTerminalRunStatus,
  runTerminalMessage,
  syncRunPresentationState,
  type ADKAssistantMessageState,
} from "./adkChatPresentation";
import { streamADKChat } from "./adkChatStream";
import {
  applyApprovalResolutionToChat,
  applyFinalResponse,
} from "./adkPageChatResults";
import { scrollToBottom, type ChatMessage } from "./adkPageMessages";
import { loadSessionChatHistory } from "./adkPageRunHistory";
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
  const chatMessages = ref<ChatMessage[]>([]);
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
    const lastMessage = chatMessages.value.at(-1);
    if (!lastMessage || lastMessage.role !== "assistant") return true;
    if (
      lastMessage.content.trim() !== "" ||
      (lastMessage.reasoningContent ?? "").trim() !== ""
    ) {
      return false;
    }
    if (
      lastMessage.run &&
      lastMessage.run.status !== "RUNNING" &&
      lastMessage.run.status !== "PENDING"
    ) {
      return false;
    }
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
    chatMessages.value = [];
    try {
      const detail = await loadSessionChatHistory(sessionId);
      chatMessages.value = detail.chatMessages;
      await sessionState.finishSessionSelection(detail.session.agentId);
    } catch {
      // session may not have messages yet
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
    chatMessages.value.push({
      id: `local-u-${Date.now()}`,
      role: "user",
      content: text,
    });
    const assistantMessage = reactive<ChatMessage>(
      createAssistantMessageState(`local-a-${Date.now()}`),
    );
    chatMessages.value.push(assistantMessage);
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
          if (event.type === "run" && event.run) {
            activeRunId.value = event.run.id;
            syncRunPresentationState(
              assistantMessage as ADKAssistantMessageState,
              event.run,
            );
          }
          if (event.type === "delta") {
            if (event.delta) {
              assistantMessage.content += event.delta;
            }
            if (event.reasoningDelta) {
              assistantMessage.reasoningContent =
                (assistantMessage.reasoningContent ?? "") +
                event.reasoningDelta;
            }
            if (event.toolProgress) {
              if (
                assistantMessage.preToolContent === undefined &&
                assistantMessage.content.trim() !== ""
              ) {
                assistantMessage.preToolContent = assistantMessage.content;
                assistantMessage.content = "";
              }
              if (
                assistantMessage.preToolReasoning === undefined &&
                (assistantMessage.reasoningContent ?? "").trim() !== ""
              ) {
                assistantMessage.preToolReasoning =
                  assistantMessage.reasoningContent;
                assistantMessage.reasoningContent = "";
              }
              assistantMessage.toolProgress = event.toolProgress;
            }
            await scrollToBottom(threadRef);
          }
          if (event.type === "final" && event.response) {
            applyFinalResponse(assistantMessage, event.response);
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
      applyFinalResponse(assistantMessage, response);
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
      const fallbackMessage =
        error instanceof Error ? error.message : "Agents chat failed";
      if (
        assistantMessage.content.trim() === "" &&
        (assistantMessage.reasoningContent ?? "").trim() === ""
      ) {
        assistantMessage.content = fallbackMessage;
      } else {
        sessionState.errorMessage.value = fallbackMessage;
      }
      await scrollToBottom(threadRef);
    } finally {
      sendingChat.value = false;
      activeRunId.value = "";
    }
  }

  async function cancelActiveRun(): Promise<void> {
    if (!activeRunId.value) return;
    try {
      const run = await fetchEnvelopeWithInit<ADKRun>(
        `/api/v1/adk/runs/${encodeURIComponent(activeRunId.value)}/cancel`,
        { method: "POST" },
      );
      const message = chatMessages.value.find(
        (item) => item.run?.id === run.id,
      );
      if (message) message.run = run;
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "取消运行失败";
    }
  }

  async function resolveApproval(approval: ADKApproval): Promise<void> {
    approvalsBusy.value = true;
    try {
      const resolution = await submitApproval(approval, "approve");
      await finalizeApprovalBatch([resolution]);
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "审批处理失败";
      await sessionState.refreshAll();
    } finally {
      approvalsBusy.value = false;
    }
  }

  async function denyApproval(approval: ADKApproval): Promise<void> {
    approvalsBusy.value = true;
    try {
      const resolution = await submitApproval(approval, "deny");
      await finalizeApprovalBatch([resolution]);
    } catch (error) {
      sessionState.errorMessage.value =
        error instanceof Error ? error.message : "审批处理失败";
      await sessionState.refreshAll();
    } finally {
      approvalsBusy.value = false;
    }
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
    chatMessages,
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
  };

  function applyApprovalResolution(resolution: ADKApprovalResolution): void {
    chatMessages.value = applyApprovalResolutionToChat(
      chatMessages.value,
      resolution,
    );
  }

  async function submitApproval(
    approval: ADKApproval,
    action: "approve" | "deny",
  ): Promise<ADKApprovalResolution> {
    const resolution = await fetchEnvelopeWithInit<ADKApprovalResolution>(
      `/api/v1/adk/approvals/${encodeURIComponent(approval.id)}/${action}`,
      { method: "POST" },
    );
    applyApprovalResolution(resolution);
    return resolution;
  }

  async function resolveApprovalsBatch(
    approvals: ADKApproval[],
    action: "approve" | "deny",
  ): Promise<void> {
    if (approvals.length === 0) return;
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
    await scrollToBottom(threadRef);
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
    let latestRun = run;
    while (Date.now() < deadline) {
      await delay(900);
      try {
        latestRun = await fetchEnvelope<ADKRun>(
          `/api/v1/adk/runs/${encodeURIComponent(run.id)}`,
        );
        chatMessages.value = chatMessages.value.map((message) =>
          message.run?.id === latestRun.id
            ? { ...message, run: latestRun }
            : message,
        );
        if (isTerminalRunStatus(latestRun.status)) {
          await reloadSessionMessages(sessionId);
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

  async function reloadSessionMessages(sessionId: string): Promise<void> {
    if (sessionState.selectedSessionId.value !== sessionId) {
      return;
    }
    const detail = await loadSessionChatHistory(sessionId);
    chatMessages.value = detail.chatMessages;
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
