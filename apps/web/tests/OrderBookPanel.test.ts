// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

import {
  getSharedLiveSocketHub,
  resetSharedLiveSocketHubForTests,
} from "../src/composables/sharedLiveSocket";
import { MockWebSocket } from "./helpers";

const marketDataSnapshot = ref<any>(null);
const marketSecurityDetails = ref<any>(null);
const prefs = ref<{ market?: string; symbol?: string; period?: string }>({});

const fetchEnvelopeMock = vi.fn();
const fetchEnvelopeWithInitMock = vi.fn();

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => fetchEnvelopeMock(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) => fetchEnvelopeWithInitMock(...args),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => ({
    currentMarketDataSnapshot: marketDataSnapshot,
    currentMarketSecurityDetails: marketSecurityDetails,
  }),
}));

vi.mock("../src/composables/useWorkspaceLayout", () => ({
  useWorkspaceLayout: () => ({
    prefs,
  }),
}));

import OrderBookPanel from "../src/components/workspace/OrderBookPanel.vue";

describe("OrderBookPanel", () => {
  beforeEach(() => {
    resetSharedLiveSocketHubForTests();
    MockWebSocket.instances = [];
    fetchEnvelopeMock.mockReset();
    fetchEnvelopeWithInitMock.mockReset();
    fetchEnvelopeMock.mockResolvedValue({
      descriptor: {
        capabilities: [
          {
            readFeatures: {
              orderBook: {
                defaultNum: 10,
                numPresets: [5, 10, 20, 50],
              },
            },
          },
        ],
      },
    });
    fetchEnvelopeWithInitMock.mockResolvedValue({
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 10,
      },
      depth: {
        symbol: "US.TME",
        bids: [{ price: 18.48, volume: 220, orderCount: 3 }],
        asks: [{ price: 18.52, volume: 180, orderCount: 2 }],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:00Z",
        fromCache: false,
      },
    });
    prefs.value = { market: "US", symbol: "TME", period: "1m" };
    marketDataSnapshot.value = {
      snapshot: {
        price: 18.5,
        previousClosePrice: 18,
        bid: 18.48,
        ask: 18.52,
      },
    };
    marketSecurityDetails.value = null;
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
  });

  afterEach(() => {
    resetSharedLiveSocketHubForTests();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("subscribes depth updates over the shared websocket", async () => {
    const hub = getSharedLiveSocketHub();
    const wrapper = mount(OrderBookPanel);

    await Promise.resolve();
    await nextTick();

    expect(fetchEnvelopeMock).toHaveBeenCalledWith("/api/v1/brokers/futu/runtime");
    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/market-data/depth/US/TME?num=10",
      expect.objectContaining({
        signal: expect.any(AbortSignal),
      }),
    );
    expect(hub.snapshotSubscriptions().depth).toEqual([
      {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 10,
      },
    ]);

    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
    await Promise.resolve();
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]?.url).toBe("ws://127.0.0.1:3000/api/v1/ws/live");

    MockWebSocket.instances[0]?.emitMessage({
      type: "market.depth",
      at: "2026-06-02T00:00:00Z",
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 10,
      },
      depth: {
        symbol: "US.TME",
        bids: [{ price: 18.48, volume: 220, orderCount: 3 }],
        asks: [{ price: 18.52, volume: 180, orderCount: 2 }],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:00Z",
        fromCache: false,
      },
    });
    await nextTick();

    expect(wrapper.text()).toContain("18.52");
    expect(wrapper.text()).toContain("220");
    expect(wrapper.get("[data-testid='depth-bid-price-col']").text()).toContain("18.48");
    expect(wrapper.get("[data-testid='depth-bid-size-col']").text()).toContain("220");
    expect(wrapper.get("[data-testid='depth-ask-price-col']").text()).toContain("18.52");
    expect(wrapper.get("[data-testid='depth-ask-size-col']").text()).toContain("180");

    wrapper.unmount();
  });

  it("keeps one websocket connection when the page becomes visible again", async () => {
    const hub = getSharedLiveSocketHub();
    const originalVisibilityState = document.visibilityState;
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });

    const wrapper = mount(OrderBookPanel);
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");

    await Promise.resolve();
    await nextTick();

    const initialStreamCount = MockWebSocket.instances.length;

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    await nextTick();

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    await nextTick();

    expect(MockWebSocket.instances.length).toBe(initialStreamCount);
    expect(hub.snapshotSubscriptions().depth).toContainEqual({
      market: "US",
      symbol: "TME",
      instrumentId: "US.TME",
      num: 10,
    });

    wrapper.unmount();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: originalVisibilityState,
    });
  });

  it("clears depth data on instrument switch and ignores stale stream payloads", async () => {
    const hub = getSharedLiveSocketHub();
    const wrapper = mount(OrderBookPanel);
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");

    await Promise.resolve();
    await nextTick();

    MockWebSocket.instances[0]?.emitMessage({
      type: "market.depth",
      at: "2026-06-02T00:00:00Z",
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 10,
      },
      depth: {
        symbol: "US.TME",
        bids: [{ price: 18.48, volume: 220, orderCount: 3 }],
        asks: [{ price: 18.52, volume: 180, orderCount: 2 }],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:00Z",
        fromCache: false,
      },
    });
    await nextTick();

    expect(wrapper.text()).toContain("18.52");

    const oldStream = MockWebSocket.instances[0];
    prefs.value = { market: "US", symbol: "AAPL", period: "1m" };
    marketDataSnapshot.value = null;
    await nextTick();

    expect(wrapper.text()).not.toContain("18.52");
    expect(hub.snapshotSubscriptions().depth).toContainEqual({
      market: "US",
      symbol: "AAPL",
      instrumentId: "US.AAPL",
      num: 10,
    });

    oldStream?.emitMessage({
      type: "market.depth",
      at: "2026-06-02T00:00:01Z",
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 10,
      },
      depth: {
        symbol: "US.TME",
        bids: [{ price: 18.4, volume: 900, orderCount: 3 }],
        asks: [{ price: 18.6, volume: 800, orderCount: 2 }],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:01Z",
        fromCache: false,
      },
    });
    await nextTick();

    expect(wrapper.text()).not.toContain("18.60");
    expect(wrapper.text()).not.toContain("900");

    wrapper.unmount();
  });

  it("keeps the shared depth subscription when only the chart period changes", async () => {
    const hub = getSharedLiveSocketHub();
    const wrapper = mount(OrderBookPanel);
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");

    await Promise.resolve();
    await nextTick();

    MockWebSocket.instances[0]?.emitMessage({
      type: "market.depth",
      at: "2026-06-02T00:00:00Z",
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 10,
      },
      depth: {
        symbol: "US.TME",
        bids: [{ price: 18.48, volume: 220, orderCount: 3 }],
        asks: [{ price: 18.52, volume: 180, orderCount: 2 }],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:00Z",
        fromCache: false,
      },
    });
    await nextTick();

    expect(wrapper.text()).toContain("18.52");
    expect(MockWebSocket.instances).toHaveLength(1);

    prefs.value = { market: "US", symbol: "TME", period: "5m" };
    await nextTick();

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(hub.snapshotSubscriptions().depth).toContainEqual({
      market: "US",
      symbol: "TME",
      instrumentId: "US.TME",
      num: 10,
    });
    expect(wrapper.text()).toContain("18.52");

    wrapper.unmount();
  });
});
