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
vi.mock("@/composables/workspace/useConsoleData", () => ({
  useConsoleData: () => stores.consoleData,
}));
vi.mock("@/composables/workspace/useWorkspaceLayout", () => ({
  useWorkspaceTradingPrefs: () => stores.workspace,
}));
vi.mock("@/composables/market-data/sharedLiveSocket", () => ({
  getSharedLiveSocketHub: () => stores.liveHub,
}));
import LightweightChart from "../../../src/components/workspace/LightweightChart.vue";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "@/composables/trading/brokerProviderSelection";

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
    currentMarketSecurityDetails: ref(null),
    marketDataQueryMarket: ref("US"),
    marketDataQuerySymbol: ref("AAPL"),
    marketDataQueryPeriod: ref("1m"),
    marketDataQueryLimit: ref(200),
    marketDataQueryError: ref(""),
    isLoadingOlderMarketData: ref(false),
    hasMoreMarketDataHistory: ref(true),
    marketDataNextBefore: ref("2026-07-03T12:00:00.000Z"),
    marketDataOlderError: ref(""),
    marketInstrumentSearchOptions: ref([
      { instrumentId: "US.AAPL", name: "Apple Inc." },
    ]),
    isLoadingMarketDataQuery: ref(false),
    loadMarketDataQuery: vi.fn().mockResolvedValue(undefined),
    selectMarketDataInstrument: vi.fn(),
    selectWorkspaceInstrument: vi.fn(),
    acquireMarketDataSubscription: vi.fn().mockResolvedValue(true),
    createStableWebConsumerId: vi.fn((scope: string) => `${scope}:1`),
    heartbeatMarketDataConsumer: vi.fn().mockResolvedValue(undefined),
    releaseMarketDataSubscription: vi.fn().mockResolvedValue(undefined),
    activeMarketDataInstrumentId: ref("US.AAPL"),
    isMarketDataStale: vi.fn(() => false),
    isLiveStreamConnected: ref(true),
  };
}

function createWorkspaceState() {
  const prefs = ref<{
    market: string;
    symbol: string;
    period: string;
    chartType?: "standard" | "heikinashi";
  }>({
    market: "us",
    symbol: "aapl",
    period: "1m",
    chartType: "standard",
  });
  return {
    prefs,
    update: vi.fn((patch: Partial<typeof prefs.value>) => {
      prefs.value = { ...prefs.value, ...patch };
    }),
  };
}

