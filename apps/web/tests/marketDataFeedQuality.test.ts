import { describe, expect, it } from "vitest";

import {
  marketDataFeedQualityLabel,
  resolveMarketDataFeedQuality,
  type MarketDataFeedQualityInput,
  type MarketDataFeedQualityState,
} from "../src/composables/marketDataFeedQuality";

function qualityInput(
  patch: Partial<MarketDataFeedQualityInput> = {},
): MarketDataFeedQualityInput {
  return {
    connectionState: "connected",
    hasUsableData: true,
    ...patch,
  };
}

describe("market data feed quality", () => {
  it.each([
    [qualityInput(), "healthy"],
    [qualityInput({ fromCache: true }), "degraded"],
    [qualityInput({ transportMode: " SNAPSHOT-POLL-FALLBACK " }), "degraded"],
    [qualityInput({ connectionState: "connecting" }), "degraded"],
    [qualityInput({ connectionState: "disconnected" }), "degraded"],
    [qualityInput({ connectionState: "unsupported" }), "degraded"],
    [qualityInput({ connectionState: "error" }), "degraded"],
    [qualityInput({ connectionState: "error", hasUsableData: false }), "unavailable"],
    [qualityInput({ error: "feed failed", hasUsableData: false }), "unavailable"],
    [qualityInput({ connectionState: "idle" }), "idle"],
    [qualityInput({ connectionState: "idle", transportMode: "custom" }), "degraded"],
  ] as const)("resolves %# to %s", (input, expected) => {
    expect(resolveMarketDataFeedQuality(input)).toBe(expected);
  });

  it.each([
    [qualityInput(), "healthy", "实时推送正常"],
    [qualityInput({ fromCache: true }), "degraded", "正在使用缓存数据"],
    [
      qualityInput({ transportMode: "snapshot-poll-fallback" }),
      "degraded",
      "已降级到轮询",
    ],
    [qualityInput({ connectionState: "connecting" }), "degraded", "实时连接中"],
    [
      qualityInput({ connectionState: "disconnected" }),
      "degraded",
      "实时连接已中断",
    ],
    [
      qualityInput({ connectionState: "unsupported" }),
      "degraded",
      "数据源不支持实时推送",
    ],
    [
      qualityInput({ connectionState: "error" }),
      "degraded",
      "实时连接异常，显示最近行情",
    ],
    [qualityInput({ transportMode: "custom" }), "degraded", "行情传输已降级"],
    [qualityInput(), "unavailable", "数据源不可用"],
    [qualityInput(), "idle", "等待行情订阅"],
  ] as const)("labels %# as %s", (input, state, expected) => {
    expect(
      marketDataFeedQualityLabel(
        input,
        state as MarketDataFeedQualityState,
      ),
    ).toBe(expected);
  });
});
