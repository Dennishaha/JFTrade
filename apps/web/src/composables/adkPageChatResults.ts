import type { ADKApprovalResolution, ADKRun } from "@jftrade/ui-contracts";

import { syncRunPresentationState, type ADKAssistantMessageState } from "./adkChatPresentation";
import { applyPersistedRunState, toChatMessage, type ChatMessage } from "./adkPageMessages";

export function applyFinalResponse(
  message: ChatMessage,
  response: { reply: string; reasoningContent?: string; run: ADKRun },
): void {
  if (message.preToolContent !== undefined) {
    message.content = response.reply.replace(message.preToolContent, "").trim();
    if (response.reasoningContent && message.preToolReasoning) {
      message.reasoningContent = response.reasoningContent.replace(message.preToolReasoning, "").trim();
    }
  } else {
    message.content = response.reply;
    message.reasoningContent = response.reasoningContent ?? message.reasoningContent ?? "";
  }
  syncRunPresentationState(message as ADKAssistantMessageState, response.run);
  message.toolProgress = "";
  message.reasoningExpanded = false;
}

export function applyApprovalResolutionToChat(
  chatMessages: ChatMessage[],
  resolution: ADKApprovalResolution,
): ChatMessage[] {
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
  nextMessages[messageIndex] = {
    ...nextMessages[messageIndex]!,
    content: message.content,
    reasoningContent: message.reasoningContent ?? "",
    reasoningExpanded: false,
    run: resolution.run!,
  };
  return nextMessages;
}
