<script setup lang="ts">
import MarkdownIt from "markdown-it";
import mermaid from "mermaid";
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";

import ADKChatComposer from "../components/adk-page/ADKChatComposer.vue";
import ADKChatThread from "../components/adk-page/ADKChatThread.vue";
import ADKSessionSidebar from "../components/adk-page/ADKSessionSidebar.vue";
import { useADKPageController } from "../composables/useADKPageController";

const router = useRouter();
const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
});

const defaultFenceRenderer = markdown.renderer.rules.fence;
markdown.renderer.rules.fence = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  if (!token) {
    return defaultFenceRenderer
      ? defaultFenceRenderer(tokens, idx, options, env, self)
      : self.renderToken(tokens, idx, options);
  }
  const language = token.info.trim().split(/\s+/)[0];
  if (language === "mermaid") {
    return `<div class="mermaid">${markdown.utils.escapeHtml(token.content.trim())}</div>`;
  }
  if (defaultFenceRenderer) {
    return defaultFenceRenderer(tokens, idx, options, env, self);
  }
  return self.renderToken(tokens, idx, options);
};

const threadRef = ref<HTMLElement | null>(null);
let mermaidRenderFrame: number | null = null;
const {
  activeRunId,
  agentName,
  agentOptions,
  approvalTool,
  canSendChat,
  chatDraft,
  chatMessages,
  composerBlockMessage,
  createNewSession,
  deleteSession,
  errorMessage,
  formatPermission,
  handleAgentChange,
  handleComposerKeydown,
  handleProviderChange,
  loading,
  openProviderSettings,
  pendingApprovals,
  preview,
  providerOptions,
  providers,
  renameSession,
  resolveApproval,
  selectedAgent,
  selectedAgentId,
  selectedProvider,
  selectedProviderId,
  selectedSessionId,
  sendingChat,
  sessionAgentFilter,
  sessionSearch,
  sessions,
  sessionTitle,
  showTypingIndicator,
  SUGGESTIONS,
  savingProviderSelection,
  selectSession,
  sendChat,
  tools,
  visibleSessions,
  cancelActiveRun,
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

watch(chatMessages, () => {
  scheduleMermaidRender();
}, { deep: true, flush: "post" });

function scheduleMermaidRender(): void {
  if (typeof window === "undefined" || mermaidRenderFrame !== null) return;
  mermaidRenderFrame = window.requestAnimationFrame(() => {
    mermaidRenderFrame = null;
    void renderMermaidDiagrams();
  });
}

async function renderMermaidDiagrams(): Promise<void> {
  const mermaidBlocks = threadRef.value?.querySelectorAll<HTMLElement>(".mermaid");
  if (!mermaidBlocks || mermaidBlocks.length === 0) return;
  try {
    await mermaid.run({ nodes: mermaidBlocks, suppressErrors: true });
  } catch (error) {
    console.warn("Failed to render mermaid diagrams", error);
  }
}

function renderMarkdown(content: string): string {
  return markdown
    .render(content)
    .replace(/<a /g, '<a target="_blank" rel="noopener noreferrer" ');
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
          :chat-messages="chatMessages"
          :sending-chat="sendingChat"
          :show-typing-indicator="showTypingIndicator"
          :providers="providers"
          :selected-provider="selectedProvider"
          :pending-approvals="pendingApprovals"
          :suggestions="SUGGESTIONS"
          :chat-draft="chatDraft"
          :approval-tool="approvalTool"
          :preview="preview"
          :render-markdown="renderMarkdown"
          :resolve-approval="resolveApproval"
          @update:chat-draft="chatDraft = $event"
        />
      </div>

      <v-alert
        v-if="errorMessage"
        type="warning"
        variant="tonal"
        density="compact"
        closable
        class="adk-error-bar"
        @click:close="errorMessage = ''"
      >
        {{ errorMessage }}
      </v-alert>

      <ADKChatComposer
        :active-run-id="activeRunId"
        :agent-options="agentOptions"
        :can-send-chat="canSendChat"
        :chat-draft="chatDraft"
        :composer-block-message="composerBlockMessage"
        :loading="loading"
        :provider-options="providerOptions"
        :saving-provider-selection="savingProviderSelection"
        :selected-agent-id="selectedAgentId"
        :selected-provider-id="selectedProviderId"
        :sending-chat="sendingChat"
        :cancel-active-run="cancelActiveRun"
        :handle-agent-change="handleAgentChange"
        :handle-composer-keydown="handleComposerKeydown"
        :handle-provider-change="handleProviderChange"
        :open-provider-settings="openProviderSettings"
        :send-chat="sendChat"
        @update:chat-draft="chatDraft = $event"
        @update:selected-agent-id="selectedAgentId = $event"
        @update:selected-provider-id="selectedProviderId = $event"
      />
    </div>
  </div>
</template>
