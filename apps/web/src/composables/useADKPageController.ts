import { computed, type Ref } from "vue";
import type { Router } from "vue-router";

import type { ADKApproval } from "@/contracts";

import { useADKPageChatState } from "./useADKPageChatState";
import { useADKPageSessionState } from "./useADKPageSessionState";

const DEFAULT_SUGGESTIONS = [
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
    const provider = selectedProvider.value;
    if (!provider) return "请先在 Agents 配置中新增模型 Provider。";
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
      initialized: sessionState.initialized,
      refreshAll: sessionState.refreshAll,
      finishSessionSelection: sessionState.finishSessionSelection,
      selectedProvider,
      selectedAgentId: sessionState.selectedAgentId,
      selectedProviderId: sessionState.selectedProviderId,
      selectedSessionId: sessionState.selectedSessionId,
      sessions: sessionState.sessions,
    },
    composerBlockMessage,
  );
  const suggestions = computed(() => DEFAULT_SUGGESTIONS);
  const composerPlaceholder = computed(() =>
    chatState.activeChildRunId.value
      ? "子智能体视图仅支持观察和审批"
      : "输入问题或任务...",
  );
  const emptyStateHint = computed(
    () => "可直接输入问题，也可以用 @tool_name 显式调用内置工具",
  );
  const effectiveComposerBlockMessage = computed(() => {
    if (chatState.activeChildRunId.value) {
      return "子智能体视图仅支持观察和审批，请返回父对话后继续发送消息。";
    }
    return composerBlockMessage.value;
  });
  const effectiveCanSendChat = computed(
    () => chatState.canSendChat.value && !chatState.activeChildRunId.value,
  );
  const effectiveCanInterruptChat = computed(
    () => chatState.canInterruptChat.value && !chatState.activeChildRunId.value,
  );

  return {
    activeRunId: chatState.activeRunId,
    activeRunStatus: chatState.activeRunStatus,
    agentName: sessionState.agentName,
    agentOptions: sessionState.agentOptions,
    approvalTool: sessionState.approvalTool,
    approvalsBusy: chatState.approvalsBusy,
    activeChildRunId: chatState.activeChildRunId,
    canInterruptChat: effectiveCanInterruptChat,
    canSendChat: effectiveCanSendChat,
    childRunItems: chatState.childRunItems,
    childTimelineEntries: chatState.childTimelineEntries,
    childViewContext: chatState.childViewContext,
    chatDraft: chatState.chatDraft,
    composerBlockMessage: effectiveComposerBlockMessage,
    cancelActiveRun: chatState.cancelActiveRun,
    contextBusy: chatState.contextBusy,
    contextDetailsOpen: chatState.contextDetailsOpen,
    creatingSession: sessionState.creatingSession,
    createNewSession: async () => {
      await chatState.flushComposerState();
      await sessionState.createNewSession(() => {
        chatState.timelineEntries.value = [];
        chatState.clearSessionContext();
        chatState.clearWorkflowPlanRun();
        chatState.resetComposerState();
      });
      if (sessionState.selectedSessionId.value) {
        await chatState.initializeSessionContext(sessionState.selectedSessionId.value);
      }
    },
    deleteSession: async (sessionId: string) => {
      await chatState.flushComposerState();
      const deleted = await sessionState.deleteSession(sessionId, () => {
        chatState.timelineEntries.value = [];
        chatState.clearSessionContext();
        chatState.clearWorkflowPlanRun();
        chatState.resetComposerState();
      });
      if (deleted) {
        chatState.removeSessionRuntimeState(sessionId);
      }
    },
    errorMessage: sessionState.errorMessage,
    formatPermission,
    goalObjectiveDraft: chatState.goalObjectiveDraft,
    goalObjectiveError: chatState.goalObjectiveError,
    goalObjectiveSaving: chatState.goalObjectiveSaving,
    goalLifecycleBusy: chatState.goalLifecycleBusy,
    goalPaused: chatState.goalPaused,
    goalTimedOut: chatState.goalTimedOut,
    goalPauseRequested: chatState.goalPauseRequested,
    showGoalObjectiveEditor: chatState.showGoalObjectiveEditor,
    canSaveGoalObjective: chatState.canSaveGoalObjective,
    canPauseGoal: chatState.canPauseGoal,
    canResumeGoal: chatState.canResumeGoal,
    hasBlockingRun: chatState.hasBlockingRun,
    handleAgentChange: sessionState.handleAgentChange,
    handleComposerKeydown: chatState.handleComposerKeydown,
    handleProviderChange: sessionState.handleProviderChange,
    interruptAndQueueChat: chatState.interruptAndQueueChat,
    interruptingRunId: chatState.interruptingRunId,
    loading: sessionState.loading,
    openProviderSettings: sessionState.openProviderSettings,
    pauseGoalRun: chatState.pauseGoalRun,
    parentApprovalQueue: chatState.parentApprovalQueue,
    preview,
    providerOptions: sessionState.providerOptions,
    providers: sessionState.providers,
    queueDispatchingId: chatState.queueDispatchingId,
    queuedMessages: chatState.queuedMessages,
    revokeQueuedMessage: chatState.revokeQueuedMessage,
    resumeGoalRun: chatState.resumeGoalRun,
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
    selectedApprovalQueue: chatState.selectedApprovalQueue,
    selectedAgentId: sessionState.selectedAgentId,
    selectedProvider,
    selectedProviderId: sessionState.selectedProviderId,
    selectedSessionId: sessionState.selectedSessionId,
    sendingChat: chatState.sendingChat,
    sessionContext: chatState.visibleSessionContext,
    sessionAgentFilter: sessionState.sessionAgentFilter,
    sessions: sessionState.sessions,
    sessionSearch: sessionState.sessionSearch,
    sessionTitle: sessionState.sessionTitle,
    activityIndicator: chatState.activityIndicator,
    suggestions,
    composerPlaceholder,
    emptyStateHint,
    savingProviderSelection: sessionState.savingProviderSelection,
    slashCommands: chatState.slashCommands,
    selectSession: chatState.selectSession,
    sendChat: chatState.sendChat,
    setActiveChildRunId: chatState.setActiveChildRunId,
    timelineEntries: chatState.timelineEntries,
    updateGoalObjective: chatState.updateGoalObjective,
    updateGoalObjectiveDraft: chatState.updateGoalObjectiveDraft,
    visibleTimelineEntries: chatState.visibleTimelineEntries,
    visibleWorkflowPlanRun: chatState.visibleWorkflowPlanRun,
    visibleSessions: sessionState.visibleSessions,
    workModeOverride: chatState.workModeOverride,
    permissionModeOverride: chatState.permissionModeOverride,
    openContextDetails: chatState.openContextDetails,
  };
}

function formatPermission(mode: string): string {
  switch (mode) {
    case "less_approval":
      return "减少审批";
    case "all":
      return "全部允许";
    default:
      return "请求批准";
  }
}

function preview(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
}
