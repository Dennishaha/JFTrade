import { nextTick, type Ref } from "vue";

import type { ADKRun, ADKTranscriptEntry } from "@jftrade/ui-contracts";

import {
  createAssistantMessageState,
  syncRunPresentationState,
  type ADKAssistantMessageState,
} from "./adkChatPresentation";
import { normalizeAssistantContent } from "./adkChatStream";

export interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  reasoningContent?: string | undefined;
  reasoningExpanded?: boolean | undefined;
  toolProgress?: string | undefined;
  preToolContent?: string | undefined;
  preToolReasoning?: string | undefined;
  run?: ADKRun | undefined;
  toolSummaryExpanded?: boolean | undefined;
  expandedToolCallIds?: string[] | undefined;
}

export function toChatMessage(message: ADKTranscriptEntry): ChatMessage {
  const normalized = normalizeAssistantContent(message.content, message.reasoningContent);
  return {
    ...(message.role === "assistant" ? createAssistantMessageState(message.id) : {}),
    id: message.id,
    role: (message.role === "user" ? "user" : "assistant") as "user" | "assistant",
    content: normalized.content,
    reasoningContent: normalized.reasoningContent,
    reasoningExpanded: false,
    toolProgress: "",
  };
}

export function applyPersistedRunState(message: ChatMessage, run: ADKRun | undefined): void {
  syncRunPresentationState(message as ADKAssistantMessageState, run);
  if (!run || message.role !== "assistant") return;
  const preToolContent = (run.preToolContent ?? "").trim();
  const preToolReasoning = (run.preToolReasoning ?? "").trim();
  if (preToolContent !== "") {
    message.preToolContent = preToolContent;
    message.content = stripKnownPrefix(message.content, preToolContent);
  }
  if (preToolReasoning !== "") {
    message.preToolReasoning = preToolReasoning;
    message.reasoningContent = stripKnownPrefix(message.reasoningContent ?? "", preToolReasoning);
  }
}

function stripKnownPrefix(value: string, prefix: string): string {
  const normalizedValue = value.trim();
  const normalizedPrefix = prefix.trim();
  if (normalizedValue === "" || normalizedPrefix === "") return normalizedValue;
  if (normalizedValue === normalizedPrefix) return "";
  if (normalizedValue.startsWith(normalizedPrefix)) {
    return normalizedValue.slice(normalizedPrefix.length).trim();
  }
  return normalizedValue;
}

export async function scrollToBottom(threadRef: Ref<HTMLElement | null>): Promise<void> {
  await nextTick();
  if (threadRef.value) {
    threadRef.value.scrollTop = threadRef.value.scrollHeight;
  }
}
