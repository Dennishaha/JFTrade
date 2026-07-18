// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  getSharedLiveSocketHub,
  resetSharedLiveSocketHubForTests,
} from "../src/composables/sharedLiveSocket";
import { createLiveEnvelope, MockWebSocket } from "./helpers";

afterEach(() => {
  resetSharedLiveSocketHubForTests();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("SharedLiveSocketHub", () => {
  it("reports unsupported sockets and does not reconnect without an active url", () => {
    vi.stubGlobal("WebSocket", undefined as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();

    expect(hub.connect("ws://127.0.0.1:3000/api/v1/ws/live")).toBeNull();
    expect(hub.connectionState.value).toBe("unsupported");
    expect(hub.reconnect()).toBeNull();
  });

  it("buffers recent events, updates heartbeats, and removes listeners cleanly", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();
    const listener = vi.fn();
    const removeListener = hub.addEventListener(listener);

    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    for (let index = 0; index < 25; index += 1) {
      const at = `2026-07-03T00:${String(index).padStart(2, "0")}:00.000Z`;
      MockWebSocket.instances[0]?.emitMessage(
        createLiveEnvelope(
          {
            type: "heartbeat",
            at,
            transport: {
              mode: "push-stream",
              activeInstruments: 1,
              freshInstruments: 1,
              staleInstruments: 0,
            },
          },
          {
            source: "system",
            entityId: "live-heartbeat",
            eventId: `heartbeat-${index}`,
          },
        ),
      );
    }

    expect(listener).toHaveBeenCalledTimes(25);
    expect(hub.events.value).toHaveLength(20);
    expect(hub.events.value[0]).toMatchObject({
      type: "heartbeat",
      at: "2026-07-03T00:05:00.000Z",
    });
    expect(hub.lastHeartbeat.value).toBe("2026-07-03T00:24:00.000Z");
    expect(hub.lastHeartbeatEvent.value?.transport?.mode).toBe("push-stream");
    expect(hub.lastHeartbeatEvent.value?.transport?.freshInstruments).toBe(1);

    removeListener();
    MockWebSocket.instances[0]?.emitMessage(
      createLiveEnvelope(
        {
          type: "heartbeat",
          at: "2026-07-03T00:25:00.000Z",
        },
        {
          source: "system",
          entityId: "live-heartbeat",
          eventId: "heartbeat-25",
        },
      ),
    );

    expect(listener).toHaveBeenCalledTimes(25);
  });

  it("ignores non-string messages and marks payload type mismatches as errors", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    const socket = MockWebSocket.instances[0] as unknown as {
      dispatchEvent: (type: string, event: MessageEvent<unknown>) => void;
    };

    socket.dispatchEvent("message", { data: { raw: true } } as MessageEvent);
    expect(hub.connectionState.value).toBe("connected");
    expect(hub.events.value).toEqual([]);

    socket.dispatchEvent(
      "message",
      {
        data: JSON.stringify({
          ...createLiveEnvelope(
            {
              type: "heartbeat",
              at: "2026-07-03T12:00:00.000Z",
            },
            {
              source: "system",
              entityId: "live-heartbeat",
              eventId: "heartbeat-mismatch",
            },
          ),
          payload: {
            type: "system.notification",
            id: "notification-1",
            at: "2026-07-03T12:00:00.000Z",
            level: "info",
            title: "Mismatch",
          },
        }),
      } as MessageEvent<string>,
    );

    expect(hub.connectionState.value).toBe("error");
  });

  it("normalizes subscriptions, clamps depth, and avoids resending identical snapshots", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();
    const activeOwnerA = hub.createOwnerId("chart");
    const activeOwnerB = hub.createOwnerId("watchlist");
    const securityOwner = hub.createOwnerId("details");
    const depthOwnerA = hub.createOwnerId("depth");
    const depthOwnerB = hub.createOwnerId("depth");
    const refreshOwner = hub.createOwnerId("console");
    const providerOwner = hub.createOwnerId("provider");

    hub.setProviderBrokerId(providerOwner, " alpha ");
    hub.setActiveInstrument(activeOwnerA, " us.aapl ");
    hub.setActiveInstrument(activeOwnerB, "US.AAPL");
    hub.setSecurityDetailsTarget(securityOwner, {
      market: " us ",
      symbol: " aapl ",
      instrumentId: " us.aapl ",
    });
    hub.setDepthTarget(depthOwnerA, {
      market: " us ",
      symbol: " aapl ",
      instrumentId: " us.aapl ",
      num: 99.8,
    });
    hub.setDepthTarget(depthOwnerB, {
      market: " us ",
      symbol: " aapl ",
      instrumentId: " us.aapl ",
      num: 0.4,
    });
    hub.setConsoleRefreshEnabled(refreshOwner, true);

    expect(hub.snapshotSubscriptions()).toEqual({
      providerBrokerId: "alpha",
      activeInstruments: ["US.AAPL"],
      securityDetails: [
        {
          market: "US",
          symbol: "AAPL",
          instrumentId: "US.AAPL",
        },
      ],
      depth: [
        {
          market: "US",
          symbol: "AAPL",
          instrumentId: "US.AAPL",
          num: 1,
        },
        {
          market: "US",
          symbol: "AAPL",
          instrumentId: "US.AAPL",
          num: 50,
        },
      ],
      consoleRefresh: true,
    });

    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    const socket = MockWebSocket.instances[0];
    expect(socket?.sentMessages).toHaveLength(1);

    hub.setConsoleRefreshEnabled(refreshOwner, true);
    hub.setSecurityDetailsTarget(securityOwner, {
      market: "US",
      symbol: "AAPL",
      instrumentId: "US.AAPL",
    });
    expect(socket?.sentMessages).toHaveLength(1);

    hub.setDepthTarget(depthOwnerB, null);
    hub.setActiveInstrument(activeOwnerB, null);
    hub.setSecurityDetailsTarget(securityOwner, null);

    expect(socket?.sentMessages).toHaveLength(3);
    expect(JSON.parse(socket?.sentMessages.at(-1) ?? "{}")).toEqual({
      type: "subscribe",
      subscriptions: {
        providerBrokerId: "alpha",
        activeInstruments: ["US.AAPL"],
        securityDetails: [],
        depth: [
          {
            market: "US",
            symbol: "AAPL",
            instrumentId: "US.AAPL",
            num: 50,
          },
        ],
        consoleRefresh: true,
      },
    });
  });

  it("filters late market events from the previous provider", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const hub = getSharedLiveSocketHub();
    const listener = vi.fn();
    hub.addEventListener(listener);
    hub.setProviderBrokerId(hub.createOwnerId("provider"), "alpha");
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    const heartbeat = (providerBrokerId: string) =>
      createLiveEnvelope(
        {
          type: "heartbeat",
          at: "2026-07-18T00:00:00Z",
          providerBrokerId,
          transport: { mode: "snapshot-poll-fallback" },
        },
        {
          source: "system",
          entityId: "live-websocket",
        },
      );
    MockWebSocket.instances[0]?.emitMessage(heartbeat("futu"));
    expect(hub.lastHeartbeatEvent.value).toBeNull();
    MockWebSocket.instances[0]?.emitMessage(heartbeat("alpha"));
    expect(hub.lastHeartbeatEvent.value?.providerBrokerId).toBe("alpha");
    listener.mockClear();

    const event = (brokerId: string) =>
      createLiveEnvelope(
        {
          type: "market.depth",
          at: "2026-07-18T00:00:00Z",
          brokerId,
          request: {
            market: "US",
            symbol: "AAPL",
            instrumentId: "US.AAPL",
            num: 10,
          },
          depth: { bids: [], asks: [] },
          meta: {
            instrumentId: "US.AAPL",
            source: brokerId,
            resolvedAt: "2026-07-18T00:00:00Z",
            fromCache: false,
          },
        },
        {
          source: "market-data",
          entityId: "US.AAPL|10",
        },
      );

    MockWebSocket.instances[0]?.emitMessage(event("futu"));
    expect(listener).not.toHaveBeenCalled();
    MockWebSocket.instances[0]?.emitMessage(event("alpha"));
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("waits for connection immediately, after async connects, and on timeouts", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();
    const delayedConnection = hub.waitForConnection(500);

    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    await expect(delayedConnection).resolves.toBe(true);
    await expect(hub.waitForConnection(500)).resolves.toBe(true);

    resetSharedLiveSocketHubForTests();
    const idleHub = getSharedLiveSocketHub();
    vi.useFakeTimers();

    const timeout = idleHub.waitForConnection(50);
    await vi.advanceTimersByTimeAsync(50);

    await expect(timeout).resolves.toBe(false);
  });

  it("keeps an active connection when reconnect is requested repeatedly", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();
    const socket = hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");

    expect(hub.reconnect()).toBe(socket);
    expect(MockWebSocket.instances).toHaveLength(1);

    await Promise.resolve();

    expect(hub.reconnect()).toBe(socket);
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]?.closed).toBe(false);
  });

  it("does not let a replaced socket schedule another reconnect", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    hub.connect("ws://127.0.0.1:3001/api/v1/ws/live");

    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[0]?.closed).toBe(true);

    await vi.advanceTimersByTimeAsync(5_000);

    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1]?.closed).toBe(false);
  });

  it("reconnects after close events and clears scheduled reconnects on disconnect", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    MockWebSocket.instances[0]?.close();
    expect(hub.connectionState.value).toBe("disconnected");

    await vi.advanceTimersByTimeAsync(499);
    expect(MockWebSocket.instances).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(1);
    await Promise.resolve();
    expect(MockWebSocket.instances).toHaveLength(2);

    MockWebSocket.instances[1]?.close();
    hub.disconnect();
    await vi.advanceTimersByTimeAsync(5_000);

    expect(MockWebSocket.instances).toHaveLength(2);
    expect(hub.connectionState.value).toBe("disconnected");
  });

  it("isolates stale socket failures, orders subscription targets, and reconnects a closed live channel", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const hub = getSharedLiveSocketHub();
    const firstSocket = hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    (firstSocket as unknown as {
      dispatchEvent: (type: string, event: MessageEvent<string>) => void;
    }).dispatchEvent("message", { data: "{not valid json" } as MessageEvent<string>);
    expect(hub.connectionState.value).toBe("error");

    const currentSocket = hub.connect("ws://127.0.0.1:3001/api/v1/ws/live");
    (firstSocket as unknown as MockWebSocket).emitError();
    await Promise.resolve();
    expect(hub.connectionState.value).toBe("connected");

    hub.setSecurityDetailsTarget(hub.createOwnerId("detail"), {
      market: "US",
      symbol: "MSFT",
      instrumentId: "US.MSFT",
    });
    hub.setSecurityDetailsTarget(hub.createOwnerId("detail"), {
      market: "HK",
      symbol: "00700",
      instrumentId: "HK.00700",
    });
    hub.setDepthTarget(hub.createOwnerId("depth"), {
      market: "US",
      symbol: "MSFT",
      instrumentId: "US.MSFT",
      num: 5,
    });
    hub.setDepthTarget(hub.createOwnerId("depth"), {
      market: "HK",
      symbol: "00700",
      instrumentId: "HK.00700",
      num: 5,
    });
    expect(hub.snapshotSubscriptions()).toMatchObject({
      securityDetails: [
        { instrumentId: "HK.00700" },
        { instrumentId: "US.MSFT" },
      ],
      depth: [
        { instrumentId: "HK.00700" },
        { instrumentId: "US.MSFT" },
      ],
    });

    (currentSocket as unknown as MockWebSocket).close();
    const reconnected = hub.reconnect();
    expect(reconnected).not.toBeNull();
    expect(MockWebSocket.instances).toHaveLength(3);
  });
});
