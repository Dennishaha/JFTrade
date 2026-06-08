import { computed, onMounted, ref, watch } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKAuditEvent,
  ADKMemoryEntry,
  ADKOptimizationTask,
  ADKProvider,
  ADKRun,
  ADKSkill,
  ADKTask,
  ADKToolDescriptor,
} from "@jftrade/ui-contracts";

import {
  cancelADKOptimizationTask,
  cancelADKRun,
  deleteADKAgent,
  deleteADKProvider,
  fallbackPage,
  fetchADKApprovalsPage,
  fetchADKAuditPage,
  fetchADKMetrics,
  fetchADKMemory,
  fetchADKOptimizationTasks,
  fetchADKRunsPage,
  fetchADKSettingsSnapshot,
  fetchADKSkills,
  fetchADKTasks,
  installADKSkill,
  nextPage,
  pageSummary,
  previousPage,
  saveADKAgent,
  saveADKProvider,
  saveADKRuntimeSettings,
  testADKProvider,
  type ADKMetricsResponse,
  type PageEnvelope,
  uninstallADKSkill,
} from "./adkSettingsApi";
import { runTerminalMessage } from "./adkChatPresentation";
import {
  formatPermission,
  isInternalSkill,
  permissionModes,
  preview,
  riskColor,
  riskLabel,
  toolCallStatusColor,
} from "./adkSettingsPresentation";
import { formatDateTime, formatGenericStatusLabel } from "./consoleDataFormatting";
import { useADKAgentForm } from "./useADKAgentForm";
import { useADKProviderForm } from "./useADKProviderForm";

