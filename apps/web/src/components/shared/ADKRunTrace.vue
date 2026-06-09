<script setup lang="ts">
import { computed } from "vue";

import type { ADKRun } from "@jftrade/ui-contracts";

import type { ADKToolVisualization as ADKToolVisualizationModel } from "../../composables/adkToolVisualizations";
import { buildADKToolVisualization } from "../../composables/adkToolVisualizations";
import { formatGenericStatusLabel } from "../../composables/consoleDataFormatting";
import {
  isActiveRunStatus,
  normalizedDisplayStatus,
  runStatusTone,
  runTerminalMessage,
} from "../../composables/adkChatPresentation";
import ADKToolVisualization from "./ADKToolVisualization.vue";

const props = withDefaults(defineProps<{
  run?: ADKRun | undefined;
  toolProgress?: string | undefined;
  busy?: boolean;
  compact?: boolean;
  summaryExpanded?: boolean | undefined;
  expandedToolCallIds?: string[] | undefined;
}>(), {
  toolProgress: "",
  busy: false,
  compact: false,
  summaryExpanded: false,
  expandedToolCallIds: () => [],
});

const emit = defineEmits<{
  "update:summaryExpanded": [value: boolean];
  "update:expandedToolCallIds": [value: string[]];
}>();

const toolCalls = computed(() => props.run?.toolCalls ?? []);
const hasToolCalls = computed(() => toolCalls.value.length > 0);
const hasMultipleToolCalls = computed(() => toolCalls.value.length > 1);
const hasActiveToolCalls = computed(() =>
  toolCalls.value.some((toolCall) => !isTerminalToolStatus(toolCall.status)),
);
const isRunning = computed(() => props.busy && (props.run == null || isActiveRunStatus(props.run?.status)));
const showProgress = computed(() =>
  isRunning.value
  && props.toolProgress.trim() !== ""
  && (!hasToolCalls.value || hasActiveToolCalls.value),
);
const showSummaryCard = computed(() => hasMultipleToolCalls.value);
const showExpandedToolCalls = computed(() =>
  hasToolCalls.value && (!showSummaryCard.value || props.summaryExpanded),
);
const summaryStatus = computed(() => {
  if (hasActiveToolCalls.value) return "RUNNING";
  const runStatus = normalizedDisplayStatus(props.run?.status);
  if (runStatus === "FAILED" || runStatus === "TIMED_OUT" || runStatus === "CANCELLED" || runStatus === "DENIED") {
    return runStatus;
  }
  return "COMPLETED";
});
const summaryTitle = computed(() => `调用了 ${toolCalls.value.length} 个工具`);
const summaryHint = computed(() => {
  const terminalMessage = runTerminalMessage(props.run);
  if (terminalMessage) return terminalMessage;
  return props.summaryExpanded ? "收起本轮工具调用轨迹" : "展开查看本轮工具调用轨迹";
});
const toolVisualizationMap = computed(() => {
  const visualizations = new Map<string, ADKToolVisualizationModel>();
  for (const toolCall of toolCalls.value) {
    if (toolCall.output === undefined) continue;
    const visualization = buildADKToolVisualization(toolCall.toolName, toolCall.output);
    if (visualization) {
      visualizations.set(toolCall.id, visualization);
    }
  }
  return visualizations;
});

function isTerminalToolStatus(status: string | undefined): boolean {
  const normalized = (status ?? "").trim().toUpperCase();
  return normalized === "SUCCEEDED"
    || normalized === "COMPLETED"
    || normalized === "FAILED"
    || normalized === "TIMED_OUT"
    || normalized === "DENIED"
    || normalized === "CANCELLED";
}

function preview(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
}

function toolVisualization(toolCall: ADKRun["toolCalls"][number]) {
  return toolVisualizationMap.value.get(toolCall.id);
}

