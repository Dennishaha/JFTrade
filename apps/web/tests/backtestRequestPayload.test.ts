import { describe, expect, it } from "vitest";

import {
  buildBacktestStartRequestPayload,
  buildBacktestSyncRequestPayload,
  type BacktestFormState,
} from "../src/composables/useBacktestRuns";

const formState: BacktestFormState = {
  definitionId: "strategy-1",
  definitionVersion: "v1",
  market: "US",
  code: "AAPL",
  instrumentId: "US.AAPL",
  interval: "5m",
  startDate: "2026-03-08",
  endDate: "2026-03-09",
  initialBalance: 100000,
  rehabType: "forward",
  useExtendedHours: true,
};

const instrument = {
  market: "US",
  code: "AAPL",
  symbol: "US.AAPL",
};

describe("backtest request payloads", () => {
  it("submits market date labels without frontend timezone conversion", () => {
    const payload = buildBacktestStartRequestPayload(formState, instrument);
    expect(payload.startDate).toBe("2026-03-08");
    expect(payload.endDate).toBe("2026-03-09");
    expect(payload).not.toHaveProperty("startTime");
    expect(payload).not.toHaveProperty("endTime");
  });

  it("uses the same date labels for K-line synchronization", () => {
    const payload = buildBacktestSyncRequestPayload(formState, instrument);
    expect(payload.startDate).toBe("2026-03-08");
    expect(payload.endDate).toBe("2026-03-09");
    expect(payload).not.toHaveProperty("since");
    expect(payload).not.toHaveProperty("until");
  });
});
