// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const stores = vi.hoisted(() => ({
  consoleData: null as ReturnType<typeof createConsoleDataState> | null,
  workspace: null as ReturnType<typeof createWorkspaceState> | null,
  liveHub: null as { waitForConnection: ReturnType<typeof vi.fn> } | null,
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => stores.consoleData,
}));

vi.mock("../src/composables/useWorkspaceLayout", () => ({
  useWorkspaceTradingPrefs: () => stores.workspace,
}));

vi.mock("../src/composables/sharedLiveSocket", () => ({
  getSharedLiveSocketHub: () => stores.liveHub,
}));

import LightweightChart from "../src/components/workspace/LightweightChart.vue";

function createCandlesResult(session = "all", extendedHours = true) {
  return {
    request: {
      instrument: {
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      },
      period: "1m",
      limit: 2,
    },
    candles: [
      {
        period: "1m",
        at: "2026-07-03T12:00:00.000Z",
        open: 200,
        high: 201,
        low: 199.5,
        close: 200.5,
        volume: 1000,
      },
    ],
    totalReturned: 1,
    meta: {
      instrumentId: "US.AAPL",
      source: "test",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
      session,
      extendedHours,
    },
  };
}

function createSnapshotResult(price = 200, session?: string) {
  return {
    request: {
      market: "US",
      symbol: "AAPL",
      instrumentId: "US.AAPL",
    },
    snapshot: {
      price,
      bid: price - 0.1,
      ask: price + 0.1,
      volume: 1000,
      turnover: 200000,
      at: "2026-07-03T12:00:00.000Z",
      observedAt: "2026-07-03T12:00:00.000Z",
      ...(session != null ? { session } : {}),
    },
    meta: {
      instrumentId: "US.AAPL",
      source: "test",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
    },
  };
}

function createConsoleDataState() {
  return {
    currentMarketDataCandles: ref(createCandlesResult()),
    currentMarketDataSnapshot: ref(createSnapshotResult()),
    marketDataQueryMarket: ref("US"),
    marketDataQuerySymbol: ref("AAPL"),
    marketDataQueryPeriod: ref("1m"),
    marketDataQueryLimit: ref(200),
    marketDataQueryError: ref(""),
    marketInstrumentSearchOptions: ref([
      { instrumentId: "US.AAPL", name: "Apple Inc." },
    ]),
    isLoadingMarketDataQuery: ref(false),
    loadMarketDataQuery: vi.fn().mockResolvedValue(undefined),
    selectWorkspaceInstrument: vi.fn(),
    acquireMarketDataSubscription: vi.fn().mockResolvedValue(undefined),
    createStableWebConsumerId: vi.fn(() => "workspace-chart:1"),
    heartbeatMarketDataConsumer: vi.fn().mockResolvedValue(undefined),
    releaseMarketDataSubscription: vi.fn().mockResolvedValue(undefined),
    activeMarketDataInstrumentId: ref("US.AAPL"),
    isMarketDataStale: vi.fn(() => false),
    isLiveStreamConnected: ref(true),
  };
}

function createWorkspaceState() {
  const prefs = ref({
    market: "us",
    symbol: "aapl",
    period: "1m",
  });
  return {
    prefs,
    update: vi.fn((patch: Partial<typeof prefs.value>) => {
      prefs.value = { ...prefs.value, ...patch };
    }),
  };
}

function mountChart() {
  const wrapper = mount(LightweightChart, {
    attachTo: document.body,
    global: {
      stubs: {
        KlineChart: {
          props: ["candles", "minHeight"],
          template:
            "<div class='kline-chart-stub'>{{ candles.length }} candles / {{ minHeight }}</div>",
        },
      },
    },
  });
  return wrapper;
}

