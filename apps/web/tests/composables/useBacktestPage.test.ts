// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, reactive } from "vue";

const mocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiGetPath: vi.fn(),
  apiPut: vi.fn(),
  apiPost: vi.fn(),
  apiDeletePath: vi.fn(),
  routerReplace: vi.fn(),
  route: { path: "/backtest", query: {} as Record<string, unknown> },
  extendedHoursMarkets: new Set<string>(),
  quoteCurrencies: new Map<string, string>([
    ["HK", "HKD"],
    ["US", "USD"],
    ["CN", "CNY"],
  ]),
  knownMarkets: new Set<string>(["HK", "US", "CN"]),
}));

vi.mock("vue-router", () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.routerReplace }),
}));

vi.mock("@/composables/market-data/marketProfiles", () => ({
  useMarketProfiles: () => ({
    defaultMarket: { value: "HK" },
    loadMarketProfiles: async () => {},
    findMarketProfile: (market: string | null | undefined) =>
      mocks.knownMarkets.has((market ?? "").trim().toUpperCase())
        ? { code: (market ?? "").trim().toUpperCase() }
        : null,
    quoteCurrencyForMarket: (market: string | null | undefined) =>
      mocks.quoteCurrencies.get((market ?? "").trim().toUpperCase()) ?? "",
    supportsExtendedHoursForMarket: (market: string | null | undefined) =>
      mocks.extendedHoursMarkets.has((market ?? "").trim().toUpperCase()),
    normalizeInstrumentRefWithMarketApi: async () => ({
      market: "HK",
      prefix: "HK",
      code: "00700",
      instrumentId: "HK.00700",
    }),
  }),
}));

vi.mock("@/composables/shared/apiClient", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/composables/shared/apiClient")>();
  return {
    ...actual,
    apiGet: mocks.apiGet,
    apiGetPath: mocks.apiGetPath,
    apiPut: mocks.apiPut,
    apiPost: mocks.apiPost,
    apiDeletePath: mocks.apiDeletePath,
  };
});

vi.mock("@/composables/market-data/useKlineSyncTask", () => ({
  useKlineSyncTask: () => ({
    syncing: { value: false },
    syncProgress: { value: null },
    syncError: { value: "" },
    startSync: vi.fn(),
    cancelSync: vi.fn(),
  }),
}));

import { useBacktestPage } from "@/composables/backtest/useBacktestPage";
import { queryClient } from "@/composables/settings/serverState";

type BacktestPageState = ReturnType<typeof useBacktestPage>;

interface RunFixture {
  id: string;
  status: string;
  definitionId: string;
  definitionVersion?: string;
  symbol?: string;
  interval?: string;
  createdAt: string;
  updatedAt?: string;
  result?: Record<string, unknown>;
  requestOverrides?: Record<string, unknown>;
}

const DEFAULT_DEFINITIONS = [
  { id: "def-1", name: "策略一", version: "2.0.0", symbol: "US.AAPL" },
  { id: "def-2", name: "策略二", version: "1.5.0" },
];

const FUTU_BACKTEST_PROVIDER_SETTINGS = {
  activeProvider: "futu",
  availableProviders: [
    {
      selectionId: "futu",
      providerId: "futu-opend",
      displayName: "Futu OpenD",
      capabilities: {
        historicalCandles: true,
        streamingCandles: true,
        extendedHours: true,
        candleIntervals: ["tick", "1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"],
        priceAdjustments: ["none", "forward", "backward"],
      },
    },
  ],
};

function makeRun(fixture: RunFixture) {
  const symbol = fixture.symbol ?? "US.AAPL";
  const request: Record<string, unknown> = {
    definitionId: fixture.definitionId,
    symbol,
    interval: fixture.interval ?? "5m",
    chartType: "standard",
    startDate: "2026-06-01",
    endDate: "2026-06-30",
    startTime: "2026-06-01T00:00:00Z",
    endTime: "2026-06-30T00:00:00Z",
    initialBalance: 100000,
    rehabType: "forward",
    useExtendedHours: false,
    ...fixture.requestOverrides,
  };
  if (fixture.definitionVersion != null) {
    request.definitionVersion = fixture.definitionVersion;
  }
  return {
    id: fixture.id,
    status: fixture.status,
    request,
    ...(fixture.result == null ? {} : { result: fixture.result }),
    createdAt: fixture.createdAt,
    updatedAt: fixture.updatedAt ?? fixture.createdAt,
  };
}

function makeComparisonResult(overrides: Record<string, unknown> = {}) {
  return {
    symbol: "US.AAPL",
    interval: "5m",
    startTime: "2026-06-01T00:00:00Z",
    endTime: "2026-06-30T00:00:00Z",
    quoteCurrency: "USD",
    finalBalance: 101000,
    pnl: 1000,
    maxDrawdown: 0.05,
    currentDrawdown: 0.02,
    totalTrades: 10,
    winRate: 0.6,
    totalFees: 100,
    ...overrides,
  };
}

