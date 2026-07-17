import { afterEach, describe, expect, it } from "vitest";

import {
  getLiveEventBus,
  parseLiveEventEnvelope,
  resetLiveEventBusForTests,
} from "../src/composables/liveEventBus";

afterEach(() => {
  resetLiveEventBusForTests();
});

describe("liveEventBus", () => {
  it("rejects legacy live payloads without an explicit envelope", () => {
    const envelope = parseLiveEventEnvelope({
      type: "market-data.tick",
      at: "2026-06-30T00:00:00.000Z",
      instrument: { market: "HK", symbol: "00700", instrumentId: "HK.00700" },
      snapshot: { price: 380.5, bid: 380.4, ask: 380.6, volume: 10, turnover: 3805, at: "2026-06-30T00:00:00.000Z" },
      source: "bbgo:futu",
    });

    expect(envelope).toBeNull();
  });

  it("accepts explicit envelopes while preserving payload fields", () => {
    const payload = {
      type: "market-data.tick",
      at: "2026-06-30T00:00:00.000Z",
      instrument: { market: "HK", symbol: "00700", instrumentId: "HK.00700" },
      snapshot: { price: 380.5, bid: 380.4, ask: 380.6, volume: 10, turnover: 3805, at: "2026-06-30T00:00:00.000Z" },
      source: "bbgo:futu",
    };

    const envelope = parseLiveEventEnvelope({
      eventId: "market-tick-1",
      type: "market-data.tick",
      source: "market-data",
      entityId: "HK.00700",
      serverTime: "2026-06-30T00:00:00.000Z",
      payload,
    });

    expect(envelope).toMatchObject({
      eventId: "market-tick-1",
      source: "market-data",
      entityId: "HK.00700",
    });
    expect(envelope?.payload).toMatchObject({
      type: "market-data.tick",
      source: "bbgo:futu",
    });
  });

  it("rejects malformed envelopes before they can affect live reducers", () => {
    expect(parseLiveEventEnvelope(null)).toBeNull();
    expect(parseLiveEventEnvelope({})).toBeNull();
    expect(parseLiveEventEnvelope({ type: "market-data.tick" })).toBeNull();
    expect(parseLiveEventEnvelope({
      eventId: "missing-payload",
      type: "market-data.tick",
      source: "market-data",
      entityId: "US.AAPL",
      serverTime: "2026-07-16T00:00:00Z",
    })).toBeNull();
    expect(parseLiveEventEnvelope({
      eventId: "invalid-source",
      type: "market-data.tick",
      source: "unknown",
      entityId: "US.AAPL",
      serverTime: "2026-07-16T00:00:00Z",
      payload: {},
    })).toBeNull();
  });

  it("drops duplicate event ids before notifying reducers", () => {
    const bus = getLiveEventBus();
    const events: string[] = [];
    bus.subscribe((event) => events.push(event.eventId));

    const payload = {
      type: "system.notification",
      id: "system-notification-1",
      at: "2026-06-30T00:00:00.000Z",
      level: "info",
      title: "Ready",
    };
    const envelope = {
      eventId: "system-notification-1",
      type: "system.notification",
      source: "notification" as const,
      entityId: "system-notification-1",
      serverTime: "2026-06-30T00:00:00.000Z",
      payload,
    };
    expect(bus.publishRaw(envelope)).not.toBeNull();
    expect(bus.publishRaw(envelope)).toBeNull();

    expect(events).toEqual(["system-notification-1"]);
  });

  it("drops out-of-order versions per source type and entity", () => {
    const bus = getLiveEventBus();
    const versions: number[] = [];
    bus.subscribe((event) => versions.push(event.version ?? 0));

    bus.publish({
      eventId: "sync-2",
      type: "backtest.kline-sync.progress",
      source: "backtest",
      entityId: "sync-1",
      version: 2,
      serverTime: "2026-06-30T00:00:02.000Z",
      payload: { taskId: "sync-1", status: "running" },
    });
    bus.publish({
      eventId: "sync-1",
      type: "backtest.kline-sync.progress",
      source: "backtest",
      entityId: "sync-1",
      version: 1,
      serverTime: "2026-06-30T00:00:01.000Z",
      payload: { taskId: "sync-1", status: "queued" },
    });

    expect(versions).toEqual([2]);
  });

  it("bounds the duplicate-event cache so an old event can be observed after eviction", () => {
    const bus = getLiveEventBus();
    for (let index = 0; index <= 500; index += 1) {
      expect(bus.publish({
        eventId: `event-${index}`,
        type: "system.notification",
        source: "system",
        entityId: "console",
        serverTime: "2026-07-16T00:00:00Z",
        payload: { index },
      })).toBe(true);
    }

    expect(bus.publish({
      eventId: "event-0",
      type: "system.notification",
      source: "system",
      entityId: "console",
      serverTime: "2026-07-16T00:00:01Z",
      payload: { replayedAfterRetentionWindow: true },
    })).toBe(true);
  });
});