export function useADKSettingsSectionState() {
  const providers = ref<ADKProvider[]>([]);
  const agents = ref<ADKAgent[]>([]);
  const tools = ref<ADKToolDescriptor[]>([]);
  const skills = ref<ADKSkill[]>([]);
  const runs = ref<ADKRun[]>([]);
  const approvals = ref<ADKApproval[]>([]);
  const optimizationTasks = ref<ADKOptimizationTask[]>([]);
  const tasks = ref<ADKTask[]>([]);
  const memoryEntries = ref<ADKMemoryEntry[]>([]);
  const agentTemplates = ref<Array<Omit<ADKAgent, "createdAt" | "updatedAt">>>([]);
  const auditEvents = ref<ADKAuditEvent[]>([]);
  const metrics = ref<ADKMetricsResponse | null>(null);
  const loading = ref(false);
  const errorMessage = ref("");
  const successMessage = ref("");
  const activeTab = ref("providers");
  const runStatusFilter = ref("attention");
  const approvalStatusFilter = ref("PENDING");
  const auditKindFilter = ref("");
  const taskStatusFilter = ref("");
  const taskAgentFilter = ref("");
  const memoryScopeFilter = ref("");
  const memoryAgentFilter = ref("");
  const memoryKeyFilter = ref("");
  const toolCategoryFilter = ref("");
  const toolRiskFilter = ref("");
  const toolSearchQuery = ref("");
  const toolDetailDialogOpen = ref(false);
  const selectedToolName = ref("");
  const agentTemplateNotice = ref("");
  const runPage = ref<PageEnvelope>({ limit: 20, offset: 0, total: 0, returned: 0, hasMore: false });
  const approvalPage = ref<PageEnvelope>({ limit: 10, offset: 0, total: 0, returned: 0, hasMore: false });
  const auditPage = ref<PageEnvelope>({ limit: 12, offset: 0, total: 0, returned: 0, hasMore: false });
  const skillUrl = ref("");
  const runtimeSettingsForm = ref({
    runTimeoutSeconds: 600,
    streamIdleTimeoutSeconds: 300,
  });

  const providerOptions = computed(() =>
    providers.value.map((p) => ({
      title: `${p.displayName} · ${p.model}${p.hasApiKey ? "" : " · 未配置密钥"}`,
      value: p.id,
    })),
  );
  const filteredToolsByFacet = computed(() =>
    tools.value.filter((tool) => {
      if (toolCategoryFilter.value && tool.category !== toolCategoryFilter.value) return false;
      if (toolRiskFilter.value && tool.riskLevel !== toolRiskFilter.value) return false;
      return true;
    }),
  );
  const filteredTools = computed(() =>
    filteredToolsByFacet.value.filter((tool) => {
      const query = toolSearchQuery.value.trim().toLowerCase();
      if (query !== "") {
        const haystack = [
          tool.name,
          tool.displayName,
          tool.description,
          tool.category,
          tool.permission,
          tool.outputSummary ?? "",
        ]
          .join(" ")
          .toLowerCase();
        if (!haystack.includes(query)) return false;
      }
      return true;
    }),
  );
  const selectedTool = computed(() =>
    tools.value.find((tool) => tool.name === selectedToolName.value) ?? null,
  );
  const toolOptions = computed(() =>
    filteredToolsByFacet.value.map((t) => ({ title: `${t.displayName} (${t.name})`, value: t.name })),
  );
  const toolCategoryOptions = computed(() =>
    Array.from(new Set(tools.value.map((tool) => tool.category).filter(Boolean))).sort(),
  );
  const toolRiskOptions = computed(() =>
    Array.from(new Set(tools.value.map((tool) => tool.riskLevel).filter(Boolean))).sort(),
  );
  const skillOptions = computed(() =>
    skills.value.map((s) => ({ title: s.displayName, value: s.id })),
  );
  const pendingApprovals = computed(() => approvals.value.filter((a) => a.status === "PENDING"));
  const filteredRuns = computed(() =>
    runStatusFilter.value === "attention"
      ? runs.value.filter((run) => ["PENDING_APPROVAL", "FAILED", "TIMED_OUT"].includes(run.status))
      : runStatusFilter.value === ""
        ? runs.value
        : runs.value.filter((run) => run.status === runStatusFilter.value),
  );

  async function refreshAll(): Promise<void> {
    loading.value = true;
    errorMessage.value = "";
    try {
      const snapshot = await fetchADKSettingsSnapshot();
      providers.value = snapshot.providers;
      agents.value = snapshot.agents;
      tools.value = snapshot.tools;
      skills.value = snapshot.skills;
      runtimeSettingsForm.value = {
        runTimeoutSeconds: Math.max(1, Math.round((snapshot.runtimeSettings?.runTimeoutMs ?? 600_000) / 1000)),
        streamIdleTimeoutSeconds: Math.max(1, Math.round((snapshot.runtimeSettings?.streamIdleTimeoutMs ?? 300_000) / 1000)),
      };
      optimizationTasks.value = snapshot.optimizationTasks;
      tasks.value = snapshot.tasks;
      memoryEntries.value = snapshot.memoryEntries;
      agentTemplates.value = snapshot.agentTemplates;
      metrics.value = snapshot.metrics;
      await Promise.all([refreshRuns(), refreshApprovals(), refreshAuditEvents()]);
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "加载智能体配置失败";
    } finally {
      loading.value = false;
    }
  }

  async function refreshRuns(): Promise<void> {
    const response = await fetchADKRunsPage(runPage.value, runStatusFilter.value);
    runs.value = response.runs ?? [];
    runPage.value = response.page ?? fallbackPage(runPage.value.limit, runPage.value.offset, runs.value.length);
  }

  async function refreshApprovals(): Promise<void> {
    const response = await fetchADKApprovalsPage(approvalPage.value, approvalStatusFilter.value);
    approvals.value = response.approvals ?? [];
    approvalPage.value = response.page ?? fallbackPage(approvalPage.value.limit, approvalPage.value.offset, approvals.value.length);
  }

  async function refreshAuditEvents(): Promise<void> {
    const response = await fetchADKAuditPage(auditPage.value, auditKindFilter.value);
    auditEvents.value = response.events ?? [];
    auditPage.value = response.page ?? fallbackPage(auditPage.value.limit, auditPage.value.offset, auditEvents.value.length);
  }

  async function refreshSkills(): Promise<void> {
    skills.value = await fetchADKSkills();
  }

  async function refreshOptimizationTasks(): Promise<void> {
    optimizationTasks.value = await fetchADKOptimizationTasks();
  }

  async function refreshTasks(): Promise<void> {
    tasks.value = await fetchADKTasks({
      status: taskStatusFilter.value,
      agentId: taskAgentFilter.value,
    });
  }

  async function refreshMemory(): Promise<void> {
    memoryEntries.value = await fetchADKMemory({
      scope: memoryScopeFilter.value,
      agentId: memoryAgentFilter.value,
      key: memoryKeyFilter.value,
    });
  }

  async function refreshMetrics(): Promise<void> {
    metrics.value = await fetchADKMetrics();
  }

  async function cancelRun(run: ADKRun): Promise<void> {
    try {
      await cancelADKRun(run.id);
      await Promise.all([refreshRuns(), refreshApprovals(), refreshAuditEvents(), refreshMetrics()]);
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "取消运行失败";
    }
  }

  async function cancelOptimizationTask(task: ADKOptimizationTask): Promise<void> {
    try {
      await cancelADKOptimizationTask(task.id);
      await Promise.all([refreshOptimizationTasks(), refreshMetrics()]);
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "取消优化任务失败";
    }
  }

  async function installSkill(): Promise<void> {
    const url = skillUrl.value.trim();
    if (url === "") return;
    try {
      await installADKSkill(url);
      skillUrl.value = "";
      await refreshSkills();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "安装失败";
    }
  }

  function applyAgentTemplate(template: Omit<ADKAgent, "createdAt" | "updatedAt">): void {
    agentFormState.agentForm.value = {
      id: "",
      name: template.name,
      instruction: template.instruction,
      providerId: providers.value[0]?.id ?? "",
      model: template.model ?? "",
      tools: [...template.tools],
      skills: [...template.skills],
      permissionMode: template.permissionMode,
      memoryEnabled: template.memoryEnabled,
      status: template.status,
    };
    agentTemplateNotice.value = `已载入「${template.name}」模板。保存智能体表单后生效。`;
  }

  async function uninstallSkill(skill: ADKSkill): Promise<void> {
    if (isInternalSkill(skill)) {
      errorMessage.value = "内部来源的技能不允许卸载";
      return;
    }
    try {
      await uninstallADKSkill(skill.id);
      await refreshSkills();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "卸载失败";
    }
  }

  function previousRunsPage(): Promise<void> {
    previousPage(runPage.value);
    return refreshRuns();
  }

  function nextRunsPage(): Promise<void> {
    nextPage(runPage.value, runPage.value);
    return refreshRuns();
  }

  function previousApprovalsPage(): Promise<void> {
    previousPage(approvalPage.value);
    return refreshApprovals();
  }

  function nextApprovalsPage(): Promise<void> {
    nextPage(approvalPage.value, approvalPage.value);
    return refreshApprovals();
  }

  function previousAuditPage(): Promise<void> {
    previousPage(auditPage.value);
    return refreshAuditEvents();
  }

  function nextAuditPage(): Promise<void> {
    nextPage(auditPage.value, auditPage.value);
    return refreshAuditEvents();
  }

  function openToolDetail(toolName: string): void {
    selectedToolName.value = toolName;
    toolDetailDialogOpen.value = true;
  }

  function closeToolDetail(): void {
    toolDetailDialogOpen.value = false;
  }

  async function saveRuntimeSettings(): Promise<void> {
    try {
      const settings = await saveADKRuntimeSettings({
        runTimeoutMs: Math.max(1, Math.round(Number(runtimeSettingsForm.value.runTimeoutSeconds || 0) * 1000)),
        streamIdleTimeoutMs: Math.max(1, Math.round(Number(runtimeSettingsForm.value.streamIdleTimeoutSeconds || 0) * 1000)),
      });
      runtimeSettingsForm.value = {
        runTimeoutSeconds: Math.max(1, Math.round(settings.runTimeoutMs / 1000)),
        streamIdleTimeoutSeconds: Math.max(1, Math.round(settings.streamIdleTimeoutMs / 1000)),
      };
      successMessage.value = "ADK 运行时设置已保存";
      await refreshAll();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "保存运行时设置失败";
    }
  }

  const providerFormState = useADKProviderForm(refreshAll, successMessage, errorMessage);
  const agentFormState = useADKAgentForm(providers, tools, skills, refreshAll, successMessage, errorMessage);

  onMounted(() => {
    void refreshAll().then(() => {
      if (agentFormState.agentForm.value.providerId === "" && providers.value.length > 0) {
        agentFormState.agentForm.value.providerId = providers.value[0]!.id;
      }
      if (agentFormState.agentForm.value.tools.length === 0) {
        agentFormState.agentForm.value.tools = tools.value.slice(0, 8).map((t) => t.name);
      }
      if (agentFormState.agentForm.value.id === "" && agentFormState.agentForm.value.skills.length === 0) {
        agentFormState.agentForm.value.skills = skills.value.map((skill) => skill.id);
      }
    });
  });

  watch(runStatusFilter, () => {
    runPage.value.offset = 0;
    void refreshRuns();
  });

  watch(approvalStatusFilter, () => {
    approvalPage.value.offset = 0;
    void refreshApprovals();
  });

  watch(auditKindFilter, () => {
    auditPage.value.offset = 0;
    void refreshAuditEvents();
  });

  watch([taskStatusFilter, taskAgentFilter], () => {
    void refreshTasks();
  });

  watch([memoryScopeFilter, memoryAgentFilter, memoryKeyFilter], () => {
    void refreshMemory();
  });

  return {
    activeTab,
    agents,
    agentForm: agentFormState.agentForm,
    approvalPage,
    approvals,
    approvalStatusFilter,
    auditEvents,
    auditKindFilter,
    auditPage,
    agentTemplates,
    agentTemplateNotice,
    applyAgentTemplate,
    cancelOptimizationTask,
    cancelRun,
    deleteAgent: agentFormState.deleteAgent,
    deleteProvider: providerFormState.deleteProvider,
    duplicateAgent: agentFormState.duplicateAgent,
    editAgent: agentFormState.editAgent,
    editProvider: providerFormState.editProvider,
    errorMessage,
    filteredRuns,
    formatDateTime,
    formatGenericStatusLabel,
    formatPermission,
    installSkill,
    isInternalSkill,
    loading,
    metrics,
    memoryAgentFilter,
    memoryEntries,
    memoryKeyFilter,
    memoryScopeFilter,
    newAgentForm: agentFormState.newAgentForm,
    newProviderForm: providerFormState.newProviderForm,
    nextApprovalsPage,
    nextAuditPage,
    nextRunsPage,
    optimizationTasks,
    pageSummary,
    pendingApprovals,
    permissionModes,
    preview,
    previousApprovalsPage,
    previousAuditPage,
    previousRunsPage,
    providerForm: providerFormState.providerForm,
    runtimeSettingsForm,
    providerOptions,
    providers,
    refreshAll,
    refreshMetrics,
    riskColor,
    riskLabel,
    runPage,
    runStatusFilter,
    runTerminalMessage,
    saveAgent: agentFormState.saveAgent,
    saveProvider: providerFormState.saveProvider,
    saveRuntimeSettings,
    skillOptions,
    skills,
    skillUrl,
    successMessage,
    testProvider: providerFormState.testProvider,
    taskAgentFilter,
    taskStatusFilter,
    tasks,
    toolCallStatusColor,
    toolCategoryFilter,
    toolCategoryOptions,
    toolDetailDialogOpen,
    toolOptions,
    toolRiskFilter,
    toolRiskOptions,
    toolSearchQuery,
    openToolDetail,
    closeToolDetail,
    filteredTools,
    selectedTool,
    tools,
    uninstallSkill,
  };
}
