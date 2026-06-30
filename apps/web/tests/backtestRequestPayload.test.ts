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
  instrumentType: "stock",
  interval: "5m",
  startDate: "2026-03-08",
  endDate: "2026-03-09",
  initialBalance: 100000,
  rehabType: "forward",
  useExtendedHours: true,
  brokerFeeMode: "market_preset",
  marketFeeMode: "market_preset",
  brokerFeeRules: [],
  marketFeeRules: [],
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
    expect(payload.instrumentType).toBe("stock");
    expect(payload.tradingCosts?.brokerFees?.mode).toBe("market_preset");
    expect(payload.tradingCosts?.brokerFees?.presetId).toBe("futu_hk_us_stock_2026_06_30");
    expect(payload.tradingCosts?.marketFees?.mode).toBe("market_preset");
    expect(payload.tradingCosts?.marketFees?.presetId).toBe("us_stock_market_fees_2026_06_30");
  });

  it("uses the same date labels for K-line synchronization", () => {
    const payload = buildBacktestSyncRequestPayload(formState, instrument);
    expect(payload.startDate).toBe("2026-03-08");
    expect(payload.endDate).toBe("2026-03-09");
    expect(payload).not.toHaveProperty("since");
    expect(payload).not.toHaveProperty("until");
  });
});
