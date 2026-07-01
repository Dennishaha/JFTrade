<script setup lang="ts">
import type { ADKWorkflowTrigger } from "@/contracts";
import type { TriggerFormModel } from "@/features/adkWorkflowForms";
import {
  scheduleFrequencyOptions,
  weekdayOptions,
} from "@/features/adkWorkflowStudio";

defineProps<{
  triggerForm: TriggerFormModel;
  selectedTrigger: ADKWorkflowTrigger | null;
  schedulePreviewRuns: string[];
  formatDateTime: (value: string) => string;
}>();
</script>

<template>
  <div class="adk-inspector-section is-inner">
    <h3>定时规则</h3>
    <v-select
      v-model="triggerForm.schedule.frequency"
      :items="scheduleFrequencyOptions"
      label="频率"
      density="comfortable"
    />
    <div class="adk-inspector-grid">
      <v-text-field
        v-model="triggerForm.schedule.time"
        label="时间"
        type="time"
        density="comfortable"
      />
      <v-text-field
        v-model="triggerForm.schedule.timezone"
        label="时区"
        density="comfortable"
      />
    </div>
    <v-select
      v-if="triggerForm.schedule.frequency === 'weekly'"
      v-model="triggerForm.schedule.weekdays"
      :items="weekdayOptions"
      label="星期"
      density="comfortable"
      multiple
      chips
    />
    <v-text-field
      v-if="triggerForm.schedule.frequency === 'custom'"
      v-model="triggerForm.schedule.customCron"
      label="自定义定时表达式"
      density="comfortable"
    />
    <div class="adk-trigger-health">
      <strong>未来运行预览</strong>
      <span v-for="item in schedulePreviewRuns" :key="item">{{ item }}</span>
      <small v-if="selectedTrigger?.nextRunAt">
        下一次：{{ formatDateTime(selectedTrigger.nextRunAt) }}
      </small>
      <small v-if="selectedTrigger?.lastError">
        最近错误：{{ selectedTrigger.lastError }}
      </small>
    </div>
  </div>
</template>
