// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { useLiveStream } from "../src/composables/useLiveStream";
import { MockEventSource } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
});

describe("useLiveStream", () => {
  it("connects to the live SSE endpoint and records events", async () => {
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const live = useLiveStream();
    live.connect("http://127.0.0.1:3000/api/v1/stream/live");
    await Promise.resolve();

    expect(MockEventSource.instances).toHaveLength(1);
    expect(live.connectionState.value).toBe("connected");
    expect(MockEventSource.instances[0]?.url).toBe(
      "http://127.0.0.1:3000/api/v1/stream/live",
    );

    MockEventSource.instances[0]?.emitMessage({
      type: "heartbeat",
      at: "2026-05-17T00:00:00.000Z",
    });

    expect(live.lastHeartbeat.value).toBe("2026-05-17T00:00:00.000Z");
    expect(live.events.value.at(-1)).toMatchObject({
      type: "heartbeat",
      at: "2026-05-17T00:00:00.000Z",
    });
  });

  it("treats initial SSE failures as disconnected and post-connect failures as error", async () => {
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const live = useLiveStream();
    live.connect("http://127.0.0.1:3000/api/v1/stream/live");

    const stream = MockEventSource.instances[0];
    stream?.emitError();
    expect(live.connectionState.value).toBe("disconnected");

    stream?.onopen?.(new Event("open"));
    expect(live.connectionState.value).toBe("connected");

    stream?.emitError();
    expect(live.connectionState.value).toBe("error");

    live.disconnect();
    expect(stream?.closed).toBe(true);
    expect(live.connectionState.value).toBe("disconnected");
  });
});
