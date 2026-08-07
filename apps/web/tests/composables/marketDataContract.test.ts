import { describe, expect, it } from "vitest";

import {
  mapMarketDataDepthResponse,
  mapMarketDataSubscriptions,
  mapMarketInstrumentReferenceResponse,
  type MarketInstrumentResolution,
  type MarketSubscriptionsWire,
} from "@/composables/market-data/marketDataContract";

describe("market data wire mappers", () => {
  it("maps the generated instrument resolution without inventing legacy fields", () => {
    const response: MarketInstrumentResolution = {
      query: "AAPL",
      requestedMarket: "US",
      resolutionStatus: "resolved",
      totalReturned: 1,
      failures: [],
      entries: [{
        market: "US",
        resolvedMarket: "US",
        instrumentId: "US.AAPL",
        code: "AAPL",
        symbol: "AAPL",
        name: "Apple",
        selectable: true,
      }],
    };

    expect(mapMarketInstrumentReferenceResponse(response)).toEqual({
      query: "AAPL",
      totalReturned: 1,
      entries: [{
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
        name: "Apple",
        securityType: null,
        lotSize: null,
        exchange: null,
        status: null,
        source: "",
        updatedAt: "",
      }],
    });
  });

  it("preserves available instrument metadata and explains unavailable matches", () => {
    const response: MarketInstrumentResolution = {
      query: "700",
      requestedMarket: "HK",
      resolutionStatus: "resolved",
      totalReturned: 2,
      failures: [],
      entries: [
        {
          market: "HK",
          resolvedMarket: "HK",
          instrumentId: "HK.00700",
          code: "00700",
          symbol: "00700",
          name: "Tencent",
          securityType: "EQUITY",
          lotSize: 100,
          selectable: true,
          source: "futu",
        },
        {
          market: "HK",
          resolvedMarket: "HK",
          instrumentId: "HK.BAD",
          code: "BAD",
          symbol: "BAD",
          selectable: false,
          unavailableReason: "market data permission required",
        },
      ],
    };

    const mapped = mapMarketInstrumentReferenceResponse(response);

    expect(mapped.entries[0]).toMatchObject({
      securityType: "EQUITY",
      lotSize: 100,
      source: "futu",
      status: null,
    });
    expect(mapped.entries[1]).toMatchObject({
      lotSize: null,
      status: "market data permission required",
    });
  });

  it("drops malformed depth levels and details while retaining valid order book rows", () => {
    const mapped = mapMarketDataDepthResponse({
      request: {
        instrumentId: "HK.00700",
        market: "HK",
        num: 10,
        symbol: "00700",
      },
      depth: {
        accountId: "SIM-1",
        symbol: "HK.00700",
        name: "Tencent",
        svrRecvTimeBid: null,
        svrRecvTimeAsk: "2026-07-26T10:00:00Z",
        bids: [
          {
            price: 520,
            volume: 1_000,
            orderCount: 3,
            detailList: [
              { orderId: 101, volume: 600 },
              { orderId: "invalid", volume: 400 },
              null,
            ],
          },
          { price: Number.NaN, volume: 100, orderCount: 1 },
          "invalid",
        ],
        asks: [
          { price: 521, volume: 800, orderCount: 2, detailList: null },
          { price: 522, volume: "invalid", orderCount: 1 },
        ],
      },
      meta: {
        fromCache: false,
        instrumentId: "HK.00700",
        resolvedAt: "2026-07-26T10:00:00Z",
        source: "futu",
      },
    } as never);

    expect(mapped.depth).toMatchObject({
      accountId: "SIM-1",
      symbol: "HK.00700",
      name: "Tencent",
      svrRecvTimeBid: null,
      svrRecvTimeAsk: "2026-07-26T10:00:00Z",
      bids: [{
        price: 520,
        volume: 1_000,
        orderCount: 3,
        detailList: [{ orderId: 101, volume: 600 }],
      }],
      asks: [{
        price: 521,
        volume: 800,
        orderCount: 2,
        detailList: null,
      }],
    });
  });

  it("uses request symbols when a legacy depth payload omits identity fields", () => {
    const mapped = mapMarketDataDepthResponse({
      request: {
        instrumentId: "US.AAPL",
        market: "US",
        num: 5,
        symbol: "AAPL",
      },
      depth: { bids: null, asks: "invalid" },
      meta: {
        fromCache: true,
        instrumentId: "US.AAPL",
        resolvedAt: "2026-07-26T10:00:00Z",
        source: "cache",
      },
    });

    expect(mapped.depth).toEqual({
      accountId: "",
      symbol: "AAPL",
      name: null,
      svrRecvTimeBid: null,
      svrRecvTimeAsk: null,
      bids: [],
      asks: [],
    });
  });

  it("preserves nullable quota fields and drops unknown broker states", () => {
    const response: MarketSubscriptionsWire = {
      totalActiveSubscriptions: 1,
      quota: {
        totalUsed: 1,
        totalLimit: null,
        totalRemaining: null,
        byMarket: [{ market: "US", used: 1, limit: null, remaining: null }],
      },
      entries: [{
        channel: "SNAPSHOT",
        consumers: ["web:test"],
        createdAt: "2026-07-26T00:00:00Z",
        depthLevel: null,
        instrumentId: "US.AAPL",
        interval: null,
        key: "SNAPSHOT:US:AAPL",
        market: "US",
        refCount: 1,
        symbol: "AAPL",
        updatedAt: "2026-07-26T00:00:00Z",
        brokerState: "future_state",
      }],
    };

    const mapped = mapMarketDataSubscriptions(response);
    expect(mapped.quota.totalLimit).toBeNull();
    expect(mapped.entries[0]).not.toHaveProperty("brokerState");
  });

  it("maps all managed broker states and populated subscription diagnostics", () => {
    const response = {
      consumerId: "web:watchlist",
      providerBrokerId: "futu",
      action: "acquire",
      instruments: [{ market: "US", symbol: "AAPL", channel: "QUOTE" }],
      transport: { mode: "websocket" },
      desiredCount: 5,
      ownActiveCount: 4,
      pendingReleaseCount: 1,
      totalUsedQuota: 8,
      remainQuota: 22,
      totalActiveSubscriptions: 8,
      quota: {
        totalUsed: 8,
        totalLimit: 30,
        totalRemaining: 22,
        byMarket: [{ market: "US", used: 8, limit: 30, remaining: 22 }],
      },
      entries: [
        "active",
        "fallback",
        "pending_subscribe",
        "pending_unsubscribe",
        "retrying",
        "unmanaged",
      ].map((brokerState, index) => ({
        channel: "QUOTE",
        consumers: ["web:watchlist"],
        createdAt: "2026-07-26T00:00:00Z",
        depthLevel: null,
        instrumentId: `US.TEST${index}`,
        interval: null,
        key: `QUOTE:US:TEST${index}`,
        market: "US",
        refCount: 1,
        symbol: `TEST${index}`,
        updatedAt: "2026-07-26T00:00:00Z",
        brokerState,
        subscribedAt: "2026-07-26T00:00:01Z",
        unsubscribeEligibleAt: "2026-07-26T00:01:00Z",
        lastError: "retry scheduled",
      })),
      brokerState: {
        desiredCount: 5,
        ownActiveCount: 4,
        pendingReleaseCount: 1,
        totalUsedQuota: 8,
        remainQuota: 22,
        ownUsedQuota: 4,
        checkedAt: "2026-07-26T00:00:02Z",
        reconciledAt: null,
        lastError: "one subscription is retrying",
        entries: [
          {
            key: "QUOTE:US:AAPL",
            kind: "quote",
            instrumentId: "US.AAPL",
            interval: null,
            brokerState: "active",
            subscribedAt: "2026-07-26T00:00:01Z",
            unsubscribeEligibleAt: null,
            lastError: null,
          },
          null,
        ],
      },
    } as unknown as MarketSubscriptionsWire;

    const mapped = mapMarketDataSubscriptions(response);

    expect(mapped).toMatchObject({
      consumerId: "web:watchlist",
      providerBrokerId: "futu",
      action: "acquire",
      desiredCount: 5,
      ownActiveCount: 4,
      pendingReleaseCount: 1,
      totalUsedQuota: 8,
      remainQuota: 22,
      transport: { mode: "websocket" },
    });
    expect(mapped.entries.map((entry) => entry.brokerState)).toEqual([
      "active",
      "fallback",
      "pending_subscribe",
      "pending_unsubscribe",
      "retrying",
      "unmanaged",
    ]);
    expect(mapped.brokerState).toMatchObject({
      ownUsedQuota: 4,
      checkedAt: "2026-07-26T00:00:02Z",
      reconciledAt: null,
      lastError: "one subscription is retrying",
    });
    expect(mapped.brokerState?.entries[1]).toEqual({
      key: "",
      kind: "",
      instrumentId: "",
      interval: null,
      brokerState: "",
      subscribedAt: null,
      unsubscribeEligibleAt: null,
      lastError: null,
    });
  });
});
