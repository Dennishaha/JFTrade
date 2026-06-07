<script setup lang="ts">
import { computed } from "vue";

import type { ADKRun } from "@jftrade/ui-contracts";

import { formatGenericStatusLabel } from "../../composables/consoleDataFormatting";
import {
  isActiveRunStatus,
  normalizedDisplayStatus,
  runStatusTone,
  runTerminalMessage,
} from "../../composables/adkChatPresentation";

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
const hasActiveToolCalls = computed(() =>
  toolCalls.value.some((toolCall) => !isTerminalToolStatus(toolCall.status)),
);
const effectiveSummaryStatus = computed(() => deriveSummaryStatus(props.run));
const isRunning = computed(() => props.busy && (props.run == null || isActiveRunStatus(props.run?.status)));
const showProgress = computed(() =>
  isRunning.value
  && props.toolProgress.trim() !== ""
  && (!hasToolCalls.value || hasActiveToolCalls.value),
);
const showSummary = computed(() => !showProgress.value && hasToolCalls.value);
const useSingleToolSummary = computed(() => showSummary.value && toolCalls.value.length === 1);

function isTerminalToolStatus(status: string | undefined): boolean {
  const normalized = (status ?? "").trim().toUpperCase();
  return normalized === "SUCCEEDED"
    || normalized === "COMPLETED"
    || normalized === "FAILED"
    || normalized === "TIMED_OUT"
    || normalized === "DENIED"
    || normalized === "CANCELLED";
}

function deriveSummaryStatus(run: ADKRun | undefined): string | undefined {
  const calls = run?.toolCalls ?? [];
  if (calls.length === 0) return normalizedDisplayStatus(run?.status);
  if (calls.some((toolCall) => (toolCall.status ?? "").trim().toUpperCase() === "PENDING_APPROVAL")) return "PENDING_APPROVAL";
  if (calls.some((toolCall) => {
    const status = (toolCall.status ?? "").trim().toUpperCase();
    return status === "FAILED" || status === "TIMED_OUT" || status === "DENIED";
  })) {
    return "FAILED";
  }
  if (calls.some((toolCall) => {
    const status = (toolCall.status ?? "").trim().toUpperCase();
    return status === "" || status === "RUNNING" || status === "PENDING";
  })) {
    return "RUNNING";
  }
  if (calls.every((toolCall) => (toolCall.status ?? "").trim().toUpperCase() === "CANCELLED")) return "CANCELLED";
  if (calls.every((toolCall) => {
    const status = (toolCall.status ?? "").trim().toUpperCase();
    return status === "SUCCEEDED" || status === "COMPLETED";
  })) {
    return "COMPLETED";
  }
  if (calls.some((toolCall) => (toolCall.status ?? "").trim().toUpperCase() === "CANCELLED")) {
    return "CANCELLED";
  }
  return normalizedDisplayStatus(run?.status);
}

function preview(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
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

    <div v-else-if="useSingleToolSummary" class="adk-run-trace-tools">
      <button
        type="button"
        class="adk-run-trace-card adk-run-trace-card--tool"
        @click="toggleTool(toolCalls[0]!.id)"
      >
          <span class="adk-run-trace-card__main">
          <span class="adk-run-trace-card__title">{{ toolCalls[0]!.toolName }}</span>
          <span class="adk-run-trace-card__meta">
            <span class="adk-status-pill" :class="`is-${runStatusTone(normalizedDisplayStatus(toolCalls[0]!.status))}`">
              {{ formatGenericStatusLabel(normalizedDisplayStatus(toolCalls[0]!.status)) }}
            </span>
            <span v-if="durationLabel(toolCalls[0]!.durationMs)">{{ durationLabel(toolCalls[0]!.durationMs) }}</span>
            <span v-if="runTerminalMessage(run)">{{ runTerminalMessage(run) }}</span>
          </span>
        </span>
        <span class="adk-run-trace-card__chevron">{{ isToolExpanded(toolCalls[0]!.id) ? "−" : "+" }}</span>
      </button>

      <div v-if="isToolExpanded(toolCalls[0]!.id)" class="adk-run-trace-detail">
        <div class="adk-json-label">输入参数</div>
        <pre class="adk-json">{{ preview(toolCalls[0]!.input) }}</pre>
        <template v-if="toolCalls[0]!.output !== undefined">
          <div class="adk-json-label mt-2">输出结果</div>
          <pre class="adk-json">{{ preview(toolCalls[0]!.output) }}</pre>
        </template>
        <template v-if="toolCalls[0]!.error">
          <div class="adk-json-label mt-2 adk-json-label--error">错误信息</div>
          <pre class="adk-json">{{ toolCalls[0]!.error }}</pre>
        </template>
      </div>
    </div>

    <div v-else-if="showSummary" class="adk-run-trace-tools">
      <button
        type="button"
        class="adk-run-trace-card adk-run-trace-card--summary"
        @click="toggleSummary"
      >
        <span class="adk-run-trace-card__main">
          <span class="adk-run-trace-card__title">已调用工具 {{ toolCalls.length }} 个</span>
          <span class="adk-run-trace-card__meta">
            <span v-if="effectiveSummaryStatus" class="adk-status-pill" :class="`is-${runStatusTone(effectiveSummaryStatus)}`">
              {{ formatGenericStatusLabel(effectiveSummaryStatus) }}
            </span>
            <span>{{ runTerminalMessage(run) || "展开查看本轮工具调用轨迹" }}</span>
          </span>
        </span>
        <span class="adk-run-trace-card__chevron">{{ summaryExpanded ? "−" : "+" }}</span>
      </button>

      <div v-if="summaryExpanded" class="adk-run-trace-list">
        <div
          v-for="toolCall in toolCalls"
          :key="toolCall.id"
          class="adk-run-trace-list-item"
        >
          <button
            type="button"
            class="adk-run-trace-card adk-run-trace-card--tool"
            @click="toggleTool(toolCall.id)"
          >
            <span class="adk-run-trace-card__main">
              <span class="adk-run-trace-card__title">{{ toolCall.toolName }}</span>
              <span class="adk-run-trace-card__meta">
                <span class="adk-status-pill" :class="`is-${runStatusTone(normalizedDisplayStatus(toolCall.status))}`">
                  {{ formatGenericStatusLabel(normalizedDisplayStatus(toolCall.status)) }}
                </span>
                <span v-if="durationLabel(toolCall.durationMs)">{{ durationLabel(toolCall.durationMs) }}</span>
              </span>
            </span>
            <span class="adk-run-trace-card__chevron">{{ isToolExpanded(toolCall.id) ? "−" : "+" }}</span>
          </button>

          <div v-if="isToolExpanded(toolCall.id)" class="adk-run-trace-detail">
            <div class="adk-json-label">输入参数</div>
            <pre class="adk-json">{{ preview(toolCall.input) }}</pre>
            <template v-if="toolCall.output !== undefined">
              <div class="adk-json-label mt-2">输出结果</div>
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
