<script setup lang="ts">
import { computed } from "vue";

import { formatGenericStatusLabel } from "../../composables/consoleDataFormatting";
import {
  type ADKChildRunQueueItem,
  workflowQueueTone,
} from "../../composables/useADKWorkflowQueueState";
import ADKQueuePanel from "./ADKQueuePanel.vue";

const props = defineProps<{
  items: ADKChildRunQueueItem[];
  activeChildRunId?: string;
}>();

const emit = defineEmits<{
  select: [runId: string];
}>();

const status = computed(() => highestPriorityStatus(props.items.map((item) => item.status)));
const summary = computed(() => {
  const first = props.items[0];
  if (!first) return "";
  return `#${first.index} ${first.stepTitle || first.id} · ${formatGenericStatusLabel(first.status)}`;
});

function formatUpdatedAt(value: string | undefined): string {
  if (!value) return "尚未刷新";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function highestPriorityStatus(statuses: string[]): string {
  const priority = ["PENDING_APPROVAL", "BLOCKED", "FAILED", "RUNNING", "IN_PROGRESS", "PENDING", "TODO", "COMPLETED", "DONE"];
  return priority.find((status) =>
    statuses.some((candidate) => String(candidate).toUpperCase() === status),
  ) || statuses[0] || "";
}
</script>

<template>
  <ADKQueuePanel
    v-if="items.length > 0"
    title="子智能体"
    :count="items.length"
    :status="status"
    :status-label="formatGenericStatusLabel(status)"
    :summary="summary"
  >
    <div class="adk-workspace-queue-list">
      <div
        v-for="item in items"
        :key="item.id"
        class="adk-workspace-queue-item"
        :class="{ 'is-active-child': item.id === activeChildRunId }"
      >
        <span class="adk-workspace-queue-item__index">#{{ item.index }}</span>
        <span class="adk-workspace-queue-status" :class="`is-${workflowQueueTone(item.status)}`">
          {{ formatGenericStatusLabel(item.status) }}
        </span>
        <div class="adk-workspace-queue-item__main">
          <div class="adk-workspace-queue-item__title">
            {{ item.stepTitle || `子智能体 ${item.id}` }}
          </div>
          <div class="adk-workspace-queue-item__meta">
            <span v-if="item.stepIndex">步骤 #{{ item.stepIndex }} · </span>
            <span>{{ formatUpdatedAt(item.updatedAt) }}</span>
            <span v-if="item.pendingApprovalCount > 0">
              · 待审批 {{ item.pendingApprovalCount }}
            </span>
          </div>
        </div>
        <div class="adk-workspace-queue-item__actions">
          <button
            type="button"
            class="adk-workspace-queue-button"
            @click="emit('select', item.id)"
          >
            进入
          </button>
        </div>
      </div>
    </div>
  </ADKQueuePanel>
</template>

<style scoped>
.is-active-child {
  border-color: color-mix(in srgb, var(--tv-accent) 52%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 12%, var(--tv-bg-elevated));
}
</style>
