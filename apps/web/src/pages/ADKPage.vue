<script setup lang="ts">
import mermaid from "mermaid";
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";

import ADKChatComposer from "../components/adk-page/ADKChatComposer.vue";
import ADKChatThread from "../components/adk-page/ADKChatThread.vue";
import ADKSessionSidebar from "../components/adk-page/ADKSessionSidebar.vue";
import { useADKMarkdownRenderer } from "../composables/useADKMarkdownRenderer";
import { useADKPageController } from "../composables/useADKPageController";

const router = useRouter();
const { renderMarkdown } = useADKMarkdownRenderer({ enableMermaid: true });

const threadRef = ref<HTMLElement | null>(null);
let mermaidRenderFrame: number | null = null;
const {
  activeRunId,
  agentName,
  agentOptions,
  approvalTool,
  approvalsBusy,
  canSendChat,
  chatDraft,
  timelineEntries,
  composerBlockMessage,
  contextBusy,
  contextDetailsOpen,
  createNewSession,
  deleteSession,
  errorMessage,
  formatPermission,
  handleAgentChange,
  handleComposerKeydown,
  handleProviderChange,
  loading,
  openProviderSettings,
  preview,
  providerOptions,
  providers,
  runSlashCommand,
  renameSession,
  resolveApprovalGroup,
  resolveApproval,
  savingProviderSelection,
  selectedAgent,
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
  SUGGESTIONS,
  slashCommands,
  selectSession,
  sendChat,
  visibleSessions,
  cancelActiveRun,
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
  timelineEntries,
  () => {
    scheduleMermaidRender();
  },
  { deep: true, flush: "post" },
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
      <div ref="threadRef" class="adk-thread">
        <ADKChatThread
          variant="page"
          :timeline-entries="timelineEntries"
          :sending-chat="sendingChat"
          :show-typing-indicator="showTypingIndicator"
          :error-message="errorMessage"
          :approvals-busy="approvalsBusy"
          :suggestions="SUGGESTIONS"
          empty-state-title="开始与智能体对话"
          empty-state-hint="可直接输入问题，也可以用 @tool_name 显式调用内置工具"
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

      <ADKChatComposer
        variant="page"
        :active-run-id="activeRunId"
        :agent-options="agentOptions"
        :can-send-chat="canSendChat"
        :chat-draft="chatDraft"
        :composer-block-message="composerBlockMessage"
        :context-busy="contextBusy"
        :context-details-open="contextDetailsOpen"
        :context-snapshot="sessionContext"
        :loading="loading"
        :provider-options="providerOptions"
        :slash-commands="slashCommands"
        :saving-provider-selection="savingProviderSelection"
        :selected-agent-id="selectedAgentId"
        :selected-provider-id="selectedProviderId"
        :sending-chat="sendingChat"
        :cancel-active-run="cancelActiveRun"
        :handle-agent-change="handleAgentChange"
        :handle-composer-keydown="handleComposerKeydown"
        :handle-provider-change="handleProviderChange"
        :open-context-details="openContextDetails"
        :open-provider-settings="openProviderSettings"
        :run-slash-command="runSlashCommand"
        :send-chat="sendChat"
        @update:chat-draft="chatDraft = $event"
        @update:context-details-open="contextDetailsOpen = $event"
        @update:selected-agent-id="selectedAgentId = $event"
        @update:selected-provider-id="selectedProviderId = $event"
      />
    </div>
  </div>
</template>
