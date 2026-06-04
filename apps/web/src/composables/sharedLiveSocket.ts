import { ref, watch, type Ref } from "vue";

import { buildRuntimeLiveSocketUrl } from "../runtimeConfig";
import {
  normalizeMarketDataTickLiveEvent,
  type MarketDataTickLiveEvent,
  type MarketSecurityDetailsQueryResult,
} from "./marketDataRealtime";

export type { MarketDataTickLiveEvent };

const MAX_BUFFERED_EVENTS = 20;
const INITIAL_RECONNECT_DELAY_MS = 500;
const MAX_RECONNECT_DELAY_MS = 5000;
const MAX_RECONNECT_ATTEMPTS = 10;

export type LiveSocketConnectionState =
  | "idle"
  | "connecting"
  | "connected"
  | "disconnected"
  | "error"
  | "unsupported";

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

export type ConsoleRefreshLiveStreamEvent = {
  type: "console.refresh";
  at: string;
  checkedAt?: string;
};

export type MarketSecurityDetailsLiveStreamEvent =
  MarketSecurityDetailsQueryResult & {
    type: "market.security-details";
    at: string;
  };

export type MarketDepthLiveStreamEvent = {
  type: "market.depth";
  at: string;
  request: {
    market: string;
    symbol: string;
    instrumentId: string;
    num: number;
  };
  depth: unknown;
  meta: {
    instrumentId: string;
    source: string | null;
    resolvedAt: string;
    fromCache: boolean;
  };
};

export type LiveStreamEvent =
  | SystemNotificationLiveStreamEvent
  | MarketDataTickLiveEvent
  | ConsoleRefreshLiveStreamEvent
  | MarketSecurityDetailsLiveStreamEvent
  | MarketDepthLiveStreamEvent
  | {
      type: string;
      at: string;
    };

export interface LiveSocketSubscriptionSnapshot {
  activeInstruments: string[];
  securityDetails: Array<{
    market: string;
    symbol: string;
    instrumentId: string;
  }>;
  depth: Array<{
    market: string;
    symbol: string;
    instrumentId: string;
    num: number;
  }>;
  consoleRefresh: boolean;
}

function normalizeInstrumentId(value: string): string {
  return value.trim().toUpperCase();
}

function normalizeTarget<T extends { market: string; symbol: string; instrumentId: string }>(
  target: T,
): T {
  return {
    ...target,
    market: target.market.trim().toUpperCase(),
    symbol: target.symbol.trim().toUpperCase(),
    instrumentId: normalizeInstrumentId(target.instrumentId),
  };
}

class SharedLiveSocketHub {
  readonly connectionState = ref<LiveSocketConnectionState>("idle");
  readonly lastHeartbeat = ref<string | null>(null);
  readonly events = ref<LiveStreamEvent[]>([]);

  private socket: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempts = 0;
  private shouldReconnect = false;
  private currentAttemptConnected = false;
  private activeUrl: string | null = null;
  private lastSentSubscriptionPayload = "";
  private eventListeners = new Set<(event: LiveStreamEvent) => void>();
  private activeInstrumentOwners = new Map<string, string>();
  private securityDetailsOwners = new Map<
    string,
    { market: string; symbol: string; instrumentId: string }
  >();
  private depthOwners = new Map<
    string,
    { market: string; symbol: string; instrumentId: string; num: number }
  >();
  private consoleRefreshOwners = new Set<string>();
  private ownerSeq = 0;

