// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref } from "vue";

import {
  getSharedLiveSocketHub,
  resetSharedLiveSocketHubForTests,
} from "../src/composables/sharedLiveSocket";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "../src/composables/brokerProviderSelection";
import { provideWorkspaceTradingPreferencesStore } from "../src/composables/useWorkspaceLayout";
import { createLiveEnvelope, MockWebSocket } from "./helpers";

const marketDataSnapshot = ref<any>(null);
const marketSecurityDetails = ref<any>(null);
let workspacePrefs: ReturnType<
  typeof provideWorkspaceTradingPreferencesStore
>["prefs"] | null = null;

const fetchEnvelopeMock = vi.fn();
const fetchEnvelopeWithInitMock = vi.fn();
const acquireMarketDataSubscriptionMock = vi.fn();
const heartbeatMarketDataConsumerMock = vi.fn();
const releaseMarketDataSubscriptionMock = vi.fn();

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => fetchEnvelopeMock(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) => fetchEnvelopeWithInitMock(...args),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => ({
    currentMarketDataSnapshot: marketDataSnapshot,
    currentMarketSecurityDetails: marketSecurityDetails,
    acquireMarketDataSubscription: (...args: unknown[]) =>
      acquireMarketDataSubscriptionMock(...args),
    createStableWebConsumerId: () => "web:workspace-depth:window:test",
    heartbeatMarketDataConsumer: (...args: unknown[]) =>
      heartbeatMarketDataConsumerMock(...args),
    releaseMarketDataSubscription: (...args: unknown[]) =>
      releaseMarketDataSubscriptionMock(...args),
  }),
}));

import OrderBookPanel from "../src/components/workspace/OrderBookPanel.vue";

type SetupState = Record<string, unknown>;

function mountOrderBookPanel(options: {
  market?: string;
  symbol?: string;
  period?: string;
} = {}) {
  const Host = defineComponent({
    setup() {
      const store = provideWorkspaceTradingPreferencesStore();
      store.update({
        market: options.market ?? "US",
        symbol: options.symbol ?? "TME",
        period: options.period ?? "1m",
      });
      workspacePrefs = store.prefs;
      return () => h(OrderBookPanel);
    },
  });
  return mount(Host);
}

function panelSetup(wrapper: ReturnType<typeof mountOrderBookPanel>): SetupState {
  return wrapper.findComponent(OrderBookPanel).vm.$.setupState as SetupState;
}

function readSetupValue<T>(
  wrapper: ReturnType<typeof mountOrderBookPanel>,
  key: string,
): T {
  const value = panelSetup(wrapper)[key];
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T }).value;
  }
  return value as T;
}

function callSetup<T>(
  wrapper: ReturnType<typeof mountOrderBookPanel>,
  key: string,
  ...args: unknown[]
): T {
  return (panelSetup(wrapper)[key] as (...values: unknown[]) => T)(...args);
}

async function flushOrderBook(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await nextTick();
  await Promise.resolve();
  await nextTick();
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, resolve, reject };
}

function createDepthEnvelope<TPayload extends {
  type: "market.depth";
  at: string;
  request: { instrumentId: string; num: number };
}>(payload: TPayload) {
  return createLiveEnvelope(payload, {
    source: "market-data",
    entityId: `${payload.request.instrumentId}|${payload.request.num}`,
  });
}

