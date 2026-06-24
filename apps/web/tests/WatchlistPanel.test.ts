// @vitest-environment jsdom

import type {
  MarketProfileDto,
  MarketSecurityDetails,
  MarketSecurityDetailsQueryResult,
} from "@/contracts";
import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { afterEach, describe, expect, it } from "vitest";
import { defineComponent, h } from "vue";

import WatchlistPanel from "../src/components/workspace/WatchlistPanel.vue";
import type { MarketDataSnapshotQueryResult } from "../src/composables/marketDataRealtime";
import { useMarketProfiles } from "../src/composables/marketProfiles";
import { provideConsoleDataStore } from "../src/composables/useConsoleData";
import { provideWorkspaceLayoutStore } from "../src/composables/useWorkspaceLayout";

afterEach(() => {
  window.localStorage?.clear();
  useMarketProfiles().marketProfiles.value = [];
});

describe("WatchlistPanel", () => {
  it("renders equity security summary and fundamentals", () => {
    const wrapper = mountWatchlistPanel({
      market: "HK",
      symbol: "00700",
      security: createSecurityDetails({
        instrumentId: "HK.00700",
        market: "HK",
        symbol: "00700",
        name: "Tencent Holdings",
        securityType: "Eqty",
        exchangeType: "HK_HKEX",
        equity: {
          issuedShares: 9600000000,
          issuedMarketValue: 3085440000000,
          netAsset: 950000000000,
          netProfit: 185000000000,
          earningsPerShare: 19.2,
          outstandingShares: 9300000000,
          outstandingMarketVal: 2989020000000,
          netAssetPerShare: 98.3,
          earningsYieldRate: 6,
          peRate: 16.7,
          pbRate: 3.2,
          peTTMRate: 17.1,
          dividendRatioTTM: 1.1,
        },
      }),
    });

    expect(wrapper.text()).toContain("HK.00700 · Tencent Holdings");
    expect(wrapper.text()).toContain("Security");
    expect(wrapper.text()).toContain("股票基本面");
    expect(wrapper.text()).toContain("PE TTM");
    expect(wrapper.text()).toContain("HK_HKEX");
    expect(wrapper.text()).toContain("321.400");

    wrapper.unmount();
  });

  it("renders option details for derivatives", () => {
    const wrapper = mountWatchlistPanel({
      market: "US",
      symbol: "AAPL250117C00200000",
      security: createSecurityDetails({
        instrumentId: "US.AAPL250117C00200000",
        market: "US",
        symbol: "AAPL250117C00200000",
        name: "AAPL 2025-01-17 200C",
        securityType: "Drvt",
        exchangeType: "US_Option",
        option: {
          optionType: "Call",
          owner: { instrumentId: "US.AAPL", market: "US", symbol: "AAPL" },
          strikeTime: "2025-01-17",
          strikePrice: 200,
          contractSize: 100,
          contractSizeFloat: 100,
          openInterest: 14280,
          impliedVolatility: 24.2,
          premium: 3.4,
          delta: 0.61,
          gamma: 0.02,
          vega: 0.11,
          theta: -0.08,
          rho: 0.04,
          strikeTimestamp: 1737072000,
          expiryDateDistance: 45,
          contractNominalValue: 20000,
          ownerLotMultiplier: 1,
          contractMultiplier: 100,
        },
      }),
      snapshot: createSnapshotResult("US", "AAPL250117C00200000", 18.4),
    });

    expect(wrapper.text()).toContain("US.AAPL250117C00200000 · AAPL 2025-01-17 200C");
    expect(wrapper.text()).toContain("期权信息");
    expect(wrapper.text()).toContain("Call");
    expect(wrapper.text()).toContain("US.AAPL");
    expect(wrapper.text()).toContain("200.000");
    expect(wrapper.text()).toContain("24.20%");

    wrapper.unmount();
  });

  it("renders extended session cards only for markets that support extended hours", () => {
    const usWrapper = mountWatchlistPanel({
      market: "US",
      symbol: "AAPL",
      security: createSecurityDetails({
        instrumentId: "US.AAPL",
        market: "US",
        symbol: "AAPL",
        name: "Apple",
        securityType: "Eqty",
        exchangeType: "US_NASDAQ",
      }),
      snapshot: createExtendedSnapshotResult("US", "AAPL", "pre"),
    });

    expect(usWrapper.text()).toContain("盘前价格");
    usWrapper.unmount();

    const hkWrapper = mountWatchlistPanel({
      market: "HK",
      symbol: "00700",
      security: createSecurityDetails({
        instrumentId: "HK.00700",
        market: "HK",
        symbol: "00700",
        name: "Tencent Holdings",
        securityType: "Eqty",
        exchangeType: "HK_HKEX",
      }),
      snapshot: createExtendedSnapshotResult("HK", "00700", "pre"),
    });

    expect(hkWrapper.text()).not.toContain("盘前价格");
    hkWrapper.unmount();
  });

  it("shows only recent after-hours cards when the US market is closed", () => {
    const wrapper = mountWatchlistPanel({
      market: "US",
      symbol: "AAPL",
      security: createSecurityDetails({
        instrumentId: "US.AAPL",
        market: "US",
        symbol: "AAPL",
        name: "Apple",
        securityType: "Eqty",
        exchangeType: "US_NASDAQ",
      }),
      snapshot: createClosedExtendedSnapshotResult("US", "AAPL"),
    });

    expect(wrapper.text()).toContain("休市");
    expect(wrapper.text()).toContain("最近盘后价格");
    expect(wrapper.text()).not.toContain("盘前价格");

    wrapper.unmount();
  });

  it("renders overnight session and overnight pricing for US snapshots", () => {
    const wrapper = mountWatchlistPanel({
      market: "US",
      symbol: "AAPL",
      security: createSecurityDetails({
        instrumentId: "US.AAPL",
        market: "US",
        symbol: "AAPL",
        name: "Apple",
        securityType: "Eqty",
        exchangeType: "US_NASDAQ",
      }),
      snapshot: createExtendedSnapshotResult("US", "AAPL", "overnight"),
    });

    expect(wrapper.text()).toContain("夜盘");
    expect(wrapper.text()).toContain("夜盘价格");
    expect(wrapper.text()).toContain("最近盘后价格");

    wrapper.unmount();
  });

  it("does not render security details that belong to a previous instrument", () => {
    const wrapper = mountWatchlistPanel({
      market: "US",
      symbol: "AAPL",
      security: createSecurityDetails({
        instrumentId: "HK.00700",
        market: "HK",
        symbol: "00700",
        name: "Tencent Holdings",
        securityType: "Eqty",
        exchangeType: "HK_HKEX",
      }),
      snapshot: createSnapshotResult("HK", "00700", 321.4),
    });

    expect(wrapper.text()).toContain("US.AAPL");
    expect(wrapper.text()).not.toContain("Tencent Holdings");
    expect(wrapper.text()).not.toContain("321.400");

    wrapper.unmount();
  });

  it("renders warrant details", () => {
    const wrapper = mountWatchlistPanel({
      market: "HK",
      symbol: "21164",
      security: createSecurityDetails({
        instrumentId: "HK.21164",
        market: "HK",
        symbol: "21164",
        name: "Tencent Bull 21164",
        securityType: "Warrant",
        exchangeType: "HK_HKEX",
        currentPrice: 0.118,
        openPrice: 0.115,
        highPrice: 0.121,
        lowPrice: 0.113,
        lastClosePrice: 0.114,
        warrant: {
          conversionRate: 10000,
          warrantType: "Bull",
          strikePrice: 320,
          maturityTime: "2026-12-30",
          endTradeTime: "2026-12-29",
          owner: { instrumentId: "HK.00700", market: "HK", symbol: "00700" },
          recoveryPrice: 300,
          streetVolume: 32000000,
          issueVolume: 64000000,
          streetRate: 50,
          delta: 0.48,
          impliedVolatility: 28.5,
          premium: 12.6,
          leverage: 8.2,
          issuerCode: "SG",
        },
      }),
      snapshot: createSnapshotResult("HK", "21164", 0.118),
    });

    expect(wrapper.text()).toContain("窝轮信息");
    expect(wrapper.text()).toContain("Bull");
    expect(wrapper.text()).toContain("HK.00700");
    expect(wrapper.text()).toContain("SG");
    expect(wrapper.text()).toContain("12.60%");

    wrapper.unmount();
  });

  it("renders future details", () => {
    const wrapper = mountWatchlistPanel({
      market: "HK",
      symbol: "HSIMAIN",
      security: createSecurityDetails({
        instrumentId: "HK.HSIMAIN",
        market: "HK",
        symbol: "HSIMAIN",
        name: "HSI Main",
        securityType: "Future",
        exchangeType: "HK_HKEX",
        currentPrice: 18456,
        openPrice: 18412,
        highPrice: 18502,
        lowPrice: 18380,
        lastClosePrice: 18390,
        future: {
          lastSettlePrice: 18390,
          position: 182233,
          positionChange: 4201,
          lastTradeTime: "2026-06-29",
          lastTradeTimestamp: 1782691200,
          isMainContract: true,
        },
      }),
      snapshot: createSnapshotResult("HK", "HSIMAIN", 18456),
    });

    expect(wrapper.text()).toContain("期货信息");
    expect(wrapper.text()).toContain("182,233");
    expect(wrapper.text()).toContain("4,201");
    expect(wrapper.text()).toContain("2026-06-29");
    expect(wrapper.text()).toContain("是");

    wrapper.unmount();
  });

  it("renders trust details", () => {
    const wrapper = mountWatchlistPanel({
      market: "US",
      symbol: "SPY",
      security: createSecurityDetails({
        instrumentId: "US.SPY",
        market: "US",
        symbol: "SPY",
        name: "SPDR S&P 500 ETF",
        securityType: "Trust",
        exchangeType: "US_NYSE",
        currentPrice: 590.6,
        openPrice: 589.2,
        highPrice: 592.1,
        lowPrice: 587.4,
        lastClosePrice: 588.1,
        trust: {
          dividendYield: 1.3,
          aum: 580000000000,
          outstandingUnit: 985000000,
          netAssetValue: 589.8,
          premium: 0.14,
          assetClass: "Stock",
        },
      }),
      snapshot: createSnapshotResult("US", "SPY", 590.6),
    });

    expect(wrapper.text()).toContain("基金信息");
    expect(wrapper.text()).toContain("Stock");
    expect(wrapper.text()).toContain("1.30%");
    expect(wrapper.text()).toContain("589.800");
    expect(wrapper.text()).toContain("0.14%");

    wrapper.unmount();
  });

  it("renders index breadth details", () => {
    const wrapper = mountWatchlistPanel({
      market: "HK",
      symbol: "HSI",
      security: createSecurityDetails({
        instrumentId: "HK.HSI",
        market: "HK",
        symbol: "HSI",
        name: "Hang Seng Index",
        securityType: "Index",
        exchangeType: "HK_HKEX",
        currentPrice: 18456.2,
        openPrice: 18398.1,
        highPrice: 18512.4,
        lowPrice: 18354.6,
        lastClosePrice: 18390.5,
        index: {
          raiseCount: 58,
          fallCount: 21,
          equalCount: 3,
        },
      }),
      snapshot: createSnapshotResult("HK", "HSI", 18456.2),
    });

    expect(wrapper.text()).toContain("指数成分");
    expect(wrapper.text()).toContain("Hang Seng Index");
    expect(wrapper.text()).toContain("58");
    expect(wrapper.text()).toContain("21");
    expect(wrapper.text()).toContain("3");

    wrapper.unmount();
  });

  it("renders plate breadth details", () => {
    const wrapper = mountWatchlistPanel({
      market: "HK",
      symbol: "TECH",
      security: createSecurityDetails({
        instrumentId: "HK.TECH",
        market: "HK",
        symbol: "TECH",
        name: "Technology Sector",
        securityType: "Plate",
        exchangeType: "HK_HKEX",
        currentPrice: 7850.3,
        openPrice: 7792.5,
        highPrice: 7898.8,
        lowPrice: 7765.1,
        lastClosePrice: 7808.4,
        plate: {
          raiseCount: 42,
          fallCount: 17,
          equalCount: 5,
        },
      }),
      snapshot: createSnapshotResult("HK", "TECH", 7850.3),
    });

    expect(wrapper.text()).toContain("板块成分");
    expect(wrapper.text()).toContain("Technology Sector");
    expect(wrapper.text()).toContain("42");
    expect(wrapper.text()).toContain("17");
    expect(wrapper.text()).toContain("5");

    wrapper.unmount();
  });
});

