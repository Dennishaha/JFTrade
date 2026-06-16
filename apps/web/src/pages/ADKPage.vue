<script setup lang="ts">
import mermaid from "mermaid";
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";

import ADKChatComposer from "../components/adk-page/ADKChatComposer.vue";
import ADKChatThread from "../components/adk-page/ADKChatThread.vue";
import ADKApprovalQueuePanel from "../components/adk-page/ADKApprovalQueuePanel.vue";
import ADKChildRunQueuePanel from "../components/adk-page/ADKChildRunQueuePanel.vue";
import ADKSessionSidebar from "../components/adk-page/ADKSessionSidebar.vue";
import ADKWorkflowPlanPanel from "../components/adk-page/ADKWorkflowPlanPanel.vue";
import { useADKMarkdownRenderer } from "../composables/useADKMarkdownRenderer";
import { useADKPageController } from "../composables/useADKPageController";

const router = useRouter();
const { renderMarkdown } = useADKMarkdownRenderer({ enableMermaid: true });

const threadRef = ref<HTMLElement | null>(null);
const childHeaderRef = ref<HTMLElement | null>(null);
const showChildStickyBar = ref(false);
let mermaidRenderFrame: number | null = null;
const {
  activeRunId,
  activeRunStatus,
  activeChildRunId,
  agentName,
  agentOptions,
  approvalTool,
  approvalsBusy,
  canInterruptChat,
  canSendChat,
  childRunItems,
  childViewContext,
  chatDraft,
  composerBlockMessage,
  cancelActiveRun,
  contextBusy,
  contextDetailsOpen,
  createNewSession,
  deleteSession,
  errorMessage,
  formatPermission,
  goalObjectiveDraft,
  goalObjectiveError,
  goalObjectiveSaving,
  showGoalObjectiveEditor,
  canSaveGoalObjective,
  hasBlockingRun,
  handleAgentChange,
  handleComposerKeydown,
  handleProviderChange,
  interruptAndQueueChat,
  interruptingRunId,
  loading,
  openProviderSettings,
  preview,
  providerOptions,
  providers,
  queueDispatchingId,
  queuedMessages,
  revokeQueuedMessage,
  runSlashCommand,
  renameSession,
  resolveApprovalGroup,
  resolveApproval,
  savingProviderSelection,
  selectedAgent,
  selectedApprovalQueue,
  selectedAgentId,
  selectedProvider,
  selectedProviderId,
  selectedSessionId,
  sendingChat,
  sessionContext,
  sessionAgentFilter,
  sessionSearch,
  sessions,
  sessionTitle,
  showTypingIndicator,
  suggestions,
  composerPlaceholder,
  emptyStateHint,
  slashCommands,
  selectSession,
  sendChat,
  setActiveChildRunId,
  updateGoalObjective,
  updateGoalObjectiveDraft,
  visibleSessions,
  visibleTimelineEntries,
  visibleWorkflowPlanRun,
  workModeOverride,
  openContextDetails,
} = useADKPageController(router, threadRef);

onMounted(() => {
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: "strict",
  });
});

onBeforeUnmount(() => {
  if (mermaidRenderFrame !== null) {
    window.cancelAnimationFrame(mermaidRenderFrame);
    mermaidRenderFrame = null;
  }
});

watch(
  visibleTimelineEntries,
  () => {
    scheduleMermaidRender();
    void nextTick(updateChildStickyBar);
  },
  { deep: true, flush: "post" },
);

watch(
  childViewContext,
  () => {
    void nextTick(updateChildStickyBar);
  },
  { flush: "post" },
);

function scheduleMermaidRender(): void {
  if (typeof window === "undefined" || mermaidRenderFrame !== null) return;
  mermaidRenderFrame = window.requestAnimationFrame(() => {
    mermaidRenderFrame = null;
    void renderMermaidDiagrams();
  });
}

async function renderMermaidDiagrams(): Promise<void> {
  const mermaidBlocks =
    threadRef.value?.querySelectorAll<HTMLElement>(".mermaid");
  if (!mermaidBlocks || mermaidBlocks.length === 0) return;
  try {
    await mermaid.run({ nodes: mermaidBlocks, suppressErrors: true });
  } catch (error) {
    console.warn("Failed to render mermaid diagrams", error);
  }
}

