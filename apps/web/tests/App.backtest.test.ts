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
} from "./helpers";
import BacktestPage from "../src/pages/BacktestPage.vue";
import * as backtestPresentation from "../src/components/backtest/backtestRunPresentation";
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

  it("uses one all-market search and derives the backtest fee type from the selected result", async () => {
    installBacktestPageFetch({ runs: [] });
    const { wrapper } = await mountApp("/backtest");
    await flushRequests();

    const search = wrapper.get('[data-testid="backtest-instrument-search"]');
    const input = search.get('[data-testid="backtest-instrument-code"]');
    expect(search.find("select").exists()).toBe(false);
    expect(search.find(".v-combobox").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("标的类型");

    await input.setValue("Apple");
    await search.get('[data-testid="backtest-instrument-submit"]').trigger("click");
    await flushRequests();
    const usOption = Array.from(
      document.body.querySelectorAll<HTMLElement>(".instrument-search-box__option"),
    ).find((option) => option.textContent?.includes("Apple"));
    expect(usOption).toBeDefined();
    usOption!.click();
    await nextTick();

    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    expect(readSetupValue<string>(setup.instrumentType)).toBe("etf");
    expect(readSetupValue<string>(setup.codeInput)).toBe("US.AAPL");

    await input.setValue("贵州茅台");
    await search.get('[data-testid="backtest-instrument-submit"]').trigger("click");
    await flushRequests();
    const shOption = Array.from(
      document.body.querySelectorAll<HTMLElement>(".instrument-search-box__option"),
    ).find((option) => option.textContent?.includes("600519"));
    expect(shOption).toBeDefined();
    shOption!.click();
    await nextTick();
    expect(readSetupValue<string>(setup.instrumentType)).toBe("stock");
    expect(readSetupValue<string>(setup.codeInput)).toBe("SH.600519");

    const stored = JSON.parse(
      window.localStorage.getItem(backtestFormStorageKey) ?? "{}",
    ) as { instrumentType?: string; codeInput?: string };
    expect(stored).toMatchObject({
      instrumentType: "stock",
      codeInput: "SH.600519",
    });

    wrapper.unmount();
  });

  it("keeps many backtest results bounded to the active page", async () => {
    installBacktestPageFetch({
      runs: Array.from({ length: 30 }, (_, index) => buildBacktestRun(index + 1)),
    });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();

    expect(wrapper.text()).toContain("第 1-5 条，共 30 条");
    expect(wrapper.text()).toContain("run-001");
    expect(wrapper.text()).toContain("run-005");
    expect(wrapper.text()).not.toContain("run-006");
    expect([...new Set(wrapper.text().match(/run-\d{3}/g))]).toHaveLength(5);

    wrapper.unmount();
  });

  it("falls back safely when persisted form preferences are malformed", async () => {
    window.localStorage.setItem(backtestFormStorageKey, "{broken-json");
    installBacktestPageFetch({ runs: [] });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();

    const stored = JSON.parse(window.localStorage.getItem(backtestFormStorageKey) ?? "{}") as {
      selectedMarket?: string;
      codeInput?: string;
      interval?: string;
      initialBalance?: number;
    };
    expect(stored).toMatchObject({
      selectedMarket: "HK",
      codeInput: "00700",
      interval: "5m",
      initialBalance: 1_000_000,
    });

    wrapper.unmount();
  });

  it("normalizes persisted chart type and forces Tick backtests to standard candles", async () => {
    window.localStorage.setItem(
      backtestFormStorageKey,
      JSON.stringify({
        selectedDefinitionId: "",
        selectedMarket: "US",
        codeInput: "AAPL",
        interval: "5m",
        chartType: " HEIKINASHI ",
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
    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    expect(readSetupValue<string>(setup.chartType)).toBe("heikinashi");

    writeSetupValue(setup, "interval", "tick");
    await nextTick();
    await nextTick();

    expect(readSetupValue<string>(setup.chartType)).toBe("standard");
    expect(
      page.findAll("select")[2]?.element.hasAttribute("disabled"),
    ).toBe(true);
    expect(
      JSON.parse(window.localStorage.getItem(backtestFormStorageKey) ?? "{}"),
    ).toMatchObject({ interval: "tick", chartType: "standard" });
    wrapper.unmount();
  });

  it("uses the request chart type when an older result has no chart type", async () => {
    const fallbackRun = buildDetailedBacktestRun();
    fallbackRun.request.chartType = "standard";
    delete fallbackRun.result.chartType;
    installBacktestPageFetch({ runs: [fallbackRun] });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();
    await flushRequests();
    const page = wrapper.getComponent(BacktestPage);

    expect(
      page.get('[data-testid="backtest-chart"]').attributes("data-chart-type"),
    ).toBe("standard");
    wrapper.unmount();
  });

  it("shows the chart empty state when a completed result has no equity samples", async () => {
    const noChartRun = buildDetailedBacktestRun();
    noChartRun.result.pnlCurve = [];
    installBacktestPageFetch({ runs: [noChartRun] });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();
    await flushRequests();

    expect(wrapper.text()).toContain("暂无权益曲线数据。");
    expect(wrapper.find('[data-testid="backtest-chart"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("formats and bounds a detailed business result across fees, orders, errors and logs", async () => {
    const richRun = buildDetailedBacktestRun();
    installBacktestPageFetch({
      runs: [richRun, buildBacktestRun(2)],
      definitions: [
        {
          id: "strategy-1",
          name: "EMA Reversal",
          version: "v2",
          symbol: "HK.00700",
          interval: "5m",
          derivedWarmupBars: 120,
          derivedWarmupInterval: "15m",
        },
		{},
      ],
    });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();
    await flushRequests();
    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;
    const call = <T>(name: string, ...args: unknown[]) =>
      (setup[name] as (...values: unknown[]) => T)(...args);

    expect(wrapper.text()).toContain("历史回测");
    expect(wrapper.text()).toContain("新建回测");
    expect(wrapper.text()).toContain("回测工作台");
    expect(wrapper.text()).toContain("图表");
    expect(wrapper.text()).toContain("订单");
    expect(wrapper.text()).toContain("属性");
    expect(wrapper.text()).toContain("最终资金");
    expect(wrapper.text()).not.toContain("绩效摘要");
    expect(wrapper.text()).not.toContain("订单列表");
    expect(wrapper.text()).not.toContain("属性与日志");
    expect(wrapper.text()).not.toContain("历史记录");
    expect(wrapper.text()).not.toContain("提交步骤");
    expect(wrapper.text()).not.toContain("回测/实盘一致性边界");
    expect(wrapper.text()).not.toContain("回测/实盘一致性");
    expect(page.get(".bt-report-window").classes()).toEqual(
      expect.arrayContaining(["min-h-0", "flex-1", "overflow-hidden"]),
    );
    expect(page.get(".bt-report-chart-tab").classes()).toEqual(
      expect.arrayContaining(["h-full", "min-h-0", "flex-col"]),
    );
    expect(page.get(".bt-report-workspace").exists()).toBe(true);
    expect(page.get(".bt-report-window-item--chart").classes()).toContain("bt-report-window-item");
    expect(page.findAll(".bt-report-stat")).toHaveLength(9);
    expect(page.findAll('[data-testid^="backtest-kpi-"]')).toHaveLength(9);
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(false);
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(true);
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(["history"]);
    expect(readSetupValue<string>(setup.activeReportTab)).toBe("chart");
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("setup");
    expect(wrapper.get('[data-testid="backtest-mobile-section-setup"]').classes()).toContain("is-active");
    await wrapper.get('[data-testid="backtest-mobile-section-report"]').trigger("click");
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("report");
    await wrapper.get('[data-testid="backtest-mobile-section-setup"]').trigger("click");
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("setup");
    const firstHistoryRun = page.get(".bt-history-run");
    await firstHistoryRun.trigger("click");
    await firstHistoryRun.trigger("keydown", { key: "Enter" });
    await firstHistoryRun.trigger("keydown", { key: " " });
    call("selectFocusedRun", "run-002");
    await nextTick();
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("report");
    expect(readSetupValue(setup.focusedRun)).toMatchObject({ id: "run-002" });
    call("selectFocusedRun", richRun.id);
    await nextTick();
    expect(readSetupValue<[number, number]>(setup.backtestPaneSizes)).toEqual([30, 70]);
    writeSetupValue(setup, "activeReportTab", "chart");
    await nextTick();
    expect(
      page.get('[data-testid="backtest-chart"]').attributes("data-chart-type"),
    ).toBe(
      "heikinashi",
    );
    expect(
      page.get('[data-testid="backtest-chart"]').attributes("data-ha-open"),
    ).toBe("99.5");
    call("handleBacktestPaneResized", { panes: [{ size: 34 }, { size: 66 }] });
    expect(readSetupValue<[number, number]>(setup.backtestPaneSizes)).toEqual([34, 66]);
    call("handleBacktestPaneResized", { panes: [{ size: -1 }, { size: 101 }] });
    expect(readSetupValue<[number, number]>(setup.backtestPaneSizes)).toEqual([34, 66]);
    expect(window.localStorage.getItem("jftrade.backtest.layout.v1")).toBeNull();

    writeSetupValue(setup, "activeReportTab", "orders");
    await nextTick();
    expect(wrapper.text()).toContain("最终资金");
    expect(wrapper.text()).toContain("101,250.50");
    expect(wrapper.get(".bt-order-table-scroll").exists()).toBe(true);
    expect(wrapper.get(".bt-order-table").exists()).toBe(true);
    expect(wrapper.get(".bt-warmup-label").text()).toBe("预热");
    writeSetupValue(setup, "activeReportTab", "properties");
    await nextTick();
    expect(wrapper.text()).toContain("最终资金");

    call("toggleNewBacktestForm");
    await nextTick();
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(true);
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(
      expect.arrayContaining(["setup", "history"]),
    );
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("setup");
    expect(wrapper.text()).toContain("策略与标的");
    expect(wrapper.text()).toContain("数据范围");
    expect(wrapper.text()).toContain("资金与成本");
    expect(wrapper.text()).toContain("运行");
    expect(wrapper.text()).toContain("同步K线");
    expect(wrapper.text()).toContain("开始回测");
    const syncButton = page.findAll("button").find((button) => button.text().includes("同步K线"));
    const runButton = page.findAll("button").find((button) => button.text().includes("开始回测"));
    expect(syncButton).toBeDefined();
    expect(runButton).toBeDefined();
    await runButton!.trigger("click");
    await flushRequests();

    expect(call("formatBacktestRehabType", "none")).toBe("不复权");
    expect(call("formatBacktestRehabType", "backward")).toBe("后复权");
    expect(call("formatBacktestRehabType", "forward")).toBe("前复权");
    expect(call("resolveRunQuoteCurrency", richRun)).toBe("USD");
    expect(call("resolveRunQuoteCurrency", buildBacktestRun(2))).toBe("HKD");
    expect(call("resolveRunSessionMode", richRun)).toBe("含扩展时段");
    expect(call("resolveRunSessionMode", buildBacktestRun(2))).toBe("常规时段");
    expect(backtestPresentation.resolveBacktestPriceBasisNote(richRun)).toContain("已闭合历史 K 线");
    expect(backtestPresentation.resolveBacktestPriceBasisNote({
      request: { rehabType: "forward", interval: "1d" },
    })).toContain("前复权1d");
    expect(call("resolveStrategyName", "strategy-1")).toBe("EMA Reversal");
    expect(call("resolveStrategyName", "missing")).toBe("missing");
    expect(call("resolveStrategyName", undefined)).toBe("未命名策略");
    expect(call("resolveStrategyDefinition", "strategy-1")).toMatchObject({ name: "EMA Reversal" });
    expect(call("resolveStrategyDefinition", undefined)).toBeNull();
    expect(call("formatStrategyVersion", "1.2.0")).toBe("v1.2.0");
    expect(call("formatStrategyVersion", " ")).toBe("版本未知");
    expect(call("resolveBacktestStrategyVersionNotice", richRun)).toContain("旧版本策略回测结果");
    expect(call("resolveBacktestStrategyVersionNotice", {
      request: { definitionId: "deleted", definitionVersion: "0.9.0" },
    })).toContain("当前策略定义已不存在");
    expect(call("resolveBacktestStrategyVersionNotice", {
      request: { definitionId: "strategy-1", definitionVersion: "" },
    })).toBe("");
    expect(call("resolveBacktestStrategyVersionNotice", {
      request: { definitionId: "strategy-1", definitionVersion: "v2" },
    })).toBe("");

    expect(readSetupValue(setup.selectedDefinition)).toMatchObject({ id: "strategy-1" });
    call("handleResolvedBacktestInstrument", {
      market: "SH",
      resolvedMarket: "CN",
      instrumentId: "SH.600519",
      code: "600519",
      symbol: "600519",
      name: "贵州茅台",
      securityType: "STOCK",
      lotSize: 100,
      source: "test-static",
      isWatched: false,
      selectable: true,
      unavailableReason: null,
    });
    expect(readSetupValue<string>(setup.selectedMarket)).toBe("CN");
    expect(readSetupValue<string>(setup.codeInput)).toBe("SH.600519");
    expect(readSetupValue<Record<string, unknown>>(setup.backtestFormState)).toMatchObject({
      market: "CN",
      code: "SH.600519",
      instrumentId: "SH.600519",
    });
    writeSetupValue(setup, "instrumentSearchQuery", "贵州茅台");
    await nextTick();
    expect(readSetupValue<boolean>(setup.instrumentSelectionResolved)).toBe(false);
    expect(readSetupValue<string>(setup.codeInput)).toBe("SH.600519");
    expect(readSetupValue<Record<string, unknown>>(setup.backtestFormState)).toMatchObject({
      market: "CN",
      code: "",
      instrumentId: "",
    });
    writeSetupValue(setup, "codeInput", "");
    await nextTick();
    expect(readSetupValue<string>(setup.displayInstrumentId)).toBe("");
    writeSetupValue(setup, "selectedMarket", "US");
    writeSetupValue(setup, "codeInput", "US:AAPL");
    expect(readSetupValue<string>(setup.displayInstrumentId)).toBe("US.AAPL");
    writeSetupValue(setup, "codeInput", "AAPL");
    writeSetupValue(setup, "instrumentSearchQuery", "US.AAPL");
    expect(readSetupValue<string>(setup.displayInstrumentId)).toBe("US.AAPL");
    writeSetupValue(setup, "interval", "custom");
    expect(readSetupValue<string>(setup.periodLabel)).toBe("custom");
    expect(readSetupValue<string>(setup.extendedHoursHint)).toContain("不支持扩展交易时段");
    writeSetupValue(setup, "interval", "5m");
    writeSetupValue(setup, "useExtendedHours", true);
    await nextTick();
    expect(readSetupValue<string>(setup.extendedHoursHint)).toContain("盘前、盘后与夜盘");
    writeSetupValue(setup, "useExtendedHours", false);
    expect(readSetupValue<string>(setup.extendedHoursHint)).toContain("regular session");

    writeSetupValue(setup, "selectedDefinitionId", "");
    expect(readSetupValue<string>(setup.warmupPreviewValue)).toBe("--");
    writeSetupValue(setup, "selectedDefinitionId", "strategy-1");
    writeSetupValue(setup, "warmupPreviewPending", true);
    expect(readSetupValue<string>(setup.warmupPreviewValue)).toBe("计算中...");
    writeSetupValue(setup, "warmupPreviewPending", false);
    writeSetupValue(setup, "warmupPreviewBars", null);
    expect(readSetupValue<string>(setup.warmupPreviewValue)).toBe("自动推导");

    writeSetupValue(setup, "brokerFeeRulesText", '[{"name":"commission","rate":0.001}]');
    writeSetupValue(setup, "marketFeeRulesText", "{not-an-array}");
    await nextTick();
    expect(readSetupValue<unknown[]>(setup.brokerFeeRules)).toHaveLength(1);
    expect(readSetupValue<unknown[]>(setup.marketFeeRules)).toEqual([]);
    writeSetupValue(setup, "marketFeeRulesText", "[");
    await nextTick();
    expect(readSetupValue<unknown[]>(setup.marketFeeRules)).toEqual([]);
    expect(readSetupValue(setup.backtestFormState)).toMatchObject({
      definitionId: "strategy-1",
      market: "US",
      code: "AAPL",
      brokerFeeRules: [{ name: "commission", rate: 0.001 }],
    });
    await call<Promise<void>>("startBacktest");
    writeSetupValue(setup, "codeInput", "US:AAPL");
    await call<Promise<void>>("startBacktest");
    expect(readSetupValue<boolean>(setup.running)).toBe(false);

    const formSelects = page.findAll("select");
    expect(formSelects.length).toBeGreaterThanOrEqual(8);
    await formSelects[0]!.setValue("strategy-1");
    await formSelects[1]!.setValue("1d");
    await formSelects[2]!.setValue("heikinashi");
    await formSelects[3]!.setValue("backward");
    await formSelects[4]!.setValue("custom");
    await formSelects[5]!.setValue("custom");
    await nextTick();
    const formTextareas = page.findAll("textarea");
    expect(formTextareas).toHaveLength(2);
    await formTextareas[0]!.setValue('[{"name":"broker","rate":0.002}]');
    await formTextareas[1]!.setValue('[{"name":"market","rate":0.001}]');

    const codeField = page.findAll("input").find((input) =>
      input.attributes("placeholder") === "输入代码或名称",
    );
    expect(codeField).toBeDefined();
    await codeField!.setValue("00388");
    const dateFields = page.findAll('input[type="date"]');
    expect(dateFields).toHaveLength(2);
    await dateFields[0]!.setValue("2026-01-01");
    await dateFields[1]!.setValue("2026-02-01");
    await page.get('input[type="number"]').setValue("200000");

    writeSetupValue(setup, "selectedMarket", "US");
    writeSetupValue(setup, "interval", "5m");
    await nextTick();
    await page.get('input[type="checkbox"]').setValue(true);

    writeSetupValue(setup, "error", "temporary request error");
    await nextTick();
    const errorBanner = page.get(".backtest-error-banner__content");
    await errorBanner.trigger("click");
    expect(readSetupValue<boolean>(setup.errorExpanded)).toBe(true);
    await page.get('button[aria-label="关闭错误提示"]').trigger("click");
    expect(readSetupValue<string>(setup.error)).toBe("");

    const resultSearch = page.findAll("input").find((input) =>
      input.attributes("placeholder") === "搜索策略、标的、回测 ID",
    );
    expect(resultSearch).toBeDefined();
    await resultSearch!.setValue("US.AAPL");
    await formSelects[5]!.setValue("completed");
    await formSelects[6]!.setValue("strategy-1");

    expect(call("statusChip", "completed")).toMatchObject({ color: "success" });
    expect(call("statusChip", "failed")).toMatchObject({ color: "error" });
    expect(call("statusChip", "cancelled")).toMatchObject({ color: "warning" });
    expect(call("statusChip", "running")).toMatchObject({ color: "info" });
    expect(call("statusChip", "queued")).toMatchObject({ color: "warning" });
    expect(call("statusChip", "unknown")).toMatchObject({ color: "" });
    expect(backtestPresentation.pnlColor(10)).toBe("tv-up");
    expect(backtestPresentation.pnlColor(-1)).toBe("tv-down");
    expect(backtestPresentation.pnlPrefix(0)).toBe("+");
    expect(backtestPresentation.pnlPrefix(-1)).toBe("");
    expect(backtestPresentation.usesClosedTradeStats(richRun.result)).toBe(true);
    expect(backtestPresentation.usesClosedTradeStats({ totalTrades: 2 })).toBe(false);
    expect(backtestPresentation.backtestFillCount(richRun.result)).toBe(40);
    expect(backtestPresentation.backtestFillCount({ totalTrades: 2 })).toBe(2);
    expect(backtestPresentation.drawdownColor(0.1)).toBe("bt-metric-negative");
    expect(backtestPresentation.drawdownColor(0)).toBe("bt-text");
    expect(backtestPresentation.formatPercentMetric(0.1234)).toBe("12.34%");
    expect(backtestPresentation.formatPercentMetric(Number.NaN)).toBe("0.00%");
    expect(backtestPresentation.formatBacktestTimestamp(undefined)).toBe("--");
    expect(backtestPresentation.formatBacktestTimestamp("2026-06-01T00:00:00.000Z")).not.toBe("--");
    expect(backtestPresentation.formatBacktestRunDate("2026-06-01")).toBe("2026-06-01");
    expect(backtestPresentation.formatBacktestOrderSide("BUY")).toBe("买入");
    expect(backtestPresentation.formatBacktestOrderSide("SELL")).toBe("卖出");
    expect(backtestPresentation.formatBacktestOrderSide("SHORT")).toBe("SHORT");
    expect(backtestPresentation.formatBacktestOrderStatus("NEW")).toBe("已下单");
    expect(backtestPresentation.formatBacktestOrderStatus("FILLED")).toBe("已成交");
    expect(backtestPresentation.formatBacktestOrderStatus("CANCELED")).toBe("已撤单");
    expect(backtestPresentation.formatBacktestOrderStatus("REJECTED")).toBe("已拒绝");
    expect(backtestPresentation.formatBacktestOrderStatus("PARTIAL")).toBe("PARTIAL");
    expect(backtestPresentation.formatBacktestOrderPrice(0, "LIMIT", "101.2500")).toBe("101.2500");
    expect(backtestPresentation.formatBacktestOrderPrice(101.25, "LIMIT")).toContain("101.25");
    expect(backtestPresentation.formatBacktestOrderPrice(0, "MARKET")).toBe("市价");
    expect(backtestPresentation.formatBacktestOrderPrice(0, "LIMIT")).toBe("--");
    expect(backtestPresentation.formatBacktestQuantity(undefined)).toBe("--");
    expect(backtestPresentation.formatBacktestQuantity(10, "10.000")).toBe("10.000");
    expect(backtestPresentation.formatBacktestQuantity(10)).toContain("10");
    expect(backtestPresentation.formatBacktestFee(0, "USD")).toBe("--");
    expect(backtestPresentation.formatBacktestFee(2.5, "USD")).toContain("USD");
    expect(backtestPresentation.formatBacktestFee(2.5)).not.toContain("USD");

    expect(backtestPresentation.runtimeErrorTotal(richRun.result)).toBe(150);
    expect(backtestPresentation.runtimeErrorRepeatCount(richRun.result, "timeout")).toBe(50);
    expect(backtestPresentation.runtimeErrorRepeatCount(richRun.result, "other")).toBe(1);
    expect(backtestPresentation.runtimeErrorSummary(richRun.result)).toBe("运行时错误 150 次，仅显示 125 条样本");
    expect(backtestPresentation.runtimeErrorSummary({ runtimeErrors: ["one"] })).toBe("运行时错误 (1)");
    expect(backtestPresentation.warningTotal({ warningTotal: 8, warnings: ["one"] })).toBe(8);
    expect(backtestPresentation.warningTotal({ warnings: ["one", "two"] })).toBe(2);
    expect(backtestPresentation.warningSummary({ warnings: ["one"], warningTotal: 3, warningsTruncated: true })).toBe(
      "回测警告 (3)，仅显示 1 条样本",
    );
    expect(backtestPresentation.warningSummary({ warnings: ["one"], ignoredOrders: 2 })).toBe(
      "回测警告 1 条，忽略订单 2 笔",
    );
    expect(backtestPresentation.visibleBacktestWarnings(richRun)).toHaveLength(120);
    expect(backtestPresentation.hiddenBacktestWarningCount(richRun)).toBe(5);
    expect(backtestPresentation.visibleBacktestOrderBook(richRun)).toHaveLength(200);
    expect(backtestPresentation.hiddenBacktestOrderBookCount(richRun)).toBe(5);
    expect(backtestPresentation.visibleBacktestRuntimeErrors(richRun)).toHaveLength(120);
    expect(backtestPresentation.hiddenBacktestRuntimeErrorCount(richRun)).toBe(5);
    expect(backtestPresentation.visibleBacktestLogs(richRun)).toHaveLength(120);
    expect(backtestPresentation.hiddenBacktestLogCount(richRun)).toBe(85);
    call("resetResultsFilters");
    await nextTick();
    expect(readSetupValue(setup.focusedRun)).toMatchObject({ id: richRun.id });
    expect(readSetupValue<string>(setup.activeReportTab)).toBe("properties");
    expect(readSetupValue<string>(setup.selectedRunId)).toBe(richRun.id);
    call("selectFocusedRun", "run-002");
    await nextTick();
    expect(readSetupValue(setup.focusedRun)).toMatchObject({ id: "run-002" });
    expect(readSetupValue<string>(setup.activeReportTab)).toBe("chart");
    expect(backtestPresentation.resolveQueriedCandleBounds(undefined)).toBeNull();
    expect(backtestPresentation.resolveQueriedCandleBounds([{ time: "invalid" }])).toBeNull();
    expect(backtestPresentation.resolveQueriedCandleBounds(richRun.result.candles)).toMatchObject({ count: 2 });

    writeSetupValue(setup, "resultsSearchQuery", "US.AAPL");
    await nextTick();
    expect(readSetupValue<unknown[]>(setup.filteredRuns)).toHaveLength(1);
    expect(readSetupValue(setup.focusedRun)).toMatchObject({ id: richRun.id });
    expect(readSetupValue<string>(setup.resultsPageSummary)).toContain("筛选后");
    call("toggleNewBacktestForm");
    await nextTick();
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(false);
    writeSetupValue(setup, "resultsStatusFilter", "failed");
    await nextTick();
    expect(readSetupValue<unknown[]>(setup.filteredRuns)).toHaveLength(0);
    expect(readSetupValue<string>(setup.emptyResultsMessage)).toContain("没有匹配");
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(false);
    call("selectBacktestMobileSection", "report");
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("setup");
    call("resetResultsFilters");
    await nextTick();
    expect(readSetupValue<string>(setup.resultsSearchQuery)).toBe("");
    expect(readSetupValue<number>(setup.resultsPage)).toBe(1);
    expect(readSetupValue(setup.focusedRun)).toMatchObject({ id: richRun.id });
    writeSetupValue(setup, "resultsStrategyFilter", "missing-strategy");
    writeSetupValue(setup, "resultsPage", 99);
    await nextTick();
    await nextTick();
    expect(readSetupValue<number>(setup.resultsPage)).toBe(1);
    wrapper.unmount();
  });

  it("uses a closed overlay sidebar at medium widths and supports toggle, backdrop, and Escape", async () => {
    const mediumQuery = "(min-width: 769px) and (max-width: 1180px)";
    let mediumChangeListener: ((event: MediaQueryListEvent) => void) | null = null;
    const addEventListener = vi.fn(
      (eventName: string, listener: EventListenerOrEventListenerObject) => {
        if (eventName === "change" && typeof listener === "function") {
          mediumChangeListener = listener as (event: MediaQueryListEvent) => void;
        }
      },
    );
    const removeEventListener = vi.fn();
    const matchMedia = vi.fn((query: string) => ({
      matches: query === mediumQuery,
      media: query,
      onchange: null,
      addEventListener: query === mediumQuery ? addEventListener : vi.fn(),
      removeEventListener: query === mediumQuery ? removeEventListener : vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(() => true),
    }));
    vi.stubGlobal("matchMedia", matchMedia);

    installBacktestPageFetch({
      runs: [buildDetailedBacktestRun()],
      definitions: [{ id: "strategy-1", name: "EMA Reversal", version: "v2", symbol: "US.AAPL" }],
    });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();
    await flushRequests();
    const page = wrapper.getComponent(BacktestPage);
    const setup = page.vm.$.setupState as Record<string, unknown>;

    expect(matchMedia).toHaveBeenCalledWith(mediumQuery);
    expect(readSetupValue<boolean>(setup.isMediumBacktestWorkbench)).toBe(true);
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(false);
    expect(page.classes()).toContain("backtest-page--sidebar-closed");

    await page.get('[data-testid="backtest-sidebar-toggle"]').trigger("click");
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(true);
    expect(page.get('[data-testid="backtest-sidebar-backdrop"]').exists()).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await nextTick();
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(false);

    await page.get('[data-testid="backtest-sidebar-toggle"]').trigger("click");
    await page.get('[data-testid="backtest-sidebar-backdrop"]').trigger("click");
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(false);

    const listener = mediumChangeListener;
    expect(listener).not.toBeNull();
    listener?.({ matches: false } as MediaQueryListEvent);
    await nextTick();
    expect(readSetupValue<boolean>(setup.isMediumBacktestWorkbench)).toBe(false);
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(true);

    wrapper.unmount();
    expect(removeEventListener).toHaveBeenCalledWith("change", expect.any(Function));
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