function mountWatchlistPanel(options: {
  market: string;
  symbol: string;
  security: MarketSecurityDetails;
  snapshot?: MarketDataSnapshotQueryResult;
}) {
  const { market, symbol, security } = options;
  const snapshot = options.snapshot ?? createSnapshotResult(market, symbol, security.currentPrice);

  const Host = defineComponent({
    setup() {
      useMarketProfiles().marketProfiles.value = testMarketProfiles;
      const workspaceLayout = provideWorkspaceLayoutStore();
      workspaceLayout.update({ market, symbol });
      const store = provideConsoleDataStore(workspaceLayout);
      store.selectWorkspaceInstrument({ market, symbol, period: "1m" });
      store.marketDataSnapshot.value = snapshot;
      store.marketSecurityDetails.value = createSecurityResult(security);
      return () => h(WatchlistPanel);
    },
  });

  return mount(Host, {
    global: {
      plugins: [createPinia()],
    },
  });
}

const testMarketProfiles: MarketProfileDto[] = [
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
];

function createSnapshotResult(
  market: string,
  symbol: string,
  price: number,
): MarketDataSnapshotQueryResult {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: {
      market,
      symbol,
      instrumentId,
    },
    snapshot: {
      price,
      bid: price - 0.1,
      ask: price + 0.1,
      openPrice: price - 1,
      highPrice: price + 1,
      lowPrice: price - 1.2,
      previousClosePrice: price - 0.8,
      lastClosePrice: price - 1.1,
      volume: 1282100,
      turnover: 411020000,
      at: "2026-05-22T01:30:00.000Z",
      session: market === "US" ? "regular" : "unknown",
    },
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-05-22T01:30:00.000Z",
      fromCache: false,
    },
  };
}