function clearErrorMessage(): void {
  errorMessage.value = "";
}

function updateChildStickyBar(): void {
  const host = threadRef.value;
  const header = childHeaderRef.value;
  if (!childViewContext.value || !host || !header) {
    showChildStickyBar.value = false;
    return;
  }
  showChildStickyBar.value =
    host.scrollTop > header.offsetTop + header.offsetHeight - 8;
}

function leaveChildView(): void {
  setActiveChildRunId("");
  showChildStickyBar.value = false;
}
</script>

<template>
  <div class="adk-shell">
    <ADKSessionSidebar
      :selected-agent-id="selectedAgentId"
      :selected-session-id="selectedSessionId"
      :selected-agent="selectedAgent"
      :session-search="sessionSearch"
      :session-agent-filter="sessionAgentFilter"
      :agent-options="agentOptions"
      :visible-sessions="visibleSessions"
      :sessions="sessions"
      :format-permission="formatPermission"
      :session-title="sessionTitle"
      :agent-name="agentName"
      :create-new-session="createNewSession"
      :select-session="selectSession"
      :rename-session="renameSession"
      :delete-session="deleteSession"
      @update:session-search="sessionSearch = $event"
      @update:session-agent-filter="sessionAgentFilter = $event"
    />

    <div class="adk-main">
      <div ref="threadRef" class="adk-thread" @scroll="updateChildStickyBar">
        <div v-if="childViewContext" ref="childHeaderRef" class="adk-child-view-header">
          <div class="adk-child-view-header__crumb">
            <span>父 Agent</span>
            <span>/</span>
            <strong>{{ childViewContext.title }}</strong>
            <span>/</span>
            <code>{{ childViewContext.runId }}</code>
          </div>
          <p>{{ childViewContext.message }}</p>
          <button type="button" @click="leaveChildView">
            返回父对话
          </button>
        </div>
        <div
          v-if="childViewContext && showChildStickyBar"
          class="adk-child-view-sticky"
        >
          <div class="adk-child-view-sticky__label">
            <span>父 Agent /</span>
            <strong>{{ childViewContext.title }}</strong>
            <code>{{ childViewContext.runId }}</code>
          </div>
          <button type="button" @click="leaveChildView">返回</button>
        </div>
        <ADKChatThread
          variant="page"
          :active-run-id="activeRunId"
          :active-run-status="activeRunStatus"
          :has-blocking-run="hasBlockingRun"
          :timeline-entries="visibleTimelineEntries"
          :sending-chat="sendingChat"
          :show-typing-indicator="showTypingIndicator"
          :error-message="errorMessage"
          :approvals-busy="approvalsBusy"
          :suggestions="suggestions"
          empty-state-title="开始与智能体对话"
          :empty-state-hint="emptyStateHint"
          :empty-state-provider-hint="
            providers.length === 0
              ? '尚未添加模型提供商，请先前往 Agents 配置添加。'
              : selectedProvider
                ? `当前模型提供商：${selectedProvider.displayName} · ${selectedProvider.model}`
                : ''
          "
          :approval-tool="approvalTool"
          :clear-error-message="clearErrorMessage"
          :preview="preview"
          :render-markdown="renderMarkdown"
          :resolve-approval-group="resolveApprovalGroup"
          :resolve-approval="resolveApproval"
          @update:chat-draft="chatDraft = $event"
        />
      </div>

      <ADKApprovalQueuePanel
        :items="selectedApprovalQueue"
        :approvals-busy="approvalsBusy"
        :approval-tool="approvalTool"
        :preview="preview"
        :resolve-approval-group="resolveApprovalGroup"
        :resolve-approval="resolveApproval"
      />
      <ADKChildRunQueuePanel
        :items="childRunItems"
        :active-child-run-id="activeChildRunId"
        @select="setActiveChildRunId"
      />
      <ADKWorkflowPlanPanel :run="visibleWorkflowPlanRun" />

      <ADKChatComposer
        variant="page"
        :active-run-id="activeRunId"
        :active-run-status="activeRunStatus"
        :agent-options="agentOptions"
        :can-interrupt-chat="canInterruptChat"
        :can-send-chat="canSendChat"
        :chat-draft="chatDraft"
        :composer-block-message="composerBlockMessage"
        :context-busy="contextBusy"
        :context-details-open="contextDetailsOpen"
        :context-snapshot="sessionContext"
        :goal-objective-draft="goalObjectiveDraft"
        :goal-objective-error="goalObjectiveError"
        :goal-objective-saving="goalObjectiveSaving"
        :show-goal-objective-editor="showGoalObjectiveEditor"
        :can-save-goal-objective="canSaveGoalObjective"
        :has-blocking-run="hasBlockingRun"
        :interrupting-run-id="interruptingRunId"
        :loading="loading"
        :placeholder="composerPlaceholder"
        :provider-options="providerOptions"
        :queued-messages="queuedMessages"
        :queue-dispatching-id="queueDispatchingId"
        :slash-commands="slashCommands"
        :saving-provider-selection="savingProviderSelection"
        :selected-agent-id="selectedAgentId"
        :selected-provider-id="selectedProviderId"
        :sending-chat="sendingChat"
        :work-mode-override="workModeOverride"
        :cancel-active-run="cancelActiveRun"
        :handle-agent-change="handleAgentChange"
        :handle-composer-keydown="handleComposerKeydown"
        :handle-provider-change="handleProviderChange"
        :interrupt-and-queue-chat="interruptAndQueueChat"
        :open-context-details="openContextDetails"
        :open-provider-settings="openProviderSettings"
        :revoke-queued-message="revokeQueuedMessage"
        :run-slash-command="runSlashCommand"
        :send-chat="sendChat"
        :update-goal-objective="updateGoalObjective"
        :update-goal-objective-draft="updateGoalObjectiveDraft"
        @update:chat-draft="chatDraft = $event"
        @update:context-details-open="contextDetailsOpen = $event"
        @update:selected-agent-id="selectedAgentId = $event"
        @update:selected-provider-id="selectedProviderId = $event"
        @update:work-mode-override="workModeOverride = $event"
      />
    </div>
  </div>
