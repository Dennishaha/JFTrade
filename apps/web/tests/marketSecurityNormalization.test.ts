import { describe, expect, it } from "vitest";

import { normalizeMarketSecurityDetailsQueryResult } from "../src/composables/marketSecurityNormalization";

describe("normalizeMarketSecurityDetailsQueryResult", () => {
  it("normalizes nested market security number fields across detail blocks", () => {
    const normalized = normalizeMarketSecurityDetailsQueryResult({
      request: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      security: {
        securityId: "700",
        listTimestamp: "1718000000",
        lotSize: "100",
        currentPrice: "388.5",
        volume: "120000",
        extended: {
          preMarket: {
            price: "390.1",
            volume: "1200",
          },
          afterMarket: {
            changeRate: "1.5",
          },
          overnight: {
            turnover: "2500000",
          },
        },
        equity: {
          issuedShares: "1000000",
          peRate: "18.5",
        },
        warrant: {
          strikePrice: "400",
          owner: {
            instrumentId: "1",
            market: "HK",
            symbol: "00700",
          },
        },
        option: {
          contractSize: "10",
          owner: {
            instrumentId: "2",
            market: "US",
            symbol: "AAPL",
          },
        },
        index: {
          raiseCount: "25",
        },
        plate: {
          equalCount: "4",
        },
        future: {
          position: "88",
          lastTradeTimestamp: "1718000999",
        },
        trust: {
          aum: "4500000",
        },
      },
    } as never);

    expect(normalized.security).toMatchObject({
      securityId: 700,
      listTimestamp: 1718000000,
      lotSize: 100,
      currentPrice: 388.5,
      volume: 120000,
      extended: {
        preMarket: {
          price: 390.1,
          volume: 1200,
        },
        afterMarket: {
          changeRate: 1.5,
        },
        overnight: {
          turnover: 2500000,
        },
      },
      equity: {
        issuedShares: 1000000,
        peRate: 18.5,
      },
      warrant: {
        strikePrice: 400,
        owner: {
          instrumentId: "1",
          market: "HK",
          symbol: "00700",
        },
      },
      option: {
        contractSize: 10,
        owner: {
          instrumentId: "2",
          market: "US",
          symbol: "AAPL",
        },
      },
      index: {
        raiseCount: 25,
      },
      plate: {
        equalCount: 4,
      },
      future: {
        position: 88,
        lastTradeTimestamp: 1718000999,
      },
      trust: {
        aum: 4500000,
      },
    });
  });

  it("keeps non-numeric business values intact while nulling invalid nested records", () => {
    const empty = normalizeMarketSecurityDetailsQueryResult({
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
      },
      security: "missing",
    } as never);
    expect(empty.security).toBeNull();

    const normalized = normalizeMarketSecurityDetailsQueryResult({
      request: {
        market: "US",
        symbol: "TME",
        instrumentId: "US.TME",
      },
      security: {
      securityId: "not-a-number",
      lotSize: Number.POSITIVE_INFINITY,
      currentPrice: "",
      turnover: true,
      extended: "broken",
      equity: null,
        warrant: {
          strikePrice: "bad",
          owner: "broker-feed",
        },
        option: {
          owner: 42,
        },
        future: {
          position: null,
        },
      },
    } as never);

    expect(normalized.security).toMatchObject({
      securityId: "not-a-number",
      lotSize: Number.POSITIVE_INFINITY,
      currentPrice: "",
      turnover: true,
      extended: null,
      equity: null,
      warrant: {
        strikePrice: "bad",
        owner: null,
      },
      option: {
        owner: null,
      },
      future: {
        position: null,
      },
    });
  });
});
