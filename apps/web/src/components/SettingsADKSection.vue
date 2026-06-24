<script setup lang="ts">
import { ref, watch } from "vue";
import { useRoute, useRouter, type LocationQueryRaw } from "vue-router";

import ADKAgentsPanel from "./adk-settings/ADKAgentsPanel.vue";
import ADKProvidersPanel from "./adk-settings/ADKProvidersPanel.vue";
import ADKRunsPanel from "./adk-settings/ADKRunsPanel.vue";
import ADKSkillsPanel from "./adk-settings/ADKSkillsPanel.vue";
import ADKToolsPanel from "./adk-settings/ADKToolsPanel.vue";
import { useADKSettingsSectionState } from "../composables/useADKSettingsSectionState";

const {
  activeTab,
  agents,
  agentForm,
  agentTemplateNotice,
  agentTemplates,
  applyAgentTemplate,
  approvalPage,
  approvals,
  approvalStatusFilter,
  auditEvents,
  auditKindFilter,
  auditPage,
  cancelOptimizationTask,
  cancelRun,
  deleteAgent,
  deleteProvider,
  duplicateAgent,
  editAgent,
  editProvider,
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
  newAgentForm,
  newProviderForm,
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
  providerForm,
  runtimeSettingsForm,
  providerOptions,
  providers,
  resumeRun,
  riskColor,
  riskLabel,
  runPage,
  runStatusFilter,
  runTerminalMessage,
  saveAgent,
  saveProvider,
  saveRuntimeSettings,
  setDefaultProvider,
  skillOptions,
  skills,
  skillUrl,
  successMessage,
  taskAgentFilter,
  taskStatusFilter,
  tasks,
  testProvider,
  toolDetailDialogOpen,
  toolCallStatusColor,
  toolCategoryFilter,
  toolCategoryOptions,
  toolSearchQuery,
  toolOptions,
  toolRiskFilter,
  toolRiskOptions,
  filteredTools,
  openToolDetail,
  closeToolDetail,
  selectedTool,
  tools,
  uninstallSkill,
} = useADKSettingsSectionState();

const observationTab = ref("workflow");
const route = useRoute();
const router = useRouter();

const adkTabs = new Set(["providers", "agents", "tools", "skills", "observation"]);
const observationTabs = new Set(["workflow", "runs"]);

function firstQueryValue(value: unknown): string | undefined {
  if (Array.isArray(value)) {
    return typeof value[0] === "string" ? value[0] : undefined;
  }
  return typeof value === "string" ? value : undefined;
}

function normalizeADKTab(value: unknown): string {
  const tab = firstQueryValue(value);
  return tab && adkTabs.has(tab) ? tab : "providers";
}

function normalizeObservationTab(value: unknown): string {
  const tab = firstQueryValue(value);
  return tab && observationTabs.has(tab) ? tab : "workflow";
}

function isADKSettingsRoute(): boolean {
  const section = Array.isArray(route.params.section)
    ? route.params.section[0]
    : route.params.section;
  return route.path === "/settings/adk" || section === "adk";
}

function buildADKQuery(tab: string, view: string): LocationQueryRaw {
  const query: LocationQueryRaw = { ...route.query };
  delete query.tab;
  delete query.view;

  if (tab !== "providers") {
    query.tab = tab;
  }
  if (tab === "observation") {
    query.view = view;
  }

  return query;
}

function comparableQuery(query: LocationQueryRaw): string {
  return JSON.stringify(
    Object.entries(query)
      .filter(([, value]) => value !== undefined)
      .map(([key, value]) => ({
        key,
        value: Array.isArray(value)
          ? value.map((entry) => String(entry))
          : String(value),
      }))
      .sort((left, right) => left.key.localeCompare(right.key)),
  );
}

function syncRouteQuery(tab: string, view: string): void {
  if (!isADKSettingsRoute()) return;
  const nextQuery = buildADKQuery(tab, view);
  if (comparableQuery(route.query) === comparableQuery(nextQuery)) return;
  void router.replace({ path: route.path, query: nextQuery });
}

