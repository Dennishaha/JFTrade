import { describe, expect, it } from "vitest";

import {
  nextTechnicalIndicatorConditionNodeText,
  patternTypeLabel,
} from "../src/features/strategyVisualBuilderIndicatorBlock";

describe("strategy visual indicator condition labels", () => {
  it("keeps the displayed condition labels aligned with executable indicator patterns", () => {
    expect(
      nextTechnicalIndicatorConditionNodeText({
        indicatorType: "movingAverage",
        conditionMode: "pattern",
        patternType: "goldenCross",
      }),
    ).toContain("金叉");
    expect(
      nextTechnicalIndicatorConditionNodeText({
        indicatorType: "macd",
        conditionMode: "pattern",
        patternType: "deathCross",
      }),
    ).toContain("死叉");
    expect(
      nextTechnicalIndicatorConditionNodeText({
        indicatorType: "rsi",
        conditionMode: "pattern",
        patternType: "topDivergence",
        lookback: 8,
      }),
    ).toContain("顶背离 (8)");
    expect(patternTypeLabel("bottomDivergence")).toBe("底背离");
    expect(patternTypeLabel("closeAboveUpperBand")).toBe("突破上轨");
    expect(patternTypeLabel("closeBelowLowerBand")).toBe("跌破下轨");
    expect(patternTypeLabel("legacy-pattern" as never)).toBe("形态");
  });
});
