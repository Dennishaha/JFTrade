import { ref } from "vue";

import { buildRuntimeApiUrl } from "../runtimeConfig";
import {
  createEventSourceStream,
  type EventSourceConnectionState,
} from "./eventSourceStream";
import {
  normalizeMarketDataTickLiveEvent,
  type MarketDataTickLiveEvent,
} from "./marketDataRealtime";

const MAX_BUFFERED_EVENTS = 20;

function buildEventStreamUrl(path: string): string {
  return buildRuntimeApiUrl(path);
}

export type LiveStreamConnectionState =
  EventSourceConnectionState;

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
  const lastHeartbeat = ref<string | null>(null);
  const events = ref<LiveStreamEvent[]>([]);
  const stream = createEventSourceStream<LiveStreamEvent>({
    parseEvent: (rawPayload) =>
      normalizeMarketDataTickLiveEvent(rawPayload) ??
      (rawPayload as LiveStreamEvent),
    onMessage: (payload) => {
      events.value = [
        ...events.value.slice(-(MAX_BUFFERED_EVENTS - 1)),
        payload,
      ];

      if (payload.type === "heartbeat") {
        lastHeartbeat.value = payload.at;
      }
    },
  });

  function disconnect(): void {
    stream.activeUrl.value = null;
    stream.disconnect(true);
  }

  function connect(url = buildEventStreamUrl("/api/sse/live")): EventSource | null {
    return stream.connect(url);
  }

  return {
    connect,
    connectionState: stream.connectionState,
    disconnect,
    events,
    lastHeartbeat,
  };
}
