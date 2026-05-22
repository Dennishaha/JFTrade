import { describe, expect, it } from "vitest";

import {
  resolveMarketDataRealtimeTickBucketAt,
  resolveMarketDataRealtimeTickObservedAt,
} from "../src/composables/marketDataRealtimeTickContext";

describe("marketDataRealtimeTickContext", () => {
  it("prefers snapshot observedAt over websocket event time", () => {
    expect(
      resolveMarketDataRealtimeTickObservedAt({
        eventAt: "2026-05-17T01:30:05.000Z",
        snapshot: {
          at: "2026-05-17T01:29:59.000Z",
          observedAt: "2026-05-17T01:30:04.000Z",
        },
      }),
    ).toBe("2026-05-17T01:30:04.000Z");
  });

  it("uses websocket event time to resolve the next realtime bucket", () => {
    expect(
      resolveMarketDataRealtimeTickBucketAt({
        period: "1m",
        candles: [
          {
            period: "1m",
            open: 320,
            high: 320.8,
            low: 319.9,
            close: 320.5,
            volume: 18000,
            at: "2026-05-17T01:29:00.000Z",
          },
        ],
        eventAt: "2026-05-17T01:30:05.000Z",
        snapshot: {
          price: 321.8,
          volume: 1282000,
          at: "2026-05-17T01:29:59.000Z",
        },
      }),
    ).toBe("2026-05-17T01:30:00.000Z");
  });
});