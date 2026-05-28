import { describe, expect, it } from "vitest";

import {
  buildBacktestDayInclusiveEndTime,
  buildBacktestDayStartTime,
} from "../src/pages/backtestTimeWindow";

describe("backtest time window helpers", () => {
  it("builds a UTC day start timestamp for sync and backtest queries", () => {
    expect(buildBacktestDayStartTime("2026-05-20")).toBe(
      "2026-05-20T00:00:00Z",
    );
  });

  it("keeps the end date inclusive through the last millisecond of the day", () => {
    expect(buildBacktestDayInclusiveEndTime(" 2026-05-20 ")).toBe(
      "2026-05-20T23:59:59.999Z",
    );
  });
});