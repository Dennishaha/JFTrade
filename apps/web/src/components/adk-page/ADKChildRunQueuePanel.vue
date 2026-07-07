<script setup lang="ts">
import { computed } from "vue";

import { formatGenericStatusLabel } from "../../composables/consoleDataFormatting";
import {
  type ADKChildRunQueueItem,
} from "../../composables/useADKWorkflowQueueState";
import ADKChildRunTrace from "../shared/ADKChildRunTrace.vue";
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

function highestPriorityStatus(statuses: string[]): string {
  const priority = [
    "PENDING_APPROVAL",
    "BLOCKED",
    "FAILED",
    "RUNNING",
    "IN_PROGRESS",
    "PENDING",
    "TODO",
    "COMPLETED",
    "DONE",
  ];
  return priority.find((statusValue) =>
    statuses.some((candidate) => String(candidate).toUpperCase() === statusValue),
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
      <ADKChildRunTrace
        v-for="item in items"
        :key="item.id"
        :item="item"
        :active="item.id === activeChildRunId"
        show-enter
        @select="emit('select', $event)"
      />
    </div>
  </ADKQueuePanel>
</template>
