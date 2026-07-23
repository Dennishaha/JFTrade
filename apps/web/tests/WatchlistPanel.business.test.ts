// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import type { MarketSecurityDetails } from "@/contracts";

const mocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
  resolveBrokerQuery: vi.fn(),
  supportsBrokerReadFeature: vi.fn(),
}));

const marketProfilesState = vi.hoisted(() => ({
  extendedHoursMarkets: new Set<string>(),
}));

let consoleDataState: ReturnType<typeof createConsoleDataState>;
let workspacePrefsState: { prefs: ReturnType<typeof ref<{ market: string; symbol: string }>> };

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => mocks.fetchEnvelope(...args),
}));

vi.mock("../src/composables/consoleDataBrokerAccountSelection", () => ({
  resolveBrokerQuery: (...args: unknown[]) => mocks.resolveBrokerQuery(...args),
}));

vi.mock("../src/composables/marketProfiles", () => ({
  useMarketProfiles: () => ({
    marketProfiles: ref([]),
    pricePrecisionForMarket: (market: string | null | undefined) =>
      (market ?? "").trim().toUpperCase() === "HK" ? 3 : 2,
    supportsExtendedHoursForMarket: (market: string | null | undefined) =>
      marketProfilesState.extendedHoursMarkets.has(
        (market ?? "").trim().toUpperCase(),
      ),
  }),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleDataState,
}));

vi.mock("../src/composables/useWorkspaceLayout", () => ({
  useWorkspaceTradingPrefs: () => workspacePrefsState,
}));

import WatchlistPanel from "../src/components/workspace/InstrumentOverviewPanel.vue";

type SetupState = Record<string, unknown>;

const wrappers: VueWrapper[] = [];

function createConsoleDataState() {
  return {
    currentMarketDataSnapshot: ref(createSnapshotResult("US", "AAPL", 100, 100)),
    currentMarketSecurityDetails: ref(
      createSecurityResult(
        createSecurityDetails({
          instrumentId: "US.AAPL",
          market: "US",
          symbol: "AAPL",
          name: "Apple",
          securityType: "Eqty",
          exchangeType: "US_NASDAQ",
        }),
      ),
    ),
    marketInstrumentSearchOptions: ref<Array<{ instrumentId: string; name: string }>>(
      [],
    ),
    selectedBrokerAccount: ref(null),
    brokerRuntime: ref({
      descriptor: {
        id: "futu",
      },
    }),
    systemStatus: ref({
      defaultTradingEnvironment: "REAL",
    }),
    supportsBrokerReadFeature: mocks.supportsBrokerReadFeature,
  };
}

function mountWatchlistPanel() {
  const wrapper = mount(WatchlistPanel, {
    global: {
      stubs: {
        MarketStatusBadge: {
          props: ["state"],
          template: "<div data-testid='market-status'>{{ state }}</div>",
        },
        DenseMetricStrip: defineComponent({
          props: ["items"],
          template:
            "<div class='dense-strip'><div v-for='item in items' :key='item.label'>{{ item.label }}:{{ item.value }}</div></div>",
        }),
      },
    },
    attachTo: document.body,
  });
  wrappers.push(wrapper);
  return wrapper;
}

function panelSetup(wrapper: VueWrapper): SetupState {
  return wrapper.vm.$.setupState as SetupState;
}

function readSetupValue<T>(wrapper: VueWrapper, key: string): T {
  const value = panelSetup(wrapper)[key];
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T }).value;
  }
  return value as T;
}

function writeSetupValue<T>(wrapper: VueWrapper, key: string, value: T): void {
  const current = panelSetup(wrapper)[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: T }).value = value;
    return;
  }
  panelSetup(wrapper)[key] = value;
}

function callSetup<T>(wrapper: VueWrapper, key: string, ...args: unknown[]): T {
  return (panelSetup(wrapper)[key] as (...values: unknown[]) => T)(...args);
}