  connect(url = buildRuntimeLiveSocketUrl("/api/v1/ws/live")): WebSocket | null {
    if (typeof WebSocket === "undefined") {
      this.connectionState.value = "unsupported";
      return null;
    }

    this.shouldReconnect = true;
    this.activeUrl = url;
    this.clearReconnectTimer();
    this.closeActiveSocket(false);
    this.connectionState.value = "connecting";
    this.currentAttemptConnected = false;

    const nextSocket = new WebSocket(url);
    this.socket = nextSocket;

    nextSocket.addEventListener("open", () => {
      if (this.socket !== nextSocket) {
        return;
      }
      this.connectionState.value = "connected";
      this.currentAttemptConnected = true;
      this.reconnectAttempts = 0;
      this.sendSubscriptionSnapshot(true);
    });

    nextSocket.addEventListener("message", (event) => {
      if (this.socket !== nextSocket || typeof event.data !== "string") {
        return;
      }
      try {
        const rawPayload = JSON.parse(event.data) as unknown;
        const payload =
          normalizeMarketDataTickLiveEvent(rawPayload) ??
          (rawPayload as LiveStreamEvent);
        this.events.value = [
          ...this.events.value.slice(-(MAX_BUFFERED_EVENTS - 1)),
          payload,
        ];
        if (payload.type === "heartbeat") {
          this.lastHeartbeat.value = payload.at;
        }
        for (const listener of this.eventListeners) {
          listener(payload);
        }
      } catch {
        this.connectionState.value = "error";
      }
    });

    nextSocket.addEventListener("error", () => {
      if (this.socket !== nextSocket) {
        return;
      }
      this.connectionState.value = this.currentAttemptConnected
        ? "error"
        : "disconnected";
      nextSocket.close();
    });

    nextSocket.addEventListener("close", () => {
      if (this.socket === nextSocket) {
        this.socket = null;
      }
      if (this.connectionState.value !== "unsupported") {
        this.connectionState.value =
          this.connectionState.value === "error" ? "error" : "disconnected";
      }
      this.scheduleReconnect();
    });

    return nextSocket;
  }

  disconnect(): void {
    this.shouldReconnect = false;
    this.activeUrl = null;
    this.reconnectAttempts = 0;
    this.clearReconnectTimer();
    this.closeActiveSocket(true);
  }

  reset(): void {
    this.disconnect();
    this.connectionState.value = "idle";
    this.lastHeartbeat.value = null;
    this.events.value = [];
    this.lastSentSubscriptionPayload = "";
    this.eventListeners.clear();
    this.activeInstrumentOwners.clear();
    this.securityDetailsOwners.clear();
    this.depthOwners.clear();
    this.consoleRefreshOwners.clear();
  }

  reconnect(): WebSocket | null {
    if (this.activeUrl == null) {
      return null;
    }
    return this.connect(this.activeUrl);
  }

  addEventListener(listener: (event: LiveStreamEvent) => void): () => void {
    this.eventListeners.add(listener);
    return () => {
      this.eventListeners.delete(listener);
    };
  }

  createOwnerId(prefix: string): string {
    this.ownerSeq += 1;
    return `${prefix}:${this.ownerSeq}`;
  }

  setActiveInstrument(ownerId: string, instrumentId: string | null): void {
    if (instrumentId == null || normalizeInstrumentId(instrumentId) === "") {
      this.activeInstrumentOwners.delete(ownerId);
    } else {
      this.activeInstrumentOwners.set(ownerId, normalizeInstrumentId(instrumentId));
    }
    this.sendSubscriptionSnapshot();
  }

  setSecurityDetailsTarget(
    ownerId: string,
    target: { market: string; symbol: string; instrumentId: string } | null,
  ): void {
    if (target == null || normalizeInstrumentId(target.instrumentId) === "") {
      this.securityDetailsOwners.delete(ownerId);
    } else {
      this.securityDetailsOwners.set(ownerId, normalizeTarget(target));
    }
    this.sendSubscriptionSnapshot();
  }

  setDepthTarget(
    ownerId: string,
    target:
      | { market: string; symbol: string; instrumentId: string; num: number }
      | null,
  ): void {
    if (target == null || normalizeInstrumentId(target.instrumentId) === "") {
      this.depthOwners.delete(ownerId);
    } else {
      this.depthOwners.set(ownerId, {
        ...normalizeTarget(target),
        num: Math.max(1, Math.min(50, Math.trunc(target.num))),
      });
    }
    this.sendSubscriptionSnapshot();
  }

  setConsoleRefreshEnabled(ownerId: string, enabled: boolean): void {
    if (enabled) {
      this.consoleRefreshOwners.add(ownerId);
    } else {
      this.consoleRefreshOwners.delete(ownerId);
    }
    this.sendSubscriptionSnapshot();
  }

