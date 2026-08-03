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
  buttonStub,
  createResponse,
  flushRequests,
  mountApp,
  passthroughStub,
  windowStub,
} from "../helpers";
import BacktestPage from "../../src/pages/BacktestPage.vue";
import AppTabs from "../../src/components/shared/AppTabs.vue";
import * as backtestPresentation from "../../src/components/backtest/backtestRunPresentation";

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

type SetupState = Record<string, unknown>;

function pageSetup(wrapper: any) {
  const page = wrapper.getComponent(BacktestPage);
  const setup = page.vm.$.setupState as SetupState;
  const call = <T>(name: string, ...args: unknown[]) =>
    (setup[name] as (...values: unknown[]) => T)(...args);
  return { page, setup, call };
}

describe("Backtest page compact UI interactions", () => {
  it("drives sidebar panels, run actions, history filters and both delete entries through the DOM", async () => {
    const richRun = buildDetailedBacktestRun();
    const fetchMock = installBacktestPageFetch({
      runs: [richRun, buildBacktestRun(2)],
      definitions: [
        { id: "strategy-1", name: "EMA Reversal", version: "v2", symbol: "US.AAPL" },
      ],
    });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();
    await flushRequests();
    const { page, setup, call } = pageSetup(wrapper);

    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(["history"]);

    // Header report-mode controls must drive the same state transitions as
    // route restoration and the programmatic comparison helpers.
    await page.get('[data-testid="backtest-open-version-comparison"]').trigger("click");
    expect(readSetupValue<string>(setup.reportMode)).toBe("compare");
    await page.get('[data-testid="backtest-report-mode-single"]').trigger("click");
    expect(readSetupValue<string>(setup.reportMode)).toBe("single");

    // The drawer header remains the direct close affordance on compact
    // layouts, even though the sidebar stays mounted for state preservation.
    await page
      .get('.bt-sidebar-drawer-head button[aria-label="关闭回测配置与历史"]')
      .trigger("click");
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(false);

    // Toggle the setup panel open and closed through its title button.
    await page.get('[data-testid="backtest-side-panel-setup-title"]').trigger("click");
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(["history", "setup"]);
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(true);
    expect(readSetupValue<boolean>(setup.newBacktestFormTouched)).toBe(true);
    await page.get('[data-testid="backtest-side-panel-setup-title"]').trigger("click");
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(false);
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(["history"]);

    // Non-array panel updates fall back to an empty selection.
    call("handleBacktestPanelsUpdate", "bogus");
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual([]);

    // Toggle the history panel through its title button.
    await page.get('[data-testid="backtest-side-panel-history-title"]').trigger("click");
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(["history"]);
    await page.get('[data-testid="backtest-side-panel-history-title"]').trigger("click");
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual([]);

    // The header action always re-adds the history panel alongside the form.
    await page.get('[data-testid="backtest-open-new-form"]').trigger("click");
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(["setup", "history"]);
    expect(readSetupValue<boolean>(setup.showNewBacktestForm)).toBe(true);
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(true);
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("setup");

    // Direct panel-open helper: both history-membership branches.
    call("setBacktestSetupPanelOpen", true);
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(["setup", "history"]);
    call("setBacktestSetupPanelOpen", false);
    expect(readSetupValue<string[]>(setup.expandedBacktestPanels)).toEqual(["history"]);
    await page.get('[data-testid="backtest-open-new-form"]').trigger("click");

    // 「同步K线」runs the full sync flow against the mocked task endpoints.
    const syncButton = page.findAll("button").find((button) => button.text().includes("同步K线"));
    expect(syncButton).toBeDefined();
    expect(syncButton!.attributes("disabled")).toBeUndefined();
    await syncButton!.trigger("click");
    await flushRequests();
    await flushRequests();
    expect(
      fetchMock.mock.calls.some(([input, init]) =>
        String(input).includes("/api/v1/backtests/sync") &&
        (init as RequestInit | undefined)?.method === "POST",
      ),
    ).toBe(true);
    expect(readSetupValue<boolean>(setup.syncing)).toBe(false);
    expect(readSetupValue<string>(setup.error)).toBe("");

    // Native history filter selects update the filter refs through v-model.
    const strategyFilter = page
      .findAll("select")
      .find((select) => select.attributes("aria-label") === "按策略筛选");
    expect(strategyFilter).toBeDefined();
    await strategyFilter!.setValue("strategy-1");
    expect(readSetupValue<string>(setup.resultsStrategyFilter)).toBe("strategy-1");
    await strategyFilter!.setValue("all");

    // Focus the completed run so the header exposes its terminal delete action.
    call("selectFocusedRun", richRun.id);
    await nextTick();

    await page.get('button[aria-label="删除当前回测结果"]').trigger("click");
    await nextTick();
    expect(wrapper.text()).toContain(`确认永久删除回测记录 ${richRun.id}`);
    const cancelButton = () =>
      wrapper.findAll("button").find((button) => button.text().trim() === "取消");
    await cancelButton()!.trigger("click");
    await nextTick();
    expect(wrapper.text()).not.toContain(`确认永久删除回测记录 ${richRun.id}`);

    // The history row delete is a v-btn: the stub only re-emits `click`, so emit
    // a DOM-like event payload to exercise the @click.stop handler.
    const rowDelete = page
      .findAllComponents(buttonStub as any)
      .find((component: any) => component.attributes("title") === "删除回测结果");
    expect(rowDelete).toBeDefined();
    rowDelete!.vm.$emit("click", { stopPropagation: () => undefined });
    await nextTick();
    expect(wrapper.text()).toContain("确认永久删除回测记录");
    await cancelButton()!.trigger("click");
    await nextTick();

    wrapper.unmount();
  });

  it("renders running/queued states, notices, tab switches and sync placeholders", async () => {
    const runningRun = buildBacktestRun(1) as Record<string, any>;
    runningRun.id = "run-running";
    runningRun.status = "running";
    const queuedRun = buildBacktestRun(2) as Record<string, any>;
    queuedRun.id = "run-queued";
    queuedRun.status = "queued";

    installBacktestPageFetch({
      runs: [runningRun, queuedRun],
      definitions: [
        { id: "strategy-1", name: "EMA Reversal", version: "v2", symbol: "US.AAPL" },
      ],
    });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();
    await flushRequests();
    const { page, setup, call } = pageSetup(wrapper);

    // History rows render both in-flight status texts.
    expect(wrapper.text()).toContain("回测运行中…");
    expect(wrapper.text()).toContain("排队等待中…");

    // The report notices mirror the running status and flag the stale version.
    call("selectFocusedRun", "run-running");
    await flushRequests();
    await nextTick();
    expect(wrapper.text()).toContain("旧版本策略回测结果");

    // Detail-load failures surface in the notices block.
    (setup.detailErrors as Record<string, string>)["run-running"] = "detail offline";
    await nextTick();
    expect(wrapper.text()).toContain("detail offline");

    // Queued runs render the queued notice branch.
    call("selectFocusedRun", "run-queued");
    await nextTick();
    const noticeTexts = page.findAll(".bt-report-notice").map((node) => node.text());
    expect(noticeTexts.some((text) => text.includes("排队等待中…"))).toBe(true);

    // Shared tabs and the report window update the same active-tab state.
    const tabs = page.findComponent(AppTabs);
    tabs.vm.$emit("update:modelValue", "orders");
    await nextTick();
    expect(readSetupValue<string>(setup.activeReportTab)).toBe("orders");
    page.findComponent(windowStub as any).vm.$emit("update:modelValue", "properties");
    await nextTick();
    expect(readSetupValue<string>(setup.activeReportTab)).toBe("properties");

    // Sync placeholders: pending start, empty current interval and retries.
    await page.get('[data-testid="backtest-open-new-form"]').trigger("click");
    writeSetupValue(setup, "syncing", true);
    writeSetupValue(setup, "syncProgress", null);
    await nextTick();
    expect(wrapper.text()).toContain("正在启动同步…");
    writeSetupValue(setup, "syncProgress", {
      taskId: "sync-live",
      status: "running",
      symbol: "US.AAPL",
      currentInterval: "",
      totalIntervals: 0,
      completedIntervals: 0,
      totalBatches: 0,
      completedBatches: 3,
      retries: 2,
      startedAt: "2026-07-03T00:00:00Z",
      updatedAt: "2026-07-03T00:01:00Z",
    });
    await nextTick();
    expect(wrapper.text()).toContain("同步中 · 准备");
    expect(wrapper.text()).toContain("重试 2");
    writeSetupValue(setup, "syncing", false);
    writeSetupValue(setup, "syncProgress", null);
    await nextTick();

    wrapper.unmount();
  });

  it("handles comparison native select changes and ignores guarded events", async () => {
    const baseline = buildDetailedBacktestRun();
    baseline.id = "run-baseline";
    baseline.createdAt = "2026-07-01T00:00:00.000Z";
    baseline.updatedAt = "2026-07-01T00:00:00.000Z";
    baseline.request.definitionVersion = "0.1.0";

    const candidate = buildDetailedBacktestRun();
    candidate.id = "run-candidate";
    candidate.createdAt = "2026-07-03T00:00:00.000Z";
    candidate.updatedAt = "2026-07-03T00:00:00.000Z";
    candidate.request.definitionVersion = "0.1.1";

    installBacktestPageFetch({
      runs: [baseline, candidate],
      definitions: [
        { id: "strategy-1", name: "Versioned EMA", version: "0.1.1", symbol: "US.AAPL" },
        { id: "strategy-2", name: "Other Strategy", version: "0.0.1", symbol: "US.AAPL" },
      ],
      versionsByDefinitionId: {
        "strategy-1": [
          { version: "0.1.1", name: "Versioned EMA", savedAt: "2026-07-03T00:00:00.000Z", isCurrent: true, script: "new" },
          { version: "0.1.0", name: "Versioned EMA", savedAt: "2026-07-01T00:00:00.000Z", script: "old" },
        ],
      },
    });

    const { wrapper } = await mountApp(
      "/backtest?mode=compare&definitionId=strategy-1&leftVersion=0.1.0&rightVersion=0.1.1&leftRunId=run-baseline&rightRunId=run-candidate",
    );
    for (let attempt = 0; attempt < 5; attempt += 1) {
      await flushRequests();
    }
    const { page, setup, call } = pageSetup(wrapper);
    expect(readSetupValue<string>(setup.reportMode)).toBe("compare");

    const definitionSelect = () => page.get('[data-testid="backtest-comparison-definition"]');

    // Re-selecting the current definition short-circuits without clearing runs.
    await definitionSelect().setValue("strategy-1");
    expect(readSetupValue<string>(setup.comparisonDefinitionId)).toBe("strategy-1");
    expect(readSetupValue<string>(setup.leftComparisonRunId)).toBe("run-baseline");

    // Switching definitions clears the selection and reloads version history.
    await definitionSelect().setValue("strategy-2");
    await flushRequests();
    expect(readSetupValue<string>(setup.comparisonDefinitionId)).toBe("strategy-2");
    expect(readSetupValue<string>(setup.leftComparisonRunId)).toBe("");
    expect(readSetupValue<unknown[]>(setup.comparisonVersions)).toEqual([]);

    await definitionSelect().setValue("strategy-1");
    await flushRequests();
    await flushRequests();
    expect(readSetupValue<string>(setup.leftComparisonVersion)).toBe("0.1.0");
    expect(readSetupValue<string>(setup.rightComparisonVersion)).toBe("0.1.1");
    expect(readSetupValue<string>(setup.leftComparisonRunId)).toBe("run-baseline");
    expect(readSetupValue<string>(setup.rightComparisonRunId)).toBe("run-candidate");

    // Picking the opposite side's version is rejected by the guard.
    call("changeComparisonVersion", "left", "0.1.1");
    expect(readSetupValue<string>(setup.leftComparisonVersion)).toBe("0.1.0");
    call("changeComparisonVersion", "right", "0.1.0");
    expect(readSetupValue<string>(setup.rightComparisonVersion)).toBe("0.1.1");

    // Re-applying a version through the native select resets its run selection.
    await page.get('[data-testid="backtest-comparison-left-version"]').setValue("0.1.0");
    await flushRequests();
    expect(readSetupValue<string>(setup.leftComparisonRunId)).toBe("");
    await page.get('[data-testid="backtest-comparison-left-run"]').setValue("run-baseline");
    expect(readSetupValue<string>(setup.leftComparisonRunId)).toBe("run-baseline");

    await page.get('[data-testid="backtest-comparison-right-version"]').setValue("0.1.1");
    await flushRequests();
    expect(readSetupValue<string>(setup.rightComparisonRunId)).toBe("");
    await page.get('[data-testid="backtest-comparison-right-run"]').setValue("run-candidate");
    expect(readSetupValue<string>(setup.rightComparisonRunId)).toBe("run-candidate");

    // Non-select events never produce a value.
    expect(call("nativeSelectValue", new Event("change"))).toBe("");

    wrapper.unmount();
  });

  it("supports legacy matchMedia listeners and the mobile sidebar toggle branch", async () => {
    const addListener = vi.fn();
    const removeListener = vi.fn();
    vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
      matches: false,
      media: query,
      addListener,
      removeListener,
    })));

    installBacktestPageFetch({ runs: [] });
    const { wrapper } = await mountApp("/backtest");
    await flushRequests();
    const { page, setup } = pageSetup(wrapper);

    // Legacy MediaQueryList without addEventListener falls back to addListener.
    expect(addListener).toHaveBeenCalledWith(expect.any(Function));

    // Narrow viewport: the toggle only reselects the setup section.
    Object.defineProperty(window, "innerWidth", { configurable: true, writable: true, value: 500 });
    writeSetupValue(setup, "backtestSidebarOpen", false);
    writeSetupValue(setup, "backtestMobileSection", "report");
    await page.get('[data-testid="backtest-sidebar-toggle"]').trigger("click");
    expect(readSetupValue<string>(setup.backtestMobileSection)).toBe("setup");
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(false);

    // Wide viewport: the toggle flips sidebar visibility.
    Object.defineProperty(window, "innerWidth", { configurable: true, writable: true, value: 1280 });
    await page.get('[data-testid="backtest-sidebar-toggle"]').trigger("click");
    expect(readSetupValue<boolean>(setup.backtestSidebarOpen)).toBe(true);

    wrapper.unmount();
    expect(removeListener).toHaveBeenCalledWith(expect.any(Function));
  });

  it("uses render-window fallbacks and renders order-table extras", async () => {
    const sparseRun = buildBacktestRun(1) as Record<string, any>;
    sparseRun.id = "run-sparse";
    sparseRun.result = {
      symbol: "US.AAPL",
      interval: "5m",
      startTime: "2026-07-01T00:00:00Z",
      endTime: "2026-07-03T00:00:00Z",
      finalBalance: 100_000,
      pnl: 0,
      totalTrades: 0,
      winRate: 0,
      pnlCurve: [{ time: "2026-07-01T00:00:00.000Z", equity: 100_000 }],
    };

    const richRun = buildDetailedBacktestRun();
    richRun.id = "run-rich";
    richRun.result.orderBook[0].clientOrderId = "client-0";
    richRun.result.orderBook[1].filledQuantity = 5;
    richRun.result.orderBook[2].totalFee = 2.5;
    richRun.result.orderBook[2].brokerFee = 1.5;
    richRun.result.orderBook[2].marketFee = 1;
    richRun.result.orderBook[2].feeCurrency = "USD";

    installBacktestPageFetch({
      runs: [sparseRun, richRun, buildBacktestRun(3), buildBacktestRun(4), buildBacktestRun(5), buildBacktestRun(6)],
      definitions: [
        { id: "strategy-1", name: "EMA Reversal", version: "v1", symbol: "US.AAPL" },
      ],
    });

    const { wrapper } = await mountApp("/backtest");
    await flushRequests();
    await flushRequests();
    const { page, setup, call } = pageSetup(wrapper);

    // A result without chart arrays still mounts the chart with empty fallbacks.
    call("selectFocusedRun", "run-sparse");
    await flushRequests();
    await nextTick();
    expect(wrapper.get('[data-testid="backtest-chart"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("暂无订单记录。");

    // Nullish render-window helpers tolerate missing result fields.
    expect(backtestPresentation.visibleBacktestOrderBook({ result: {} })).toEqual([]);
    expect(backtestPresentation.hiddenBacktestOrderBookCount({ result: undefined })).toBe(0);
    expect(backtestPresentation.visibleBacktestRuntimeErrors({})).toEqual([]);
    expect(backtestPresentation.hiddenBacktestRuntimeErrorCount({ result: {} })).toBe(0);
    expect(backtestPresentation.visibleBacktestWarnings({ result: null })).toEqual([]);
    expect(backtestPresentation.hiddenBacktestWarningCount({})).toBe(0);
    expect(backtestPresentation.visibleBacktestLogs({ result: {} })).toEqual([]);
    expect(backtestPresentation.hiddenBacktestLogCount({})).toBe(0);

    // The detailed run renders order extras and truncated collections.
    call("selectFocusedRun", "run-rich");
    await flushRequests();
    await nextTick();
    const text = wrapper.text();
    expect(text).toContain("client-0");
    expect(text).toContain("成交 5");
    expect(text).toContain("另有 5 笔订单。");
    expect(text).toContain("另有 5 条错误。");
    expect(text).toContain("另有 5 条警告。");
    expect(text).toContain("另有 85 条日志。");
    const orderTable = wrapper.get(".bt-order-table");
    expect(orderTable.text()).toContain("券商");
    expect(orderTable.text()).toContain("市场");

    // The sidebar pagination forwards page changes through its v-model handler.
    const pagination = page
      .findAllComponents(passthroughStub as any)
      .find((component: any) => component.classes().includes("bt-sidebar-pagination"));
    expect(pagination).toBeDefined();
    pagination!.vm.$emit("update:modelValue", 2);
    await nextTick();
    expect(readSetupValue<number>(setup.resultsPage)).toBe(2);

    wrapper.unmount();
  });
});

function installBacktestPageFetch(options: {
  runs: unknown[];
  listRuns?: unknown[];
  definitions?: unknown[];
  versionsByDefinitionId?: Record<string, Array<{
    version: string;
    name?: string;
    savedAt?: string;
    isCurrent?: boolean;
    script?: string;
  }>>;
}) {
  const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
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
        return createResponse({ entries: [] });
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
        const definition = options.definitions?.[0] ?? {};
        return createResponse(definition);
      }
      if (url.includes("/api/v1/strategy-definitions")) {
        return createResponse(options.definitions ?? []);
      }
      if (url.includes("/api/v1/backtests/sync") && init?.method === "POST") {
        return createResponse({ taskId: "sync-task-1", message: "started" });
      }
      if (url.includes("/api/v1/backtests/sync/")) {
        return createResponse({
          taskId: "sync-task-1",
          status: "completed",
          symbol: "US.AAPL",
          currentInterval: "5m",
          totalIntervals: 1,
          completedIntervals: 1,
          totalBatches: 1,
          completedBatches: 1,
          retries: 0,
          startedAt: "2026-07-03T00:00:00Z",
          updatedAt: "2026-07-03T00:01:00Z",
        });
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
