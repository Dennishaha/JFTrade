<script setup lang="ts">
import { computed, onScopeDispose, ref, watch } from "vue";

import type { ADKToolCall, ADKToolDescriptor } from "@/types";

import { deriveToolGroupStatus, type ADKTimelineEntryState } from "@/composables/adk/adkTimeline";
import {
  isActiveRunStatus,
  normalizedDisplayStatus,
  runErrorSummary,
} from "@/composables/adk/adkChatPresentation";
import {
  classifyToolAction,
  formatTraceDuration,
  parseTraceTime,
  summarizeToolGroup,
  toolResultMeta,
  toolTraceRowLabel,
  truncateTraceText,
} from "@/composables/adk/adkToolTracePresentation";
import {
  turnTraceBlockRun,
  turnTraceElapsedMs,
  type ADKTurnTraceBlock,
} from "@/composables/adk/adkTurnTraceGrouping";
import {
  buildADKToolVisualization,
  type ADKToolVisualization as ADKToolVisualizationModel,
} from "@/composables/adk/adkToolVisualizations";
import { useExternalLink } from "@/composables/shared/externalLink";
import ADKToolVisualization from "../shared/ADKToolVisualization.vue";

const TOOL_ROW_RENDER_LIMIT = 20;

const props = withDefaults(
  defineProps<{
    block: ADKTurnTraceBlock;
    activeRunId?: string;
    activeRunStatus?: string;
    hasBlockingRun?: boolean;
    compact?: boolean;
    toolByName?: ((name: string) => ADKToolDescriptor | undefined) | undefined;
    renderMarkdown: (content: string) => string;
    preview: (value: unknown) => string;
  }>(),
  {
    activeRunId: "",
    activeRunStatus: "",
    hasBlockingRun: false,
    compact: false,
    toolByName: undefined,
  },
);

const { handleExternalLinkClick } = useExternalLink();

const firstEntry = computed(() => props.block.entries[0]);
const run = computed(() => turnTraceBlockRun(props.block));
const isActiveRun = computed(
  () => props.hasBlockingRun && props.block.runId === props.activeRunId,
);

const allToolCalls = computed(() =>
  props.block.entries.flatMap((entry) => entry.toolCalls ?? []),
);

const isTail = computed(
  () =>
    props.block.segmentPosition === "only" ||
    props.block.segmentPosition === "last",
);

const blockStatus = computed(() => {
  if (isActiveRun.value && isTail.value && props.activeRunStatus.trim() !== "") {
    return props.activeRunStatus;
  }
  if (isActiveRun.value && !isTail.value) {
    // Earlier segments of a still-running run are finished work: derive their
    // status from their own tool calls instead of mirroring the run status.
    return deriveToolGroupStatus(allToolCalls.value);
  }
  const runStatus = normalizedDisplayStatus(run.value?.status);
  if (runStatus && runStatus.trim() !== "") return runStatus;
  return deriveToolGroupStatus(allToolCalls.value);
});

const isActive = computed(
  () => isActiveRun.value && isTail.value && isActiveRunStatus(blockStatus.value),
);

const expanded = computed(
  () => firstEntry.value?.turnTraceExpanded ?? isActive.value,
);

const nowMs = ref(Date.now());
// Sub-second tick so live durations (e.g. 思考过程 · 0.3s 起) advance smoothly.
const TICK_INTERVAL_MS = 250;
let tickTimer: ReturnType<typeof setInterval> | undefined;

watch(
  isActive,
  (active) => {
    if (active && tickTimer === undefined) {
      nowMs.value = Date.now();
      tickTimer = setInterval(() => {
        nowMs.value = Date.now();
      }, TICK_INTERVAL_MS);
    }
    if (!active && tickTimer !== undefined) {
      clearInterval(tickTimer);
      tickTimer = undefined;
    }
  },
  { immediate: true },
);

onScopeDispose(() => {
  if (tickTimer !== undefined) clearInterval(tickTimer);
});

const elapsedMs = computed(() =>
  turnTraceElapsedMs({
    block: props.block,
    run: run.value,
    nowMs: nowMs.value,
    active: isActive.value,
  }),
);

const elapsedLabel = computed(() => formatTraceDuration(elapsedMs.value));

const headerLabel = computed(() => {
  const status = blockStatus.value;
  const suffix = elapsedLabel.value === "" ? "" : ` · ${elapsedLabel.value}`;
  if (status === "PENDING_APPROVAL") return `等待审批${suffix}`;
  if (status === "PENDING_INPUT") return `等待回答${suffix}`;
  if (isActive.value) return `正在工作${suffix}`;
  return elapsedLabel.value === "" ? "已工作" : `已工作 ${elapsedLabel.value}`;
});

