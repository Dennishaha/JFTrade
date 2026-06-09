<script setup lang="ts">
import { computed } from "vue";

import type { ADKApproval, ADKToolDescriptor } from "@jftrade/ui-contracts";

import type { ChatMessage } from "../../composables/adkPageMessages";
import ADKRunTrace from "../shared/ADKRunTrace.vue";

const props = withDefaults(defineProps<{
  variant?: "page" | "dock";
  chatMessages: ChatMessage[];
  sendingChat: boolean;
  showTypingIndicator: boolean;
  errorMessage: string;
  pendingApprovals: ADKApproval[];
  approvalsBusy: boolean;
  suggestions: string[];
  emptyStateTitle: string;
  emptyStateHint: string;
  emptyStateProviderHint?: string;
  approvalTool: (approval: ADKApproval) => ADKToolDescriptor | undefined;
  clearErrorMessage: () => void;
  preview: (value: unknown) => string;
  renderMarkdown: (content: string) => string;
  resolveAllApprovals: () => void | Promise<void>;
  resolveApproval: (approval: ADKApproval, approved: boolean) => void | Promise<void>;
  denyAllApprovals: () => void | Promise<void>;
}>(), {
  variant: "page",
  emptyStateProviderHint: "",
});

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
</script>

<template>
  <div :class="threadClass">
    <div v-if="chatMessages.length === 0 && !sendingChat" :class="emptyClass">
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

    <template v-for="message in chatMessages" :key="message.id">
      <div v-if="message.role === 'user'" class="adk-msg adk-msg--user">
        <div class="adk-bubble adk-bubble--user">{{ message.content }}</div>
      </div>

      <div v-else class="adk-msg adk-msg--assistant">
        <div
          v-if="(message.preToolContent ?? '').trim() !== ''"
          class="adk-bubble adk-bubble--assistant adk-markdown"
          v-html="renderMarkdown(message.preToolContent!)"
        />

        <div
          v-if="(message.preToolReasoning ?? '').trim() !== ''"
          class="adk-reasoning"
        >
          <button
            type="button"
            class="adk-reasoning-toggle"
            :aria-expanded="message.reasoningExpanded ? 'true' : 'false'"
            @click="message.reasoningExpanded = !message.reasoningExpanded"
          >
            <v-icon size="12">
              {{ message.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
            </v-icon>
            <span>{{ message.reasoningExpanded ? "隐藏深度思考" : "查看深度思考" }}</span>
          </button>
          <div
            v-if="message.reasoningExpanded"
            class="adk-bubble adk-bubble--assistant adk-reasoning-body adk-markdown"
            v-html="renderMarkdown(message.preToolReasoning ?? '')"
          />
        </div>

        <ADKRunTrace
          :run="message.run"
          :tool-progress="message.toolProgress"
          :busy="sendingChat && chatMessages[chatMessages.length - 1] === message"
          :compact="variant === 'dock'"
          :summary-expanded="message.toolSummaryExpanded"
          :expanded-tool-call-ids="message.expandedToolCallIds"
          @update:summary-expanded="message.toolSummaryExpanded = $event"
          @update:expanded-tool-call-ids="message.expandedToolCallIds = $event"
        />

        <template v-if="message.preToolContent !== undefined">
          <div
            v-if="(message.reasoningContent ?? '').trim() !== ''"
            class="adk-reasoning"
          >
            <button
              type="button"
              class="adk-reasoning-toggle"
              :aria-expanded="message.reasoningExpanded ? 'true' : 'false'"
              @click="message.reasoningExpanded = !message.reasoningExpanded"
            >
              <v-icon size="12">
                {{ message.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
              </v-icon>
              <span>{{ message.reasoningExpanded ? "隐藏深度思考" : "查看深度思考" }}</span>
            </button>
            <div
              v-if="message.reasoningExpanded"
              class="adk-bubble adk-bubble--assistant adk-reasoning-body adk-markdown"
              v-html="renderMarkdown(message.reasoningContent ?? '')"
            />
          </div>
        </template>

        <div
          v-if="message.content.trim() !== ''"
          class="adk-bubble adk-bubble--assistant adk-markdown"
          v-html="renderMarkdown(message.content)"
        />

        <template v-if="message.preToolContent === undefined">
          <div
            v-if="(message.reasoningContent ?? '').trim() !== ''"
            class="adk-reasoning"
          >
            <button
              type="button"
              class="adk-reasoning-toggle"
              :aria-expanded="message.reasoningExpanded ? 'true' : 'false'"
              @click="message.reasoningExpanded = !message.reasoningExpanded"
            >
              <v-icon size="12">
                {{ message.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
              </v-icon>
              <span>{{ message.reasoningExpanded ? "隐藏深度思考" : "查看深度思考" }}</span>
            </button>
            <div
              v-if="message.reasoningExpanded"
              class="adk-bubble adk-bubble--assistant adk-reasoning-body adk-markdown"
              v-html="renderMarkdown(message.reasoningContent ?? '')"
            />
          </div>
        </template>
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

    <div v-if="pendingApprovals.length > 0 && !sendingChat" class="adk-approvals">
      <div class="adk-approvals-header">
        <div class="adk-approvals-header__title">
          <v-icon size="16" color="warning">fa-solid fa-shield-halved</v-icon>
          <span>待审批工具调用</span>
          <span class="adk-approvals-count">{{ pendingApprovals.length }}</span>
        </div>
        <div class="adk-approvals-bulk">
          <v-btn
            class="adk-approvals-approve-all"
            color="primary"
            size="x-small"
            :disabled="approvalsBusy"
            @click="resolveAllApprovals"
          >
            全部审批
          </v-btn>
          <v-btn
            class="adk-approvals-deny-all"
            variant="outlined"
            color="error"
            size="x-small"
            :disabled="approvalsBusy"
            @click="denyAllApprovals"
          >
            全部拒绝
          </v-btn>
        </div>
      </div>
      <div
        v-for="approval in pendingApprovals"
        :key="approval.id"
        class="adk-approval-card"
      >
        <div class="adk-approval-row">
          <div class="adk-approval-main">
            <div class="adk-approval-tool">
              <v-icon size="14" class="mr-1">fa-solid fa-code</v-icon>
              <strong>{{ approval.toolName }}</strong>
              <v-chip
                v-if="approvalTool(approval)"
                size="x-small"
                :color="approvalTool(approval)?.riskLevel === 'critical' ? 'error' : 'warning'"
                variant="tonal"
              >
                {{ approvalTool(approval)?.riskLevel ?? "unknown" }} risk
              </v-chip>
            </div>
            <div class="adk-approval-meta">
              <span v-if="approval.reason">{{ approval.reason }}</span>
              <span>Run: {{ approval.runId }}</span>
            </div>
          </div>
          <div class="adk-approval-actions">
            <v-btn
              class="adk-approval-btn--approve"
              color="primary"
              size="x-small"
              :disabled="approvalsBusy"
              @click="resolveApproval(approval, true)"
            >
              批准
            </v-btn>
            <v-btn
              class="adk-approval-btn--deny"
              variant="outlined"
              color="error"
              size="x-small"
              :disabled="approvalsBusy"
              @click="resolveApproval(approval, false)"
            >
              拒绝
            </v-btn>
          </div>
        </div>
        <pre class="adk-json adk-approval-input">{{ preview(approval.input) }}</pre>
      </div>
    </div>
  </div>
</template>
