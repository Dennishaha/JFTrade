// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { useLiveSocket } from "../src/composables/useLiveSocket";
import { MockWebSocket } from "./helpers";

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("useLiveSocket", () => {
  it("reconnects after an unexpected websocket close", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);

    const live = useLiveSocket();
    live.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(live.connectionState.value).toBe("connected");

    MockWebSocket.instances[0]?.close();
    expect(live.connectionState.value).toBe("disconnected");

    await vi.advanceTimersByTimeAsync(500);
    await Promise.resolve();

    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1]?.url).toBe(
      "ws://127.0.0.1:3000/api/v1/ws/live",
    );

    live.disconnect();
    MockWebSocket.instances[1]?.close();
    await vi.advanceTimersByTimeAsync(5000);

    expect(MockWebSocket.instances).toHaveLength(2);
  });
});