const COMPARISON_VERSIONS = [
  {
    definitionId: "def-1",
    version: "2.0.0",
    name: "策略一",
    savedAt: "2026-07-10T00:00:00Z",
    isCurrent: true,
  },
  {
    definitionId: "def-1",
    version: "1.0.0",
    name: "策略一",
    savedAt: "2026-07-05T00:00:00Z",
    isCurrent: false,
  },
];

function installApiMock(options: {
  definitions?: Array<Record<string, unknown>>;
  runs?: Array<Record<string, unknown>>;
  versions?: Array<Record<string, unknown>>;
} = {}) {
  const definitions = options.definitions ?? DEFAULT_DEFINITIONS;
  const runs = options.runs ?? [];
  const versions = options.versions ?? COMPARISON_VERSIONS;
  const runsById = new Map(runs.map((run) => [String(run.id), run]));

  mocks.apiGet.mockImplementation(async (path: string) => {
    if (path === "/api/v1/settings/backtest-market-data-provider") {
      return FUTU_BACKTEST_PROVIDER_SETTINGS;
    }
    if (path === "/api/v1/strategy-definitions") return definitions;
    if (path === "/api/v1/backtests") return { runs };
    throw new Error(`unexpected apiGet ${path}`);
  });
  mocks.apiGetPath.mockImplementation(async (_path: string, url: string) => {
    const backtestDetail = url.match(/\/api\/v1\/backtests\/([^/?]+)$/);
    if (backtestDetail) {
      const run = runsById.get(decodeURIComponent(backtestDetail[1]!));
      if (run == null) throw new Error(`unknown run ${url}`);
      return run;
    }
    const versionDoc = url.match(
      /\/api\/v1\/strategy-definitions\/([^/]+)\/versions\/([^/?]+)$/,
    );
    if (versionDoc) {
      const version = versions.find(
        (entry) => entry.version === decodeURIComponent(versionDoc[2]!),
      );
      if (version == null) throw new Error(`unknown version ${url}`);
      return { ...version };
    }
    if (/\/api\/v1\/strategy-definitions\/[^/]+\/versions$/.test(url)) {
      return { versions };
    }
    if (url.includes("/api/v1/strategy-definitions/")) {
      return { derivedWarmupBars: 12, derivedWarmupInterval: "5m" };
    }
    throw new Error(`unexpected apiGetPath ${url}`);
  });
  mocks.apiPost.mockResolvedValue({});
  mocks.apiPut.mockResolvedValue(FUTU_BACKTEST_PROVIDER_SETTINGS);
  mocks.apiDeletePath.mockImplementation(async (_path: string, url: string) => ({
    deleted: true,
    id: decodeURIComponent(url.split("/").pop() ?? ""),
  }));
}

function mountBacktestPage(): BacktestPageState {
  let state: BacktestPageState | null = null;
  const Host = defineComponent({
    setup() {
      state = useBacktestPage();
      return () => h("div");
    },
  });
  const wrapper = mount(Host);
  mountedWrappers.push(wrapper);
  if (state == null) throw new Error("backtest page state was not initialized");
  return state;
}

const mountedWrappers: Array<{ unmount: () => void }> = [];

beforeEach(() => {
  queryClient.clear();
  vi.clearAllMocks();
  window.localStorage.clear();
  window.sessionStorage.clear();
  mocks.extendedHoursMarkets.clear();
  mocks.route = reactive({ path: "/backtest", query: {} });
});

afterEach(() => {
  for (const wrapper of mountedWrappers.splice(0)) wrapper.unmount();
  queryClient.clear();
  vi.useRealTimers();
});

