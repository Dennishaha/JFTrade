import { describe, expect, it } from "vitest";

import { brokerSupportedChartSessions } from "@/composables/trading/brokerCandleSessions";

describe("broker candle session capabilities", () => {
  it("exposes Yahoo intraday extended hours but keeps daily data regular-only", () => {
    expect(brokerSupportedChartSessions("yfinance", "US", "1m")).toEqual([
      "regular",
      "extended",
    ]);
    expect(brokerSupportedChartSessions("yfinance", "US", "1d")).toEqual([
      "regular",
    ]);
    expect(brokerSupportedChartSessions("yfinance", "HK", "1h")).toEqual([
      "regular",
    ]);
  });

  it("exposes AKShare regular-only sessions and handles missing descriptors", () => {
    expect(brokerSupportedChartSessions("akshare", "SH", "1m")).toEqual([
      "regular",
    ]);
    expect(brokerSupportedChartSessions("missing", "US", "1m")).toBeNull();
    expect(brokerSupportedChartSessions("futu", "US", "1m", [])).toBeNull();
  });

  it("filters Futu sessions by period and tolerates unavailable candle features", () => {
    const descriptors = [
      {
        id: "futu",
        displayName: "Futu",
        capabilities: [
          {
            market: "US",
            supportsQuote: true,
            supportsTrade: false,
            features: [
              {
                id: "market.candles",
                state: "available" as const,
                supportedSessions: [
                  { id: "regular" as const, supportedPeriods: ["1m", "1d"] },
                  { id: "extended" as const, supportedPeriods: ["1m"] },
                  { id: "overnight" as const, supportedPeriods: ["1m"] },
                ],
              },
            ],
          },
        ],
      },
    ];

    expect(brokerSupportedChartSessions("futu", "US", "1m", descriptors)).toEqual([
      "regular",
      "extended",
      "overnight",
    ]);
    expect(brokerSupportedChartSessions("futu", "US", "1d", descriptors)).toEqual([
      "regular",
    ]);
    expect(brokerSupportedChartSessions("futu", "US", "", descriptors)).toEqual([
      "regular",
      "extended",
      "overnight",
    ]);
    expect(
      brokerSupportedChartSessions("futu", "HK", "1m", descriptors),
    ).toEqual([]);

    descriptors[0]!.capabilities![0]!.features = [
      { id: "market.candles", state: "unavailable" },
    ];
    expect(brokerSupportedChartSessions("futu", "US", "1m", descriptors)).toEqual([]);
  });
});
