// @vitest-environment jsdom

import { computed, defineComponent, h, nextTick, ref } from "vue";
import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BacktestFormState } from "../src/composables/useBacktestRuns";
import { queryClient, queryKeys } from "../src/composables/serverState";

const mocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiGetPath: vi.fn(),
  apiDeletePath: vi.fn(),
  apiPost: vi.fn(),
  startSync: vi.fn(),
  cancelKlineSync: vi.fn(),
  syncing: { value: false },
  syncProgress: { value: null as { status: string } | null },
  syncError: { value: "" },
}));
const {
  apiGet,
  apiGetPath,
  apiDeletePath,
  apiPost,
  startSync,
  cancelKlineSync,
  syncing,
  syncProgress,
  syncError,
} = mocks;

vi.mock("../src/composables/apiClient", () => ({
  apiGet: mocks.apiGet,
  apiGetPath: mocks.apiGetPath,
  apiDeletePath: mocks.apiDeletePath,
  apiPost: mocks.apiPost,
}));

vi.mock("../src/composables/useKlineSyncTask", () => ({
  useKlineSyncTask: () => ({
    syncing: mocks.syncing,
    syncProgress: mocks.syncProgress,
    syncError: mocks.syncError,
    startSync: mocks.startSync,
    cancelSync: mocks.cancelKlineSync,
  }),
}));

import {
  toBacktestStartRequestWire,
  useBacktestRuns,
} from "../src/composables/useBacktestRuns";

const baseForm: BacktestFormState = {
  definitionId: "def-1",
  definitionVersion: "1.0.0",
  market: "US",
  code: "AAPL",
  instrumentId: "US.AAPL",
  instrumentType: "stock",
  interval: "5m",
  chartType: "standard",
  startDate: "2026-06-01",
  endDate: "2026-06-30",
  initialBalance: 100000,
  rehabType: "forward",
  useExtendedHours: false,
  brokerFeeMode: "market_preset",
  marketFeeMode: "market_preset",
  brokerFeeRules: [],
  marketFeeRules: [],
};

type BacktestState = ReturnType<typeof useBacktestRuns>;
let wrappers: VueWrapper[] = [];

