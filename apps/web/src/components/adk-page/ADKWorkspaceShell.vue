<script setup lang="ts">
import mermaid from "mermaid";
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import { useRouter } from "vue-router";

import ADKApprovalQueuePanel from "./ADKApprovalQueuePanel.vue";
import ADKChatComposer from "./ADKChatComposer.vue";
import ADKChatThread from "./ADKChatThread.vue";
import ADKChildRunQueuePanel from "./ADKChildRunQueuePanel.vue";
import ADKSessionSidebar from "./ADKSessionSidebar.vue";
import ADKWorkflowPlanPanel from "./ADKWorkflowPlanPanel.vue";
import { useADKMarkdownRenderer } from "../../composables/useADKMarkdownRenderer";
import { useADKPageController } from "../../composables/useADKPageController";

const props = withDefaults(
  defineProps<{
    layout?: "desktop" | "mobile";
    emptyStateTitle?: string;
  }>(),
  {
    layout: "desktop",
    emptyStateTitle: "开始与智能体对话",
  },
);

const router = useRouter();
const { renderMarkdown } = useADKMarkdownRenderer({ enableMermaid: true });

const threadRef = ref<HTMLElement | null>(null);
const childHeaderRef = ref<HTMLElement | null>(null);
const showChildStickyBar = ref(false);
const mobileSessionPanelOpen = ref(false);
const isNarrowViewport = ref(false);
let mermaidRenderFrame: number | null = null;
let narrowViewportMediaQuery: MediaQueryList | null = null;

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
  creatingSession,
  createNewSession,
  deleteSession,
  errorMessage,
  formatPermission,
  goalObjectiveDraft,
  goalObjectiveError,
  goalObjectiveSaving,
  goalLifecycleBusy,
  goalPaused,
  goalTimedOut,
  goalPauseRequested,
  showGoalObjectiveEditor,
  canSaveGoalObjective,
  canPauseGoal,
  canResumeGoal,
  hasBlockingRun,
  handleAgentChange,
  handleComposerKeydown,
  handleProviderChange,
  interruptAndQueueChat,
  interruptingRunId,
  loading,
  openProviderSettings,
  pauseGoalRun,
  preview,
  providerOptions,
  providers,
  queueDispatchingId,
  queuedMessages,
  revokeQueuedMessage,
  resumeGoalRun,
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
  activityIndicator,
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
  permissionModeOverride,
  openContextDetails,
} = useADKPageController(router, threadRef);

const mermaidRenderSignature = computed(() =>
  visibleTimelineEntries.value
    .filter((entry) => String(entry.text ?? "").includes("```mermaid"))
    .map((entry) =>
      [entry.id, entry.runId ?? "", entry.status ?? "", entry.text ?? ""].join(
        "\u0000",
      ),
    )
    .join("\u0001"),
);

const emptyStateProviderHint = computed(() =>
  providers.value.length === 0
    ? "尚未添加模型提供商，请先前往 Agents 配置添加。"
    : selectedProvider.value
      ? `当前模型提供商：${selectedProvider.value.displayName} · ${selectedProvider.value.model}`
      : "",
);
const effectiveLayout = computed<"desktop" | "mobile">(() =>
  props.layout === "mobile" || isNarrowViewport.value ? "mobile" : "desktop",
);

const selectedSession = computed(
  () =>
    sessions.value.find((session) => session.id === selectedSessionId.value) ??
    null,
);
const mobileSessionLabel = computed(() =>
  selectedSession.value ? sessionTitle(selectedSession.value) : "未选择会话",
);
const mobileAgentLabel = computed(() => {
  const agent = selectedAgent.value;
  if (!agent) return "请选择 Agent";
  return `${agent.name} · ${formatPermission(agent.permissionMode)}`;
});

onMounted(() => {
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: "strict",
  });
  if (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function"
  ) {
    narrowViewportMediaQuery = window.matchMedia("(max-width: 720px)");
    isNarrowViewport.value = narrowViewportMediaQuery.matches;
    if (typeof narrowViewportMediaQuery.addEventListener === "function") {
      narrowViewportMediaQuery.addEventListener("change", syncNarrowViewport);
    } else {
      narrowViewportMediaQuery.addListener(syncNarrowViewport);
    }
  }
});

