<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";

import type { ADKApproval, ADKApprovalResolution, ADKMessage, ADKRun } from "@jftrade/ui-contracts";

import ADKChatComposer from "../components/adk-page/ADKChatComposer.vue";
import ADKChatThread from "../components/adk-page/ADKChatThread.vue";
import {
  createAssistantMessageState,
  isTerminalRunStatus,
  runTerminalMessage,
  syncRunPresentationState,
} from "../composables/adkChatPresentation";
import { normalizeAssistantContent, streamADKChat } from "../composables/adkChatStream";
import {
  applyApprovalResolutionToChat,
  applyFinalResponse,
  mergeAssistantFinalText,
} from "../composables/adkPageChatResults";
import { scrollToBottom, type ChatMessage } from "../composables/adkPageMessages";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { useADKMarkdownRenderer } from "../composables/useADKMarkdownRenderer";

interface ApprovalsResponse {
  approvals: ADKApproval[];
}

interface SessionDetailResponse {
  messages: ADKMessage[];
}

const { renderMarkdown } = useADKMarkdownRenderer();

const messages = ref<ChatMessage[]>([]);
const draft = ref("");
const busy = ref(false);
const approvalsBusy = ref(false);
const errorMessage = ref("");
const scrollHost = ref<HTMLElement | null>(null);
const pendingApprovals = ref<ADKApproval[]>([]);

const suggestions = [
  "查看系统状态",
  "查看待审批动作",
  "分析最近一次运行",
  "总结当前智能体进展",
];

const canSendChat = computed(() =>
  draft.value.trim() !== "" && !busy.value,
);
const showTypingIndicator = computed(() => {
  if (!busy.value) return false;
  const lastMessage = messages.value.at(-1);
  if (!lastMessage || lastMessage.role !== "assistant") return true;
  if (lastMessage.content.trim() !== "" || (lastMessage.reasoningContent ?? "").trim() !== "") return false;
  if (lastMessage.run && lastMessage.run.status !== "RUNNING" && lastMessage.run.status !== "PENDING") return false;
  return true;
});

onMounted(() => {
  void refreshApprovals();
});

async function send(): Promise<void> {
  const text = draft.value.trim();
  if (!text || busy.value) return;

  draft.value = "";
  errorMessage.value = "";
  messages.value.push({ id: `dock-user-${Date.now()}`, role: "user", content: text });
  const assistantMessage = reactive<ChatMessage>(createAssistantMessageState(`dock-assistant-${Date.now()}`));
  messages.value.push(assistantMessage);
  await scrollToBottom(scrollHost);
  busy.value = true;

  try {
    const response = await streamADKChat({ message: text }, async (event) => {
      if (event.type === "run" && event.run) {
        syncRunPresentationState(assistantMessage, event.run);
      }
      if (event.type === "delta") {
        if (event.delta) {
          assistantMessage.content += event.delta;
        }
        if (event.reasoningDelta) {
          assistantMessage.reasoningContent = (assistantMessage.reasoningContent ?? "") + event.reasoningDelta;
        }
        if (event.toolProgress) {
          if (assistantMessage.preToolContent === undefined && assistantMessage.content.trim() !== "") {
            assistantMessage.preToolContent = assistantMessage.content;
            assistantMessage.content = "";
          }
          if (assistantMessage.preToolReasoning === undefined && (assistantMessage.reasoningContent ?? "").trim() !== "") {
            assistantMessage.preToolReasoning = assistantMessage.reasoningContent;
            assistantMessage.reasoningContent = "";
          }
          assistantMessage.toolProgress = event.toolProgress;
        }
        await scrollToBottom(scrollHost);
      }
      if (event.type === "final" && event.response) {
        applyFinalResponse(assistantMessage, event.response);
        pendingApprovals.value = event.response.pendingApprovals;
        const failMsg = runTerminalMessage(event.response.run);
        if (failMsg) {
          errorMessage.value = failMsg;
        }
      }
      if (event.type === "error") {
        throw new Error(event.message || "Agents chat failed");
      }
    });

    applyFinalResponse(assistantMessage, response);
    pendingApprovals.value = response.pendingApprovals;
    const failMsg = runTerminalMessage(response.run);
    if (failMsg) {
      errorMessage.value = failMsg;
    }
  } catch (error) {
    const fallbackMessage = error instanceof Error ? error.message : "Agents chat failed";
    if (assistantMessage.content.trim() === "" && (assistantMessage.reasoningContent ?? "").trim() === "") {
      assistantMessage.content = fallbackMessage;
    } else {
      errorMessage.value = fallbackMessage;
    }
  } finally {
    busy.value = false;
    await scrollToBottom(scrollHost);
  }
}

async function refreshApprovals(): Promise<void> {
  try {
    const response = await fetchEnvelope<ApprovalsResponse>("/api/v1/adk/approvals?status=PENDING&limit=20");
    pendingApprovals.value = response.approvals;
  } catch {
    pendingApprovals.value = [];
  }
}

async function resolveApproval(approval: ADKApproval, approved: boolean): Promise<void> {
  if (approvalsBusy.value) return;
  approvalsBusy.value = true;
  errorMessage.value = "";
  try {
    const resolution = await submitApproval(approval, approved ? "approve" : "deny");
    await finalizeApprovalBatch([resolution]);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "审批处理失败";
    await refreshApprovals();
  } finally {
    approvalsBusy.value = false;
  }
}

