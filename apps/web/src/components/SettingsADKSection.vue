<script setup lang="ts">
import ADKAgentsPanel from "./adk-settings/ADKAgentsPanel.vue";
import ADKProvidersPanel from "./adk-settings/ADKProvidersPanel.vue";
import ADKRunsPanel from "./adk-settings/ADKRunsPanel.vue";
import ADKSkillsPanel from "./adk-settings/ADKSkillsPanel.vue";
import { useADKSettingsSectionState } from "../composables/useADKSettingsSectionState";

const {
  activeTab,
  agents,
  agentForm,
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
  providerOptions,
  providers,
  riskColor,
  riskLabel,
  runPage,
  runStatusFilter,
  runTerminalMessage,
  saveAgent,
  saveProvider,
  skillOptions,
  skills,
  skillUrl,
  successMessage,
  testProvider,
  toolCallStatusColor,
  toolOptions,
  tools,
  uninstallSkill,
} = useADKSettingsSectionState();
</script>

<template>
  <div class="grid gap-5">

    <v-alert
      v-if="errorMessage"
      type="warning"
      variant="tonal"
      density="compact"
      closable
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
      @click:close="successMessage = ''"
    >
      {{ successMessage }}
    </v-alert>

    <v-tabs v-model="activeTab" class="tv-page-tabs">
      <v-tab value="providers">AI Providers</v-tab>
      <v-tab value="agents">Agents</v-tab>
      <v-tab value="skills">Skills</v-tab>
      <v-tab value="runs">运行记录</v-tab>
    </v-tabs>

    <v-window v-model="activeTab">

      <!-- ─── Providers ─── -->
      <v-window-item value="providers">
        <ADKProvidersPanel
          :provider-form="providerForm"
          :providers="providers"
          :save-provider="saveProvider"
          :new-provider-form="newProviderForm"
          :edit-provider="editProvider"
          :test-provider="testProvider"
          :delete-provider="deleteProvider"
        />
      </v-window-item>

      <!-- ─── Agents ─── -->
      <v-window-item value="agents">
        <ADKAgentsPanel
          :agent-form="agentForm"
          :agents="agents"
          :provider-options="providerOptions"
          :tool-options="toolOptions"
          :skill-options="skillOptions"
          :permission-modes="permissionModes"
          :tools="tools"
          :format-permission="formatPermission"
          :risk-color="riskColor"
          :risk-label="riskLabel"
          :save-agent="saveAgent"
          :new-agent-form="newAgentForm"
          :edit-agent="editAgent"
          :duplicate-agent="duplicateAgent"
          :delete-agent="deleteAgent"
        />
      </v-window-item>

      <!-- ─── Skills ─── -->
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

      <!-- ─── Runs ─── -->
      <v-window-item value="runs">
        <ADKRunsPanel
          :metrics="metrics"
          :pending-approvals="pendingApprovals"
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
  </div>
</template>
