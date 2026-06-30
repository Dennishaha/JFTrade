<script setup lang="ts">
import { computed, watch } from "vue";

import type { ADKApproval, ADKRun, ADKToolDescriptor } from "@/contracts";

import {
  buildTimelineRun,
  type ADKTimelineEntryState,
} from "../../composables/adkTimeline";
import ADKRunTrace from "../shared/ADKRunTrace.vue";

const props = withDefaults(
  defineProps<{
    layout?: "desktop" | "mobile";
    activeRunId?: string;
    activeRunStatus?: string;
    hasBlockingRun?: boolean;
    timelineEntries: ADKTimelineEntryState[];
    sendingChat: boolean;
    activityIndicator: "idle" | "typing" | "child_finished";
    errorMessage: string;
    approvalsBusy: boolean;
    suggestions: string[];
    emptyStateTitle: string;
    emptyStateHint: string;
    emptyStateProviderHint?: string;
    timelineTotal?: number;
    timelineWindowStart?: number;
    timelineWindowEnd?: number;
    timelineAtLatest?: boolean;
    approvalTool: (approval: ADKApproval) => ADKToolDescriptor | undefined;
    clearErrorMessage: () => void;
    preview: (value: unknown) => string;
    renderMarkdown: (content: string) => string;
    resolveApprovalGroup: (
      approvals: ADKApproval[],
      approved: boolean,
    ) => void | Promise<void>;
    resolveApproval: (
      approval: ADKApproval,
      approved: boolean,
    ) => void | Promise<void>;
  }>(),
  {
    layout: "desktop",
    activeRunId: "",
    activeRunStatus: "",
    hasBlockingRun: false,
    emptyStateProviderHint: "",
    timelineTotal: 0,
    timelineWindowStart: 0,
    timelineWindowEnd: 0,
    timelineAtLatest: true,
  },
);

const emit = defineEmits<{
  "update:chatDraft": [value: string];
  "showOlderTimeline": [];
  "showNewerTimeline": [];
  "showLatestTimeline": [];
}>();

const threadClass = computed(() => ({
  "adk-chat-thread": true,
  "adk-chat-thread--mobile": props.layout === "mobile",
  "adk-chat-thread--desktop": props.layout === "desktop",
}));

const emptyClass = computed(() => ({
  "adk-empty": true,
  "adk-empty--mobile": props.layout === "mobile",
}));
const hasTimelineWindow = computed(
  () => props.timelineTotal > props.timelineEntries.length,
);
const timelineWindowLabel = computed(() => {
  if (!hasTimelineWindow.value || props.timelineEntries.length === 0) return "";
  return `${props.timelineWindowStart + 1}-${props.timelineWindowEnd} / ${props.timelineTotal}`;
});
const canShowOlderTimeline = computed(
  () => hasTimelineWindow.value && props.timelineWindowStart > 0,
);
const canShowNewerTimeline = computed(
  () => hasTimelineWindow.value && props.timelineWindowEnd < props.timelineTotal,
);

const markdownCache = new Map<
  string,
  { renderMarkdown: (content: string) => string; text: string; html: string }
>();

watch(
  () => props.timelineEntries.map((entry) => entry.id),
  (entryIds) => {
    const retainedIds = new Set(entryIds);
    for (const key of markdownCache.keys()) {
      if (!retainedIds.has(key)) {
        markdownCache.delete(key);
      }
    }
  },
  { flush: "post" },
);

function isEntryActiveRun(entry: ADKTimelineEntryState): boolean {
  return !!entry.runId && props.hasBlockingRun && entry.runId === props.activeRunId;
}

function entryToolRun(entry: ADKTimelineEntryState): ADKRun {
  const run = buildTimelineRun(entry);
  const toolCalls = run.toolCalls ?? [];
  if (!isEntryActiveRun(entry)) {
    return { ...run, toolCalls };
  }
  return {
    ...run,
    status: props.activeRunStatus || run.status,
    toolCalls: toolCalls.map((toolCall) => {
      if (
        props.activeRunStatus === "RUNNING" &&
        (toolCall.status === "PENDING_APPROVAL" || toolCall.status === "PENDING")
      ) {
        return { ...toolCall, status: "RUNNING" };
      }
      return toolCall;
    }),
  };
}

