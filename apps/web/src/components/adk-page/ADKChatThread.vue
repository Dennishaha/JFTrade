<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { ADKApproval, ADKToolDescriptor } from "@/types";

import { type ADKTimelineEntryState } from "@/composables/adk/adkTimeline";
import { groupTurnTraceEntries } from "@/composables/adk/adkTurnTraceGrouping";
import { useExternalLink } from "@/composables/shared/externalLink";
import ADKChildRunTrace from "../shared/ADKChildRunTrace.vue";
import ADKTurnTrace from "./ADKTurnTrace.vue";

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
    toolByName?: ((name: string) => ADKToolDescriptor | undefined) | undefined;
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
    toolByName: undefined,
  },
);

const emit = defineEmits<{
  "update:chatDraft": [value: string];
  "showOlderTimeline": [];
  "showNewerTimeline": [];
  "showLatestTimeline": [];
}>();
const { handleExternalLinkClick } = useExternalLink();

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
const errorAlertLines = computed(() =>
  props.errorMessage
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== ""),
);
const errorAlertTitle = computed(() => errorAlertLines.value[0] ?? "");
const errorAlertDetails = computed(() => errorAlertLines.value.slice(1));
const errorAlertExpanded = ref(false);
const canExpandErrorAlert = computed(() => errorAlertDetails.value.length > 0);

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

watch(
  () => props.errorMessage,
  () => {
    errorAlertExpanded.value = false;
  },
);

const threadItems = computed(() => groupTurnTraceEntries(props.timelineEntries));

function threadItemKey(item: (typeof threadItems.value)[number]): string {
  return item.type === "turn_trace" ? item.key : item.entry.id;
}

function handleMarkdownClick(event: MouseEvent): void {
  const link = (event.target as Element | null)?.closest("a[href]");
  if (!(link instanceof HTMLAnchorElement)) return;
  handleExternalLinkClick(event, link.getAttribute("href") || link.href);
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
          class="ma-1 cursor-pointer"
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

    <template v-for="item in threadItems" :key="threadItemKey(item)">
      <div v-if="item.type === 'turn_trace'" class="adk-msg adk-msg--assistant">
        <ADKTurnTrace
          :block="item"
          :active-run-id="activeRunId"
          :active-run-status="activeRunStatus"
          :has-blocking-run="hasBlockingRun"
          :compact="layout === 'mobile'"
          :tool-by-name="toolByName"
          :render-markdown="renderMarkdown"
          :preview="preview"
        />
      </div>

      <div v-else-if="item.entry.kind === 'user_message'" class="adk-msg adk-msg--user">
        <div class="adk-user-prompt-row">
          <div
            v-if="hasProcessedUserPrompt(item.entry)"
            class="adk-user-prompt-toggle"
            aria-label="用户提示词可观测切换"
          >
            <button
              type="button"
              :class="{ 'is-active': item.entry.userPromptVariant !== 'processed' }"
              @click="item.entry.userPromptVariant = 'original'"
            >
              原文
            </button>
            <button
              type="button"
              :class="{ 'is-active': item.entry.userPromptVariant === 'processed' }"
              @click="item.entry.userPromptVariant = 'processed'"
            >
              可观测
            </button>
          </div>
          <div :class="userBubbleClass(item.entry)">{{ userPromptText(item.entry) }}</div>
        </div>
      </div>

      <div v-else-if="item.entry.kind === 'context_notice'" class="adk-msg adk-msg--notice">
        <div :class="contextNoticeClass(item.entry)">
          <v-progress-linear
            v-if="item.entry.status === 'streaming'"
            indeterminate
            rounded
            color="primary"
            class="adk-notice-progress"
          />
          <v-icon v-else-if="item.entry.status === 'error'" size="13">
            fa-solid fa-circle-exclamation
          </v-icon>
          <v-icon v-else size="13">fa-solid fa-check</v-icon>
          <span>{{ item.entry.text ?? "" }}</span>
        </div>
      </div>

      <div v-else-if="item.entry.kind === 'assistant_reasoning'" class="adk-msg adk-msg--assistant">
        <div class="adk-reasoning">
          <button
            type="button"
            class="adk-reasoning-toggle"
            :aria-expanded="item.entry.reasoningExpanded ? 'true' : 'false'"
            @click="item.entry.reasoningExpanded = !item.entry.reasoningExpanded"
          >
            <v-icon size="12">
              {{ item.entry.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
            </v-icon>
            <span>{{ item.entry.reasoningExpanded ? "隐藏深度思考" : "查看深度思考" }}</span>
          </button>
          <div
            v-if="item.entry.reasoningExpanded"
            class="adk-bubble adk-bubble--assistant adk-reasoning-body"
          >
            {{ item.entry.text ?? "" }}
          </div>
        </div>
      </div>

      <div
        v-else-if="item.entry.kind === 'child_run_group' && item.entry.childRunItem"
        class="adk-msg adk-msg--assistant"
      >
        <ADKChildRunTrace
          :item="item.entry.childRunItem"
          :compact="layout === 'mobile'"
          variant="timeline"
        />
      </div>

      <template v-else-if="item.entry.kind === 'approval_group'" />

      <div v-else-if="item.entry.kind === 'input_request' && item.entry.inputRequest" class="adk-msg adk-msg--assistant">
        <div
          class="adk-input-request-notice"
          :class="{ 'is-pending': item.entry.inputRequest.status === 'PENDING' }"
        >
          <v-icon size="13">fa-regular fa-circle-question</v-icon>
          <div>
            <strong>{{ item.entry.inputRequest.title || "Agent 需要你的选择" }}</strong>
            <span>
              {{
                item.entry.inputRequest.status === "PENDING"
                  ? item.entry.inputRequest.questions.length > 1
                    ? `正在等待你的回答 · ${item.entry.inputRequest.questions.length} 个问题`
                    : "正在等待你的回答"
                  : item.entry.inputRequest.status === "ANSWERED"
                    ? "已收到你的回答，继续执行中"
                    : "该提问已取消"
              }}
            </span>
          </div>
        </div>
      </div>

      <div v-else class="adk-msg adk-msg--assistant">
        <div
          v-if="(item.entry.text ?? '').trim() !== ''"
          class="adk-bubble adk-bubble--assistant adk-markdown"
          @click="handleMarkdownClick"
          v-html="renderedMarkdown(item.entry)"
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
        <div class="adk-inline-alert__content">
          <div class="adk-inline-alert__summary">
            <strong v-if="errorAlertTitle" class="adk-inline-alert__title">
              {{ errorAlertTitle }}
            </strong>
            <button
              v-if="canExpandErrorAlert"
              type="button"
              class="adk-inline-alert__toggle"
              @click="errorAlertExpanded = !errorAlertExpanded"
            >
              {{ errorAlertExpanded ? "收起" : "展开" }}
            </button>
          </div>
          <div v-if="errorAlertExpanded" class="adk-inline-alert__details">
            <span
              v-for="(line, index) in errorAlertDetails"
              :key="`${index}:${line}`"
              class="adk-inline-alert__detail"
            >
              {{ line }}
            </span>
          </div>
        </div>
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
