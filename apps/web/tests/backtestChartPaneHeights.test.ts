import { describe, expect, it } from "vitest";

import { resolveBacktestChartPaneHeights } from "../src/components/backtestChartPaneHeights";

describe("resolveBacktestChartPaneHeights", () => {
  it("distributes normal chart height across four panes without drift", () => {
    const heights = resolveBacktestChartPaneHeights(480);
    expect(heights).toEqual([251, 57, 105, 67]);
    expect(heights.reduce((sum, height) => sum + height, 0)).toBe(480);
  });

  it("scales minimum pane heights down for compact containers", () => {
    const heights = resolveBacktestChartPaneHeights(260);
    expect(heights).toEqual([137, 33, 55, 35]);
    expect(heights.reduce((sum, height) => sum + height, 0)).toBe(260);
  });
});
