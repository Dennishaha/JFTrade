import { computed, onMounted, ref, type Ref } from "vue";
import type { Router } from "vue-router";

import type {
  ADKAgent,
  ADKApproval,
  ADKProvider,
  ADKSession,
  ADKToolDescriptor,
} from "@/contracts";

import {
  createADKPageSession,
  deleteADKPageSession,
  fetchADKPageSessionData,
  renameADKPageSession,
} from "./adkPageSessionApi";
import { formatDateTime } from "./consoleDataFormatting";
import { scrollToBottom } from "./adkThreadScroll";

function defaultProvider(providers: ADKProvider[]): ADKProvider | null {
  return providers.find((item) => item.default) ?? providers[0] ?? null;
}

export function useADKPageSessionState(router: Router, threadRef: Ref<HTMLElement | null>) {
  const agents = ref<ADKAgent[]>([]);
  const providers = ref<ADKProvider[]>([]);
  const sessions = ref<ADKSession[]>([]);
  const approvals = ref<ADKApproval[]>([]);
  const tools = ref<ADKToolDescriptor[]>([]);
  const loading = ref(false);
  const initialized = ref(false);
  const errorMessage = ref("");
  const selectedAgentId = ref("");
  const selectedProviderId = ref("");
  const selectedSessionId = ref("");
  const savingProviderSelection = ref(false);
  const creatingSession = ref(false);
  const sessionSearch = ref("");
  const sessionAgentFilter = ref("");
  let sessionMutationRevision = 0;
  let sessionRefreshSequence = 0;
  let latestAppliedSessionRefresh = 0;
  const sessionMutations = new Map<
    string,
    { revision: number; kind: "upsert" | "delete" }
  >();

  const agentOptions = computed(() =>
    agents.value.map((a) => ({
      title: `${a.name} · ${formatPermission(a.permissionMode)}`,
      value: a.id,
    })),
  );
  const providerOptions = computed(() =>
    [...providers.value]
      .sort((a, b) => Number(b.default) - Number(a.default))
      .map((p, index) => {
        const isDefault = p.default && index === 0;
        return {
          title: `${p.displayName} · ${p.model}${isDefault ? " · 默认" : ""}${p.enabled ? "" : " · 已停用"}${p.hasApiKey ? "" : " · 未配置 Key"}`,
          value: isDefault ? "" : p.id,
          providerId: p.id,
          displayName: p.displayName,
          model: p.model,
          isDefault,
        };
      }),
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
  const selectedProvider = computed(() => {
    const providerID = selectedProviderId.value.trim();
    if (providerID === "") {
      return defaultProvider(providers.value);
    }
    return providers.value.find((p) => p.id === providerID) ?? null;
  });
  const approvalTool = (approval: ADKApproval) =>
    tools.value.find((tool) => tool.name === approval.toolName);

  onMounted(() => {
    void refreshAll();
  });

  async function refreshAll(): Promise<void> {
    const refreshSequence = ++sessionRefreshSequence;
    const refreshSessionRevision = sessionMutationRevision;
    loading.value = true;
    errorMessage.value = "";
    try {
      const data = await fetchADKPageSessionData();
      agents.value = data.agents;
      providers.value = data.providers;
      if (refreshSequence >= latestAppliedSessionRefresh) {
        latestAppliedSessionRefresh = refreshSequence;
        sessions.value = reconcileRefreshedSessions(
          data.sessions,
          refreshSessionRevision,
        );
      }
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
      initialized.value = true;
    }
  }

  async function createNewSession(onReset: () => void): Promise<void> {
    if (selectedAgentId.value === "" || creatingSession.value) return;
    const agentId = selectedAgentId.value;
    const previousSearch = sessionSearch.value;
    const previousAgentFilter = sessionAgentFilter.value;
    sessionSearch.value = "";
    sessionAgentFilter.value = agentId;
    creatingSession.value = true;
    errorMessage.value = "";
    try {
      const session = await createADKPageSession(agentId);
      recordSessionMutation(session.id, "upsert");
      sessions.value = [
        session,
        ...sessions.value.filter((item) => item.id !== session.id),
      ];
      selectedSessionId.value = session.id;
      onReset();
    } catch (error) {
      sessionSearch.value = previousSearch;
      sessionAgentFilter.value = previousAgentFilter;
      errorMessage.value = error instanceof Error ? error.message : "创建会话失败";
    } finally {
      creatingSession.value = false;
    }
  }

  async function deleteSession(sessionId: string, onReset: () => void): Promise<boolean> {
    try {
      await deleteADKPageSession(sessionId);
      recordSessionMutation(sessionId, "delete");
      sessions.value = sessions.value.filter((s) => s.id !== sessionId);
      if (selectedSessionId.value === sessionId) {
        selectedSessionId.value = "";
        onReset();
      }
      return true;
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "关闭会话失败";
      return false;
    }
  }

  async function renameSession(session: ADKSession): Promise<void> {
    const title = window.prompt("会话名称", session.title)?.trim();
    if (!title || title === session.title) return;
    try {
      const updated = await renameADKPageSession(session.id, title);
      recordSessionMutation(updated.id, "upsert");
      sessions.value = sessions.value.map((item) => item.id === updated.id ? updated : item);
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "重命名会话失败";
    }
  }

  async function handleProviderChange(providerId: string): Promise<void> {
    if (providerId === "" || savingProviderSelection.value) {
      return;
    }
    errorMessage.value = "";
    selectedProviderId.value = providerId;
  }

  function syncSelectedProviderFromAgent(options: { force?: boolean } = {}): void {
    const providerID = selectedAgent.value?.providerId ?? "";
    const selectionMissing = selectedProviderId.value === "";
    const selectionUnavailable =
      selectedProviderId.value !== "" &&
      !providers.value.some((provider) => provider.id === selectedProviderId.value);
    if (options.force || selectionMissing || selectionUnavailable) {
      selectedProviderId.value = providerID;
    }
  }

  function handleAgentChange(): void {
    syncSelectedProviderFromAgent({ force: true });
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

  function recordSessionMutation(
    sessionId: string,
    kind: "upsert" | "delete",
  ): void {
    sessionMutationRevision += 1;
    sessionMutations.set(sessionId, {
      revision: sessionMutationRevision,
      kind,
    });
  }

  function reconcileRefreshedSessions(
    remoteSessions: ADKSession[],
    refreshSessionRevision: number,
  ): ADKSession[] {
    if (sessionMutationRevision === refreshSessionRevision) {
      for (const [sessionId, mutation] of sessionMutations) {
        if (mutation.revision <= refreshSessionRevision) {
          sessionMutations.delete(sessionId);
        }
      }
      return remoteSessions;
    }

    const remoteByID = new Map(
      remoteSessions.map((session) => [session.id, session]),
    );
    const locallyPreferred: ADKSession[] = [];
    for (const session of sessions.value) {
      const mutation = sessionMutations.get(session.id);
      if (!mutation || mutation.revision <= refreshSessionRevision) continue;
      if (mutation.kind === "delete") {
        remoteByID.delete(session.id);
        continue;
      }
      remoteByID.set(session.id, session);
      locallyPreferred.push(session);
    }
    for (const [sessionId, mutation] of sessionMutations) {
      if (
        mutation.revision > refreshSessionRevision &&
        mutation.kind === "delete"
      ) {
        remoteByID.delete(sessionId);
      }
    }
    const merged = [
      ...locallyPreferred,
      ...remoteSessions
        .filter((session) => remoteByID.has(session.id))
        .map((session) => remoteByID.get(session.id)!),
    ];
    return merged.filter(
      (session, index) =>
        merged.findIndex((candidate) => candidate.id === session.id) === index,
    );
  }

  return {
    agents,
    approvals,
    approvalTool,
    agentName,
    agentOptions,
    createNewSession,
    creatingSession,
    deleteSession,
    errorMessage,
    finishSessionSelection,
    handleAgentChange,
    handleProviderChange,
    loading,
    initialized,
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
    case "less_approval": return "减少审批";
    case "all": return "全部允许";
    default: return "请求批准";
  }
}
