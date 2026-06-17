<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";

import type {
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKRun,
  ADKSessionContextSnapshot,
} from "@/contracts";

import ADKChatComposer from "../components/adk-page/ADKChatComposer.vue";
import ADKChatThread from "../components/adk-page/ADKChatThread.vue";
import ADKApprovalQueuePanel from "../components/adk-page/ADKApprovalQueuePanel.vue";
import ADKChildRunQueuePanel from "../components/adk-page/ADKChildRunQueuePanel.vue";
import ADKWorkflowPlanPanel from "../components/adk-page/ADKWorkflowPlanPanel.vue";
import {
  resolveADKApprovalBatchOnce,
  type ADKApprovalAction,
} from "../composables/adkApprovalResolution";
import {
  isTerminalRunStatus,
  runTerminalMessage,
} from "../composables/adkChatPresentation";
import {
  normalizeADKApprovalResolution,
  normalizeADKChatResponse,
  normalizeADKRun,
  normalizeADKTimelineEntry,
} from "../composables/adkNormalization";
import { streamADKChat } from "../composables/adkChatStream";
import {
  buildActiveChatRunState,
  buildQueueSessionKey,
  createQueuedChatMessage,
  hasPendingRunApproval,
  isBlockingRunStatus,
  type ActiveChatRunState,
  type QueuedChatMessage,
} from "../composables/adkChatRuntime";
import { monitorADKRunContinuation } from "../composables/adkRunContinuation";
import { scrollToBottom } from "../composables/adkThreadScroll";
import { loadSessionChatHistory } from "../composables/adkPageRunHistory";
import {
  applyApprovalResolutions,
  createTimelineEntryState,
  replaceTimelineEntries,
  upsertTimelineEntry,
  type ADKTimelineEntryState,
} from "../composables/adkTimeline";
import {
  compactADKSessionContext,
  fetchADKSessionContext,
} from "../composables/adkSessionContextApi";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { useADKMarkdownRenderer } from "../composables/useADKMarkdownRenderer";
import type { SlashCommandItem } from "../composables/useADKPageChatState";
import {
  sessionContextFromRunUsage,
  useADKWorkflowQueueState,
} from "../composables/useADKWorkflowQueueState";

interface ApprovalsResponse {
  approvals: ADKApproval[];
}

const { renderMarkdown } = useADKMarkdownRenderer();

const timelineEntries = ref<ADKTimelineEntryState[]>([]);
const draft = ref("");
const sendingChat = ref(false);
const activeRun = ref<ActiveChatRunState | null>(null);
const queuedChatMessages = ref<QueuedChatMessage[]>([]);
const queueDispatchingId = ref("");
const interruptingRunId = ref("");
const approvalsBusy = ref(false);
const contextBusy = ref(false);
const contextDetailsOpen = ref(false);
const errorMessage = ref("");
const scrollHost = ref<HTMLElement | null>(null);
const childHeaderRef = ref<HTMLElement | null>(null);
const showChildStickyBar = ref(false);
const sessionId = ref("");
const sessionContext = ref<ADKSessionContextSnapshot | null>(null);
const workflowQueues = useADKWorkflowQueueState({
  timelineEntries,
  selectedSessionId: sessionId,
});

const suggestions = [
  "查看系统状态",
  "查看待审批动作",
  "分析最近一次运行",
  "总结当前智能体进展",
];

