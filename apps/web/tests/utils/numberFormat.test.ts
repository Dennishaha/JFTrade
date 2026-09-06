import { describe, expect, it } from "vitest";

import {
  formatCompactNumber,
  formatMarketPrice,
  formatMoney,
  formatNumber,
  formatPercent,
  formatQuantity,
  marketPricePrecision,
} from "../../src/utils/numberFormat";

describe("number formatting contract", () => {
  it("uses one empty fallback and groups large values", () => {
    expect(formatNumber(null)).toBe("—");
    expect(formatNumber(Number.NaN)).toBe("—");
    expect(formatNumber(12_345.67891)).toBe("12,345.6789");
    expect(formatQuantity(12_345.678901234)).toBe("12,345.67890123");
  });

  it("formats prices from the market precision contract", () => {
    expect(marketPricePrecision("NASDAQ")).toBe(2);
    expect(marketPricePrecision("HKEX")).toBe(3);
    expect(marketPricePrecision("US.AAPL")).toBe(2);
    expect(marketPricePrecision("AAPL.US")).toBe(2);
    expect(marketPricePrecision("HK.00700")).toBe(3);
    expect(marketPricePrecision("00700.HK")).toBe(3);
    expect(marketPricePrecision("SH.600519")).toBe(2);
    expect(marketPricePrecision("600519.SH")).toBe(2);
    expect(formatMarketPrice(12.3, { market: "US" })).toBe("12.30");
    expect(formatMarketPrice(12.3, { market: "HK" })).toBe("12.300");
    expect(formatMarketPrice(8.7654, { market: "SH" })).toBe("8.77");
    expect(formatMarketPrice(0.123456, { market: "UNKNOWN" })).toBe("0.12346");
    expect(formatMarketPrice(1_234.5, { market: "UNKNOWN" })).toBe("1,234.50");
    // Preserves decimals when market qualifier is present or uninitialized precision
    expect(formatMarketPrice(12.3, { market: "US", precision: null })).toBe("12.30");
    expect(formatMarketPrice(189.5, { market: "US.AAPL" })).toBe("189.50");
    expect(formatMarketPrice(189.5, { market: "AAPL.US" })).toBe("189.50");
    expect(formatMarketPrice(380.2, { market: "HK.00700" })).toBe("380.200");
    expect(formatMarketPrice(380.2, { market: "00700.HK" })).toBe("380.200");
    expect(formatMarketPrice(12.3456, { precision: 4 })).toBe("12.3456");
    expect(formatMarketPrice(12.3, { precision: 0 })).toBe("12");
  });

  it("keeps compact, percent, and money outputs deterministic", () => {
    expect(formatCompactNumber(1_500)).toBe("1.5K");
    expect(formatCompactNumber(1_500_000)).toBe("1.50M");
    expect(formatCompactNumber(-1_500_000_000)).toBe("-1.50B");
    expect(formatPercent(1.25, { showPositiveSign: true })).toBe("+1.25%");
    expect(formatPercent(0.125, { input: "ratio" })).toBe("12.50%");
    expect(formatMoney(12_345.6, "USD", { maximumFractionDigits: 2 })).toBe(
      "12,345.6 USD",
    );
  });
});
