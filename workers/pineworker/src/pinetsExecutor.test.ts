import { describe, expect, test } from "vitest";
import { createNativePineTSExecutor, NativePineTSExecutor, normalizePineSourceForPineTS } from "./pinetsExecutor";
import { prepareCandleBatch, prepareRunScriptRequest } from "./preparedRequest";
import type { PreparedRunScriptRequest, RunScriptRequest } from "./types";

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

    const result = await executor.run(preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles: [{ openTime: 1, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
    }));

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

  test("reuses compatible candle arrays without remapping", async () => {
    const candles = [{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }];
    let receivedCandles: unknown[] | undefined;
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor(candles: unknown[]) {
          receivedCandles = candles;
        }

        async run() {
          return { plots: { close: [11] } };
        }
      },
    });

    const request = preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    });
    await executor.run(request);

    expect(receivedCandles).toBe(request.candles);
  });

  test("remaps candles with extra runtime fields before passing to PineTS", async () => {
    const candles = [{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100, extra: 1 }];
    let receivedCandles: unknown[] | undefined;
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor(candles: unknown[]) {
          receivedCandles = candles;
        }

        async run() {
          return { plots: { close: [11] } };
        }
      },
    });

    await executor.run(preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(receivedCandles).not.toBe(candles);
    expect(receivedCandles).toEqual([{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }]);
  });

  test("does not let native PineTS mutate reused request candles", async () => {
    const candles = [
      { openTime: 1_700_000_000_000, closeTime: 1_700_000_059_999, open: 10, high: 12, low: 9, close: 11, volume: 100 },
      { openTime: 1_700_000_060_000, closeTime: 1_700_000_119_999, open: 11, high: 13, low: 10, close: 12, volume: 110 },
    ];
    const before = JSON.stringify(candles);
    const executor = await createNativePineTSExecutor("pinets-test");

    await executor.run(preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")\nplot(close)`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(JSON.stringify(candles)).toBe(before);
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

    await executor.run(preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nstrategy("x")\nplot(timenow)`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles: [{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
    }));

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

function preparedRequest(request: RunScriptRequest): PreparedRunScriptRequest {
  const { candles, ...fields } = request;
  return prepareRunScriptRequest(fields, prepareCandleBatch(candles));
}