const activeRunId = computed(() => activeRun.value?.runId ?? "");
const activeRunStatus = computed(() => activeRun.value?.status ?? "");
const hasBlockingRun = computed(() =>
  isBlockingRunStatus(activeRun.value?.status),
);
const currentQueueSessionKey = computed(() =>
  buildQueueSessionKey(sessionId.value),
);
const queuedMessages = computed(() =>
  queuedChatMessages.value.filter(
    (message) => message.sessionKey === currentQueueSessionKey.value,
  ),
);
const canSendChat = computed(
  () => draft.value.trim() !== "" && !workflowQueues.activeChildRunId.value,
);
const canInterruptChat = computed(
  () => canSendChat.value && hasBlockingRun.value && !workflowQueues.activeChildRunId.value,
);
const showTypingIndicator = computed(() => {
  return sendingChat.value || hasBlockingRun.value;
});
const displayedTimelineEntries = computed(
  () => workflowQueues.visibleTimelineEntries.value,
);
const composerBlockMessage = computed(() =>
  workflowQueues.activeChildRunId.value
    ? "子智能体视图仅支持观察和审批，请返回父对话后继续发送消息。"
    : "",
);
const composerPlaceholder = computed(() =>
  workflowQueues.activeChildRunId.value ? "子智能体视图仅支持观察和审批" : "问点什么...",
);
const visibleSessionContext = computed(() =>
  workflowQueues.activeChildRunId.value
    ? sessionContextFromRunUsage(
        workflowQueues.childRunSnapshots.value[
          workflowQueues.activeChildRunId.value
        ],
        sessionContext.value,
      )
    : sessionContext.value,
);
const slashCommands = computed<SlashCommandItem[]>(() => {
  const hasSession = sessionId.value.trim() !== "";
  return [
    {
      id: "context",
      command: "/context",
      title: "查看上下文占用",
      description: hasSession
        ? "打开当前会话的上下文详情"
        : "需要先开始一个会话",
      disabled: !hasSession,
    },
    {
      id: "compact",
      command: "/compact",
      title: "压缩当前会话",
      description: hasSession ? "执行标准上下文压缩" : "需要先开始一个会话",
      disabled: !hasSession,
    },
    {
      id: "compact-aggressive",
      command: "/compact-aggressive",
      title: "激进压缩当前会话",
      description: hasSession ? "执行更强的摘要压缩" : "需要先开始一个会话",
      disabled: !hasSession,
    },
  ];
});

onMounted(() => {
  void refreshApprovals();
});

watch(
  () => workflowQueues.childViewContext.value,
  () => {
    void nextTick(updateChildStickyBar);
  },
  { flush: "post" },
);

watch(
  displayedTimelineEntries,
  () => {
    void nextTick(updateChildStickyBar);
  },
  { deep: true, flush: "post" },
);

async function send(): Promise<void> {
  const text = draft.value.trim();
  if (!text) return;

  const exactCommand = slashCommands.value.find(
    (item) => item.command.toLowerCase() === text.toLowerCase(),
  );
  if (exactCommand && !exactCommand.disabled) {
    draft.value = "";
    await runSlashCommand(exactCommand.id);
    return;
  }

  if (hasBlockingRun.value || sendingChat.value) {
    enqueueChatMessage(text, "queued");
    draft.value = "";
    await scrollToBottom(scrollHost);
    return;
  }

  draft.value = "";
  await executeChatMessage(text);
}

async function interruptAndQueueChat(): Promise<void> {
  const text = draft.value.trim();
  if (!text) return;
  if (!hasBlockingRun.value && !sendingChat.value) {
    await send();
    return;
  }
  const currentRunId = activeRunId.value;
  enqueueChatMessage(text, "interrupt");
  draft.value = "";
  await scrollToBottom(scrollHost);
  if (!currentRunId || interruptingRunId.value === currentRunId) {
    return;
  }
  interruptingRunId.value = currentRunId;
  await cancelActiveRun(currentRunId);
}

async function refreshApprovals(): Promise<void> {
  try {
    await fetchEnvelope<ApprovalsResponse>(
      "/api/v1/adk/approvals?status=PENDING&limit=20",
    );
  } catch {
    // Ignore dock approval refresh failures.
  }
}

async function refreshSessionContext(): Promise<void> {
  if (!sessionId.value) {
    sessionContext.value = null;
    return;
  }
  try {
    sessionContext.value = await fetchADKSessionContext(sessionId.value);
  } catch {
    sessionContext.value = null;
  }
}

async function compactContext(mode: "normal" | "aggressive"): Promise<void> {
  if (!sessionId.value) {
    errorMessage.value = "当前没有可压缩的会话";
    return;
  }
  const currentSessionId = sessionId.value;
  contextBusy.value = true;
  try {
    sessionContext.value = await compactADKSessionContext(
      currentSessionId,
      mode,
    );
    contextDetailsOpen.value = true;
    if (sessionId.value === currentSessionId) {
      await reloadTimeline();
    }
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "上下文压缩失败";
    try {
      if (sessionId.value === currentSessionId) {
        await reloadTimeline();
      }
    } catch {
      // Keep the explicit error message if the timeline refresh also fails.
    }
  } finally {
    contextBusy.value = false;
  }
}

