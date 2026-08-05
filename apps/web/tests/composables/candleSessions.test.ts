import { describe, expect, it } from "vitest";

import {
  candleMatchesSessions,
  candleSessionForValue,
  intersectCandleSessions,
  normalizeCandleSessions,
  summarizeCandleSessions,
} from "@/composables/market-data/candleSessions";

describe("candle session selection", () => {
  it("normalizes CSV and repeated values in canonical order", () => {
    expect(normalizeCandleSessions(["overnight,regular", "extended", "regular"])).toEqual([
      "regular",
      "extended",
      "overnight",
    ]);
  });

  it("retains only available sessions and summarizes common combinations", () => {
    expect(intersectCandleSessions(["regular", "overnight"], ["regular", "extended"])).toEqual(["regular"]);
    expect(summarizeCandleSessions(["regular"])).toBe("盘中");
    expect(summarizeCandleSessions(["regular", "extended"])).toBe("盘中+盘前后");
    expect(summarizeCandleSessions(["regular", "extended", "overnight"])).toBe("全天");
  });

  it("maps pre and after candles to the extended selection", () => {
    expect(candleMatchesSessions("pre", ["extended"])).toBe(true);
    expect(candleMatchesSessions("after", ["regular"])).toBe(false);
    expect(candleMatchesSessions(undefined, ["regular"])).toBe(true);
  });

  it("drops empty and unknown values while preserving null-safe defaults", () => {
    expect(normalizeCandleSessions(undefined)).toEqual([]);
    expect(normalizeCandleSessions([null, "", "unknown", " overnight "])).toEqual([
      "overnight",
    ]);
    expect(summarizeCandleSessions(["extended", "overnight"])).toBe("盘前后+夜盘");
    expect(candleSessionForValue("rth")).toBe("regular");
    expect(candleSessionForValue("eth")).toBe("extended");
    expect(candleSessionForValue("overnight")).toBe("overnight");
    expect(candleSessionForValue("unclassified")).toBeNull();
  });
});