describe("useBacktestPage default form state", () => {
  it("opens with a three-year window ending today and market defaults", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 6, 15, 12, 0, 0));
    installApiMock();
    const page = mountBacktestPage();

    expect(page.selectedMarket.value).toBe("HK");
    expect(page.codeInput.value).toBe("00700");
    expect(page.interval.value).toBe("5m");
    expect(page.chartType.value).toBe("standard");
    expect(page.initialBalance.value).toBe(1_000_000);
    expect(page.instrumentType.value).toBe("stock");
    expect(page.rehabType.value).toBe("forward");
    expect(page.brokerFeeMode.value).toBe("market_preset");
    expect(page.marketFeeMode.value).toBe("market_preset");
    expect(page.useExtendedHours.value).toBe(false);
    expect(page.startDate.value).toBe("2023-07-15");
    expect(page.endDate.value).toBe("2026-07-15");
  });

  it("clamps a leap-day window start to the target month's last day", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2024, 1, 29, 12, 0, 0));
    installApiMock();
    const page = mountBacktestPage();

    expect(page.startDate.value).toBe("2021-02-28");
    expect(page.endDate.value).toBe("2024-02-29");
  });

  it("restores stored preferences while discarding invalid values", () => {
    window.localStorage.setItem(
      "jftrade.backtest.form.v1",
      JSON.stringify({
        selectedDefinitionId: " def-9 ",
        selectedMarket: "sh",
        codeInput: "600519",
        interval: "17m",
        chartType: "heikinashi",
        startDate: "2020-01-02",
        endDate: "not-a-date",
        initialBalance: -5,
        instrumentType: "bond",
        rehabType: "weird",
        useExtendedHours: true,
        brokerFeeMode: "bogus",
        marketFeeMode: "custom",
        brokerFeeRulesText: "[{\"id\":\"commission\"}]",
        marketFeeRulesText: "not json",
      }),
    );
    installApiMock();
    const page = mountBacktestPage();

    expect(page.selectedDefinitionId.value).toBe("def-9");
    expect(page.selectedMarket.value).toBe("CN");
    expect(page.codeInput.value).toBe("SH.600519");
    expect(page.instrumentSearchQuery.value).toBe("SH.600519");
    expect(page.interval.value).toBe("5m");
    expect(page.chartType.value).toBe("heikinashi");
    expect(page.startDate.value).toBe("2020-01-02");
    expect(page.endDate.value).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(page.initialBalance.value).toBe(1_000_000);
    expect(page.instrumentType.value).toBe("stock");
    expect(page.rehabType.value).toBe("forward");
    expect(page.brokerFeeMode.value).toBe("market_preset");
    expect(page.marketFeeMode.value).toBe("custom");
    expect(page.brokerFeeRules.value).toEqual([{ id: "commission" }]);
    expect(page.marketFeeRules.value).toEqual([]);
    // CN does not support extended hours in this setup, so the stored flag is forced off.
    expect(page.useExtendedHours.value).toBe(false);
  });

  it("persists form edits back to local storage", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    page.interval.value = "15m";
    await flushPromises();

    const stored = JSON.parse(
      window.localStorage.getItem("jftrade.backtest.form.v1") ?? "{}",
    ) as Record<string, unknown>;
    expect(stored.interval).toBe("15m");
    expect(stored.selectedMarket).toBe("HK");
    expect(stored.codeInput).toBe("00700");
  });

  it("loads and atomically saves the module-specific provider selection", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.backtestMarketDataProvider.value).toBe("futu");
    expect(page.availableRehabTypes.value.map((option) => option.value)).toEqual([
      "forward",
      "backward",
      "none",
    ]);

    const yfinanceSettings = {
      activeProvider: "yfinance",
      availableProviders: [
        ...FUTU_BACKTEST_PROVIDER_SETTINGS.availableProviders,
        {
          selectionId: "yfinance",
          providerId: "yahoo-finance",
          displayName: "Yahoo Finance (yfinance)",
          capabilities: {
            historicalCandles: true,
            streamingCandles: false,
            extendedHours: true,
            candleIntervals: ["1m", "5m", "1d"],
            priceAdjustments: ["none"],
            historicalLookbackDays: { "1m": 7 },
          },
        },
      ],
    };
    mocks.apiPut.mockResolvedValueOnce(yfinanceSettings);
    page.backtestMarketDataProvider.value = "yfinance";
    await page.saveBacktestProviderSettings();
    await flushPromises();

    expect(mocks.apiPut).toHaveBeenCalledWith(
      "/api/v1/settings/backtest-market-data-provider",
      { activeProvider: "yfinance" },
    );
    expect(page.rehabType.value).toBe("none");
    expect(page.availableKlinePeriods.value.map((period) => period.value)).toEqual([
      "1m",
      "5m",
      "1d",
    ]);

    const akshareSettings = {
      activeProvider: "akshare",
      availableProviders: [
        ...yfinanceSettings.availableProviders,
        {
          selectionId: "akshare",
          providerId: "akshare",
          displayName: "AKShare",
          capabilities: {
            historicalCandles: true,
            streamingCandles: false,
            extendedHours: false,
            candleIntervals: ["1m", "5m", "1d"],
            priceAdjustments: ["none"],
            historicalLookbackDays: { "1m": 5, "US:5m": 5 },
          },
        },
      ],
    };
    mocks.apiPut.mockResolvedValueOnce(akshareSettings);
    page.selectedMarket.value = "US";
    page.interval.value = "5m";
    page.startDate.value = "2025-07-13";
    page.backtestMarketDataProvider.value = "akshare";
    await page.saveBacktestProviderSettings();
    await flushPromises();

    expect(page.backtestRangeError.value).toContain("最近 5 天");
  });

  it("rejects an unsupported provider returned by the settings service", async () => {
    installApiMock();
    mocks.apiGet.mockResolvedValueOnce({
      activeProvider: "unsupported",
      availableProviders: [],
    });

    const page = mountBacktestPage();
    await flushPromises();

    expect(page.backtestMarketDataProvider.value).toBe("yfinance");
    expect(page.selectedBacktestProvider.value).toBeNull();
    expect(page.backtestProviderError.value).toBe(
      "服务端返回了不支持的回测行情提供者",
    );
  });

  it("rolls back a failed provider switch when descriptors are unavailable", async () => {
    installApiMock();
    mocks.apiGet.mockResolvedValueOnce({
      activeProvider: "futu",
      availableProviders: null,
    });
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.backtestProviderDescriptors.value).toEqual([]);
    expect(page.selectedBacktestProvider.value).toBeNull();

    page.backtestMarketDataProvider.value = "akshare";
    mocks.apiPut.mockRejectedValueOnce("provider preparation failed");
    await page.saveBacktestProviderSettings();

    expect(page.backtestMarketDataProvider.value).toBe("futu");
    expect(page.backtestProviderSaving.value).toBe(false);
    expect(page.backtestProviderError.value).toBe("provider preparation failed");

    page.backtestMarketDataProvider.value = "yfinance";
    mocks.apiPut.mockRejectedValueOnce(new Error("provider health check failed"));
    await page.saveBacktestProviderSettings();

    expect(page.backtestMarketDataProvider.value).toBe("futu");
    expect(page.backtestProviderError.value).toBe("provider health check failed");
  });

  it("gates the submittable instrument on the search query matching the selection", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.displayInstrumentId.value).toBe("HK.00700");
    expect(page.instrumentSelectionResolved.value).toBe(true);
    expect(page.backtestFormState.value.code).toBe("00700");
    // instrumentId only fills when the code input itself carries a market prefix.
    expect(page.backtestFormState.value.instrumentId).toBe("");

    page.codeInput.value = "US.AAPL";
    page.instrumentSearchQuery.value = "US.AAPL";
    await flushPromises();
    expect(page.backtestFormState.value.code).toBe("US.AAPL");
    expect(page.backtestFormState.value.instrumentId).toBe("US.AAPL");

    page.codeInput.value = "00700";
    page.instrumentSearchQuery.value = "hk.00941";
    await flushPromises();

    expect(page.instrumentSelectionResolved.value).toBe(false);
    expect(page.backtestFormState.value.code).toBe("");
    expect(page.backtestFormState.value.instrumentId).toBe("");

    expect(page.canonicalBacktestInstrumentInput("HK", "00700")).toBe("HK.00700");
    expect(page.canonicalBacktestInstrumentInput("HK", "US.AAPL")).toBe("US.AAPL");
    expect(page.canonicalBacktestInstrumentInput("", "00700")).toBe("00700");
    expect(page.canonicalBacktestInstrumentInput("HK", "hk:00700")).toBe("HK.00700");
  });

  it("derives extended-hours support from the market profile and interval", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.extendedHoursSupported.value).toBe(false);
    expect(page.extendedHoursHint.value).toBe(
      "当前市场或周期不支持扩展交易时段回放与对应同步版本。",
    );
    expect(page.quoteCurrency.value).toBe("HKD");

    mocks.extendedHoursMarkets.add("US");
    page.selectedMarket.value = "US";
    await flushPromises();
    expect(page.extendedHoursSupported.value).toBe(true);
    expect(page.extendedHoursHint.value).toContain("regular-only");
    expect(page.quoteCurrency.value).toBe("USD");

    page.useExtendedHours.value = true;
    await flushPromises();
    expect(page.extendedHoursHint.value).toContain("extended");

    page.interval.value = "3d";
    await flushPromises();
    expect(page.extendedHoursSupported.value).toBe(false);
    expect(page.useExtendedHours.value).toBe(false);
  });

  it("derives the warmup preview text from the definition detail", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.selectedDefinitionId.value).toBe("def-1");
    expect(page.warmupPreviewValue.value).toBe("12 根");
    expect(page.warmupPreviewNote.value).toContain("5m");

    page.selectedDefinitionId.value = "";
    await flushPromises();
    expect(page.warmupPreviewValue.value).toBe("--");
  });
});