async function runSlashCommand(command: SlashCommandItem["id"]): Promise<void> {
  switch (command) {
    case "context":
      await refreshSessionContext();
      contextDetailsOpen.value = true;
      return;
    case "compact":
      await compactContext("normal");
      return;
    case "compact-aggressive":
      await compactContext("aggressive");
      return;
  }
}

async function resolveApproval(
  approval: ADKApproval,
  approved: boolean,
): Promise<void> {
  await resolveApprovalGroup([approval], approved);
}

async function resolveApprovalGroup(
  approvals: ADKApproval[],
  approved: boolean,
): Promise<void> {
  if (approvals.length === 0 || approvalsBusy.value) return;
  approvalsBusy.value = true;
  errorMessage.value = "";
  const action = approved ? "approve" : "deny";
  try {
    const { resolutions, errors } = await resolveADKApprovalBatchOnce({
      approvals,
      action,
      submit: submitApproval,
      onResolution: (resolution) => {
        timelineEntries.value = applyApprovalResolutions(
          timelineEntries.value,
          [resolution],
        );
        if (resolution.run) {
          const run = resolution.parentRun ?? resolution.run;
          void workflowQueues.syncWorkflowRun(resolution.run);
          void workflowQueues.syncWorkflowRun(resolution.parentRun);
          syncActiveRun(
            run,
            !isTerminalRunStatus(run.status),
          );
        }
      },
    });
    if (resolutions.length > 0) {
      await finalizeApprovalBatch(resolutions);
    }
    if (errors.length > 0) {
      errorMessage.value =
        errors.length === 1 ? errors[0]! : `批量审批部分失败：${errors[0]}`;
    }
  } finally {
    approvalsBusy.value = false;
  }
}

function clearErrorMessage(): void {
  errorMessage.value = "";
}

function handleComposerKeydown(event: KeyboardEvent): void {
  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    void send();
  }
}

function applySuggestion(suggestion: string): void {
  draft.value = suggestion;
  void send();
}

function preview(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
}

function openContextDetails(): void {
  contextDetailsOpen.value = true;
}

function revokeQueuedMessage(messageId: string): void {
  queuedChatMessages.value = queuedChatMessages.value.filter(
    (message) => message.id !== messageId,
  );
}

async function submitApproval(
  approval: ADKApproval,
  action: ADKApprovalAction,
): Promise<ADKApprovalResolution> {
  return normalizeADKApprovalResolution(
    await fetchEnvelopeWithInit<ADKApprovalResolution>(
      `/api/v1/adk/approvals/${encodeURIComponent(approval.id)}/${action}`,
      { method: "POST" },
    ),
  );
}

async function finalizeApprovalBatch(
  resolutions: ADKApprovalResolution[],
): Promise<void> {
  await refreshApprovals();
  await refreshSessionContext();
  await reloadTimeline();
  const runs = Array.from(
    new Map(
      resolutions
        .map((resolution) => resolution.parentRun ?? resolution.run)
        .filter((run): run is ADKRun => run != null)
        .map((run) => [run.id, run]),
    ).values(),
  );

  for (const run of runs) {
    syncActiveRun(run, !isTerminalRunStatus(run.status));
    if (isTerminalRunStatus(run.status)) {
      await handleTerminalRun(run);
      continue;
    }
    await waitForRunContinuation(run);
  }
}

async function waitForRunContinuation(run: ADKRun | undefined): Promise<void> {
  if (!run || isTerminalRunStatus(run.status)) {
    return;
  }
  try {
    const latestRun = await monitorADKRunContinuation(run, {
      onProgress: async (nextRun) => {
        syncActiveRun(nextRun, true);
        const failMsg = runTerminalMessage(nextRun);
        if (failMsg) {
          errorMessage.value = failMsg;
        }
        await reloadTimeline();
      },
      onTerminal: async (terminalRun) => {
        await reloadTimeline();
        await handleTerminalRun(terminalRun);
      },
    });
    if (latestRun && !isTerminalRunStatus(latestRun.status)) {
      syncActiveRun(latestRun, true);
      const failMsg = runTerminalMessage(latestRun);
      if (failMsg) {
        errorMessage.value = failMsg;
      }
      if (hasPendingRunApproval(latestRun)) {
        await reloadTimeline();
      }
    }
  } catch {
    // Ignore polling failures and keep the latest visible state.
  }
}

