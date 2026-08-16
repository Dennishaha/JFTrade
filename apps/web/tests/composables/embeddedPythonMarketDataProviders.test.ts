import { describe, expect, it } from "vitest";

import {
  embeddedPythonMarketDataFeatureIDs,
  embeddedPythonMarketDataProviderOption,
  pythonMarketDataProviderName,
  statusMatchesPythonMarketDataProvider,
} from "@/composables/market-data/embeddedPythonMarketDataProviders";
import { brokerSupportedChartPeriods } from "@/composables/trading/brokerProviderSelection";

describe("embedded Python market-data providers", () => {
  it("presents AKShare as a selectable delayed HTTP provider", () => {
    expect(embeddedPythonMarketDataProviderOption("akshare", "HK")).toEqual({
      id: "akshare",
      label: "AKShare",
      shortLabel: "AKShare",
      securityFirm: "内置行情查询",
      state: "degraded",
      displayState: "available",
      tone: "success",
      selectable: true,
      reason: "延迟 HTTP 轮询，不支持实时推流或 Level 2",
    });
    expect(embeddedPythonMarketDataProviderOption("akshare", "JP")).toMatchObject({
      selectable: false,
      displayState: "unavailable",
      reason: "当前标的市场不在内置 AKShare 支持范围",
    });
    expect(brokerSupportedChartPeriods("akshare", "SH", [])).toEqual([
      "1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo",
    ]);
  });

  it("marks news and corporate actions as embedded-capable features", () => {
    expect(embeddedPythonMarketDataFeatureIDs.has("market.news")).toBe(true);
    expect(embeddedPythonMarketDataFeatureIDs.has("market.corporate_actions")).toBe(true);
    expect(embeddedPythonMarketDataFeatureIDs.has("research.news")).toBe(true);
    expect(embeddedPythonMarketDataFeatureIDs.has("research.corporate_actions")).toBe(true);
    expect(embeddedPythonMarketDataFeatureIDs.has("research.instrument")).toBe(false);
  });

  it("keeps Yahoo aliases and provider-specific status identities", () => {
    expect(pythonMarketDataProviderName("yfinance")).toBe("Yahoo");
    expect(statusMatchesPythonMarketDataProvider("yfinance", "yahoo-finance")).toBe(true);
    expect(statusMatchesPythonMarketDataProvider("akshare", "akshare")).toBe(true);
    expect(statusMatchesPythonMarketDataProvider("akshare", "yfinance")).toBe(false);
  });
});
