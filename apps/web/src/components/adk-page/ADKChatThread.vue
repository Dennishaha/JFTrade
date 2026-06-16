<script setup lang="ts">
import { computed } from "vue";

import type { ADKApproval, ADKRun, ADKToolDescriptor } from "@/contracts";

import {
  buildTimelineRun,
  type ADKTimelineEntryState,
} from "../../composables/adkTimeline";
import ADKRunTrace from "../shared/ADKRunTrace.vue";

const props = withDefaults(
  defineProps<{
    variant?: "page" | "dock";
    activeRunId?: string;
    activeRunStatus?: string;
    hasBlockingRun?: boolean;
    timelineEntries: ADKTimelineEntryState[];
    sendingChat: boolean;
    showTypingIndicator: boolean;
    errorMessage: string;
    approvalsBusy: boolean;
    suggestions: string[];
    emptyStateTitle: string;
    emptyStateHint: string;
    emptyStateProviderHint?: string;
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
    variant: "page",
    activeRunId: "",
    activeRunStatus: "",
    hasBlockingRun: false,
    emptyStateProviderHint: "",
  },
);

defineEmits<{
  "update:chatDraft": [value: string];
}>();

const threadClass = computed(() => ({
  "adk-chat-thread": true,
  "adk-chat-thread--dock": props.variant === "dock",
  "adk-chat-thread--page": props.variant === "page",
}));

const emptyClass = computed(() => ({
  "adk-empty": true,
  "adk-empty--dock": props.variant === "dock",
}));

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
    (props.sendingChat && props.timelineEntries[props.timelineEntries.length - 1] === entry)
  );
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

    <template v-for="entry in timelineEntries" :key="entry.id">
      <div v-if="entry.kind === 'user_message'" class="adk-msg adk-msg--user">
        <div class="adk-bubble adk-bubble--user">{{ entry.text ?? "" }}</div>
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
          :compact="variant === 'dock'"
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
          v-html="renderMarkdown(entry.text ?? '')"
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

    <div v-if="showTypingIndicator" class="adk-msg adk-msg--assistant">
      <div class="adk-typing">
        <span class="adk-dot" />
        <span class="adk-dot" />
        <span class="adk-dot" />
      </div>
    </div>
  </div>
</template>