async function cancelActiveRun(runId = activeRunId.value): Promise<void> {
  if (!runId) return;
  try {
    const run = normalizeADKRun(await fetchEnvelopeWithInit<ADKRun>(
      `/api/v1/adk/runs/${encodeURIComponent(runId)}/cancel`,
      { method: "POST" },
    ));
    syncActiveRun(run, !isTerminalRunStatus(run.status));
    await reloadTimeline();
    if (isTerminalRunStatus(run.status)) {
      await handleTerminalRun(run);
      return;
    }
    await waitForRunContinuation(run);
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "取消运行失败";
  } finally {
    if (interruptingRunId.value === runId) {
      interruptingRunId.value = "";
    }
  }
}

async function reloadTimeline(): Promise<void> {
  if (!sessionId.value) return;
  const detail = await loadSessionChatHistory(sessionId.value);
  timelineEntries.value = detail.timelineEntries;
  await refreshSessionContext();
  await scrollToBottom(scrollHost);
}

async function applyAuthoritativeTimeline(
  response: ADKChatResponse,
): Promise<void> {
  const normalizedResponse = normalizeADKChatResponse(response);
  timelineEntries.value = replaceTimelineEntries(
    normalizedResponse.timeline,
    timelineEntries.value,
    new Map([[normalizedResponse.run.id, normalizedResponse.run]]),
  );
  await scrollToBottom(scrollHost);
}

async function executeChatMessage(text: string): Promise<boolean> {
  errorMessage.value = "";
  workflowQueues.clearWorkflowQueues();
  timelineEntries.value = [
    ...timelineEntries.value,
    createTimelineEntryState({
      id: `dock-user-${Date.now()}`,
      sessionId: sessionId.value,
      kind: "user_message",
      createdAt: new Date().toISOString(),
      sequence: timelineEntries.value.length + 1,
      status: "streaming",
      text,
    }),
  ];
  await scrollToBottom(scrollHost);
  sendingChat.value = true;

  try {
    const response = await streamADKChat(
      { sessionId: sessionId.value, message: text },
      async (event) => {
        if (event.type === "session" && event.session?.id) {
          setSessionId(event.session.id);
        }
        if (event.type === "context" && event.context) {
          sessionContext.value = event.context;
        }
        if (event.type === "run" && event.run?.id) {
          syncActiveRun(normalizeADKRun(event.run));
        }
        if (event.type === "timeline" && event.timeline) {
          timelineEntries.value = upsertTimelineEntry(
            timelineEntries.value,
            normalizeADKTimelineEntry(event.timeline),
          );
          await scrollToBottom(scrollHost);
        }
        if (event.type === "final" && event.response) {
          const response = normalizeADKChatResponse(event.response);
          await applyAuthoritativeTimeline(response);
          syncActiveRun(
            response.run,
            !isTerminalRunStatus(response.run.status),
          );
          if (response.context) {
            sessionContext.value = response.context;
          }
          const failMsg = runTerminalMessage(response.run);
          if (failMsg) {
            errorMessage.value = failMsg;
          }
        }
        if (event.type === "error") {
          throw new Error(event.message || "Agents chat failed");
        }
      },
    );

    const normalizedResponse = normalizeADKChatResponse(response);
    setSessionId(normalizedResponse.session.id);
    await applyAuthoritativeTimeline(normalizedResponse);
    syncActiveRun(
      normalizedResponse.run,
      !isTerminalRunStatus(normalizedResponse.run.status),
    );
    if (normalizedResponse.context) {
      sessionContext.value = normalizedResponse.context;
    } else {
      await refreshSessionContext();
    }
    const failMsg = runTerminalMessage(normalizedResponse.run);
    if (failMsg) {
      errorMessage.value = failMsg;
    }
    return true;
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "Agents chat failed";
    return false;
  } finally {
    sendingChat.value = false;
    await scrollToBottom(scrollHost);
    await dispatchQueuedMessagesIfIdle();
  }
}