</template>

<style scoped>
.adk-child-view-header {
  display: grid;
  gap: 8px;
  margin: 16px clamp(16px, 3vw, 32px) 8px;
  padding: 12px;
  border: 1px solid color-mix(in srgb, var(--tv-accent) 35%, var(--tv-border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--tv-accent) 10%, var(--tv-bg-surface));
  color: var(--tv-text);
}

.adk-child-view-header__crumb {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  color: var(--tv-text-muted);
}

.adk-child-view-header__crumb code {
  font-size: 11px;
}

.adk-child-view-header p {
  margin: 0;
  color: var(--tv-text);
  font-size: 13px;
  line-height: 1.5;
}

.adk-child-view-header button {
  justify-self: start;
  border: 0;
  border-radius: 999px;
  padding: 5px 10px;
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent);
  color: color-mix(in srgb, var(--tv-accent) 82%, var(--tv-text));
  cursor: pointer;
  font-size: 12px;
}

.adk-child-view-sticky {
  position: sticky;
  top: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-height: 30px;
  margin: -4px clamp(16px, 3vw, 32px) 8px;
  padding: 4px 10px;
  border: 1px solid color-mix(in srgb, var(--tv-accent) 30%, var(--tv-border));
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-bg-surface) 88%, transparent);
  color: var(--tv-text);
  box-shadow: 0 10px 28px rgba(2, 6, 23, 0.22);
  backdrop-filter: blur(14px);
}

.adk-child-view-sticky__label {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 12px;
  white-space: nowrap;
}

.adk-child-view-sticky__label strong {
  color: var(--tv-text);
  font-weight: 700;
}

.adk-child-view-sticky__label code {
  overflow: hidden;
  max-width: 220px;
  color: var(--tv-text-dim);
  font-size: 11px;
  text-overflow: ellipsis;
}

.adk-child-view-sticky button {
  flex: 0 0 auto;
  border: 0;
  border-radius: 999px;
  padding: 3px 9px;
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent);
  color: color-mix(in srgb, var(--tv-accent) 82%, var(--tv-text));
  cursor: pointer;
  font-size: 12px;
}

@media (max-width: 720px) {
  .adk-child-view-sticky {
    margin-right: 12px;
    margin-left: 12px;
  }
}
</style>
