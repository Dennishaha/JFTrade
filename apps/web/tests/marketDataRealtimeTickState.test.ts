import { describe, expect, it } from "vitest";

import {
  resolveMarketDataTickVolumeUpdate,
  type MarketDataRealtimeTickVolumeState,
} from "../src/composables/marketDataRealtimeTickState";

describe("marketDataRealtimeTickState", () => {
  it("returns zero delta for the first sample", () => {
    expect(resolveMarketDataTickVolumeUpdate(null, "HK.00700", 1282000)).toEqual(
      {
        deltaVolume: 0,
        nextState: {
          instrumentId: "HK.00700",
          lastCumulativeVolume: 1282000,
        },
      },
    );
  });

  it("returns incremental delta for the same instrument", () => {
    const previousState: MarketDataRealtimeTickVolumeState = {
      instrumentId: "HK.00700",
      lastCumulativeVolume: 1282000,
    };

    expect(
      resolveMarketDataTickVolumeUpdate(previousState, "HK.00700", 1282200),
    ).toEqual({
      deltaVolume: 200,
      nextState: {
        instrumentId: "HK.00700",
        lastCumulativeVolume: 1282200,
      },
    });
  });

  it("returns zero delta when cumulative volume moves backwards", () => {
    const previousState: MarketDataRealtimeTickVolumeState = {
      instrumentId: "HK.00700",
      lastCumulativeVolume: 1282200,
    };

    expect(
      resolveMarketDataTickVolumeUpdate(previousState, "HK.00700", 1282100),
    ).toEqual({
      deltaVolume: 0,
      nextState: {
        instrumentId: "HK.00700",
        lastCumulativeVolume: 1282100,
      },
    });
  });

  it("returns zero delta when the instrument changes", () => {
    const previousState: MarketDataRealtimeTickVolumeState = {
      instrumentId: "HK.00700",
      lastCumulativeVolume: 1282200,
    };

    expect(
      resolveMarketDataTickVolumeUpdate(previousState, "US.AAPL", 981000),
    ).toEqual({
      deltaVolume: 0,
      nextState: {
        instrumentId: "US.AAPL",
        lastCumulativeVolume: 981000,
      },
    });
  });

  it("keeps previous state when cumulative volume is invalid", () => {
    const previousState: MarketDataRealtimeTickVolumeState = {
      instrumentId: "HK.00700",
      lastCumulativeVolume: 1282200,
    };

    expect(
      resolveMarketDataTickVolumeUpdate(previousState, "HK.00700", Number.NaN),
    ).toEqual({
      deltaVolume: 0,
      nextState: previousState,
    });
  });
});