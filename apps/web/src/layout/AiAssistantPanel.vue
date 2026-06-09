<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import type {
  ADKApproval,
  ADKApprovalResolution,
  ADKChatResponse,
  ADKRun,
  ADKSessionContextSnapshot,
} from "@jftrade/ui-contracts";

import ADKChatComposer from "../components/adk-page/ADKChatComposer.vue";
import ADKChatThread from "../components/adk-page/ADKChatThread.vue";
import { isTerminalRunStatus, runTerminalMessage } from "../composables/adkChatPresentation";
import { streamADKChat } from "../composables/adkChatStream";
import { scrollToBottom } from "../composables/adkThreadScroll";
import { loadSessionChatHistory } from "../composables/adkPageRunHistory";
import {
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

interface ApprovalsResponse {
  approvals: ADKApproval[];
}

const { renderMarkdown } = useADKMarkdownRenderer();

const timelineEntries = ref<ADKTimelineEntryState[]>([]);
const draft = ref("");
const busy = ref(false);
const approvalsBusy = ref(false);
const contextBusy = ref(false);
const contextDetailsOpen = ref(false);
const errorMessage = ref("");
const scrollHost = ref<HTMLElement | null>(null);
const pendingApprovals = ref<ADKApproval[]>([]);
const sessionId = ref("");
const activeRunId = ref("");
const sessionContext = ref<ADKSessionContextSnapshot | null>(null);

const suggestions = [
  "查看系统状态",
  "查看待审批动作",
  "分析最近一次运行",
  "总结当前智能体进展",
];

const canSendChat = computed(() => draft.value.trim() !== "" && !busy.value);
const showTypingIndicator = computed(() => {
  if (!busy.value) return false;
  const lastEntry = timelineEntries.value.at(-1);
  if (!lastEntry) return true;
  if (lastEntry.kind === "tool_group") return false;
  if ((lastEntry.text ?? "").trim() !== "") return false;
  return true;
});
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

async function send(): Promise<void> {
  const text = draft.value.trim();
  if (!text || busy.value) return;

  const exactCommand = slashCommands.value.find(
    (item) => item.command.toLowerCase() === text.toLowerCase(),
  );
  if (exactCommand && !exactCommand.disabled) {
    draft.value = "";
    await runSlashCommand(exactCommand.id);
    return;
  }

  draft.value = "";
  errorMessage.value = "";
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
  busy.value = true;

  try {
    const response = await streamADKChat(
      { sessionId: sessionId.value, message: text },
      async (event) => {
        if (event.type === "session" && event.session?.id) {
          sessionId.value = event.session.id;
        }
        if (event.type === "context" && event.context) {
          sessionContext.value = event.context;
        }
        if (event.type === "run" && event.run?.id) {
          activeRunId.value = event.run.id;
        }
        if (event.type === "timeline" && event.timeline) {
          timelineEntries.value = upsertTimelineEntry(
            timelineEntries.value,
            event.timeline,
          );
          await scrollToBottom(scrollHost);
        }
        if (event.type === "final" && event.response) {
          await applyAuthoritativeTimeline(event.response, event.response.session.id);
          pendingApprovals.value = event.response.pendingApprovals;
          if (event.response.context) {
            sessionContext.value = event.response.context;
          }
          const failMsg = runTerminalMessage(event.response.run);
          if (failMsg) {
            errorMessage.value = failMsg;
          }
        }
        if (event.type === "error") {
          throw new Error(event.message || "Agents chat failed");
        }
      },
    );

    sessionId.value = response.session.id;
    await applyAuthoritativeTimeline(response, response.session.id);
    pendingApprovals.value = response.pendingApprovals;
    if (response.context) {
      sessionContext.value = response.context;
    } else {
      await refreshSessionContext();
    }
    const failMsg = runTerminalMessage(response.run);
    if (failMsg) {
      errorMessage.value = failMsg;
    }
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "Agents chat failed";
  } finally {
    busy.value = false;
    activeRunId.value = "";
    await scrollToBottom(scrollHost);
  }
}

async function refreshApprovals(): Promise<void> {
  try {
    const response = await fetchEnvelope<ApprovalsResponse>(
      "/api/v1/adk/approvals?status=PENDING&limit=20",
    );
    pendingApprovals.value = response.approvals;
  } catch {
    pendingApprovals.value = [];
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
  contextBusy.value = true;
  try {
    sessionContext.value = await compactADKSessionContext(sessionId.value, mode);
    contextDetailsOpen.value = true;
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "上下文压缩失败";
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
  const resolutions: ADKApprovalResolution[] = [];
  const errors: string[] = [];
  try {
    for (const approval of approvals) {
      try {
        resolutions.push(await submitApproval(approval, action));
      } catch (error) {
        errors.push(error instanceof Error ? error.message : "审批处理失败");
      }
    }
    await finalizeApprovalBatch(resolutions);
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

async function submitApproval(
  approval: ADKApproval,
  action: "approve" | "deny",
): Promise<ADKApprovalResolution> {
  return fetchEnvelopeWithInit<ADKApprovalResolution>(
    `/api/v1/adk/approvals/${encodeURIComponent(approval.id)}/${action}`,
    { method: "POST" },
  );
}

async function finalizeApprovalBatch(
  resolutions: ADKApprovalResolution[],
): Promise<void> {
  await refreshApprovals();
  await refreshSessionContext();
  const runs = Array.from(
    new Map(
      resolutions
        .map((resolution) => resolution.run)
        .filter((run): run is ADKRun => run != null)
        .map((run) => [run.id, run]),
    ).values(),
  );

  for (const run of runs) {
    if (isTerminalRunStatus(run.status)) {
      await reloadTimeline();
      const failMsg = runTerminalMessage(run);
      if (failMsg) {
        errorMessage.value = failMsg;
      }
      continue;
    }
    await waitForApprovalContinuation(run);
  }
}

async function waitForApprovalContinuation(
  run: ADKRun | undefined,
): Promise<void> {
  if (!run || isTerminalRunStatus(run.status)) {
    return;
  }

  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    await delay(900);
    try {
      const latestRun = await fetchEnvelope<ADKRun>(
        `/api/v1/adk/runs/${encodeURIComponent(run.id)}`,
      );
      if (isTerminalRunStatus(latestRun.status)) {
        await reloadTimeline();
        const failMsg = runTerminalMessage(latestRun);
        if (failMsg) {
          errorMessage.value = failMsg;
        }
        return;
      }
    } catch {
      return;
    }
  }
}

async function reloadTimeline(): Promise<void> {
  if (!sessionId.value) return;
  const detail = await loadSessionChatHistory(sessionId.value);
  timelineEntries.value = detail.timelineEntries;
  await scrollToBottom(scrollHost);
}

async function applyAuthoritativeTimeline(
  response: ADKChatResponse,
  _nextSessionId: string,
): Promise<void> {
  timelineEntries.value = replaceTimelineEntries(response.timeline, timelineEntries.value);
  await scrollToBottom(scrollHost);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
</script>

<template>
  <div
    style="display: flex; flex-direction: column; height: 100%; min-height: 0"
  >
    <div ref="scrollHost" class="adk-thread adk-thread--dock">
      <ADKChatThread
        variant="dock"
        :timeline-entries="timelineEntries"
        :sending-chat="busy"
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

    <ADKChatComposer
      variant="dock"
      :active-run-id="activeRunId"
      :can-send-chat="canSendChat"
      :chat-draft="draft"
      :context-busy="contextBusy"
      :context-details-open="contextDetailsOpen"
      :context-snapshot="sessionContext"
      :sending-chat="busy"
      :slash-commands="slashCommands"
      :suggestions="suggestions"
      placeholder="问点什么..."
      :handle-composer-keydown="handleComposerKeydown"
      :open-context-details="openContextDetails"
      :run-slash-command="runSlashCommand"
      :send-chat="send"
      :apply-suggestion="applySuggestion"
      @update:chat-draft="draft = $event"
      @update:context-details-open="contextDetailsOpen = $event"
    />
  </div>
</template>
