<script setup lang="ts">
import { computed } from "vue";

import type { ADKRun, ADKWorkflowStepState } from "@/contracts";

import { formatGenericStatusLabel } from "../../composables/consoleDataFormatting";
import { workflowQueueTone } from "../../composables/useADKWorkflowQueueState";
import ADKQueuePanel from "./ADKQueuePanel.vue";

const props = defineProps<{
  run?: ADKRun | null | undefined;
}>();

const steps = computed(() => props.run?.workflowPlan ?? []);
const displaySteps = computed(() =>
  steps.value.map((step) => ({
    ...step,
    status: effectiveStepStatus(step),
  })),
);
const status = computed(
  () =>
    highestPriorityStatus(displaySteps.value.map((step) => step.status)) ||
    props.run?.workflowStatus ||
    props.run?.status ||
    "",
);
const mode = computed(() =>
  String(props.run?.workMode ?? "").trim().toLowerCase(),
);
const showPanel = computed(() => mode.value !== "chat" && steps.value.length > 0);
const summary = computed(() => {
  const first = displaySteps.value[0];
  if (!first) return "";
  return `${first.title || "步骤 1"} · ${stepStatusLabel(first.status)}`;
});

function stepKey(step: ADKWorkflowStepState, index: number): string {
  return step.taskId || `${step.title}-${index}`;
}

function stepStatusLabel(status: string | undefined): string {
  return formatGenericStatusLabel(status);
}

function effectiveStepStatus(step: ADKWorkflowStepState): string {
  const runStatus = String(props.run?.status ?? "").trim().toUpperCase();
  const workflowStatus = String(props.run?.workflowStatus ?? "").trim().toUpperCase();
  if (
    String(step.childRunId ?? "").trim() !== "" &&
    (runStatus === "COMPLETED" ||
      workflowStatus === "COMPLETED" ||
      workflowStatus === "COMPLETE")
  ) {
    return "DONE";
  }
  return String(step.status ?? "").trim();
}

function highestPriorityStatus(statuses: string[]): string {
  const priority = [
    "PENDING_APPROVAL",
    "BLOCKED",
    "FAILED",
    "TIMED_OUT",
    "DENIED",
    "RUNNING",
    "IN_PROGRESS",
    "PENDING",
    "TODO",
    "COMPLETED",
    "DONE",
  ];
  return (
    priority.find((status) =>
      statuses.some((candidate) => String(candidate).toUpperCase() === status),
    ) ||
    statuses[0] ||
    ""
  );
}
</script>

<template>
  <ADKQueuePanel
    v-if="showPanel"
    class="adk-workflow-plan-panel"
    title="执行计划"
    :count="steps.length"
    :status="status"
    :status-label="formatGenericStatusLabel(status)"
    :summary="summary"
  >
    <div
      v-if="run?.objective || run?.iteration || run?.childRunIds?.length"
      class="adk-workflow-queue__meta"
    >
      <span v-if="run?.objective">目标：{{ run.objective }}</span>
      <span v-if="run?.iteration">第 {{ run.iteration }} 轮</span>
      <span v-if="run?.childRunIds?.length">{{ run.childRunIds.length }} 个子智能体</span>
    </div>
    <ol class="adk-workspace-queue-list adk-workflow-queue__list">
      <li
        v-for="(step, index) in displaySteps"
        :key="stepKey(step, index)"
        class="adk-workspace-queue-item adk-workflow-queue__item"
      >
        <span class="adk-workspace-queue-item__index">#{{ index + 1 }}</span>
        <span class="adk-workspace-queue-status" :class="`is-${workflowQueueTone(step.status)}`">
          {{ stepStatusLabel(step.status) }}
        </span>
        <div class="adk-workspace-queue-item__main">
          <div class="adk-workspace-queue-item__title">
            {{ step.title || `步骤 ${index + 1}` }}
          </div>
          <div class="adk-workspace-queue-item__meta">
            <span v-if="step.description || step.message">
              {{ step.description || step.message }}
            </span>
            <span v-if="step.iteration"> · 轮次 {{ step.iteration }}</span>
            <span v-if="step.childRunId"> · 子智能体 {{ step.childRunId }}</span>
          </div>
        </div>
      </li>
    </ol>
  </ADKQueuePanel>
</template>

<style scoped>
.adk-workflow-queue__list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.adk-workflow-queue__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
  color: var(--tv-text-muted);
  font-size: 12px;
}
</style>