describe("useBacktestPage rehab options follow provider capabilities", () => {
  function makeProviderDescriptor(
    selectionId: string,
    priceAdjustments?: string[],
  ) {
    return {
      selectionId,
      providerId: `${selectionId}-provider`,
      displayName: selectionId,
      capabilities: {
        historicalCandles: true,
        streamingCandles: false,
        extendedHours: false,
        candleIntervals: ["1m", "5m", "1d"],
        ...(priceAdjustments == null ? {} : { priceAdjustments }),
      },
    };
  }

  function rehabOptionValues(page: BacktestPageState) {
    return page.availableRehabTypes.value.map((option) => option.value);
  }

  it("offers all three adjustments for futu", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.backtestMarketDataProvider.value).toBe("futu");
    expect(page.availableRehabTypes.value).toEqual([
      { value: "forward", label: "前复权" },
      { value: "backward", label: "后复权" },
      { value: "none", label: "不复权" },
    ]);
  });

  it("limits yfinance to none and forward while keeping a valid selection", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    page.backtestProviderDescriptors.value = [
      makeProviderDescriptor("yfinance", ["none", "forward"]),
    ] as never;
    page.backtestMarketDataProvider.value = "yfinance";
    await flushPromises();

    expect(rehabOptionValues(page)).toEqual(["forward", "none"]);
    expect(page.rehabType.value).toBe("forward");
  });

  it("offers all three adjustments for akshare", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    page.backtestProviderDescriptors.value = [
      makeProviderDescriptor("akshare", ["none", "forward", "backward"]),
    ] as never;
    page.backtestMarketDataProvider.value = "akshare";
    await flushPromises();

    expect(rehabOptionValues(page)).toEqual(["forward", "backward", "none"]);
  });

  it("falls back to none when the provider does not advertise adjustments", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    page.backtestProviderDescriptors.value = [
      makeProviderDescriptor("akshare"),
    ] as never;
    page.backtestMarketDataProvider.value = "akshare";
    await flushPromises();

    expect(rehabOptionValues(page)).toEqual(["none"]);
    expect(page.rehabType.value).toBe("none");
  });

  it("falls back to none for an unknown provider without dropping the stored selection early", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    page.backtestProviderDescriptors.value = [];
    page.backtestMarketDataProvider.value = "akshare";
    await flushPromises();

    expect(page.selectedBacktestProvider.value).toBeNull();
    expect(rehabOptionValues(page)).toEqual(["none"]);
    // 能力未加载时不清空已保存的选择，等待供应商能力恢复。
    expect(page.rehabType.value).toBe("forward");
  });

  it("resets an unsupported selection to none after a provider switch", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    page.rehabType.value = "backward";
    await flushPromises();
    expect(page.rehabType.value).toBe("backward");

    page.backtestProviderDescriptors.value = [
      makeProviderDescriptor("yfinance", ["none", "forward"]),
    ] as never;
    page.backtestMarketDataProvider.value = "yfinance";
    await flushPromises();

    expect(rehabOptionValues(page)).toEqual(["forward", "none"]);
    expect(page.rehabType.value).toBe("none");
  });
});

