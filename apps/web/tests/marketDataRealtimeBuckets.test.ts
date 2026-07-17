import { describe, expect, it } from "vitest";

import {
  finalizeMarketDataRealtimeCandleDisplayAt,
  resolveMarketDataRealtimeBucketStart,
} from "../src/composables/marketDataRealtimeBuckets";

describe("market data realtime bucket boundaries", () => {
  it("does not fabricate a chart bucket for tick-only updates", () => {
    expect(
      resolveMarketDataRealtimeBucketStart("tick", [], {
        price: 42,
        volume: 3,
        at: "2026-06-02T09:31:12.000Z",
      }),
    ).toBeNull();
  });

  it("keeps absent or non-displayable series unchanged", () => {
    expect(
      finalizeMarketDataRealtimeCandleDisplayAt("1m", "2026-06-02T09:31:00.000Z", null),
    ).toBeNull();

    const dailySeries = {
      candles: [
        {
          period: "1d",
          at: "2026-06-02T00:00:00.000Z",
          open: 40,
          high: 43,
          low: 39,
          close: 42,
          volume: 100,
        },
      ],
    };

    // Daily bars are already represented by their trading date. Assigning an
    // intraday display end here would move the candle on the chart timeline.
    expect(
      finalizeMarketDataRealtimeCandleDisplayAt(
        "1d",
        "2026-06-02T00:00:00.000Z",
        dailySeries,
      ),
    ).toBe(dailySeries);
  });
});
