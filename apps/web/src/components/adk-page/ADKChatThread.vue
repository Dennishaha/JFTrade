<script setup lang="ts">
import type { ADKApproval, ADKProvider, ADKToolDescriptor } from "@jftrade/ui-contracts";

import ADKRunTrace from "../shared/ADKRunTrace.vue";
import type { ChatMessage } from "../../composables/adkPageMessages";

defineProps<{
  chatMessages: ChatMessage[];
  sendingChat: boolean;
  showTypingIndicator: boolean;
  providers: ADKProvider[];
  selectedProvider: ADKProvider | null;
  pendingApprovals: ADKApproval[];
  suggestions: string[];
  chatDraft: string;
  approvalTool: (approval: ADKApproval) => ADKToolDescriptor | undefined;
  preview: (value: unknown) => string;
  renderMarkdown: (content: string) => string;
  resolveApproval: (approval: ADKApproval, approved: boolean) => void | Promise<void>;
}>();

defineEmits<{
  "update:chatDraft": [value: string];
}>();
</script>

<template>
  <div>
    <div v-if="chatMessages.length === 0 && !sendingChat" class="adk-empty">
      <v-icon size="52" class="adk-empty-icon">fa-solid fa-robot</v-icon>
      <p class="adk-empty-title">开始与 Agent 对话</p>
      <p class="adk-empty-hint">
        可直接输入问题，也可以用 @tool_name 显式调用内置工具
      </p>
      <p v-if="providers.length === 0" class="adk-empty-hint">
        尚未添加模型提供商，请先前往 Agents 配置添加。
      </p>
      <p v-else-if="selectedProvider" class="adk-empty-hint">
        当前模型提供商：{{ selectedProvider.displayName }} · {{ selectedProvider.model }}
      </p>
      <div class="adk-suggestions">
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
            @click="message.reasoningExpanded = !message.reasoningExpanded"
          >
            <v-icon size="12">
              {{ message.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
            </v-icon>
            <span>深度思考</span>
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
              @click="message.reasoningExpanded = !message.reasoningExpanded"
            >
              <v-icon size="12">
                {{ message.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
              </v-icon>
              <span>深度思考</span>
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
              @click="message.reasoningExpanded = !message.reasoningExpanded"
            >
              <v-icon size="12">
                {{ message.reasoningExpanded ? "fa-solid fa-chevron-down" : "fa-solid fa-chevron-right" }}
              </v-icon>
              <span>深度思考</span>
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

    <div v-if="showTypingIndicator" class="adk-msg adk-msg--assistant">
      <div class="adk-typing">
        <span class="adk-dot" />
        <span class="adk-dot" />
        <span class="adk-dot" />
      </div>
    </div>

    <div v-if="pendingApprovals.length > 0 && !sendingChat" class="adk-approvals">
      <div class="adk-approvals-header">
        <v-icon size="16" color="warning">fa-solid fa-shield-halved</v-icon>
        <span>需要批准以下工具调用</span>
      </div>
      <div
        v-for="approval in pendingApprovals"
        :key="approval.id"
        class="adk-approval-card"
      >
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
        <p v-if="approval.reason" class="adk-approval-reason">{{ approval.reason }}</p>
        <p class="adk-approval-reason">Run: {{ approval.runId }}</p>
        <pre class="adk-json mt-2">{{ preview(approval.input) }}</pre>
        <div class="adk-approval-actions">
          <v-btn color="primary" size="small" @click="resolveApproval(approval, true)">批准</v-btn>
          <v-btn variant="outlined" color="error" size="small" @click="resolveApproval(approval, false)">拒绝</v-btn>
        </div>
      </div>
    </div>
  </div>
</template>
