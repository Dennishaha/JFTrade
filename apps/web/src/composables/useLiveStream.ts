import { ref } from "vue";

import { buildRuntimeApiUrl } from "../runtimeConfig";
import {
  normalizeMarketDataTickLiveEvent,
  type MarketDataTickLiveEvent,
} from "./marketDataRealtime";

const MAX_BUFFERED_EVENTS = 20;

function buildEventStreamUrl(path: string): string {
  return buildRuntimeApiUrl(path);
}

export type LiveStreamConnectionState =
  | "idle"
  | "connecting"
  | "connected"
  | "disconnected"
  | "error"
  | "unsupported";

export type MarketDataTickLiveStreamEvent = MarketDataTickLiveEvent;

export type SystemNotificationLiveStreamEvent = {
  type: "system.notification";
  id: string;
  at: string;
  level: "info" | "success" | "warn" | "error";
  title: string;
  message?: string;
  source?: string;
  brokerId?: string;
  category?: string;
};

export type LiveStreamEvent =
  | SystemNotificationLiveStreamEvent
  | MarketDataTickLiveStreamEvent
  | {
      type: string;
      at: string;
    };

export function useLiveStream() {
  const connectionState = ref<LiveStreamConnectionState>("idle");
  const lastHeartbeat = ref<string | null>(null);
  const events = ref<LiveStreamEvent[]>([]);
  let stream: EventSource | null = null;
  let activeUrl: string | null = null;
  let currentAttemptConnected = false;

  function closeActiveStream(markDisconnected: boolean): void {
    const activeStream = stream;
    stream = null;
    activeStream?.close();

    if (markDisconnected && connectionState.value !== "unsupported") {
      connectionState.value = "disconnected";
    }
  }

  function disconnect(): void {
    activeUrl = null;
    currentAttemptConnected = false;
    closeActiveStream(true);
  }

  function connect(
    url = buildEventStreamUrl("/api/v1/stream/live"),
  ): EventSource | null {
    if (typeof EventSource === "undefined") {
      connectionState.value = "unsupported";
      return null;
    }

    activeUrl = url;
    currentAttemptConnected = false;
    closeActiveStream(false);
    connectionState.value = "connecting";

    const nextStream = new EventSource(url);
    stream = nextStream;

    nextStream.onopen = () => {
      if (stream !== nextStream) {
        return;
      }

      connectionState.value = "connected";
      currentAttemptConnected = true;
    };

    nextStream.onmessage = (event) => {
      if (stream !== nextStream) {
        return;
      }

      try {
        const rawPayload = JSON.parse(event.data as string) as unknown;
        const payload =
          normalizeMarketDataTickLiveEvent(rawPayload) ??
          (rawPayload as LiveStreamEvent);
        events.value = [
          ...events.value.slice(-(MAX_BUFFERED_EVENTS - 1)),
          payload,
        ];

        if (payload.type === "heartbeat") {
          lastHeartbeat.value = payload.at;
        }

        connectionState.value = "connected";
        currentAttemptConnected = true;
      } catch {
        connectionState.value = "error";
      }
    };

    nextStream.onerror = () => {
      if (stream !== nextStream) {
        return;
      }

      connectionState.value = currentAttemptConnected
        ? "error"
        : "disconnected";
    };

    return nextStream;
  }

  return {
    connect,
    connectionState,
    disconnect,
    events,
    lastHeartbeat,
  };
}
