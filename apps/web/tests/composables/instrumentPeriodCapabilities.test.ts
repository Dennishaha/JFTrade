import { describe, expect, it } from "vitest";

import {
  fallbackInstrumentPeriod,
  resolveInstrumentRequestPeriod,
  resolveInstrumentSupportedPeriods,
} from "@/composables/market-data/instrumentPeriodCapabilities";
import type { MarketSecurityDetailsQueryResult } from "@/types";

const renderablePeriods = new Set(["1m", "5m", "1d", "1w", "1mo"]);

function details(options: {
  instrumentID?: string;
  brokerID?: string;
  source?: string;
  supportedPeriods?: string[];
  security?: boolean;
} = {}): MarketSecurityDetailsQueryResult {
  const instrumentID = options.instrumentID ?? "US.AAPL";
  return {
    request: { market: "US", symbol: "AAPL", instrumentId: instrumentID },
    security:
      options.security === false
        ? null
        : { instrumentId: instrumentID, supportedPeriods: options.supportedPeriods },
    meta: {
      instrumentId: instrumentID,
      brokerId: options.brokerID,
      source: options.source ?? "",
      resolvedAt: "2026-08-04T00:00:00Z",
      fromCache: false,
    },
  } as MarketSecurityDetailsQueryResult;
}

function supported(input: Partial<Parameters<typeof resolveInstrumentSupportedPeriods>[0]> = {}) {
  return resolveInstrumentSupportedPeriods({
    providerID: "akshare",
    instrumentID: "US.AAPL",
    providerPeriods: ["1m", "5m", "1d"],
    details: details({ source: "akshare:eastmoney", supportedPeriods: ["1D"] }),
    requireDetails: true,
    ...input,
  });
}

describe("instrument period capabilities", () => {
  it("accepts canonical and legacy provider identities", () => {
    expect(supported({ providerID: "yfinance", details: details({ brokerID: "yahoo-finance" }) })).toEqual([]);
    expect(supported({ providerID: "yfinance", details: details({ brokerID: "yfinance" }) })).toEqual([]);
    expect(supported({ providerID: "futu", details: details({ brokerID: "futu-opend" }) })).toEqual([]);
    expect(supported({ providerID: "futu", details: details({ brokerID: "futu" }) })).toEqual([]);
    expect(supported({ providerID: "akshare", details: details({ brokerID: "akshare" }) })).toEqual([]);
    expect(supported({ providerID: "", details: details({ brokerID: "akshare" }) })).toBeNull();
    expect(supported({ providerID: "akshare", details: details({ brokerID: "yfinance" }) })).toBeNull();
  });

  it("accepts each provider source family and rejects stale sources", () => {
    for (const [providerID, source] of [
      ["akshare", "akshare"],
      ["akshare", "akshare:eastmoney"],
      ["yfinance", "yfinance"],
      ["yfinance", "yahoo-finance"],
      ["futu", "futu"],
      ["futu", "futu-opend"],
      ["futu", "futu:snapshot"],
      ["futu", "bbgo:futu"],
      ["custom", "custom"],
      ["custom", "custom:catalog"],
    ]) {
      expect(supported({ providerID, details: details({ source }) })).toEqual([]);
    }
    expect(supported({ providerID: "", details: details({ source: "custom" }) })).toBeNull();
    expect(supported({ providerID: "akshare", details: details({ source: "yfinance" }) })).toBeNull();
  });

  it("requires current-instrument details and intersects normalized periods", () => {
    expect(supported({ providerPeriods: null })).toBeNull();
    expect(supported({ details: null })).toBeNull();
    expect(supported({ details: details({ instrumentID: "HK.00700", source: "akshare" }) })).toBeNull();
    expect(
      supported({
        details: details({
          source: "akshare:eastmoney",
          supportedPeriods: [" 1D ", "1d", "5M", "tick", ""],
        }),
      }),
    ).toEqual(["1d", "5m"]);
    expect(supported({ details: details({ source: "akshare", security: false }) })).toEqual([]);
    expect(supported({ details: null, requireDetails: false })).toEqual(["1m", "5m", "1d"]);
  });

  it("chooses deterministic fallbacks and only requests supported periods", () => {
    expect(fallbackInstrumentPeriod(["tick", "1m", "1d"])).toBe("1m");
    expect(fallbackInstrumentPeriod(["tick", "5m", "1d"])).toBe("5m");
    expect(fallbackInstrumentPeriod(["tick", "1d"])).toBe("1d");
    expect(fallbackInstrumentPeriod(["tick", "1w"])).toBe("1w");
    expect(fallbackInstrumentPeriod(["tick"])).toBe("tick");

    expect(
      resolveInstrumentRequestPeriod({
        preferredPeriod: "1d",
        providerPeriods: ["tick", "1d"],
        supportedPeriods: ["1d"],
        requireDetails: true,
        renderablePeriods,
      }),
    ).toBe("1d");
    expect(
      resolveInstrumentRequestPeriod({
        preferredPeriod: "tick",
        providerPeriods: ["tick", "5m", "1d"],
        supportedPeriods: null,
        requireDetails: true,
        renderablePeriods,
      }),
    ).toBe("5m");
    expect(
      resolveInstrumentRequestPeriod({
        preferredPeriod: "tick",
        providerPeriods: null,
        supportedPeriods: null,
        requireDetails: true,
        renderablePeriods,
      }),
    ).toBe("");
    expect(
      resolveInstrumentRequestPeriod({
        preferredPeriod: "1m",
        providerPeriods: ["1m"],
        supportedPeriods: [],
        requireDetails: false,
        renderablePeriods,
      }),
    ).toBe("");
  });
});