describe("OrderBookPanel", () => {
  beforeEach(() => {
    resetBrokerProviderSelectionForTests();
    resetSharedLiveSocketHubForTests();
    MockWebSocket.instances = [];
    fetchEnvelopeMock.mockReset();
    fetchEnvelopeWithInitMock.mockReset();
    acquireMarketDataSubscriptionMock.mockReset();
    heartbeatMarketDataConsumerMock.mockReset();
    releaseMarketDataSubscriptionMock.mockReset();
    acquireMarketDataSubscriptionMock.mockResolvedValue(true);
    heartbeatMarketDataConsumerMock.mockResolvedValue(undefined);
    releaseMarketDataSubscriptionMock.mockResolvedValue(undefined);
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
    resetBrokerProviderSelectionForTests();
    resetSharedLiveSocketHubForTests();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("keeps the order-book header semantic without repeating the instrument", () => {
    const wrapper = mountOrderBookPanel({ market: "SH", symbol: "600519" });

    const header = wrapper.get(".tv-panel-head").text();
    expect(header).toContain("盘口");
    expect(header).not.toContain("600519");
    expect(header).not.toContain("上证");
    expect(header).not.toContain("SH.600519");

    wrapper.unmount();
  });

  it("subscribes depth updates over the shared websocket", async () => {
    const hub = getSharedLiveSocketHub();
    const wrapper = mountOrderBookPanel();

    await flushOrderBook();

    expect(fetchEnvelopeMock).not.toHaveBeenCalled();
    expect(acquireMarketDataSubscriptionMock).toHaveBeenCalledWith({
      consumerId: "web:workspace-depth:window:test",
      market: "US",
      symbol: "TME",
      channel: "ORDER_BOOK",
    });
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

    MockWebSocket.instances[0]?.emitMessage(createDepthEnvelope({
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
    }));
    await nextTick();

    expect(wrapper.text()).toContain("18.52");
    expect(wrapper.text()).toContain("220");
    expect(wrapper.get("[data-testid='depth-bid-price-col']").text()).toContain("18.48");
    expect(wrapper.get("[data-testid='depth-bid-size-col']").text()).toContain("220");
    expect(wrapper.get("[data-testid='depth-ask-price-col']").text()).toContain("18.52");
    expect(wrapper.get("[data-testid='depth-ask-size-col']").text()).toContain("180");

    wrapper.unmount();
    expect(releaseMarketDataSubscriptionMock).toHaveBeenCalledWith({
      consumerId: "web:workspace-depth:window:test",
      market: "US",
      symbol: "TME",
      channel: "ORDER_BOOK",
      keepalive: true,
    });
  });

  it("keeps one websocket connection when the page becomes visible again", async () => {
    const hub = getSharedLiveSocketHub();
    const originalVisibilityState = document.visibilityState;
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });

    const wrapper = mountOrderBookPanel();
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
    const wrapper = mountOrderBookPanel();
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");

    await Promise.resolve();
    await nextTick();

    MockWebSocket.instances[0]?.emitMessage(createDepthEnvelope({
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
    }));
    await nextTick();

    expect(wrapper.text()).toContain("18.52");

    const oldStream = MockWebSocket.instances[0];
    workspacePrefs!.value = {
      ...workspacePrefs!.value,
      market: "US",
      symbol: "AAPL",
      period: "1m",
    };
    marketDataSnapshot.value = null;
    await nextTick();
    await flushOrderBook();

    expect(releaseMarketDataSubscriptionMock).toHaveBeenCalledWith({
      consumerId: "web:workspace-depth:window:test",
      market: "US",
      symbol: "TME",
      channel: "ORDER_BOOK",
      keepalive: false,
    });
    expect(acquireMarketDataSubscriptionMock).toHaveBeenLastCalledWith({
      consumerId: "web:workspace-depth:window:test",
      market: "US",
      symbol: "AAPL",
      channel: "ORDER_BOOK",
    });
    expect(
      releaseMarketDataSubscriptionMock.mock.invocationCallOrder[0]!,
    ).toBeLessThan(acquireMarketDataSubscriptionMock.mock.invocationCallOrder[1]!);

    expect(wrapper.text()).not.toContain("18.52");
    expect(hub.snapshotSubscriptions().depth).toContainEqual({
      market: "US",
      symbol: "AAPL",
      instrumentId: "US.AAPL",
      num: 10,
    });

    oldStream?.emitMessage(createDepthEnvelope({
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
    }));
    await nextTick();

    expect(wrapper.text()).not.toContain("18.60");
    expect(wrapper.text()).not.toContain("900");

    wrapper.unmount();
  });

  it("routes depth reads and leases through the selected provider", async () => {
    useBrokerProviderSelection().selectBrokerProvider("alpha");
    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    expect(acquireMarketDataSubscriptionMock).toHaveBeenCalledWith({
      consumerId: "web:workspace-depth:window:test",
      brokerId: "alpha",
      market: "US",
      symbol: "TME",
      channel: "ORDER_BOOK",
    });
    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/market-data/depth/US/TME?num=10&brokerId=alpha",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );

    wrapper.unmount();
  });

  it("keeps the shared depth subscription when only the chart period changes", async () => {
    const hub = getSharedLiveSocketHub();
    const wrapper = mountOrderBookPanel();
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");

    await Promise.resolve();
    await nextTick();

    MockWebSocket.instances[0]?.emitMessage(createDepthEnvelope({
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
    }));
    await nextTick();

    expect(wrapper.text()).toContain("18.52");
    expect(acquireMarketDataSubscriptionMock).toHaveBeenCalledTimes(1);
    expect(releaseMarketDataSubscriptionMock).not.toHaveBeenCalled();
    expect(MockWebSocket.instances).toHaveLength(1);

    workspacePrefs!.value = {
      ...workspacePrefs!.value,
      period: "5m",
    };
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

  it("uses the stable default preset and keeps ratio helpers aligned with market data", async () => {
    marketSecurityDetails.value = {
      security: {
        instrumentId: "US.TME",
        bidPrice: 18.41,
        askPrice: 18.59,
        bidVolume: 600,
        askVolume: 400,
        currentPrice: 18.55,
        lastClosePrice: 18,
      },
    };

    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/market-data/depth/US/TME?num=10",
      expect.objectContaining({
        signal: expect.any(AbortSignal),
      }),
    );
    expect(wrapper.text()).toContain("Bid 60.00%");
    expect(wrapper.text()).toContain("Ask 40.00%");
    expect(readSetupValue<number | null>(wrapper, "lastPrice")).toBe(18.55);
    expect(readSetupValue<number | null>(wrapper, "changeFromClose")).toBeCloseTo(0.55);
    expect(readSetupValue<number | null>(wrapper, "changePercent")).toBeCloseTo(
      3.055555,
      5,
    );
    expect(readSetupValue<string | null>(wrapper, "bidRatioPercent")).toBe("60.00");
    expect(readSetupValue<string | null>(wrapper, "askRatioPercent")).toBe("40.00");
    expect(callSetup<string>(wrapper, "fmtPrice", null)).toBe("—");
    expect(callSetup<string>(wrapper, "fmtPrice", 0.45678)).toBe("0.46");
    expect(callSetup<string>(wrapper, "fmtPrice", 8.7654)).toBe("8.77");
    expect(callSetup<string>(wrapper, "fmtPrice", 18.54)).toBe("18.54");
    expect(callSetup<string>(wrapper, "fmtSize", null)).toBe("—");
    expect(callSetup<string>(wrapper, "fmtSize", 900)).toBe("900");
    expect(callSetup<string>(wrapper, "fmtSize", 1_500)).toBe("1.5K");
    expect(callSetup<string>(wrapper, "fmtSize", 1_500_000)).toBe("1.50M");
    expect(callSetup<string>(wrapper, "fmtSize", 1_500_000_000)).toBe("1.50B");
    wrapper.unmount();
  });

  it("disables depth when there is no valid instrument and avoids issuing broker depth requests", async () => {
    const hub = getSharedLiveSocketHub();
    const wrapper = mountOrderBookPanel({
      market: "",
      symbol: "",
    });

    await flushOrderBook();

    expect(fetchEnvelopeWithInitMock).not.toHaveBeenCalled();
    expect(acquireMarketDataSubscriptionMock).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("盘口不可用");
    expect(hub.snapshotSubscriptions().depth).toEqual([]);
    expect(callSetup<string | null>(wrapper, "buildDepthUrl")).toBeNull();
    expect(callSetup<boolean>(wrapper, "isDepthDataStale")).toBe(true);

    wrapper.unmount();
  });

  it("uses the default preset without a provider-specific capability request", async () => {
    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/market-data/depth/US/TME?num=10",
      expect.objectContaining({
        signal: expect.any(AbortSignal),
      }),
    );
    expect(
      wrapper
        .findAll("button")
        .find((button) => button.text() === "10")
        ?.classes(),
    ).toContain("is-active");
    expect(fetchEnvelopeMock).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it("shows an empty depth state when the broker responds without ladder levels", async () => {
    fetchEnvelopeWithInitMock.mockResolvedValueOnce({
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 10,
      },
      depth: {
        symbol: "US.TME",
        bids: [],
        asks: [],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:00Z",
        fromCache: false,
      },
    });

    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    expect(wrapper.find("[data-state='empty']").text()).toContain("暂无深度数据");
    expect(readSetupValue<unknown[]>(wrapper, "depthLevels")).toHaveLength(0);

    wrapper.unmount();
  });

  it("ignores non-depth live payloads and payloads for other depth presets", async () => {
    fetchEnvelopeWithInitMock.mockResolvedValueOnce({
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 10,
      },
      depth: {
        symbol: "US.TME",
        bids: [],
        asks: [],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:00Z",
        fromCache: false,
      },
    });

    const hub = getSharedLiveSocketHub();
    const wrapper = mountOrderBookPanel();
    hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");

    await flushOrderBook();

    MockWebSocket.instances[0]?.emitMessage(
      createLiveEnvelope(
        {
          type: "system.notification",
          id: "notice-1",
          at: "2026-06-02T00:01:00Z",
          level: "info",
          title: "ignore",
        },
        {
          source: "system",
          entityId: "notice-1",
        },
      ),
    );
    MockWebSocket.instances[0]?.emitMessage(createDepthEnvelope({
      type: "market.depth",
      at: "2026-06-02T00:01:00Z",
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 5,
      },
      depth: {
        symbol: "US.TME",
        bids: [{ price: 18.4, volume: 900, orderCount: 1 }],
        asks: [{ price: 18.6, volume: 800, orderCount: 1 }],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:01:00Z",
        fromCache: false,
      },
    }));
    await nextTick();

    expect(wrapper.text()).toContain("暂无深度数据");
    expect(wrapper.text()).not.toContain("18.60");
    expect(wrapper.text()).not.toContain("900");

    wrapper.unmount();
  });

  it("surfaces explicit and fallback errors from broker depth requests", async () => {
    fetchEnvelopeWithInitMock.mockRejectedValueOnce(new Error("网络断开"));

    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    expect(wrapper.get(".market-feed-issue-badge").text()).toContain("行情异常");
    expect(wrapper.get(".market-feed-issue-badge").attributes("title")).toContain("网络断开");
    expect(wrapper.get(".tv-ob-depth-placeholder[data-state='error']").text()).toContain("网络断开");
    expect(wrapper.find(".tv-ob-preset-error").exists()).toBe(false);

    fetchEnvelopeWithInitMock.mockRejectedValueOnce({});
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "20")
      ?.trigger("click");
    await flushOrderBook();

    expect(wrapper.get(".tv-ob-depth-placeholder[data-state='error']").text()).toContain(
      "获取盘口深度失败",
    );

    wrapper.unmount();
  });

  it("drops stale request responses when a newer depth refresh is already in flight", async () => {
    const firstRequest = deferred<unknown>();
    const secondRequest = deferred<unknown>();
    fetchEnvelopeWithInitMock.mockImplementationOnce(
      () => firstRequest.promise,
    );
    fetchEnvelopeWithInitMock.mockImplementationOnce(
      () => secondRequest.promise,
    );

    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "20")
      ?.trigger("click");
    await nextTick();

    const firstSignal = fetchEnvelopeWithInitMock.mock.calls[0]?.[1]
      ?.signal as AbortSignal | undefined;
    expect(firstSignal?.aborted).toBe(true);

    firstRequest.resolve({
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
    secondRequest.resolve({
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 20,
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
    await flushOrderBook();

    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledTimes(2);
    expect(acquireMarketDataSubscriptionMock).toHaveBeenCalledTimes(1);
    expect(releaseMarketDataSubscriptionMock).not.toHaveBeenCalled();
    expect(wrapper.get("[data-testid='depth-ask-price-col']").text()).toContain(
      "18.60",
    );
    expect(
      wrapper.get("[data-testid='depth-ask-price-col']").text(),
    ).not.toContain("18.52");
    expect(wrapper.get("[data-testid='depth-bid-size-col']").text()).toContain("900");

    wrapper.unmount();
  });

  it("ignores responses for a different instrument and recovers when the page becomes visible or online again", async () => {
    fetchEnvelopeWithInitMock.mockResolvedValueOnce({
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
        instrumentId: "US.OTHER",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:00Z",
        fromCache: false,
      },
    });

    const hub = getSharedLiveSocketHub();
    const waitForConnectionSpy = vi
      .spyOn(hub, "waitForConnection")
      .mockResolvedValue(false);
    const originalVisibilityState = document.visibilityState;
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });

    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    expect(wrapper.find("[data-state='empty']").text()).toContain("暂无深度数据");

    fetchEnvelopeWithInitMock.mockResolvedValue({
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 20,
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

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "20")
      ?.trigger("click");
    await flushOrderBook();

    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledTimes(2);

    document.dispatchEvent(new Event("visibilitychange"));
    await flushOrderBook();

    expect(waitForConnectionSpy).toHaveBeenCalledWith(3_000);
    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledTimes(3);

    window.dispatchEvent(new Event("online"));
    await flushOrderBook();

    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledTimes(4);
    expect(wrapper.text()).toContain("18.60");

    wrapper.unmount();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: originalVisibilityState,
    });
  });

  it("clears loading state when depth is requested without a valid instrument", async () => {
    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    workspacePrefs!.value = {
      ...workspacePrefs!.value,
      market: "",
      symbol: "",
      period: "1m",
    };
    await nextTick();

    await callSetup<Promise<void>>(wrapper, "fetchDepth");

    expect(readSetupValue(wrapper, "depthData")).toBeNull();
    expect(readSetupValue(wrapper, "depthError")).toBe("");
    expect(readSetupValue(wrapper, "isLoadingDepth")).toBe(false);

    wrapper.unmount();
  });

  it("does not surface errors from depth requests that were already aborted by a newer request", async () => {
    const firstRequest = deferred<unknown>();
    const secondRequest = deferred<unknown>();
    fetchEnvelopeWithInitMock.mockImplementationOnce(
      () => firstRequest.promise,
    );
    fetchEnvelopeWithInitMock.mockImplementationOnce(
      () => secondRequest.promise,
    );

    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "20")
      ?.trigger("click");
    await nextTick();

    firstRequest.reject(new Error("aborted"));
    secondRequest.resolve({
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
        num: 20,
      },
      depth: {
        symbol: "US.TME",
        bids: [{ price: 18.4, volume: 900, orderCount: 3 }],
        asks: [{ price: 18.6, volume: 800, orderCount: 2 }],
      },
      meta: {
        instrumentId: "US.TME",
        source: "bbgo:futu",
        resolvedAt: "2026-06-02T00:00:02Z",
        fromCache: false,
      },
    });
    await flushOrderBook();

    expect(wrapper.find(".tv-ob-depth-placeholder[data-state='error']").exists()).toBe(false);
    expect(wrapper.get("[data-testid='depth-ask-price-col']").text()).toContain(
      "18.60",
    );

    wrapper.unmount();
  });

  it("does not open the depth stream when the order-book lease cannot be acquired", async () => {
    acquireMarketDataSubscriptionMock.mockResolvedValueOnce(false);

    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    expect(fetchEnvelopeWithInitMock).not.toHaveBeenCalled();
    expect(getSharedLiveSocketHub().snapshotSubscriptions().depth).toEqual([]);
    expect(wrapper.get(".tv-ob-depth-placeholder[data-state='error']").text()).toContain(
      "盘口订阅申请失败",
    );

    wrapper.unmount();
    expect(releaseMarketDataSubscriptionMock).not.toHaveBeenCalled();
  });

  it("releases a stale subscription acquired after a rapid instrument switch", async () => {
    const firstAcquire = deferred<boolean>();
    acquireMarketDataSubscriptionMock
      .mockImplementationOnce(() => firstAcquire.promise)
      .mockResolvedValueOnce(true);

    const wrapper = mountOrderBookPanel();
    await Promise.resolve();
    await nextTick();

    workspacePrefs!.value = {
      ...workspacePrefs!.value,
      market: "US",
      symbol: "AAPL",
    };
    await nextTick();

    firstAcquire.resolve(true);
    await flushOrderBook();

    expect(releaseMarketDataSubscriptionMock).toHaveBeenCalledWith({
      consumerId: "web:workspace-depth:window:test",
      market: "US",
      symbol: "TME",
      channel: "ORDER_BOOK",
      keepalive: false,
    });
    expect(acquireMarketDataSubscriptionMock).toHaveBeenLastCalledWith({
      consumerId: "web:workspace-depth:window:test",
      market: "US",
      symbol: "AAPL",
      channel: "ORDER_BOOK",
    });
    expect(getSharedLiveSocketHub().snapshotSubscriptions().depth).toContainEqual({
      market: "US",
      symbol: "AAPL",
      instrumentId: "US.AAPL",
      num: 10,
    });

    wrapper.unmount();
  });

  it("skips visibility recovery when the socket reconnects and depth data is still fresh", async () => {
    const hub = getSharedLiveSocketHub();
    const waitForConnectionSpy = vi
      .spyOn(hub, "waitForConnection")
      .mockResolvedValue(true);
    const originalVisibilityState = document.visibilityState;
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });

    const wrapper = mountOrderBookPanel();
    await flushOrderBook();

    document.dispatchEvent(new Event("visibilitychange"));
    await flushOrderBook();

    expect(waitForConnectionSpy).toHaveBeenCalledWith(3_000);
    expect(fetchEnvelopeWithInitMock).toHaveBeenCalledTimes(1);

    wrapper.unmount();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: originalVisibilityState,
    });
  });
});
