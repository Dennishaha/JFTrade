// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const stores = vi.hoisted(() => ({
  consoleData: null as ReturnType<typeof createConsoleDataState> | null,
  workspace: null as ReturnType<typeof createWorkspaceState> | null,
  liveHub: null as {
    waitForConnection: ReturnType<typeof vi.fn>;
    connectionState?: ReturnType<typeof ref>;
    lastHeartbeatEvent?: ReturnType<typeof ref>;
  } | null,
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
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "../src/composables/brokerProviderSelection";

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
    acquireMarketDataSubscription: vi.fn().mockResolvedValue(true),
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

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

afterEach(() => {
  resetBrokerProviderSelectionForTests();
  vi.useRealTimers();
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

describe("LightweightChart", () => {
  it("keeps only chart controls and issues in the internal header", async () => {
    stores.consoleData = createConsoleDataState();
    stores.consoleData.marketInstrumentSearchOptions.value = [
      { instrumentId: "SH.600519", name: "贵州茅台" },
    ];
    stores.workspace = createWorkspaceState();
    stores.workspace.prefs.value = {
      market: "SH",
      symbol: "600519",
      period: "1m",
    };
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart();
    await flushUi();

    const header = wrapper.get(".lightweight-chart-head");
    expect(header.text()).not.toContain("图表");
    expect(header.text()).not.toContain("600519");
    expect(header.text()).not.toContain("贵州茅台");
    expect(header.text()).not.toContain("根");
    expect(header.text()).not.toContain("上限");
    expect(header.findAll(".lightweight-chart-head__periods button").length).toBeGreaterThan(0);
    expect(header.get('button[title="刷新"]').exists()).toBe(true);
    expect(header.find(".instrument-identity").exists()).toBe(false);
    wrapper.unmount();
  });

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
    expect(wrapper.get(".lightweight-chart-head").text()).not.toContain("US.AAPL");
    expect(wrapper.get(".lightweight-chart-head").text()).not.toContain("Apple Inc.");
    expect(wrapper.text()).not.toContain("盘前/盘后K线");
    expect(wrapper.html()).not.toContain("扩展时段");

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

  it("reconfirms the lease when live data is fresh and preserves existing data when it is only slightly stale", async () => {
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
		expect(stores.consoleData.acquireMarketDataSubscription).toHaveBeenCalled();
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

  it("reconfirms a loaded target after a disconnected visibility recovery and reloads on online and refresh events", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = {
      waitForConnection: vi.fn().mockResolvedValue(false),
      connectionState: ref("reconnecting"),
      lastHeartbeatEvent: ref({ transport: { mode: "poll" } }),
    };
    stores.consoleData.isLiveStreamConnected.value = false;

    const wrapper = mountChart();
    await flushUi();
    stores.consoleData.loadMarketDataQuery.mockClear();

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    await flushUi();
    expect(stores.consoleData.loadMarketDataQuery).toHaveBeenCalledWith({
      preserveExisting: true,
    });

    window.dispatchEvent(new Event("online"));
    await flushUi();
    await wrapper.get("button[title='刷新']").trigger("click");
    await flushUi();
    expect(stores.consoleData.loadMarketDataQuery.mock.calls.length).toBeGreaterThanOrEqual(2);
    wrapper.unmount();
  });

  it("releases and reacquires the chart lease for the selected provider", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };
    useBrokerProviderSelection().selectBrokerProvider("alpha");

    const wrapper = mountChart();
    await flushUi();

    expect(stores.consoleData.acquireMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      brokerId: "alpha",
      market: "US",
      symbol: "AAPL",
      channel: "KLINE",
      interval: "1m",
    });
    expect(stores.consoleData.heartbeatMarketDataConsumer).toHaveBeenCalledWith(
      "workspace-chart:1",
      "alpha",
    );

    wrapper.unmount();
  });

  it("does not hold or release a failed subscription and handles an empty chart target", async () => {
    stores.consoleData = createConsoleDataState();
    stores.consoleData.acquireMarketDataSubscription.mockResolvedValue(false);
    stores.consoleData.marketInstrumentSearchOptions.value = [];
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const failedWrapper = mountChart();
    await flushUi();
    expect(stores.consoleData.heartbeatMarketDataConsumer).not.toHaveBeenCalled();
    stores.consoleData.releaseMarketDataSubscription.mockClear();
    failedWrapper.unmount();
    expect(stores.consoleData.releaseMarketDataSubscription).not.toHaveBeenCalled();

    stores.consoleData = createConsoleDataState();
    stores.consoleData.marketInstrumentSearchOptions.value = [];
    stores.workspace = createWorkspaceState();
    stores.workspace.prefs.value = { market: "", symbol: "", period: "1m" };
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };
    const emptyWrapper = mountChart();
    await flushUi();
    expect(stores.consoleData.acquireMarketDataSubscription).not.toHaveBeenCalled();
    expect(emptyWrapper.get(".lightweight-chart-head").text()).not.toContain("Apple Inc.");
    emptyWrapper.unmount();
  });

  it("releases a held tick subscription without an interval when switching back to K-line", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.workspace.prefs.value.period = "tick";
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart();
    await flushUi();
    stores.consoleData.releaseMarketDataSubscription.mockClear();
    stores.workspace.update({ period: "1m" });
    await flushUi();

    expect(stores.consoleData.releaseMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "TICK",
    });
    wrapper.unmount();
  });

  it("drops an out-of-order acquire and keeps the newest chart target", async () => {
    const firstAcquire = deferred<boolean>();
    const secondAcquire = deferred<boolean>();
    stores.consoleData = createConsoleDataState();
    stores.consoleData.acquireMarketDataSubscription
      .mockImplementationOnce(() => firstAcquire.promise)
      .mockImplementationOnce(() => secondAcquire.promise);
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };
    useBrokerProviderSelection().selectBrokerProvider("alpha");

    const wrapper = mountChart();
    await nextTick();
    window.dispatchEvent(new Event("online"));
    await nextTick();
    expect(stores.consoleData.acquireMarketDataSubscription).toHaveBeenCalledTimes(1);

    stores.workspace.update({ symbol: "MSFT" });
    await flushUi();
    expect(stores.consoleData.acquireMarketDataSubscription).toHaveBeenCalledTimes(2);
    firstAcquire.resolve(true);
    await flushUi();
    expect(stores.consoleData.releaseMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "KLINE",
      interval: "1m",
      brokerId: "alpha",
    });
    secondAcquire.resolve(true);
    await flushUi();
    wrapper.unmount();
  });

  it("drops an acquire that becomes stale while its heartbeat is pending", async () => {
    const firstHeartbeat = deferred<void>();
    stores.consoleData = createConsoleDataState();
    stores.consoleData.heartbeatMarketDataConsumer
      .mockImplementationOnce(() => firstHeartbeat.promise)
      .mockResolvedValue(undefined);
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };
    useBrokerProviderSelection().selectBrokerProvider("alpha");

    const wrapper = mountChart();
    await flushUi();
    stores.workspace.update({ symbol: "MSFT" });
    await flushUi();
    firstHeartbeat.resolve();
    await flushUi();

    expect(stores.consoleData.releaseMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "KLINE",
      interval: "1m",
      brokerId: "alpha",
    });
    wrapper.unmount();
  });

  it("releases stale Tick acquires and heartbeats without adding a K-line interval", async () => {
    const acquire = deferred<boolean>();
    stores.consoleData = createConsoleDataState();
    stores.consoleData.acquireMarketDataSubscription
      .mockImplementationOnce(() => acquire.promise)
      .mockResolvedValue(true);
    stores.workspace = createWorkspaceState();
    stores.workspace.prefs.value.period = "tick";
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const acquireWrapper = mountChart();
    await nextTick();
    stores.workspace.update({ symbol: "MSFT" });
    await flushUi();
    acquire.resolve(true);
    await flushUi();
    expect(stores.consoleData.releaseMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "TICK",
    });
    acquireWrapper.unmount();

    const heartbeat = deferred<void>();
    stores.consoleData = createConsoleDataState();
    stores.consoleData.heartbeatMarketDataConsumer
      .mockImplementationOnce(() => heartbeat.promise)
      .mockResolvedValue(undefined);
    stores.workspace = createWorkspaceState();
    stores.workspace.prefs.value.period = "tick";
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const heartbeatWrapper = mountChart();
    await flushUi();
    stores.workspace.update({ symbol: "MSFT" });
    await flushUi();
    heartbeat.resolve();
    await flushUi();
    expect(stores.consoleData.releaseMarketDataSubscription).toHaveBeenCalledWith({
      consumerId: "workspace-chart:1",
      market: "US",
      symbol: "AAPL",
      channel: "TICK",
    });
    heartbeatWrapper.unmount();
  });

  it("keeps snapshot and candle fallbacks out of the compact header", async () => {
    stores.consoleData = createConsoleDataState();
    const snapshot = createSnapshotResult(200, "regular");
    delete snapshot.snapshot.observedAt;
    stores.consoleData.currentMarketDataSnapshot.value = snapshot;
    stores.consoleData.currentMarketDataCandles.value = createCandlesResult(
      "regular",
      false,
    );
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart();
    await flushUi();
    expect(wrapper.text()).not.toContain("盘中");

    stores.consoleData.currentMarketDataSnapshot.value = null;
    stores.consoleData.currentMarketDataCandles.value.meta.fromCache = true;
    await nextTick();
    expect(wrapper.get(".kline-chart-stub").text()).toContain("1 candles");

    stores.consoleData.currentMarketDataCandles.value.candles[0] = {
      ...stores.consoleData.currentMarketDataCandles.value.candles[0],
      displayAt: "2026-07-03 20:00:00",
    };
    await nextTick();
    expect(wrapper.get(".kline-chart-stub").text()).toContain("1 candles");

    stores.consoleData.currentMarketDataCandles.value = {
      ...createCandlesResult("", false),
      candles: [],
    };
    await nextTick();
    expect(wrapper.get(".kline-chart-stub").text()).toContain("0 candles");

    stores.consoleData.currentMarketDataCandles.value = null;
    await nextTick();
    expect(wrapper.get(".kline-chart-stub").text()).toContain("0 candles");
    wrapper.unmount();
  });

  it("does not expose regular-session diagnostics in the compact header", async () => {
    stores.consoleData = createConsoleDataState();
    stores.consoleData.currentMarketDataSnapshot.value = createSnapshotResult();
    delete stores.consoleData.currentMarketDataSnapshot.value.snapshot.session;
    stores.consoleData.currentMarketDataCandles.value = createCandlesResult("regular", false);
    stores.consoleData.isLiveStreamConnected.value = false;
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart();
    await flushUi();
    expect(wrapper.text()).not.toContain("盘中");
    expect(wrapper.html()).not.toContain("常规交易时段数据");
    wrapper.unmount();
  });
});