describe("useBacktestPage comparison metric derivation", () => {
  it("formats comparison metrics by kind with safe fallbacks", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.formatComparisonMetric(undefined, "currency", "USD")).toBe("--");
    expect(page.formatComparisonMetric(Number.NaN, "number")).toBe("--");
    expect(page.formatComparisonMetric(0.1234, "percent")).toBe("12.34%");
    expect(page.formatComparisonMetric(1234.5, "currency", "HKD")).toBe("1,234.50 HKD");
    expect(page.formatComparisonMetric(1234.5, "currency", "")).toBe("1,234.50");
    expect(page.formatComparisonMetric(7.123, "number")).toBe("7.12");
    expect(page.formatComparisonCurrency(undefined, "USD")).toBe("--");
  });

  it("formats metric deltas with a positive sign and guards missing sides", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(
      page.comparisonMetricDelta({ label: "交易数", kind: "number", left: 10, right: 15 }),
    ).toBe("+5");
    expect(
      page.comparisonMetricDelta({ label: "收益", kind: "currency", left: 1000, right: 2500.5 }),
    ).toBe("+1,500.50");
    expect(
      page.comparisonMetricDelta({ label: "胜率", kind: "percent", left: 0.6, right: 0.55 }),
    ).toBe("-5.00%");
    expect(
      page.comparisonMetricDelta({ label: "交易数", kind: "number", left: undefined, right: 15 }),
    ).toBe("--");
  });

  it("resolves run quote currencies and session modes from the run context", async () => {
    mocks.extendedHoursMarkets.add("US");
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(
      page.resolveRunQuoteCurrency({
        request: { symbol: "HK.00700" },
        result: { quoteCurrency: "USD" },
      }),
    ).toBe("USD");
    expect(page.resolveRunQuoteCurrency({ request: { symbol: "HK.00700" } })).toBe("HKD");
    expect(page.resolveRunQuoteCurrency({ request: { symbol: "XX.0000" } })).toBe("");

    expect(
      page.resolveRunSessionMode({
        request: { symbol: "HK.00700", interval: "5m", useExtendedHours: true },
      }),
    ).toBe("常规时段");
    expect(
      page.resolveRunSessionMode({
        request: { symbol: "US.AAPL", interval: "5m", useExtendedHours: true },
      }),
    ).toBe("含扩展时段");
    expect(
      page.resolveRunSessionMode({
        request: { symbol: "US.AAPL", interval: "5m", useExtendedHours: false },
      }),
    ).toBe("仅常规时段");
    expect(
      page.resolveRunSessionMode({
        request: { symbol: "US.AAPL", interval: "3d", useExtendedHours: true },
      }),
    ).toBe("常规时段");
  });

  it("explains when a run used an outdated or missing strategy definition", async () => {
    installApiMock();
    const page = mountBacktestPage();
    await flushPromises();

    expect(
      page.resolveBacktestStrategyVersionNotice({
        request: { definitionId: "def-1", definitionVersion: " " },
      }),
    ).toBe("");
    expect(
      page.resolveBacktestStrategyVersionNotice({
        request: { definitionId: "def-gone", definitionVersion: "1.0.0" },
      }),
    ).toBe("历史策略回测结果：当前策略定义已不存在；该结果基于策略 v1.0.0。");
    expect(
      page.resolveBacktestStrategyVersionNotice({
        request: { definitionId: "def-1", definitionVersion: "1.0.0" },
      }),
    ).toBe("旧版本策略回测结果：当时策略 v1.0.0，当前已更新到 v2.0.0。");
    expect(
      page.resolveBacktestStrategyVersionNotice({
        request: { definitionId: "def-1", definitionVersion: "2.0.0" },
      }),
    ).toBe("");

    expect(page.formatStrategyVersion("")).toBe("版本未知");
    expect(page.formatStrategyVersion("3.1.0")).toBe("v3.1.0");
    expect(page.resolveStrategyName("def-1")).toBe("策略一");
    expect(page.resolveStrategyName("def-gone")).toBe("def-gone");
    expect(page.resolveStrategyName(undefined)).toBe("未命名策略");
  });
});