function makeRun(id: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    status: "completed",
    request: {
      definitionId: "def-1",
      symbol: "US.AAPL",
      interval: "5m",
      startTime: "2026-06-01T00:00:00Z",
      endTime: "2026-06-30T00:00:00Z",
      initialBalance: 100000,
    },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function mountBacktestRuns(input: {
  form?: Partial<BacktestFormState>;
  normalizeInstrument?: ReturnType<typeof vi.fn>;
} = {}) {
  const form = ref<BacktestFormState>({ ...baseForm, ...input.form });
  const normalizeInstrument = input.normalizeInstrument ?? vi.fn(async () => ({
    market: "US",
    prefix: "US",
    code: "AAPL",
    instrumentId: "US.AAPL",
  }));
  let state: BacktestState | undefined;
  const wrapper = mount(defineComponent({
    setup() {
      state = useBacktestRuns({ formState: computed(() => form.value), normalizeInstrument });
      return () => h("div");
    },
  }));
  wrappers.push(wrapper);
  if (state == null) throw new Error("backtest composable was not initialized");
  return { state, form, normalizeInstrument, wrapper };
}

beforeEach(() => {
  queryClient.clear();
  vi.clearAllMocks();
  vi.useRealTimers();
  syncing.value = false;
  syncProgress.value = null;
  syncError.value = "";
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useBacktestRuns", () => {
  it("maps sparse and complete backtest requests to the generated wire contract", () => {
    expect(toBacktestStartRequestWire({
      definitionId: "def-sparse",
      interval: "1d",
      startDate: "",
      endDate: "",
      initialBalance: 10_000,
    })).toEqual({
      definitionId: "def-sparse",
      market: "",
      code: "",
      symbol: "",
      interval: "1d",
      chartType: "standard",
      initialBalance: 10_000,
      rehabType: "",
      tradingCosts: {
        brokerFees: {},
        marketFees: {},
      },
    });

    expect(toBacktestStartRequestWire({
      definitionId: "def-full",
      definitionVersion: "2.0.0",
      market: "US",
      code: "AAPL",
      symbol: "US.AAPL",
      instrumentType: "stock",
      interval: "5m",
      chartType: "heikinashi",
      startDate: "2026-01-01",
      endDate: "2026-02-01",
      startTime: "2026-01-01T00:00:00Z",
      endTime: "2026-02-01T00:00:00Z",
      initialBalance: 100_000,
      rehabType: "forward",
      useExtendedHours: true,
      executionModel: "conservative-bar-v1",
      tradingCosts: {
        brokerFees: {
          mode: "custom",
          presetId: "broker-preset",
          rules: [{
            id: "commission",
            category: "broker",
            basis: "notional",
            rate: 0.001,
          }],
        },
        marketFees: {
          mode: "market_preset",
          presetId: "market-preset",
          rules: [{
            id: "exchange",
            label: "交易所费用",
            category: "exchange",
            basis: "order",
            fixedAmount: 1,
          }],
        },
      },
    })).toMatchObject({
      definitionVersion: "2.0.0",
      market: "US",
      code: "AAPL",
      symbol: "US.AAPL",
      instrumentType: "stock",
      chartType: "heikinashi",
      startDate: "2026-01-01",
      endDate: "2026-02-01",
      startTime: "2026-01-01T00:00:00Z",
      endTime: "2026-02-01T00:00:00Z",
      useExtendedHours: true,
      executionModel: "conservative-bar-v1",
      tradingCosts: {
        brokerFees: {
          mode: "custom",
          presetId: "broker-preset",
          rules: [expect.objectContaining({ label: "" })],
        },
        marketFees: {
          rules: [expect.objectContaining({ label: "交易所费用" })],
        },
      },
    });
  });

  it("loads and normalizes full decimal transport results", async () => {
    apiGet.mockResolvedValue({
      runs: [makeRun("run-1", {
        result: {
          symbol: "US.AAPL",
          interval: "5m",
          chartType: " HEIKINASHI ",
          heikinAshiSeed: { open: 121.25, close: 122.5 },
          startTime: "2026-06-01T00:00:00Z",
          endTime: "2026-06-30T00:00:00Z",
          finalBalance: 100010,
          pnl: 10,
          tradeStatsVersion: 2,
          totalFills: 2,
          totalTrades: 1,
          winRate: 1,
          trades: [{
            time: "2026-06-02T00:00:00Z",
            side: "BUY",
            price: "123.4500",
            qty: "bad-decimal",
            pnl: 10,
            brokerFee: 1,
            marketFee: 2,
            totalFee: 3,
            feeCurrency: "USD",
            warmup: true,
          }],
          orderBook: [{
            orderId: "order-1",
            clientOrderId: "client-1",
            symbol: "US.AAPL",
            side: "BUY",
            quantity: "10.000",
            orderType: "LIMIT",
            orderPrice: "123.4500",
            submittedAt: "2026-06-02T00:00:00Z",
            status: "FILLED",
            filledQuantity: 10,
            filledPrice: "not-a-number",
            filledAt: "2026-06-02T00:00:01Z",
            brokerFee: 1,
            marketFee: 2,
            totalFee: 3,
            feeCurrency: "USD",
            warmup: true,
          }, {
            orderId: "order-2",
            symbol: "US.AAPL",
            side: "SELL",
            quantity: 5,
            status: "NEW",
          }],
          candles: [{
            time: "2026-06-02T00:00:00Z",
            open: "",
            high: Number.POSITIVE_INFINITY,
            low: "122.00",
            close: 123.45,
            volume: "1000",
          }],
          pnlCurve: [{ time: "2026-06-02T00:00:00Z", value: 10 }],
          drawdownCurve: [{ time: "2026-06-02T00:00:00Z", value: -1 }],
          runtimeErrors: ["warning"],
          logs: ["filled"],
        },
      })],
    });
    const { state } = mountBacktestRuns();

    await state.loadRuns();

    expect(state.runs.value).toHaveLength(1);
    const result = state.runs.value[0]?.result;
    expect(result).toMatchObject({
      chartType: "heikinashi",
      heikinAshiSeed: { open: 121.25, close: 122.5 },
      tradeStatsVersion: 2,
      totalFills: 2,
      totalTrades: 1,
    });
    expect(result?.trades?.[0]).toMatchObject({
      price: 123.45,
      priceText: "123.4500",
      qty: 0,
      qtyText: "bad-decimal",
      totalFee: 3,
      warmup: true,
    });
    expect(result?.orderBook?.[0]).toMatchObject({
      quantity: 10,
      quantityText: "10.000",
      filledPrice: undefined,
      filledPriceText: "not-a-number",
      warmup: true,
    });
    expect(result?.orderBook?.[1]).toMatchObject({
      quantity: 5,
      orderPrice: undefined,
      filledQuantity: undefined,
      filledPrice: undefined,
    });
    expect(result?.candles?.[0]).toMatchObject({
      open: 0,
      openText: undefined,
      high: 0,
      low: 122,
      close: 123.45,
      volume: 1000,
    });
  });

  it("normalizes or drops malformed Heikin Ashi seed transport metadata", async () => {
    const resultBase = {
      symbol: "US.AAPL",
      interval: "5m",
      startTime: "2026-06-01T00:00:00Z",
      endTime: "2026-06-30T00:00:00Z",
      finalBalance: 100000,
      pnl: 0,
      totalTrades: 0,
      winRate: 0,
    };
    apiGet.mockResolvedValue({
      runs: [
        makeRun("ha-malformed", {
          request: {
            definitionId: "def-1",
            symbol: "US.AAPL",
            interval: "5m",
            chartType: " HEIKINASHI ",
            startTime: "2026-06-01T00:00:00Z",
            endTime: "2026-06-30T00:00:00Z",
            initialBalance: 100000,
          },
          result: {
            ...resultBase,
            chartType: null,
            heikinAshiSeed: { open: null, close: 12 },
          },
        }),
        makeRun("ha-string-seed", {
          result: {
            ...resultBase,
            heikinAshiSeed: { open: "11.25", close: "12.50" },
          },
        }),
        makeRun("ha-array-seed", {
          result: { ...resultBase, heikinAshiSeed: [] },
        }),
      ],
    });
    const { state } = mountBacktestRuns();

    await state.loadRuns();

    const malformed = state.runs.value.find((run) => run.id === "ha-malformed");
    expect(malformed?.request.chartType).toBe("heikinashi");
    expect(malformed?.result?.chartType).toBe("standard");
    expect(malformed?.result?.heikinAshiSeed).toBeUndefined();
    expect(
      state.runs.value.find((run) => run.id === "ha-string-seed")?.result
        ?.heikinAshiSeed,
    ).toEqual({ open: 11.25, close: 12.5 });
    expect(
      state.runs.value.find((run) => run.id === "ha-array-seed")?.result
        ?.heikinAshiSeed,
    ).toBeUndefined();
  });

  it("normalizes optional OpenAPI transport fields and complete fee schedules", async () => {
    const completeRule = {
      id: "rule-complete",
      label: "Complete",
      category: "exchange",
      side: "buy",
      basis: "share",
      rate: 0.001,
      fixedAmount: 1,
      minAmount: 2,
      maxAmount: 3,
      maxRate: 0.01,
      rounding: "ceil",
      currency: "USD",
      appliesTo: "fills",
      effectiveFrom: "2026-01-01",
      effectiveTo: "2026-12-31",
      sourceUrl: "https://example.test/fees",
    };
    const categoryRules = [
      completeRule,
      { category: "clearing", side: "sell", basis: "order" },
      { category: "regulatory", side: "both", basis: "notional" },
      { category: "tax", side: "invalid", basis: "invalid" },
      {},
    ];
    apiGet.mockResolvedValue({
      runs: [
        {
          request: {},
          result: {
            trades: [{ price: 0, qty: 0 }],
            orderBook: [{ quantity: 0 }],
            candles: [{ open: 0, high: 0, low: 0, close: 0, volume: 0 }],
            pnlCurve: [{}],
            drawdownCurve: [{}],
            feeBreakdown: [{}],
            tradingCosts: {
              brokerFees: { mode: "script", rules: categoryRules },
              marketFees: { mode: "none" },
            },
            executionModel: "conservative-bar-v1",
          },
        },
        makeRun("run-fees", {
          request: {
            definitionId: "def-2",
            definitionVersion: "2.0.0",
            market: "HK",
            code: "00700",
            symbol: "HK.00700",
            instrumentType: "stock",
            interval: "1d",
            startDate: "2026-01-01",
            endDate: "2026-02-01",
            startTime: "2026-01-01T00:00:00Z",
            endTime: "2026-02-01T00:00:00Z",
            marketTimezone: "Asia/Hong_Kong",
            initialBalance: 1_000_000,
            rehabType: "forward",
            useExtendedHours: false,
            tradingCosts: {
              brokerFees: { mode: "market_preset", presetId: "futu-hk" },
              marketFees: { mode: "custom", rules: categoryRules },
            },
            executionModel: "conservative-bar-v1",
          },
          result: {
            symbol: "HK.00700",
            interval: "1d",
            startTime: "2026-01-01T00:00:00Z",
            endTime: "2026-02-01T00:00:00Z",
            finalBalance: 1_000_100,
            pnl: 100,
            totalTrades: 1,
            winRate: 1,
            feeBreakdown: [{
              ruleId: "rule-complete",
              label: "Complete",
              group: "broker",
              category: "exchange",
              currency: "USD",
              amount: 3,
              count: 1,
            }],
            pnlCurve: [{ time: "2026-01-02T00:00:00Z", equity: 1_000_100 }],
            drawdownCurve: [{ time: "2026-01-02T00:00:00Z", drawdown: 0 }],
            tradingCosts: {
              brokerFees: { mode: "invalid", rules: [] },
            },
          },
        }),
      ],
    });
    const { state } = mountBacktestRuns();

    await state.loadRuns();

    const empty = state.runs.value.find((run) => run.id === "");
    expect(empty).toMatchObject({ id: "", status: "", createdAt: "", updatedAt: "" });
    expect(empty?.result?.trades?.[0]).toMatchObject({ time: "", side: "" });
    expect(empty?.result?.feeBreakdown?.[0]).toEqual({
      ruleId: "", label: "", group: "", category: "", currency: "", amount: 0, count: 0,
    });
    expect(empty?.result?.tradingCosts?.brokerFees?.rules).toHaveLength(5);
    expect(empty?.result?.executionModel).toBe("conservative-bar-v1");
    expect(empty?.result?.chartType).toBeUndefined();

    const complete = state.runs.value.find((run) => run.id === "run-fees");
    expect(complete?.request).toMatchObject({
      definitionVersion: "2.0.0",
      market: "HK",
      code: "00700",
      instrumentType: "stock",
      marketTimezone: "Asia/Hong_Kong",
      executionModel: "conservative-bar-v1",
    });
    expect(complete?.result?.feeBreakdown?.[0]).toMatchObject({ amount: 3, count: 1 });
    expect(complete?.result?.tradingCosts?.brokerFees).toEqual({ rules: [] });
  });

  it("keeps the preferred duplicate run and sorts newest first", async () => {
    apiGet.mockResolvedValue({
      runs: [
        makeRun("same", { status: "running", createdAt: "2026-07-01T00:00:00Z", updatedAt: "2026-07-02T00:00:00Z" }),
        makeRun("same", { status: "completed", createdAt: "2026-07-01T00:00:00Z", updatedAt: "2026-07-01T00:00:00Z", result: { symbol: "US.AAPL" } }),
        makeRun("new", { createdAt: "2026-07-03T00:00:00Z" }),
      ],
    });
    const { state } = mountBacktestRuns();
    await state.loadRuns();

    expect(state.runs.value.find((run) => run.id === "same")?.status).toBe("running");
    expect(state.filteredRuns.value.map((run) => run.id)).toEqual(["new", "same"]);

    apiGet.mockRejectedValueOnce(new Error("backend starting"));
    await expect(state.loadRuns()).resolves.toBeUndefined();
  });

  it("prefers richer duplicate runs when timestamps do not decide the merge", async () => {
    const minimalResult = {
      symbol: "US.AAPL",
      interval: "5m",
      startTime: "2026-06-01T00:00:00Z",
      endTime: "2026-06-30T00:00:00Z",
      finalBalance: 100001,
      pnl: 1,
      totalTrades: 1,
      winRate: 1,
    };
    apiGet.mockResolvedValue({
      runs: [
        makeRun("prefer-result", { status: "running", updatedAt: "invalid-date" }),
        makeRun("prefer-result", { status: "running", updatedAt: "invalid-date", result: minimalResult }),
        makeRun("keep-result", { status: "running", updatedAt: "invalid-date", result: minimalResult }),
        makeRun("keep-result", { status: "running", updatedAt: "invalid-date" }),
        makeRun("prefer-completed", { status: "running", updatedAt: "2026-07-01T00:00:00Z" }),
        makeRun("prefer-completed", { status: "completed", updatedAt: "2026-07-01T00:00:00Z" }),
      ],
    });
    const { state } = mountBacktestRuns();

    await state.loadRuns();

    expect(state.runs.value.find((run) => run.id === "prefer-result")?.result).toBeDefined();
    expect(state.runs.value.find((run) => run.id === "keep-result")?.result).toBeDefined();
    expect(state.runs.value.find((run) => run.id === "prefer-completed")?.status).toBe("completed");
  });

  it("loads missing details once and reports detail failures", async () => {
    const { state } = mountBacktestRuns();
    queryClient.setQueryData(queryKeys.backtestRuns(), [makeRun("run-1", { status: "running" })]);
    await nextTick();
    apiGetPath.mockResolvedValueOnce(makeRun("run-1", {
      status: "completed",
      updatedAt: "2026-07-02T00:00:00Z",
      result: { symbol: "US.AAPL", interval: "5m", startTime: "", endTime: "", finalBalance: 1, pnl: 0, totalTrades: 0, winRate: 0 },
    }));

    await state.toggleRun("run-1");
    expect(state.expandedRuns["run-1"]).toBe(true);
    expect(state.detailLoading["run-1"]).toBe(false);
    expect(state.runs.value[0]?.result).toBeDefined();

    await state.toggleRun("run-1");
    expect(apiGetPath).toHaveBeenCalledTimes(1);

    queryClient.setQueryData(queryKeys.backtestRuns(), [makeRun("run-2", { status: "running" })]);
    apiGetPath.mockRejectedValueOnce(new Error("detail unavailable"));
    await state.toggleRun("run-2");
    expect(state.detailErrors["run-2"]).toContain("detail unavailable");

    state.detailLoading["run-2"] = true;
    await state.toggleRun("run-2");
    expect(apiGetPath).toHaveBeenCalledTimes(2);
  });

  it("removes terminal runs only after the server confirms deletion", async () => {
    const { state } = mountBacktestRuns();
    queryClient.setQueryData(queryKeys.backtestRuns(), [
      makeRun("done"),
      makeRun("running", { status: "running" }),
      makeRun("failed", { status: "failed" }),
    ]);
    state.expandedRuns.done = true;
    apiDeletePath.mockResolvedValue({ deleted: true, id: "done" });

    await expect(state.deleteRun(" ")).resolves.toBe(false);
    await expect(state.deleteRun("running")).resolves.toBe(false);
    expect(apiDeletePath).not.toHaveBeenCalled();
    await expect(state.deleteRun("done")).resolves.toBe(true);
    expect(apiDeletePath).toHaveBeenCalledWith(
      "/api/v1/backtests/{runId}",
      "/api/v1/backtests/done",
    );
    expect(state.expandedRuns.done).toBeUndefined();

    apiDeletePath.mockRejectedValueOnce(new Error("delete unsupported"));
    await expect(state.deleteRun("failed")).resolves.toBe(false);
    expect(state.runs.value.map((run) => run.id)).toEqual(["running", "failed"]);
    expect(state.error.value).toContain("delete unsupported");

    apiDeletePath.mockResolvedValueOnce({ deleted: false, id: "failed" });
    await expect(state.deleteRun("failed")).resolves.toBe(false);
    expect(state.runs.value.map((run) => run.id)).toEqual(["running", "failed"]);
    expect(state.error.value).toContain("服务端未确认删除");
  });

  it("validates, starts, reports, and cancels K-line synchronization", async () => {
    const invalid = mountBacktestRuns({
      normalizeInstrument: vi.fn(async () => ({ market: "", prefix: "", code: "", instrumentId: "" })),
    });
    await invalid.state.syncKlines();
    expect(invalid.state.error.value).toContain("有效的市场与代码");
    expect(startSync).not.toHaveBeenCalled();

    const valid = mountBacktestRuns({ form: { useExtendedHours: true } });
    syncError.value = "sync rejected";
    await valid.state.syncKlines();
    expect(startSync).toHaveBeenCalledWith(expect.objectContaining({
      market: "US",
      code: "AAPL",
      symbol: "US.AAPL",
      sessionScope: "extended",
    }));
    expect(valid.state.error.value).toBe("sync rejected");

    valid.normalizeInstrument.mockRejectedValueOnce("network down");
    await valid.state.syncKlines();
    expect(valid.state.error.value).toContain("network down");

    syncProgress.value = { status: "running" };
    await valid.state.cancelSync();
    expect(cancelKlineSync).toHaveBeenCalledOnce();
    expect(syncProgress.value.status).toBe("cancelled");
    syncProgress.value = null;
    await valid.state.cancelSync();
  });

  it("starts a backtest, patches polled status, and preserves unrelated runs", async () => {
    vi.useFakeTimers();
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue(undefined);
    apiPost.mockResolvedValue({ id: "run-new", status: "queued" });
    apiGet
      .mockResolvedValueOnce({ runs: [makeRun("run-new", { status: "queued" }), makeRun("other")] })
      .mockResolvedValueOnce({ runs: [makeRun("run-new", { status: "completed" }), makeRun("other")] });
    apiGetPath
      .mockResolvedValueOnce({ id: "run-new", status: "running" })
      .mockResolvedValueOnce({ id: "run-new", status: "completed" });
    const { state } = mountBacktestRuns();

    const start = state.startBacktest();
    await vi.runAllTicks();
    await start;
    expect(apiPost).toHaveBeenCalledWith(
      "/api/v1/backtests",
      expect.objectContaining({ definitionId: "def-1", symbol: "US.AAPL" }),
    );
    expect(state.running.value).toBe(false);

    await vi.advanceTimersByTimeAsync(4000);
    for (let index = 0; index < 5; index += 1) await Promise.resolve();
    await vi.advanceTimersByTimeAsync(0);
    await nextTick();
    expect(apiGetPath).toHaveBeenCalledWith(
      "/api/v1/backtests/{runId}/status",
      "/api/v1/backtests/run-new/status",
    );
    expect(apiGet).toHaveBeenCalledTimes(2);
    const cachedRuns = queryClient.getQueryData<Array<{ id: string; status: string }>>(queryKeys.backtestRuns());
    expect(cachedRuns?.find((run) => run.id === "run-new")?.status).toBe("completed");
    expect(cachedRuns?.find((run) => run.id === "other")?.status).toBe("completed");
  });

  it("handles missing definitions, invalid instruments, start errors, and polling exhaustion", async () => {
    const missingDefinition = mountBacktestRuns({ form: { definitionId: "" } });
    await missingDefinition.state.startBacktest();
    expect(apiPost).not.toHaveBeenCalled();

    const invalid = mountBacktestRuns({
      normalizeInstrument: vi.fn(async () => ({ market: "US", prefix: "", code: "", instrumentId: "" })),
    });
    await invalid.state.startBacktest();
    expect(invalid.state.error.value).toContain("有效的市场与代码");
    expect(invalid.state.running.value).toBe(false);

    const failed = mountBacktestRuns();
    apiPost.mockRejectedValueOnce("start unavailable");
    await failed.state.startBacktest();
    expect(failed.state.error.value).toContain("start unavailable");

    vi.useFakeTimers();
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue(undefined);
    apiPost.mockResolvedValueOnce({ id: "run-failing", status: "queued" });
    apiGet.mockResolvedValue({ runs: [makeRun("run-failing", { status: "queued" })] });
    apiGetPath.mockRejectedValue(new Error("status unavailable"));
    const polling = mountBacktestRuns();
    await polling.state.startBacktest();
    await vi.advanceTimersByTimeAsync(6000);
    expect(polling.state.error.value).toContain("status unavailable");
    expect(apiGetPath).toHaveBeenCalledTimes(3);
  });

  it("clears a pending status poll when its component unmounts", async () => {
    vi.useFakeTimers();
    apiPost.mockResolvedValueOnce({ id: "run-unmounted", status: "queued" });
    apiGet.mockResolvedValue({ runs: [makeRun("run-unmounted", { status: "queued" })] });
    apiGetPath.mockResolvedValue({ id: "run-unmounted", status: "running" });
    const mounted = mountBacktestRuns();

    await mounted.state.startBacktest();
    mounted.wrapper.unmount();
    await vi.advanceTimersByTimeAsync(10_000);

    expect(apiGetPath).not.toHaveBeenCalled();
  });
});
