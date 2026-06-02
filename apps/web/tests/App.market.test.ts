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
} from "@jftrade/ui-contracts";
import type { MarketDataSubscriptionsResponse } from "@jftrade/ui-contracts";

import {
  MockEventSource,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

function findLiveEventStream(): MockEventSource | undefined {
  return MockEventSource.instances.find((instance) =>
    instance.url.includes("/api/v1/stream/live"),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
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

describe("Market page", () => {
  it("updates the global workspace instrument from the top bar with explicit market and code fields", async () => {
    vi.stubGlobal("fetch", buildStandardFetchMock());
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");

    await wrapper.get(".tv-rightdock-toggle").trigger("click");
    await wrapper.get('[data-testid="rightdock-tab-context"]').trigger("click");
    await flushRequests();

    expect(wrapper.get('[data-testid="rightdock-symbol-info"]').text()).toBe("HK:00700");

    await wrapper.get('[data-testid="topbar-instrument-market"]').setValue("US");
    await wrapper.get('[data-testid="topbar-instrument-code"]').setValue("TME");
    await wrapper.get('[data-testid="topbar-instrument-form"]').trigger("submit");
    await flushRequests();

    expect(wrapper.get('[data-testid="rightdock-symbol-info"]').text()).toBe("US:TME");

    wrapper.unmount();
  });

  it("shows empty state when no subscriptions are active", async () => {
    vi.stubGlobal("fetch", buildStandardFetchMock());
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");

    expect(wrapper.text()).toContain("行情订阅");
    expect(wrapper.text()).toContain("订阅配额");
    expect(wrapper.text()).toContain("实时订阅已改为动态池");
    expect(wrapper.text()).toContain("HK.00700");
    expect(wrapper.text()).toContain("Tencent Holdings");
    expect(wrapper.text()).toContain("320.5");
    expect(wrapper.find(".kline-chart-shell").exists()).toBe(true);

    wrapper.unmount();
  });

  it("renders active subscriptions and quota when API returns data", async () => {
    const marketData: MarketDataSubscriptionsResponse = {
      totalActiveSubscriptions: 2,
      quota: {
        totalUsed: 2,
        totalLimit: 100,
        totalRemaining: 98,
        byMarket: [{ market: "HK", used: 2, limit: 100, remaining: 98 }],
      },
      entries: [
        {
          key: "futu:HK:00700.HK:quote",
          channel: "quote",
          market: "HK",
          symbol: "00700.HK",
          instrumentId: "HK-00700",
          interval: null,
          depthLevel: null,
          consumers: ["strategy-momentum"],
          refCount: 1,
          createdAt: "2026-05-17T00:00:00.000Z",
          updatedAt: "2026-05-17T00:01:00.000Z",
        },
        {
          key: "futu:HK:09988.HK:quote",
          channel: "quote",
          market: "HK",
          symbol: "09988.HK",
          instrumentId: "HK-09988",
          interval: null,
          depthLevel: null,
          consumers: ["strategy-momentum"],
          refCount: 1,
          createdAt: "2026-05-17T00:00:00.000Z",
          updatedAt: "2026-05-17T00:01:00.000Z",
        },
      ],
    };

    vi.stubGlobal(
      "fetch",
      buildStandardFetchMock({
        "market-data/subscriptions": marketData,
        "market-data/subscriptions:post": marketData,
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");

    expect(wrapper.text()).toContain("HK.00700");
    expect(wrapper.text()).toContain("09988.HK");
    expect(wrapper.text()).toContain("HK");
    expect(wrapper.text()).toContain("行情查询结果");
    expect(wrapper.text()).toContain("近期K线");
    expect(wrapper.text()).toContain("历史K线价格");
    expect(wrapper.text()).toContain("开盘 → 收盘");
    expect(wrapper.text()).toContain("Tencent Holdings");
    expect(wrapper.text()).toContain("最新报价");
    expect(wrapper.text()).toContain("+1.600");
    expect(wrapper.text()).toContain("实时通道：已连接");
    expect(wrapper.text()).toContain("Tick");
    expect(wrapper.find(".kline-chart-shell").exists()).toBe(true);

    wrapper.unmount();
  });

  it("shows nav items for all six console sections", async () => {
    vi.stubGlobal("fetch", buildStandardFetchMock());
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");

    expect(wrapper.text()).toContain("行情");
    expect(wrapper.text()).toContain("策略");
    expect(wrapper.text()).toContain("系统");
    expect(wrapper.text()).toContain("我的账户");
    expect(wrapper.text()).toContain("风控");
    expect(wrapper.text()).toContain("交易概览");

    wrapper.unmount();
  });

  it("automatically acquires and releases the visible instrument subscription", async () => {
    const fetchMock = buildStandardFetchMock();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");

    await flushRequests();

    const acquireCalls = fetchMock.mock.calls.filter(
      ([url, init]) =>
        String(url).includes("/api/v1/market-data/subscriptions") &&
        !String(url).includes("/release") &&
        init?.method === "POST",
    );
    expect(acquireCalls.length).toBeGreaterThanOrEqual(2);
    const acquireBodies = acquireCalls.map(
      ([, init]) => JSON.parse(String(init?.body)) as Record<string, unknown>,
    );
    const acquireBody = acquireBodies.find(
      (body) => body.channel === "SNAPSHOT",
    );
    const tickAcquireBody = acquireBodies.find(
      (body) => body.channel === "TICK",
    );
    expect(acquireBody).toMatchObject({
      market: "HK",
      symbol: "00700",
      channel: "SNAPSHOT",
    });
    expect(tickAcquireBody).toMatchObject({
      market: "HK",
      symbol: "00700",
      channel: "TICK",
    });
    expect(acquireBody?.consumerId).toEqual(
      expect.stringMatching(/^web:market-page:/),
    );
    expect(wrapper.text()).toContain("HK.00700");

    wrapper.unmount();
    await flushRequests();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/market-data/subscriptions/release"),
      expect.objectContaining({
        method: "POST",
        keepalive: true,
      }),
    );
  });

  it("refreshes snapshot and candles through the broker when the market page loads", async () => {
    const fetchMock = buildStandardFetchMock();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");

    await flushRequests();

    expect(
      fetchMock.mock.calls.some(([url]) =>
        String(url).includes(
          "/api/v1/market-data/snapshots/HK/00700?refresh=true",
        ),
      ),
    ).toBe(true);
    expect(
      fetchMock.mock.calls.some(([url]) =>
        String(url).includes(
          "/api/v1/market-data/candles/HK/00700?period=1m&limit=500&refresh=true",
        ),
      ),
    ).toBe(true);
    expect(
      fetchMock.mock.calls.some(([url]) =>
        String(url).includes("/api/v1/market-data/instruments?limit=50"),
      ),
    ).toBe(true);

    wrapper.unmount();
  });

  it("keeps rendering candles when snapshot refresh fails", async () => {
    const standardFetch = buildStandardFetchMock();
    const fetchMock = vi.fn(
      async (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/api/v1/market-data/snapshots/HK/00700")) {
          return {
            ok: false,
            json: async () => ({
              ok: false,
              error: {
                code: "SNAPSHOT_REFRESH_FAILED",
                message: "Snapshot refresh failed",
              },
              timestamp: "2026-05-17T01:30:00.000Z",
            }),
          } as Response;
        }

        return standardFetch(input, init);
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");
    await flushRequests();

    expect(wrapper.text()).toContain("Snapshot refresh failed");
    expect(wrapper.text()).toContain("320.5");
    expect(wrapper.find(".kline-chart-shell").exists()).toBe(true);

    wrapper.unmount();
  });

  it("applies backend live SSE ticks to the market page snapshot", async () => {
    vi.stubGlobal("fetch", buildStandardFetchMock());
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");
    const liveStream = findLiveEventStream();

    liveStream?.emitMessage({
      type: "market-data.tick",
      at: "2026-05-17T01:31:30.000Z",
      brokerId: "futu",
      instrument: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 321.8,
        bid: 321.7,
        ask: 321.9,
        openPrice: 319.8,
        highPrice: 322,
        lowPrice: 319.6,
        previousClosePrice: 318.9,
        volume: 1282000,
        turnover: 411000000,
        at: "2026-05-17T01:31:30.000Z",
      },
      source: "futu",
    });
    await new Promise((resolve) => setTimeout(resolve, 40));
    await flushRequests();

    expect(wrapper.text()).toContain("321.8");
    expect(wrapper.text()).toContain("+2.900");

    wrapper.unmount();
  });

  it("keeps applying live SSE ticks after the event buffer is full", async () => {
    vi.stubGlobal("fetch", buildStandardFetchMock());
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/market");
    const liveStream = findLiveEventStream();

    for (let index = 0; index < 25; index += 1) {
      liveStream?.emitMessage({
        type: "market-data.tick",
        at: `2026-05-17T01:31:${String(index).padStart(2, "0")}.000Z`,
        brokerId: "futu",
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        snapshot: {
          price: 321 + index / 10,
          bid: 320.9 + index / 10,
          ask: 321.1 + index / 10,
          volume: 1282000 + index,
          turnover: 411000000 + index,
          at: `2026-05-17T01:31:${String(index).padStart(2, "0")}.000Z`,
        },
        source: "futu",
      });
    }
    await new Promise((resolve) => setTimeout(resolve, 40));
    await flushRequests();

    expect(wrapper.text()).toContain("323.4");

    wrapper.unmount();
  });
});
