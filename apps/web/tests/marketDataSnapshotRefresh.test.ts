// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import {
  getSharedLiveSocketHub,
  resetSharedLiveSocketHubForTests,
} from "../src/composables/sharedLiveSocket";
import { createMarketDataSnapshotRefresher } from "../src/composables/marketDataSnapshotRefresh";
import { createLiveEnvelope, MockWebSocket } from "./helpers";

function createSecurityDetails(market: string, symbol: string, name: string) {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: {
      market,
      symbol,
      instrumentId,
    },
    security: {
      instrumentId,
      name,
      currentPrice: 321.4,
    },
    meta: {
      instrumentId,
      source: "bbgo:futu",
      resolvedAt: "2026-06-02T00:00:00Z",
      fromCache: false,
    },
  };
}

afterEach(() => {
  resetSharedLiveSocketHubForTests();
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("createMarketDataSnapshotRefresher", () => {
  it("refreshes security details over the shared websocket subscription", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const hub = getSharedLiveSocketHub();
    const marketSecurityDetails = ref<any>(
      createSecurityDetails("HK", "00700", "Initial"),
    );

    const refresher = createMarketDataSnapshotRefresher({
      marketSecurityDetails,
    });

    refresher.scheduleMarketSnapshotBackgroundRefresh();
    expect(hub.snapshotSubscriptions().securityDetails).toEqual([
      {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
    ]);

    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]?.url).toBe(
      "ws://127.0.0.1:3000/api/v1/ws/live",
    );

    const payload = {
      ...createSecurityDetails("HK", "00700", "Tencent Holdings"),
      type: "market.security-details",
      at: "2026-06-02T00:00:00Z",
    };
    MockWebSocket.instances[0]?.emitMessage(createLiveEnvelope(payload, {
      source: "market-data",
      entityId: "HK.00700",
    }));

    expect(marketSecurityDetails.value.security.name).toBe("Tencent Holdings");

    refresher.stopMarketSnapshotBackgroundRefresh();
    expect(hub.snapshotSubscriptions().securityDetails).toEqual([]);
  });

  it("keeps the active detail intact for incomplete targets and other instruments", async () => {
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    const hub = getSharedLiveSocketHub();
    const marketSecurityDetails = ref<any>(
      createSecurityDetails(" ", "00700", "Incomplete request"),
    );
    const refresher = createMarketDataSnapshotRefresher({
      marketSecurityDetails,
    });

    // A partially resolved instrument must not leave a stale websocket
    // subscription alive while the security page is changing targets.
    refresher.scheduleMarketSnapshotBackgroundRefresh();
    expect(hub.snapshotSubscriptions().securityDetails).toEqual([]);

    marketSecurityDetails.value = createSecurityDetails("HK", "00700", "Tencent");
    refresher.scheduleMarketSnapshotBackgroundRefresh();
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();

    const otherInstrument = {
      ...createSecurityDetails("HK", "00941", "China Mobile"),
      type: "market.security-details",
      at: "2026-06-02T00:00:00Z",
    };
    MockWebSocket.instances[0]?.emitMessage(createLiveEnvelope(otherInstrument, {
      source: "market-data",
      entityId: "HK.00941",
    }));

    expect(marketSecurityDetails.value.security.name).toBe("Tencent");
    refresher.stopMarketSnapshotBackgroundRefresh();
  });
});