async function flushWatchlist(): Promise<void> {
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

beforeEach(() => {
  vi.clearAllMocks();
  document.body.innerHTML = "";
  marketProfilesState.extendedHoursMarkets.clear();
  consoleDataState = createConsoleDataState();
  workspacePrefsState = {
    prefs: ref({
      market: "US",
      symbol: "AAPL",
    }),
  };
  mocks.fetchEnvelope.mockResolvedValue({
    marginRatios: [],
    connectivity: "connected",
    lastError: "",
  });
  mocks.resolveBrokerQuery.mockReturnValue(new URLSearchParams());
  mocks.supportsBrokerReadFeature.mockReturnValue(false);
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  document.body.innerHTML = "";
});

describe("WatchlistPanel business flows", () => {
  it("places the watchlist action in the realtime price card", async () => {
    const wrapper = mountWatchlistPanel();

    const favorite = wrapper.get('[data-testid="instrument-overview-favorite"]');
    expect(favorite.attributes("title")).toBe("加入自选");
    expect(favorite.element.closest(".instrument-overview__quote-card")).not.toBeNull();
    expect(wrapper.get(".tv-panel-title").text()).toBe("行情");
    expect(wrapper.find(".tv-panel-head .instrument-identity").exists()).toBe(
      false,
    );
    expect(wrapper.get(".quote-summary__identity-row").text()).toContain(
      "US.AAPL · Apple",
    );
    await favorite.trigger("click");
    await nextTick();
    expect(readSetupValue<boolean>(wrapper, "membershipDialogOpen")).toBe(true);
  });

  it("prefers searched instrument names and keeps neutral snapshots free of up/down coloring", () => {
    consoleDataState.marketInstrumentSearchOptions.value = [
      {
        instrumentId: "US.AAPL",
        name: "Apple Inc.",
      },
    ];
    consoleDataState.currentMarketSecurityDetails.value = createSecurityResult(
      createSecurityDetails({
        instrumentId: "US.AAPL",
        market: "US",
        symbol: "AAPL",
        name: "Fallback Apple",
        securityType: "Eqty",
        exchangeType: "US_NASDAQ",
        sessionStatus: "",
      }),
    );
    consoleDataState.currentMarketDataSnapshot.value = createSnapshotResult(
      "US",
      "AAPL",
      100,
      100,
    );

    const wrapper = mountWatchlistPanel();

    expect(wrapper.text()).toContain("US.AAPL · Apple Inc.");
    expect(wrapper.get(".quote-summary__price").classes()).not.toEqual(
      expect.arrayContaining(["tv-up", "tv-down"]),
    );
    expect(wrapper.get(".quote-summary__change").classes()).not.toEqual(
      expect.arrayContaining(["tv-up", "tv-down"]),
    );
    expect(callSetup<string>(wrapper, "formatSecurityStatus", createSecurityDetails({
      instrumentId: "US.HALT",
      market: "US",
      symbol: "HALT",
      name: "Halt Corp",
      securityType: "Eqty",
      exchangeType: "US_NASDAQ",
      isSuspend: true,
      sessionStatus: "",
    }))).toBe("停牌");
    expect(callSetup<string>(wrapper, "formatSecurityStatus", createSecurityDetails({
      instrumentId: "US.OPEN",
      market: "US",
      symbol: "OPEN",
      name: "Open Corp",
      securityType: "Eqty",
      exchangeType: "US_NASDAQ",
      sessionStatus: "",
    }))).toBe("正常");
    expect(callSetup<string>(wrapper, "formatPlainNumber", null)).toBe("—");
    expect(callSetup<string>(wrapper, "formatCompactNumber", null)).toBe("—");
    expect(callSetup<string>(wrapper, "formatInteger", null)).toBe("—");
    expect(callSetup<string>(wrapper, "formatPercentValue", null)).toBe("—");
    expect(callSetup<string>(wrapper, "formatOwner", null)).toBe("—");
  });

  it("loads qualified-symbol margin ratios and shows the hover popover for long and short permits", async () => {
    mocks.supportsBrokerReadFeature.mockReturnValue(true);
    workspacePrefsState.prefs.value = {
      market: "HK",
      symbol: "00700",
    };
    consoleDataState.currentMarketDataSnapshot.value = createSnapshotResult(
      "HK",
      "00700",
      321.4,
      318.9,
    );
    consoleDataState.currentMarketSecurityDetails.value = createSecurityResult(
      createSecurityDetails({
        instrumentId: "HK.00700",
        market: "HK",
        symbol: "00700",
        name: "Tencent Holdings",
        securityType: "Eqty",
        exchangeType: "HK_HKEX",
      }),
    );
    consoleDataState.selectedBrokerAccount.value = {
      brokerId: "futu",
      accountId: "REAL-001",
      market: "HK",
      tradingEnvironment: "REAL",
      selectionKey: "real-hk",
    };
    mocks.resolveBrokerQuery.mockReturnValue(
      new URLSearchParams("accountId=REAL-001&market=HK"),
    );
    mocks.fetchEnvelope.mockResolvedValueOnce({
      marginRatios: [
        {
          symbol: "HK.00700",
          isLongPermit: true,
          isShortPermit: true,
          initialMarginLongRatio: 50,
          maintenanceLongRatio: 35,
          alertLongRatio: 25,
          marginCallLongRatio: 20,
          shortFeeRate: 1.2,
          initialMarginShortRatio: 60,
          maintenanceShortRatio: 45,
          alertShortRatio: 40,
          marginCallShortRatio: 30,
          shortPoolRemain: 12345,
        },
      ],
      connectivity: "connected",
      lastError: "",
    });

    const wrapper = mountWatchlistPanel();
    await flushWatchlist();

    expect(mocks.resolveBrokerQuery).toHaveBeenCalledWith({
      selection: consoleDataState.selectedBrokerAccount.value,
      runtime: consoleDataState.brokerRuntime.value,
      status: consoleDataState.systemStatus.value,
    });
    expect(mocks.fetchEnvelope).toHaveBeenCalledWith(
      "/api/v1/brokers/futu/margin-ratios?accountId=REAL-001&market=HK&symbol=00700",
    );
    expect(wrapper.text()).toContain("融");
    expect(wrapper.text()).toContain("沽");

    const trigger = wrapper
      .findAll("span")
      .find((node) => node.text().includes("融") && node.text().includes("沽"));
    expect(trigger).toBeTruthy();
    Object.assign(trigger!.element, {
      getBoundingClientRect: () => ({
        bottom: 40,
        right: 120,
        left: 0,
        top: 0,
        width: 0,
        height: 0,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      }),
    });

    await trigger!.trigger("mouseenter");
    await nextTick();

    expect(readSetupValue(wrapper, "marginPopoverTop")).toBe(48);
    expect(readSetupValue(wrapper, "marginPopoverRight")).toBe(
      window.innerWidth - 120,
    );
    expect(document.body.textContent).toContain("融资融券信息");
    expect(document.body.textContent).toContain("卖空池剩余");
    expect(document.body.textContent).toContain("12,345");

    await trigger!.trigger("mouseleave");
    await nextTick();
    expect(readSetupValue(wrapper, "marginHovered")).toBe(false);
  });

  it("resets margin state when the account or symbol is missing", async () => {
    mocks.supportsBrokerReadFeature.mockReturnValue(true);

    const wrapper = mountWatchlistPanel();
    writeSetupValue(wrapper, "marginHovered", true);
    writeSetupValue(wrapper, "currentMarginRatio", {
      marginRatios: [{ symbol: "US.AAPL", isLongPermit: true }],
      connectivity: "connected",
      lastError: "",
    });

    callSetup<void>(wrapper, "updateMarginPopoverPosition");
    await callSetup<Promise<void>>(wrapper, "fetchCurrentMarginRatio");

    expect(readSetupValue<{ marginRatios: unknown[] }>(wrapper, "currentMarginRatio").marginRatios).toHaveLength(0);
    expect(readSetupValue(wrapper, "marginHovered")).toBe(false);
    expect(mocks.fetchEnvelope).not.toHaveBeenCalled();

    consoleDataState.selectedBrokerAccount.value = {
      brokerId: "futu",
      accountId: "REAL-001",
      market: "US",
      tradingEnvironment: "REAL",
      selectionKey: "real-us",
    };
    workspacePrefsState.prefs.value = {
      market: "US",
      symbol: "",
    };

    await callSetup<Promise<void>>(wrapper, "fetchCurrentMarginRatio");

    expect(mocks.fetchEnvelope).not.toHaveBeenCalled();
    expect(readSetupValue(wrapper, "isLoadingCurrentMarginRatio")).toBe(false);
  });

  it("keeps only the newest margin-ratio response and clears the entry after a failed refresh", async () => {
    mocks.supportsBrokerReadFeature.mockReturnValue(true);
    consoleDataState.selectedBrokerAccount.value = {
      brokerId: "futu",
      accountId: "REAL-001",
      market: "US",
      tradingEnvironment: "REAL",
      selectionKey: "real-us",
    };
    mocks.resolveBrokerQuery.mockReturnValue(
      new URLSearchParams("accountId=REAL-001&market=US"),
    );

    const first = deferred<unknown>();
    const second = deferred<unknown>();
    mocks.fetchEnvelope.mockImplementationOnce(() => first.promise);
    mocks.fetchEnvelope.mockImplementationOnce(() => second.promise);

    const wrapper = mountWatchlistPanel();
    await flushWatchlist();

    workspacePrefsState.prefs.value = {
      market: "US",
      symbol: "TSLA",
    };
    await flushWatchlist();

    first.resolve({
      marginRatios: [
        {
          symbol: "US.AAPL",
          isLongPermit: true,
          isShortPermit: false,
        },
      ],
      connectivity: "connected",
      lastError: "",
    });
    second.resolve({
      marginRatios: [
        {
          symbol: "US.TSLA",
          isLongPermit: false,
          isShortPermit: true,
        },
      ],
      connectivity: "connected",
      lastError: "",
    });
    await flushWatchlist();

    expect(wrapper.text()).toContain("沽");
    expect(wrapper.text()).not.toContain("融沽融");

    mocks.fetchEnvelope.mockRejectedValueOnce(new Error("broker unavailable"));
    await callSetup<Promise<void>>(wrapper, "fetchCurrentMarginRatio");

    expect(readSetupValue<{ marginRatios: unknown[] }>(wrapper, "currentMarginRatio").marginRatios).toHaveLength(0);
    expect(readSetupValue(wrapper, "isLoadingCurrentMarginRatio")).toBe(false);
  });

  it("shows the snapshot placeholder and empty detail sections when market data is unavailable", () => {
    consoleDataState.currentMarketDataSnapshot.value = null;
    consoleDataState.currentMarketSecurityDetails.value = null;

    const wrapper = mountWatchlistPanel();

    expect(wrapper.text()).toContain("当前标的暂无快照");
    expect(readSetupValue<unknown[]>(wrapper, "securitySummaryRows")).toHaveLength(0);
    expect(readSetupValue<unknown[]>(wrapper, "typedDetailSections")).toHaveLength(0);
  });
});

function createSnapshotResult(
  market: string,
  symbol: string,
  price: number,
  previousClosePrice: number,
) {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: {
      market,
      symbol,
      instrumentId,
    },
    snapshot: {
      price,
      bid: price - 0.1,
      ask: price + 0.1,
      previousClosePrice,
      observedAt: "2026-06-01T10:00:00Z",
      at: "2026-06-01T10:00:00Z",
      session: market === "US" ? "regular" : "unknown",
    },
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-06-01T10:00:00Z",
      fromCache: false,
    },
  };
}

