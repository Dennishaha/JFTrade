import type { LiveSocketConnectionState } from "./sharedLiveSocket";

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

function normalizedTransportMode(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? "";
}

export function resolveMarketDataFeedQuality(
  input: MarketDataFeedQualityInput,
): MarketDataFeedQualityState {
  const transportMode = normalizedTransportMode(input.transportMode);
  if (input.error?.trim() && !input.hasUsableData) return "unavailable";
  if (input.fromCache || transportMode === "snapshot-poll-fallback") {
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
        return "已降级到轮询";
      }
      switch (input.connectionState) {
        case "connecting":
          return "实时连接中";
        case "disconnected":
          return "实时连接已中断";
        case "unsupported":
          return "数据源不支持实时推送";
        case "error":
          return "实时连接异常，显示最近行情";
        default:
          return "行情传输已降级";
      }
    }
    case "unavailable":
      return "数据源不可用";
    default:
      return "等待行情订阅";
  }
}
