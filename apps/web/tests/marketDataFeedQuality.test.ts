import { describe, expect, it } from "vitest";

import {
  marketDataFeedQualityLabel,
  resolveMarketDataFeedPresentation,
  resolveMarketDataFeedQuality,
  type MarketDataFeedQualityInput,
  type MarketDataFeedQualityState,
} from "@/composables/market-data/marketDataFeedQuality";

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
      "快照轮询（推送回退）",
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
      "不支持推送，使用快照行情",
    ],
    [
      qualityInput({ connectionState: "error" }),
      "degraded",
      "实时连接异常，显示最近行情",
    ],
    [qualityInput({ transportMode: "custom" }), "degraded", "当前查询方式受限"],
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

  it("presents delayed snapshots as an expected HTTP query", () => {
    const presentation = resolveMarketDataFeedPresentation(
      qualityInput({ transportMode: "snapshot-poll-delayed" }),
    );

    expect(presentation).toEqual({
      state: "live",
      connectionLabel: "HTTP 定时查询",
      qualityLabel: "非实时快照，时效以供应商返回为准",
      expected: true,
    });
  });

  it("keeps push fallback as a warning with specific wording", () => {
    const presentation = resolveMarketDataFeedPresentation(
      qualityInput({ transportMode: "snapshot-poll-fallback" }),
    );

    expect(presentation.state).toBe("stale");
    expect(presentation.expected).toBe(false);
    expect(presentation.connectionLabel).toBe("快照轮询（推送回退）");
    expect(presentation.qualityLabel).toBe("快照轮询（推送回退）");
  });
});
