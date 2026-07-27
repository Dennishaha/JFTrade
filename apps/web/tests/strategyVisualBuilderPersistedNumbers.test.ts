import { describe, expect, it } from "vitest";

import {
  normalizeDerivedSeriesBlockProperties,
  normalizeMtfSeriesBlockProperties,
  normalizeSeriesConditionBlockProperties,
  normalizeTimeFilterBlockProperties,
} from "../src/features/strategyVisualBuilderCatalog";

describe("strategy visual block persisted-number normalization", () => {
  it("normalizes string-backed numeric controls from persisted visual strategy documents", () => {
    expect(
      normalizeSeriesConditionBlockProperties({
        mode: "valuewhen",
        occurrence: "-2.6",
      }).occurrence,
    ).toBe(0);
    expect(
      normalizeDerivedSeriesBlockProperties({ historyOffset: "3.6" }).historyOffset,
    ).toBe(4);
    expect(
      normalizeMtfSeriesBlockProperties({ historyOffset: "not-a-number" }).historyOffset,
    ).toBe(1);

    expect(
      normalizeTimeFilterBlockProperties({
        mode: "between",
        startHour: "8.6",
        startMinute: "-1",
        endHour: "30",
        endMinute: "59.6",
        dayOfWeek: "9",
      }),
    ).toMatchObject({
      startHour: 9,
      startMinute: 0,
      endHour: 23,
      endMinute: 59,
      dayOfWeek: 7,
    });
  });
});
