// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

import { createConsoleDataConsoleStreamController } from "@/composables/market-data/consoleDataConsoleStream";
import {
  getSharedLiveSocketHub,
  resetSharedLiveSocketHubForTests,
} from "@/composables/market-data/sharedLiveSocket";
import { MockWebSocket } from "../helpers";

afterEach(() => {
  resetSharedLiveSocketHubForTests();
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("createConsoleDataConsoleStreamController", () => {
  it("initializes the console-refresh lease only once for repeated shell startup", async () => {
    const reloadSystemState = vi.fn(async () => undefined);
    const controller = createConsoleDataConsoleStreamController({
      liveStreamStatus: ref<"disconnected" | "connected" | "degraded">("disconnected"),
      liveStreamCheckedAt: ref(""),
      reloadSystemState,
    });

    await controller.initialize();
    await controller.initialize();

    expect(reloadSystemState).toHaveBeenCalledTimes(1);
    expect(reloadSystemState).toHaveBeenCalledWith();
    controller.dispose();
  });

  it("reconciles server state after a disconnected live channel recovers", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const hub = getSharedLiveSocketHub();
    const reloadSystemState = vi.fn(async () => undefined);
    const controller = createConsoleDataConsoleStreamController({
      liveStreamStatus: ref<"disconnected" | "connected" | "degraded">(
        "disconnected",
      ),
      liveStreamCheckedAt: ref(""),
      reloadSystemState,
    });

    await controller.initialize();
    expect(reloadSystemState).toHaveBeenCalledTimes(1);

    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();
    await nextTick();

    expect(reloadSystemState).toHaveBeenCalledTimes(2);
    expect(reloadSystemState).toHaveBeenLastCalledWith({
      background: true,
      bypassCooldown: true,
    });

    const firstSocket = MockWebSocket.instances[0];
    firstSocket?.close();
    await nextTick();
    hub.reconnect();
    await Promise.resolve();
    await nextTick();

    expect(reloadSystemState).toHaveBeenCalledTimes(3);
    expect(reloadSystemState).toHaveBeenLastCalledWith({
      background: true,
      bypassCooldown: true,
    });

    controller.dispose();
  });

  it("reconciles server state after a hub backpressure resync event", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const hub = getSharedLiveSocketHub();
    const reloadSystemState = vi.fn(async () => undefined);
    const controller = createConsoleDataConsoleStreamController({
      liveStreamStatus: ref<"disconnected" | "connected" | "degraded">(
        "disconnected",
      ),
      liveStreamCheckedAt: ref(""),
      reloadSystemState,
    });

    await controller.initialize();
    hub.setProviderBrokerId(hub.createOwnerId("provider"), "alpha");
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();
    await nextTick();
    expect(reloadSystemState).toHaveBeenCalledTimes(2);

    MockWebSocket.instances[0]?.emitMessage({
      eventId: "live-resync-console",
      type: "live.resync",
      source: "system",
      entityId: "live-websocket",
      serverTime: "2026-07-18T00:00:01Z",
      payload: {
        type: "live.resync",
        at: "2026-07-18T00:00:01Z",
        reason: "broadcast_lagged",
        droppedEvents: 4,
        action: "resubscribe",
      },
    });
    await nextTick();

    expect(reloadSystemState).toHaveBeenCalledTimes(3);
    expect(reloadSystemState).toHaveBeenLastCalledWith({
      background: true,
      bypassCooldown: true,
    });
    controller.dispose();
  });
});