  snapshotSubscriptions(): LiveSocketSubscriptionSnapshot {
    const activeInstruments = Array.from(
      new Set(this.activeInstrumentOwners.values()),
    ).sort();

    const securityDetails = Array.from(this.securityDetailsOwners.values()).sort(
      (left, right) => left.instrumentId.localeCompare(right.instrumentId),
    );

    const depth = Array.from(this.depthOwners.values()).sort((left, right) => {
      if (left.instrumentId === right.instrumentId) {
        return left.num - right.num;
      }
      return left.instrumentId.localeCompare(right.instrumentId);
    });

    return {
      activeInstruments,
      securityDetails,
      depth,
      consoleRefresh: this.consoleRefreshOwners.size > 0,
    };
  }

  /**
   * Wait for the WebSocket to reach "connected" state.
   * Resolves `true` if connected within `timeoutMs`, `false` on timeout.
   * Useful in visibility-change handlers to avoid acting on stale state
   * while a reconnect is still in progress.
   */
  waitForConnection(timeoutMs = 3_000): Promise<boolean> {
    if (this.connectionState.value === "connected") {
      return Promise.resolve(true);
    }

    return new Promise((resolve) => {
      let settled = false;
      const finish = (result: boolean): void => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        stopWatch();
        resolve(result);
      };

      const stopWatch = watch(
        this.connectionState,
        (state) => {
          if (state === "connected") {
            finish(true);
          }
        },
        { immediate: false },
      );

      const timer = setTimeout(() => finish(false), timeoutMs);
    });
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer == null) {
      return;
    }
    clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  private closeActiveSocket(markDisconnected: boolean): void {
    const activeSocket = this.socket;
    this.socket = null;
    activeSocket?.close();
    if (markDisconnected && this.connectionState.value !== "unsupported") {
      this.connectionState.value = "disconnected";
    }
  }

  private scheduleReconnect(): void {
    if (
      !this.shouldReconnect ||
      this.activeUrl == null ||
      this.socket != null ||
      this.reconnectTimer != null ||
      this.reconnectAttempts >= MAX_RECONNECT_ATTEMPTS
    ) {
      return;
    }
    const delay = Math.min(
      INITIAL_RECONNECT_DELAY_MS * 2 ** this.reconnectAttempts,
      MAX_RECONNECT_DELAY_MS,
    );
    this.reconnectAttempts += 1;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (this.shouldReconnect && this.activeUrl != null) {
        this.connect(this.activeUrl);
      }
    }, delay);
  }

  private sendSubscriptionSnapshot(force = false): void {
    if (this.socket == null || this.socket.readyState !== WebSocket.OPEN) {
      return;
    }
    const payload = JSON.stringify({
      type: "subscribe",
      subscriptions: this.snapshotSubscriptions(),
    });
    if (!force && payload === this.lastSentSubscriptionPayload) {
      return;
    }
    this.socket.send(payload);
    this.lastSentSubscriptionPayload = payload;
  }
}

let sharedLiveSocketHub: SharedLiveSocketHub | null = null;

export function getSharedLiveSocketHub(): SharedLiveSocketHub {
  sharedLiveSocketHub ??= new SharedLiveSocketHub();
  return sharedLiveSocketHub;
}

export function resetSharedLiveSocketHubForTests(): void {
  sharedLiveSocketHub?.reset();
}

export type SharedLiveSocketHubStore = {
  connectionState: Ref<LiveSocketConnectionState>;
  lastHeartbeat: Ref<string | null>;
  events: Ref<LiveStreamEvent[]>;
  connect: (url?: string) => WebSocket | null;
  disconnect: () => void;
  reconnect: () => WebSocket | null;
  waitForConnection: (timeoutMs?: number) => Promise<boolean>;
  addEventListener: (listener: (event: LiveStreamEvent) => void) => () => void;
  createOwnerId: (prefix: string) => string;
  setActiveInstrument: (ownerId: string, instrumentId: string | null) => void;
  setSecurityDetailsTarget: (
    ownerId: string,
    target: { market: string; symbol: string; instrumentId: string } | null,
  ) => void;
  setDepthTarget: (
    ownerId: string,
    target:
      | { market: string; symbol: string; instrumentId: string; num: number }
      | null,
  ) => void;
  setConsoleRefreshEnabled: (ownerId: string, enabled: boolean) => void;
  snapshotSubscriptions: () => LiveSocketSubscriptionSnapshot;
};