const headerTone = computed(() => {
  if (isActive.value) return "running";
  switch (blockStatus.value) {
    case "FAILED":
    case "TIMED_OUT":
      return "error";
    case "CANCELLED":
    case "DENIED":
      return "muted";
    case "PENDING_APPROVAL":
    case "PENDING_INPUT":
    case "PAUSED":
      return "warning";
    default:
      return "success";
  }
});

const failureHint = computed(() => {
  if (headerTone.value !== "error") return "";
  const summary = runErrorSummary(run.value);
  return summary ? truncateTraceText(summary.title, 80) : "";
});

const showAllToolRows = ref<Set<string>>(new Set());
const toolVisualizationCache = new Map<
  string,
  { output: unknown; visualization: ADKToolVisualizationModel | null }
>();
const markdownCache = new Map<string, { text: string; html: string }>();

watch(
  () => props.block.entries.map((entry) => entry.id).join("|"),
  () => {
    const retainedIds = new Set(props.block.entries.map((entry) => entry.id));
    for (const key of markdownCache.keys()) {
      if (!retainedIds.has(key)) markdownCache.delete(key);
    }
    const retainedToolCallIds = new Set(
      props.block.entries.flatMap((entry) =>
        (entry.toolCalls ?? []).map((toolCall) => toolCall.id),
      ),
    );
    for (const key of toolVisualizationCache.keys()) {
      if (!retainedToolCallIds.has(key)) toolVisualizationCache.delete(key);
    }
  },
  { flush: "post" },
);

function toggleExpanded(): void {
  const entry = firstEntry.value;
  if (!entry) return;
  entry.turnTraceExpanded = !expanded.value;
}

function nextEntry(entry: ADKTimelineEntryState): ADKTimelineEntryState | undefined {
  const index = props.block.entries.findIndex((candidate) => candidate.id === entry.id);
  return index >= 0 ? props.block.entries[index + 1] : undefined;
}

function reasoningDurationLabel(entry: ADKTimelineEntryState): string {
  const start = parseTraceTime(entry.createdAt);
  if (start == null) return "";
  const following = nextEntry(entry);
  if (following) {
    const end = parseTraceTime(following.createdAt);
    if (end != null && end > start) return formatTraceDuration(end - start);
    return "";
  }
  if (isActive.value) {
    return formatTraceDuration(Math.max(0, nowMs.value - start));
  }
  const updated = parseTraceTime(entry.updatedAt);
  if (updated != null && updated > start) return formatTraceDuration(updated - start);
  return "";
}

function reasoningLabel(entry: ADKTimelineEntryState): string {
  const duration = reasoningDurationLabel(entry);
  return duration === "" ? "思考过程" : `思考过程 · ${duration}`;
}

function toggleReasoning(entry: ADKTimelineEntryState): void {
  entry.reasoningExpanded = !entry.reasoningExpanded;
}

function displayToolCalls(entry: ADKTimelineEntryState): ADKToolCall[] {
  const toolCalls = entry.toolCalls ?? [];
  if (!isActiveRun.value || !isTail.value || props.activeRunStatus !== "RUNNING") {
    return toolCalls;
  }
  return toolCalls.map((toolCall) => {
    if (toolCall.status === "PENDING_APPROVAL" || toolCall.status === "PENDING") {
      return { ...toolCall, status: "RUNNING" };
    }
    return toolCall;
  });
}

function groupSummary(entry: ADKTimelineEntryState) {
  return summarizeToolGroup(displayToolCalls(entry));
}

function groupProgress(entry: ADKTimelineEntryState): string {
  if (isActiveRun.value && isTail.value) {
    if (props.activeRunStatus === "PENDING_APPROVAL") return "等待审批...";
    if (props.activeRunStatus === "PENDING") return "等待执行...";
    return "工具执行中...";
  }
  return entry.status === "streaming" ? "工具执行中..." : "";
}

function groupSummaryLine(entry: ADKTimelineEntryState): string {
  const summary = groupSummary(entry);
  const progress = groupProgress(entry);
  const parts = [...summary.parts];
  if (progress !== "") {
    parts.push(progress);
  } else {
    const duration = formatTraceDuration(summary.durationMs);
    if (duration !== "") parts.push(duration);
  }
  return parts.join(" · ");
}