async function handleTerminalRun(run: ADKRun): Promise<void> {
  syncActiveRun(run);
  const failMsg = runTerminalMessage(run);
  if (failMsg) {
    errorMessage.value = failMsg;
  }
  if (interruptingRunId.value === run.id) {
    interruptingRunId.value = "";
  }
  await dispatchQueuedMessagesIfIdle();
}

async function dispatchQueuedMessagesIfIdle(): Promise<void> {
  if (
    sendingChat.value ||
    hasBlockingRun.value ||
    queueDispatchingId.value !== ""
  ) {
    return;
  }
  const nextMessage = queuedMessages.value[0];
  if (!nextMessage) {
    return;
  }
  queueDispatchingId.value = nextMessage.id;
  const sent = await executeChatMessage(nextMessage.text);
  if (sent) {
    queuedChatMessages.value = queuedChatMessages.value.filter(
      (message) => message.id !== nextMessage.id,
    );
  }
  queueDispatchingId.value = "";
  if (sent) {
    await dispatchQueuedMessagesIfIdle();
  }
}

function enqueueChatMessage(
  text: string,
  mode: "queued" | "interrupt",
): QueuedChatMessage {
  const message = createQueuedChatMessage(
    text,
    currentQueueSessionKey.value,
    mode,
  );
  queuedChatMessages.value =
    mode === "interrupt"
      ? [message, ...queuedChatMessages.value]
      : [...queuedChatMessages.value, message];
  return message;
}

function setSessionId(nextSessionId: string): void {
  const normalized = nextSessionId.trim();
  if (!normalized) return;
  const previousSessionId = sessionId.value.trim();
  sessionId.value = normalized;
  if (previousSessionId !== "") {
    return;
  }
  const previousQueueKey = buildQueueSessionKey(previousSessionId);
  const nextQueueKey = buildQueueSessionKey(normalized);
  if (previousQueueKey === nextQueueKey) return;
  queuedChatMessages.value = queuedChatMessages.value.map((message) =>
    message.sessionKey === previousQueueKey
      ? { ...message, sessionKey: nextQueueKey }
      : message,
  );
}

function syncActiveRun(
  run: ADKRun | undefined,
  waitingForContinuation = false,
): void {
  if (!run) return;
  void workflowQueues.syncWorkflowRun(run);
  if (isTerminalRunStatus(run.status)) {
    if (!activeRun.value || activeRun.value.runId === run.id) {
      activeRun.value = null;
    }
    return;
  }
  activeRun.value = buildActiveChatRunState(run, waitingForContinuation);
}

function updateChildStickyBar(): void {
  const host = scrollHost.value;
  const header = childHeaderRef.value;
  if (!workflowQueues.childViewContext.value || !host || !header) {
    showChildStickyBar.value = false;
    return;
  }
  showChildStickyBar.value =
    host.scrollTop > header.offsetTop + header.offsetHeight - 8;
}

function leaveChildView(): void {
  workflowQueues.setActiveChildRunId("");
  showChildStickyBar.value = false;
}
</script>