function createExtendedSnapshotResult(
  market: string,
  symbol: string,
  session: "pre" | "after" | "overnight",
): MarketDataSnapshotQueryResult {
  const result = createSnapshotResult(market, symbol, 190);
  result.snapshot.session = session;
  result.snapshot.extended = {
    preMarket: {
      price: 191,
      changeRate: 1.2,
      quoteTime: "2026-05-22T08:15:00.000Z",
    },
    afterMarket: {
      price: 192,
      changeRate: 1.8,
      quoteTime: "2026-05-21T20:00:00.000Z",
    },
    overnight: {
      price: 193,
      changeRate: 2.3,
      quoteTime: "2026-05-21T23:00:00.000Z",
    },
  };
  return result;
}

function createClosedExtendedSnapshotResult(
  market: string,
  symbol: string,
): MarketDataSnapshotQueryResult {
  const result = createSnapshotResult(market, symbol, 190);
  result.snapshot.session = "closed";
  result.snapshot.extended = {
    preMarket: {
      price: 191,
      changeRate: 1.2,
      quoteTime: "2026-06-18T20:00:00.000Z",
    },
    afterMarket: {
      price: 192,
      changeRate: 1.8,
      quoteTime: "2026-06-18T20:00:00.000Z",
    },
  };
  return result;
}