function groupStatusTone(entry: ADKTimelineEntryState): string {
  const status = groupSummary(entry).status;
  switch (status) {
    case "FAILED":
    case "TIMED_OUT":
      return "error";
    case "DENIED":
    case "CANCELLED":
      return "muted";
    case "PENDING_APPROVAL":
      return "warning";
    case "RUNNING":
      return "running";
    default:
      return "success";
  }
}

function visibleToolRows(entry: ADKTimelineEntryState): ADKToolCall[] {
  const toolCalls = displayToolCalls(entry);
  if (showAllToolRows.value.has(entry.id)) return toolCalls;
  return toolCalls.slice(0, TOOL_ROW_RENDER_LIMIT);
}

function hiddenToolRowCount(entry: ADKTimelineEntryState): number {
  if (showAllToolRows.value.has(entry.id)) return 0;
  return Math.max(0, (entry.toolCalls ?? []).length - TOOL_ROW_RENDER_LIMIT);
}

function showAllRows(entry: ADKTimelineEntryState): void {
  const next = new Set(showAllToolRows.value);
  next.add(entry.id);
  showAllToolRows.value = next;
}

function rowPresentation(toolCall: ADKToolCall) {
  const action = classifyToolAction(toolCall.toolName);
  const { label, argument } = toolTraceRowLabel(
    toolCall,
    props.toolByName?.(toolCall.toolName),
  );
  return { action, label, argument, meta: toolResultMeta(toolCall) };
}

function rowStatusTone(status: string | undefined): string {
  switch ((status ?? "").trim().toUpperCase()) {
    case "SUCCEEDED":
    case "COMPLETED":
      return "success";
    case "FAILED":
    case "TIMED_OUT":
      return "error";
    case "DENIED":
    case "CANCELLED":
      return "muted";
    case "PENDING_APPROVAL":
      return "warning";
    case "RUNNING":
    case "PENDING":
      return "running";
    default:
      return "success";
  }
}

function isToolExpanded(entry: ADKTimelineEntryState, toolCallId: string): boolean {
  return (entry.expandedToolCallIds ?? []).includes(toolCallId);
}

function toggleTool(entry: ADKTimelineEntryState, toolCallId: string): void {
  const ids = new Set(entry.expandedToolCallIds ?? []);
  if (ids.has(toolCallId)) {
    ids.delete(toolCallId);
  } else {
    ids.add(toolCallId);
  }
  entry.expandedToolCallIds = Array.from(ids);
}

function toolVisualization(toolCall: ADKToolCall) {
  if (toolCall.output === undefined) return null;
  const cached = toolVisualizationCache.get(toolCall.id);
  if (cached && cached.output === toolCall.output) return cached.visualization;
  const visualization = buildADKToolVisualization(toolCall.toolName, toolCall.output);
  toolVisualizationCache.set(toolCall.id, {
    output: toolCall.output,
    visualization,
  });
  return visualization;
}

function renderedBlockMarkdown(entry: ADKTimelineEntryState): string {
  const text = entry.text ?? "";
  const cached = markdownCache.get(entry.id);
  if (cached && cached.text === text) return cached.html;
  const html = props.renderMarkdown(text);
  markdownCache.set(entry.id, { text, html });
  return html;
}

function handleMarkdownClick(event: MouseEvent): void {
  const link = (event.target as Element | null)?.closest("a[href]");
  if (!(link instanceof HTMLAnchorElement)) return;
  handleExternalLinkClick(event, link.getAttribute("href") || link.href);
}
</script>

