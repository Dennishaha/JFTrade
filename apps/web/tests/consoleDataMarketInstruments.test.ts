import { ref } from "vue";
import { describe, expect, it } from "vitest";

import {
  createConsoleDataMarketInstrumentsController,
  normalizeInstrumentParts,
} from "../src/composables/consoleDataMarketInstruments";

describe("console-data market instrument search", () => {
  it("normalizes embedded exchange prefixes and rejects incomplete identifiers", () => {
    expect(normalizeInstrumentParts({ market: "hk", symbol: "00700" })).toEqual({
      market: "HK",
      symbol: "00700",
    });
    expect(normalizeInstrumentParts({ market: "US", symbol: "hk:00700" })).toEqual({
      market: "HK",
      symbol: "00700",
    });
    expect(normalizeInstrumentParts({ symbol: "AAPL" }, "us")).toEqual({
      market: "US",
      symbol: "AAPL",
    });
    expect(normalizeInstrumentParts({ market: "HK", symbol: "HK." })).toBeNull();
    expect(normalizeInstrumentParts({ market: "", symbol: "" })).toBeNull();
  });

  it("merges references, broker mappings, subscriptions, holdings, and orders by normalized id", () => {
    const controller = createConsoleDataMarketInstrumentsController({
      marketDataQueryMarket: ref("US"),
      selectedBrokerAccount: ref({ market: "HK" }),
      marketInstrumentReferences: ref([
        {
          market: "HK",
          symbol: "00700",
          name: null,
          brokerMappings: [{ brokerId: "futu", brokerMarket: "HK", brokerSymbol: "00700", displayName: "腾讯" }],
        },
        { market: "", symbol: "" },
      ]),
      marketDataSubscriptions: ref({
        entries: [{ market: "HK", symbol: "00700" }, { market: "US", symbol: "AAPL", name: "Apple" }],
      } as never),
      portfolioPositions: ref({ positions: [{ market: "HK", symbol: "00700", name: "腾讯控股" }] } as never),
      brokerPositions: ref({ positions: [{ market: "US", symbol: "AAPL" }] } as never),
      brokerOrders: ref({ orders: [{ market: "HK", symbol: "00700" }] } as never),
      activeExecutionOrders: ref({ orders: [{ market: "US", symbol: "AAPL" }, { symbol: "MSFT" }] } as never),
    });

    const options = controller.marketInstrumentSearchOptions.value;
    expect(options).toEqual([
      expect.objectContaining({
        instrumentId: "HK.00700",
        name: "腾讯",
        sources: ["broker-order", "broker:futu", "portfolio", "reference", "subscription"],
      }),
      expect.objectContaining({
        instrumentId: "HK.MSFT",
        sources: ["execution-order"],
      }),
      expect.objectContaining({
        instrumentId: "US.AAPL",
        name: "Apple",
        sources: ["broker-position", "execution-order", "subscription"],
      }),
    ]);
    expect(options[0]?.label).toContain("腾讯");
  });
});