function createSecurityResult(
  security: MarketSecurityDetails,
): MarketSecurityDetailsQueryResult {
  return {
    request: {
      market: security.market,
      symbol: security.symbol,
      instrumentId: security.instrumentId,
    },
    security,
    meta: {
      instrumentId: security.instrumentId,
      source: "test",
      resolvedAt: "2026-05-22T01:30:00.000Z",
      fromCache: false,
    },
  };
}

function createSecurityDetails(
  overrides: Partial<MarketSecurityDetails> & {
    instrumentId: string;
    market: string;
    symbol: string;
    name: string;
    securityType: string;
    exchangeType: string;
  },
): MarketSecurityDetails {
  return {
    instrumentId: overrides.instrumentId,
    market: overrides.market,
    symbol: overrides.symbol,
    securityId: overrides.securityId ?? 1,
    name: overrides.name,
    securityType: overrides.securityType,
    exchangeType: overrides.exchangeType,
    listTime: overrides.listTime ?? "2024-01-01",
    listTimestamp: overrides.listTimestamp ?? 1704067200,
    delisting: overrides.delisting ?? false,
    lotSize: overrides.lotSize ?? 100,
    isSuspend: overrides.isSuspend ?? false,
    priceSpread: overrides.priceSpread ?? 0.01,
    updateTime: overrides.updateTime ?? "2026-05-22 09:30:00",
    updateTimestamp: overrides.updateTimestamp ?? 1779413400,
    highPrice: overrides.highPrice ?? 322.6,
    openPrice: overrides.openPrice ?? 319.8,
    lowPrice: overrides.lowPrice ?? 319.6,
    lastClosePrice: overrides.lastClosePrice ?? 318.9,
    currentPrice: overrides.currentPrice ?? 321.4,
    volume: overrides.volume ?? 1282100,
    turnover: overrides.turnover ?? 411020000,
    turnoverRate: overrides.turnoverRate ?? 1.25,
    askPrice: overrides.askPrice ?? null,
    bidPrice: overrides.bidPrice ?? null,
    askVolume: overrides.askVolume ?? null,
    bidVolume: overrides.bidVolume ?? null,
    amplitude: overrides.amplitude ?? null,
    averagePrice: overrides.averagePrice ?? null,
    bidAskRatio: overrides.bidAskRatio ?? null,
    volumeRatio: overrides.volumeRatio ?? 1.1,
    highest52WeeksPrice: overrides.highest52WeeksPrice ?? 400.5,
    lowest52WeeksPrice: overrides.lowest52WeeksPrice ?? 260.2,
    highestHistoryPrice: overrides.highestHistoryPrice ?? null,
    lowestHistoryPrice: overrides.lowestHistoryPrice ?? null,
    sessionStatus: overrides.sessionStatus ?? "Normal",
    closePrice5Minute: overrides.closePrice5Minute ?? null,
    highPrecisionVolume: overrides.highPrecisionVolume ?? null,
    highPrecisionAskVol: overrides.highPrecisionAskVol ?? null,
    highPrecisionBidVol: overrides.highPrecisionBidVol ?? null,
    extended: overrides.extended ?? null,
    equity: overrides.equity ?? null,
    warrant: overrides.warrant ?? null,
    option: overrides.option ?? null,
    index: overrides.index ?? null,
    plate: overrides.plate ?? null,
    future: overrides.future ?? null,
    trust: overrides.trust ?? null,
  };
}
