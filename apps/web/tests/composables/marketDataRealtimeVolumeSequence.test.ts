import { describe, expect, it } from "vitest";

import {
  resolveMarketDataVolumeSequence,
  type MarketDataRealtimeVolumeSequenceInput,
  type MarketDataRealtimeVolumeSequenceState,
} from "@/composables/market-data/marketDataRealtimeVolumeSequence";

function resolve(
  previousState: MarketDataRealtimeVolumeSequenceState | null,
  overrides: Partial<MarketDataRealtimeVolumeSequenceInput> = {},
) {
  return resolveMarketDataVolumeSequence({
    previousState,
    observedAt: "2026-05-17T01:30:00.000Z",
    cumulativeVolume: 1_282_000,
    ...overrides,
  });
}

describe("marketDataRealtimeVolumeSequence", () => {
  it("rejects an unparseable observation clock and keeps the previous state untouched", () => {
    const established = resolve(null).nextState;

    expect(resolve(null, { observedAt: "not-a-clock" })).toEqual({
      deltaVolume: 0,
      ignored: true,
      isDuplicate: false,
      source: "none",
      nextState: null,
    });
    expect(resolve(established, { observedAt: "" })).toEqual({
      deltaVolume: 0,
      ignored: true,
      isDuplicate: false,
      source: "none",
      nextState: established,
    });
  });

  it("seeds the cumulative baseline on the first sample without emitting volume", () => {
    const resolution = resolve(null);

    expect(resolution).toMatchObject({
      deltaVolume: 0,
      ignored: false,
      isDuplicate: false,
      source: "none",
    });
    expect(resolution.nextState).toEqual({
      lastCumulativeVolume: 1_282_000,
      lastObservedAt: "2026-05-17T01:30:00.000Z",
      lastObservedAtMs: Date.parse("2026-05-17T01:30:00.000Z"),
      lastSampleCumulativeVolume: 1_282_000,
      lastSampleVolumeDelta: null,
    });
  });

  it("uses an explicit delta to price the first sample of a new cumulative sequence", () => {
    const resolution = resolve(null, { volumeDelta: 250 });

    expect(resolution.deltaVolume).toBe(250);
    expect(resolution.source).toBe("delta");
    expect(resolution.nextState?.lastCumulativeVolume).toBe(1_282_000);
  });

  it("advances the sequence by the cumulative difference", () => {
    const first = resolve(null);
    const second = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });

    expect(second).toMatchObject({
      deltaVolume: 200,
      ignored: false,
      isDuplicate: false,
      source: "cumulative",
    });
    expect(second.nextState?.lastCumulativeVolume).toBe(1_282_200);
  });

  it("drops an out-of-order sample and keeps the established sequence", () => {
    const first = resolve(null);
    const second = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });
    const stale = resolve(second.nextState, {
      observedAt: "2026-05-17T01:30:10.000Z",
      cumulativeVolume: 9_999_999,
      volumeDelta: 42,
    });

    expect(stale).toEqual({
      deltaVolume: 0,
      ignored: true,
      isDuplicate: false,
      source: "none",
      nextState: second.nextState,
    });
  });

  it("drops a cumulative rollback observed at the same clock tick", () => {
    const first = resolve(null);
    const rollback = resolve(first.nextState, {
      cumulativeVolume: 100,
    });

    expect(rollback).toEqual({
      deltaVolume: 0,
      ignored: true,
      isDuplicate: false,
      source: "none",
      nextState: first.nextState,
    });
  });

  it("rebases a newer cumulative rollback as a new sequence", () => {
    const first = resolve(null);
    const reset = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:30.000Z",
      cumulativeVolume: 100,
    });
    const afterReset = resolve(reset.nextState, {
      observedAt: "2026-05-17T01:30:40.000Z",
      cumulativeVolume: 160,
    });

    expect(reset).toMatchObject({
      deltaVolume: 0,
      ignored: false,
      source: "none",
    });
    expect(reset.nextState?.lastCumulativeVolume).toBe(100);
    expect(afterReset).toMatchObject({
      deltaVolume: 60,
      source: "cumulative",
    });
  });

  it("falls back to the explicit delta when a newer sample reopens the sequence", () => {
    const first = resolve(null);
    const reset = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:30.000Z",
      cumulativeVolume: 100,
      volumeDelta: 17,
    });

    expect(reset).toMatchObject({
      deltaVolume: 17,
      ignored: false,
      source: "delta",
    });
    expect(reset.nextState?.lastCumulativeVolume).toBe(100);
  });

  it("flags an exact same-tick replay as duplicate without adding volume", () => {
    const first = resolve(null, { volumeDelta: 30 });
    const replay = resolve(first.nextState, { volumeDelta: 30 });

    expect(replay).toMatchObject({
      deltaVolume: 0,
      ignored: false,
      isDuplicate: true,
      source: "none",
    });
    expect(replay.nextState?.lastCumulativeVolume).toBe(1_282_000);
  });

  it("does not confuse a same-value later sample with a duplicate", () => {
    const first = resolve(null);
    const second = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: 1_282_200,
    });
    const unchangedLater = resolve(second.nextState, {
      observedAt: "2026-05-17T01:30:30.000Z",
      cumulativeVolume: 1_282_200,
    });

    expect(unchangedLater).toMatchObject({
      deltaVolume: 0,
      isDuplicate: false,
      source: "cumulative",
    });
  });

  it("tracks delta-only samples without inventing a cumulative baseline", () => {
    const first = resolve(null, { cumulativeVolume: null, volumeDelta: 35 });
    const second = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: null,
      volumeDelta: 15,
    });

    expect(first).toMatchObject({ deltaVolume: 35, source: "delta" });
    expect(first.nextState?.lastCumulativeVolume).toBeNull();
    expect(second).toMatchObject({ deltaVolume: 15, source: "delta" });
    expect(second.nextState?.lastCumulativeVolume).toBeNull();
  });

  it("advances a known cumulative baseline with delta-only samples", () => {
    const first = resolve(null);
    const deltaSample = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: null,
      volumeDelta: 20,
    });
    const cumulativeCatchUp = resolve(deltaSample.nextState, {
      observedAt: "2026-05-17T01:30:30.000Z",
      cumulativeVolume: 1_282_050,
    });

    expect(deltaSample).toMatchObject({ deltaVolume: 20, source: "delta" });
    expect(deltaSample.nextState?.lastCumulativeVolume).toBe(1_282_020);
    expect(cumulativeCatchUp).toMatchObject({
      deltaVolume: 30,
      source: "cumulative",
    });
  });

  it("discards non-finite or negative volume fields instead of corrupting the sequence", () => {
    const first = resolve(null);
    const invalid = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: Number.NaN,
      volumeDelta: -5,
    });

    expect(invalid).toMatchObject({
      deltaVolume: 0,
      ignored: false,
      isDuplicate: false,
      source: "none",
    });
    expect(invalid.nextState?.lastCumulativeVolume).toBe(1_282_000);
    expect(invalid.nextState?.lastSampleCumulativeVolume).toBeNull();
    expect(invalid.nextState?.lastSampleVolumeDelta).toBeNull();
  });

  it("still applies a valid delta when the cumulative field is unusable", () => {
    const first = resolve(null);
    const resolution = resolve(first.nextState, {
      observedAt: "2026-05-17T01:30:20.000Z",
      cumulativeVolume: Number.POSITIVE_INFINITY,
      volumeDelta: 12,
    });

    expect(resolution).toMatchObject({ deltaVolume: 12, source: "delta" });
    expect(resolution.nextState?.lastCumulativeVolume).toBe(1_282_012);
  });
});
