// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { useLiveStream } from "../src/composables/useLiveStream";
import { resetSharedLiveSocketHubForTests } from "../src/composables/sharedLiveSocket";
import { MockWebSocket } from "./helpers";

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

    MockWebSocket.instances[0]?.emitMessage({
      type: "heartbeat",
      at: "2026-05-17T00:00:00.000Z",
    });

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
});
