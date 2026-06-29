import { describe, expect, test } from "bun:test";
import { NativePineTSExecutor } from "./pinetsExecutor";

describe("NativePineTSExecutor", () => {
  test("passes custom candles and Pine source to PineTS", async () => {
    const calls: unknown[] = [];
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor(candles: unknown[], symbol?: string, timeframe?: string, periods?: number) {
          calls.push({ candles, symbol, timeframe, periods });
        }

        setAlertMode(mode: string) {
          calls.push({ alertMode: mode });
        }

        async run(source: string, periods?: number) {
          calls.push({ source, periods });
          return { plots: { close: [11] } };
        }
      },
    }, "pinets-test");

    const result = await executor.run({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles: [{ openTime: 1, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
    });

    expect(executor.version()).toBe("pinets-test");
    expect(calls[0]).toEqual({
      candles: [{ openTime: 1, closeTime: 1, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
      symbol: "US.AAPL",
      timeframe: "1",
      periods: 1,
    });
    expect(calls[1]).toEqual({ alertMode: "all" });
    expect(calls[2]).toEqual({ source: expect.stringContaining("indicator"), periods: 1 });
    expect(result.plots?.close).toEqual([11]);
  });
});