describe("useBacktestPage run list filtering and selection", () => {
  const LIST_RUNS = [
    makeRun({ id: "run-a", status: "completed", definitionId: "def-1", createdAt: "2026-07-01T00:00:00Z" }),
    makeRun({ id: "run-b", status: "failed", definitionId: "def-1", symbol: "HK.00700", createdAt: "2026-07-02T00:00:00Z" }),
    makeRun({ id: "run-c", status: "completed", definitionId: "def-2", createdAt: "2026-07-03T00:00:00Z" }),
    makeRun({ id: "run-d", status: "running", definitionId: "def-2", symbol: "US.TSLA", createdAt: "2026-07-04T00:00:00Z" }),
    makeRun({ id: "run-e", status: "completed", definitionId: "def-1", createdAt: "2026-07-05T00:00:00Z" }),
    makeRun({ id: "run-f", status: "cancelled", definitionId: "def-3", createdAt: "2026-07-06T00:00:00Z" }),
    makeRun({
      id: "run-g",
      status: "completed",
      definitionId: "def-1",
      symbol: "HK.00700",
      createdAt: "2026-07-07T00:00:00Z",
      result: makeComparisonResult({
        symbol: "HK.00700",
        quoteCurrency: "HKD",
        pnlCurve: [{ time: "2026-06-30T00:00:00Z", equity: 101000 }],
      }),
    }),
  ];

  async function mountWithRuns() {
    installApiMock({ runs: LIST_RUNS });
    const page = mountBacktestPage();
    await flushPromises();
    return page;
  }

  it("filters runs by status, strategy, and free-text search", async () => {
    const page = await mountWithRuns();

    expect(page.filteredRuns.value.map((run) => run.id)).toEqual([
      "run-g", "run-f", "run-e", "run-d", "run-c", "run-b", "run-a",
    ]);
    expect(page.resultStrategyOptions.value).toEqual([
      { value: "all", title: "全部策略" },
      { value: "def-1", title: "策略一" },
      { value: "def-3", title: "def-3" },
      { value: "def-2", title: "策略二" },
    ]);
    expect(page.hasResultsFilters.value).toBe(false);

    page.resultsStatusFilter.value = "completed";
    expect(page.filteredRuns.value.map((run) => run.id)).toEqual([
      "run-g", "run-e", "run-c", "run-a",
    ]);
    expect(page.hasResultsFilters.value).toBe(true);

    page.resultsStatusFilter.value = "all";
    page.resultsStrategyFilter.value = "def-1";
    expect(page.filteredRuns.value.map((run) => run.id)).toEqual([
      "run-g", "run-e", "run-b", "run-a",
    ]);

    page.resultsStrategyFilter.value = "all";
    page.resultsSearchQuery.value = "hk.00700";
    expect(page.filteredRuns.value.map((run) => run.id)).toEqual(["run-g", "run-b"]);

    page.resultsSearchQuery.value = "策略一";
    expect(page.filteredRuns.value.map((run) => run.id)).toEqual([
      "run-g", "run-e", "run-b", "run-a",
    ]);

    page.resultsSearchQuery.value = "不存在的标的";
    expect(page.filteredRuns.value).toEqual([]);
    expect(page.emptyResultsMessage.value).toBe("没有匹配当前搜索或筛选条件的回测结果。");

    page.resetResultsFilters();
    expect(page.hasResultsFilters.value).toBe(false);
    expect(page.filteredRuns.value).toHaveLength(7);
  });

  it("reports the empty-list message before any run exists", async () => {
    installApiMock({ runs: [] });
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.emptyResultsMessage.value).toBe("暂无回测记录。请在左侧配置参数并启动回测。");
    expect(page.resultsPageSummary.value).toBe("");
  });

  it("paginates the filtered results five per page", async () => {
    const page = await mountWithRuns();

    expect(page.resultsPageCount.value).toBe(2);
    expect(page.pagedRuns.value.map((run) => run.id)).toEqual([
      "run-g", "run-f", "run-e", "run-d", "run-c",
    ]);
    expect(page.resultsPageSummary.value).toBe("第 1-5 条，共 7 条");

    page.resultsPage.value = 2;
    expect(page.pagedRuns.value.map((run) => run.id)).toEqual(["run-b", "run-a"]);
    expect(page.resultsPageSummary.value).toBe("第 6-7 条，共 7 条");

    page.resultsSearchQuery.value = "hk.00700";
    await flushPromises();
    expect(page.resultsPage.value).toBe(1);
    expect(page.resultsPageSummary.value).toBe("筛选后第 1-2 条，共 2 条；全部结果 7 条");
  });

  it("focuses the newest run and switches focus explicitly", async () => {
    const page = await mountWithRuns();

    expect(page.selectedRunId.value).toBe("run-g");
    expect(page.focusedRun.value?.id).toBe("run-g");
    expect(page.focusedRunResultReady.value).toBe(true);
    expect(page.focusedRunHasChartData.value).toBe(true);

    page.selectFocusedRun("run-b");
    await flushPromises();

    expect(page.reportMode.value).toBe("single");
    expect(page.activeReportTab.value).toBe("chart");
    expect(page.backtestMobileSection.value).toBe("report");
    expect(page.focusedRun.value?.id).toBe("run-b");
    expect(page.focusedRunResultReady.value).toBe(false);
    expect(page.focusedRunHasChartData.value).toBe(false);
    // The failed run has no embedded result, so the detail endpoint was queried.
    expect(
      mocks.apiGetPath.mock.calls.some(([, url]) =>
        String(url).includes("/api/v1/backtests/run-b"),
      ),
    ).toBe(true);
  });

  it("guards deletion to terminal runs and removes confirmed runs", async () => {
    const page = await mountWithRuns();

    page.requestDeleteRun("run-d");
    expect(page.pendingDeleteRunId.value).toBe("");

    page.requestDeleteRun("run-g");
    expect(page.pendingDeleteRunId.value).toBe("run-g");
    expect(page.pendingDeleteMessage.value).toContain("run-g");
    expect(page.pendingDeleteMessage.value).toContain("策略一");
    expect(page.pendingDeleteMessage.value).toContain("HK.00700");

    await page.confirmDeleteRun();
    await flushPromises();

    expect(mocks.apiDeletePath).toHaveBeenCalledWith(
      "/api/v1/backtests/{runId}",
      "/api/v1/backtests/run-g",
    );
    expect(page.pendingDeleteRunId.value).toBe("");
    expect(page.filteredRuns.value.some((run) => run.id === "run-g")).toBe(false);
  });
});

