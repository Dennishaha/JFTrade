// @vitest-environment jsdom

import { computed, defineComponent, h, nextTick, ref } from "vue";
import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BacktestFormState } from "../src/composables/useBacktestRuns";
import { queryClient, queryKeys } from "../src/composables/serverState";

const mocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  fetchEnvelope: vi.fn(),
  fetchEnvelopeWithInit: vi.fn(),
  startSync: vi.fn(),
  cancelKlineSync: vi.fn(),
  syncing: { value: false },
  syncProgress: { value: null as { status: string } | null },
  syncError: { value: "" },
}));
const {
  apiGet,
  fetchEnvelope,
  fetchEnvelopeWithInit,
  startSync,
  cancelKlineSync,
  syncing,
  syncProgress,
  syncError,
} = mocks;

vi.mock("../src/composables/apiClient", () => ({
  apiGet: mocks.apiGet,
  fetchEnvelope: mocks.fetchEnvelope,
  fetchEnvelopeWithInit: mocks.fetchEnvelopeWithInit,
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

import { useBacktestRuns } from "../src/composables/useBacktestRuns";

const baseForm: BacktestFormState = {
  definitionId: "def-1",
  definitionVersion: "1.0.0",
  market: "US",
  code: "AAPL",
  instrumentId: "US.AAPL",
  instrumentType: "stock",
  interval: "5m",
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
  return { state, form, normalizeInstrument };
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
  it("loads and normalizes full decimal transport results", async () => {
    apiGet.mockResolvedValue({
      runs: [makeRun("run-1", {
        result: {
          symbol: "US.AAPL",
          interval: "5m",
          startTime: "2026-06-01T00:00:00Z",
          endTime: "2026-06-30T00:00:00Z",
          finalBalance: 100010,
          pnl: 10,
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
    expect(result?.trades?.[0]).toMatchObject({
      price: 123.45,
      priceText: "123.4500",
      qty: 0,
      qtyText: "bad-decimal",
      totalFee: 3,
    });
    expect(result?.orderBook?.[0]).toMatchObject({
      quantity: 10,
      quantityText: "10.000",
      filledPrice: undefined,
      filledPriceText: "not-a-number",
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

  it("loads missing details once and reports detail failures", async () => {
    const { state } = mountBacktestRuns();
    queryClient.setQueryData(queryKeys.backtestRuns(), [makeRun("run-1", { status: "running" })]);
    await nextTick();
    fetchEnvelope.mockResolvedValueOnce(makeRun("run-1", {
      status: "completed",
      updatedAt: "2026-07-02T00:00:00Z",
      result: { symbol: "US.AAPL", interval: "5m", startTime: "", endTime: "", finalBalance: 1, pnl: 0, totalTrades: 0, winRate: 0 },
    }));

    await state.toggleRun("run-1");
    expect(state.expandedRuns["run-1"]).toBe(true);
    expect(state.detailLoading["run-1"]).toBe(false);
    expect(state.runs.value[0]?.result).toBeDefined();

    await state.toggleRun("run-1");
    expect(fetchEnvelope).toHaveBeenCalledTimes(1);

    queryClient.setQueryData(queryKeys.backtestRuns(), [makeRun("run-2", { status: "running" })]);
    fetchEnvelope.mockRejectedValueOnce(new Error("detail unavailable"));
    await state.toggleRun("run-2");
    expect(state.detailErrors["run-2"]).toContain("detail unavailable");

    state.detailLoading["run-2"] = true;
    await state.toggleRun("run-2");
    expect(fetchEnvelope).toHaveBeenCalledTimes(2);
  });

  it("deletes terminal runs remotely and always removes local state", async () => {
    const { state } = mountBacktestRuns();
    queryClient.setQueryData(queryKeys.backtestRuns(), [
      makeRun("done"),
      makeRun("running", { status: "running" }),
      makeRun("failed", { status: "failed" }),
    ]);
    state.expandedRuns.done = true;
    fetchEnvelopeWithInit.mockResolvedValue({ deleted: true, id: "done" });

    await state.deleteRun(" ");
    await state.deleteRun("running");
    expect(fetchEnvelopeWithInit).not.toHaveBeenCalled();
    await state.deleteRun("done");
    expect(fetchEnvelopeWithInit).toHaveBeenCalledWith("/api/v1/backtests/done", { method: "DELETE" });
    expect(state.expandedRuns.done).toBeUndefined();

    fetchEnvelopeWithInit.mockRejectedValueOnce(new Error("delete unsupported"));
    await expect(state.deleteRun("failed")).resolves.toBeUndefined();
    expect(state.runs.value.map((run) => run.id)).toEqual([]);
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

  it("starts a backtest and applies a terminal polling update", async () => {
    vi.useFakeTimers();
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue(undefined);
    fetchEnvelopeWithInit.mockResolvedValue({ id: "run-new", status: "queued" });
    apiGet.mockResolvedValue({ runs: [makeRun("run-new", { status: "completed" })] });
    fetchEnvelope.mockResolvedValue({ id: "run-new", status: "completed" });
    const { state } = mountBacktestRuns();

    const start = state.startBacktest();
    await vi.runAllTicks();
    await start;
    expect(fetchEnvelopeWithInit).toHaveBeenCalledWith("/api/v1/backtests", expect.objectContaining({ method: "POST" }));
    expect(state.running.value).toBe(false);

    await vi.advanceTimersByTimeAsync(2000);
    for (let index = 0; index < 5; index += 1) await Promise.resolve();
    await vi.advanceTimersByTimeAsync(0);
    await nextTick();
    expect(fetchEnvelope).toHaveBeenCalledWith("/api/v1/backtests/run-new/status");
    expect(apiGet).toHaveBeenCalledTimes(2);
    const cachedRuns = queryClient.getQueryData<Array<{ id: string; status: string }>>(queryKeys.backtestRuns());
    expect(cachedRuns?.find((run) => run.id === "run-new")?.status).toBe("completed");
  });

  it("handles missing definitions, invalid instruments, start errors, and polling exhaustion", async () => {
    const missingDefinition = mountBacktestRuns({ form: { definitionId: "" } });
    await missingDefinition.state.startBacktest();
    expect(fetchEnvelopeWithInit).not.toHaveBeenCalled();

    const invalid = mountBacktestRuns({
      normalizeInstrument: vi.fn(async () => ({ market: "US", prefix: "", code: "", instrumentId: "" })),
    });
    await invalid.state.startBacktest();
    expect(invalid.state.error.value).toContain("有效的市场与代码");
    expect(invalid.state.running.value).toBe(false);

    const failed = mountBacktestRuns();
    fetchEnvelopeWithInit.mockRejectedValueOnce("start unavailable");
    await failed.state.startBacktest();
    expect(failed.state.error.value).toContain("start unavailable");

    vi.useFakeTimers();
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue(undefined);
    fetchEnvelopeWithInit.mockResolvedValueOnce({ id: "run-failing", status: "queued" });
    apiGet.mockResolvedValue({ runs: [makeRun("run-failing", { status: "queued" })] });
    fetchEnvelope.mockRejectedValue(new Error("status unavailable"));
    const polling = mountBacktestRuns();
    await polling.state.startBacktest();
    await vi.advanceTimersByTimeAsync(6000);
    expect(polling.state.error.value).toContain("status unavailable");
    expect(fetchEnvelope).toHaveBeenCalledTimes(3);
  });
});
