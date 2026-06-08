import type { ADKApprovalResolution, ADKRun } from "@jftrade/ui-contracts";

import { syncRunPresentationState, type ADKAssistantMessageState } from "./adkChatPresentation";
import { normalizeAssistantContent } from "./adkChatStream";
import { applyPersistedRunState, toChatMessage, type ChatMessage } from "./adkPageMessages";

interface AssistantTextState {
  content: string;
  reasoningContent?: string | undefined;
  preToolContent?: string | undefined;
  preToolReasoning?: string | undefined;
}

function normalizeText(value: string | undefined): string {
  return (value ?? "").trim();
}

function stripKnownPrefix(value: string, prefix: string | undefined): string {
  const normalizedValue = normalizeText(value);
  const normalizedPrefix = normalizeText(prefix);
  if (normalizedValue === "" || normalizedPrefix === "") return normalizedValue;
  if (normalizedValue === normalizedPrefix) return "";
  if (normalizedValue.startsWith(normalizedPrefix)) {
    return normalizedValue.slice(normalizedPrefix.length).trim();
  }
  return normalizedValue;
}

export function mergeAssistantText(existing: string | undefined, incoming: string | undefined): string {
  const current = normalizeText(existing);
  const next = normalizeText(incoming);
  if (next === "") return current;
  if (current === "") return next;
  if (next === current) return current;
  if (next.startsWith(current)) return next;
  if (current.startsWith(next)) return current;
  return `${current}\n\n${next}`;
}

export function mergeAssistantFinalText(
  message: AssistantTextState,
  content: string,
  reasoningContent?: string,
): void {
  const nextContent = message.preToolContent === undefined
    ? normalizeText(content)
    : stripKnownPrefix(content, message.preToolContent);
  message.content = mergeAssistantText(message.content, nextContent);

  const nextReasoning = message.preToolReasoning === undefined
    ? normalizeText(reasoningContent)
    : stripKnownPrefix(reasoningContent ?? "", message.preToolReasoning);
  message.reasoningContent = mergeAssistantText(message.reasoningContent, nextReasoning);
}

export function applyFinalResponse(
  message: ChatMessage,
  response: { reply: string; reasoningContent?: string; run: ADKRun },
): void {
  const normalized = normalizeAssistantContent(response.reply, response.reasoningContent);
  mergeAssistantFinalText(message, normalized.content, normalized.reasoningContent);
  syncRunPresentationState(message as ADKAssistantMessageState, response.run);
  message.toolProgress = "";
  message.reasoningExpanded = false;
}

export function applyApprovalResolutionToChat(
  chatMessages: ChatMessage[],
  resolution: ADKApprovalResolution,
): ChatMessage[] {
  if (!resolution.message && resolution.run) {
    const messageIndex = chatMessages.findIndex((item) => item.run?.id === resolution.run?.id);
    if (messageIndex < 0) return chatMessages;
    const nextMessages = [...chatMessages];
    nextMessages[messageIndex] = {
      ...nextMessages[messageIndex]!,
      run: resolution.run,
    };
    return nextMessages;
  }

  if (!resolution.message) {
    return chatMessages;
  }

  const message = toChatMessage(resolution.message);
  if (resolution.run) {
    applyPersistedRunState(message, resolution.run);
  }

  const messageIndex = resolution.run
    ? chatMessages.findIndex((item) => item.run?.id === resolution.run?.id)
    : -1;

  if (messageIndex < 0) {
    return [...chatMessages, message];
  }

  const nextMessages = [...chatMessages];
  const existingMessage = { ...nextMessages[messageIndex]! };
  mergeAssistantFinalText(existingMessage, message.content, message.reasoningContent);
  nextMessages[messageIndex] = {
    ...existingMessage,
    reasoningExpanded: false,
    run: resolution.run!,
  };
  return nextMessages;
}