onBeforeUnmount(() => {
  if (mermaidRenderFrame !== null) {
    window.cancelAnimationFrame(mermaidRenderFrame);
    mermaidRenderFrame = null;
  }
  if (narrowViewportMediaQuery) {
    if (typeof narrowViewportMediaQuery.removeEventListener === "function") {
      narrowViewportMediaQuery.removeEventListener(
        "change",
        syncNarrowViewport,
      );
    } else {
      narrowViewportMediaQuery.removeListener(syncNarrowViewport);
    }
  }
  narrowViewportMediaQuery = null;
});

watch(
  mermaidRenderSignature,
  (signature) => {
    if (signature !== "") {
      scheduleMermaidRender();
    }
  },
  { flush: "post" },
);

watch(
  () => visibleTimelineEntries.value.length,
  () => {
    void nextTick(updateChildStickyBar);
  },
  { flush: "post" },
);

watch(
  childViewContext,
  () => {
    void nextTick(updateChildStickyBar);
  },
  { flush: "post" },
);

watch(selectedSessionId, () => {
  if (effectiveLayout.value === "mobile" && selectedSessionId.value !== "") {
    mobileSessionPanelOpen.value = false;
  }
});

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

function syncNarrowViewport(event: MediaQueryListEvent | MediaQueryList): void {
  isNarrowViewport.value = event.matches;
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

function toggleMobileSessionPanel(): void {
  mobileSessionPanelOpen.value = !mobileSessionPanelOpen.value;
}

async function handleMobileCreateNewSession(): Promise<void> {
  await createNewSession();
  mobileSessionPanelOpen.value = false;
}

async function handleMobileSessionSelect(sessionId: string): Promise<void> {
  await selectSession(sessionId);
  mobileSessionPanelOpen.value = false;
}
</script>

<template>
  <div
    class="adk-shell"
    :class="{ 'adk-shell--mobile': effectiveLayout === 'mobile' }"
  >
    <ADKSessionSidebar
      v-if="effectiveLayout === 'desktop'"
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
      :creating-session="creatingSession"
      :create-new-session="createNewSession"
      :select-session="selectSession"
      :rename-session="renameSession"
      :delete-session="deleteSession"
      @update:session-search="sessionSearch = $event"
      @update:session-agent-filter="sessionAgentFilter = $event"
    />

    <div class="adk-main">
      <div v-if="effectiveLayout === 'mobile'" class="adk-mobile-toolbar">
        <button
          type="button"
          class="adk-mobile-toolbar__button"
          :class="{ 'is-active': mobileSessionPanelOpen }"
          data-testid="adk-mobile-sessions-toggle"
          :aria-expanded="mobileSessionPanelOpen ? 'true' : 'false'"
          @click="toggleMobileSessionPanel"
        >
          <span>会话</span>
          <span class="adk-mobile-toolbar__count">{{ sessions.length }}</span>
        </button>
        <div class="adk-mobile-toolbar__summary">
          <strong :title="mobileSessionLabel">{{ mobileSessionLabel }}</strong>
          <span :title="mobileAgentLabel">{{ mobileAgentLabel }}</span>
        </div>
        <button
          type="button"
          class="adk-mobile-toolbar__button"
          :disabled="selectedAgentId === '' || creatingSession"
          @click="void handleMobileCreateNewSession()"
        >
          {{ creatingSession ? "创建中" : "新建" }}
        </button>
      </div>

      <div
        v-if="effectiveLayout === 'mobile' && mobileSessionPanelOpen"
        class="adk-mobile-session-drawer"
        data-testid="adk-mobile-session-drawer"
      >
        <div class="adk-mobile-session-drawer__actions">
          <button
            type="button"
            class="adk-mobile-session-drawer__close"
            data-testid="adk-mobile-sessions-close"
            @click="mobileSessionPanelOpen = false"
          >
            收起
          </button>
        </div>
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
          :creating-session="creatingSession"
          :create-new-session="handleMobileCreateNewSession"
          :select-session="handleMobileSessionSelect"
          :rename-session="renameSession"
          :delete-session="deleteSession"
          @update:session-search="sessionSearch = $event"
          @update:session-agent-filter="sessionAgentFilter = $event"
        />
      </div>

      <div
        ref="threadRef"
        class="adk-thread"
        :class="{ 'adk-thread--mobile': effectiveLayout === 'mobile' }"
        @scroll="updateChildStickyBar"
      >
        <div
          v-if="childViewContext"
          ref="childHeaderRef"
          class="adk-child-view-header"
          :class="{ 'adk-child-view-header--mobile': effectiveLayout === 'mobile' }"
        >
          <div class="adk-child-view-header__crumb">
            <span>父 Agent</span>
            <span>/</span>
            <strong>{{ childViewContext.title }}</strong>
            <span>/</span>
            <code>{{ childViewContext.runId }}</code>
          </div>
          <p>{{ childViewContext.message }}</p>
          <button type="button" @click="leaveChildView">返回父对话</button>
        </div>
        <div
          v-if="childViewContext && showChildStickyBar"
          class="adk-child-view-sticky"
          :class="{ 'adk-child-view-sticky--mobile': effectiveLayout === 'mobile' }"
        >
          <div class="adk-child-view-sticky__label">
            <span>父 Agent /</span>
            <strong>{{ childViewContext.title }}</strong>
            <code>{{ childViewContext.runId }}</code>
          </div>
          <button type="button" @click="leaveChildView">返回</button>
        </div>
        <ADKChatThread
          :layout="effectiveLayout"
          :active-run-id="activeRunId"
          :active-run-status="activeRunStatus"
          :has-blocking-run="hasBlockingRun"
          :timeline-entries="visibleTimelineEntries"
          :sending-chat="sendingChat"
          :activity-indicator="activityIndicator"
          :error-message="errorMessage"
          :approvals-busy="approvalsBusy"
          :suggestions="suggestions"
          :empty-state-title="emptyStateTitle"
          :empty-state-hint="emptyStateHint"
          :empty-state-provider-hint="emptyStateProviderHint"
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
        :layout="effectiveLayout"
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
        :goal-lifecycle-busy="goalLifecycleBusy"
        :goal-paused="goalPaused"
        :goal-timed-out="goalTimedOut"
        :goal-pause-requested="goalPauseRequested"
        :show-goal-objective-editor="showGoalObjectiveEditor"
        :can-save-goal-objective="canSaveGoalObjective"
        :can-pause-goal="canPauseGoal"
        :can-resume-goal="canResumeGoal"
        :has-blocking-run="hasBlockingRun"
        :interrupting-run-id="interruptingRunId"
        :loading="loading"
        :placeholder="composerPlaceholder"
        :provider-options="providerOptions"
        :queued-messages="queuedMessages"
        :queue-dispatching-id="queueDispatchingId"
        :slash-commands="slashCommands"
        :saving-provider-selection="savingProviderSelection"
        :selected-agent="selectedAgent"
        :selected-agent-id="selectedAgentId"
        :selected-session-id="selectedSessionId"
        :selected-provider="selectedProvider"
        :selected-provider-id="selectedProviderId"
        :sending-chat="sendingChat"
        :suggestions="suggestions"
        :default-work-mode="selectedAgent?.workMode ?? 'chat'"
        :default-permission-mode="selectedAgent?.permissionMode ?? 'approval'"
        :permission-mode-override="permissionModeOverride"
        :work-mode-override="workModeOverride"
        :cancel-active-run="cancelActiveRun"
        :handle-agent-change="handleAgentChange"
        :handle-composer-keydown="handleComposerKeydown"
        :handle-provider-change="handleProviderChange"
        :interrupt-and-queue-chat="interruptAndQueueChat"
        :open-context-details="openContextDetails"
        :open-provider-settings="openProviderSettings"
        :pause-goal-run="pauseGoalRun"
        :revoke-queued-message="revokeQueuedMessage"
        :resume-goal-run="resumeGoalRun"
        :run-slash-command="runSlashCommand"
        :send-chat="sendChat"
        :update-goal-objective="updateGoalObjective"
        :update-goal-objective-draft="updateGoalObjectiveDraft"
        @update:chat-draft="chatDraft = $event"
        @update:context-details-open="contextDetailsOpen = $event"
        @update:selected-agent-id="selectedAgentId = $event"
        @update:selected-provider-id="selectedProviderId = $event"
        @update:permission-mode-override="permissionModeOverride = $event"
        @update:work-mode-override="workModeOverride = $event"
      />
    </div>
  </div>
</template>

<style scoped>
.adk-main {
  position: relative;
}

.adk-mobile-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 50px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--tv-border);
  background:
    linear-gradient(
      135deg,
      color-mix(in srgb, var(--tv-accent) 8%, transparent),
      transparent 58%
    ),
    color-mix(in srgb, var(--tv-bg-surface) 92%, transparent);
}

