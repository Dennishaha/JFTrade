import { describe, expect, it } from "vitest";

import {
  normalizeBacktestDateLabel,
} from "../src/pages/backtestTimeWindow";

describe("backtest time window helpers", () => {
  it("keeps date labels unchanged without constructing a browser Date", () => {
    expect(normalizeBacktestDateLabel(" 2026-05-20 ")).toBe("2026-05-20");
    expect(normalizeBacktestDateLabel("2024-02-29")).toBe("2024-02-29");
    expect(normalizeBacktestDateLabel("2026-02-29")).toBe("");
    expect(normalizeBacktestDateLabel("2026-13-01")).toBe("");
  });

  it("keeps a market date label independent of the browser timezone", () => {
    expect(normalizeBacktestDateLabel("2026-01-01")).toBe("2026-01-01");
  });
});
