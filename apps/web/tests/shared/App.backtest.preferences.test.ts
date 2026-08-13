// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import {
  emptyBrokerSettings,
  emptyMarketDataSubscriptions,
  emptyOnboardingState,
  emptyPluginCatalog,
  emptySystemStatus,
} from "@/types";

import {
  MockWebSocket,
  createResponse,
  flushRequests,
  mountApp,
} from "../helpers";
import BacktestPage from "../../src/pages/BacktestPage.vue";
import * as backtestPresentation from "../../src/components/backtest/backtestRunPresentation";
import { queryClient, queryKeys } from "@/composables/settings/serverState";

const backtestFormStorageKey = "jftrade.backtest.form.v1";

vi.mock("@/components/backtest/BacktestChart.vue", () => ({
  default: {
    props: ["chartType", "heikinAshiSeed"],
    template:
      '<div data-testid="backtest-chart" :data-chart-type="chartType" :data-ha-open="heikinAshiSeed?.open" />',
  },
}));

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
  window.localStorage.clear();
  window.sessionStorage.clear();
});

describe("Backtest page", () => {
  it("clears a persisted strategy selection when the rebuilt database is empty", async () => {
    window.localStorage.setItem(
      backtestFormStorageKey,
      JSON.stringify({ selectedDefinitionId: "stale-definition" }),
    );
    const fetchMock = installBacktestPageFetch({ runs: [], definitions: [] });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();

    const requestedURLs = fetchMock.mock.calls.map(([input]) => String(input));
    expect(requestedURLs.some((url) => url.includes("/strategy-definitions/stale-definition"))).toBe(false);
    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    expect(readSetupValue<string>(setup.selectedDefinitionId)).toBe("");
    expect(JSON.parse(window.localStorage.getItem(backtestFormStorageKey) ?? "{}")).toMatchObject({
      selectedDefinitionId: "",
    });

    wrapper.unmount();
  });

  it("refreshes definitions and replaces a persisted ID that is no longer present", async () => {
    window.localStorage.setItem(
      backtestFormStorageKey,
      JSON.stringify({ selectedDefinitionId: "stale-definition" }),
    );
    queryClient.setQueryData(queryKeys.strategyDefinitions(), [{
      id: "stale-definition",
      name: "Cached strategy",
      version: "v1",
    }]);
    const fetchMock = installBacktestPageFetch({
      runs: [],
      definitions: [{
        id: "current-definition",
        name: "Current strategy",
        version: "v2",
        symbol: "HK.00700",
      }],
    });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();

    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    expect(readSetupValue<string>(setup.selectedDefinitionId)).toBe("current-definition");
    expect(fetchMock.mock.calls.some(([input]) => String(input).endsWith("/api/v1/strategy-definitions"))).toBe(true);
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("/strategy-definitions/current-definition?"))).toBe(true);

    wrapper.unmount();
    queryClient.removeQueries({ queryKey: queryKeys.strategyDefinitions() });
  });

  it("clears a selected definition after a not-found warmup response without retrying", async () => {
    const definitionId = "deleted-definition";
    window.localStorage.setItem(
      backtestFormStorageKey,
      JSON.stringify({ selectedDefinitionId: definitionId }),
    );
    const fetchMock = installBacktestPageFetch({
      runs: [],
      definitions: [{
        id: definitionId,
        name: "Deleted strategy",
        version: "v1",
        symbol: "HK.00700",
      }],
      missingDefinitionIds: [definitionId],
    });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();

    const detailRequests = fetchMock.mock.calls.filter(([input]) =>
      String(input).includes(`/strategy-definitions/${definitionId}?`),
    );
    expect(detailRequests).toHaveLength(1);
    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    expect(readSetupValue<string>(setup.selectedDefinitionId)).toBe("");
    expect(JSON.parse(window.localStorage.getItem(backtestFormStorageKey) ?? "{}")).toMatchObject({
      selectedDefinitionId: "",
    });

    wrapper.unmount();
  });

  it("restores version comparison from the URL with completed runs and source snapshots", async () => {
    const baseline = buildDetailedBacktestRun();
    baseline.id = "run-baseline";
    baseline.createdAt = "2026-07-01T00:00:00.000Z";
    baseline.updatedAt = "2026-07-01T00:00:00.000Z";
    baseline.request.definitionVersion = "0.1.0";
    baseline.result.finalBalance = 101_000;
    baseline.result.pnl = 1_000;

    const candidate = buildDetailedBacktestRun();
    candidate.id = "run-candidate";
    candidate.createdAt = "2026-07-03T00:00:00.000Z";
    candidate.updatedAt = "2026-07-03T00:00:00.000Z";
    candidate.request.definitionVersion = "0.1.1";
    candidate.result.finalBalance = 102_500;
    candidate.result.pnl = 2_500;

    installBacktestPageFetch({
      runs: [baseline, candidate],
      definitions: [{
        id: "strategy-1",
        name: "Versioned EMA",
        version: "0.1.1",
        symbol: "US.AAPL",
      }],
      versionsByDefinitionId: {
        "strategy-1": [
          {
            version: "0.1.1",
            name: "Versioned EMA",
            savedAt: "2026-07-03T00:00:00.000Z",
            isCurrent: true,
            script: '//@version=6\nstrategy("Versioned EMA v0.1.1")',
          },
          {
            version: "0.1.0",
            name: "Versioned EMA",
            savedAt: "2026-07-01T00:00:00.000Z",
            script: '//@version=6\nstrategy("Versioned EMA v0.1.0")',
          },
        ],
      },
    });

    const { router, wrapper } = await mountApp(
      "/backtest?mode=compare&definitionId=strategy-1&leftVersion=0.1.0&rightVersion=0.1.1&leftRunId=run-baseline&rightRunId=run-candidate",
    );
    await flushRequests();
    await flushRequests();

    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    expect(readSetupValue<string>(setup.reportMode)).toBe("compare");
    expect(readSetupValue<string>(setup.comparisonDefinitionId)).toBe("strategy-1");
    expect(readSetupValue<string>(setup.leftComparisonVersion)).toBe("0.1.0");
    expect(readSetupValue<string>(setup.rightComparisonVersion)).toBe("0.1.1");
    expect(readSetupValue<string>(setup.leftComparisonRunId)).toBe("run-baseline");
    expect(readSetupValue<string>(setup.rightComparisonRunId)).toBe("run-candidate");
    expect(wrapper.text()).toContain("版本对比");
    expect(wrapper.text()).toContain("候选 − 基线");
    expect(wrapper.text()).toContain("回测配置");
    expect(wrapper.find('[data-testid="strategy-source-diff"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-source-diff-left-fallback"]').element.value).toContain("v0.1.0");
    expect(wrapper.get('[data-testid="strategy-source-diff-right-fallback"]').element.value).toContain("v0.1.1");
    expect(router.currentRoute.value.query).toMatchObject({
      mode: "compare",
      definitionId: "strategy-1",
      leftVersion: "0.1.0",
      rightVersion: "0.1.1",
      leftRunId: "run-baseline",
      rightRunId: "run-candidate",
    });

    wrapper.unmount();
  });

  it("loads both comparison result details when the run list only contains summaries", async () => {
    const baseline = buildDetailedBacktestRun();
    baseline.id = "run-summary-baseline";
    baseline.request.definitionVersion = "0.1.0";
    baseline.result.finalBalance = 101_000;

    const candidate = buildDetailedBacktestRun();
    candidate.id = "run-summary-candidate";
    candidate.createdAt = "2026-07-03T00:00:00.000Z";
    candidate.updatedAt = "2026-07-03T00:00:00.000Z";
    candidate.request.definitionVersion = "0.1.1";
    candidate.result.finalBalance = 102_500;

    const summaries = [baseline, candidate].map(({ result: _result, ...summary }) => summary);
    const fetchMock = installBacktestPageFetch({
      runs: [baseline, candidate],
      listRuns: summaries,
      definitions: [{
        id: "strategy-1",
        name: "Versioned EMA",
        version: "0.1.1",
        symbol: "US.AAPL",
      }],
      versionsByDefinitionId: {
        "strategy-1": [
          { version: "0.1.1", name: "Versioned EMA", isCurrent: true },
          { version: "0.1.0", name: "Versioned EMA" },
        ],
      },
    });

    const { wrapper } = await mountApp(
      "/backtest?mode=compare&definitionId=strategy-1&leftVersion=0.1.0&rightVersion=0.1.1&leftRunId=run-summary-baseline&rightRunId=run-summary-candidate",
    );
    for (let attempt = 0; attempt < 5; attempt += 1) {
      await flushRequests();
    }

    const requestedURLs = fetchMock.mock.calls.map(([input]) => String(input));
    expect(requestedURLs).toContain("/api/v1/backtests/run-summary-baseline");
    expect(requestedURLs).toContain("/api/v1/backtests/run-summary-candidate");
    expect(wrapper.text()).toContain("版本对比");

    wrapper.unmount();
  });

  it("handles comparison selector boundaries, metric deltas, and unavailable snapshots", async () => {
    const older = buildDetailedBacktestRun();
    older.id = "compare-older";
    older.request.definitionId = "compare-strategy";
    older.request.definitionVersion = "0.1.0";
    older.request.tradingCosts = {
      brokerFees: { mode: "market_preset", presetId: "broker-v1" },
      marketFees: { mode: "custom" },
    };
    older.result.tradingCosts = older.request.tradingCosts;
    older.result.finalBalance = 101_000;
    older.result.pnl = 1_000;
    older.result.chartType = "standard";
    older.result.executionModel = "model-a";

    const olderFallbackTimestamp = buildDetailedBacktestRun();
    olderFallbackTimestamp.id = "compare-older-fallback";
    olderFallbackTimestamp.request.definitionId = "compare-strategy";
    olderFallbackTimestamp.request.definitionVersion = "0.1.0";
    olderFallbackTimestamp.updatedAt = "invalid";
    olderFallbackTimestamp.createdAt = "2026-07-02T00:00:00.000Z";

    const newer = buildDetailedBacktestRun();
    newer.id = "compare-newer";
    newer.request.definitionId = "compare-strategy";
    newer.request.definitionVersion = "0.2.0";
    newer.request.chartType = "heikinashi";
    newer.result.quoteCurrency = "HKD";
    newer.result.finalBalance = 102_500;
    newer.result.pnl = 2_500;
    newer.result.executionModel = "model-b";

    const ignored = buildBacktestRun(9) as any;
    ignored.status = "running";
    ignored.request.definitionId = "compare-strategy";
    ignored.request.definitionVersion = "0.1.0";

    const fetchMock = installBacktestPageFetch({
      runs: [older, olderFallbackTimestamp, newer, ignored],
      definitions: [
        { id: "compare-strategy", name: "Compare", version: "0.2.0", symbol: "US.AAPL" },
      ],
      versionsByDefinitionId: {
        "compare-strategy": [
          { version: "0.2.0", name: "Compare", savedAt: "2026-07-03T00:00:00Z", isCurrent: true, script: "new" },
          { version: "0.1.0", name: "Compare", savedAt: "2026-07-01T00:00:00Z", script: "old" },
          { version: "0.0.1", name: "Compare", savedAt: "invalid", script: "oldest" },
        ],
      },
    });
    const baseFetchImplementation = fetchMock.getMockImplementation();

    const { router, wrapper } = await mountApp("/backtest");
    await flushRequests();
    await flushRequests();
    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    const call = <T>(name: string, ...args: unknown[]) =>
      (setup[name] as (...values: unknown[]) => T)(...args);

    expect(call("comparisonRunTimestamp", olderFallbackTimestamp)).toBe(Date.parse(olderFallbackTimestamp.createdAt));
    expect(call("comparisonRunTimestamp", { ...olderFallbackTimestamp, createdAt: "invalid" })).toBe(0);
    expect(call("completedRunsForComparisonVersion", " ")).toEqual([]);
    expect(call("versionOptionTitle", { version: "0.2.0", isCurrent: true })).toBe("v0.2.0（当前）");
    expect(call("versionOptionTitle", { version: "0.1.0", isCurrent: false })).toBe("v0.1.0");
    expect(call<string>("comparisonRunOptionTitle", older)).toContain("compare-older");

    writeSetupValue(setup, "comparisonDefinitionId", "compare-strategy");
    await call<Promise<void>>("loadComparisonVersions", "compare-strategy");
    await flushRequests();
    expect(readSetupValue<string>(setup.leftComparisonVersion)).toBe("0.1.0");
    expect(readSetupValue<string>(setup.rightComparisonVersion)).toBe("0.2.0");
    expect(readSetupValue<unknown[]>(setup.leftComparisonRuns)).toHaveLength(2);
    expect(readSetupValue<string>(setup.leftComparisonRunId)).toBe("compare-older-fallback");
    expect(readSetupValue<string>(setup.rightComparisonRunId)).toBe("compare-newer");

    expect(call("formatComparisonCurrency", undefined, "USD")).toBe("--");
    expect(call<string>("formatComparisonCurrency", 1234.5, "")).not.toContain("USD");
    expect(call<string>("formatComparisonCurrency", 1234.5, "USD")).toContain("USD");
    expect(call("formatComparisonMetric", Number.NaN, "number")).toBe("--");
    expect(call("formatComparisonMetric", 0.125, "percent")).toBe("12.50%");
    expect(call<string>("formatComparisonMetric", 12.5, "currency", "USD")).toContain("USD");
    expect(call<string>("formatComparisonMetric", 12.5, "number")).toContain("12.5");
    expect(call("comparisonMetricDelta", { kind: "number", left: undefined, right: 1 })).toBe("--");
    expect(call("comparisonMetricDelta", { kind: "currency", left: 1, right: 2 })).toBe("币种不同");
    expect(call<string>("comparisonMetricDelta", { kind: "percent", left: 0.2, right: 0.1 })).toContain("-10.00%");
    expect(call("compareConfigValue", "same", "same")).toEqual({ label: "", left: "same", right: "same", same: true });
    expect(call("compareConfigValue", "left", "right")).toMatchObject({ same: false });
    expect(call<string>("comparisonFeeConfig", older)).toContain("market_preset:broker-v1");
    expect(call<string>("comparisonFeeConfig", { ...older, result: undefined, request: { ...older.request, tradingCosts: undefined } })).toContain("market_preset");
    expect(call("comparisonChartType", newer)).toBe("Heikin Ashi");
    expect(call("comparisonChartType", older)).toBe("标准K线");
    expect(readSetupValue<unknown[]>(setup.comparisonMetrics)).toHaveLength(7);
    expect(readSetupValue<unknown[]>(setup.comparisonConfigRows)).toHaveLength(9);
    expect(readSetupValue<boolean>(setup.comparisonConditionsMatch)).toBe(false);

    writeSetupValue(setup, "leftComparisonRunId", "");
    writeSetupValue(setup, "rightComparisonRunId", "");
    await nextTick();
    expect(readSetupValue<unknown[]>(setup.comparisonConfigRows)).toEqual([]);
    expect(call<string>("comparisonMetricDelta", { kind: "number", left: 1, right: 2 })).toContain("+1");
    expect(call("comparisonChartType", { ...newer, result: undefined })).toBe("Heikin Ashi");
    expect(call<string>("comparisonFeeConfig", {
      ...older,
      result: undefined,
      request: {
        ...older.request,
        tradingCosts: { brokerFees: { mode: "custom" }, marketFees: null },
      },
    })).toBe("券商 custom / 市场 market_preset");

    const versionOptions = readSetupValue<unknown[]>(setup.comparisonVersions);
    writeSetupValue(setup, "leftComparisonVersion", "");
    writeSetupValue(setup, "rightComparisonVersion", "0.2.0");
    call("applyComparisonVersionDefaults");
    expect(readSetupValue<string>(setup.leftComparisonVersion)).toBe("0.1.0");
    writeSetupValue(setup, "leftComparisonVersion", "0.1.0");
    writeSetupValue(setup, "rightComparisonVersion", "");
    call("applyComparisonVersionDefaults");
    expect(readSetupValue<string>(setup.rightComparisonVersion)).toBe("0.2.0");
    writeSetupValue(setup, "leftComparisonVersion", "0.1.0");
    writeSetupValue(setup, "rightComparisonVersion", "0.1.0");
    call("applyComparisonVersionDefaults");
    expect(readSetupValue<string>(setup.rightComparisonVersion)).toBe("0.2.0");
    writeSetupValue(setup, "comparisonVersions", [versionOptions[0]]);
    writeSetupValue(setup, "leftComparisonVersion", "missing");
    writeSetupValue(setup, "rightComparisonVersion", "missing");
    call("applyComparisonVersionDefaults");
    expect(readSetupValue<string>(setup.leftComparisonVersion)).toBe("0.2.0");
    writeSetupValue(setup, "comparisonVersions", versionOptions);

    call("changeComparisonRun", "left", "compare-older");
    call("changeComparisonRun", "right", "compare-newer");
    expect(readSetupValue<string>(setup.leftComparisonRunId)).toBe("compare-older");
    expect(readSetupValue<string>(setup.rightComparisonRunId)).toBe("compare-newer");
    call("changeComparisonRun", "right", 42);
    expect(readSetupValue<string>(setup.rightComparisonRunId)).toBe("");

    writeSetupValue(setup, "leftComparisonVersion", "0.1.0");
    writeSetupValue(setup, "rightComparisonVersion", "0.2.0");
    call("changeComparisonVersion", "left", "0.2.0");
    expect(readSetupValue<string>(setup.leftComparisonVersion)).toBe("0.1.0");
    call("changeComparisonVersion", "left", "0.0.1");
    call("changeComparisonVersion", "right", "0.2.0");
    call("changeComparisonVersion", "right", null);
    await flushRequests();
    expect(readSetupValue<string>(setup.leftComparisonVersion)).toBe("0.0.1");
    expect(readSetupValue<string>(setup.rightComparisonVersion)).toBe("");

    await call<Promise<void>>("loadComparisonSnapshot", "left", "");
    expect(readSetupValue(setup.leftComparisonSnapshot)).toBeNull();
    call("clearComparisonSnapshots");
    expect(readSetupValue(setup.rightComparisonSnapshot)).toBeNull();

    writeSetupValue(setup, "reportMode", "single");
    writeSetupValue(setup, "comparisonDefinitionId", "");
    writeSetupValue(setup, "selectedDefinitionId", "compare-strategy");
    call("activateComparisonMode");
    await flushRequests();
    expect(readSetupValue<string>(setup.reportMode)).toBe("compare");
    call("activateSingleReportMode");
    expect(readSetupValue<string>(setup.reportMode)).toBe("single");
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("report");

    writeSetupValue(setup, "resultsStatusFilter", "failed");
    await nextTick();
    call("activateSingleReportMode");
    writeSetupValue(setup, "backtestMobileSection", "setup");
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("setup");
    writeSetupValue(setup, "resultsStatusFilter", "all");
    writeSetupValue(setup, "definitions", []);
    writeSetupValue(setup, "comparisonDefinitionId", "");
    writeSetupValue(setup, "selectedDefinitionId", "");
    call("activateComparisonMode");
    expect(readSetupValue<string>(setup.comparisonDefinitionId)).toBe("");

    writeSetupValue(setup, "definitions", [{ id: "compare-strategy", name: "Compare", version: "0.2.0" }]);

    call("changeComparisonDefinition", "compare-strategy");
    call("changeComparisonDefinition", 42);
    await flushRequests();
    expect(readSetupValue<string>(setup.comparisonDefinitionId)).toBe("");
    await call<Promise<void>>("loadComparisonVersions", "");
    expect(readSetupValue<unknown[]>(setup.comparisonVersions)).toEqual([]);

    writeSetupValue(setup, "comparisonDefinitionId", "compare-strategy");
    writeSetupValue(setup, "comparisonVersions", [
      ...versionOptions,
      { definitionId: "compare-strategy", version: "broken-version", name: "Broken", savedAt: "", isCurrent: false },
      { definitionId: "compare-strategy", version: "broken-right", name: "Broken right", savedAt: "", isCurrent: false },
    ]);
    writeSetupValue(setup, "leftComparisonVersion", "broken-version");
    fetchMock.mockRejectedValue(new Error("snapshot offline"));
    await call<Promise<void>>("loadComparisonSnapshot", "left", "broken-version");
    expect(readSetupValue<Record<string, string>>(setup.comparisonSnapshotErrors).left).toContain("snapshot offline");

    fetchMock.mockImplementation(baseFetchImplementation!);
    writeSetupValue(setup, "rightComparisonVersion", "broken-right");
    fetchMock.mockRejectedValue("right snapshot offline");
    await call<Promise<void>>("loadComparisonSnapshot", "right", "broken-right");
    expect(readSetupValue<Record<string, string>>(setup.comparisonSnapshotErrors).right).toContain("right snapshot offline");

    fetchMock.mockImplementation(baseFetchImplementation!);
    writeSetupValue(setup, "comparisonDefinitionId", "broken-definition");
    fetchMock.mockRejectedValue(new Error("history offline"));
    await call<Promise<void>>("loadComparisonVersions", "broken-definition");
    expect(readSetupValue<string>(setup.comparisonVersionsError)).toContain("history offline");
    expect(readSetupValue<boolean>(setup.isLoadingComparisonVersions)).toBe(false);

    fetchMock.mockImplementation(baseFetchImplementation!);
    writeSetupValue(setup, "reportMode", "compare");
    writeSetupValue(setup, "comparisonDefinitionId", "compare-strategy");
    writeSetupValue(setup, "leftComparisonVersion", "0.1.0");
    writeSetupValue(setup, "rightComparisonVersion", "0.2.0");
    writeSetupValue(setup, "leftComparisonRunId", "compare-older");
    writeSetupValue(setup, "rightComparisonRunId", "compare-newer");
    call("syncComparisonRoute");
    await flushRequests();
    await flushRequests();
    expect(router.currentRoute.value.query).toMatchObject({ mode: "compare", leftVersion: "0.1.0", rightVersion: "0.2.0" });
    expect(call("comparisonQueryMatchesRoute")).toBe(true);
    call("syncComparisonRoute");

    wrapper.unmount();
  });

  it("falls back stored expired markets to the backend default market", async () => {
    window.localStorage.setItem(
      backtestFormStorageKey,
      JSON.stringify({
        selectedDefinitionId: "",
        selectedMarket: "SG",
        codeInput: "D05",
        interval: "5m",
        startDate: "2026-01-01",
        endDate: "2026-01-02",
        initialBalance: 1000000,
        rehabType: "forward",
        useExtendedHours: false,
      }),
    );

    installBacktestPageFetch({ runs: [] });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();

    const stored = JSON.parse(
      window.localStorage.getItem(backtestFormStorageKey) ?? "{}",
    ) as { selectedMarket?: string };
    expect(stored.selectedMarket).toBe("HK");
    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(true);
    expect(wrapper.text()).toContain("策略与标的");

    wrapper.unmount();
  });

  it("migrates legacy SH/SZ bare-code preferences to CN with a qualified instrument", async () => {
    window.localStorage.setItem(
      backtestFormStorageKey,
      JSON.stringify({
        selectedDefinitionId: "",
        selectedMarket: "SH",
        codeInput: "600519",
        interval: "5m",
        startDate: "2026-01-01",
        endDate: "2026-01-02",
        initialBalance: 1_000_000,
        instrumentType: "stock",
        rehabType: "forward",
        useExtendedHours: false,
      }),
    );

    installBacktestPageFetch({ runs: [] });
    const { wrapper } = await mountApp("/backtest");
    await flushRequests();

    const stored = JSON.parse(
      window.localStorage.getItem(backtestFormStorageKey) ?? "{}",
    ) as { selectedMarket?: string; codeInput?: string };
    expect(stored).toMatchObject({
      selectedMarket: "CN",
      codeInput: "SH.600519",
    });

    wrapper.unmount();
  });
});

