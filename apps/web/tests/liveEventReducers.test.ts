import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createBacktestLiveReducer,
  createMarketDataLiveReducer,
  createNotificationLiveReducer,
  formatLiveEventTypeLabel,
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

  it("flushes only the latest market tick and ignores unrelated events", async () => {
    vi.useFakeTimers();
    const applyMarketDataTickEvent = vi.fn();
    const reducer = createMarketDataLiveReducer({
      applyMarketDataTickEvent,
      flushIntervalMs: 10,
    });

    expect(
      reducer.handle(
        createLiveEnvelope(
          {
            type: "system.notification",
            id: "notification-1",
            at: "2026-06-30T00:00:00.000Z",
            level: "info",
            title: "Ignored",
          },
          {
            source: "notification",
            entityId: "notification-1",
          },
        ),
      ),
    ).toBe(false);

    reducer.handle(
      createLiveEnvelope(
        {
          type: "market-data.tick",
          at: "2026-06-30T00:00:00.000Z",
          brokerId: "futu",
          instrument: {
            market: "HK",
            symbol: "00700",
            instrumentId: "HK.00700",
          },
          snapshot: {
            price: 380.5,
            bid: 380.4,
            ask: 380.6,
            volume: 10,
            turnover: 3805,
            at: "2026-06-30T00:00:00.000Z",
          },
          source: "bbgo:futu",
        },
        {
          source: "market-data",
          entityId: "HK.00700",
          eventId: "market-data.tick|HK.00700|1",
        },
      ),
    );
    reducer.handle(
      createLiveEnvelope(
        {
          type: "market-data.tick",
          at: "2026-06-30T00:00:01.000Z",
          brokerId: "futu",
          instrument: {
            market: "HK",
            symbol: "00700",
            instrumentId: "HK.00700",
          },
          snapshot: {
            price: 381,
            bid: 380.9,
            ask: 381.1,
            volume: 12,
            turnover: 4572,
            at: "2026-06-30T00:00:01.000Z",
          },
          source: "bbgo:futu",
        },
        {
          source: "market-data",
          entityId: "HK.00700",
          eventId: "market-data.tick|HK.00700|2",
        },
      ),
    );

    await vi.advanceTimersByTimeAsync(10);

    expect(applyMarketDataTickEvent).toHaveBeenCalledTimes(1);
    expect(applyMarketDataTickEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: expect.objectContaining({ price: 381 }),
      }),
    );

    reducer.dispose();
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

  it("normalizes notification payload defaults and routes matching backtest progress only", () => {
    const push = vi.fn();
    const loadSystemState = vi.fn();
    const reducer = createNotificationLiveReducer({
      notifications: { push },
      loadSystemState,
    });
    const applyProgress = vi.fn();
    const backtestReducer = createBacktestLiveReducer({
      activeTaskId: () => "sync-1",
      applyProgress,
    });

    expect(
      reducer.handle(
        createLiveEnvelope(
          {
            type: "system.notification",
            id: "notification-3",
            at: "",
            level: "unknown",
            title: "",
            brokerId: "futu",
            message: "Broker heartbeat",
          },
          {
            source: "notification",
            entityId: "notification-3",
            serverTime: "2026-06-30T00:00:02.000Z",
          },
        ),
      ),
    ).toBe(true);
    expect(push).toHaveBeenCalledWith({
      level: "info",
      title: "实时通知",
      source: "futu",
      at: "2026-06-30T00:00:02.000Z",
      message: "Broker heartbeat",
    });
    expect(loadSystemState).not.toHaveBeenCalled();

    expect(
      backtestReducer.handle(
        createLiveEnvelope(
          {
            type: "backtest.kline-sync.progress",
            taskId: "sync-2",
            status: "running",
          },
          {
            source: "backtest",
            entityId: "sync-2",
          },
        ),
      ),
    ).toBe(false);
    expect(
      backtestReducer.handle(
        createLiveEnvelope(
          {
            type: "backtest.kline-sync.progress",
            taskId: "sync-1",
            status: "completed",
            symbol: "HK.00700",
            currentInterval: "5m",
            totalIntervals: 1,
            completedIntervals: 1,
            totalBatches: 1,
            completedBatches: 1,
            retries: 0,
            startedAt: "2026-06-30T00:00:00.000Z",
            updatedAt: "2026-06-30T00:00:02.000Z",
          },
          {
            source: "backtest",
            entityId: "sync-1",
          },
        ),
      ),
    ).toBe(true);
    expect(applyProgress).toHaveBeenCalledWith(
      expect.objectContaining({ taskId: "sync-1", status: "completed" }),
    );

    expect(formatLiveEventTypeLabel("heartbeat")).toBe("心跳");
    expect(formatLiveEventTypeLabel("backtest.kline-sync.progress")).toBe(
      "回测任务",
    );
    expect(formatLiveEventTypeLabel("custom.event")).toBe("custom.event");
  });

  it("labels every visible event family and rejects malformed backtest stream messages", () => {
    expect(formatLiveEventTypeLabel("market-data.tick")).toBe("行情推送");
    expect(formatLiveEventTypeLabel("system.notification")).toBe("系统通知");
    expect(formatLiveEventTypeLabel("console.refresh")).toBe("控制台刷新");
    expect(formatLiveEventTypeLabel("market.security-details")).toBe("证券详情");
    expect(formatLiveEventTypeLabel("market.depth")).toBe("盘口");

    const applyProgress = vi.fn();
    const reducer = createBacktestLiveReducer({
      activeTaskId: () => "sync-1",
      applyProgress,
    });
    expect(reducer.handle({
      eventId: "malformed-backtest-event",
      type: "backtest.kline-sync.progress",
      source: "backtest",
      entityId: "sync-1",
      serverTime: "2026-07-16T00:00:00Z",
      payload: null,
    })).toBe(false);
    expect(applyProgress).not.toHaveBeenCalled();
  });
});
