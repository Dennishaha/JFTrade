<script setup lang="ts">
import type {
  ADKWorkflowNodeRun,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
} from "@/contracts";
import type { PageEnvelope } from "@/composables/adkWorkflowsApi";
import type {
  TriggerFormModel,
  WorkflowFormModel,
} from "@/features/adkWorkflowForms";
import {
  inspectorTitle,
  type InspectorNodeKind,
} from "@/features/adkWorkflowStudio";
import ADKWorkflowAgentInspector from "./ADKWorkflowAgentInspector.vue";
import ADKWorkflowMonitorPanel from "./ADKWorkflowMonitorPanel.vue";
import ADKWorkflowStartInspector from "./ADKWorkflowStartInspector.vue";
import ADKWorkflowTriggerInspector from "./ADKWorkflowTriggerInspector.vue";

defineProps<{
  inspectorKind: InspectorNodeKind;
  workflowForm: WorkflowFormModel;
  triggerForm: TriggerFormModel;
  selectedTrigger: ADKWorkflowTrigger | null;
  selectedNodeRun: ADKWorkflowNodeRun | null;
  selectedLog: ADKWorkflowTriggerLog | null;
  visibleLogs: ADKWorkflowTriggerLog[];
  selectedNodeId: string;
  workflowStats: {
    total: number;
    successRate: number;
    avgMs: number;
    recent: number;
  };
  triggerRunSummary: {
    total: number;
    latest: ADKWorkflowTriggerLog | null;
    failures: number;
  } | null;
  schedulePreviewRuns: string[];
  webhookEndpoint: string;
  webhookCurlSample: string;
  latestMarketEvent: unknown;
  logTriggerOptions: Array<{ title: string; value: string }>;
  logStatusFilter: string;
  logTriggerFilter: string;
  logKeywordFilter: string;
  logFromFilter: string;
  logToFilter: string;
  logLoading: boolean;
  triggerLoading: boolean;
  runningTrigger: boolean;
  saving: boolean;
  logPage: PageEnvelope;
  logPageSummary: string;
  preservedInputCount: number;
  preservedConfigCount: number;
  agentOptions: Array<{ title: string; value: string }>;
  providerOptions: Array<{ title: string; value: string }>;
  inputVariableOptions: Array<{ title: string; value: string }>;
  providerName: (providerId: string) => string;
  formatDateTime: (value: string) => string;
  runLink: (log: ADKWorkflowTriggerLog) => string;
}>();

defineEmits<{
  hideInspector: [];
  refreshNodeData: [];
  addInputRow: [];
  removeInputRow: [index: number];
  insertPromptVariable: [value: string];
  runSelectedTrigger: [];
  removeSelectedTrigger: [];
  refreshLogs: [];
  selectLog: [logId: string];
  selectNode: [nodeId: string];
  copyResultMarkdown: [];
  previousLogPage: [];
  nextLogPage: [];
  "update:logStatusFilter": [value: string];
  "update:logTriggerFilter": [value: string];
  "update:logKeywordFilter": [value: string];
  "update:logFromFilter": [value: string];
  "update:logToFilter": [value: string];
}>();
</script>

<template>
  <aside class="adk-workflow-inspector">
    <div class="adk-workflow-inspector__head">
      <span>{{ inspectorTitle(inspectorKind) }}</span>
      <button
        data-testid="adk-workflow-inspector-hide"
        type="button"
        class="adk-workflow-inspector__hide"
        title="隐藏右栏"
        aria-label="隐藏右栏"
        @click="$emit('hideInspector')"
      >
        <span class="fa-solid fa-chevron-right" aria-hidden="true" />
      </button>
    </div>

    <ADKWorkflowStartInspector
      v-if="inspectorKind === 'start'"
      :workflow-form="workflowForm"
      :selected-node-run="selectedNodeRun"
      :preserved-input-count="preservedInputCount"
      @refresh-node-data="$emit('refreshNodeData')"
      @add-input-row="$emit('addInputRow')"
      @remove-input-row="$emit('removeInputRow', $event)"
    />

    <ADKWorkflowAgentInspector
      v-else-if="inspectorKind === 'agent'"
      :workflow-form="workflowForm"
      :selected-node-run="selectedNodeRun"
      :agent-options="agentOptions"
      :provider-options="providerOptions"
      :input-variable-options="inputVariableOptions"
      :provider-name="providerName"
      @refresh-node-data="$emit('refreshNodeData')"
      @insert-prompt-variable="$emit('insertPromptVariable', $event)"
    />

    <ADKWorkflowTriggerInspector
      v-else-if="inspectorKind === 'trigger'"
      :trigger-form="triggerForm"
      :selected-trigger="selectedTrigger"
      :selected-node-run="selectedNodeRun"
      :trigger-run-summary="triggerRunSummary"
      :schedule-preview-runs="schedulePreviewRuns"
      :webhook-endpoint="webhookEndpoint"
      :webhook-curl-sample="webhookCurlSample"
      :latest-market-event="latestMarketEvent"
      :trigger-loading="triggerLoading"
      :running-trigger="runningTrigger"
      :saving="saving"
      :preserved-config-count="preservedConfigCount"
      :format-date-time="formatDateTime"
      @refresh-node-data="$emit('refreshNodeData')"
      @run-selected-trigger="$emit('runSelectedTrigger')"
      @remove-selected-trigger="$emit('removeSelectedTrigger')"
    />

    <ADKWorkflowMonitorPanel
      v-else
      :workflow-name="workflowForm.name"
      :selected-node-id="selectedNodeId"
      :selected-log="selectedLog"
      :visible-logs="visibleLogs"
      :workflow-stats="workflowStats"
      :log-trigger-options="logTriggerOptions"
      :log-status-filter="logStatusFilter"
      :log-trigger-filter="logTriggerFilter"
      :log-keyword-filter="logKeywordFilter"
      :log-from-filter="logFromFilter"
      :log-to-filter="logToFilter"
      :log-loading="logLoading"
      :log-page="logPage"
      :log-page-summary="logPageSummary"
      :format-date-time="formatDateTime"
      :run-link="runLink"
      @refresh-logs="$emit('refreshLogs')"
      @select-log="$emit('selectLog', $event)"
      @select-node="$emit('selectNode', $event)"
      @copy-result-markdown="$emit('copyResultMarkdown')"
      @previous-log-page="$emit('previousLogPage')"
      @next-log-page="$emit('nextLogPage')"
      @update:log-status-filter="$emit('update:logStatusFilter', $event)"
      @update:log-trigger-filter="$emit('update:logTriggerFilter', $event)"
      @update:log-keyword-filter="$emit('update:logKeywordFilter', $event)"
      @update:log-from-filter="$emit('update:logFromFilter', $event)"
      @update:log-to-filter="$emit('update:logToFilter', $event)"
    />
  </aside>
</template>
