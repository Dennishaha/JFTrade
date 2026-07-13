// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import {
  emptyBrokerSettings,
  emptyMarketDataSubscriptions,
  emptyOnboardingState,
  emptyPluginCatalog,
  emptyStorageOverview,
  emptySystemStatus,
} from "@/contracts";

import {
  MockWebSocket,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";
import BacktestPage from "../src/pages/BacktestPage.vue";

const backtestFormStorageKey = "jftrade.backtest.form.v1";

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
  window.localStorage.clear();
  window.sessionStorage.clear();
});

describe("Backtest page", () => {
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
    expect(wrapper.text()).toContain("回测报告");
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
    expect(page.get(".bt-report-window-item--chart").classes()).toContain("bt-report-window-item");
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(false);
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
    writeSetupValue(setup, "activeReportTab", "properties");
    await nextTick();
    expect(wrapper.text()).toContain("最终资金");

    call("toggleNewBacktestForm");
    await nextTick();
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(true);
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
    expect(call("resolveBacktestPriceBasisNote", richRun)).toContain("已闭合历史 K 线");
    expect(call("resolveBacktestPriceBasisNote", {
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
    writeSetupValue(setup, "selectedMarket", "US");
    writeSetupValue(setup, "codeInput", "");
    await nextTick();
    expect(readSetupValue<string>(setup.displayInstrumentId)).toBe("");
    expect(readSetupValue<unknown[]>(setup.codeSuggestions)).toEqual([
      { value: "AAPL", title: "AAPL · Apple" },
    ]);
    writeSetupValue(setup, "codeInput", "US:AAPL");
    expect(readSetupValue<string>(setup.displayInstrumentId)).toBe("US.AAPL");
    writeSetupValue(setup, "codeInput", "AAPL");
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
    expect(formSelects.length).toBeGreaterThanOrEqual(9);
    await formSelects[0]!.setValue("strategy-1");
    await formSelects[1]!.setValue("HK");
    await formSelects[2]!.setValue("etf");
    await formSelects[3]!.setValue("1d");
    await formSelects[4]!.setValue("backward");
    await formSelects[5]!.setValue("custom");
    await formSelects[6]!.setValue("custom");
    await nextTick();
    const formTextareas = page.findAll("textarea");
    expect(formTextareas).toHaveLength(2);
    await formTextareas[0]!.setValue('[{"name":"broker","rate":0.002}]');
    await formTextareas[1]!.setValue('[{"name":"market","rate":0.001}]');

    const codeField = page.findAll("input").find((input) =>
      input.attributes("placeholder") === "00700",
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
    const closeErrorButton = page.findAll("button").find((button) => button.text() === "关闭");
    expect(closeErrorButton).toBeDefined();
    await closeErrorButton!.trigger("click");
    expect(readSetupValue<string>(setup.error)).toBe("");

    const resultSearch = page.findAll("input").find((input) =>
      input.attributes("placeholder") === "搜索策略、标的、回测 ID",
    );
    expect(resultSearch).toBeDefined();
    await resultSearch!.setValue("US.AAPL");
    await formSelects[7]!.setValue("completed");
    await formSelects[8]!.setValue("strategy-1");

    expect(call("statusChip", "completed")).toMatchObject({ color: "success" });
    expect(call("statusChip", "failed")).toMatchObject({ color: "error" });
    expect(call("statusChip", "cancelled")).toMatchObject({ color: "warning" });
    expect(call("statusChip", "running")).toMatchObject({ color: "info" });
    expect(call("statusChip", "queued")).toMatchObject({ color: "warning" });
    expect(call("statusChip", "unknown")).toMatchObject({ color: "" });
    expect(call("pnlColor", 10)).toBe("tv-up");
    expect(call("pnlColor", -1)).toBe("tv-down");
    expect(call("pnlPrefix", 0)).toBe("+");
    expect(call("pnlPrefix", -1)).toBe("");
    expect(call("drawdownColor", 0.1)).toBe("bt-metric-negative");
    expect(call("drawdownColor", 0)).toBe("bt-text");
    expect(call("formatPercentMetric", 0.1234)).toBe("12.34%");
    expect(call("formatPercentMetric", Number.NaN)).toBe("0.00%");
    expect(call("formatBacktestTimestamp", undefined)).toBe("--");
    expect(call<string>("formatBacktestTimestamp", "2026-06-01T00:00:00.000Z")).not.toBe("--");
    expect(call("formatBacktestRunDate", "2026-06-01")).toBe("2026-06-01");
    expect(call("formatBacktestOrderSide", "BUY")).toBe("买入");
    expect(call("formatBacktestOrderSide", "SELL")).toBe("卖出");
    expect(call("formatBacktestOrderSide", "SHORT")).toBe("SHORT");
    expect(call("formatBacktestOrderStatus", "NEW")).toBe("已下单");
    expect(call("formatBacktestOrderStatus", "FILLED")).toBe("已成交");
    expect(call("formatBacktestOrderStatus", "CANCELED")).toBe("已撤单");
    expect(call("formatBacktestOrderStatus", "REJECTED")).toBe("已拒绝");
    expect(call("formatBacktestOrderStatus", "PARTIAL")).toBe("PARTIAL");
    expect(call("formatBacktestOrderPrice", 0, "LIMIT", "101.2500")).toBe("101.2500");
    expect(call<string>("formatBacktestOrderPrice", 101.25, "LIMIT")).toContain("101.25");
    expect(call("formatBacktestOrderPrice", 0, "MARKET")).toBe("市价");
    expect(call("formatBacktestOrderPrice", 0, "LIMIT")).toBe("--");
    expect(call("formatBacktestQuantity", undefined)).toBe("--");
    expect(call("formatBacktestQuantity", 10, "10.000")).toBe("10.000");
    expect(call<string>("formatBacktestQuantity", 10)).toContain("10");
    expect(call("formatBacktestFee", 0, "USD")).toBe("--");
    expect(call<string>("formatBacktestFee", 2.5, "USD")).toContain("USD");
    expect(call<string>("formatBacktestFee", 2.5)).not.toContain("USD");

    expect(call("runtimeErrorTotal", richRun.result)).toBe(150);
    expect(call("runtimeErrorRepeatCount", richRun.result, "timeout")).toBe(50);
    expect(call("runtimeErrorRepeatCount", richRun.result, "other")).toBe(1);
    expect(call("runtimeErrorSummary", richRun.result)).toBe("运行时错误 150 次，仅显示 125 条样本");
    expect(call("runtimeErrorSummary", { runtimeErrors: ["one"] })).toBe("运行时错误 (1)");
    expect(call("warningTotal", { warningTotal: 8, warnings: ["one"] })).toBe(8);
    expect(call("warningTotal", { warnings: ["one", "two"] })).toBe(2);
    expect(call("warningSummary", { warnings: ["one"], warningTotal: 3, warningsTruncated: true })).toBe(
      "回测警告 (3)，仅显示 1 条样本",
    );
    expect(call("warningSummary", { warnings: ["one"], ignoredOrders: 2 })).toBe(
      "回测警告 1 条，忽略订单 2 笔",
    );
    expect(call("visibleBacktestWarnings", richRun)).toHaveLength(120);
    expect(call("hiddenBacktestWarningCount", richRun)).toBe(5);
    expect(call("visibleBacktestOrderBook", richRun)).toHaveLength(200);
    expect(call("hiddenBacktestOrderBookCount", richRun)).toBe(5);
    expect(call("visibleBacktestRuntimeErrors", richRun)).toHaveLength(120);
    expect(call("hiddenBacktestRuntimeErrorCount", richRun)).toBe(5);
    expect(call("visibleBacktestLogs", richRun)).toHaveLength(120);
    expect(call("hiddenBacktestLogCount", richRun)).toBe(85);
    call("resetResultsFilters");
    await nextTick();
    expect(readSetupValue(setup.focusedRun)).toMatchObject({ id: richRun.id });
    expect(readSetupValue<string>(setup.activeReportTab)).toBe("properties");
    expect(readSetupValue<string>(setup.selectedRunId)).toBe(richRun.id);
    call("selectFocusedRun", "run-002");
    await nextTick();
    expect(readSetupValue(setup.focusedRun)).toMatchObject({ id: "run-002" });
    expect(readSetupValue<string>(setup.activeReportTab)).toBe("chart");
    expect(call("resolveQueriedCandleBounds", undefined)).toBeNull();
    expect(call("resolveQueriedCandleBounds", [{ time: "invalid" }])).toBeNull();
    expect(call("resolveQueriedCandleBounds", richRun.result.candles)).toMatchObject({ count: 2 });

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
});

function installBacktestPageFetch(options: { runs: unknown[]; definitions?: unknown[] }): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse(emptySystemStatus);
      }
      if (url.includes("/api/v1/system/storage/overview")) {
        return createResponse(emptyStorageOverview);
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
          ],
        });
      }
      if (url.includes("/api/v1/strategy-definitions/")) {
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
        return createResponse({ runs: options.runs });
      }

      throw new Error(`Unexpected request: ${url}`);
    }),
  );
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
    rehabType: "none",
    useExtendedHours: true,
    definitionVersion: "v1",
  };
  run.result = {
    quoteCurrency: "USD",
    pnl: 1250.5,
    pnlPct: 0.0125,
    maxDrawdown: 0.08,
    finalBalance: 101_250.5,
    totalTrades: 20,
    winRate: 0.55,
    currentDrawdown: 0.02,
    totalBrokerFees: 12.5,
    totalMarketFees: 8.5,
    totalFees: 21,
    tradingCosts: {},
    trades: [],
    pnlCurve: [],
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
