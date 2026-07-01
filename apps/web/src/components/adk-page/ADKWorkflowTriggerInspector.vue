<script setup lang="ts">
import type {
  ADKWorkflowNodeRun,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
} from "@/contracts";
import type { TriggerFormModel } from "@/features/adkWorkflowForms";
import {
  triggerStatusOptions,
  triggerTypeOptions,
} from "@/features/adkWorkflowStudio";
import ADKWorkflowEventTriggerPanel from "./ADKWorkflowEventTriggerPanel.vue";
import ADKWorkflowMarketTriggerPanel from "./ADKWorkflowMarketTriggerPanel.vue";
import ADKWorkflowNodeRunPreview from "./ADKWorkflowNodeRunPreview.vue";
import ADKWorkflowScheduleTriggerPanel from "./ADKWorkflowScheduleTriggerPanel.vue";
import ADKWorkflowWebhookTriggerPanel from "./ADKWorkflowWebhookTriggerPanel.vue";

defineProps<{
  triggerForm: TriggerFormModel;
  selectedTrigger: ADKWorkflowTrigger | null;
  selectedNodeRun: ADKWorkflowNodeRun | null;
  triggerRunSummary: {
    total: number;
    latest: ADKWorkflowTriggerLog | null;
    failures: number;
  } | null;
  schedulePreviewRuns: string[];
  webhookEndpoint: string;
  webhookCurlSample: string;
  latestMarketEvent: unknown;
  triggerLoading: boolean;
  runningTrigger: boolean;
  saving: boolean;
  preservedConfigCount: number;
  formatDateTime: (value: string) => string;
}>();

defineEmits<{
  refreshNodeData: [];
  runSelectedTrigger: [];
  removeSelectedTrigger: [];
}>();
</script>

<template>
  <section class="adk-inspector-section">
    <div class="adk-inspector-heading">
      <h3>触发器</h3>
      <div class="adk-inspector-actions">
        <v-btn
          size="x-small"
          variant="text"
          :loading="runningTrigger"
          :disabled="saving || triggerLoading"
          @click="$emit('runSelectedTrigger')"
        >
          运行
        </v-btn>
        <v-btn
          size="x-small"
          color="error"
          variant="text"
          @click="$emit('removeSelectedTrigger')"
        >
          删除
        </v-btn>
      </div>
    </div>
    <div v-if="triggerLoading" class="adk-workflow-muted">正在加载触发器...</div>
    <v-select
      v-model="triggerForm.type"
      :items="triggerTypeOptions"
      label="类型"
      density="comfortable"
      :disabled="triggerForm.id !== ''"
      @update:model-value="$emit('refreshNodeData')"
    />
    <v-select
      v-model="triggerForm.status"
      :items="triggerStatusOptions"
      label="状态"
      density="comfortable"
      @update:model-value="$emit('refreshNodeData')"
    />
    <v-text-field
      v-model="triggerForm.title"
      label="标题"
      density="comfortable"
      @update:model-value="$emit('refreshNodeData')"
    />

    <ADKWorkflowScheduleTriggerPanel
      v-if="triggerForm.type === 'schedule'"
      :trigger-form="triggerForm"
      :selected-trigger="selectedTrigger"
      :schedule-preview-runs="schedulePreviewRuns"
      :format-date-time="formatDateTime"
    />

    <ADKWorkflowWebhookTriggerPanel
      v-else-if="triggerForm.type === 'webhook'"
      :trigger-form="triggerForm"
      :selected-trigger="selectedTrigger"
      :trigger-run-summary="triggerRunSummary"
      :webhook-endpoint="webhookEndpoint"
      :webhook-curl-sample="webhookCurlSample"
      :format-date-time="formatDateTime"
    />

    <ADKWorkflowEventTriggerPanel
      v-else-if="triggerForm.type === 'event'"
      :trigger-form="triggerForm"
    />

    <ADKWorkflowMarketTriggerPanel
      v-else-if="triggerForm.type === 'market_threshold'"
      :trigger-form="triggerForm"
      :selected-trigger="selectedTrigger"
      :latest-market-event="latestMarketEvent"
      :format-date-time="formatDateTime"
    />

    <span v-if="preservedConfigCount > 0" class="adk-workflow-preserved">
      已保留 {{ preservedConfigCount }} 个高级配置字段
    </span>
    <ADKWorkflowNodeRunPreview :run="selectedNodeRun" />
  </section>
</template>
