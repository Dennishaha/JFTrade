<script setup lang="ts">
import { computed, ref } from "vue";

import { formatGenericStatusLabel } from "../../composables/consoleDataFormatting";
import {
  runErrorSummary,
} from "../../composables/adkChatPresentation";
import {
  type ADKChildRunQueueItem,
  workflowQueueTone,
} from "../../composables/useADKWorkflowQueueState";

const props = withDefaults(
  defineProps<{
    item: ADKChildRunQueueItem;
    compact?: boolean;
    active?: boolean;
    showEnter?: boolean;
    variant?: "queue" | "timeline";
  }>(),
  {
    compact: false,
    active: false,
    showEnter: false,
    variant: "queue",
  },
);

const emit = defineEmits<{
  select: [runId: string];
}>();

const expanded = ref(false);

const run = computed(() => props.item.run);
const status = computed(() => String(props.item.status ?? "").trim());
const error = computed(() => runErrorSummary(run.value));
const title = computed(
  () => props.item.stepTitle || props.item.stepMessage || `子智能体 ${props.item.id}`,
);
const statusTone = computed(() => workflowQueueTone(status.value));
const collapsedHint = computed(() => {
  if (props.item.errorSummary) return props.item.errorSummary;
  if (props.item.pendingApprovalCount > 0) {
    return `待审批 ${props.item.pendingApprovalCount}`;
  }
  return "";
});
const detailRows = computed(() =>
  [
    ["Run ID", props.item.id],
    ["Parent Run ID", props.item.parentRunId || run.value?.parentRunId],
    ["错误码", props.item.errorCode || error.value?.code],
    ["Provider", run.value?.providerName || run.value?.providerId],
    ["Model", run.value?.model],
    ["用户消息", props.item.userMessage || run.value?.userMessage],
    ["失败原因", props.item.errorDetail || error.value?.detail],
  ].filter((row): row is [string, string] => String(row[1] ?? "").trim() !== ""),
);
const usageText = computed(() => {
  const usage = run.value?.usage;
  if (!usage) return "";
  const parts = [
    usage.modelCalls != null ? `模型 ${usage.modelCalls}` : "",
    usage.toolCallsTotal != null ? `工具 ${usage.toolCallsTotal}` : "",
    usage.tokensIn != null ? `输入 ${usage.tokensIn}` : "",
    usage.tokensOut != null ? `输出 ${usage.tokensOut}` : "",
    usage.durationMs != null ? `耗时 ${durationLabel(usage.durationMs)}` : "",
  ].filter(Boolean);
  return parts.join(" · ");
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

function durationLabel(durationMs: number): string {
  if (durationMs < 1000) return `${durationMs} ms`;
  return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 1 : 0)} s`;
}

function toggleExpanded(): void {
  expanded.value = !expanded.value;
}

function handleCardKeydown(event: KeyboardEvent): void {
  if (event.key !== "Enter" && event.key !== " ") return;
  event.preventDefault();
  toggleExpanded();
}

function selectChild(): void {
  emit("select", props.item.id);
}
</script>

<template>
  <div
    class="adk-child-run-trace"
    :class="{
      'adk-child-run-trace--compact': compact,
      'adk-child-run-trace--timeline': variant === 'timeline',
      'is-active-child': active,
    }"
  >
    <div
      role="button"
      tabindex="0"
      class="adk-run-trace-card adk-run-trace-card--tool adk-child-run-trace__card"
      @click="toggleExpanded"
      @keydown="handleCardKeydown"
    >
      <span class="adk-run-trace-card__main">
        <span class="adk-run-trace-card__title">
          <span class="adk-run-trace-card__index">#{{ item.index }}</span>
          {{ title }}
        </span>
        <span class="adk-run-trace-card__meta">
          <span
            class="adk-status-pill"
            :class="`is-${statusTone}`"
          >
            {{ formatGenericStatusLabel(status) }}
          </span>
          <span v-if="item.stepIndex">步骤 #{{ item.stepIndex }}</span>
          <span>{{ formatUpdatedAt(item.updatedAt) }}</span>
          <span v-if="collapsedHint" class="adk-run-trace-card__error">
            {{ collapsedHint }}
          </span>
        </span>
      </span>
      <span class="adk-child-run-trace__actions">
        <button
          v-if="showEnter"
          type="button"
          class="adk-workspace-queue-button"
          @click.stop="selectChild"
        >
          进入
        </button>
        <span class="adk-run-trace-card__chevron">{{ expanded ? "-" : "+" }}</span>
      </span>
    </div>

    <div v-if="expanded" class="adk-run-trace-detail adk-child-run-trace__detail">
      <div v-if="usageText" class="adk-child-run-trace__usage">{{ usageText }}</div>
      <dl class="adk-child-run-trace__fields">
        <template v-for="[label, value] in detailRows" :key="label">
          <dt>{{ label }}</dt>
          <dd>{{ value }}</dd>
        </template>
      </dl>
    </div>
  </div>
</template>
