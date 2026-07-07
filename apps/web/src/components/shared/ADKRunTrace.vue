<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { ADKRun } from "@/contracts";

import type { ADKToolVisualization as ADKToolVisualizationModel } from "../../composables/adkToolVisualizations";
import { buildADKToolVisualization } from "../../composables/adkToolVisualizations";
import { formatGenericStatusLabel } from "../../composables/consoleDataFormatting";
import {
  firstFailedToolCall,
  isActiveRunStatus,
  normalizedDisplayStatus,
  runErrorSummary,
  runStatusTone,
  runTerminalMessage,
  toolCallErrorSummary,
} from "../../composables/adkChatPresentation";
import ADKToolVisualization from "./ADKToolVisualization.vue";

const TOOL_CALL_RENDER_WINDOW = 80;

const props = withDefaults(
  defineProps<{
    run?: ADKRun | undefined;
    toolProgress?: string | undefined;
    busy?: boolean;
    compact?: boolean;
    variant?: "panel" | "timeline";
    summaryExpanded?: boolean | undefined;
    expandedToolCallIds?: string[] | undefined;
  }>(),
  {
    toolProgress: "",
    busy: false,
    compact: false,
    variant: "panel",
    summaryExpanded: false,
    expandedToolCallIds: () => [],
  },
);

const emit = defineEmits<{
  "update:summaryExpanded": [value: boolean];
  "update:expandedToolCallIds": [value: string[]];
}>();

const toolCalls = computed(() => props.run?.toolCalls ?? []);
const hasToolCalls = computed(() => toolCalls.value.length > 0);
const hasMultipleToolCalls = computed(() => toolCalls.value.length > 1);
const toolCallPage = ref(1);
const failedToolCall = computed(() => firstFailedToolCall(props.run));
const hasActiveToolCalls = computed(() =>
  toolCalls.value.some((toolCall) => !isTerminalToolStatus(toolCall.status)),
);
const isRunning = computed(
  () =>
    props.busy && (props.run == null || isActiveRunStatus(props.run?.status)),
);
const showProgress = computed(
  () =>
    isRunning.value &&
    props.toolProgress.trim() !== "" &&
    (!hasToolCalls.value || hasActiveToolCalls.value),
);
const showSummaryCard = computed(() => hasMultipleToolCalls.value);
const showExpandedToolCalls = computed(
  () => hasToolCalls.value && (!showSummaryCard.value || props.summaryExpanded),
);
const summaryStatus = computed(() => {
  if (hasActiveToolCalls.value) return "RUNNING";
  const runStatus = normalizedDisplayStatus(props.run?.status);
  if (
    runStatus === "FAILED" ||
    runStatus === "TIMED_OUT" ||
    runStatus === "CANCELLED" ||
    runStatus === "DENIED"
  ) {
    return runStatus;
  }
  return "COMPLETED";
});
const summaryTitle = computed(() => `调用了 ${toolCalls.value.length} 个工具`);
const toolCallPageCount = computed(() =>
  Math.max(1, Math.ceil(toolCalls.value.length / TOOL_CALL_RENDER_WINDOW)),
);
const visibleToolCallStart = computed(() =>
  Math.min(
    Math.max(0, (toolCallPage.value - 1) * TOOL_CALL_RENDER_WINDOW),
    Math.max(0, toolCalls.value.length - 1),
  ),
);
const visibleToolCallEnd = computed(() =>
  Math.min(toolCalls.value.length, visibleToolCallStart.value + TOOL_CALL_RENDER_WINDOW),
);
const visibleToolCalls = computed(() =>
  toolCalls.value.slice(visibleToolCallStart.value, visibleToolCallEnd.value),
);
const hasToolCallWindow = computed(
  () => toolCalls.value.length > TOOL_CALL_RENDER_WINDOW,
);
const toolCallWindowLabel = computed(() =>
  hasToolCallWindow.value
    ? `${visibleToolCallStart.value + 1}-${visibleToolCallEnd.value} / ${toolCalls.value.length}`
    : "",
);
const summaryHint = computed(() => {
  if (failedToolCall.value) {
    const errorSummary = toolCallErrorSummary(failedToolCall.value);
    return `${failedToolCall.value.toolName}: ${truncate(errorSummary, 140)}`;
  }
  const terminalMessage = runTerminalMessage(props.run);
  if (terminalMessage) return terminalMessage;
  return props.summaryExpanded
    ? "Collapse this tool trace"
    : "Expand this tool trace";
});
const showWorkflowMeta = computed(
  () =>
    props.run?.workMode &&
    props.run.workMode !== "chat" &&
    (props.run.workflowStatus ||
      props.run.objective ||
      (props.run.childRunIds?.length ?? 0) > 0),
);
const workflowErrorSummary = computed(() => runErrorSummary(props.run));
const workflowModeLabel = computed(() => {
  switch (props.run?.workMode) {
    case "loop":
      return "目标模式";
    default:
      return "工作流";
  }
});
const toolVisualizationCache = new Map<
  string,
  { output: unknown; visualization: ADKToolVisualizationModel | null }
