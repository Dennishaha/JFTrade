import { describe, expect, it } from "vitest";

import {
  resolveMarketDataTickVolumeUpdate,
  type MarketDataRealtimeTickVolumeState,
  type MarketDataRealtimeTickVolumeUpdateInput,
} from "../src/composables/marketDataRealtimeTickState";

function update(
  previousState: MarketDataRealtimeTickVolumeState | null,
  overrides: Partial<MarketDataRealtimeTickVolumeUpdateInput> = {},
) {
  return resolveMarketDataTickVolumeUpdate({
    previousState,
    instrumentId: "HK.00700",
    bucketAt: "2026-05-17T01:30:05.000Z",
    observedAt: "2026-05-17T01:30:05.000Z",
    cumulativeVolume: 1_282_000,
    ...overrides,
  });
}

describe("marketDataRealtimeTickState", () => {
  it("returns zero for the first cumulative sample", () => {
    const resolution = update(null);

    expect(resolution.deltaVolume).toBe(0);
    expect(resolution.ignored).toBe(false);
    expect(resolution.nextState).toMatchObject({
      instrumentId: "HK.00700",
      period: "tick",
      bucketAt: "2026-05-17T01:30:05.000Z",
      currentSampleVolume: 0,
      sequence: { lastCumulativeVolume: 1_282_000 },
    });
  });

  it("returns the incremental volume for the next cumulative sample", () => {
    const first = update(null);
    const second = update(first.nextState, {
      bucketAt: "2026-05-17T01:30:20.000Z",
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });

    expect(second.deltaVolume).toBe(200);
  });

  it("uses an explicit delta on the first sample", () => {
    expect(
      update(null, { cumulativeVolume: null, volumeDelta: 12 }).deltaVolume,
    ).toBe(12);
  });

  it("does not erase a same-timestamp tick when the sample is repeated", () => {
    const first = update(null, { cumulativeVolume: null, volumeDelta: 12 });
    const duplicate = update(first.nextState, {
      cumulativeVolume: null,
      volumeDelta: 12,
    });

    expect(duplicate.deltaVolume).toBe(12);
    expect(duplicate.nextState?.currentSampleVolume).toBe(12);
  });

  it("starts a new sequence when a newer cumulative sample moves backwards", () => {
    const first = update(null);
    const second = update(first.nextState, {
      bucketAt: "2026-05-17T01:30:20.000Z",
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });
    const reset = update(second.nextState, {
      bucketAt: "2026-05-17T01:30:30.000Z",
      observedAt: "2026-05-17T01:30:30.000Z",
      cumulativeVolume: 100,
    });
    const afterReset = update(reset.nextState, {
      bucketAt: "2026-05-17T01:30:40.000Z",
      observedAt: "2026-05-17T01:30:40.000Z",
      cumulativeVolume: 160,
    });

    expect(reset.deltaVolume).toBe(0);
    expect(afterReset.deltaVolume).toBe(60);
  });

  it("ignores an out-of-order sample", () => {
    const current = update(null, {
      bucketAt: "2026-05-17T01:30:20.000Z",
      observedAt: "2026-05-17T01:30:20.000Z",
    }).nextState;
    const old = update(current, {
      bucketAt: "2026-05-17T01:30:10.000Z",
      observedAt: "2026-05-17T01:30:10.000Z",
      cumulativeVolume: 1_281_900,
    });

    expect(old).toEqual({
      deltaVolume: 0,
      ignored: true,
      nextState: current,
    });
  });

  it("does not carry a cumulative sequence across instruments", () => {
    const first = update(null);
    const switched = update(first.nextState, {
      instrumentId: "US.AAPL",
      bucketAt: "2026-05-17T01:30:20.000Z",
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 981_000,
    });

    expect(switched.deltaVolume).toBe(0);
    expect(switched.nextState?.instrumentId).toBe("US.AAPL");
  });
});
