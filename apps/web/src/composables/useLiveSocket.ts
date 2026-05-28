import { ref } from "vue";

import {
  normalizeMarketDataTickLiveEvent,
  type MarketDataTickLiveEvent,
} from "./marketDataRealtime";

const apiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL as string | undefined
)?.replace(/\/$/, "");
const MAX_BUFFERED_EVENTS = 20;
const INITIAL_RECONNECT_DELAY_MS = 500;
const MAX_RECONNECT_DELAY_MS = 5000;

export type LiveSocketConnectionState =
  | "idle"
  | "connecting"
  | "connected"
  | "disconnected"
  | "error"
  | "unsupported";

export type MarketDataTickLiveSocketEvent = MarketDataTickLiveEvent;

export type SystemNotificationLiveSocketEvent = {
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

export type LiveSocketEvent =
  | SystemNotificationLiveSocketEvent
  | MarketDataTickLiveSocketEvent
  | {
      type: string;
      at: string;
    };

function buildLiveSocketUrl(path: string): string {
  if (!apiBaseUrl) {
    return `ws://127.0.0.1:3000${path}`;
  }

  const url = new URL(apiBaseUrl);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = path;
  url.search = "";
  url.hash = "";
  return url.toString();
}

export function useLiveSocket() {
  const connectionState = ref<LiveSocketConnectionState>("idle");
  const lastHeartbeat = ref<string | null>(null);
  const events = ref<LiveSocketEvent[]>([]);
  let socket: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectAttempts = 0;
  let shouldReconnect = false;
  let activeUrl: string | null = null;
  // Track whether the current socket attempt ever reached "connected". A
  // WebSocket "error" event before that means the backend is unreachable
  // (e.g. dev environment with no live API), which should surface as a soft
  // "disconnected" rather than a red "error" indicator.
  let currentAttemptConnected = false;

  function clearReconnectTimer(): void {
    if (reconnectTimer == null) {
      return;
    }

    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }

  function closeActiveSocket(markDisconnected: boolean): void {
    const activeSocket = socket;
    socket = null;
    activeSocket?.close();

    if (markDisconnected && connectionState.value !== "unsupported") {
      connectionState.value = "disconnected";
    }
  }

  function scheduleReconnect(): void {
    if (
      !shouldReconnect ||
      activeUrl == null ||
      socket != null ||
      reconnectTimer != null
    ) {
      return;
    }

    const delay = Math.min(
      INITIAL_RECONNECT_DELAY_MS * 2 ** reconnectAttempts,
      MAX_RECONNECT_DELAY_MS,
    );
    reconnectAttempts += 1;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      if (shouldReconnect && activeUrl != null) {
        connect(activeUrl);
      }
    }, delay);
  }

  function disconnect(): void {
    shouldReconnect = false;
    activeUrl = null;
    reconnectAttempts = 0;
    clearReconnectTimer();
    closeActiveSocket(true);
  }

  function connect(
    url = buildLiveSocketUrl("/api/v1/ws/live"),
  ): WebSocket | null {
    if (typeof WebSocket === "undefined") {
      connectionState.value = "unsupported";
      return null;
    }

    shouldReconnect = true;
    activeUrl = url;
    clearReconnectTimer();
    closeActiveSocket(false);
    connectionState.value = "connecting";
    currentAttemptConnected = false;

    const nextSocket = new WebSocket(url);
    socket = nextSocket;

    nextSocket.addEventListener("open", () => {
      if (socket !== nextSocket) {
        return;
      }

      connectionState.value = "connected";
      currentAttemptConnected = true;
      reconnectAttempts = 0;
    });

    nextSocket.addEventListener("message", (event) => {
      if (socket !== nextSocket || typeof event.data !== "string") {
        return;
      }

      try {
        const rawPayload = JSON.parse(event.data) as unknown;
        const payload =
          normalizeMarketDataTickLiveEvent(rawPayload) ??
          (rawPayload as LiveSocketEvent);
        events.value = [
          ...events.value.slice(-(MAX_BUFFERED_EVENTS - 1)),
          payload,
        ];

        if (payload.type === "heartbeat") {
          lastHeartbeat.value = payload.at;
        }
      } catch {
        connectionState.value = "error";
      }
    });

    nextSocket.addEventListener("error", () => {
      if (socket !== nextSocket) {
        return;
      }

      // Only flag a red "error" when a previously-established connection
      // broke. A failure during the initial handshake (e.g. backend not
      // reachable) is a normal "disconnected" state that the reconnect loop
      // will keep retrying.
      connectionState.value = currentAttemptConnected
        ? "error"
        : "disconnected";
      nextSocket.close();
    });

    nextSocket.addEventListener("close", () => {
      if (socket === nextSocket) {
        socket = null;
      }

      if (connectionState.value !== "unsupported") {
        connectionState.value =
          connectionState.value === "error" ? "error" : "disconnected";
      }
      scheduleReconnect();
    });

    return nextSocket;
  }

  return {
    connect,
    connectionState,
    disconnect,
    events,
    lastHeartbeat,
  };
}
