import { describe, expect, it } from "vitest";

import {
  createKlinePriceFormat,
  formatKlinePrice,
  formatKlineVolume,
  resolveKlinePricePrecision,
} from "../src/charting/lightweightChartsFormatting";

describe("lightweight chart value formatting", () => {
  it("keeps regular prices within three decimals", () => {
    expect(formatKlinePrice(0)).toBe("0");
    expect(formatKlinePrice(23.649999999999999)).toBe("23.65");
    expect(formatKlinePrice(110.1116)).toBe("110.112");
    expect(formatKlinePrice(-12.3456)).toBe("-12.346");
    expect(formatKlinePrice(0.001)).toBe("0.001");
  });

  it("uses compact scientific notation only for very small nonzero prices", () => {
    expect(formatKlinePrice(0.00012345678)).toBe("1.23e-4");
    expect(formatKlinePrice(-0.00001)).toBe("-1e-5");
    expect(formatKlinePrice(Number.POSITIVE_INFINITY)).toBe("Infinity");
    expect(formatKlinePrice(Number.NaN)).toBe("NaN");
  });

  it("preserves the underlying tick precision independently of display precision", () => {
    expect(resolveKlinePricePrecision([])).toBe(3);
    expect(
      resolveKlinePricePrecision([
        {
          at: "2026-07-25T00:00:00Z",
          open: 91.5,
          high: 92.125,
          low: 90.75,
          close: 91.55,
          volume: 1_000,
        },
      ]),
    ).toBe(3);
    expect(
      resolveKlinePricePrecision([
        {
          at: "2026-07-25T00:00:00Z",
          open: 0.00012341,
          high: 0.00012349,
          low: 0.0001234,
          close: 0.00012345,
          volume: 1_000,
        },
      ]),
    ).toBe(8);
    expect(createKlinePriceFormat(8).minMove).toBe(0.00000001);
  });

  it("formats volume with stable K, M, and B boundaries", () => {
    expect(formatKlineVolume(999)).toBe("999");
    expect(formatKlineVolume(1_200)).toBe("1.2K");
    expect(formatKlineVolume(-12_500)).toBe("-12.5K");
    expect(formatKlineVolume(999_999)).toBe("1M");
    expect(formatKlineVolume(60_000_000)).toBe("60M");
    expect(formatKlineVolume(1_250_000_000)).toBe("1.25B");
    expect(formatKlineVolume(Number.POSITIVE_INFINITY)).toBe("Infinity");
  });
});