.adk-mobile-toolbar__button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid color-mix(in srgb, var(--tv-accent) 18%, var(--tv-border));
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-bg-surface) 78%, transparent);
  color: var(--tv-text);
  font-size: 11px;
  cursor: pointer;
}

.adk-mobile-toolbar__button.is-active {
  background: color-mix(in srgb, var(--tv-accent) 12%, var(--tv-bg-surface));
  border-color: color-mix(in srgb, var(--tv-accent) 28%, var(--tv-border));
  color: var(--tv-text);
}

.adk-mobile-toolbar__button:disabled {
  opacity: 0.6;
  cursor: default;
}

.adk-mobile-toolbar__count {
  min-width: 16px;
  padding: 1px 6px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-accent) 16%, transparent);
  color: color-mix(in srgb, var(--tv-accent) 82%, var(--tv-text));
  font-size: 10px;
  text-align: center;
}

.adk-mobile-toolbar__summary {
  min-width: 0;
  flex: 1 1 auto;
  display: grid;
  gap: 1px;
}

.adk-mobile-toolbar__summary strong,
.adk-mobile-toolbar__summary span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.adk-mobile-toolbar__summary strong {
  color: var(--tv-text);
  font-size: 12px;
}

.adk-mobile-toolbar__summary span {
  color: var(--tv-text-muted);
  font-size: 10px;
}

