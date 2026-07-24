import { describe, expect, it } from "vitest";

import {
  buildEarningsCalendarPath,
  createEarningsCalendarFilters,
  earningsCalendarFilterCount,
  earningsCalendarRange,
  moveEarningsCalendarAnchor,
  validateEarningsCalendarFilters,
} from "../../src/components/research/earningsCalendarModel";

describe("earningsCalendarModel", () => {
  it("calculates Sunday-first 35/42 day month ranges across years and leap years", () => {
    const july = earningsCalendarRange("month", "2026-07-23", "2026-07-23");
    expect(july.beginDate).toBe("2026-06-28");
    expect(july.endDate).toBe("2026-08-01");
    expect(july.days).toHaveLength(35);
    expect(july.days[0]?.weekday).toBe("周日");

    const august = earningsCalendarRange("month", "2026-08-01");
    expect(august.beginDate).toBe("2026-07-26");
    expect(august.endDate).toBe("2026-09-05");
    expect(august.days).toHaveLength(42);

    const leapFebruary = earningsCalendarRange("month", "2024-02-12");
    expect(leapFebruary.beginDate).toBe("2024-01-28");
    expect(leapFebruary.endDate).toBe("2024-03-02");
    expect(leapFebruary.days.some((day) => day.key === "2024-02-29")).toBe(true);
  });

  it("moves day, week, and month anchors without overflowing short months", () => {
    expect(moveEarningsCalendarAnchor("day", "2026-07-23", 1)).toBe("2026-07-24");
    expect(moveEarningsCalendarAnchor("week", "2026-07-23", -1)).toBe("2026-07-16");
    expect(moveEarningsCalendarAnchor("month", "2024-01-31", 1)).toBe("2024-02-29");
    expect(moveEarningsCalendarAnchor("month", "2024-03-31", -1)).toBe("2024-02-29");
  });

  it("validates ranges and converts 亿/万 to OpenD raw units", () => {
    const filters = {
      ...createEarningsCalendarFilters(),
      stockScope: "position" as const,
      marketCapMin: "1.25",
      marketCapMax: "10",
      optionVolumeMin: "2",
      ivMin: "10",
      ivMax: "90",
    };
    expect(validateEarningsCalendarFilters(filters, "US")).toEqual({
      valid: true,
      errors: {},
    });
    expect(earningsCalendarFilterCount(filters)).toBe(4);

    const path = buildEarningsCalendarPath({
      market: "US",
      range: { beginDate: "2026-07-23", endDate: "2026-07-23" },
      sort: "market_cap",
      filters,
    });
    expect(path).toContain("marketCapMin=125000000");
    expect(path).toContain("marketCapMax=1000000000");
    expect(path).toContain("optionVolumeMin=20000");
    expect(path).toContain("ivMin=10");
    expect(path).toContain("stockScope=position");
  });

  it("rejects negative, reversed, and over-100 percentage ranges", () => {
    const filters = {
      ...createEarningsCalendarFilters(),
      marketCapMin: "10",
      marketCapMax: "5",
      optionVolumeMin: "-1",
      ivMax: "101",
    };
    const result = validateEarningsCalendarFilters(filters, "US");
    expect(result.valid).toBe(false);
    expect(result.errors.marketCapMax).toContain("上限");
    expect(result.errors.optionVolumeMin).toContain("非负");
    expect(result.errors.ivMax).toContain("100");
  });

  it("drops option-only filters from mainland requests", () => {
    const path = buildEarningsCalendarPath({
      market: "SH",
      range: { beginDate: "2026-07-23", endDate: "2026-07-23" },
      sort: "hot",
      filters: {
        ...createEarningsCalendarFilters(),
        marketCapMin: "2",
        ivMin: "10",
        optionVolumeMax: "5",
      },
    });
    expect(path).toContain("marketCapMin=200000000");
    expect(path).not.toContain("ivMin");
    expect(path).not.toContain("optionVolumeMax");
  });
});