function installBacktestPageFetch(options: {
  runs: unknown[];
  listRuns?: unknown[];
  definitions?: unknown[];
  missingDefinitionIds?: string[];
  versionsByDefinitionId?: Record<string, Array<{
    version: string;
    name?: string;
    savedAt?: string;
    isCurrent?: boolean;
    script?: string;
  }>>;
}) {
  const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse(emptySystemStatus);
      }
      if (url.includes("/api/v1/settings/onboarding")) {
        return createResponse(emptyOnboardingState);
      }
      if (url.includes("/api/v1/settings/backtest-market-data-provider")) {
        return createResponse({
          activeProvider: "futu",
          availableProviders: [{
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
          }],
        });
      }
      if (url.includes("/api/v1/settings/brokers")) {
        return createResponse(emptyBrokerSettings);
      }
      if (url.includes("/api/v1/plugins")) {
        return createResponse(emptyPluginCatalog);
      }
      if (url.includes("/api/v1/market-data/subscriptions")) {
        return createResponse(emptyMarketDataSubscriptions);
      }
      if (url.includes("/api/v1/market-data/instruments?")) {
        return createResponse({
          entries: [
            {
              market: "US",
              code: "AAPL",
              symbol: "AAPL",
              instrumentId: "US.AAPL",
              name: "Apple",
              securityType: "Trust",
              brokerMappings: [],
            },
            {
              market: "SH",
              code: "600519",
              symbol: "600519",
              instrumentId: "SH.600519",
              name: "贵州茅台",
              securityType: "Eqty",
              brokerMappings: [],
            },
            {
              market: "SZ",
              code: "600519",
              symbol: "600519",
              instrumentId: "SZ.600519",
              name: "深市同码标的",
              securityType: "Eqty",
              brokerMappings: [],
            },
          ],
        });
      }
      if (url.includes("/api/v1/market-data/instruments/normalize")) {
        return createResponse({
          market: "US",
          prefix: "US",
          code: "AAPL",
          instrumentId: "US.AAPL",
        });
      }
      if (url.includes("/api/v1/market-data/markets")) {
        return createResponse({
          defaultMarket: "HK",
          updatedAt: "2026-06-12T00:00:00.000Z",
          markets: [
            {
              code: "HK",
              resolvedMarket: "HK",
              preferredPrefix: "HK",
              displayName: "Hong Kong",
              quoteCurrency: "HKD",
              supportsExtendedHours: false,
              requiresExchangePrefix: false,
              aliases: ["HKEX"],
              regularSessions: [],
              precision: { price: 3, quote: 3 },
              tickSize: 0.001,
            },
            {
              code: "US",
              resolvedMarket: "US",
              preferredPrefix: "US",
              displayName: "US",
              quoteCurrency: "USD",
              supportsExtendedHours: true,
              requiresExchangePrefix: false,
              aliases: ["NYSE", "NASDAQ"],
              regularSessions: [],
              precision: { price: 2, quote: 2 },
              tickSize: 0.01,
            },
            {
              code: "CN",
              resolvedMarket: "CN",
              preferredPrefix: "",
              displayName: "沪深",
              quoteCurrency: "CNY",
              supportsExtendedHours: false,
              requiresExchangePrefix: true,
              aliases: ["SH", "SZ", "CNSH", "CNSZ"],
              regularSessions: [],
              precision: { price: 2, quote: 2 },
              tickSize: 0.01,
            },
          ],
        });
      }
      const definitionVersionsMatch = url.match(/\/api\/v1\/strategy-definitions\/([^/?#]+)\/versions$/);
      const definitionVersionMatch = url.match(/\/api\/v1\/strategy-definitions\/([^/?#]+)\/versions\/([^/?#]+)$/);
      if (definitionVersionsMatch) {
        const definitionId = decodeURIComponent(definitionVersionsMatch[1] ?? "");
        const definition = options.definitions?.find((item) =>
          typeof item === "object" && item !== null && "id" in item &&
          (item as { id?: unknown }).id === definitionId,
        ) as Record<string, unknown> | undefined;
        return createResponse((options.versionsByDefinitionId?.[definitionId] ?? []).map((version) => ({
          definitionId,
          version: version.version,
          name: version.name ?? definition?.name ?? "",
          savedAt: version.savedAt ?? "2026-07-01T00:00:00.000Z",
          isCurrent: version.isCurrent ?? false,
        })));
      }
      if (definitionVersionMatch) {
        const definitionId = decodeURIComponent(definitionVersionMatch[1] ?? "");
        const versionValue = decodeURIComponent(definitionVersionMatch[2] ?? "");
        const definition = options.definitions?.find((item) =>
          typeof item === "object" && item !== null && "id" in item &&
          (item as { id?: unknown }).id === definitionId,
        ) as Record<string, unknown> | undefined;
        const version = options.versionsByDefinitionId?.[definitionId]?.find((item) => item.version === versionValue);
        return createResponse({
          ...(definition ?? {}),
          definitionId,
          version: versionValue,
          name: version?.name ?? definition?.name ?? "",
          savedAt: version?.savedAt ?? "2026-07-01T00:00:00.000Z",
          isCurrent: version?.isCurrent ?? false,
          script: version?.script ?? "",
        });
      }
      if (url.includes("/api/v1/strategy-definitions/")) {
        const definitionId = decodeURIComponent(
          url.match(/\/api\/v1\/strategy-definitions\/([^/?#]+)/)?.[1] ?? "",
        );
        if (options.missingDefinitionIds?.includes(definitionId)) {
          return {
            ok: false,
            status: 404,
            statusText: "Not Found",
            json: async () => ({
              ok: false,
              error: { code: "NOT_FOUND", message: "strategy definition not found" },
            }),
          } as Response;
        }
        const definition = options.definitions?.[0] ?? {};
        return createResponse(definition);
      }
      if (url.includes("/api/v1/strategy-definitions")) {
        return createResponse(options.definitions ?? []);
      }
      const backtestDetailMatch = url.match(/\/api\/v1\/backtests\/([^/?#]+)/);
      if (backtestDetailMatch) {
        const runId = decodeURIComponent(backtestDetailMatch[1] ?? "");
        return createResponse(
          options.runs.find((run) => {
            return (
              typeof run === "object" &&
              run !== null &&
              "id" in run &&
              (run as { id?: unknown }).id === runId
            );
          }) ?? options.runs[0] ?? {},
        );
      }
      if (url.includes("/api/v1/backtests")) {
        return createResponse({ runs: options.listRuns ?? options.runs });
      }

      throw new Error(`Unexpected request: ${url}`);
    });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function buildBacktestRun(index: number): unknown {
  const id = `run-${String(index).padStart(3, "0")}`;
  return {
    id,
    status: "completed",
    createdAt: `2026-06-${String(31 - index).padStart(2, "0")}T00:00:00.000Z`,
    updatedAt: `2026-06-${String(31 - index).padStart(2, "0")}T00:00:00.000Z`,
    request: {
      definitionId: "strategy-1",
      definitionVersion: "v1",
      market: "HK",
      code: "00700",
      symbol: `HK.${String(index).padStart(5, "0")}`,
      interval: "1d",
      startDate: "2026-01-01",
      endDate: "2026-01-31",
      initialBalance: 100000,
      rehabType: "forward",
    },
  };
}

function buildDetailedBacktestRun(): any {
  const run = buildBacktestRun(1) as Record<string, any>;
  run.status = "completed";
  run.request = {
    ...run.request,
    symbol: "US.AAPL",
    market: "US",
    code: "AAPL",
    interval: "5m",
    chartType: "standard",
    rehabType: "none",
    useExtendedHours: true,
    definitionVersion: "v1",
  };
  run.result = {
    chartType: "heikinashi",
    heikinAshiSeed: { open: 99.5, close: 100.25 },
    quoteCurrency: "USD",
    pnl: 1250.5,
    pnlPct: 0.0125,
    maxDrawdown: 0.08,
    finalBalance: 101_250.5,
    tradeStatsVersion: 2,
    totalFills: 40,
    totalTrades: 20,
    winRate: 0.55,
    currentDrawdown: 0.02,
    totalBrokerFees: 12.5,
    totalMarketFees: 8.5,
    totalFees: 21,
    tradingCosts: {},
    trades: [],
    pnlCurve: [{ time: "2026-06-01T00:00:00.000Z", equity: 100_000 }],
    drawdownCurve: [],
    error: "",
    runtimeErrorTotal: 150,
    runtimeErrorsTruncated: true,
    runtimeErrors: Array.from({ length: 125 }, (_, index) => (index === 0 ? "timeout" : `error-${index}`)),
    runtimeErrorCounts: { timeout: 50 },
    warningTotal: 130,
    warningsTruncated: true,
    ignoredOrders: 2,
    warnings: Array.from({ length: 125 }, (_, index) => `warning-${index}`),
    logs: Array.from({ length: 205 }, (_, index) => `log-${index}`),
    orderBook: Array.from({ length: 205 }, (_, index) => ({
      id: `order-${index}`,
      side: index % 2 === 0 ? "BUY" : "SELL",
      status: "FILLED",
      orderType: "LIMIT",
      price: 100 + index,
      quantity: 10,
      warmup: index === 0,
    })),
    candles: [
      { time: "2026-06-02T00:00:00.000Z", open: 101, high: 102, low: 100, close: 101, volume: 10 },
      { time: "invalid", open: 0, high: 0, low: 0, close: 0, volume: 0 },
      { time: "2026-06-01T00:00:00.000Z", open: 100, high: 101, low: 99, close: 100, volume: 10 },
    ],
  };
  return run;
}

function readSetupValue<T>(value: unknown): T {
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T }).value;
  }
  return value as T;
}

function writeSetupValue(setup: Record<string, unknown>, key: string, value: unknown): void {
  const current = setup[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: unknown }).value = value;
    return;
  }
  setup[key] = value;
}
