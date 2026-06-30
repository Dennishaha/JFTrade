import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createMarketDataLiveReducer,
  createNotificationLiveReducer,
} from "../src/composables/liveEventReducers";
import { getLiveEventBus, resetLiveEventBusForTests } from "../src/composables/liveEventBus";
import { createLiveEnvelope } from "./helpers";

afterEach(() => {
  resetLiveEventBusForTests();
  vi.useRealTimers();
});

describe("live event reducers", () => {
  it("patches market data state from unified bus events", async () => {
    vi.useFakeTimers();
    const applyMarketDataTickEvent = vi.fn();
    const reducer = createMarketDataLiveReducer({
      applyMarketDataTickEvent,
      flushIntervalMs: 10,
    });
    const stop = getLiveEventBus().subscribe(reducer.handle);

    const payload = {
      type: "market-data.tick",
      at: "2026-06-30T00:00:00.000Z",
      brokerId: "futu",
      instrument: { market: "HK", symbol: "00700", instrumentId: "HK.00700" },
      snapshot: { price: 380.5, bid: 380.4, ask: 380.6, volume: 10, turnover: 3805, at: "2026-06-30T00:00:00.000Z" },
      source: "bbgo:futu",
    };

    getLiveEventBus().publishRaw(createLiveEnvelope(payload, {
      source: "market-data",
      entityId: "HK.00700",
      eventId: "market-data.tick|HK.00700|2026-06-30T00:00:00.000Z",
    }));
    await vi.advanceTimersByTimeAsync(10);

    expect(applyMarketDataTickEvent).toHaveBeenCalledWith(payload);

    reducer.dispose();
    stop();
  });

  it("routes broker order notifications to the notification sink and system refresh", () => {
    const push = vi.fn();
    const loadSystemState = vi.fn();
    const reducer = createNotificationLiveReducer({
      notifications: { push },
      loadSystemState,
    });
    const stop = getLiveEventBus().subscribe(reducer.handle);

    const payload = {
      type: "system.notification",
      id: "system-notification-2",
      at: "2026-06-30T00:00:01.000Z",
      level: "success",
      title: "订单已提交",
      source: "execution-orders",
      category: "broker.order.submitted",
      brokerId: "futu",
    };

    getLiveEventBus().publishRaw(createLiveEnvelope(payload, {
      source: "notification",
      entityId: "system-notification-2",
      eventId: "system-notification-2",
    }));

    expect(push).toHaveBeenCalledWith(expect.objectContaining({
      level: "success",
      title: "订单已提交",
      source: "execution-orders",
      category: "broker.order.submitted",
    }));
    expect(loadSystemState).toHaveBeenCalledWith({
      background: true,
      bypassCooldown: true,
    });

    stop();
  });
});