watch(
  () => [route.params.section, route.query.tab, route.query.view],
  () => {
    if (!isADKSettingsRoute()) return;
    const nextTab = normalizeADKTab(route.query.tab);
    const nextObservationTab = normalizeObservationTab(route.query.view);
    if (activeTab.value !== nextTab) {
      activeTab.value = nextTab;
    }
    if (observationTab.value !== nextObservationTab) {
      observationTab.value = nextObservationTab;
    }
    syncRouteQuery(nextTab, nextObservationTab);
  },
  { immediate: true },
);

watch([activeTab, observationTab], ([tab, view]) => {
  const nextTab = normalizeADKTab(tab);
  const nextObservationTab = normalizeObservationTab(view);
  if (activeTab.value !== nextTab) {
    activeTab.value = nextTab;
  }
  if (observationTab.value !== nextObservationTab) {
    observationTab.value = nextObservationTab;
  }
  syncRouteQuery(nextTab, nextObservationTab);
});

function agentName(agentId: string | undefined): string {
  if (!agentId) return "未绑定智能体";
  return agents.value.find((agent) => agent.id === agentId)?.name ?? agentId;
}

function taskStatusColor(status: string): string {
  switch (status) {
    case "DONE":
      return "success";
    case "IN_PROGRESS":
      return "info";
    case "BLOCKED":
      return "warning";
    case "CANCELLED":
      return "grey";
    case "TODO":
      return "default";
    default:
      return "error";
  }
}

function memoryScopeLabel(scope: string): string {
  return scope === "agent" ? "智能体记忆" : "工作区记忆";
}

function memoryScopeHint(scope: string): string {
  return scope === "agent" ? "仅当前智能体使用" : "对开启记忆的智能体全局可见";
}
</script>