.adk-mobile-session-drawer {
  position: absolute;
  inset: 50px 8px 8px;
  z-index: 12;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--tv-accent) 18%, var(--tv-border));
  border-radius: 18px;
  background: color-mix(in srgb, var(--tv-bg-surface) 96%, transparent);
  box-shadow: 0 18px 42px rgba(2, 6, 23, 0.26);
}

.adk-mobile-session-drawer__actions {
  display: flex;
  justify-content: flex-end;
  padding: 8px 8px 0;
  flex: 0 0 auto;
}

.adk-mobile-session-drawer__close {
  min-height: 28px;
  padding: 0 10px;
  border: 1px solid color-mix(in srgb, var(--tv-accent) 18%, var(--tv-border));
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-bg-surface) 92%, transparent);
  color: var(--tv-text);
  font-size: 12px;
  cursor: pointer;
}

.adk-mobile-session-drawer :deep(.adk-sidebar) {
  flex: 1 1 auto;
  width: 100%;
  height: auto;
  min-height: 0;
  border-right: 0;
  background: transparent;
}

.adk-mobile-session-drawer :deep(.adk-sidebar-header) {
  padding-top: 10px;
  border-right: 0;
}

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

.adk-child-view-header--mobile {
  margin: 10px 10px 8px;
  padding: 10px;
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

.adk-child-view-sticky--mobile {
  gap: 8px;
  min-height: 28px;
  margin: -4px 10px 8px;
  padding: 4px 9px;
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

.adk-child-view-sticky--mobile .adk-child-view-sticky__label {
  gap: 5px;
  font-size: 11px;
}

.adk-child-view-sticky--mobile .adk-child-view-sticky__label code {
  max-width: 116px;
  font-size: 10px;
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

.adk-child-view-sticky--mobile button {
  padding: 3px 8px;
  font-size: 11px;
}
</style>
