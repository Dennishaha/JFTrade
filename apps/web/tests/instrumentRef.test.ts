import { describe, expect, it } from "vitest";

import { normalizeInstrumentId, resolveInstrumentRef } from "../src/composables/instrumentRef";
import { buildBacktestInstrumentPayload } from "../src/composables/useBacktestRuns";

describe("instrumentRef", () => {
  it("normalizes colon-form instrument ids", () => {
    expect(normalizeInstrumentId("us:tme")).toBe("US.TME");
  });

  it("resolves explicit market and code into a canonical instrument ref", () => {
    expect(resolveInstrumentRef({ market: "US", code: "tme" })).toEqual({
      market: "US",
      code: "TME",
      symbol: "TME",
      instrumentId: "US.TME",
    });
  });

  it("preserves exchange-qualified A-share symbols", () => {
    expect(resolveInstrumentRef({ instrumentId: "sh.600519" })).toEqual({
      market: "SH",
      code: "600519",
      symbol: "600519",
      instrumentId: "SH.600519",
    });
  });

  it("builds backtest payloads from explicit market and code", () => {
    expect(buildBacktestInstrumentPayload({
      market: "US",
      code: "AAPL",
      instrumentId: "US.AAPL",
    })).toEqual({
      market: "US",
      code: "AAPL",
      symbol: "US.AAPL",
    });
  });
});