<template>
  <div class="adk-settings-section grid gap-5">

    <v-alert
      v-if="errorMessage"
      type="warning"
      variant="tonal"
      density="compact"
      closable
      class="adk-settings-alert"
      @click:close="errorMessage = ''"
    >
      {{ errorMessage }}
    </v-alert>

    <v-alert
      v-if="successMessage"
      type="success"
      variant="tonal"
      density="compact"
      closable
      class="adk-settings-alert"
      @click:close="successMessage = ''"
    >
      {{ successMessage }}
    </v-alert>

    <v-tabs v-model="activeTab" class="tv-page-tabs">
      <v-tab value="providers">模型服务</v-tab>
      <v-tab value="agents">智能体</v-tab>
      <v-tab value="tools">工具</v-tab>
      <v-tab value="skills">技能</v-tab>

      <!-- 观察放最后 -->
      <v-tab value="observation">观察</v-tab>
    </v-tabs>

    <v-window v-model="activeTab" class="adk-settings-window">

      <!-- ─── Providers ─── -->
      <v-window-item value="providers">
        <ADKProvidersPanel
          :provider-form="providerForm"
          :runtime-settings-form="runtimeSettingsForm"
          :providers="providers"
          :save-provider="saveProvider"
          :save-runtime-settings="saveRuntimeSettings"
          :new-provider-form="newProviderForm"
          :edit-provider="editProvider"
          :test-provider="testProvider"
          :delete-provider="deleteProvider"
          :set-default-provider="setDefaultProvider"
        />
      </v-window-item>

      <!-- ─── Agents ─── -->
      <v-window-item value="agents">
        <ADKAgentsPanel
          :agent-form="agentForm"
          :agents="agents"
          :agent-templates="agentTemplates"
          :agent-template-notice="agentTemplateNotice"
          :provider-options="providerOptions"
          :tool-options="toolOptions"
          :skill-options="skillOptions"
          :permission-modes="permissionModes"
          :tools="tools"
          :format-permission="formatPermission"
          :risk-color="riskColor"
          :risk-label="riskLabel"
          :tool-category-filter="toolCategoryFilter"
          :tool-category-options="toolCategoryOptions"
          :tool-risk-filter="toolRiskFilter"
          :tool-risk-options="toolRiskOptions"
          :apply-agent-template="applyAgentTemplate"
          :save-agent="saveAgent"
          :new-agent-form="newAgentForm"
          :edit-agent="editAgent"
          :duplicate-agent="duplicateAgent"
          :delete-agent="deleteAgent"
          @update:tool-category-filter="toolCategoryFilter = $event"
          @update:tool-risk-filter="toolRiskFilter = $event"
        />
      </v-window-item>

      <v-window-item value="tools">
        <ADKToolsPanel
          :tools="tools"
          :filtered-tools="filteredTools"
          :selected-tool="selectedTool"
          :tool-category-filter="toolCategoryFilter"
          :tool-category-options="toolCategoryOptions"
          :tool-risk-filter="toolRiskFilter"
          :tool-risk-options="toolRiskOptions"
          :tool-search-query="toolSearchQuery"
          :tool-detail-dialog-open="toolDetailDialogOpen"
          :preview="preview"
          :format-permission-mode="formatPermission"
          :risk-color="riskColor"
          :risk-label="riskLabel"
          :open-tool-detail="openToolDetail"
          :close-tool-detail="closeToolDetail"
          @update:tool-category-filter="toolCategoryFilter = $event"
          @update:tool-risk-filter="toolRiskFilter = $event"
          @update:tool-search-query="toolSearchQuery = $event"
          @update:tool-detail-dialog-open="toolDetailDialogOpen = $event"
        />
      </v-window-item>

      <v-window-item value="observation">
        <section class="grid gap-4">
          <v-tabs v-model="observationTab" class="tv-page-tabs">
            <v-tab value="workflow">工作流</v-tab>
            <v-tab value="runs">运行与审计</v-tab>
          </v-tabs>
          <v-window v-model="observationTab" class="adk-settings-window">
            <v-window-item value="workflow">
              <section class="grid gap-5 lg:grid-cols-2">
                <v-card flat class="card-shell border-0">
                  <v-card-title>智能体任务</v-card-title>
                  <v-card-text class="grid gap-3">
                    <v-alert type="info" variant="tonal" density="compact" class="adk-settings-alert">
                      任务由智能体在对话和工具执行中自动创建与更新，用于跟踪策略研究、诊断和待处理事项；这里仅用于观察，不需要手动配置。
                    </v-alert>
                    <div class="grid gap-3 md:grid-cols-2">
                      <v-select
                        v-model="taskStatusFilter"
                        label="按状态过滤"
                        density="comfortable"
                        clearable
                        :items="['TODO', 'IN_PROGRESS', 'BLOCKED', 'DONE', 'CANCELLED']"
                      />
                      <v-select
                        v-model="taskAgentFilter"
                        label="按智能体过滤"
                        density="comfortable"
                        clearable
                        :items="[{ title: '全部智能体', value: '' }, ...agents.map((agent) => ({ title: agent.name, value: agent.id }))]"
                      />
                    </div>
                    <div class="grid gap-2">
                      <div v-for="task in tasks" :key="task.id" class="rounded border border-slate-200 p-3 text-sm">
                        <div class="flex flex-wrap items-start justify-between gap-2">
                          <div>
                            <div class="font-medium text-slate-900">{{ task.title }}</div>
                            <div class="text-xs text-slate-500">更新 {{ formatDateTime(task.updatedAt) }}</div>
                          </div>
                          <v-chip size="small" :color="taskStatusColor(task.status)" variant="tonal">{{ task.status }}</v-chip>
                        </div>
                        <div class="mt-2 flex flex-wrap gap-3 text-xs text-slate-500">
                          <span>智能体：{{ agentName(task.agentId) }}</span>
                          <span v-if="task.runId"> · 运行：{{ task.runId }}</span>
                          <span v-if="task.dependsOn?.length"> · 依赖：{{ task.dependsOn.length }}</span>
                        </div>
                        <div v-if="task.description" class="mt-1 text-slate-600">{{ task.description }}</div>
                      </div>
                      <div v-if="tasks.length === 0" class="text-sm text-slate-500">暂无智能体创建的任务。</div>
                    </div>
                  </v-card-text>
                </v-card>

                <v-card flat class="card-shell border-0">
                  <v-card-title>智能体记忆</v-card-title>
                  <v-card-text class="grid gap-3">
                    <v-alert type="info" variant="tonal" density="compact" class="adk-settings-alert">
                      记忆由智能体在对话中自动记录。只有开启记忆的智能体才会在提示词中读取工作区记忆和当前智能体记忆。
                    </v-alert>
                    <div class="grid gap-3 md:grid-cols-3">
                      <v-select
                        v-model="memoryScopeFilter"
                        label="按范围过滤"
                        density="comfortable"
                        clearable
                        :items="[{ title: '工作区', value: 'workspace' }, { title: '智能体', value: 'agent' }]"
                      />
                      <v-select
                        v-model="memoryAgentFilter"
                        label="按智能体过滤"
                        density="comfortable"
                        clearable
                        :items="[{ title: '全部智能体', value: '' }, ...agents.map((agent) => ({ title: agent.name, value: agent.id }))]"
                      />
                      <v-text-field v-model="memoryKeyFilter" label="按 key 过滤" density="comfortable" clearable />
                    </div>
                    <div class="grid gap-2">
                      <div v-for="entry in memoryEntries" :key="entry.id" class="rounded border border-slate-200 p-3 text-sm">
                        <div class="flex flex-wrap items-start justify-between gap-2">
                          <div>
                            <div class="font-medium text-slate-900">{{ entry.key }}</div>
                            <div class="text-xs text-slate-500">更新 {{ formatDateTime(entry.updatedAt) }}</div>
                          </div>
                          <v-chip size="small" variant="tonal">{{ memoryScopeLabel(entry.scope) }}</v-chip>
                        </div>
                        <div class="mt-2 flex flex-wrap gap-3 text-xs text-slate-500">
                          <span>{{ memoryScopeHint(entry.scope) }}</span>
                          <span v-if="entry.agentId">智能体：{{ agentName(entry.agentId) }}</span>
                        </div>
                        <div class="mt-1 text-slate-600">{{ entry.value }}</div>
                      </div>
                      <div v-if="memoryEntries.length === 0" class="text-sm text-slate-500">暂无智能体记录的记忆。</div>
                    </div>
                  </v-card-text>
                </v-card>
              </section>
            </v-window-item>

            <v-window-item value="runs">
              <ADKRunsPanel
                :metrics="metrics"
                :pending-approvals="pendingApprovals"
                :agents="agents"
                :providers="providers"
                :run-status-filter="runStatusFilter"
                :run-page="runPage"
                :filtered-runs="filteredRuns"
                :approval-status-filter="approvalStatusFilter"
                :approval-page="approvalPage"
                :approvals="approvals"
                :optimization-tasks="optimizationTasks"
                :audit-kind-filter="auditKindFilter"
                :audit-page="auditPage"
                :audit-events="auditEvents"
                :page-summary="pageSummary"
                :format-generic-status-label="formatGenericStatusLabel"
                :format-date-time="formatDateTime"
                :tool-call-status-color="toolCallStatusColor"
                :preview="preview"
                :run-terminal-message="runTerminalMessage"
                :cancel-run="cancelRun"
                :resume-run="resumeRun"
                :cancel-optimization-task="cancelOptimizationTask"
                :previous-runs-page="previousRunsPage"
                :next-runs-page="nextRunsPage"
                :previous-approvals-page="previousApprovalsPage"
                :next-approvals-page="nextApprovalsPage"
                :previous-audit-page="previousAuditPage"
                :next-audit-page="nextAuditPage"
                @update:run-status-filter="runStatusFilter = $event"
                @update:approval-status-filter="approvalStatusFilter = $event"
                @update:audit-kind-filter="auditKindFilter = $event"
              />
            </v-window-item>
          </v-window>
        </section>
      </v-window-item>

      <v-window-item value="skills">
        <ADKSkillsPanel
          :skill-url="skillUrl"
          :skills="skills"
          :is-internal-skill="isInternalSkill"
          :install-skill="installSkill"
          :uninstall-skill="uninstallSkill"
          @update:skill-url="skillUrl = $event"
        />
      </v-window-item>
    </v-window>
  </div>
</template>

<style scoped>
.adk-settings-section {
  align-content: start;
}

.adk-settings-alert {
  align-self: start;
  width: 100%;
  min-height: 0;
  max-height: 7.5rem;
  overflow: auto;
}

.adk-settings-alert :deep(.v-alert__content) {
  align-self: center;
  min-width: 0;
  line-height: 1.45;
}

.adk-settings-alert :deep(.v-alert__prepend),
.adk-settings-alert :deep(.v-alert__close) {
  align-self: center;
}

.adk-settings-window {
  align-self: start;
  width: 100%;
}

.adk-settings-window :deep(.v-window__container),
.adk-settings-window :deep(.v-window-item) {
  min-height: 0;
}
</style>