function createSecurityResult(security: MarketSecurityDetails) {
  return {
    request: {
      market: security.market,
      symbol: security.symbol,
      instrumentId: security.instrumentId,
    },
    security,
    meta: {
      instrumentId: security.instrumentId,
      source: "test",
      resolvedAt: "2026-06-01T10:00:00Z",
      fromCache: false,
    },
  };
}

function createSecurityDetails(
  overrides: Partial<MarketSecurityDetails> & {
    instrumentId: string;
    market: string;
    symbol: string;
    name: string;
    securityType: string;
    exchangeType: string;
  },
): MarketSecurityDetails {
  return {
    instrumentId: overrides.instrumentId,
    market: overrides.market,
    symbol: overrides.symbol,
    securityId: overrides.securityId ?? 1,
    name: overrides.name,
    securityType: overrides.securityType,
    exchangeType: overrides.exchangeType,
    listTime: overrides.listTime ?? "2024-01-01",
    listTimestamp: overrides.listTimestamp ?? 1704067200,
    delisting: overrides.delisting ?? false,
    lotSize: overrides.lotSize ?? 100,
    isSuspend: overrides.isSuspend ?? false,
    priceSpread: overrides.priceSpread ?? 0.01,
    updateTime: overrides.updateTime ?? "2026-06-01 09:30:00",
    updateTimestamp: overrides.updateTimestamp ?? 1780306200,
    highPrice: overrides.highPrice ?? 101.5,
    openPrice: overrides.openPrice ?? 99.8,
    lowPrice: overrides.lowPrice ?? 98.9,
    lastClosePrice: overrides.lastClosePrice ?? 100,
    currentPrice: overrides.currentPrice ?? 100,
    volume: overrides.volume ?? 1200000,
    turnover: overrides.turnover ?? 400000000,
    turnoverRate: overrides.turnoverRate ?? 1.1,
    askPrice: overrides.askPrice ?? null,
    bidPrice: overrides.bidPrice ?? null,
    askVolume: overrides.askVolume ?? null,
    bidVolume: overrides.bidVolume ?? null,
    amplitude: overrides.amplitude ?? null,
    averagePrice: overrides.averagePrice ?? null,
    bidAskRatio: overrides.bidAskRatio ?? null,
    volumeRatio: overrides.volumeRatio ?? 1.2,
    highest52WeeksPrice: overrides.highest52WeeksPrice ?? 120,
    lowest52WeeksPrice: overrides.lowest52WeeksPrice ?? 80,
    highestHistoryPrice: overrides.highestHistoryPrice ?? null,
    lowestHistoryPrice: overrides.lowestHistoryPrice ?? null,
    sessionStatus: overrides.sessionStatus ?? "Normal",
    closePrice5Minute: overrides.closePrice5Minute ?? null,
    highPrecisionVolume: overrides.highPrecisionVolume ?? null,
    highPrecisionAskVol: overrides.highPrecisionAskVol ?? null,
    highPrecisionBidVol: overrides.highPrecisionBidVol ?? null,
    extended: overrides.extended ?? null,
    equity: overrides.equity ?? null,
    warrant: overrides.warrant ?? null,
    option: overrides.option ?? null,
    index: overrides.index ?? null,
    plate: overrides.plate ?? null,
    future: overrides.future ?? null,
    trust: overrides.trust ?? null,
  };
}
