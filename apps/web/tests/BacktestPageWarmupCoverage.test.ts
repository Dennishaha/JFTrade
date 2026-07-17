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
import BacktestPage from "../src/pages/BacktestPage.vue";
import { queryClient, queryKeys } from "../src/composables/serverState";
import { createResponse, flushRequests, mountApp } from "./helpers";

interface PendingWarmupRequest {
  interval: string;
  reject: (error: unknown) => void;
  resolve: (response: Response) => void;
}

let installedBacktestFetch: ReturnType<typeof vi.fn> | null = null;

afterEach(() => {
  installedBacktestFetch = null;
  vi.unstubAllGlobals();
  window.localStorage.clear();
  window.sessionStorage.clear();
});

function readSetup<T>(value: unknown): T {
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T }).value;
  }
  return value as T;
}

function writeSetup(setup: Record<string, unknown>, key: string, value: unknown): void {
  const current = setup[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: unknown }).value = value;
    return;
  }
  setup[key] = value;
}

function installBacktestFetch(): PendingWarmupRequest[] {
  const pending: PendingWarmupRequest[] = [];
  const fetchMock = vi.fn((input: string | URL | Request) => {
    const url = String(input);
    if (url.includes("/api/v1/system/status")) return createResponse(emptySystemStatus);
    if (url.includes("/api/v1/system/storage/overview")) return createResponse(emptyStorageOverview);
    if (url.includes("/api/v1/settings/onboarding")) return createResponse(emptyOnboardingState);
    if (url.includes("/api/v1/settings/brokers")) return createResponse(emptyBrokerSettings);
    if (url.includes("/api/v1/plugins")) return createResponse(emptyPluginCatalog);
    if (url.includes("/api/v1/market-data/subscriptions")) return createResponse(emptyMarketDataSubscriptions);
    if (url.includes("/api/v1/market-data/markets")) {
      return createResponse({
        defaultMarket: "HK",
        markets: [{
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
        }],
      });
    }
    if (url.includes("/api/v1/strategy-definitions/") && url.includes("?")) {
      const interval = new URL(url, "http://localhost").searchParams.get("interval") ?? "5m";
      return new Promise<Response>((resolve, reject) => {
        pending.push({ interval, resolve, reject });
      });
    }
    if (url.includes("/api/v1/strategy-definitions")) {
      return createResponse([{
        id: "strategy-1",
        name: "Warmup Strategy",
        version: "v1",
        symbol: "HK.00700",
        interval: "5m",
      }]);
    }
    if (url.includes("/api/v1/backtests")) return createResponse({ runs: [] });
    if (url.includes("/api/v1/market-data/instruments/normalize")) {
      return createResponse({ market: "HK", prefix: "HK", code: "00700", instrumentId: "HK.00700" });
    }
    return createResponse({});
  });
  installedBacktestFetch = fetchMock;
  vi.stubGlobal("fetch", fetchMock);
  return pending;
}

function settleWarmupInterval(
  pending: PendingWarmupRequest[],
  interval: string,
  detail: Record<string, unknown>,
): void {
  for (let index = pending.length - 1; index >= 0; index -= 1) {
    const request = pending[index];
    if (request?.interval !== interval) continue;
    pending.splice(index, 1);
    request.resolve(createResponse(detail));
  }
}

function rejectWarmupInterval(
  pending: PendingWarmupRequest[],
  interval: string,
): void {
  for (let index = pending.length - 1; index >= 0; index -= 1) {
    const request = pending[index];
    if (request?.interval !== interval) continue;
    pending.splice(index, 1);
    request.reject(new Error(`warmup ${interval} unavailable`));
  }
}

