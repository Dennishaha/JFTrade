import { describe, expect, it } from "vitest";

import {
  resolveMarketDataBarVolumeValue,
  resolveMarketDataBarVolumeUpdate,
  type MarketDataRealtimeBarVolumeState,
  type MarketDataRealtimeBarVolumeUpdateInput,
} from "../src/composables/marketDataRealtimeBarVolumeState";

function update(
  previousState: MarketDataRealtimeBarVolumeState | null,
  overrides: Partial<MarketDataRealtimeBarVolumeUpdateInput> = {},
) {
  return resolveMarketDataBarVolumeUpdate({
    previousState,
    instrumentId: "HK.00700",
    period: "1m",
    bucketAt: "2026-05-17T01:30:00.000Z",
    observedAt: "2026-05-17T01:30:05.000Z",
    cumulativeVolume: 1_282_000,
    existingCandleVolume: null,
    existingCandleUnfinalized: false,
    ...overrides,
  });
}

describe("marketDataRealtimeBarVolumeState", () => {
  it("clears state when there is no active bucket", () => {
    expect(update(null, { bucketAt: null })).toEqual({
      currentBarVolume: null,
      ignored: false,
      nextState: null,
    });
  });

  it("uses an existing candle but does not treat the first cumulative sample as bar volume", () => {
    const resolution = update(null, {
      existingCandleVolume: 240,
      existingCandleUnfinalized: true,
    });

    expect(resolution.currentBarVolume).toBe(240);
    expect(resolution.ignored).toBe(false);
    expect(resolution.nextState).toMatchObject({
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      currentBarVolume: 240,
      sequence: { lastCumulativeVolume: 1_282_000 },
    });
  });

  it("adds a cumulative increment within the same bucket", () => {
    const first = update(null);
    const second = update(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });

    expect(second.currentBarVolume).toBe(200);
    expect(second.nextState?.sequence.lastCumulativeVolume).toBe(1_282_200);
  });

  it("carries the cumulative sequence into the next bucket", () => {
    const first = update(null);
    const second = update(first.nextState, {
      bucketAt: "2026-05-17T01:31:00.000Z",
      observedAt: "2026-05-17T01:31:05.000Z",
      cumulativeVolume: 1_282_400,
    });

    expect(second.currentBarVolume).toBe(400);
    expect(second.nextState?.bucketAt).toBe("2026-05-17T01:31:00.000Z");
  });

  it("does not count a repeated cumulative sample twice", () => {
    const first = update(null);
    const second = update(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });
    const duplicate = update(second.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });
    const unchangedLaterSample = update(duplicate.nextState, {
      observedAt: "2026-05-17T01:30:30.000Z",
      cumulativeVolume: 1_282_200,
    });

    expect(duplicate.currentBarVolume).toBe(200);
    expect(unchangedLaterSample.currentBarVolume).toBe(200);
  });

  it("rebases a newer cumulative rollback as a new sequence", () => {
    const first = update(null);
    const second = update(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });
    const reset = update(second.nextState, {
      observedAt: "2026-05-17T01:30:30.000Z",
      cumulativeVolume: 100,
    });
    const afterReset = update(reset.nextState, {
      observedAt: "2026-05-17T01:30:40.000Z",
      cumulativeVolume: 160,
    });

    expect(reset.currentBarVolume).toBe(200);
    expect(reset.nextState?.sequence.lastCumulativeVolume).toBe(100);
    expect(afterReset.currentBarVolume).toBe(260);
  });

  it("uses an explicit delta on the first sample", () => {
    const resolution = update(null, {
      cumulativeVolume: null,
      volumeDelta: 35,
    });

    expect(resolution.currentBarVolume).toBe(35);
  });

  it("ignores an out-of-order event and older bucket", () => {
    const current = update(null, {
      bucketAt: "2026-05-17T01:31:00.000Z",
      observedAt: "2026-05-17T01:31:20.000Z",
    }).nextState;
    const resolution = update(current, {
      bucketAt: "2026-05-17T01:30:00.000Z",
      observedAt: "2026-05-17T01:30:40.000Z",
      cumulativeVolume: 1_281_900,
    });

    expect(resolution).toEqual({
      currentBarVolume: null,
      ignored: true,
      nextState: current,
    });
  });

  it("merges state volume with a newer existing candle without snapshot input", () => {
    const state = update(null, { volumeDelta: 20 }).nextState;
    expect(state).not.toBeNull();
    expect(resolveMarketDataBarVolumeValue(state!, 240)).toBe(240);
  });

  it("rejects invalid observation clocks and advances a known cumulative baseline with deltas", () => {
    const first = update(null);
    const invalid = update(first.nextState, { observedAt: "not-a-clock" });
    expect(invalid).toEqual({
      currentBarVolume: null,
      ignored: true,
      nextState: first.nextState,
    });

    const delta = update(first.nextState, {
      observedAt: "2026-05-17T01:30:10.000Z",
      cumulativeVolume: null,
      volumeDelta: 25,
    });
    expect(delta.currentBarVolume).toBe(25);
    expect(delta.nextState?.sequence.lastCumulativeVolume).toBe(1_282_025);
  });
});