<template>
  <div
    style="display: flex; flex-direction: column; height: 100%; min-height: 0"
  >
    <div ref="scrollHost" class="adk-thread adk-thread--dock" @scroll="updateChildStickyBar">
      <div
        v-if="workflowQueues.childViewContext.value"
        ref="childHeaderRef"
        class="adk-child-view-header adk-child-view-header--dock"
      >
        <div class="adk-child-view-header__crumb">
          <span>父 Agent</span>
          <span>/</span>
          <strong>{{ workflowQueues.childViewContext.value.title }}</strong>
          <span>/</span>
          <code>{{ workflowQueues.childViewContext.value.runId }}</code>
        </div>
        <p>{{ workflowQueues.childViewContext.value.message }}</p>
        <button type="button" @click="leaveChildView">
          返回父对话
        </button>
      </div>
      <div
        v-if="workflowQueues.childViewContext.value && showChildStickyBar"
        class="adk-child-view-sticky adk-child-view-sticky--dock"
      >
        <div class="adk-child-view-sticky__label">
          <span>父 Agent /</span>
          <strong>{{ workflowQueues.childViewContext.value.title }}</strong>
          <code>{{ workflowQueues.childViewContext.value.runId }}</code>
        </div>
        <button type="button" @click="leaveChildView">返回</button>
      </div>
      <ADKChatThread
        variant="dock"
        :active-run-id="activeRunId"
        :active-run-status="activeRunStatus"
        :has-blocking-run="hasBlockingRun"
        :timeline-entries="displayedTimelineEntries"
        :sending-chat="sendingChat"
        :show-typing-indicator="showTypingIndicator"
        :error-message="errorMessage"
        :approvals-busy="approvalsBusy"
        :suggestions="suggestions"
        empty-state-title="开始与侧栏助手对话"
        empty-state-hint="直接提问，或点击建议词快速发起一轮智能体对话。"
        :approval-tool="() => undefined"
        :clear-error-message="clearErrorMessage"
        :preview="preview"
        :render-markdown="renderMarkdown"
        :resolve-approval-group="resolveApprovalGroup"
        :resolve-approval="resolveApproval"
        @update:chat-draft="draft = $event"
      />
    </div>

    <ADKApprovalQueuePanel
      :items="workflowQueues.selectedApprovalQueue.value"
      :approvals-busy="approvalsBusy"
      :approval-tool="() => undefined"
      :preview="preview"
      :resolve-approval-group="resolveApprovalGroup"
      :resolve-approval="resolveApproval"
    />
    <ADKChildRunQueuePanel
      :items="workflowQueues.childRunItems.value"
      :active-child-run-id="workflowQueues.activeChildRunId.value"
      @select="workflowQueues.setActiveChildRunId"
    />
    <ADKWorkflowPlanPanel :run="workflowQueues.visibleWorkflowPlanRun.value" />

    <ADKChatComposer
      variant="dock"
      :active-run-id="activeRunId"
      :active-run-status="activeRunStatus"
      :can-interrupt-chat="canInterruptChat"
      :can-send-chat="canSendChat"
      :chat-draft="draft"
      :composer-block-message="composerBlockMessage"
      :context-busy="contextBusy"
      :context-details-open="contextDetailsOpen"
      :context-snapshot="visibleSessionContext"
      :has-blocking-run="hasBlockingRun"
      :interrupting-run-id="interruptingRunId"
      :queued-messages="queuedMessages"
      :queue-dispatching-id="queueDispatchingId"
      :sending-chat="sendingChat"
      :slash-commands="slashCommands"
      :suggestions="suggestions"
      :placeholder="composerPlaceholder"
      :cancel-active-run="cancelActiveRun"
      :handle-composer-keydown="handleComposerKeydown"
      :interrupt-and-queue-chat="interruptAndQueueChat"
      :open-context-details="openContextDetails"
      :revoke-queued-message="revokeQueuedMessage"
      :run-slash-command="runSlashCommand"
      :send-chat="send"
      :apply-suggestion="applySuggestion"
      @update:chat-draft="draft = $event"
      @update:context-details-open="contextDetailsOpen = $event"
    />
  </div>
</template>

<style scoped>
.adk-child-view-header {
  display: grid;
  gap: 8px;
  margin: 12px;
  padding: 10px;
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
  gap: 8px;
  min-height: 28px;
  margin: -4px 8px 8px;
  padding: 4px 9px;
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
  gap: 5px;
  min-width: 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 11px;
  white-space: nowrap;
}

.adk-child-view-sticky__label strong {
  color: var(--tv-text);
  font-weight: 700;
}

.adk-child-view-sticky__label code {
  overflow: hidden;
  max-width: 116px;
  color: var(--tv-text-dim);
  font-size: 10px;
  text-overflow: ellipsis;
}

.adk-child-view-sticky button {
  flex: 0 0 auto;
  border: 0;
  border-radius: 999px;
  padding: 3px 8px;
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent);
  color: color-mix(in srgb, var(--tv-accent) 82%, var(--tv-text));
  cursor: pointer;
  font-size: 11px;
}
</style>
