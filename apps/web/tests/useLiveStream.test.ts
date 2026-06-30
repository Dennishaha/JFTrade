// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { useLiveStream } from "../src/composables/useLiveStream";
import { getLiveEventBus } from "../src/composables/liveEventBus";
import { resetSharedLiveSocketHubForTests } from "../src/composables/sharedLiveSocket";
import { createLiveEnvelope, MockWebSocket } from "./helpers";

afterEach(() => {
  resetSharedLiveSocketHubForTests();
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("useLiveStream", () => {
  it("connects to the live websocket endpoint and records events", async () => {
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const live = useLiveStream();
    live.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(live.connectionState.value).toBe("connected");
    expect(MockWebSocket.instances[0]?.url).toBe(
      "ws://127.0.0.1:3000/api/v1/ws/live",
    );

    const heartbeat = {
      type: "heartbeat",
      at: "2026-05-17T00:00:00.000Z",
    };
    MockWebSocket.instances[0]?.emitMessage(createLiveEnvelope(heartbeat, {
      source: "system",
      entityId: "live-websocket",
      eventId: "heartbeat|2026-05-17T00:00:00.000Z",
    }));

    expect(live.lastHeartbeat.value).toBe("2026-05-17T00:00:00.000Z");
    expect(live.events.value.at(-1)).toMatchObject({
      type: "heartbeat",
      at: "2026-05-17T00:00:00.000Z",
    });
  });

  it("treats initial websocket failures as disconnected and post-connect failures as error", async () => {
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const live = useLiveStream();
    live.connect("ws://127.0.0.1:3000/api/v1/ws/live");

    const initialStream = MockWebSocket.instances[0];
    initialStream?.emitError();
    expect(live.connectionState.value).toBe("disconnected");

    live.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();
    const stream = MockWebSocket.instances.at(-1);
    expect(live.connectionState.value).toBe("connected");

    stream?.emitError();
    expect(live.connectionState.value).toBe("error");

    live.disconnect();
    expect(stream?.closed).toBe(true);
    expect(live.connectionState.value).toBe("disconnected");
  });

  it("does not dispatch duplicate reducer events after reconnect replay", async () => {
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const received: string[] = [];
    getLiveEventBus().subscribe((event) => received.push(event.eventId));

    const live = useLiveStream();
    live.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    const payload = {
      type: "system.notification",
      id: "system-notification-1",
      at: "2026-06-30T00:00:00.000Z",
      level: "info",
      title: "Ready",
    };
    const envelope = createLiveEnvelope(payload, {
      source: "notification",
      entityId: "system-notification-1",
      eventId: "system-notification-1",
    });
    MockWebSocket.instances[0]?.emitMessage(envelope);
    MockWebSocket.instances[0]?.close();
    live.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();
    MockWebSocket.instances.at(-1)?.emitMessage(envelope);

    expect(received).toEqual(["system-notification-1"]);
  });

  it("rejects legacy websocket payloads without an envelope", async () => {
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const received: string[] = [];
    getLiveEventBus().subscribe((event) => received.push(event.eventId));

    const live = useLiveStream();
    live.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    MockWebSocket.instances[0]?.emitMessage({
      type: "system.notification",
      id: "system-notification-legacy",
      at: "2026-06-30T00:00:00.000Z",
      level: "info",
      title: "Legacy",
    });

    expect(live.connectionState.value).toBe("error");
    expect(live.events.value).toEqual([]);
    expect(received).toEqual([]);
  });
});
