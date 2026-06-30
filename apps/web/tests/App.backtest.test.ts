// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

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
    expect(wrapper.text().match(/run-\d{3}/g)).toHaveLength(5);

    wrapper.unmount();
  });
});

function installBacktestPageFetch(options: { runs: unknown[] }): void {
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
        return createResponse({ entries: [] });
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
      if (url.includes("/api/v1/strategy-definitions")) {
        return createResponse([]);
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