function mountChart(props: Record<string, unknown> = {}) {
  const providerSelection = useBrokerProviderSelection();
  if (providerSelection.brokerDescriptors.value.length === 0) {
    const providerId = providerSelection.selectedBrokerId.value || "test";
    providerSelection.brokerDescriptors.value = [
      {
        id: providerId,
        displayName: "Test Provider",
        capabilities: [
          {
            market: "US",
            supportsQuote: true,
            supportsTrade: true,
            features: [
              {
                id: "market.candles",
                state: "available",
                supportedPeriods: [
                  "1m",
                  "3m",
                  "5m",
                  "10m",
                  "15m",
                  "30m",
                  "1h",
                  "1d",
                  "1w",
                  "1mo",
                ],
                supportedSessions: [
                  { id: "regular", supportedPeriods: ["1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"] },
                  { id: "extended", supportedPeriods: ["1m", "5m", "15m", "30m", "1h"] },
                  { id: "overnight", supportedPeriods: ["1m", "5m", "15m", "30m", "1h"] },
                ],
              },
              { id: "market.ticks", state: "available" },
            ],
          },
          {
            market: "SH",
            supportsQuote: true,
            supportsTrade: true,
            features: [
              {
                id: "market.candles",
                state: "available",
                supportedPeriods: ["1m", "5m", "1d"],
                supportedSessions: [{ id: "regular", supportedPeriods: ["1m", "5m", "1d"] }],
              },
              { id: "market.ticks", state: "available" },
            ],
          },
        ],
      },
    ];
  }
  const wrapper = mount(LightweightChart, {
    attachTo: document.body,
    props,
    global: {
      stubs: {
        KlineChart: {
          props: ["candles", "chartType", "minHeight", "indicators"],
          emits: ["load-more"],
          template:
            "<button class='kline-chart-stub' :data-chart-type='chartType' :data-indicators='indicators.join(`,`)' @click=\"$emit('load-more')\">{{ candles.length }} candles / {{ minHeight }}</button>",
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
  vi.unstubAllGlobals();
  vi.useRealTimers();
  vi.restoreAllMocks();
  window.localStorage.clear();
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
    const primaryControls = header.get(
      ".lightweight-chart-head__primary-controls",
    );
    expect(
      Array.from(primaryControls.element.children).map(
        (element) => element.className,
      ),
    ).toEqual([
      "tv-seg lightweight-chart-head__periods",
      "lightweight-chart-head__period-select",
      "kline-chart-type-selector",
      "kline-indicator-selector",
    ]);
    const compactPeriod = header.get<HTMLSelectElement>(
      ".lightweight-chart-head__period-select select",
    );
    expect(compactPeriod.element.value).toBe("1m");
    expect(compactPeriod.findAll("option")).toHaveLength(
      header.findAll(".lightweight-chart-head__periods button").length,
    );
    expect(
      header
        .get(".lightweight-chart-head__period-chevron")
        .classes(),
    ).toContain("fa-chevron-down");
    expect(header.text()).not.toContain("⌄");
    expect(header.text()).not.toContain(" V");
    expect(header.get('button[title="刷新"]').exists()).toBe(true);
    expect(header.find(".instrument-identity").exists()).toBe(false);
    expect(wrapper.get(".kline-chart-stub").attributes("data-indicators")).toBe(
      "volume",
    );
    expect(wrapper.get(".kline-chart-stub").attributes("data-chart-type")).toBe(
      "standard",
    );
    wrapper.unmount();
  });
  it("uses the compact period selector without changing period semantics", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart();
    await flushUi();
    await wrapper
      .get(".lightweight-chart-head__period-select select")
      .setValue("1d");
    await flushUi();

    expect(stores.workspace.update).toHaveBeenCalledWith({ period: "1d" });
    wrapper.unmount();
  });
  it("persists the selected chart type and closes its topbar menu via Escape or outside click", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart();
    await flushUi();

    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 320,
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 240,
    });
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function () {
        if (this.classList.contains("kline-chart-type-selector__panel")) {
          return {
            x: 0,
            y: 0,
            width: 304,
            height: 100,
            top: 0,
            right: 304,
            bottom: 100,
            left: 0,
            toJSON: () => ({}),
          };
        }
        return {
          x: 280,
          y: 205,
          width: 40,
          height: 26,
          top: 205,
          right: 320,
          bottom: 231,
          left: 280,
          toJSON: () => ({}),
        };
      },
    );
    const trigger = wrapper.get(".kline-chart-type-selector__trigger");
    await trigger.trigger("click");
    const panelSelector = ".kline-chart-type-selector__panel";
    const panel = document.body.querySelector(panelSelector) as HTMLElement;
    expect(panel).not.toBeNull();
    expect(panel.style.left).toBe("8px");
    expect(panel.style.top).toBe("101px");
    const standardOption = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>(
        ".kline-chart-type-selector__option",
      ),
    ).find((option) => option.textContent?.includes("标准 K 线"));
    const firstHeikinAshiOption = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>(
        ".kline-chart-type-selector__option",
      ),
    ).find((option) => option.textContent?.includes("平均K线图"));
    expect(standardOption).toBeDefined();
    expect(firstHeikinAshiOption).toBeDefined();
    expect(document.activeElement).toBe(standardOption);

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown" }));
    expect(document.activeElement).toBe(firstHeikinAshiOption);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Home" }));
    expect(document.activeElement).toBe(standardOption);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "End" }));
    expect(document.activeElement).toBe(firstHeikinAshiOption);

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await nextTick();
    expect(document.body.querySelector(panelSelector)).toBeNull();
    expect(document.activeElement).toBe(trigger.element);

    await trigger.trigger("click");
    document.body.dispatchEvent(
      new PointerEvent("pointerdown", { bubbles: true }),
    );
    await nextTick();
    expect(document.body.querySelector(panelSelector)).toBeNull();

    await trigger.trigger("click");
    const heikinAshiOption = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>(
        ".kline-chart-type-selector__option",
      ),
    ).find((option) => option.textContent?.includes("平均K线图"));
    expect(heikinAshiOption).toBeDefined();
    heikinAshiOption?.click();
    await flushUi();

    expect(stores.workspace.update).toHaveBeenCalledWith({
      chartType: "heikinashi",
    });
    expect(wrapper.get(".kline-chart-stub").attributes("data-chart-type")).toBe(
      "heikinashi",
    );
    wrapper.unmount();
  });
  it("falls back to standard candles and disables Heikin Ashi for Tick charts", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.workspace.prefs.value = {
      market: "US",
      symbol: "AAPL",
      period: "tick",
      chartType: "heikinashi",
    };
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart();
    await flushUi();

    expect(stores.workspace.update).toHaveBeenCalledWith({
      chartType: "standard",
    });
    expect(wrapper.get(".kline-chart-stub").attributes("data-chart-type")).toBe(
      "standard",
    );

    await wrapper.get(".kline-chart-type-selector__trigger").trigger("click");
    const heikinAshiOption = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>(
        ".kline-chart-type-selector__option",
      ),
    ).find((option) => option.textContent?.includes("平均K线图"));
    expect(heikinAshiOption?.disabled).toBe(true);
    wrapper.unmount();
  });
  it("keeps controlled Tick charts local to the embedded view", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.workspace.prefs.value = {
      market: "US",
      symbol: "AAPL",
      period: "1m",
      chartType: "heikinashi",
    };
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart({
      target: { market: "US", symbol: "AAPL" },
      period: "tick",
      variant: "embedded",
    });
    await flushUi();

    expect(wrapper.get(".kline-chart-stub").attributes("data-chart-type")).toBe(
      "standard",
    );
    expect(stores.workspace.prefs.value.chartType).toBe("heikinashi");
    expect(stores.workspace.update).not.toHaveBeenCalledWith({
      chartType: "standard",
    });
    wrapper.unmount();
  });

  it("keeps a controlled chart type selection local to the embedded chart", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart({
      target: { market: "US", symbol: "AAPL" },
      period: "5m",
      variant: "embedded",
    });
    await flushUi();
    stores.workspace.update.mockClear();

    await wrapper.get(".kline-chart-type-selector__trigger").trigger("click");
    const heikinAshiOption = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>(
        ".kline-chart-type-selector__option",
      ),
    ).find((option) => option.dataset.chartType === "heikinashi");
    expect(heikinAshiOption).toBeDefined();
    heikinAshiOption?.click();
    await flushUi();

    expect(wrapper.get(".kline-chart-stub").attributes("data-chart-type")).toBe(
      "heikinashi",
    );
    expect(stores.workspace.update).not.toHaveBeenCalledWith({
      chartType: "heikinashi",
    });
    wrapper.unmount();
  });

  it("keeps the chart type menu operable through viewport collisions and keyboard navigation", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 320,
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 240,
    });
    let panelHeight = 100;
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function () {
        if (this.classList.contains("kline-chart-type-selector__panel")) {
          return {
            x: 0,
            y: 0,
            width: 304,
            height: panelHeight,
            top: 0,
            right: 304,
            bottom: panelHeight,
            left: 0,
            toJSON: () => ({}),
          };
        }
        return {
          x: 280,
          y: 10,
          width: 40,
          height: 26,
          top: 10,
          right: 320,
          bottom: 36,
          left: 280,
          toJSON: () => ({}),
        };
      },
    );

    const wrapper = mountChart();
    await flushUi();
    window.dispatchEvent(new Event("resize"));
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown" }));

    const trigger = wrapper.get(".kline-chart-type-selector__trigger");
    await trigger.trigger("click");
    let panel = document.body.querySelector(
      ".kline-chart-type-selector__panel",
    ) as HTMLElement;
    expect(panel).not.toBeNull();
    panel.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    await nextTick();
    expect(document.body.querySelector(".kline-chart-type-selector__panel")).not.toBeNull();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab" }));
    await nextTick();
    expect(document.body.querySelector(".kline-chart-type-selector__panel")).not.toBeNull();

    await trigger.trigger("click");
    expect(document.body.querySelector(".kline-chart-type-selector__panel")).toBeNull();

    panelHeight = 300;
    await trigger.trigger("click");
    panel = document.body.querySelector(
      ".kline-chart-type-selector__panel",
    ) as HTMLElement;
    expect(panel.style.top).toBe("8px");

    const standardOption = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>(
        ".kline-chart-type-selector__option",
      ),
    ).find((option) => option.dataset.chartType === "standard");
    const heikinAshiOption = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>(
        ".kline-chart-type-selector__option",
      ),
    ).find((option) => option.dataset.chartType === "heikinashi");
    expect(standardOption).toBeDefined();
    expect(heikinAshiOption).toBeDefined();

    trigger.element.focus();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp" }));
    expect(document.activeElement).toBe(heikinAshiOption);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp" }));
    expect(document.activeElement).toBe(standardOption);

    trigger.element.focus();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown" }));
    expect(document.activeElement).toBe(standardOption);
    wrapper.unmount();
  });

  it("restores shared indicator preferences into the topbar control and chart", async () => {
    window.localStorage.setItem(
      "jftrade.workspace-chart.indicators",
      JSON.stringify(["ma5", "macd"]),
    );
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };

    const wrapper = mountChart();
    await flushUi();

    expect(
      wrapper.get(".kline-indicator-selector__trigger").text(),
    ).toContain("2");
    expect(wrapper.get(".kline-chart-stub").attributes("data-indicators")).toBe(
      "macd,ma5",
    );
    wrapper.unmount();
  });
});
