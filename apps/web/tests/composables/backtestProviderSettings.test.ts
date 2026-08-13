import { describe, expect, it } from "vitest";

import {
  backtestHistoricalRangeError,
  resolveHistoricalLookbackDays,
  type BacktestProviderDescriptor,
} from "@/composables/backtest/backtestProviderSettings";

const akshare: BacktestProviderDescriptor = {
  selectionId: "akshare",
  providerId: "akshare",
  displayName: "AKShare",
  capabilities: {
    historicalCandles: true,
    streamingCandles: false,
    extendedHours: false,
    candleIntervals: ["1m", "5m", "1d"],
    priceAdjustments: ["none"],
    historicalLookbackDays: { "1m": 5, "US:5m": 5 },
  },
};

describe("backtest provider historical windows", () => {
  it("prefers a market-scoped window without restricting other markets", () => {
    expect(resolveHistoricalLookbackDays(akshare.capabilities, "us", "5m")).toBe(5);
    expect(resolveHistoricalLookbackDays(akshare.capabilities, "HK", "5m")).toBeNull();
    expect(resolveHistoricalLookbackDays(akshare.capabilities, "HK", "1m")).toBe(5);
  });

  it("rejects the one-year US intraday range shown by the desktop flow", () => {
    expect(backtestHistoricalRangeError(
      akshare,
      "US",
      "5m",
      "2025-07-13",
      new Date("2026-08-13T00:00:00Z"),
    )).toContain("最近 5 天");
    expect(backtestHistoricalRangeError(
      akshare,
      "US",
      "5m",
      "2026-08-10",
      new Date("2026-08-13T00:00:00Z"),
    )).toBe("");
  });
});