function entryToolProgress(entry: ADKTimelineEntryState): string {
  if (isEntryActiveRun(entry)) {
    if (props.activeRunStatus === "PENDING_APPROVAL") {
      return "等待审批...";
    }
    if (props.activeRunStatus === "PENDING") {
      return "等待执行...";
    }
    return "工具执行中...";
  }
  return entry.status === "streaming" ? "工具执行中..." : "";
}

function entryToolBusy(entry: ADKTimelineEntryState): boolean {
  return (
    isEntryActiveRun(entry) ||
    (props.timelineAtLatest &&
      props.sendingChat &&
      props.timelineEntries[props.timelineEntries.length - 1] === entry)
  );
}

function contextNoticeClass(entry: ADKTimelineEntryState): Record<string, boolean> {
  return {
    "adk-context-notice": true,
    "is-streaming": entry.status === "streaming",
    "is-error": entry.status === "error",
  };
}

function hasProcessedUserPrompt(entry: ADKTimelineEntryState): boolean {
  const original = String(entry.originalText ?? entry.text ?? "").trim();
  const processed = String(entry.processedText ?? "").trim();
  return processed !== "" && processed !== original;
}

function userPromptText(entry: ADKTimelineEntryState): string {
  if (entry.userPromptVariant === "processed" && hasProcessedUserPrompt(entry)) {
    return entry.processedText ?? "";
  }
  return entry.originalText ?? entry.text ?? "";
}

function userBubbleClass(entry: ADKTimelineEntryState): Record<string, boolean> {
  return {
    "adk-bubble": true,
    "adk-bubble--user": true,
    "adk-bubble--user-processed": entry.userPromptVariant === "processed",
  };
}

function renderedMarkdown(entry: ADKTimelineEntryState): string {
  const text = entry.text ?? "";
  const key = entry.id || `${entry.sequence ?? ""}:${entry.createdAt ?? ""}`;
  const cached = markdownCache.get(key);
  if (
    cached &&
    cached.text === text &&
    cached.renderMarkdown === props.renderMarkdown
  ) {
    return cached.html;
  }
  const html = props.renderMarkdown(text);
  markdownCache.set(key, { renderMarkdown: props.renderMarkdown, text, html });
  return html;
}

function showOlderTimeline(): void {
  emit("showOlderTimeline");
}

function showNewerTimeline(): void {
  emit("showNewerTimeline");
}

function showLatestTimeline(): void {
  emit("showLatestTimeline");
}
</script>