async function resolveAllApprovals(): Promise<void> {
  await resolveApprovalsBatch("approve");
}

async function denyAllApprovals(): Promise<void> {
  await resolveApprovalsBatch("deny");
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

async function submitApproval(
  approval: ADKApproval,
  action: "approve" | "deny",
): Promise<ADKApprovalResolution> {
  const resolution = await fetchEnvelopeWithInit<ADKApprovalResolution>(
    `/api/v1/adk/approvals/${encodeURIComponent(approval.id)}/${action}`,
    { method: "POST" },
  );
  messages.value = applyApprovalResolutionToChat(messages.value, resolution);
  await scrollToBottom(scrollHost);
  return resolution;
}

async function resolveApprovalsBatch(action: "approve" | "deny"): Promise<void> {
  if (pendingApprovals.value.length === 0 || approvalsBusy.value) return;
  approvalsBusy.value = true;
  errorMessage.value = "";
  const resolutions: ADKApprovalResolution[] = [];
  const errors: string[] = [];

  try {
    for (const approval of pendingApprovals.value) {
      try {
        resolutions.push(await submitApproval(approval, action));
      } catch (error) {
        errors.push(error instanceof Error ? error.message : "审批处理失败");
      }
    }

    await finalizeApprovalBatch(resolutions);
    if (errors.length > 0) {
      errorMessage.value = errors.length === 1
        ? errors[0]!
        : `批量审批部分失败：${errors[0]}`;
    }
  } finally {
    approvalsBusy.value = false;
  }
}

async function finalizeApprovalBatch(resolutions: ADKApprovalResolution[]): Promise<void> {
  await refreshApprovals();
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
      const failMsg = runTerminalMessage(run);
      if (failMsg) {
        errorMessage.value = failMsg;
      }
      continue;
    }
    await waitForApprovalContinuation(run);
  }
}

async function waitForApprovalContinuation(run: ADKRun | undefined): Promise<void> {
  if (!run || isTerminalRunStatus(run.status)) {
    return;
  }

  const deadline = Date.now() + 15_000;
  let latestRun = run;
  while (Date.now() < deadline) {
    await delay(900);
    try {
      latestRun = await fetchEnvelope<ADKRun>(`/api/v1/adk/runs/${encodeURIComponent(run.id)}`);
      messages.value = messages.value.map((message) =>
        message.run?.id === latestRun.id ? { ...message, run: latestRun } : message,
      );
      if (isTerminalRunStatus(latestRun.status)) {
        await appendFinalMessage(latestRun);
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

async function appendFinalMessage(run: ADKRun): Promise<void> {
  if (!run.finalMessageId || !run.sessionId) return;
  const detail = await fetchEnvelope<SessionDetailResponse>(`/api/v1/adk/sessions/${encodeURIComponent(run.sessionId)}`);
  const finalMessage = detail.messages.find((message) => message.id === run.finalMessageId);
  if (!finalMessage) return;

  const normalized = normalizeAssistantContent(finalMessage.content, finalMessage.reasoningContent);
  const messageIndex = messages.value.findIndex((message) => message.run?.id === run.id);
  const nextMessage: ChatMessage = {
    ...(createAssistantMessageState(finalMessage.id)),
    id: finalMessage.id,
    role: "assistant",
    content: normalized.content,
    reasoningContent: normalized.reasoningContent,
    run,
  };

  if (messageIndex >= 0) {
    const existingMessage = { ...messages.value[messageIndex]! };
    mergeAssistantFinalText(existingMessage, nextMessage.content, nextMessage.reasoningContent);
    messages.value[messageIndex] = {
      ...existingMessage,
      reasoningExpanded: false,
      run,
    };
  } else if (!messages.value.some((message) => message.id === finalMessage.id)) {
    messages.value.push(nextMessage);
  }
  await scrollToBottom(scrollHost);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
</script>

<template>
  <div style="display: flex; flex-direction: column; height: 100%; min-height: 0">
    <div ref="scrollHost" class="adk-thread adk-thread--dock">
      <ADKChatThread
        variant="dock"
        :chat-messages="messages"
        :sending-chat="busy"
        :show-typing-indicator="showTypingIndicator"
        :error-message="errorMessage"
        :pending-approvals="pendingApprovals"
        :approvals-busy="approvalsBusy"
        :suggestions="suggestions"
        empty-state-title="开始与侧栏助手对话"
        empty-state-hint="直接提问，或点击建议词快速发起一轮智能体对话。"
        :approval-tool="() => undefined"
        :clear-error-message="clearErrorMessage"
        :preview="preview"
        :render-markdown="renderMarkdown"
        :resolve-all-approvals="resolveAllApprovals"
        :resolve-approval="resolveApproval"
        :deny-all-approvals="denyAllApprovals"
        @update:chat-draft="draft = $event"
      />
    </div>

    <ADKChatComposer
      variant="dock"
      :can-send-chat="canSendChat"
      :chat-draft="draft"
      :sending-chat="busy"
      :suggestions="suggestions"
      placeholder="问点什么..."
      :handle-composer-keydown="handleComposerKeydown"
      :send-chat="send"
      :apply-suggestion="applySuggestion"
      @update:chat-draft="draft = $event"
    />
  </div>
</template>