async function flushUi(): Promise<void> {
  await Promise.resolve();
  await nextTick();
  await Promise.resolve();
  await nextTick();
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

describe("LightweightChart", () => {
  it("loads the current workspace instrument, manages subscriptions, and releases keepalive subscriptions on unmount", async () => {
    vi.useFakeTimers();
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = {
      waitForConnection: vi.fn().mockResolvedValue(true),
    };

    const wrapper = mountChart();
    await flushUi();

    expect(stores.consoleData.selectWorkspaceInstrument).toHaveBeenCalledWith({
      market: "US",
      symbol: "AAPL",
      period: "1m",
    });
    expect(stores.consoleData.acquireMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "KLINE",
      interval: "1m",
    });
    expect(stores.consoleData.heartbeatMarketDataConsumer).toHaveBeenCalledWith(
      "workspace-chart:1",
    );
    expect(stores.consoleData.loadMarketDataQuery).toHaveBeenCalledWith({});
    expect(wrapper.text()).toContain("US.AAPL · Apple Inc.");
    expect(wrapper.text()).toContain("盘前/盘后K线");
    expect(wrapper.html()).toContain("扩展时段");

    stores.consoleData.heartbeatMarketDataConsumer.mockClear();
    await vi.advanceTimersByTimeAsync(15_000);
    expect(stores.consoleData.heartbeatMarketDataConsumer).toHaveBeenCalledWith(
      "workspace-chart:1",
    );

    stores.consoleData.releaseMarketDataSubscription.mockClear();
    stores.consoleData.acquireMarketDataSubscription.mockClear();
    await wrapper.get(".lightweight-chart-head__periods button").trigger("click");
    await flushUi();

    expect(stores.workspace.update).toHaveBeenCalledWith({ period: "tick" });
    expect(stores.consoleData.releaseMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "KLINE",
      interval: "1m",
    });
    expect(stores.consoleData.acquireMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "TICK",
    });

    stores.consoleData.releaseMarketDataSubscription.mockClear();
    wrapper.unmount();

    expect(stores.consoleData.releaseMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "TICK",
      keepalive: true,
    });
  });

  it("uses heartbeat-only recovery when live data is fresh and preserves existing data when it is only slightly stale", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = {
      waitForConnection: vi.fn().mockResolvedValue(true),
    };

    const wrapper = mountChart();
    await flushUi();

    stores.consoleData.heartbeatMarketDataConsumer.mockClear();
    stores.consoleData.loadMarketDataQuery.mockClear();
    stores.consoleData.selectWorkspaceInstrument.mockClear();
    stores.consoleData.acquireMarketDataSubscription.mockClear();

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });

    stores.consoleData.isMarketDataStale.mockImplementation(() => false);
    document.dispatchEvent(new Event("visibilitychange"));
    await flushUi();

    expect(stores.liveHub.waitForConnection).toHaveBeenCalledWith(3_000);
    expect(stores.consoleData.heartbeatMarketDataConsumer).toHaveBeenCalledTimes(1);
    expect(stores.consoleData.loadMarketDataQuery).not.toHaveBeenCalled();

    stores.consoleData.heartbeatMarketDataConsumer.mockClear();
    stores.consoleData.loadMarketDataQuery.mockClear();
    stores.consoleData.selectWorkspaceInstrument.mockClear();
    stores.consoleData.acquireMarketDataSubscription.mockClear();
    stores.consoleData.isMarketDataStale.mockImplementation(
      (ms: number) => ms === 30_000,
    );

    document.dispatchEvent(new Event("visibilitychange"));
    await flushUi();

    expect(stores.consoleData.selectWorkspaceInstrument).toHaveBeenCalledWith({
      market: "US",
      symbol: "AAPL",
      period: "1m",
    });
    expect(stores.consoleData.acquireMarketDataSubscription).toHaveBeenCalled();
    expect(stores.consoleData.loadMarketDataQuery).toHaveBeenCalledWith({
      preserveExisting: true,
    });
  });

  it("ignores hidden tab events and falls back to full reloads when the chart target is not loaded", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = {
      waitForConnection: vi.fn().mockResolvedValue(false),
    };
    stores.consoleData.currentMarketDataCandles.value = null;
    stores.consoleData.activeMarketDataInstrumentId.value = "";

    const wrapper = mountChart();
    await flushUi();

    stores.consoleData.loadMarketDataQuery.mockClear();
    stores.consoleData.selectWorkspaceInstrument.mockClear();

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    await flushUi();

    expect(stores.consoleData.loadMarketDataQuery).not.toHaveBeenCalled();

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    await flushUi();

    expect(stores.consoleData.selectWorkspaceInstrument).toHaveBeenCalledWith({
      market: "US",
      symbol: "AAPL",
      period: "1m",
    });
    expect(stores.consoleData.loadMarketDataQuery).toHaveBeenCalledWith({});
  });
});