>();

watch(
  toolCalls,
  (calls) => {
    if (toolCallPage.value > toolCallPageCount.value) {
      toolCallPage.value = toolCallPageCount.value;
    }
    const retainedIds = new Set(calls.map((toolCall) => toolCall.id));
    for (const key of toolVisualizationCache.keys()) {
      if (!retainedIds.has(key)) {
        toolVisualizationCache.delete(key);
      }
    }
  },
  { flush: "post" },
);

function isTerminalToolStatus(status: string | undefined): boolean {
  const normalized = (status ?? "").trim().toUpperCase();
  return (
    normalized === "SUCCEEDED" ||
    normalized === "COMPLETED" ||
    normalized === "FAILED" ||
    normalized === "TIMED_OUT" ||
    normalized === "DENIED" ||
    normalized === "CANCELLED"
  );
}

function preview(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
}

function toolVisualization(toolCall: ADKRun["toolCalls"][number]) {
  if (!isToolExpanded(toolCall.id) || toolCall.output === undefined) {
    return null;
  }
  const cached = toolVisualizationCache.get(toolCall.id);
  if (cached && cached.output === toolCall.output) {
    return cached.visualization;
  }
  const visualization = buildADKToolVisualization(
    toolCall.toolName,
    toolCall.output,
  );
  toolVisualizationCache.set(toolCall.id, {
    output: toolCall.output,
    visualization,
  });
  return visualization;
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

function showPreviousToolCalls(): void {
  toolCallPage.value = Math.max(1, toolCallPage.value - 1);
}

function showNextToolCalls(): void {
  toolCallPage.value = Math.min(toolCallPageCount.value, toolCallPage.value + 1);
}

function collapsedToolHint(
  toolCall: ADKRun["toolCalls"][number],
  index: number,
): string {
  const toolError = toolCallErrorSummary(toolCall);
  if (toolError) {
    return truncate(toolError, 140);
  }
  if (index === toolCalls.value.length - 1) {
    return runTerminalMessage(props.run);
  }
  return "";
}

function truncate(value: string, maxLength: number): string {
  if (value.length <= maxLength) return value;
  return `${value.slice(0, Math.max(0, maxLength - 3))}...`;
}
</script>

<template>
  <div
    v-if="run || toolProgress.trim() !== ''"
    class="adk-run-trace"
    :class="{
      'adk-run-trace--compact': compact,
      'adk-run-trace--timeline': variant === 'timeline',
      'adk-run-trace--expanded': showExpandedToolCalls,
    }"
  >
    <div
      v-if="showProgress"
      class="adk-run-trace-card adk-run-trace-card--progress"
    >
      <span class="adk-run-spinner" />
      <span class="adk-run-trace-card__main">
        <span class="adk-run-trace-card__title">{{ toolProgress }}</span>
        <span v-if="run?.status" class="adk-run-trace-card__meta">
          <span
            class="adk-status-pill"
            :class="`is-${runStatusTone(run.status)}`"
          >
            {{ formatGenericStatusLabel(run.status) }}
          </span>
        </span>
      </span>
    </div>

    <div v-if="showWorkflowMeta" class="adk-run-trace-card adk-run-trace-card--tool">
      <span class="adk-run-trace-card__main">
        <span class="adk-run-trace-card__title">
          <span class="adk-run-trace-card__index">Agent</span>
          {{ workflowModeLabel }}
        </span>
        <span class="adk-run-trace-card__meta">
          <span
            v-if="run?.workflowStatus"
            class="adk-status-pill"
            :class="`is-${runStatusTone(run.workflowStatus)}`"
          >
            {{ formatGenericStatusLabel(run.workflowStatus) }}
          </span>
          <span v-if="run?.iteration">第 {{ run.iteration }} 轮</span>
          <span v-if="run?.childRunIds?.length">{{ run.childRunIds.length }} 个子智能体</span>
          <span v-if="workflowErrorSummary" class="adk-run-trace-card__error">
            {{ workflowErrorSummary.title }}
          </span>
          <span v-if="workflowErrorSummary?.code">
            {{ workflowErrorSummary.code }}
          </span>
          <span v-if="run?.objective">{{ truncate(run.objective, 120) }}</span>
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
          <span
            class="adk-status-pill"
            :class="`is-${runStatusTone(summaryStatus)}`"
          >
            {{ formatGenericStatusLabel(summaryStatus) }}
          </span>
          <span class="adk-run-trace-card__hint">{{ summaryHint }}</span>
        </span>
      </span>
      <span class="adk-run-trace-card__chevron">{{
        summaryExpanded ? "-" : "+"
      }}</span>
    </button>

    <div v-if="showExpandedToolCalls" class="adk-run-trace-tools">
      <div v-if="hasToolCallWindow" class="adk-run-trace-window">
        <button
          type="button"
          :disabled="toolCallPage <= 1"
          @click="showPreviousToolCalls"
        >
          上一组
        </button>
        <span>{{ toolCallWindowLabel }}</span>
        <button
          type="button"
          :disabled="toolCallPage >= toolCallPageCount"
          @click="showNextToolCalls"
        >
          下一组
        </button>
      </div>
      <div class="adk-run-trace-list">
        <div
          v-for="(toolCall, index) in visibleToolCalls"
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
                <span class="adk-run-trace-card__index">#{{ visibleToolCallStart + index + 1 }}</span>
                {{ toolCall.toolName }}
              </span>
              <span class="adk-run-trace-card__meta">
                <span
                  class="adk-status-pill"
                  :class="`is-${runStatusTone(normalizedDisplayStatus(toolCall.status))}`"
                >
                  {{
                    formatGenericStatusLabel(
                      normalizedDisplayStatus(toolCall.status),
                    )
                  }}
                </span>
                <span v-if="durationLabel(toolCall.durationMs)">{{
                  durationLabel(toolCall.durationMs)
                }}</span>
                <span v-if="collapsedToolHint(toolCall, visibleToolCallStart + index)">{{
                  collapsedToolHint(toolCall, visibleToolCallStart + index)
                }}</span>
              </span>
            </span>
            <span class="adk-run-trace-card__chevron">{{
              isToolExpanded(toolCall.id) ? "-" : "+"
            }}</span>
          </button>

          <div v-if="isToolExpanded(toolCall.id)" class="adk-run-trace-detail">
            <div class="adk-json-label">Input</div>
            <pre class="adk-json">{{ preview(toolCall.input) }}</pre>
            <template v-if="toolCall.output !== undefined">
              <div class="adk-json-label mt-2">Output</div>
              <ADKToolVisualization
                v-if="toolVisualization(toolCall)"
                :visualization="toolVisualization(toolCall)!"
              />
              <pre class="adk-json">{{ preview(toolCall.output) }}</pre>
            </template>
            <template v-if="toolCall.error">
              <div class="adk-json-label mt-2 adk-json-label--error">Error</div>
              <pre class="adk-json">{{ toolCall.error }}</pre>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
