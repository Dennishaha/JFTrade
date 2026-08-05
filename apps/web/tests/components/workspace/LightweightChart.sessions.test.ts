// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const stores = vi.hoisted(() => ({
  consoleData: null as ReturnType<typeof createConsoleDataState> | null,
  workspace: null as ReturnType<typeof createWorkspaceState> | null,
  liveHub: null as { waitForConnection: ReturnType<typeof vi.fn> } | null,
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

import LightweightChart from "@/components/workspace/LightweightChart.vue";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "@/composables/trading/brokerProviderSelection";

function createCandlesResult() {
  return {
    request: {
      instrument: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
      period: "1m",
      limit: 2,
    },
    candles: [
      {
        period: "1m",
        at: "2026-07-03T12:00:00.000Z",
        open: 200,
        high: 201,
        low: 199,
        close: 200.5,
        volume: 1000,
        session: "regular",
      },
      {
        period: "1m",
        at: "2026-07-03T12:01:00.000Z",
        open: 201,
        high: 202,
        low: 200,
        close: 201.5,
        volume: 1000,
        session: "overnight",
      },
    ],
    totalReturned: 2,
    meta: {
      instrumentId: "US.AAPL",
      source: "test",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
      session: "all",
      extendedHours: true,
    },
  };
}

function createConsoleDataState() {
  return {
    currentMarketDataCandles: ref(createCandlesResult()),
    currentMarketDataSnapshot: ref({
      request: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
      snapshot: {
        price: 200,
        bid: 199.9,
        ask: 200.1,
        volume: 1000,
        turnover: 200000,
        at: "2026-07-03T12:00:00.000Z",
        observedAt: "2026-07-03T12:00:00.000Z",
        session: "regular",
      },
      meta: {
        instrumentId: "US.AAPL",
        source: "test",
        resolvedAt: "2026-07-03T12:00:00.000Z",
        fromCache: false,
      },
    }),
    currentMarketSecurityDetails: ref(null),
    marketDataQueryMarket: ref("US"),
    marketDataQuerySymbol: ref("AAPL"),
    marketDataQueryPeriod: ref("1m"),
    marketDataQueryError: ref(""),
    isLoadingOlderMarketData: ref(false),
    hasMoreMarketDataHistory: ref(false),
    marketDataNextBefore: ref(""),
    marketDataOlderError: ref(""),
    isLoadingMarketDataQuery: ref(false),
    loadMarketDataQuery: vi.fn().mockResolvedValue(undefined),
    selectMarketDataInstrument: vi.fn(),
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
  const prefs = ref({ market: "us", symbol: "aapl", period: "1m", chartType: "standard" as const });
  return { prefs, update: vi.fn((patch: Partial<typeof prefs.value>) => { prefs.value = { ...prefs.value, ...patch }; }) };
}

function mountChart() {
  useBrokerProviderSelection().brokerDescriptors.value = [{
    id: "test",
    displayName: "Test Provider",
    capabilities: [{
      market: "US",
      supportsQuote: true,
      supportsTrade: false,
      features: [{
        id: "market.candles",
        state: "available",
        supportedPeriods: ["1m"],
        supportedSessions: [
          { id: "regular", supportedPeriods: ["1m"] },
          { id: "extended", supportedPeriods: ["1m"] },
          { id: "overnight", supportedPeriods: ["1m"] },
        ],
      }],
    }],
  }];
  return mount(LightweightChart, {
    attachTo: document.body,
    global: {
      stubs: {
        KlineChart: {
          props: ["candles"],
          template: "<div class='kline-chart-stub'>{{ candles.length }} candles</div>",
        },
      },
    },
  });
}

async function flushUi() {
  await Promise.resolve();
  await nextTick();
  await Promise.resolve();
  await nextTick();
}

afterEach(() => {
  resetBrokerProviderSelectionForTests();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

describe("LightweightChart candle sessions", () => {
  it("filters rendered candles and reloads with the selected sessions", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };
    const wrapper = mountChart();
    await flushUi();
    expect(wrapper.get(".kline-chart-stub").text()).toContain("2 candles");
    expect(wrapper.text()).not.toContain("已到最早数据");
    stores.consoleData.loadMarketDataQuery.mockClear();

    await wrapper.get(".lightweight-chart-session-selector__trigger").trigger("click");
    const overnight = document.querySelectorAll<HTMLInputElement>(
      ".lightweight-chart-session-selector__option input",
    )[2]!;
    overnight.checked = false;
    overnight.dispatchEvent(new Event("change", { bubbles: true }));
    await flushUi();
    expect(wrapper.get(".kline-chart-stub").text()).toContain("1 candles");
    expect(wrapper.text()).not.toContain("已到最早数据");
    expect(stores.consoleData.loadMarketDataQuery).toHaveBeenCalledWith({
      sessions: ["regular", "extended"],
    });

    const extended = document.querySelectorAll<HTMLInputElement>(
      ".lightweight-chart-session-selector__option input",
    )[1]!;
    extended.checked = false;
    extended.dispatchEvent(new Event("change", { bubbles: true }));
    await flushUi();
    expect(wrapper.text()).not.toContain("已到最早数据");
    wrapper.unmount();
  });
});
