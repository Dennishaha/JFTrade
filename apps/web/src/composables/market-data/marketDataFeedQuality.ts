import type { LiveSocketConnectionState } from "@/composables/market-data/sharedLiveSocket";

export type MarketDataFeedQualityState =
  | "healthy"
  | "degraded"
  | "unavailable"
  | "idle";

export type MarketDataFeedQualityInput = {
  connectionState: LiveSocketConnectionState;
  transportMode?: string | null;
  fromCache?: boolean;
  hasUsableData?: boolean;
  error?: string | null;
};

export type MarketDataFeedPresentationState =
  | "live"
  | "stale"
  | "loading"
  | "empty"
  | "error";

export type MarketDataFeedPresentation = {
  state: MarketDataFeedPresentationState;
  connectionLabel: string;
  qualityLabel: string;
  expected: boolean;
};

function normalizedTransportMode(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? "";
}

export function resolveMarketDataFeedQuality(
  input: MarketDataFeedQualityInput,
): MarketDataFeedQualityState {
  const transportMode = normalizedTransportMode(input.transportMode);
  if (input.error?.trim() && !input.hasUsableData) return "unavailable";
  if (
    input.fromCache ||
    transportMode === "snapshot-poll-fallback" ||
    transportMode === "snapshot-poll-delayed"
  ) {
    return "degraded";
  }

  switch (input.connectionState) {
    case "connected":
      return "healthy";
    case "connecting":
    case "disconnected":
    case "unsupported":
      return "degraded";
    case "error":
      return input.hasUsableData ? "degraded" : "unavailable";
    default:
      return transportMode === "idle" || transportMode === ""
        ? "idle"
        : "degraded";
  }
}

export function marketDataFeedQualityLabel(
  input: MarketDataFeedQualityInput,
  state = resolveMarketDataFeedQuality(input),
): string {
  switch (state) {
    case "healthy":
      return "实时推送正常";
    case "degraded": {
      if (input.fromCache) return "正在使用缓存数据";
      if (
        normalizedTransportMode(input.transportMode) ===
        "snapshot-poll-fallback"
      ) {
        return "快照轮询（推送回退）";
      }
      if (
        normalizedTransportMode(input.transportMode) ===
        "snapshot-poll-delayed"
      ) {
        return "非实时快照，时效以供应商返回为准";
      }
      switch (input.connectionState) {
        case "connecting":
          return "实时连接中";
        case "disconnected":
          return "实时连接已中断";
        case "unsupported":
          return "不支持推送，使用快照行情";
        case "error":
          return "实时连接异常，显示最近行情";
        default:
          return "当前查询方式受限";
      }
    }
    case "unavailable":
      return "数据源不可用";
    default:
      return "等待行情订阅";
  }
}

function presentationConnectionLabel(transportMode: string): string {
  switch (transportMode) {
    case "push-stream":
      return "实时推送";
    case "snapshot-poll-delayed":
      return "HTTP 定时查询";
    case "snapshot-poll-fallback":
      return "快照轮询（推送回退）";
    case "idle":
      return "等待行情查询";
    default:
      return transportMode ? `行情查询（${transportMode}）` : "行情查询";
  }
}

/**
 * Converts raw transport capability into user-facing status. Delayed snapshots
 * are a provider's normal query mode, while fallback polling remains a warning.
 */
export function resolveMarketDataFeedPresentation(
  input: MarketDataFeedQualityInput,
): MarketDataFeedPresentation {
  const transportMode = normalizedTransportMode(input.transportMode);
  const rawState = resolveMarketDataFeedQuality(input);
  let connectionLabel = presentationConnectionLabel(transportMode);
  if (!transportMode && input.connectionState === "connected") {
    connectionLabel = "实时推送";
  }

  if (input.error?.trim()) {
    return {
      state: input.hasUsableData ? "stale" : "error",
      connectionLabel,
      qualityLabel: input.hasUsableData
        ? "查询异常，显示最近行情"
        : "查询失败，无可用行情",
      expected: false,
    };
  }
  if (input.fromCache) {
    return {
      state: "stale",
      connectionLabel: "缓存数据",
      qualityLabel: "正在使用缓存数据",
      expected: false,
    };
  }
  if (transportMode === "snapshot-poll-delayed") {
    return {
      state: "live",
      connectionLabel: "HTTP 定时查询",
      qualityLabel: "非实时快照，时效以供应商返回为准",
      expected: true,
    };
  }
  if (transportMode === "snapshot-poll-fallback") {
    return {
      state: "stale",
      connectionLabel,
      qualityLabel: "快照轮询（推送回退）",
      expected: false,
    };
  }

  switch (rawState) {
    case "healthy":
      return {
        state: "live",
        connectionLabel: connectionLabel || "实时推送",
        qualityLabel: "实时推送正常",
        expected: true,
      };
    case "unavailable":
      return {
        state: "error",
        connectionLabel,
        qualityLabel: "数据源不可用",
        expected: false,
      };
    case "idle":
      return {
        state: "empty",
        connectionLabel,
        qualityLabel: "等待行情订阅",
        expected: false,
      };
    default:
      return {
        state: input.connectionState === "connecting" ? "loading" : "stale",
        connectionLabel,
        qualityLabel: marketDataFeedQualityLabel(input, rawState),
        expected: false,
      };
  }
}
