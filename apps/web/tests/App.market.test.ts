// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrders,
  emptyMarketDataSubscriptions,
  emptyPortfolioCashBalances,
  emptyPortfolioCashReconciliation,
  emptyPortfolioPositions,
  emptyPortfolioReconciliation,
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
  emptyStorageOverview,
  emptySystemStatus,
  emptyWorkerBrokerOrderUpdates,
} from "@/contracts";

import {
  MockWebSocket,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
  window.sessionStorage?.clear();
  window.localStorage?.clear();
  document.title = "";
});

function buildStandardFetchMock(overrides: Record<string, unknown> = {}) {
  return vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);

    if (url.includes("/api/v1/market-data/subscriptions")) {
      if (init?.method === "POST") {
        return createResponse(
          overrides["market-data/subscriptions:post"] ?? {
            totalActiveSubscriptions: 1,
            quota: {
              totalUsed: 1,
              totalLimit: 500,
              totalRemaining: 499,
              byMarket: [
                { market: "HK", used: 1, limit: null, remaining: null },
              ],
            },
            entries: [
              {
                key: "SNAPSHOT:HK.00700",
                channel: "SNAPSHOT",
                market: "HK",
                symbol: "00700",
                instrumentId: "HK.00700",
                interval: null,
                depthLevel: null,
                consumers: ["web:market-page:test"],
                refCount: 1,
                createdAt: "2026-05-17T00:00:00.000Z",
                updatedAt: "2026-05-17T00:01:00.000Z",
              },
            ],
          },
        );
      }

      return createResponse(
        overrides["market-data/subscriptions"] ?? emptyMarketDataSubscriptions,
      );
    }
    if (url.includes("/api/v1/market-data/snapshots/HK/00700")) {
      return createResponse({
        request: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        snapshot: {
          price: 320.5,
          bid: 320.4,
          ask: 320.6,
          openPrice: 319.8,
          highPrice: 321,
          lowPrice: 319.6,
          previousClosePrice: 318.9,
          volume: 1280000,
          turnover: 410240000,
          at: "2026-05-17T01:30:00.000Z",
        },
        meta: {
          instrumentId: "HK.00700",
          source: "api-sample-cache",
          resolvedAt: "2026-05-17T01:30:00.000Z",
          fromCache: true,
        },
      });
    }
    if (url.includes("/api/v1/market-data/securities/HK/00700")) {
      return createResponse({
        request: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        security: null,
        meta: {
          instrumentId: "HK.00700",
          source: "api-sample-cache",
          resolvedAt: "2026-05-17T01:30:00.000Z",
          fromCache: true,
        },
      });
    }
    if (url.includes("/api/v1/market-data/candles/HK/00700")) {
      return createResponse({
        request: {
          instrument: {
            market: "HK",
            symbol: "00700",
            instrumentId: "HK.00700",
          },
          period: "1m",
          limit: 3,
        },
        candles: [
          {
            period: "1m",
            open: 320,
            high: 320.8,
            low: 319.9,
            close: 320.5,
            volume: 18000,
            at: "2026-05-17T01:29:00.000Z",
          },
        ],
        totalReturned: 1,
        meta: {
          instrumentId: "HK.00700",
          source: "api-sample-cache",
          resolvedAt: "2026-05-17T01:30:00.000Z",
          fromCache: true,
        },
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
            displayName: "China",
            quoteCurrency: "CNY",
            supportsExtendedHours: false,
            requiresExchangePrefix: true,
            aliases: [],
            regularSessions: [],
            precision: { price: 2, quote: 2 },
            tickSize: 0.01,
          },
          {
            code: "SH",
            resolvedMarket: "CN",
            preferredPrefix: "SH",
            displayName: "Shanghai",
            quoteCurrency: "CNY",
            supportsExtendedHours: false,
            requiresExchangePrefix: true,
            aliases: ["CNSH"],
            regularSessions: [],
            precision: { price: 2, quote: 2 },
            tickSize: 0.01,
          },
          {
            code: "SZ",
            resolvedMarket: "CN",
            preferredPrefix: "SZ",
            displayName: "Shenzhen",
            quoteCurrency: "CNY",
            supportsExtendedHours: false,
            requiresExchangePrefix: true,
            aliases: ["CNSZ"],
            regularSessions: [],
            precision: { price: 2, quote: 2 },
            tickSize: 0.01,
          },
        ],
      });
    }
    if (url.includes("/api/v1/market-data/instruments/normalize")) {
      const body = JSON.parse(String(init?.body ?? "{}")) as {
        market?: string;
        code?: string;
        symbol?: string;
        instrumentId?: string;
      };
      const rawInstrument = (
        body.instrumentId ??
        body.symbol ??
        body.code ??
        ""
      ).trim().toUpperCase().replace(":", ".");
      const embedded = rawInstrument.includes(".")
        ? rawInstrument.split(".", 2)
        : null;
      const market = (embedded?.[0] ?? body.market ?? "HK").trim().toUpperCase();
      const code = (embedded?.[1] ?? rawInstrument).trim().toUpperCase();
      const prefix = market === "CN" ? "" : market;
      if (prefix === "") {
        return {
          ok: false,
          status: 400,
          statusText: "Bad Request",
          json: async () => ({
            ok: false,
            error: {
              code: "MARKET_INSTRUMENT_INVALID",
              message: "market \"CN\" requires an exchange-qualified symbol",
            },
            timestamp: "2026-06-12T00:00:00.000Z",
          }),
        } as Response;
      }
      return createResponse({
        market: market === "SH" || market === "SZ" ? "CN" : market,
        prefix,
        code,
        symbol: `${prefix}.${code}`,
        instrumentId: `${prefix}.${code}`,
        resolvedMarket: market === "SH" || market === "SZ" ? "CN" : market,
      });
    }
    if (url.includes("/api/v1/market-data/instruments")) {
      return createResponse({
        query:
          new URL(String(url), "http://127.0.0.1:3000").searchParams.get(
            "query",
          ) ?? "",
        totalReturned: 1,
        entries: [
          {
            market: "HK",
            symbol: "00700",
            instrumentId: "HK.00700",
            name: "Tencent Holdings",
            securityType: "STOCK",
            lotSize: 100,
            exchange: "HKEX",
            status: "NORMAL",
            source: "seed",
            updatedAt: "2026-05-17T00:00:00.000Z",
            brokerMappings: [
              {
                brokerId: "futu",
                brokerMarket: "HK",
                brokerSymbol: "00700",
                brokerInstrumentId: "HK.00700",
                displayName: "腾讯控股",
                source: "seed",
                updatedAt: "2026-05-17T00:00:00.000Z",
              },
            ],
          },
        ],
      });
    }
    if (url.includes("/api/v1/system/status"))
      return createResponse(emptySystemStatus);
    if (url.includes("/api/v1/system/storage/overview"))
      return createResponse(emptyStorageOverview);
    if (url.includes("/api/v1/system/real-trade-approvals"))
      return createResponse(emptyRealTradeApprovals);
    if (url.includes("/api/v1/system/real-trade-hard-stops"))
      return createResponse(emptyRealTradeHardStops);
    if (url.includes("/api/v1/system/real-trade-hard-stop-events"))
      return createResponse(emptyRealTradeHardStopEvents);
    if (url.includes("/api/v1/system/real-trade-kill-switch-events"))
      return createResponse(emptyRealTradeKillSwitchEvents);
    if (url.includes("/api/v1/system/real-trade-kill-switch"))
      return createResponse(emptyRealTradeKillSwitchState);
    if (url.includes("/api/v1/system/real-trade-risk-events"))
      return createResponse(emptyRealTradeRiskEvents);
    if (url.includes("/api/v1/system/real-trade-risk-limits"))
      return createResponse(emptyRealTradeRiskState);
    if (url.includes("/api/v1/system/worker/broker-order-updates"))
      return createResponse(emptyWorkerBrokerOrderUpdates);
    if (url.includes("/api/v1/brokers/futu/runtime"))
      return createResponse(emptyBrokerRuntime);
    if (url.includes("/api/v1/brokers/futu/funds"))
      return createResponse(emptyBrokerFunds);
    if (url.includes("/api/v1/brokers/futu/positions"))
      return createResponse(emptyBrokerPositions);
    if (url.includes("/api/v1/brokers/futu/orders"))
      return createResponse(emptyBrokerOrders);
    if (url.includes("/api/v1/portfolio/futu/cash-balances"))
      return createResponse(emptyPortfolioCashBalances);
    if (url.includes("/api/v1/portfolio/futu/positions"))
      return createResponse(emptyPortfolioPositions);
    if (url.includes("/api/v1/portfolio/futu/cash-reconciliation"))
      return createResponse(emptyPortfolioCashReconciliation);
    if (url.includes("/api/v1/portfolio/futu/reconciliation"))
      return createResponse(emptyPortfolioReconciliation);
    if (url.includes("/api/v1/execution/orders"))
      return createResponse(emptyExecutionOrders);

    throw new Error(`Unexpected request: ${url}`);
  });
}

describe("Workspace market behavior", () => {
  it("sets the workspace browser title from the active instrument name", async () => {
    vi.stubGlobal("fetch", buildStandardFetchMock());

    const { wrapper } = await mountApp("/workspace");
    await flushRequests();

    expect(document.title).toBe("HK.00700-Tencent Holdings - JFTrade Console");

    wrapper.unmount();
  });

  it("does not expose removed overview and market nav entries", async () => {
    vi.stubGlobal("fetch", buildStandardFetchMock());
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/workspace");

    expect(wrapper.find('.tv-iconrail-btn[title="概览"]').exists()).toBe(false);
    expect(wrapper.find('.tv-iconrail-btn[title="行情"]').exists()).toBe(false);
    expect(wrapper.find('.tv-iconrail-btn[title="交易"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("策略");
    expect(wrapper.text()).toContain("系统");
    expect(wrapper.text()).toContain("我的账户");

    wrapper.unmount();
  });

});