<template>
  <div
    v-if="block.entries.length > 0"
    class="adk-turn-trace"
    :class="{
      'adk-turn-trace--compact': compact,
      'adk-turn-trace--expanded': expanded,
    }"
  >
    <button
      type="button"
      class="adk-turn-trace__header"
      :aria-expanded="expanded ? 'true' : 'false'"
      @click="toggleExpanded"
    >
      <span class="adk-turn-trace__status" :class="`is-${headerTone}`">
        <span v-if="headerTone === 'running'" class="adk-run-spinner" />
        <v-icon v-else-if="headerTone === 'success'" size="12">fa-solid fa-check</v-icon>
        <v-icon v-else-if="headerTone === 'error'" size="12">fa-solid fa-xmark</v-icon>
        <v-icon v-else-if="headerTone === 'warning'" size="12">fa-solid fa-hand</v-icon>
        <v-icon v-else size="12">fa-solid fa-ban</v-icon>
      </span>
      <span class="adk-turn-trace__label">{{ headerLabel }}</span>
      <span v-if="failureHint" class="adk-turn-trace__hint">{{ failureHint }}</span>
      <v-icon size="11" class="adk-turn-trace__chevron">
        {{ expanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
      </v-icon>
    </button>

    <div v-if="expanded" class="adk-turn-trace__body">
      <template v-for="entry in block.entries" :key="entry.id">
        <div v-if="entry.kind === 'assistant_reasoning'" class="adk-trace-reasoning">
          <button
            type="button"
            class="adk-trace-reasoning__toggle"
            :aria-expanded="entry.reasoningExpanded ? 'true' : 'false'"
            @click="toggleReasoning(entry)"
          >
            <v-icon size="12" class="adk-trace-reasoning__icon">fa-regular fa-lightbulb</v-icon>
            <span>{{ reasoningLabel(entry) }}</span>
            <v-icon size="10">
              {{ entry.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
            </v-icon>
          </button>
          <div
            v-if="entry.reasoningExpanded"
            class="adk-trace-reasoning__body"
          >{{ entry.text ?? "" }}</div>
        </div>

        <div
          v-else-if="entry.kind === 'assistant_message' && (entry.text ?? '').trim() !== ''"
          class="adk-turn-trace__text adk-markdown"
          @click="handleMarkdownClick"
          v-html="renderedBlockMarkdown(entry)"
        />

        <div v-else-if="entry.kind === 'tool_group'" class="adk-trace-group">
          <div class="adk-trace-group__summary">
            <span class="adk-turn-trace__status" :class="`is-${groupStatusTone(entry)}`">
              <span v-if="groupStatusTone(entry) === 'running'" class="adk-run-spinner" />
              <v-icon v-else-if="groupStatusTone(entry) === 'success'" size="11">fa-solid fa-check</v-icon>
              <v-icon v-else-if="groupStatusTone(entry) === 'error'" size="11">fa-solid fa-xmark</v-icon>
              <v-icon v-else-if="groupStatusTone(entry) === 'warning'" size="11">fa-solid fa-hand</v-icon>
              <v-icon v-else size="11">fa-solid fa-ban</v-icon>
            </span>
            <span>{{ groupSummaryLine(entry) }}</span>
          </div>
          <div class="adk-trace-tools">
            <div
              v-for="toolCall in visibleToolRows(entry)"
              :key="toolCall.id"
              class="adk-trace-tool-item"
            >
              <button
                type="button"
                class="adk-trace-tool"
                :title="toolCall.toolName"
                :aria-expanded="isToolExpanded(entry, toolCall.id) ? 'true' : 'false'"
                @click="toggleTool(entry, toolCall.id)"
              >
                <v-icon size="12" class="adk-trace-tool__icon">
                  {{ rowPresentation(toolCall).action.icon }}
                </v-icon>
                <span class="adk-trace-tool__main">
                  <span class="adk-trace-tool__label">
                    {{ rowPresentation(toolCall).label }}
                  </span>
                  <code
                    v-if="rowPresentation(toolCall).argument"
                    class="adk-trace-tool__arg"
                  >{{ rowPresentation(toolCall).argument }}</code>
                </span>
                <span class="adk-trace-tool__meta">
                  <span v-if="rowPresentation(toolCall).meta">{{ rowPresentation(toolCall).meta }}</span>
                  <span v-if="formatTraceDuration(toolCall.durationMs)">{{
                    formatTraceDuration(toolCall.durationMs)
                  }}</span>
                  <span
                    class="adk-trace-tool__status"
                    :class="`is-${rowStatusTone(toolCall.status)}`"
                  >
                    <span v-if="rowStatusTone(toolCall.status) === 'running'" class="adk-run-spinner" />
                    <v-icon v-else-if="rowStatusTone(toolCall.status) === 'success'" size="11">fa-solid fa-check</v-icon>
                    <v-icon v-else-if="rowStatusTone(toolCall.status) === 'error'" size="11">fa-solid fa-xmark</v-icon>
                    <v-icon v-else-if="rowStatusTone(toolCall.status) === 'warning'" size="11">fa-solid fa-hand</v-icon>
                    <v-icon v-else size="11">fa-solid fa-ban</v-icon>
                  </span>
                </span>
              </button>
              <div
                v-if="isToolExpanded(entry, toolCall.id)"
                class="adk-trace-tool__detail"
              >
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
            <button
              v-if="hiddenToolRowCount(entry) > 0"
              type="button"
              class="adk-trace-more"
              @click="showAllRows(entry)"
            >
              展开剩余 {{ hiddenToolRowCount(entry) }} 条工具调用
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
