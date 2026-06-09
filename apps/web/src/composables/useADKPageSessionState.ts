import { computed, onMounted, ref, type Ref } from "vue";
import type { Router } from "vue-router";

import type {
  ADKAgent,
  ADKApproval,
  ADKProvider,
  ADKSession,
  ADKToolDescriptor,
} from "@jftrade/ui-contracts";

import {
  createADKPageSession,
  deleteADKPageSession,
  fetchADKPageSessionData,
  renameADKPageSession,
  updateADKPageAgentProvider,
} from "./adkPageSessionApi";
import { formatDateTime } from "./consoleDataFormatting";
import { scrollToBottom } from "./adkThreadScroll";

export function useADKPageSessionState(router: Router, threadRef: Ref<HTMLElement | null>) {
  const agents = ref<ADKAgent[]>([]);
  const providers = ref<ADKProvider[]>([]);
  const sessions = ref<ADKSession[]>([]);
  const approvals = ref<ADKApproval[]>([]);
  const tools = ref<ADKToolDescriptor[]>([]);
  const loading = ref(false);
  const errorMessage = ref("");
  const selectedAgentId = ref("");
  const selectedProviderId = ref("");
  const selectedSessionId = ref("");
  const savingProviderSelection = ref(false);
  const sessionSearch = ref("");
  const sessionAgentFilter = ref("");

  const agentOptions = computed(() =>
    agents.value.map((a) => ({
      title: `${a.name} · ${formatPermission(a.permissionMode)}`,
      value: a.id,
    })),
  );
  const providerOptions = computed(() =>
    providers.value.map((p) => ({
      title: `${p.displayName} · ${p.model}${p.enabled ? "" : " · 已停用"}${p.hasApiKey ? "" : " · 未配置 Key"}`,
      value: p.id,
    })),
  );
  const pendingApprovals = computed(() =>
    approvals.value.filter((a) => a.status === "PENDING"),
  );
  const visibleSessions = computed(() => {
    const query = sessionSearch.value.trim().toLowerCase();
    return sessions.value.filter((session) => {
      if (sessionAgentFilter.value && session.agentId !== sessionAgentFilter.value) return false;
      return query === "" || session.title.toLowerCase().includes(query);
    });
  });
  const selectedAgent = computed(() =>
    agents.value.find((a) => a.id === selectedAgentId.value) ?? null,
  );
  const selectedProvider = computed(() =>
    providers.value.find((p) => p.id === selectedProviderId.value) ?? null,
  );
  const approvalTool = (approval: ADKApproval) =>
    tools.value.find((tool) => tool.name === approval.toolName);

  onMounted(() => {
    void refreshAll();
  });

  async function refreshAll(): Promise<void> {
    loading.value = true;
    errorMessage.value = "";
    try {
      const data = await fetchADKPageSessionData();
      agents.value = data.agents;
      providers.value = data.providers;
      sessions.value = data.sessions;
      approvals.value = data.approvals;
      tools.value = data.tools;

      if (selectedAgentId.value === "" && agents.value.length > 0) {
        selectedAgentId.value = agents.value[0]!.id;
      }
      syncSelectedProviderFromAgent();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "加载 Agents 状态失败";
    } finally {
      loading.value = false;
    }
  }

  async function createNewSession(onReset: () => void): Promise<void> {
    if (selectedAgentId.value === "") return;
    try {
      const session = await createADKPageSession(selectedAgentId.value);
      sessions.value = [session, ...sessions.value];
      selectedSessionId.value = session.id;
      onReset();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "创建会话失败";
    }
  }

  async function deleteSession(sessionId: string, onReset: () => void): Promise<void> {
    try {
      await deleteADKPageSession(sessionId);
      sessions.value = sessions.value.filter((s) => s.id !== sessionId);
      if (selectedSessionId.value === sessionId) {
        selectedSessionId.value = "";
        onReset();
      }
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "关闭会话失败";
    }
  }

  async function renameSession(session: ADKSession): Promise<void> {
    const title = window.prompt("会话名称", session.title)?.trim();
    if (!title || title === session.title) return;
    try {
      const updated = await renameADKPageSession(session.id, title);
      sessions.value = sessions.value.map((item) => item.id === updated.id ? updated : item);
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "重命名会话失败";
    }
  }

  async function handleProviderChange(providerId: string): Promise<void> {
    const agent = selectedAgent.value;
    if (!agent || providerId === "" || providerId === agent.providerId || savingProviderSelection.value) {
      return;
    }
    savingProviderSelection.value = true;
    errorMessage.value = "";
    try {
      const updated = await updateADKPageAgentProvider(agent, providerId);
      agents.value = agents.value.map((item) => (item.id === updated.id ? updated : item));
      selectedProviderId.value = updated.providerId;
    } catch (error) {
      selectedProviderId.value = agent.providerId;
      errorMessage.value = error instanceof Error ? error.message : "切换模型提供商失败";
    } finally {
      savingProviderSelection.value = false;
    }
  }

  function syncSelectedProviderFromAgent(): void {
    selectedProviderId.value = selectedAgent.value?.providerId ?? "";
  }

  function handleAgentChange(): void {
    syncSelectedProviderFromAgent();
  }

  function openProviderSettings(): void {
    void router.push("/settings/adk");
  }

  function agentName(agentId: string): string {
    return agents.value.find((agent) => agent.id === agentId)?.name ?? agentId;
  }

  function sessionTitle(session: ADKSession): string {
    if (session.title && session.title !== "新会话" && session.title.trim() !== "") {
      return session.title;
    }
    return formatDateTime(session.createdAt);
  }

  async function finishSessionSelection(agentId: string | undefined): Promise<void> {
    if (agentId && agents.value.some((a) => a.id === agentId)) {
      selectedAgentId.value = agentId;
      syncSelectedProviderFromAgent();
    }
    await scrollToBottom(threadRef);
  }

  return {
    agents,
    approvals,
    approvalTool,
    agentName,
    agentOptions,
    createNewSession,
    deleteSession,
    errorMessage,
    finishSessionSelection,
    handleAgentChange,
    handleProviderChange,
    loading,
    openProviderSettings,
    pendingApprovals,
    providerOptions,
    providers,
    refreshAll,
    renameSession,
    savingProviderSelection,
    selectedAgent,
    selectedAgentId,
    selectedProvider,
    selectedProviderId,
    selectedSessionId,
    sessionAgentFilter,
    sessionSearch,
    sessions,
    sessionTitle,
    syncSelectedProviderFromAgent,
    tools,
    visibleSessions,
  };
}

function formatPermission(mode: string): string {
  switch (mode) {
    case "sandbox_auto": return "沙盒自动";
    case "high_auto": return "高度自动";
    default: return "审批制";
  }
}
