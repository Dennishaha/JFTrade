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

  it("builds custom, disabled, and regional fee schedules", () => {
    const customRule = {
      id: "custom-fee",
      label: "Custom fee",
      group: "broker" as const,
      category: "commission" as const,
      currency: "HKD",
      model: "rate" as const,
      rate: 0.001,
    };
    const custom = buildBacktestStartRequestPayload({
      ...formState,
      instrumentType: " ETF ",
      brokerFeeMode: "custom",
      marketFeeMode: "none",
      brokerFeeRules: [customRule],
    }, { market: "HK", code: "00700", symbol: "HK.00700" });
    expect(custom.instrumentType).toBe("etf");
    expect(custom.tradingCosts?.brokerFees).toEqual({ mode: "custom", rules: [customRule] });
    expect(custom.tradingCosts?.marketFees).toEqual({ mode: "none" });

    const connect = buildBacktestStartRequestPayload(formState, {
      market: "SH",
      code: "600000",
      symbol: "SH.600000",
    });
    expect(connect.tradingCosts?.brokerFees).toEqual({ mode: "market_preset" });
    expect(connect.tradingCosts?.marketFees?.presetId).toBe("stock_connect_a_share_market_fees_2026_06_30");

    const unknown = buildBacktestStartRequestPayload({
      ...formState,
      brokerFeeMode: "script",
      marketFeeMode: "market_preset",
    }, { market: "CRYPTO", code: "BTC", symbol: "CRYPTO.BTC" });
    expect(unknown.tradingCosts?.brokerFees).toEqual({ mode: "script" });
    expect(unknown.tradingCosts?.marketFees).toEqual({ mode: "market_preset" });
  });

  it("uses regular session scope when extended hours are disabled", () => {
    expect(buildBacktestSyncRequestPayload({ ...formState, useExtendedHours: false }, instrument).sessionScope).toBe("regular");
  });
});
