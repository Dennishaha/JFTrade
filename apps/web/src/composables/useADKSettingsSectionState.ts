import { computed, onMounted, ref, watch } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKAuditEvent,
  ADKOptimizationTask,
  ADKProvider,
  ADKRun,
  ADKSkill,
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
  fetchADKOptimizationTasks,
  fetchADKRunsPage,
  fetchADKSettingsSnapshot,
  fetchADKSkills,
  installADKSkill,
  nextPage,
  pageSummary,
  previousPage,
  saveADKAgent,
  saveADKProvider,
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
  const auditEvents = ref<ADKAuditEvent[]>([]);
  const metrics = ref<ADKMetricsResponse | null>(null);
  const loading = ref(false);
  const errorMessage = ref("");
  const successMessage = ref("");
  const activeTab = ref("providers");
  const runStatusFilter = ref("attention");
  const approvalStatusFilter = ref("PENDING");
  const auditKindFilter = ref("");
  const runPage = ref<PageEnvelope>({ limit: 20, offset: 0, total: 0, returned: 0, hasMore: false });
  const approvalPage = ref<PageEnvelope>({ limit: 10, offset: 0, total: 0, returned: 0, hasMore: false });
  const auditPage = ref<PageEnvelope>({ limit: 12, offset: 0, total: 0, returned: 0, hasMore: false });
  const skillUrl = ref("");

  const providerOptions = computed(() =>
    providers.value.map((p) => ({
      title: `${p.displayName} · ${p.model}${p.hasApiKey ? "" : " · 未配置 Key"}`,
      value: p.id,
    })),
  );
  const toolOptions = computed(() =>
    tools.value.map((t) => ({ title: `${t.displayName} (${t.name})`, value: t.name })),
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
      optimizationTasks.value = snapshot.optimizationTasks;
      metrics.value = snapshot.metrics;
      await Promise.all([refreshRuns(), refreshApprovals(), refreshAuditEvents()]);
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "加载 Agents 配置失败";
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

  async function uninstallSkill(skill: ADKSkill): Promise<void> {
    if (isInternalSkill(skill)) {
      errorMessage.value = "内部来源的 Skill 不允许卸载";
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
    skillOptions,
    skills,
    skillUrl,
    successMessage,
    testProvider: providerFormState.testProvider,
    toolCallStatusColor,
    toolOptions,
    tools,
    uninstallSkill,
  };
}