describe("useBacktestPage version comparison state", () => {
  const COMPARISON_RUNS = [
    makeRun({
      id: "run-cmp-old",
      status: "completed",
      definitionId: "def-1",
      definitionVersion: "1.0.0",
      createdAt: "2026-07-08T00:00:00Z",
      result: makeComparisonResult(),
    }),
    makeRun({
      id: "run-cmp-new",
      status: "completed",
      definitionId: "def-1",
      definitionVersion: "2.0.0",
      createdAt: "2026-07-11T00:00:00Z",
      result: makeComparisonResult({ finalBalance: 103000, pnl: 3000, winRate: 0.65 }),
    }),
    makeRun({
      id: "run-cmp-running",
      status: "running",
      definitionId: "def-1",
      definitionVersion: "2.0.0",
      createdAt: "2026-07-12T00:00:00Z",
    }),
  ];

  async function mountForComparison() {
    installApiMock({ runs: COMPARISON_RUNS });
    const page = mountBacktestPage();
    await flushPromises();
    return page;
  }

  it("defaults the comparison to the previous versus latest version", async () => {
    const page = await mountForComparison();

    page.activateComparisonMode();
    await flushPromises();

    expect(page.reportMode.value).toBe("compare");
    expect(page.comparisonDefinitionId.value).toBe("def-1");
    expect(page.comparisonVersions.value.map((version) => version.version)).toEqual([
      "2.0.0", "1.0.0",
    ]);
    expect(page.leftComparisonVersion.value).toBe("1.0.0");
    expect(page.rightComparisonVersion.value).toBe("2.0.0");
    // Only completed runs qualify, so the running 2.0.0 run is excluded.
    expect(page.leftComparisonRunId.value).toBe("run-cmp-old");
    expect(page.rightComparisonRunId.value).toBe("run-cmp-new");
    expect(page.comparisonRunsReady.value).toBe(true);
    expect(page.backtestMobileSection.value).toBe("report");

    expect(page.leftComparisonVersionSelectOptions.value).toEqual([
      { value: "1.0.0", title: "v1.0.0" },
    ]);
    expect(page.rightComparisonVersionSelectOptions.value).toEqual([
      { value: "2.0.0", title: "v2.0.0（当前）" },
    ]);
    expect(page.comparisonRunOptionTitle(page.leftComparisonRun.value!)).toContain(
      "run-cmp-old",
    );
  });

  it("derives metric rows and signed deltas from both runs", async () => {
    const page = await mountForComparison();
    page.activateComparisonMode();
    await flushPromises();

    const metrics = page.comparisonMetrics.value;
    expect(metrics.map((metric) => metric.label)).toEqual([
      "最终资金", "收益", "最大回撤", "当前回撤", "交易数", "胜率", "总费用",
    ]);
    const finalBalance = metrics[0]!;
    expect(finalBalance).toMatchObject({ kind: "currency", left: 101000, right: 103000 });
    expect(page.comparisonMetricDelta(finalBalance)).toBe("+2,000.00 USD");

    const winRate = metrics.find((metric) => metric.label === "胜率")!;
    expect(winRate).toMatchObject({ kind: "percent", left: 0.6, right: 0.65 });
    expect(page.comparisonMetricDelta(winRate)).toBe("+5.00%");
  });

  it("flags configuration rows that differ between the two runs", async () => {
    const page = await mountForComparison();
    page.activateComparisonMode();
    await flushPromises();

    const rows = page.comparisonConfigRows.value;
    expect(rows.map((row) => row.label)).toEqual([
      "标的", "周期", "日期", "初始资金", "复权", "交易时段", "图表类型", "费用规则", "执行模型",
    ]);
    expect(page.comparisonConditionsMatch.value).toBe(true);
    expect(rows.find((row) => row.label === "复权")).toMatchObject({
      left: "前复权",
      right: "前复权",
      same: true,
    });
    expect(rows.find((row) => row.label === "交易时段")).toMatchObject({
      left: "常规时段",
      right: "常规时段",
      same: true,
    });
    expect(rows.find((row) => row.label === "费用规则")).toMatchObject({
      left: "券商 market_preset / 市场 market_preset",
      same: true,
    });
    expect(page.comparisonChartType(page.leftComparisonRun.value!)).toBe("标准K线");

    page.changeComparisonRun("left", "run-cmp-running");
    expect(page.leftComparisonRunId.value).toBe("run-cmp-running");
  });

  it("keeps the two comparison sides on distinct versions", async () => {
    const page = await mountForComparison();
    page.activateComparisonMode();
    await flushPromises();

    page.changeComparisonVersion("left", "2.0.0");
    expect(page.leftComparisonVersion.value).toBe("1.0.0");

    page.changeComparisonVersion("right", "1.0.0");
    expect(page.rightComparisonVersion.value).toBe("2.0.0");

    page.changeComparisonVersion("left", "2.0.0");
    page.changeComparisonRun("right", "run-cmp-old");
    expect(page.rightComparisonRunId.value).toBe("run-cmp-old");
  });

  it("reports mismatched quote currencies instead of a numeric delta", async () => {
    installApiMock({
      runs: [
        makeRun({
          id: "run-usd",
          status: "completed",
          definitionId: "def-1",
          definitionVersion: "1.0.0",
          createdAt: "2026-07-08T00:00:00Z",
          result: makeComparisonResult(),
        }),
        makeRun({
          id: "run-hkd",
          status: "completed",
          definitionId: "def-1",
          definitionVersion: "2.0.0",
          symbol: "HK.00700",
          createdAt: "2026-07-11T00:00:00Z",
          result: makeComparisonResult({ symbol: "HK.00700", quoteCurrency: "HKD" }),
        }),
      ],
    });
    const page = mountBacktestPage();
    await flushPromises();
    page.activateComparisonMode();
    await flushPromises();

    expect(page.resolveRunQuoteCurrency(page.leftComparisonRun.value!)).toBe("USD");
    expect(page.resolveRunQuoteCurrency(page.rightComparisonRun.value!)).toBe("HKD");
    const finalBalance = page.comparisonMetrics.value[0]!;
    expect(page.comparisonMetricDelta(finalBalance)).toBe("币种不同");

    const conditions = page.comparisonConfigRows.value.find((row) => row.label === "标的")!;
    expect(conditions.same).toBe(false);
    expect(page.comparisonConditionsMatch.value).toBe(false);
  });

  it("mirrors the comparison selection into the route query", async () => {
    const page = await mountForComparison();
    page.activateComparisonMode();
    await flushPromises();

    const lastReplace = mocks.routerReplace.mock.calls.at(-1)?.[0] as {
      path: string;
      query: Record<string, string>;
    };
    expect(lastReplace).toEqual({
      path: "/backtest",
      query: {
        mode: "compare",
        definitionId: "def-1",
        leftVersion: "1.0.0",
        rightVersion: "2.0.0",
        leftRunId: "run-cmp-old",
        rightRunId: "run-cmp-new",
      },
    });

    page.activateSingleReportMode();
    await flushPromises();
    expect(page.reportMode.value).toBe("single");
  });

  it("restores the comparison selection from the route query", async () => {
    mocks.route = reactive({
      path: "/backtest",
      query: {
        mode: "compare",
        definitionId: "def-1",
        leftVersion: "1.0.0",
        rightVersion: "2.0.0",
        leftRunId: "run-cmp-old",
        rightRunId: "run-cmp-new",
      },
    });
    installApiMock({ runs: COMPARISON_RUNS });
    const page = mountBacktestPage();
    await flushPromises();

    expect(page.reportMode.value).toBe("compare");
    expect(page.comparisonDefinitionId.value).toBe("def-1");
    expect(page.leftComparisonVersion.value).toBe("1.0.0");
    expect(page.rightComparisonVersion.value).toBe("2.0.0");
    expect(page.comparisonRunsReady.value).toBe(true);

    expect(page.firstQueryValue(["a", "b"])).toBe("a");
    expect(page.firstQueryValue(7)).toBe("");
    expect(page.reportModeFromQuery("compare")).toBe("compare");
    expect(page.reportModeFromQuery("other")).toBe("single");
  });
});