function durationLabel(durationMs: number | undefined): string {
  if (durationMs == null || Number.isNaN(durationMs)) return "";
  if (durationMs < 1000) return `${durationMs} ms`;
  return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 1 : 0)} s`;
}

function isToolExpanded(toolCallId: string): boolean {
  return props.expandedToolCallIds.includes(toolCallId);
}

function toggleSummary(): void {
  emit("update:summaryExpanded", !props.summaryExpanded);
}

function toggleTool(toolCallId: string): void {
  const ids = new Set(props.expandedToolCallIds);
  if (ids.has(toolCallId)) {
    ids.delete(toolCallId);
  } else {
    ids.add(toolCallId);
  }
  emit("update:expandedToolCallIds", Array.from(ids));
}
</script>

<template>
  <div v-if="run || toolProgress.trim() !== ''" class="adk-run-trace" :class="{ 'adk-run-trace--compact': compact }">
    <div v-if="showProgress" class="adk-run-trace-card adk-run-trace-card--progress">
      <span class="adk-run-spinner" />
      <span class="adk-run-trace-card__main">
        <span class="adk-run-trace-card__title">{{ toolProgress }}</span>
        <span v-if="run?.status" class="adk-run-trace-card__meta">
          <span class="adk-status-pill" :class="`is-${runStatusTone(run.status)}`">
            {{ formatGenericStatusLabel(run.status) }}
          </span>
        </span>
      </span>
    </div>

    <button
      v-if="showSummaryCard"
      type="button"
      class="adk-run-trace-card adk-run-trace-card--summary"
      @click="toggleSummary"
    >
      <span class="adk-run-trace-card__main">
        <span class="adk-run-trace-card__title">{{ summaryTitle }}</span>
        <span class="adk-run-trace-card__meta">
          <span class="adk-status-pill" :class="`is-${runStatusTone(summaryStatus)}`">
            {{ formatGenericStatusLabel(summaryStatus) }}
          </span>
          <span class="adk-run-trace-card__hint">{{ summaryHint }}</span>
        </span>
      </span>
      <span class="adk-run-trace-card__chevron">{{ summaryExpanded ? "-" : "+" }}</span>
    </button>

    <div v-if="showExpandedToolCalls" class="adk-run-trace-tools">
      <div class="adk-run-trace-list">
        <div
          v-for="(toolCall, index) in toolCalls"
          :key="toolCall.id"
          class="adk-run-trace-list-item"
        >
          <button
            type="button"
            class="adk-run-trace-card adk-run-trace-card--tool"
            @click="toggleTool(toolCall.id)"
          >
            <span class="adk-run-trace-card__main">
              <span class="adk-run-trace-card__title">
                <span class="adk-run-trace-card__index">#{{ index + 1 }}</span>
                {{ toolCall.toolName }}
              </span>
              <span class="adk-run-trace-card__meta">
                <span class="adk-status-pill" :class="`is-${runStatusTone(normalizedDisplayStatus(toolCall.status))}`">
                  {{ formatGenericStatusLabel(normalizedDisplayStatus(toolCall.status)) }}
                </span>
                <span v-if="durationLabel(toolCall.durationMs)">{{ durationLabel(toolCall.durationMs) }}</span>
                <span v-if="index === toolCalls.length - 1 && runTerminalMessage(run)">{{ runTerminalMessage(run) }}</span>
              </span>
            </span>
            <span class="adk-run-trace-card__chevron">{{ isToolExpanded(toolCall.id) ? "-" : "+" }}</span>
          </button>

          <div v-if="isToolExpanded(toolCall.id)" class="adk-run-trace-detail">
            <div class="adk-json-label">输入参数</div>
            <pre class="adk-json">{{ preview(toolCall.input) }}</pre>
            <template v-if="toolCall.output !== undefined">
              <div class="adk-json-label mt-2">输出结果</div>
              <ADKToolVisualization
                v-if="toolVisualization(toolCall)"
                :visualization="toolVisualization(toolCall)!"
              />
              <pre class="adk-json">{{ preview(toolCall.output) }}</pre>
            </template>
            <template v-if="toolCall.error">
              <div class="adk-json-label mt-2 adk-json-label--error">错误信息</div>
              <pre class="adk-json">{{ toolCall.error }}</pre>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
