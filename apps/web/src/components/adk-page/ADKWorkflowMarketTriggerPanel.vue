<script setup lang="ts">
import type { ADKWorkflowTrigger } from "@/contracts";
import type { TriggerFormModel } from "@/features/adkWorkflowForms";
import {
  formatJson,
  marketEdgeOptions,
  marketOperatorOptions,
} from "@/features/adkWorkflowStudio";

defineProps<{
  triggerForm: TriggerFormModel;
  selectedTrigger: ADKWorkflowTrigger | null;
  latestMarketEvent: unknown;
  formatDateTime: (value: string) => string;
}>();
</script>

<template>
  <div class="adk-inspector-section is-inner">
    <h3>行情阈值</h3>
    <v-text-field
      v-model="triggerForm.market.instrumentIdsText"
      label="标的列表"
      density="comfortable"
    />
    <v-text-field
      v-model="triggerForm.market.snapshotPath"
      label="指标路径"
      density="comfortable"
    />
    <div class="adk-inspector-grid">
      <v-select
        v-model="triggerForm.market.operator"
        :items="marketOperatorOptions"
        label="比较符"
        density="comfortable"
      />
      <v-text-field
        v-model="triggerForm.market.value"
        label="阈值"
        type="number"
        density="comfortable"
      />
    </div>
    <div class="adk-inspector-grid">
      <v-select
        v-model="triggerForm.market.edge"
        :items="marketEdgeOptions"
        label="触发边沿"
        density="comfortable"
      />
      <v-text-field
        v-model="triggerForm.market.cooldownSec"
        label="冷却秒数"
        type="number"
        density="comfortable"
      />
    </div>
    <div class="adk-trigger-health">
      <strong>行情观测</strong>
      <small>冷却时间：{{ triggerForm.market.cooldownSec || 900 }} 秒</small>
      <small v-if="selectedTrigger?.lastRunAt">
        上次触发：{{ formatDateTime(selectedTrigger.lastRunAt) }}
      </small>
      <small v-if="selectedTrigger?.lastError">
        最近错误：{{ selectedTrigger.lastError }}
      </small>
      <pre v-if="latestMarketEvent">{{ formatJson(latestMarketEvent) }}</pre>
    </div>
  </div>
</template>
