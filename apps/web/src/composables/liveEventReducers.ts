import type { LiveEventEnvelope } from "./liveEventBus";
import type { KlineSyncProgress } from "./useKlineSyncTask";

type NotificationLevel = "info" | "success" | "warn" | "error";

interface NotificationSink {
  push(input: {
    level: NotificationLevel;
    title: string;
    source?: string;
    at?: string;
    category?: string;
    message?: string;
  }): void;
}

interface MarketDataReducerOptions {
  applyMarketDataTickEvent: (event: unknown) => void;
  flushIntervalMs?: number;
}

interface NotificationReducerOptions {
  notifications: NotificationSink;
  loadSystemState: (options?: { background?: boolean; bypassCooldown?: boolean }) => void | Promise<void>;
}

interface BacktestReducerOptions {
  activeTaskId: () => string;
  applyProgress: (progress: KlineSyncProgress) => void;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function notificationLevel(value: unknown): NotificationLevel {
  switch (value) {
    case "success":
    case "warn":
    case "error":
      return value;
    default:
      return "info";
  }
}

export function formatLiveEventTypeLabel(type: string): string {
  if (type === "heartbeat") return "心跳";
  if (type === "market-data.tick") return "行情推送";
  if (type === "system.notification") return "系统通知";
  if (type === "console.refresh") return "控制台刷新";
  if (type === "market.security-details") return "证券详情";
  if (type === "market.depth") return "盘口";
  if (type.startsWith("backtest.")) return "回测任务";
  return type;
}

export function createMarketDataLiveReducer(options: MarketDataReducerOptions) {
  let pendingMarketTickEvent: unknown = null;
  let marketTickFlushTimer: ReturnType<typeof setTimeout> | null = null;
  const flushIntervalMs = options.flushIntervalMs ?? Math.floor(1000 / 30);

  function flush(): void {
    if (pendingMarketTickEvent != null) {
      options.applyMarketDataTickEvent(pendingMarketTickEvent);
      pendingMarketTickEvent = null;
    }

    if (marketTickFlushTimer != null) {
      clearTimeout(marketTickFlushTimer);
      marketTickFlushTimer = null;
    }
  }

  function scheduleFlush(): void {
    if (marketTickFlushTimer != null) {
      return;
    }
    marketTickFlushTimer = setTimeout(flush, flushIntervalMs);
  }

  function handle(event: LiveEventEnvelope): boolean {
    if (event.source !== "market-data" || event.type !== "market-data.tick") {
      return false;
    }
    pendingMarketTickEvent = event.payload;
    scheduleFlush();
    return true;
  }

  return {
    dispose: flush,
    flush,
    handle,
  };
}

function shouldReloadSystemStateForNotification(payload: Record<string, unknown>): boolean {
  const source = stringValue(payload.source).trim().toLowerCase();
  const category = stringValue(payload.category).trim().toLowerCase();
  return source === "execution-orders" || category.startsWith("broker.order.");
}

export function createNotificationLiveReducer(options: NotificationReducerOptions) {
  function handle(event: LiveEventEnvelope): boolean {
    if (event.source !== "notification" || event.type !== "system.notification" || !isRecord(event.payload)) {
      return false;
    }

    const payload = event.payload;
    options.notifications.push({
      level: notificationLevel(payload.level),
      title: stringValue(payload.title) || "实时通知",
      source: stringValue(payload.source) || stringValue(payload.brokerId) || "live-stream",
      at: stringValue(payload.at) || event.serverTime,
      ...(stringValue(payload.category) ? { category: stringValue(payload.category) } : {}),
      ...(stringValue(payload.message) ? { message: stringValue(payload.message) } : {}),
    });

    if (shouldReloadSystemStateForNotification(payload)) {
      void options.loadSystemState({
        background: true,
        bypassCooldown: true,
      });
    }
    return true;
  }

  return { handle };
}

export function createBacktestLiveReducer(options: BacktestReducerOptions) {
  function handle(event: LiveEventEnvelope): boolean {
    if (event.source !== "backtest" || !event.type.includes("sync") || !isRecord(event.payload)) {
      return false;
    }

    const taskId = stringValue(event.payload.taskId);
    if (taskId === "" || taskId !== options.activeTaskId()) {
      return false;
    }
    options.applyProgress(event.payload as unknown as KlineSyncProgress);
    return true;
  }

  return { handle };
}