<template>
  <div :class="threadClass">
    <div v-if="timelineEntries.length === 0 && !sendingChat" :class="emptyClass">
      <v-icon size="52" class="adk-empty-icon">fa-solid fa-robot</v-icon>
      <p class="adk-empty-title">{{ emptyStateTitle }}</p>
      <p class="adk-empty-hint">{{ emptyStateHint }}</p>
      <p v-if="emptyStateProviderHint" class="adk-empty-hint">{{ emptyStateProviderHint }}</p>
      <div v-if="suggestions.length > 0" class="adk-suggestions">
        <v-chip
          v-for="hint in suggestions"
          :key="hint"
          size="small"
          variant="outlined"
          class="ma-1"
          style="cursor: pointer"
          @click="$emit('update:chatDraft', hint)"
        >
          {{ hint }}
        </v-chip>
      </div>
    </div>

    <div
      v-if="hasTimelineWindow"
      class="adk-timeline-window"
      aria-label="时间线窗口"
    >
      <button
        type="button"
        :disabled="!canShowOlderTimeline"
        @click="showOlderTimeline"
      >
        更早
      </button>
      <span>{{ timelineWindowLabel }}</span>
      <button
        type="button"
        :disabled="!canShowNewerTimeline"
        @click="showNewerTimeline"
      >
        更新
      </button>
      <button
        v-if="!timelineAtLatest"
        type="button"
        @click="showLatestTimeline"
      >
        最新
      </button>
    </div>

    <template v-for="entry in timelineEntries" :key="entry.id">
      <div v-if="entry.kind === 'user_message'" class="adk-msg adk-msg--user">
        <div class="adk-user-prompt-row">
          <div
            v-if="hasProcessedUserPrompt(entry)"
            class="adk-user-prompt-toggle"
            aria-label="用户提示词可观测切换"
          >
            <button
              type="button"
              :class="{ 'is-active': entry.userPromptVariant !== 'processed' }"
              @click="entry.userPromptVariant = 'original'"
            >
              原文
            </button>
            <button
              type="button"
              :class="{ 'is-active': entry.userPromptVariant === 'processed' }"
              @click="entry.userPromptVariant = 'processed'"
            >
              可观测
            </button>
          </div>
          <div :class="userBubbleClass(entry)">{{ userPromptText(entry) }}</div>
        </div>
      </div>

      <div v-else-if="entry.kind === 'context_notice'" class="adk-msg adk-msg--notice">
        <div :class="contextNoticeClass(entry)">
          <v-progress-linear
            v-if="entry.status === 'streaming'"
            indeterminate
            rounded
            color="primary"
            class="adk-notice-progress"
          />
          <v-icon v-else-if="entry.status === 'error'" size="13">
            fa-solid fa-circle-exclamation
          </v-icon>
          <v-icon v-else size="13">fa-solid fa-check</v-icon>
          <span>{{ entry.text ?? "" }}</span>
        </div>
      </div>

      <div v-else-if="entry.kind === 'assistant_reasoning'" class="adk-msg adk-msg--assistant">
        <div class="adk-reasoning">
          <button
            type="button"
            class="adk-reasoning-toggle"
            :aria-expanded="entry.reasoningExpanded ? 'true' : 'false'"
            @click="entry.reasoningExpanded = !entry.reasoningExpanded"
          >
            <v-icon size="12">
              {{ entry.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
            </v-icon>
            <span>{{ entry.reasoningExpanded ? "隐藏深度思考" : "查看深度思考" }}</span>
          </button>
          <div
            v-if="entry.reasoningExpanded"
            class="adk-bubble adk-bubble--assistant adk-reasoning-body"
          >
            {{ entry.text ?? "" }}
          </div>
        </div>
      </div>

      <div v-else-if="entry.kind === 'tool_group'" class="adk-msg adk-msg--assistant">
        <ADKRunTrace
          :run="entryToolRun(entry)"
          :tool-progress="entryToolProgress(entry)"
          :busy="entryToolBusy(entry)"
          :compact="layout === 'mobile'"
          :summary-expanded="entry.toolSummaryExpanded"
          :expanded-tool-call-ids="entry.expandedToolCallIds"
          @update:summary-expanded="entry.toolSummaryExpanded = $event"
          @update:expanded-tool-call-ids="entry.expandedToolCallIds = $event"
        />
      </div>

      <template v-else-if="entry.kind === 'approval_group'" />

      <div v-else class="adk-msg adk-msg--assistant">
        <div
          v-if="(entry.text ?? '').trim() !== ''"
          class="adk-bubble adk-bubble--assistant adk-markdown"
          v-html="renderedMarkdown(entry)"
        />
      </div>
    </template>

    <div v-if="errorMessage" class="adk-msg adk-msg--assistant adk-msg--notice">
      <v-alert
        type="warning"
        variant="tonal"
        density="compact"
        closable
        class="adk-inline-alert"
        @click:close="clearErrorMessage"
      >
        {{ errorMessage }}
      </v-alert>
    </div>

    <div v-if="activityIndicator === 'typing'" class="adk-msg adk-msg--assistant">
      <div class="adk-typing">
        <span class="adk-dot" />
        <span class="adk-dot" />
        <span class="adk-dot" />
      </div>
    </div>
    <div v-else-if="activityIndicator === 'child_finished'" class="adk-msg adk-msg--assistant">
      <div class="adk-child-finished-status">
        <v-icon size="13">fa-solid fa-circle-check</v-icon>
        <span>子智能体已结束，主智能体继续处理中</span>
      </div>
    </div>

    <div
      v-if="hasTimelineWindow && !timelineAtLatest"
      class="adk-timeline-window adk-timeline-window--bottom"
      aria-label="时间线窗口底部导航"
    >
      <button type="button" @click="showLatestTimeline">最新</button>
      <span>{{ timelineWindowLabel }}</span>
    </div>
  </div>
</template>
