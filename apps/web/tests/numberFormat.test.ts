import { describe, expect, it } from "vitest";

import {
  formatCompactNumber,
  formatMarketPrice,
  formatMoney,
  formatNumber,
  formatPercent,
  formatQuantity,
  marketPricePrecision,
} from "../src/utils/numberFormat";

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
    expect(formatMarketPrice(12.3, { market: "US" })).toBe("12.30");
    expect(formatMarketPrice(12.3, { market: "HK" })).toBe("12.300");
    expect(formatMarketPrice(8.7654, { market: "SH" })).toBe("8.77");
    expect(formatMarketPrice(0.123456, { market: "UNKNOWN" })).toBe("0.12346");
    expect(formatMarketPrice(1_234.5, { market: "UNKNOWN" })).toBe("1,234.50");
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
