import type { Ref } from "vue";

import {
  buildQueueSessionKey,
  createQueuedChatMessage,
  type QueuedChatMessage,
} from "./adkChatRuntime";

export function setSelectedSessionId(
  selectedSessionId: Ref<string>,
  queuedChatMessages: Ref<QueuedChatMessage[]>,
  sessionId: string,
): void {
  const normalized = sessionId.trim();
  if (normalized === "") return;
  const previousSessionId = selectedSessionId.value.trim();
  selectedSessionId.value = normalized;
  if (previousSessionId !== "") return;
  const previousQueueKey = buildQueueSessionKey(previousSessionId);
  const nextQueueKey = buildQueueSessionKey(normalized);
  if (previousQueueKey === nextQueueKey) return;
  queuedChatMessages.value = queuedChatMessages.value.map((message) =>
    message.sessionKey === previousQueueKey
      ? { ...message, sessionKey: nextQueueKey }
      : message,
  );
}

export function enqueueChatMessage(
  queuedChatMessages: Ref<QueuedChatMessage[]>,
  queueSessionKey: string,
  text: string,
  mode: "queued" | "interrupt",
  options: { forceChat?: boolean; clientRequestId?: string } = {},
): QueuedChatMessage {
  const message = createQueuedChatMessage(text, queueSessionKey, mode, options);
  queuedChatMessages.value =
    mode === "interrupt"
      ? [message, ...queuedChatMessages.value]
      : [...queuedChatMessages.value, message];
  return message;
}

export function revokeQueuedMessage(
  queuedChatMessages: Ref<QueuedChatMessage[]>,
  messageId: string,
): void {
  queuedChatMessages.value = queuedChatMessages.value.filter(
    (message) => message.id !== messageId,
  );
}

export function handleComposerKeydown(
  event: KeyboardEvent,
  sendChat: () => Promise<void>,
): void {
  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    void sendChat();
  }
}
