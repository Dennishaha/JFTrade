import { describe, expect, test } from "bun:test";
import { normalizeMode, validateRunScriptRequest } from "./validation";
import type { RunScriptRequest } from "./types";

describe("validateRunScriptRequest", () => {
  test("accepts a valid backtest request", () => {
    expect(validateRunScriptRequest(validRequest())).toBe("backtest");
  });

  test("allows analyze mode without candles", () => {
    const request = { ...validRequest(), mode: "analyze", candles: [] };
    expect(validateRunScriptRequest(request)).toBe("analyze");
  });

  test("rejects malformed requests before dispatch", () => {
    const cases: Array<[string, (request: RunScriptRequest) => void, string]> = [
      ["job", (request) => { request.jobId = ""; }, "job id is required"],
      ["mode", (request) => { request.mode = "scan"; }, "unsupported pine worker mode"],
      ["candles", (request) => { request.candles = []; }, "candles are required"],
      ["range", (request) => { request.candles[0]!.high = 8; }, "high is below low"],
      ["volume", (request) => { request.candles[0]!.volume = -1; }, "volume is negative"],
      ["params", (request) => { request.params = { a: "1", b: "2" }; }, "param count exceeds limit"],
    ];

    for (const [name, mutate, message] of cases) {
      const request = validRequest();
      mutate(request);
      expect(() => validateRunScriptRequest(request, { maxCandles: 10, maxSourceBytes: 200, maxParamCount: 1 }), name)
        .toThrow(message);
    }
  });
});

describe("normalizeMode", () => {
  test("defaults empty mode to backtest", () => {
    expect(normalizeMode(undefined)).toBe("backtest");
    expect(normalizeMode(" LIVE ")).toBe("live");
  });
});

function validRequest(): RunScriptRequest {
  return {
    jobId: "job-1",
    scriptId: "script-1",
    source: `//@version=6\nstrategy("x")`,
    symbol: "US.AAPL",
    timeframe: "1",
    mode: "backtest",
    candles: [{
      openTime: 1_700_000_000_000,
      closeTime: 1_700_000_060_000,
      open: 10,
      high: 12,
      low: 9,
      close: 11,
      volume: 100,
    }],
    params: { threshold: "10" },
  };
}