describe("BacktestPage warmup preview race handling", () => {
  it("never lets an earlier preview response or failure replace the latest backtest interval", async () => {
    const defaultOptions = queryClient.getDefaultOptions();
    queryClient.setDefaultOptions({
      ...defaultOptions,
      queries: { ...defaultOptions.queries, retry: false },
    });
    const pending = installBacktestFetch();
    const { wrapper } = await mountApp("/backtest");
    try {
      await nextTick();
      settleWarmupInterval(pending, "5m", {
        derivedWarmupBars: 12,
        derivedWarmupInterval: "5m",
      });
      await flushRequests();

      const page = wrapper.getComponent(BacktestPage);
      const setup = page.vm.$.setupState as Record<string, unknown>;
      const loadWarmupPreview = setup.loadWarmupPreview as () => Promise<void>;

      writeSetup(setup, "interval", "1m");
      void loadWarmupPreview();
      writeSetup(setup, "interval", "15m");
      void loadWarmupPreview();
      settleWarmupInterval(pending, "1m", {
        derivedWarmupBars: 999,
        derivedWarmupInterval: "1m",
      });
      await nextTick();
      settleWarmupInterval(pending, "15m", {
        derivedWarmupBars: 36,
        derivedWarmupInterval: "15m",
      });
      await flushRequests();
      expect(readSetup<number | null>(setup.warmupPreviewBars)).toBe(36);
      expect(readSetup<string>(setup.warmupPreviewInterval)).toBe("15m");

      writeSetup(setup, "interval", "30m");
      void loadWarmupPreview();
      writeSetup(setup, "interval", "1h");
      void loadWarmupPreview();
      rejectWarmupInterval(pending, "30m");
      await nextTick();
      settleWarmupInterval(pending, "1h", {
        derivedWarmupBars: 48,
        derivedWarmupInterval: "1h",
      });
      await flushRequests();
      expect(readSetup<number | null>(setup.warmupPreviewBars)).toBe(48);
      expect(readSetup<string>(setup.warmupPreviewInterval)).toBe("1h");

      writeSetup(setup, "interval", "2h");
      void loadWarmupPreview();
      await flushRequests();
      expect(pending.some((request) => request.interval === "2h")).toBe(true);
      rejectWarmupInterval(pending, "2h");
      await flushRequests();
      expect(readSetup<number | null>(setup.warmupPreviewBars)).toBeNull();
      expect(readSetup<string>(setup.warmupPreviewInterval)).toBe("2h");
    } finally {
      wrapper.unmount();
      queryClient.setDefaultOptions(defaultOptions);
    }
  });

  it("keeps manually opened setup state and clamps result pagination when history changes", async () => {
    const pending = installBacktestFetch();
    const { wrapper } = await mountApp("/backtest");
    try {
      settleWarmupInterval(pending, "5m", {
        derivedWarmupBars: 12,
        derivedWarmupInterval: "5m",
      });
      await flushRequests();

      const page = wrapper.getComponent(BacktestPage);
      const setup = page.vm.$.setupState as Record<string, unknown>;
      const run = (id: string) => ({
        id,
        status: "completed",
        createdAt: "2026-07-03T00:00:00Z",
        updatedAt: "2026-07-03T00:00:00Z",
        request: {
          definitionId: "strategy-1",
          symbol: "HK.00700",
          market: "HK",
          code: "00700",
          interval: "5m",
          initialBalance: 100_000,
        },
        result: {
          symbol: "HK.00700",
          interval: "5m",
          startTime: "2026-07-01T00:00:00Z",
          endTime: "2026-07-03T00:00:00Z",
          finalBalance: 100_000,
          pnl: 0,
          totalTrades: 0,
          winRate: 0,
        },
      });
      const setRuns = (runs: unknown[]) => {
        queryClient.setQueryData(queryKeys.backtestRuns(), runs);
      };

      setRuns(Array.from({ length: 24 }, (_, index) => run(`run-${index}`)));
      await nextTick();
      writeSetup(setup, "resultsPage", 99);
      setRuns([run("run-only")]);
      await nextTick();
      expect(readSetup<number>(setup.resultsPage)).toBe(
        readSetup<number>(setup.resultsPageCount),
      );

      writeSetup(setup, "resultsPage", 0);
      setRuns([run("run-one"), run("run-two")]);
      await nextTick();
      expect(readSetup<number>(setup.resultsPage)).toBe(1);

      writeSetup(setup, "newBacktestFormTouched", true);
      writeSetup(setup, "showNewBacktestForm", false);
      setRuns([]);
      await nextTick();
      expect(readSetup<boolean>(setup.showNewBacktestForm)).toBe(false);
    } finally {
      wrapper.unmount();
    }
  });

  it("renders cancelled sync progress and removes a terminal report from both result entry points", async () => {
    const pending = installBacktestFetch();
    const { wrapper } = await mountApp("/backtest");
    try {
      settleWarmupInterval(pending, "5m", {
        derivedWarmupBars: 12,
        derivedWarmupInterval: "5m",
      });
      await flushRequests();

      const page = wrapper.getComponent(BacktestPage);
      const setup = page.vm.$.setupState as Record<string, unknown>;
      writeSetup(setup, "syncing", true);
      writeSetup(setup, "syncProgress", {
        taskId: "sync-cancelled",
        status: "running",
        symbol: "HK.00700",
        currentInterval: "5m",
        totalIntervals: 2,
        completedIntervals: 1,
        totalBatches: 4,
        completedBatches: 2,
        retries: 1,
        startedAt: "2026-07-03T00:00:00Z",
        updatedAt: "2026-07-03T00:01:00Z",
      });
      await nextTick();

      expect(wrapper.text()).toContain("同步中 · 5m");
      expect(wrapper.text()).toContain("重试 1");
      const cancelButton = wrapper
        .findAll("button")
        .find((button) => button.text().trim() === "取消");
      expect(cancelButton).toBeDefined();
      await cancelButton!.trigger("click");
      await nextTick();
      expect(readSetup<{ status: string } | null>(setup.syncProgress)).toMatchObject({
        status: "cancelled",
      });
      // A terminal live progress event clears the active-sync flag after the
      // local cancellation request has been accepted.
      writeSetup(setup, "syncing", false);
      await nextTick();
      expect(wrapper.text()).toContain("同步已取消 · 2 批已完成");

      const terminalRun = {
        id: "run-delete",
        status: "completed",
        createdAt: "2026-07-03T00:00:00Z",
        updatedAt: "2026-07-03T00:00:00Z",
        request: {
          definitionId: "strategy-1",
          definitionVersion: "v1",
          symbol: "HK.00700",
          market: "HK",
          code: "00700",
          interval: "5m",
          startDate: "2026-07-01T00:00:00Z",
          endDate: "2026-07-03T00:00:00Z",
          initialBalance: 100_000,
        },
        result: {
          symbol: "HK.00700",
          interval: "5m",
          startTime: "2026-07-01T00:00:00Z",
          endTime: "2026-07-03T00:00:00Z",
          finalBalance: 101_000,
          pnl: 1_000,
          totalTrades: 1,
          winRate: 1,
        },
      };
      queryClient.setQueryData(queryKeys.backtestRuns(), [terminalRun]);
      await nextTick();

      // Both the history row and report header expose the same terminal-result
      // removal action. The shared Vuetify stub intentionally does not forward
      // its native click event, so invoke the bound page action after asserting
      // the two user entry points are present.
      expect(wrapper.findAll("button[title='删除回测结果']")).toHaveLength(2);
      await (setup.deleteRun as (runID: string) => Promise<void>)("run-delete");
      await flushRequests();

      expect(queryClient.getQueryData(queryKeys.backtestRuns())).toEqual([]);
      expect(installedBacktestFetch?.mock.calls.some(([input, init]) =>
        String(input).includes("/api/v1/backtests/run-delete") &&
        (init as RequestInit | undefined)?.method === "DELETE",
      )).toBe(true);
    } finally {
      wrapper.unmount();
    }
  });
});
