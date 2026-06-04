// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

import { MockEventSource } from "./helpers";

const marketDataSnapshot = ref<any>(null);
const marketSecurityDetails = ref<any>(null);
const prefs = ref<{ market?: string; symbol?: string; period?: string }>({});

const fetchEnvelopeMock = vi.fn();
const fetchEnvelopeWithInitMock = vi.fn();

vi.mock("../src/composables/apiClient", () => ({
  buildApiUrl: (path: string) => path,
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
    MockEventSource.instances = [];
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
    vi.stubGlobal("EventSource", MockEventSource as unknown as typeof EventSource);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("streams depth updates over SSE instead of polling", async () => {
    const wrapper = mount(OrderBookPanel);

    await Promise.resolve();
    await nextTick();

    expect(fetchEnvelopeMock).toHaveBeenCalledWith("/api/v1/brokers/futu/runtime");
    expect(fetchEnvelopeWithInitMock).not.toHaveBeenCalled();
    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0]?.url).toBe("/api/sse/market/depth/US/TME?num=10");

    MockEventSource.instances[0]?.emitMessage({
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

    wrapper.unmount();
  });

  it("reconnects the depth SSE stream when the page becomes visible again", async () => {
    const originalVisibilityState = document.visibilityState;
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });

    const wrapper = mount(OrderBookPanel);

    await Promise.resolve();
    await nextTick();

    const initialStreamCount = MockEventSource.instances.length;

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

    expect(MockEventSource.instances.length).toBeGreaterThan(initialStreamCount);

    wrapper.unmount();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: originalVisibilityState,
    });
  });

  it("clears depth data on instrument switch and ignores stale stream payloads", async () => {
    const wrapper = mount(OrderBookPanel);

    await Promise.resolve();
    await nextTick();

    MockEventSource.instances[0]?.emitMessage({
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

    const oldStream = MockEventSource.instances[0];
    prefs.value = { market: "US", symbol: "AAPL", period: "1m" };
    marketDataSnapshot.value = null;
    await nextTick();

    expect(wrapper.text()).not.toContain("18.52");
    expect(MockEventSource.instances.at(-1)?.url).toBe(
      "/api/sse/market/depth/US/AAPL?num=10",
    );

    oldStream?.emitMessage({
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

  it("keeps the depth SSE stream when only the chart period changes", async () => {
    const wrapper = mount(OrderBookPanel);

    await Promise.resolve();
    await nextTick();

    MockEventSource.instances[0]?.emitMessage({
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
    expect(MockEventSource.instances).toHaveLength(1);

    prefs.value = { market: "US", symbol: "TME", period: "5m" };
    await nextTick();

    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0]?.url).toBe("/api/sse/market/depth/US/TME?num=10");
    expect(wrapper.text()).toContain("18.52");

    wrapper.unmount();
  });
});
