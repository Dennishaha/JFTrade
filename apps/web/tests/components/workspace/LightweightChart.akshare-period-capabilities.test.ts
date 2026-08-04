// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { nextTick, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

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

import LightweightChart from "../../../src/components/workspace/LightweightChart.vue";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "@/composables/trading/brokerProviderSelection";

function securityDetails(supportedPeriods: string[]) {
  return {
    request: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
    security: { instrumentId: "US.AAPL", supportedPeriods },
    meta: {
      instrumentId: "US.AAPL",
      source: "akshare:eastmoney",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
    },
  };
}

function createConsoleDataState() {
  return {
    currentMarketDataCandles: ref(null),
    currentMarketDataSnapshot: ref(null),
    currentMarketSecurityDetails: ref<ReturnType<typeof securityDetails> | null>(
      null,
    ),
    marketDataQueryMarket: ref("US"),
    marketDataQuerySymbol: ref("AAPL"),
    marketDataQueryPeriod: ref("1m"),
    marketDataQueryError: ref(""),
    isLoadingMarketDataQuery: ref(false),
    isLoadingOlderMarketData: ref(false),
    hasMoreMarketDataHistory: ref(false),
    marketDataNextBefore: ref(""),
    marketDataOlderError: ref(""),
    loadMarketDataQuery: vi.fn().mockResolvedValue(undefined),
    selectMarketDataInstrument: vi.fn(),
    selectWorkspaceInstrument: vi.fn(),
    acquireMarketDataSubscription: vi.fn().mockResolvedValue(true),
    createStableWebConsumerId: vi.fn(() => "workspace-chart:akshare"),
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
    chartType: "standard" as const,
  });
  return {
    prefs,
    update: vi.fn((patch: Partial<typeof prefs.value>) => {
      prefs.value = { ...prefs.value, ...patch };
    }),
  };
}

function mountAKShareChart() {
  const selection = useBrokerProviderSelection();
  selection.selectBrokerProvider("akshare");
  selection.brokerDescriptors.value = [
    { id: "akshare", displayName: "AKShare", capabilities: [] },
  ];
  return mount(LightweightChart, {
    attachTo: document.body,
    global: {
      stubs: {
        KlineChart: { template: "<div class='kline-chart-stub' />" },
      },
    },
  });
}

async function flushUI(): Promise<void> {
  await Promise.resolve();
  await nextTick();
  await Promise.resolve();
  await nextTick();
}

afterEach(() => {
  resetBrokerProviderSelectionForTests();
});

describe("LightweightChart AKShare period capabilities", () => {
  it("bootstraps details from an unsupported selected period before enforcing periods", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.workspace.prefs.value.period = "tick";
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };
    const wrapper = mountAKShareChart();
    await flushUI();

    expect(wrapper.text()).toContain("正在读取周期能力");
    expect(
      wrapper
        .findAll(".lightweight-chart-head__periods button")
        .every((button) => button.attributes("disabled") !== undefined),
    ).toBe(true);
    expect(stores.consoleData.loadMarketDataQuery).toHaveBeenCalledWith({});
    expect(stores.consoleData.selectWorkspaceInstrument).toHaveBeenLastCalledWith({
      market: "US",
      symbol: "AAPL",
      period: "1m",
    });

    stores.consoleData.currentMarketSecurityDetails.value = securityDetails([
      "1D",
      "1w",
      "1mo",
    ]);
    await flushUI();

    expect(wrapper.text()).not.toContain("正在读取周期能力");
    expect(
      wrapper
        .findAll(".lightweight-chart-head__periods button")
        .map((button) => button.text()),
    ).toEqual(["1D", "1W", "1月"]);
    expect(stores.workspace.update).toHaveBeenCalledWith({ period: "1d" });
    wrapper.unmount();
  });

  it("rejects period details from a previous provider or instrument", async () => {
    stores.consoleData = createConsoleDataState();
    stores.workspace = createWorkspaceState();
    stores.liveHub = { waitForConnection: vi.fn().mockResolvedValue(true) };
    const staleProvider = securityDetails(["1d"]);
    staleProvider.meta.source = "yfinance";
    stores.consoleData.currentMarketSecurityDetails.value = staleProvider;
    const wrapper = mountAKShareChart();
    await flushUI();

    expect(wrapper.text()).toContain("正在读取周期能力");
    expect(stores.workspace.update).not.toHaveBeenCalledWith({ period: "1d" });

    const staleInstrument = securityDetails(["1d"]);
    staleInstrument.request.instrumentId = "HK.00700";
    stores.consoleData.currentMarketSecurityDetails.value = staleInstrument;
    await flushUI();
    expect(wrapper.text()).toContain("正在读取周期能力");

    stores.consoleData.currentMarketSecurityDetails.value = securityDetails([
      "1d",
    ]);
    await flushUI();
    expect(wrapper.text()).not.toContain("正在读取周期能力");
    expect(stores.workspace.update).toHaveBeenCalledWith({ period: "1d" });
    wrapper.unmount();
  });
});
