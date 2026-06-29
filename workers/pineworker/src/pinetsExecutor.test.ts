import { describe, expect, test } from "vitest";
import { NativePineTSExecutor, normalizePineSourceForPineTS } from "./pinetsExecutor";

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

  test("normalizes timenow to PineTS-supported time_close", async () => {
    const calls: unknown[] = [];
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor() {}

        async run(source: string, periods?: number) {
          calls.push({ source, periods });
          return { plots: { now: [1] } };
        }
      },
    });

    await executor.run({
      jobId: "job-1",
      source: `//@version=6\nstrategy("x")\nplot(timenow)`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles: [{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
    });

    expect(calls[0]).toEqual({ source: expect.stringContaining("plot(time_close)"), periods: 1 });
  });

  test("does not normalize timenow inside comments, strings, or longer identifiers", () => {
    const source = [
      `plot(timenow)`,
      `plot(my_timenow_value)`,
      `// timenow stays in comment`,
      `label.new(bar_index, close, "timenow stays in string")`,
      `/* timenow stays in block */`,
    ].join("\n");

    expect(normalizePineSourceForPineTS(source)).toBe([
      `plot(time_close)`,
      `plot(my_timenow_value)`,
      `// timenow stays in comment`,
      `label.new(bar_index, close, "timenow stays in string")`,
      `/* timenow stays in block */`,
    ].join("\n"));
  });
});
