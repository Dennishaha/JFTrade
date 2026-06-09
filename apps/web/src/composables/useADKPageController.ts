import { computed, type Ref } from "vue";
import type { Router } from "vue-router";

import type { ADKApproval } from "@jftrade/ui-contracts";

import { useADKPageChatState } from "./useADKPageChatState";
import { useADKPageSessionState } from "./useADKPageSessionState";

const SUGGESTIONS = [
  "查看系统状态",
  "当前行情订阅",
  "投资组合摘要",
  "风险状态",
];

export function useADKPageController(
  router: Router,
  threadRef: Ref<HTMLElement | null>,
) {
  const sessionState = useADKPageSessionState(router, threadRef);
  const selectedAgent = sessionState.selectedAgent;
  const selectedProvider = sessionState.selectedProvider;
  const composerBlockMessage = computed(() => {
    const agent = selectedAgent.value;
    if (!agent) return "请选择可用 Agent";
    if (!agent.providerId)
      return "当前 Agent 未绑定模型 Provider，请先在 Agents 配置中选择 Provider。";
    const provider = sessionState.providers.value.find(
      (p) => p.id === agent.providerId,
    );
    if (!provider) return "当前 Agent 绑定的 Provider 不存在，请重新配置。";
    if (!provider.enabled)
      return "当前 Provider 已停用，请启用或切换 Provider。";
    if (!provider.hasApiKey)
      return "当前 Provider 未配置 API Key，请先在 Agents 配置中补充。";
    return "";
  });
  const chatState = useADKPageChatState(
    threadRef,
    {
      agents: sessionState.agents,
      errorMessage: sessionState.errorMessage,
      refreshAll: sessionState.refreshAll,
      finishSessionSelection: sessionState.finishSessionSelection,
      selectedAgentId: sessionState.selectedAgentId,
      selectedSessionId: sessionState.selectedSessionId,
    },
    composerBlockMessage,
  );

  return {
    activeRunId: chatState.activeRunId,
    agentName: sessionState.agentName,
    agentOptions: sessionState.agentOptions,
    approvalTool: sessionState.approvalTool,
    approvalsBusy: chatState.approvalsBusy,
    canSendChat: chatState.canSendChat,
    chatDraft: chatState.chatDraft,
    timelineEntries: chatState.timelineEntries,
    composerBlockMessage,
    contextBusy: chatState.contextBusy,
    contextDetailsOpen: chatState.contextDetailsOpen,
    createNewSession: () =>
      sessionState.createNewSession(() => {
        chatState.timelineEntries.value = [];
      }),
    deleteSession: (sessionId: string) =>
      sessionState.deleteSession(sessionId, () => {
        chatState.timelineEntries.value = [];
      }),
    errorMessage: sessionState.errorMessage,
    formatPermission,
    handleAgentChange: sessionState.handleAgentChange,
    handleComposerKeydown: chatState.handleComposerKeydown,
    handleProviderChange: sessionState.handleProviderChange,
    loading: sessionState.loading,
    openProviderSettings: sessionState.openProviderSettings,
    preview,
    providerOptions: sessionState.providerOptions,
    providers: sessionState.providers,
    runSlashCommand: chatState.runSlashCommand,
    renameSession: sessionState.renameSession,
    resolveApprovalGroup: (approvals: ADKApproval[], approved: boolean) =>
      approved
        ? chatState.resolveAllApprovals(approvals)
        : chatState.denyAllApprovals(approvals),
    resolveApproval: (approval: ADKApproval, approved: boolean) =>
      approved
        ? chatState.resolveApproval(approval)
        : chatState.denyApproval(approval),
    selectedAgent,
    selectedAgentId: sessionState.selectedAgentId,
    selectedProvider,
    selectedProviderId: sessionState.selectedProviderId,
    selectedSessionId: sessionState.selectedSessionId,
    sendingChat: chatState.sendingChat,
    sessionContext: chatState.sessionContext,
    sessionAgentFilter: sessionState.sessionAgentFilter,
    sessions: sessionState.sessions,
    sessionSearch: sessionState.sessionSearch,
    sessionTitle: sessionState.sessionTitle,
    showTypingIndicator: chatState.showTypingIndicator,
    SUGGESTIONS,
    savingProviderSelection: sessionState.savingProviderSelection,
    slashCommands: chatState.slashCommands,
    selectSession: chatState.selectSession,
    sendChat: chatState.sendChat,
    visibleSessions: sessionState.visibleSessions,
    cancelActiveRun: chatState.cancelActiveRun,
    openContextDetails: chatState.openContextDetails,
  };
}

function formatPermission(mode: string): string {
  switch (mode) {
    case "sandbox_auto":
      return "沙盒自动";
    case "high_auto":
      return "高度自动";
    default:
      return "审批制";
  }
}

function preview(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
}
