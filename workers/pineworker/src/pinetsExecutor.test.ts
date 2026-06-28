import { describe, expect, test } from "bun:test";
import { NativePineTSExecutor } from "./pinetsExecutor";

describe("NativePineTSExecutor", () => {
  test("passes custom candles and Pine source to PineTS", async () => {
    const calls: unknown[] = [];
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor(candles: unknown[]) {
          calls.push(candles);
        }

        async run(source: string) {
          calls.push(source);
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
    expect(calls[0]).toEqual([{ openTime: 1, closeTime: 1, open: 10, high: 12, low: 9, close: 11, volume: 100 }]);
    expect(calls[1]).toContain("indicator");
    expect(result.plots?.close).toEqual([11]);
  });
});
